package adapter

// claude.go — the ONE host adapter that exists in v0.8.0.
//
// Everything in this file is wire vocabulary: the exact JSON field spellings
// Claude Code sends on stdin and expects back on stdout. That vocabulary is
// confined to this file BY DESIGN, and the confinement is checked rather than
// trusted — tools/edikt/check/no-claude-leak-in-hook-core.sh fails when any
// file under internal/hook/ outside this directory names one of them.
//
// The inbound half does in Go exactly what the embedded Python in
// pre-tool-use.sh does today (json.loads over the payload, then `.get()` per
// field). The outbound half replaces the hand-built printf JSON the shims fall
// back to. Both go through encoding/json — INV-003: a struct cannot express
// the unterminated string a printf format can, which is the whole reason the
// invariant exists.
//
// A SECOND ADAPTER IS ADDITIVE. When BRAIN-009's port lands, it adds a sibling
// file with its own Parse/Encode pair and the dispatcher chooses between them.
// No handler changes, because no handler ever knew any of this.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/diktahq/edikt/tools/edikt/internal/claudepaths"
)

// HostID names this adapter for the journal and for diagnostics. Written as a
// two-token identifier rather than a bare vendor word so ADR-030's tier-2
// purity gate — which greps every non-test Go source for a bare quoted vendor
// literal as an LLM shell-out signature — does not read a host-protocol label
// as an attempt to spawn an LLM CLI. Different concerns, similar-looking text.
const HostID = "claude-code"

// claudePayload is the wire shape. Every field is a pointer or a plain value
// decoded from JSON, and the struct exists ONLY so encoding/json can populate
// it — nothing outside this file ever sees it.
type claudePayload struct {
	HookEventName  string         `json:"hook_event_name"`
	SessionID      string         `json:"session_id"`
	Cwd            string         `json:"cwd"`
	ToolName       string         `json:"tool_name"`
	ToolInput      map[string]any `json:"tool_input"`
	Prompt         string         `json:"prompt"`
	Source         string         `json:"source"`
	AgentID        string         `json:"agent_id"`
	AgentType      string         `json:"agent_type"`
	SubagentType   string         `json:"subagent_type"`
	TaskID         string         `json:"task_id"`
	TaskName       string         `json:"task_name"`
	WorktreePath   string         `json:"worktree_path"`
	TranscriptPath string         `json:"transcript_path"`
	// AgentTranscriptPath is the SUBAGENT's own message stream, distinct from
	// transcript_path (which is the PARENT session on every event that carries
	// it, SubagentStop included). Measured 2026-08-30: on one dispatch
	// transcript_path was a 14-line parent file and agent_transcript_path a
	// 4-line subagent file at <session>/subagents/agent-<agent_id>.jsonl.
	// Present ONLY on SubagentStop. This is what makes per-turn subagent
	// detail reachable from the hook path at all.
	AgentTranscriptPath string `json:"agent_transcript_path"`
	// PromptID brackets ONE user turn and is carried by UserPromptSubmit,
	// Stop, SubagentStart and SubagentStop alike — observed identical across
	// all four in a single run, which is what joins a parent turn to the
	// dispatches it spawned without inventing an identifier.
	PromptID             string `json:"prompt_id"`
	LastAssistantMessage string `json:"last_assistant_message"`
	StopHookActive       bool   `json:"stop_hook_active"`
	// FilePath is the FileChanged event's own top-level spelling —
	// file-changed.sh's `d.get('file_path','')` reads it off the payload
	// root, never off tool_input (FileChanged carries no tool_input at all).
	// A second, independent source from tool_input.file_path; see
	// Payload.FilePath's own doc comment for why one Go field absorbs both.
	FilePath string `json:"file_path"`

	// ID/Name/Subject/Status/Result/ProposedPath/PathTop/Agent are top-level
	// fallback spellings TaskCreated/TaskCompleted/WorktreeCreate/
	// WorktreeRemove/SubagentStart read alongside their primary fields —
	// see each Payload field's own doc comment for which hook reads which.
	ID           string `json:"id"`
	Name         string `json:"name"`
	Subject      string `json:"subject"`
	Status       string `json:"status"`
	Result       string `json:"result"`
	ProposedPath string `json:"proposed_path"`
	PathTop      string `json:"path"`
	Agent        string `json:"agent"`

	// AgentName is the FIRST spelling subagent-stop.sh's identity fallback
	// tries (`for k in ("agent_name", "subagent_type", "tool_name",
	// "agent")`), ahead of the three already modelled above. A fifth English
	// spelling of "which agent", kept separate for the same reason
	// SubagentTypeField is: the fallback ORDER differs per hook and belongs
	// to the handler, not to this promotion.
	AgentName string `json:"agent_name"`

	// EvaluatorOutput is edikt's own structured verdict object, carried on a
	// SubagentStop payload by the evaluator agent that produced it. Left as
	// json.RawMessage rather than a typed sub-struct for exactly the reason
	// Error is (see below): subagent-stop.sh reads it through
	// `d.get("evaluator_output", {}).get(...)` wrapped in a bare `except
	// Exception`, so a non-object value, a null, or a field of the wrong type
	// degrades to "nothing was extracted" instead of aborting. A typed field
	// would fail the WHOLE payload decode on those shapes and turn a
	// degraded-but-running hook into one the dispatcher journals as
	// payload-unparseable. See parseEvaluatorOutput below for the extraction.
	EvaluatorOutput json.RawMessage `json:"evaluator_output"`

	// Error is left as json.RawMessage, not a typed struct: stop-failure.sh's
	// python extraction is `d.get('error',{}).get('type','unknown')` inside a
	// `2>/dev/null || echo "unknown"` shell fallback — a malformed `error`
	// value (a string, a number, anything not an object) degrades to
	// "unknown"/"" rather than aborting. A typed *claudeError field would fail
	// the WHOLE payload decode on exactly that shape, turning a degraded-but-
	// running hook into one the dispatcher journals as payload-unparseable —
	// a worse failure than the one being replaced. See the manual extraction
	// below, which reproduces the same graceful degradation.
	Error json.RawMessage `json:"error"`
}

// Parse decodes a Claude Code hook payload into the host-neutral Payload.
//
// EMPTY STDIN IS NOT AN ERROR. Several events fire with no body at all, and
// treating that as a parse failure would journal a suppression for a hook that
// was working perfectly. Malformed-but-present JSON IS an error, and the
// dispatcher gives it its own journal type rather than letting it look like a
// quiet, successful run.
func Parse(raw []byte) (Payload, error) {
	p := Payload{present: map[string]bool{}, raw: raw}
	if len(trimSpace(raw)) == 0 {
		return p, nil
	}

	// Decode twice, deliberately: once into the typed struct for the values,
	// once into a generic map for PRESENCE. Go's decoder cannot distinguish an
	// absent string from a present empty one, and that difference is the thing
	// Payload.Present exists to answer. Re-deriving presence from the typed
	// struct ("it's non-empty, so it was present") would answer a different
	// question and be wrong in exactly the case anyone would ask about.
	var wire claudePayload
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Payload{}, fmt.Errorf("decode host payload: %w", err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return Payload{}, fmt.Errorf("decode host payload keys: %w", err)
	}

	p.Event = wire.HookEventName
	p.SessionID = wire.SessionID
	p.Cwd = wire.Cwd
	p.ToolName = wire.ToolName
	p.ToolInput = wire.ToolInput
	p.Prompt = wire.Prompt
	p.Source = wire.Source
	p.AgentID = wire.AgentID
	p.AgentType = wire.AgentType
	if p.AgentType == "" {
		// Two spellings for one fact, and which one arrives depends on the
		// event. Preferring agent_type keeps the newer spelling authoritative
		// while an older payload still attributes correctly instead of
		// silently reading as the parent session.
		p.AgentType = wire.SubagentType
	}
	p.TaskID = wire.TaskID
	p.TaskName = wire.TaskName
	p.WorktreePath = wire.WorktreePath
	p.TranscriptPath = wire.TranscriptPath
	p.AgentTranscriptPath = wire.AgentTranscriptPath
	p.PromptID = wire.PromptID
	p.LastAssistantMessage = wire.LastAssistantMessage
	p.StopActive = wire.StopHookActive
	p.ID = wire.ID
	p.Name = wire.Name
	p.Subject = wire.Subject
	p.Status = wire.Status
	p.Result = wire.Result
	p.ProposedPath = wire.ProposedPath
	p.TopLevelPath = wire.PathTop
	p.SubagentTypeField = wire.SubagentType
	p.AgentField = wire.Agent
	p.AgentNameField = wire.AgentName
	p.EvaluatorAgent, p.EvaluatorSeverity, p.EvaluatorFinding = parseEvaluatorOutput(wire.EvaluatorOutput)

	// error.type / error.message — see the wire struct's own comment for why
	// this is a manual, lenient extraction rather than a typed sub-struct.
	if len(wire.Error) > 0 && !bytes.Equal(bytes.TrimSpace(wire.Error), []byte("null")) {
		var eobj map[string]json.RawMessage
		if json.Unmarshal(wire.Error, &eobj) == nil {
			if raw, ok := eobj["type"]; ok {
				var s string
				if json.Unmarshal(raw, &s) == nil {
					p.ErrorType = s
					p.present["ErrorType"] = true
				}
			}
			if raw, ok := eobj["message"]; ok {
				var s string
				if json.Unmarshal(raw, &s) == nil {
					p.ErrorMessage = s
					p.present["ErrorMessage"] = true
				}
			}
		}
	}

	// Promoted tool-input fields.
	p.FilePath, _ = p.Field("file_path")
	if p.FilePath == "" {
		// FileChanged's own top-level spelling — see Payload.FilePath's doc
		// comment. No event sends both, so this fallback never shadows a
		// tool_input value a handler actually wanted.
		p.FilePath = wire.FilePath
	}
	p.OldString, _ = p.Field("old_string")
	p.NewString, _ = p.Field("new_string")
	p.Content, _ = p.Field("content")
	p.Path, _ = p.Field("path")

	for wireName, goName := range presenceMap {
		if _, ok := keys[wireName]; ok {
			p.present[goName] = true
		}
	}
	for wireName, goName := range toolInputPresenceMap {
		if _, ok := wire.ToolInput[wireName]; ok {
			p.present[goName] = true
		}
	}
	return p, nil
}

// parseEvaluatorOutput reproduces subagent-stop.sh's THREE separate reads of
// evaluator_output, including which of them fail together and which fail
// alone. Getting that grouping wrong is not a cosmetic difference: agent and
// severity decide identity and blocking, and they are read by ONE python
// expression whose exception unwinds BOTH.
//
//	eo = d.get("evaluator_output", {})
//	print(eo.get("agent","").strip().lower() + "|" + eo.get("severity","").strip().lower())
//
// The concatenation is evaluated before either half is used, so a severity
// that is a number, a null, or a list raises AttributeError on `.strip()` and
// the WHOLE line degrades to "|" — an agent that was present, well-formed and
// allowlisted reads as absent. Hence the both-or-neither rule below.
//
// findings is a SECOND python invocation with its own try, so a broken agent
// or severity does not suppress it (and vice versa). Returned raw and
// untruncated; `str(desc)[:120]` is the handler's business rule, not a wire
// fact.
//
// The `.strip().lower()` normalisation is deliberately NOT applied here.
// Callers get exactly what the host sent; how a hook canonicalises it is the
// hook's own contract.
func parseEvaluatorOutput(raw json.RawMessage) (agent, severity, finding string) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", "", ""
	}
	var eo map[string]json.RawMessage
	if json.Unmarshal(raw, &eo) != nil {
		// Not an object at all: python's `.get` on a str/list/int raises, so
		// every read below would have degraded together.
		return "", "", ""
	}
	a, aOK := lenientString(eo["agent"])
	s, sOK := lenientString(eo["severity"])
	if aOK && sOK {
		agent, severity = a, s
	}
	return agent, severity, firstFindingDescription(eo["findings"])
}

// lenientString reports the value as python's `x.get(k, "")` would, and
// whether the follow-on `.strip()` would have survived it. An ABSENT key is
// the empty string and survives (that is the `""` default); a present
// non-string value does not.
func lenientString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", true
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// firstFindingDescription mirrors:
//
//	findings = eo.get("findings", [])
//	if findings:
//	    item = findings[0]
//	    desc = item.get("description","") if isinstance(item, dict) else str(item)
//	    print(str(desc)[:120])
//
// KNOWN, BOUNDED DIVERGENCE. python's `str()` renders any object, so a
// findings[0] that is a LIST or a nested OBJECT would print its python repr
// (`['a']`, `{'k': 'v'}` — single quotes and all). Reproducing python repr in
// Go for arbitrary JSON is not worth what it would cost to be exactly wrong
// in a new way, so those two shapes yield "" here. Scalars ARE reproduced,
// because they are plausible in a real hand-written evaluator payload:
// numbers keep their source spelling, and true/false/null render as python
// spells them. The empty-array and absent cases are the same "" in both.
func firstFindingDescription(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil || len(items) == 0 {
		return ""
	}
	first := items[0]
	var obj map[string]json.RawMessage
	if json.Unmarshal(first, &obj) == nil {
		return pythonScalarString(obj["description"])
	}
	return pythonScalarString(first)
}

// pythonScalarString renders one JSON value the way python's str() would,
// for the scalar cases only. A string is itself; everything structural is ""
// (see firstFindingDescription's own note).
func pythonScalarString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	trimmed := string(bytes.TrimSpace(raw))
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	switch trimmed {
	case "true":
		return "True"
	case "false":
		return "False"
	case "null":
		return "None"
	}
	if len(trimmed) > 0 && (trimmed[0] == '-' || (trimmed[0] >= '0' && trimmed[0] <= '9')) {
		return trimmed
	}
	return ""
}

// presenceMap maps each top-level wire key to the Go field name callers pass
// to Payload.Present. One table, so the two spellings of the same fact cannot
// drift (GL-002).
var presenceMap = map[string]string{
	"hook_event_name":        "Event",
	"session_id":             "SessionID",
	"cwd":                    "Cwd",
	"tool_name":              "ToolName",
	"tool_input":             "ToolInput",
	"prompt":                 "Prompt",
	"source":                 "Source",
	"agent_id":               "AgentID",
	"agent_type":             "AgentType",
	"subagent_type":          "AgentType",
	"task_id":                "TaskID",
	"task_name":              "TaskName",
	"worktree_path":          "WorktreePath",
	"transcript_path":        "TranscriptPath",
	"agent_transcript_path":  "AgentTranscriptPath",
	"prompt_id":              "PromptID",
	"last_assistant_message": "LastAssistantMessage",
	"stop_hook_active":       "StopActive",
	"file_path":              "FilePath", // FileChanged's top-level spelling

	// TaskCreated's own TaskName default ("unknown") only applies when
	// "subject" is genuinely ABSENT, not present-and-empty; TaskCompleted's
	// own Status default ("completed") is the same shape for "result". Both
	// need Present(), unlike every other fallback field above, whose chains
	// use the same "" default at every step so presence never changes the
	// outcome.
	"subject": "Subject",
	"result":  "Result",
}

var toolInputPresenceMap = map[string]string{
	"file_path":  "FilePath",
	"old_string": "OldString",
	"new_string": "NewString",
	"content":    "Content",
	"path":       "Path",
}

// hookSpecificOutput is the nested reply object Claude Code reads.
type hookSpecificOutput struct {
	HookEventName            string `json:"hookEventName,omitempty"`
	AdditionalContext        string `json:"additionalContext,omitempty"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
}

// claudeReply is the top-level reply shape.
//
// Every field is omitempty and Continue is a pointer, so the zero Response
// marshals to exactly `{}` — the minimal valid "no comment, proceed" reply.
// A non-pointer bool would emit `"continue":false` for a hook with no opinion,
// which is the opposite instruction.
type claudeReply struct {
	Continue           *bool               `json:"continue,omitempty"`
	StopReason         string              `json:"stopReason,omitempty"`
	Decision           string              `json:"decision,omitempty"`
	Reason             string              `json:"reason,omitempty"`
	SystemMessage      string              `json:"systemMessage,omitempty"`
	HookSpecificOutput *hookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

// ErrMixedResponse is returned when a Response sets Text alongside protocol
// fields. Refusing is the point: silently preferring one would drop the other,
// and a dropped half-reply is invisible at the hook's output.
var ErrMixedResponse = errors.New("adapter: Response sets Text together with protocol fields; they are mutually exclusive")

// ErrEventNoHookSpecificOutput is returned by Encode when a Response carries
// model-facing fields (Context, Decision) for an event the host's
// hookSpecificOutput union does not include. See hookevents.go.
var ErrEventNoHookSpecificOutput = errors.New("adapter: event does not accept hookSpecificOutput")

// Encode renders a host-neutral Response as Claude Code's reply bytes.
//
// event is the lifecycle event the reply belongs to; the nested object echoes
// it back and the host ignores a reply whose echo is wrong. It is passed in
// rather than read off the Response because the DISPATCHER knows which event a
// hook is registered for even when a malformed payload never told the handler.
//
// Never string-built. INV-003.
func Encode(event string, r Response) ([]byte, error) {
	if r.Text != "" {
		// NoTrailingNewline is allowed to ride along with Text — it modifies
		// how the DISPATCHER frames these same bytes (see its own doc
		// comment on Response), not a second, competing reply. Every other
		// field must still be unset, or the Text/protocol-fields conflict
		// this check exists to catch would be masked for exactly the one
		// Response that also needs the flag.
		if (Response{Text: r.Text, NoTrailingNewline: r.NoTrailingNewline}) != r {
			return nil, ErrMixedResponse
		}
		return []byte(r.Text), nil
	}

	reply := claudeReply{}
	if r.Message != "" {
		reply.SystemMessage = r.Message
	}
	if r.Block {
		reply.Decision = "block"
		reply.Reason = r.Reason
	}
	if r.ContinueTrue {
		t := true
		reply.Continue = &t
	}
	if r.Context != "" || r.Decision != DecisionUnset {
		// The host validates hookEventName against a union that is SMALLER
		// than the set of events a hook can fire on (hookevents.go). An
		// envelope naming a non-member is a visible hook error since
		// 2.1.248 and was a silent discard before — never delivered
		// context. Refusing here is what makes "every envelope edikt can
		// emit uses a member" a property of the encoder rather than of
		// each handler's diligence.
		if !AcceptsHookSpecificOutput(event) {
			return nil, fmt.Errorf("%w: %q is not a hookSpecificOutput event on %s (recorded %s); reply with a top-level systemMessage or nothing",
				ErrEventNoHookSpecificOutput, event, HookSpecificOutputUnionVersion, HookSpecificOutputUnionRecordedAt)
		}
		hso := &hookSpecificOutput{HookEventName: event}
		hso.AdditionalContext = r.Context
		if r.Decision != DecisionUnset {
			hso.PermissionDecision = string(r.Decision)
			hso.PermissionDecisionReason = r.Reason
		}
		reply.HookSpecificOutput = hso
	}

	return MarshalPythonCompact(reply)
}

// MarshalPythonCompact renders v exactly as Python's json.dumps(v) does
// under its DEFAULTS — separators=(', ', ': ') and ensure_ascii=True — which
// is how every bash hook this adapter replaces actually called it (grep of
// templates/hooks/*.sh: `json.dumps({...})`, never a separators= or
// ensure_ascii= override). encoding/json's compact Marshal disagrees with
// both defaults: no space after ':' or ',', and raw UTF-8 instead of \uXXXX
// for anything above U+007F.
//
// WHY THIS MATTERS FOR BYTE-FOR-BYTE PARITY. The hook-corpus gate (SPEC-014
// FR-002) hashes raw reply bytes after only the volatile-field normalization
// passes — it does not re-parse and re-canonicalize JSON before comparing.
// Two byte streams that decode to the identical JSON value are, to that gate,
// two different hooks. Every message these six hooks emit carries either an
// emoji (session-start.sh's "📋", "⚠️") or a multi-key object, so both
// mismatches would fire on nearly every scenario without this pass.
func MarshalPythonCompact(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // matches json.dumps: no '<'/'>'/'&' escaping
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// json.Encoder.Encode appends a trailing newline Marshal does not; trimmed
	// so Encode's return value keeps its existing "no trailing newline" contract.
	compact := bytes.TrimRight(buf.Bytes(), "\n")
	return pythonizeJSON(compact), nil
}

// pythonizeJSON rewrites encoding/json's compact output to match Python
// json.dumps' default formatting, in one pass over the already-valid JSON:
//
//   - every non-ASCII rune becomes a \uXXXX escape (a \uXXXX\uXXXX surrogate
//     pair above U+FFFF), matching ensure_ascii=True's lowercase-hex form;
//   - every structural ':' and ',' OUTSIDE a string literal gains the single
//     trailing space Python's default (', ', ': ') separators add.
//
// In-string tracking is a single quote/backslash-parity walk: valid JSON
// never nests unescaped quotes, so toggling on an unescaped '"' is sufficient
// to know whether a ':' or ',' byte is a structural token or string content.
func pythonizeJSON(b []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(b) + len(b)/8)
	inString := false
	escaped := false
	for _, r := range string(b) {
		if inString {
			switch {
			case escaped:
				escaped = false
			case r == '\\':
				escaped = true
			case r == '"':
				inString = false
			}
			writeJSONRune(&out, r)
			continue
		}
		switch r {
		case '"':
			inString = true
			out.WriteByte('"')
		case ':':
			out.WriteString(": ")
		case ',':
			out.WriteString(", ")
		default:
			writeJSONRune(&out, r)
		}
	}
	return out.Bytes()
}

// writeJSONRune appends r as Python's ensure_ascii=True would: passed through
// unchanged below U+0080, \uXXXX-escaped (surrogate-paired above U+FFFF)
// otherwise.
func writeJSONRune(out *bytes.Buffer, r rune) {
	if r < 0x80 {
		out.WriteByte(byte(r))
		return
	}
	if r > 0xFFFF {
		hi, lo := utf16.EncodeRune(r)
		fmt.Fprintf(out, `\u%04x\u%04x`, hi, lo)
		return
	}
	fmt.Fprintf(out, `\u%04x`, r)
}

// ToolInputFilePath reads the environment variables
// instructions-loaded.sh's own FILE resolution falls back through —
// CLAUDE_TOOL_INPUT_FILE_PATH, then CLAUDE_TOOL_INPUT_PATH. Confined here for
// the same SP-004 reason as every other host-specific spelling, even though
// F-055 (docs/internal/plans/PLAN-SPEC-011-progressive-governance-delivery.md)
// found neither is ever actually SET by the real stdin-payload protocol —
// InstructionsLoaded's `if:` gate gives Claude Code a matched file path only
// through the payload the shim inherited on stdin, never through the
// process environment. Reading them is legacy behaviour this migration
// preserves rather than removes (the CHARTER here is byte-for-byte parity,
// not a bug fix — see internal/hook/instructionsloaded.go's own header), and
// test/unit/hooks/test_config_toggles.sh already exercises this exact
// fallback by setting these two variables directly.
func ToolInputFilePath() string {
	if v := os.Getenv("CLAUDE_TOOL_INPUT_FILE_PATH"); v != "" {
		return v
	}
	return os.Getenv("CLAUDE_TOOL_INPUT_PATH")
}

// RulesDirSegment is the bare on-disk path segment this host uses for
// compiled governance rules. file-changed.sh's own governance case arm
// (`*.claude/rules/*`) tests it as a plain substring with no suffix
// requirement — a wider contract than IsRulesFilePath's own `*.md`-suffixed
// one below, so the two cannot share one function. Exported so a handler
// checking "is this path under the rules directory" never respells the
// host's own directory convention itself (SP-004/SAC-003).
const RulesDirSegment = ".claude/rules/"

// IsRulesFilePath reports whether file is a loaded rules-pack path, the way
// instructions-loaded.sh's own `case "$FILE" in *.claude/rules/*.md)` glob
// does: bash's `*` matches any sequence including '/', so the pattern is
// exactly "contains the literal segment, and ends in .md" — no path-aware
// matching. The directory segment names this host's own config directory
// convention, so it lives here rather than in the handler (SP-004/SAC-003).
func IsRulesFilePath(file string) bool {
	return strings.Contains(file, RulesDirSegment) && strings.HasSuffix(file, ".md")
}

// MemoryPath resolves this host's per-project session-memory file —
// session-start.sh's own "how stale is /edikt:context" check reads it.
// The directory convention (<config root>/projects/<cwd, '/' -> '-'>/memory/
// MEMORY.md) is a host-specific storage layout, not a wire field, but it is
// exactly as host-specific as one: a second harness's memory (if it has any)
// would live somewhere else entirely, so the convention belongs here rather
// than hardcoded into a handler (SP-004/SAC-003). cwd is the caller's own
// working directory, never a host-reported payload field — session-start.sh
// encodes bash's own $PWD, never anything from the hook's stdin payload.
//
// root is the resolved config root (ConfigRoot below), not $HOME. The path
// used to be built as $HOME/.claude/..., which is where /edikt:context wrote
// it only on a machine with no profile selected: under CLAUDE_CONFIG_DIR the
// command wrote into the active profile and this hook kept reading the
// default one, so every session reported the memory missing (#11, item 6).
// The markdown side (commands/context.md, commands/doctor.md) resolves the
// same three-rung chain; the two move together or not at all.
func MemoryPath(root, cwd string) string {
	encoded := strings.ReplaceAll(cwd, "/", "-")
	return filepath.Join(root, "projects", encoded, "memory", "MEMORY.md")
}

// ConfigRoot returns this host's active configuration directory through the
// one shared resolution chain (CLAUDE_HOME -> CLAUDE_CONFIG_DIR ->
// $HOME/.claude, internal/claudepaths). Handlers call this rather than the
// resolver directly: which variables select the host's profile is host
// vocabulary, and it lives on this side of the seam (SP-004).
func ConfigRoot() string {
	return claudepaths.ResolveClaudeRoot()
}

// LastSessionSummary renders session-end.sh's own on-disk summary artifact —
// $HOME/.edikt/state/last-session.json — byte-for-byte matching
// `json.dump({'ts':..,'session_id':..,'cwd':..}, f, indent=2,
// ensure_ascii=False)`: 2-space indented, UTF-8 (this ONE write passes
// ensure_ascii=False explicitly, unlike every stdout-reply path in this hook
// tier, which is why it does not go through MarshalPythonCompact). Lives
// here, not in the handler, because "session_id" is a key name the SP-004
// leak grep treats as host wire vocabulary — this particular use is edikt's
// own artifact format, not Claude Code's payload, but the literal spelling
// is identical, and the adapter package is where such spellings belong
// regardless of which protocol they happen to serve.
func LastSessionSummary(ts, sessionID, cwd string) ([]byte, error) {
	v := struct {
		TS        string `json:"ts"`
		SessionID string `json:"session_id"`
		Cwd       string `json:"cwd"`
	}{TS: ts, SessionID: sessionID, Cwd: cwd}
	return json.MarshalIndent(v, "", "  ")
}

// LastSessionSummaryError mirrors session-end.sh's python except-block
// fallback shape: `{'ts': ..., 'error': str(e)}`, written when the primary
// summary encode fails.
func LastSessionSummaryError(ts, errMsg string) ([]byte, error) {
	v := struct {
		TS    string `json:"ts"`
		Error string `json:"error"`
	}{TS: ts, Error: errMsg}
	return json.Marshal(v)
}

// ContinueTrueReply is the literal `{"continue": true}` several hooks
// (session-end.sh, and any future migration with the same shape) print via a
// hand-built `printf`, not python3 json.dumps — never through Encode, because
// this is the reply on the branch that fires when the config file is absent
// or the operator opted out, paths with no adapter.Response value to carry
// it. Exported so a handler needing this exact shape does not spell
// "continue" itself (SP-004/SAC-003) — used via Response.Text, which the
// dispatcher writes verbatim.
const ContinueTrueReply = `{"continue": true}`

// NoPython3HookSpecificBanner renders the exact byte shape session-start.sh,
// post-compact.sh and user-prompt-submit.sh's python3-preflight guard writes
// via a hand-built `printf` — compact JSON, raw UTF-8, deliberately NOT run
// through MarshalPythonCompact's escaping: python3 is, definitionally,
// unavailable on this branch, so bash never had json.dumps available to build
// it either, and printf never asked anything to escape unicode or add
// separator spaces. message is caller-supplied business text (never a wire
// field spelling), concatenated raw — every caller today is a fixed literal
// containing no '"' or '\', matching printf's own zero-escaping behaviour
// exactly rather than defensively escaping input the bash it replaces never
// escaped either.
func NoPython3HookSpecificBanner(event, message string) string {
	if !AcceptsHookSpecificOutput(event) {
		// A non-member event cannot carry the banner as context; the
		// operator-facing shape is the most the host will read.
		return `{"systemMessage":"` + message + `"}`
	}
	return `{"hookSpecificOutput":{"hookEventName":"` + event + `","additionalContext":"` + message + `"}}`
}

// NoPython3ContinueBanner is NoPython3HookSpecificBanner's sibling for the
// `{"continue": true, "systemMessage": "..."}` shape session-end.sh's own
// python3-preflight guard prints.
func NoPython3ContinueBanner(message string) string {
	return `{"continue": true, "systemMessage": "` + message + `"}`
}

// NoPython3SystemMessageBanner is the bare `{"systemMessage": "..."}` shape
// file-changed.sh's own python3-preflight guard prints via a hand-built
// `printf` — no "continue" key, no hookSpecificOutput nesting, unlike either
// sibling above. Same zero-escaping contract: python3 is unavailable on this
// branch, so nothing here ever ran it through json.dumps either.
func NoPython3SystemMessageBanner(message string) string {
	return `{"systemMessage": "` + message + `"}`
}

// FileChangedNoPython3Message is the literal text file-changed.sh's own
// python3-preflight printf carries. It is the one no-python3 message across
// every migrated hook that names the host directly ("changed outside
// Claude") instead of writing "on this host" the way sessionStartNoPython3Message
// and its siblings do — file-changed.sh's whole point is distinguishing a
// change Claude Code itself made from one made by any other process, and
// "outside Claude" is that distinction's plain-English name; a host-neutral
// rewrite would blur exactly the fact this message exists to state. Its
// exact bytes are additionally pinned by
// test/fixtures/hook-corpus/file-changed's recorded no-python3 scenario, so
// rewording it is not available as an option even independent of the naming
// question. Lives here rather than in filechanged.go purely because it names
// the host (SP-004/SAC-003); pass it to NoPython3SystemMessageBanner exactly
// like any other caller-supplied message.
const FileChangedNoPython3Message = "⚠ edikt: python3 is missing — the external-file-change check was skipped. If a governance file (ADR/invariant) changed outside Claude, run /edikt:gov:compile to be sure."

// trimSpace avoids importing strings for one predicate on a byte slice.
func trimSpace(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && isSpace(b[i]) {
		i++
	}
	for j > i && isSpace(b[j-1]) {
		j--
	}
	return b[i:j]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

// AbsenceBanner returns the host-wire JSON a shim prints when no edikt binary
// resolves on any trusted discovery rung.
//
// It lives HERE, not in internal/hook/shim, because SP-004 says every host wire
// detail belongs to the adapter — and the shim's banner is host wire format by
// necessity: it fires precisely when the binary is absent, so nothing can be
// encoded at runtime. Embedding the field names in the shim template instead
// would put `systemMessage`, `hookSpecificOutput`, `hookEventName` and
// `additionalContext` outside adapter/, which no-claude-leak-in-hook-core.sh
// correctly flags. Rendering the banner here keeps the seam honest: a second
// harness changes this function and the shim template follows, rather than
// needing its bash rewritten.
//
// hookEventName must be the FIRING event — necessary, not sufficient. The
// host validates the name against its hookSpecificOutput union
// (hookevents.go), and a name that does not match the firing event is
// rejected the same way a name outside the union is: a visible hook error
// since 2.1.248, a silent discard before. Both look like a shim that "exits 0
// with a banner" while the session never receives it. So the envelope is
// emitted only for a member event; for the rest (PostCompact, PreCompact,
// InstructionsLoaded, …) the banner is the top-level systemMessage alone,
// which is the most the host will read on those events.
func AbsenceBanner(hookName, event string) (string, error) {
	msg := "⚠ edikt: edikt not found on any trusted discovery rung — hook " +
		hookName + " did not run (cause: binary-absent). Run /edikt:doctor to diagnose."
	reply := map[string]any{"systemMessage": msg}
	if AcceptsHookSpecificOutput(event) {
		reply["hookSpecificOutput"] = map[string]any{
			"hookEventName":     event,
			"additionalContext": msg,
		}
	}
	b, err := json.Marshal(reply)
	if err != nil {
		return "", fmt.Errorf("adapter: encoding absence banner: %w", err)
	}
	return string(b), nil
}

// ── phase-end-detector's host vocabulary (SP-004 / SAC-003) ─────────────────
//
// Three facts phase-end-detector.sh reads or writes are host-specific, so they
// live here rather than in the handler: the name of the environment variable
// this host's CLI resolves its profile from, and the closed set of model
// tokens that CLI's --model flag accepts. A handler naming either would put a
// vendor string back into the hook core, which no-claude-leak-in-hook-core.sh
// refuses — correctly, since a second host spells both differently.
//
// The CLI's own NAME is deliberately NOT here. The binary never dispatches a
// model (INV-012), so it has no reason to know what the caller execs; the
// dispatch-orchestrating shim passes the tool name in as data and the handler
// formats it into operator-facing text. That is what keeps "claude exited 2"
// reachable in the reply without the tier-2 binary knowing the word.

// HostConfigDirEnv is the environment variable this host's CLI resolves its
// profile directory from. Read (never written) by phase-end-detector's
// evaluator-auth diagnostic.
const HostConfigDirEnv = "CLAUDE_CONFIG_DIR"

// HostConfigDirNote renders the clause phase-end-detector.sh's auth-failure
// message embeds — either the variable and its value, or the fact that it is
// unset. The distinction is the whole diagnostic: a multi-profile setup whose
// profile variable never reached the hook environment makes the evaluator
// child resolve the DEFAULT profile, which is usually the unauthenticated one.
func HostConfigDirNote() string {
	if v := os.Getenv(HostConfigDirEnv); v != "" {
		return "hook environment has " + HostConfigDirEnv + "=" + v
	}
	return HostConfigDirEnv + " is NOT set in the hook environment"
}

// HostAuthFailureCause renders the likely-cause clause of the same auth-failure
// message. With the profile variable unset, that is the cause: the child falls
// back to the default profile. With it set, blaming it is wrong (issue #12) —
// the likely cause is an installed phase-end-detector.sh from before 0.9.0,
// which dispatches with --bare, and --bare never reads an OAuth login.
func HostAuthFailureCause() string {
	if os.Getenv(HostConfigDirEnv) != "" {
		return HostConfigDirEnv + " is set, so the profile is not the likely cause. " +
			"An installed phase-end-detector.sh that still dispatches with --bare never " +
			"authenticates under an OAuth login (--bare reads only ANTHROPIC_API_KEY or " +
			"apiKeyHelper); run `edikt upgrade` to reinstall the hook."
	}
	return "If this project runs under a non-default profile, ensure " + HostConfigDirEnv +
		" is exported where hooks run; otherwise the child uses the default profile, " +
		"which is unauthenticated here."
}

// evaluatorModelAllowlist is the curated set phase-end-detector.sh's own
// `case "$EVAL_MODEL" in` arm accepts before the value reaches the CLI's
// --model flag. It is an allowlist rather than a sanitizer because a
// config-supplied value of `sonnet --dangerously-skip-permissions` would
// otherwise split into several argv elements (INV-006).
var evaluatorModelAllowlist = []string{
	"opus", "sonnet", "haiku",
	"claude-opus-4-7", "claude-sonnet-4-6", "claude-haiku-4-5-20251001",
}

// EvaluatorModelAllowed reports whether a configured evaluator model may be
// passed through to the host CLI.
func EvaluatorModelAllowed(model string) bool {
	for _, m := range evaluatorModelAllowlist {
		if model == m {
			return true
		}
	}
	return false
}

// EvaluatorModelDefault is the fallback this host uses when the configured
// model is absent or off the allowlist.
const EvaluatorModelDefault = "sonnet"

// MarshalPythonIndent2 is MarshalPythonCompact's sibling for the two artifacts
// phase-end-detector writes to DISK rather than to the protocol channel — the
// verdict JSON and the plan-eval state file, both produced by
// `json.dump(obj, f, indent=2)`.
//
// Indented json.dump differs from the compact form in exactly one way that
// matters here: with indent set, Python's separators become (',', ': '), which
// is what encoding/json's own SetIndent already produces. So this applies the
// ensure_ascii=True pass and nothing else — running the compact pass's
// separator rewrite on top would insert a space after every ',' that is
// already followed by a newline.
func MarshalPythonIndent2(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return asciiEscapeJSON(bytes.TrimRight(buf.Bytes(), "\n")), nil
}

// asciiEscapeJSON rewrites every non-ASCII rune of already-valid JSON as the
// \uXXXX escape Python's ensure_ascii=True default emits, leaving structure
// untouched.
func asciiEscapeJSON(b []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(b) + len(b)/8)
	for _, r := range string(b) {
		writeJSONRune(&out, r)
	}
	return out.Bytes()
}

// HostConfigDirNoun is the plain-English name this host's CLI gives the
// directory HostConfigDirEnv points at.
//
// It exists as its own accessor for one reason: the phrase names the VENDOR,
// and a handler that spelled it inline would be reaching past the seam
// (no-claude-leak-in-hook-core.sh refuses it, correctly — a second host calls
// that directory something else). The sentence around it is the handler's
// business text; only the noun is host vocabulary.
func HostConfigDirNoun() string { return "Claude config dir" }

// SystemMessageReply renders `{"systemMessage": …}` through the serializer, in
// the byte shape python's json.dumps produces (a space after the colon, and
// \uXXXX escapes above U+007F).
//
// This is the wire shape phase-end-detector emits on most of its paths, and it
// lives here rather than in the handler because "systemMessage" is a host wire
// key. INV-003 is satisfied by construction: the value is marshalled, never
// concatenated, so a plan filename carrying a quote or a newline cannot
// corrupt the reply.
func SystemMessageReply(message string) ([]byte, error) {
	return MarshalPythonCompact(systemMessageReply{SystemMessage: message})
}

// ContinueSystemMessageReply is SystemMessageReply's sibling for the
// `{"continue": true, "systemMessage": …}` shape, with the keys in the order
// python's dict literal inserts them.
func ContinueSystemMessageReply(message string) ([]byte, error) {
	return MarshalPythonCompact(continueSystemMessageReply{Continue: true, SystemMessage: message})
}

// DecodeSystemMessage reads a systemMessage back out of a rendered reply,
// reporting whether the reply decoded at all. A handler comparing its own
// earlier output against a marker needs this and must not learn the key
// spelling to get it.
func DecodeSystemMessage(reply []byte) (string, bool) {
	var r systemMessageReply
	if err := json.Unmarshal(bytes.TrimSpace(reply), &r); err != nil {
		return "", false
	}
	return r.SystemMessage, true
}

type systemMessageReply struct {
	SystemMessage string `json:"systemMessage"`
}

type continueSystemMessageReply struct {
	Continue      bool   `json:"continue"`
	SystemMessage string `json:"systemMessage"`
}

// ── Sentinel-guard scope surface (SP-004) ────────────────────────────────
//
// The INV-005 managed-region guard decides which files it will read at all,
// and every one of those decisions is stated in THIS host's on-disk layout:
// the instruction file's name, where settings live, where compiled rules and
// skill packages are written. None of that is portable, so none of it belongs
// in the handler — a second harness must be additive, and a handler naming
// `.claude/` directly is a handler that has to be rewritten rather than
// extended.
//
// The names below are already casefolded, because the guard normalizes an
// externally-controlled basename with NFKC + casefold before comparing
// (INV-006) and comparing a folded value against an unfolded constant would
// silently never match.

// InstructionFileKey is the casefolded basename of the host's per-project
// instruction file, which carries managed regions at any depth.
const InstructionFileKey = "claude.md"

// SettingsFileKey is the casefolded basename of the host's settings file.
const SettingsFileKey = "settings.json"

// SkillFileKey is the casefolded basename of a rendered skill package.
const SkillFileKey = "skill.md"

// HostHomeEnv is the environment variable naming this host's home directory.
const HostHomeEnv = "CLAUDE_HOME"

// HostHomeDirName is the per-user directory this host keeps under $HOME.
const HostHomeDirName = ".claude"

// GovernanceRulesSegment is the path segment compiled governance rules are
// written under, relative to a project root.
const GovernanceRulesSegment = ".claude/rules/governance/"

// SkillsSegmentPrefix is the path prefix under which edikt's own skill
// packages are rendered.
const SkillsSegmentPrefix = ".claude/skills/edikt-"

// HostConfigSegment is the bare `.claude` path segment, used to recognise a
// host config directory at any depth.
const HostConfigSegment = ".claude"
