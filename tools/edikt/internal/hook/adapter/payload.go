// Package adapter is the SP-004 host seam.
//
// # WHAT THE SEAM IS FOR
//
// Every hook edikt ships is invoked by a HOST — a coding harness that spawns
// the hook, hands it a JSON payload on stdin, and reads a JSON reply back.
// In v0.8.0 there is exactly one host, and PRD-007's Non-Goals say so
// explicitly. The seam exists anyway, now, because BRAIN-009's future port has
// to be able to add a second host WITHOUT REOPENING ANY HOOK IMPLEMENTATION.
// A seam retrofitted after twenty-two handlers already read raw host JSON is
// not a seam; it is twenty-two rewrites.
//
// # THE RULE THAT MAKES IT REAL
//
// This package is the ONLY place under tools/edikt/internal/hook/ that may
// know a host's wire vocabulary — its field spellings, its reply shape, its
// event names. Handlers receive a Payload and return a Response, both
// host-neutral, and never see raw bytes. That is not a convention: it is
// mechanically enforced by tools/edikt/check/no-claude-leak-in-hook-core.sh,
// which fails when a handler file outside this directory names a host-specific
// identifier or wire field.
//
// # TWO DIRECTIONS, NOT ONE
//
// It is tempting to build only the inbound half — parse the host's payload,
// hand handlers a clean struct — and let handlers write their own replies.
// That leaks the host straight back out: the reply shape is every bit as
// host-specific as the request, and a handler that hand-builds one is bound to
// that host just as tightly as one that reads its raw fields. So the encoder
// lives here too, and a handler that wants to say "inject this text" says it
// in Response terms and never learns how this host spells it.
package adapter

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
)

// Payload is one hook invocation, in host-neutral terms.
//
// SCOPE: only what handlers actually read. This is deliberately NOT a faithful
// mirror of any host's payload — a passthrough struct would make the seam
// decorative, since a handler reaching a host-shaped field through a
// differently-named Go field is still coupled to that host's data model. The
// fields here were derived by reading every payload field access in the
// current 22 bash hooks. Phases 12-22 extend this struct as each migration
// proves it needs a field; an unread field does not belong here.
//
// EVERY FIELD IS OPTIONAL AND MAY BE EMPTY. A host that does not send one, an
// event that does not carry one, and a malformed payload all produce the zero
// value — so a handler must never read "" as "the host said empty". Where the
// difference matters, ask Present.
type Payload struct {
	// Event is the lifecycle event that fired, in the host's own vocabulary
	// (the values templates/hooks/registry.yaml's `event` enum closes over).
	// Handlers should rarely branch on this: a handler is registered for one
	// event already. It exists because the reply shape for some events must
	// echo the event back, and the encoder needs it.
	Event string

	// SessionID identifies the conversation. Empty when the host did not
	// supply one — never invented, since a guessed session id silently merges
	// two sessions' dedup state.
	SessionID string

	// Cwd is the working directory the host reports for the invocation. It is
	// NOT authoritative for the project root: a handler that needs the root
	// asks its Invocation for one, which falls back to the process's own cwd.
	Cwd string

	// ToolName is the tool being invoked, for tool-scoped events.
	ToolName string

	// ToolInput is the tool's arguments, unmodelled. Kept generic on purpose:
	// tool arguments are the host's most volatile surface, and modelling all
	// of them here would drag the whole tool vocabulary across the seam. The
	// fields hooks actually read are promoted below; anything else a future
	// handler needs goes through Field, which keeps the raw key spelling
	// inside this package's own accessor rather than in a handler.
	ToolInput map[string]any

	// FilePath, OldString, NewString and Content are the promoted tool-input
	// fields the write-time hooks compare byte-for-byte. Promoted rather than
	// left in ToolInput so a handler never writes a raw wire key literal, and
	// so INV-006's normalisation has one obvious place to apply.
	//
	// FilePath is populated from TWO distinct wire sources, and callers never
	// need to know which one fired: tool_input.file_path (PreToolUse/
	// PostToolUse events) and a top-level file_path (FileChanged's own
	// payload shape — file-changed.sh's `d.get('file_path','')` reads it off
	// the payload root, never off tool_input, because FileChanged carries no
	// tool_input at all). The two are mutually exclusive in practice — no
	// event sends both — so promoting them onto one field loses no
	// information a real payload could ever carry.
	FilePath  string
	OldString string
	NewString string
	Content   string

	// Path is tool_input.path — a SECOND spelling of "which file", read by
	// inject-directives-pre.sh/inject-directives-post.sh's own extraction
	// (`ti.get("file_path") or ti.get("path") or ""`) as a fallback when
	// file_path is absent. Kept separate from FilePath rather than folded
	// into it: those two shims' own fallback ORDER (file_path wins over
	// path) is the hook's business logic, not a wire-promotion fact, so the
	// adapter exposes both raw values and leaves the preference to the
	// handler.
	Path string

	// Prompt is the user's submitted text, for prompt-submit events.
	Prompt string

	// Source is why the event fired (a session start's origin, for example).
	Source string

	// AgentID and AgentType attribute the invocation to a subagent when the
	// host can resolve one. Both empty means UNKNOWN, never "the parent" —
	// a resolver that guessed "parent" would make every subagent look like the
	// session that spawned it.
	AgentID   string
	AgentType string

	// TaskID and TaskName identify the task for task lifecycle events.
	TaskID   string
	TaskName string

	// WorktreePath is the worktree a worktree event concerns.
	WorktreePath string

	// TranscriptPath points at the session transcript, when the host exposes
	// one. LastAssistantMessage is the final assistant turn, for stop events.
	TranscriptPath       string
	LastAssistantMessage string

	// AgentTranscriptPath is the SUBAGENT's own stream; TranscriptPath is the
	// PARENT session's, on every event that carries one. The two are different
	// files and both are supplied on SubagentStop, so "parent or subagent" was
	// never the choice it appeared to be.
	AgentTranscriptPath string

	// PromptID scopes ONE user turn. session_id scopes the session and AgentID
	// scopes a single dispatch; between them the three give a complete join
	// without a new identifier.
	//
	// FOUR MORE WIRE FIELDS ARE OBSERVED AND DELIBERATELY NOT MODELLED:
	// effort, permission_mode, background_tasks, session_crons. Nothing reads
	// them. Adding a field with no consumer is the exact defect this corpus
	// keeps finding — produced_accepted_sidecar, defect_axis, api_ms were all
	// declared, never populated, and then quoted as measurements. They are
	// recorded here so the next reader knows they exist on the wire and were
	// left out on purpose, not missed.
	PromptID string

	// StopActive reports that a stop hook is already running — the host's
	// re-entrancy guard. A handler that ignores it can recurse.
	StopActive bool

	// ErrorType and ErrorMessage carry a StopFailure event's `error` object
	// (the host's own {"type": ..., "message": ...} shape) — promoted the same
	// way FilePath/OldString/NewString/Content promote tool_input fields, so a
	// handler never names the wire key "error" itself (SP-004/SAC-003). Both
	// empty means the host sent no error object, not that it sent one with
	// empty fields — use Present("ErrorType") where that distinction matters.
	ErrorType    string
	ErrorMessage string

	// ID is a top-level "id" — TaskCreated/TaskCompleted's own fallback
	// spelling for TaskID when the newer "task_id" key is absent
	// (`d.get("task_id","") or d.get("id","")`).
	ID string

	// Name is a top-level "name" — TaskCreated/TaskCompleted's fallback
	// spelling for TaskName, tried after "task_name" and before Subject.
	Name string

	// Subject is a top-level "subject" — the SECOND fallback for TaskName,
	// tried after Name. TaskCreated's own default when even this is absent
	// differs from TaskCompleted's (see each handler), so use
	// Present("Subject") where that final default matters.
	Subject string

	// Status is a top-level "status" — TaskCompleted's own outcome field.
	Status string

	// Result is a top-level "result" — TaskCompleted's fallback for Status
	// when "status" is absent or empty. Use Present("Result") where the
	// distinction between "result present and empty" and "result absent"
	// matters (TaskCompleted's own default is "completed" only on absence).
	Result string

	// ProposedPath is a top-level "proposed_path" — WorktreeCreate's second
	// fallback spelling for the worktree path, tried after WorktreePath.
	ProposedPath string

	// TopLevelPath is a top-level "path" — WorktreeCreate/WorktreeRemove's
	// own spelling, DISTINCT from Path (tool_input.path): neither event
	// carries a tool_input at all, so this is a second, independent source
	// for the same English word "path".
	TopLevelPath string

	// SubagentTypeField is the raw top-level "subagent_type" wire value,
	// exposed directly rather than through AgentType's own agent_type-wins
	// fallback: SubagentStart's own extraction
	// (`d.get("subagent_type") or d.get("agent")`) never consults
	// "agent_type" at all, a different fallback chain than the one AgentType
	// already encodes for other events.
	SubagentTypeField string

	// AgentField is a top-level "agent" — SubagentStart's second fallback
	// for SubagentTypeField, a third spelling for "which agent" besides
	// AgentID and AgentType/SubagentTypeField.
	AgentField string

	// AgentNameField is a top-level "agent_name" — SubagentStop's FIRST
	// identity fallback, tried ahead of SubagentTypeField, ToolName and
	// AgentField. Exposed raw, with no preference encoded here: the order in
	// which a hook consults these four is that hook's contract (SubagentStart
	// consults a different two, in a different order), so the adapter carries
	// the values and the handler carries the ordering.
	AgentNameField string

	// EvaluatorAgent, EvaluatorSeverity and EvaluatorFinding are edikt's own
	// structured evaluator verdict, as SubagentStop's gate logic reads it.
	//
	// EvaluatorAgent and EvaluatorSeverity are BOTH EMPTY OR BOTH POPULATED,
	// and that is a faithful port of a real coupling rather than tidiness:
	// the bash reads them in one expression whose failure unwinds both, so a
	// severity of the wrong TYPE erases a perfectly good agent. Preserving it
	// keeps a gate that fires today firing tomorrow, and a gate that stays
	// silent today staying silent — see parseEvaluatorOutput.
	//
	// EvaluatorFinding is the first finding's description, RAW: not stripped,
	// not truncated, not defaulted. The 120-codepoint cut belongs to the
	// handler because it is a display rule, and a handler that could not see
	// the untruncated value could never tell a short finding from a cut one.
	EvaluatorAgent    string
	EvaluatorSeverity string
	EvaluatorFinding  string

	// present records which keys the host actually sent, so a handler can tell
	// "absent" from "present and empty". INV-013 in miniature: a decoder that
	// produces the same value for both makes the difference unobservable.
	present map[string]bool

	// raw is the payload exactly as the host delivered it. It is UNEXPORTED
	// and there is deliberately no accessor returning it: a handler that could
	// read these bytes would be reading host wire vocabulary, which is the one
	// thing this package exists to prevent. The single fact derived from it
	// crosses the seam as a digest — see PayloadDigest.
	raw []byte
}

// PayloadDigest is the hex sha256 of prefix concatenated with the payload
// EXACTLY as the host delivered it, minus trailing newlines.
//
// WHY A DIGEST AND NOT THE BYTES. stop-hook.sh's duplicate-invocation guard
// (bok-services field bug #4 — the same hook registered at two settings
// scopes, so every emitted line printed twice) keys a token file on
// `printf '%s' "stop-hook:$INPUT" | sha256`, where $INPUT is `$(cat)`. A
// handler cannot reproduce that key from the decoded Payload: re-encoding the
// struct would not reproduce the host's own byte stream, and two payloads
// that decode identically but arrive differently are — correctly — different
// invocations to a dedup guard. So the raw bytes stay inside the adapter and
// only the digest crosses.
//
// THE TRAILING-NEWLINE TRIM IS THE BASH SEMANTIC, NOT A TIDY-UP. Command
// substitution strips every trailing newline, so a host that terminates its
// payload with one and a host that does not produce the SAME dedup key today.
// Trimming here keeps that true; not trimming would silently halve the
// guard's hit rate against a recording that has one.
func (p Payload) PayloadDigest(prefix string) string {
	h := sha256.New()
	h.Write([]byte(prefix))
	h.Write(bytes.TrimRight(p.raw, "\n"))
	return hex.EncodeToString(h.Sum(nil))
}

// Absent reports that the host sent no payload bytes at all.
//
// DISTINCT FROM AN EMPTY OBJECT, AND A GATE DEPENDS ON THE DIFFERENCE. `{}` is
// a payload that parsed and said nothing; zero bytes is no payload to parse.
// Every field-level accessor answers identically for both — Present is false
// and every string is "" — so a handler that must treat "I could not inspect
// this at all" differently from "I inspected it and there was nothing there"
// has no other way to ask.
//
// verify-gate is that handler: bash denies the first (INV-011, absence of
// inspectable input is not a pass) and allows the second, and collapsing them
// would make an empty stdin the cheapest way past a completion-claim gate.
//
// Parse deliberately does NOT error on empty stdin — several events fire with
// no body and journaling those as suppressions would be a lie — so the fact
// survives to here rather than being reported as a failure.
//
// A HAND-BUILT Payload IS Absent, because it genuinely carries no bytes. That
// is the honest answer and it matches Present's, but it means a test that
// constructs adapter.Payload{ToolName: ...} directly exercises the
// no-payload branch rather than the one it names. Build test payloads through
// Parse when the handler under test reads this.
func (p Payload) Absent() bool { return len(trimSpace(p.raw)) == 0 }

// Present reports whether the host actually sent a value for the named
// host-neutral field (use the exact Go field name, e.g. "FilePath").
//
// Returns false for a Payload built by hand in a test, which is the honest
// answer: nothing was received at all.
func (p Payload) Present(field string) bool { return p.present[field] }

// Field reads an unmodelled tool-input value as a string.
//
// The key is a host wire spelling, which is why this accessor lives in the
// adapter package: a handler calling p.Field("some_key") is still naming a
// host field, so the leak check treats a raw wire key in a handler as a
// violation regardless of how it is reached. Prefer promoting a field onto
// Payload when a second handler needs it.
func (p Payload) Field(key string) (string, bool) {
	if p.ToolInput == nil {
		return "", false
	}
	v, ok := p.ToolInput[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// Decision is a host-neutral permission verdict.
//
// DecisionUnset is the zero value and means "no opinion" — the reply carries
// no decision at all. That distinction is load-bearing: a hook that failed to
// form an opinion and a hook that decided to allow are different facts, and
// collapsing them makes an allow indistinguishable from a crash that got
// swallowed.
type Decision string

const (
	DecisionUnset Decision = ""
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
	DecisionAsk   Decision = "ask"
)

// Response is a hook's reply, in host-neutral terms.
//
// The zero Response means "no comment, proceed", and every adapter MUST encode
// it as its host's minimal valid reply. That is the shape the panic path
// returns, so it has to be a first-class value rather than a special case.
type Response struct {
	// Context is text to add to the model's context.
	Context string

	// Message is operator-facing text for the human, not the model.
	Message string

	// Decision is the permission verdict, for events that accept one.
	Decision Decision

	// Reason explains a Decision or a Block. Never omitted when either is set:
	// an unexplained deny is indistinguishable from a bug.
	Reason string

	// Block asks the host to stop the current turn rather than continue.
	Block bool

	// ContinueTrue forces an explicit `"continue": true` into the reply
	// alongside whatever else is set — inject-directives-post.sh's own
	// python renderer emits `{"continue": True, "hookSpecificOutput": {...
	// }}` for BOTH its "context delivered" and "nothing matched" branches,
	// unlike every other PostToolUse-family hook this seam has ported so
	// far (session-start, post-compact, user-prompt-submit all reply with
	// bare hookSpecificOutput, no continue key, and the host accepts that
	// as an implicit continue). Kept OFF by default rather than folded into
	// the Context branch unconditionally, so those three hooks' already-
	// diff-verified byte shape does not move.
	ContinueTrue bool

	// Text is NON-PROTOCOL stdout — the status line's rendered string, and
	// nothing else today. When set, the encoder emits it verbatim and encodes
	// no JSON at all, because that surface is not a JSON protocol. Mutually
	// exclusive with every other field; the encoder REFUSES the combination
	// rather than silently preferring one, because a silent preference is how
	// half a reply goes missing with nothing reporting it.
	Text string

	// NoTrailingNewline suppresses the dispatcher's own trailing "\n" after
	// Text. Every OTHER Text use today (ContinueTrueReply, the NoPython3*
	// banners) is a hand-built JSON literal with no embedded newline of its
	// own, relying on the dispatcher to supply the single trailing "\n" a
	// bash `printf '...\n'` or python3 `print()` would have — see run.go's
	// writeReply. status-line.sh is the one Text producer that is NOT JSON:
	// its own `printf '%s' "$OUTPUT"` emits ZERO trailing bytes beyond the
	// text itself, by design (a status line is concatenated inline by its
	// host, not printed as its own line), and the corpus gate compares raw
	// bytes with no newline-trimming pass — so reproducing that hook without
	// this flag would append a byte bash never sent. Only ever paired with a
	// non-empty Text; Encode's mixed-response check allows the pair without
	// treating it as a second, competing field.
	NoTrailingNewline bool
}

// IsZero reports whether this Response says nothing at all.
func (r Response) IsZero() bool { return r == Response{} }
