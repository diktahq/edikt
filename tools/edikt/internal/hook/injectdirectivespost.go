package hook

// injectdirectivespost.go — the Go replacement for
// templates/hooks/inject-directives-post.sh (PLAN-v080-reliability-core
// Phase 13). PostToolUse — ADVISORY-grade write-touch injection, delivered
// as context after the write already happened (never blocking; the MUST-
// grade path is inject-directives-pre.sh, migrated separately and later —
// ADR-060 §E1's split).
//
// CALLS hookmatch DIRECTLY, IN-PROCESS — NOT A SUBPROCESS. The bash original
// shells out to `$EDIKT_BIN hook match --grade advisory ...` because bash has
// no other way to reach the matcher. This handler IS that same binary, so the
// subprocess round-trip (and its own EDIKT_BIN discovery ladder, and the
// "binary could not be resolved" failure class the bash original has to
// handle) simply does not exist here — there is nothing for a Go
// PostToolUse hook to fail to resolve. This is a real, intentional
// architectural improvement, not a preserved bug: the corpus matrix's
// no-python3 scenario is the only fail-open path left to characterize (see
// its own header comment for why the binary-absence banner has no Go-side
// equivalent to port).
//
// NO --dedup. Unlike inject-directives-pre.sh, the bash original never
// passes `--dedup` to `hook match` — advisory context is delivered on every
// matching write, not bounced-once-per-session. Ported as an absence, not
// forgotten: nothing here computes a dedup key.

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
)

func init() {
	Register("inject-directives-post", injectDirectivesPost)
}

const injectDirectivesPostNoPython3Message = "⚠ edikt: python3 is missing on this host — advisory-grade write-time directive context was not delivered for this write."

func injectDirectivesPost(inv *Invocation) Result {
	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr, "edikt inject-post: python3 unavailable; advisory context skipped")
		banner := adapter.NoPython3HookSpecificBanner("PostToolUse", injectDirectivesPostNoPython3Message)
		return Result{Response: adapter.Response{Text: banner}}
	}

	switch inv.Payload.ToolName {
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
	default:
		return quietContinue()
	}

	// `ti.get("file_path") or ti.get("path") or ""` — see adapter.Payload's
	// own doc comments on FilePath (tool_input.file_path) and Path
	// (tool_input.path) for why these are two distinct promoted fields
	// rather than one with a baked-in preference.
	filePath := inv.Payload.FilePath
	if filePath == "" {
		filePath = inv.Payload.Path
	}
	if filePath == "" {
		return quietContinue()
	}

	// ACTOR resolution — mirrors the bash original's NUL-framed python
	// extraction byte for byte. ABSENT is not the same as EMPTY: no
	// "agent_id" key at all means the main session (a RESOLVED identity
	// that may legitimately share the parent dedup marker); an agent_id
	// key present but blank means the host told us something unreadable,
	// which is UNKNOWN and must DELIVER rather than suppress.
	var actor, agentID string
	if !inv.Payload.Present("AgentID") {
		actor, agentID = "parent", "parent"
	} else if aid := strings.TrimSpace(inv.Payload.AgentID); aid != "" {
		actor, agentID = "subagent", aid
	} else {
		actor, agentID = "unknown", ""
	}
	agentType := strings.TrimSpace(inv.Payload.AgentType)
	if agentType == "" {
		agentType = "-"
	}

	root, rerr := os.Getwd()
	if rerr != nil {
		return quietContinue()
	}
	res := hookmatch.Match(root, filePath)

	// Presentation ordering — mirrors `bin/edikt hook match`'s own
	// classPriority sort (cmd/hook_match.go), applied there BEFORE its JSON
	// is written to stdout for the bash original's python renderer to
	// consume. Duplicated here rather than imported: cmd/ depends on
	// internal/hook, so the reverse import would invert the module's own
	// dependency direction. Presentation-only — never changes which
	// entries match or their grade.
	sort.SliceStable(res.Entries, func(i, j int) bool {
		return injectClassPriority(res.Entries[i].Class) < injectClassPriority(res.Entries[j].Class)
	})

	res.Entries = hookmatch.FilterGrade(res.Entries, "advisory")
	if len(res.Entries) == 0 && res.Outcome == hookmatch.OutcomeMatched {
		res.Outcome = hookmatch.OutcomeNoMatch
	}

	ids := make([]string, 0, len(res.Entries))
	for _, e := range res.Entries {
		ids = append(ids, e.ID)
	}
	_ = hookmatch.Append(hookmatch.Record{
		Shim:         "post",
		Outcome:      string(res.Outcome),
		Detail:       res.Detail,
		Path:         res.NormPath,
		Matched:      len(res.Entries),
		SessionID:    inv.Payload.SessionID,
		Actor:        actor,
		AgentType:    agentType,
		ContextID:    agentID,
		DirectiveIDs: ids,
		// Bounced is always false here — the bash original's own
		// `hook match` call never passes `--shim pre`, and Bounced in the
		// real CLI is gated on `shim == "pre"`.
		Bounced:         false,
		DedupSuppressed: false,
		BudgetExhausted: false,
	})

	if len(res.Entries) == 0 {
		return quietContinue()
	}

	lines := make([]string, len(res.Entries))
	for i, e := range res.Entries {
		lines[i] = fmt.Sprintf("- %s (%s)", e.Text, e.ID)
	}
	var reminders []string
	seen := map[string]bool{}
	for _, e := range res.Entries {
		for _, r := range e.Reminders {
			if !seen[r] {
				seen[r] = true
				reminders = append(reminders, r)
			}
		}
	}
	body := fmt.Sprintf("edikt: advisory governance for %s\n\n%s", res.NormPath, strings.Join(lines, "\n"))
	if len(reminders) > 0 {
		remLines := make([]string, len(reminders))
		for i, r := range reminders {
			remLines[i] = "- " + r
		}
		body += "\n\nBefore you act:\n" + strings.Join(remLines, "\n")
	}

	return Result{Response: adapter.Response{Context: body, ContinueTrue: true}}
}

// quietContinue mirrors the bash original's `_quiet()`: the literal
// `{"continue": true}` reply, both on the shim's own early-exit guards and
// on the python renderer's own "no entries matched" branch — the two are
// BYTE-IDENTICAL (json.dumps({"continue": True}) on a one-key dict produces
// no separators to differ over), so one constant covers both call sites.
func quietContinue() Result {
	return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
}

// injectClassPriority mirrors cmd/hook_match.go's own classPriority: lower
// sorts first, an unrecognized or empty class sorts last.
func injectClassPriority(class string) int {
	switch class {
	case "invariant":
		return 0
	case "adr":
		return 1
	case "guideline":
		return 2
	default:
		return 3
	}
}
