package adapter

// hookevents.go — the closed set of events whose hook reply may carry a
// hookSpecificOutput envelope, pinned as a DATED fixture.
//
// Claude Code validates `hookSpecificOutput.hookEventName` against a
// discriminated union, and that union is SMALLER than the set of events a
// hook can register for. An envelope naming an event outside it is not
// ignored: since 2.1.248 it is a visible hook error ("Hook JSON output
// validation failed — hookSpecificOutput.hookEventName: expected one of …"),
// and before 2.1.248 the whole reply was discarded silently. Either way the
// context never reaches the model. edikt's PostCompact hook emitted exactly
// that shape from its first Go port until 2026-09-02, and no transcript on
// this machine ever received its "Context recovered after compaction" as
// context — the defect was invisible precisely because a discarded reply
// looks like a quiet hook.
//
// The members below were read from the 2.1.257 binary's own union — every
// `hookEventName: x("<Name>")` literal in its reply schema — on 2026-09-02.
// It is a fixture, not a belief: when a host release changes the union, this
// list is re-read from that binary and the date moves with it. Encode and
// AbsenceBanner consult it so an envelope edikt can emit is always one the
// host will read.
//
// Notably ABSENT, and registered by edikt: PostCompact, PreCompact,
// InstructionsLoaded, SessionEnd, StopFailure, TaskCreated, TaskCompleted,
// WorktreeRemove. A handler on any of those may reply with a top-level
// systemMessage (operator-facing) and nothing model-facing.

// HookSpecificOutputUnionVersion names the host build the union was read from.
const HookSpecificOutputUnionVersion = "claude 2.1.257"

// HookSpecificOutputUnionRecordedAt is the date the union was read.
const HookSpecificOutputUnionRecordedAt = "2026-09-02"

// hookSpecificOutputUnion is the union, sorted, one entry per member.
var hookSpecificOutputUnion = []string{
	"CwdChanged",
	"Elicitation",
	"ElicitationResult",
	"FileChanged",
	"MessageDisplay",
	"Notification",
	"PermissionDenied",
	"PermissionRequest",
	"PostModelSwitch",
	"PostToolBatch",
	"PostToolUse",
	"PostToolUseFailure",
	"PreModelSwitch",
	"PreToolUse",
	"SessionStart",
	"Setup",
	"Stop",
	"SubagentStart",
	"SubagentStop",
	"UserPromptExpansion",
	"UserPromptSubmit",
	"WorktreeCreate",
}

// HookSpecificOutputUnion returns a copy of the pinned union.
func HookSpecificOutputUnion() []string {
	return append([]string(nil), hookSpecificOutputUnion...)
}

// AcceptsHookSpecificOutput reports whether a reply to event may carry a
// hookSpecificOutput envelope the host will read.
func AcceptsHookSpecificOutput(event string) bool {
	for _, m := range hookSpecificOutputUnion {
		if m == event {
			return true
		}
	}
	return false
}
