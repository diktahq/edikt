package adapter

import (
	"errors"
	"sort"
	"strings"
	"testing"
)

// The fixture is a dated reading of one host build. It is asserted as
// written — 22 members, sorted, no duplicates — so an edit that drops or adds
// a name without moving the date is caught here, not by a hook going quiet.
func TestHookSpecificOutputUnion_2_1_257_IsPinned(t *testing.T) {
	u := HookSpecificOutputUnion()
	if len(u) != 22 {
		t.Fatalf("union pinned at 22 members on %s (%s); got %d — re-read the host binary and move the date",
			HookSpecificOutputUnionVersion, HookSpecificOutputUnionRecordedAt, len(u))
	}
	if !sort.StringsAreSorted(u) {
		t.Fatalf("union must be sorted so a diff reads as a set change: %v", u)
	}
	seen := map[string]bool{}
	for _, m := range u {
		if seen[m] {
			t.Fatalf("duplicate member %q", m)
		}
		seen[m] = true
	}
	for _, want := range []string{"SessionStart", "PreToolUse", "PostToolUse", "UserPromptSubmit", "SubagentStart", "SubagentStop", "Stop"} {
		if !AcceptsHookSpecificOutput(want) {
			t.Errorf("%s must be a member — edikt injects context on it", want)
		}
	}
	for _, absent := range []string{"PostCompact", "PreCompact", "InstructionsLoaded", "SessionEnd", "StopFailure", "TaskCreated", "TaskCompleted", "WorktreeRemove", "statusLine", ""} {
		if AcceptsHookSpecificOutput(absent) {
			t.Errorf("%q must NOT be a member — the host rejects an envelope naming it", absent)
		}
	}
	if HookSpecificOutputUnionRecordedAt != "2026-09-02" || !strings.Contains(HookSpecificOutputUnionVersion, "2.1.257") {
		t.Fatalf("fixture provenance moved without the members being re-read: %s / %s",
			HookSpecificOutputUnionVersion, HookSpecificOutputUnionRecordedAt)
	}
}

// Encode refuses model-facing fields on a non-member event, so the property
// "every envelope edikt can emit uses a member" holds at the encoder.
func TestEncode_RefusesHookSpecificOutputOnNonMemberEvent(t *testing.T) {
	for _, ev := range []string{"PostCompact", "PreCompact", "InstructionsLoaded"} {
		_, err := Encode(ev, Response{Context: "Context recovered after compaction."})
		if !errors.Is(err, ErrEventNoHookSpecificOutput) {
			t.Errorf("%s with Context: want ErrEventNoHookSpecificOutput, got %v", ev, err)
		}
		_, err = Encode(ev, Response{Decision: DecisionDeny, Reason: "x"})
		if !errors.Is(err, ErrEventNoHookSpecificOutput) {
			t.Errorf("%s with Decision: want ErrEventNoHookSpecificOutput, got %v", ev, err)
		}
	}
	// Sensitivity control: the same Response on a member encodes, and a
	// non-member with ONLY a systemMessage encodes — that is the shape the
	// host reads on those events.
	if b, err := Encode("SessionStart", Response{Context: "x"}); err != nil || !strings.Contains(string(b), `"hookEventName": "SessionStart"`) {
		t.Fatalf("member event must encode: %v %s", err, b)
	}
	b, err := Encode("PostCompact", Response{Message: "operator only"})
	if err != nil || strings.Contains(string(b), "hookSpecificOutput") || !strings.Contains(string(b), `"systemMessage"`) {
		t.Fatalf("non-member with systemMessage only must encode without an envelope: %v %s", err, b)
	}
}

// The shim's absence banner follows the same rule: an envelope for members,
// systemMessage alone for the rest.
func TestAbsenceBanner_EnvelopeOnlyForUnionMembers(t *testing.T) {
	b, err := AbsenceBanner("post-compact", "PostCompact")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b, "hookSpecificOutput") || !strings.Contains(b, `"systemMessage"`) {
		t.Fatalf("PostCompact banner must be systemMessage only:\n%s", b)
	}
	b, err = AbsenceBanner("session-start", "SessionStart")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b, `"hookEventName":"SessionStart"`) {
		t.Fatalf("SessionStart banner must carry the envelope:\n%s", b)
	}
	if got := NoPython3HookSpecificBanner("PostCompact", "m"); strings.Contains(got, "hookSpecificOutput") {
		t.Fatalf("no-python3 banner on PostCompact must not carry an envelope: %s", got)
	}
	if got := NoPython3HookSpecificBanner("UserPromptSubmit", "m"); !strings.Contains(got, `"hookEventName":"UserPromptSubmit"`) {
		t.Fatalf("no-python3 banner on a member keeps its envelope: %s", got)
	}
}
