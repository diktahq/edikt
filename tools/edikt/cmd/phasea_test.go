package cmd

import (
	"bytes"
	"errors"
	"testing"

	"github.com/spf13/cobra"
)

func newTestCmd() (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	c := &cobra.Command{}
	var out, errb bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&errb)
	return c, &out, &errb
}

func codeOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var ece *exitCodeError
	if !errors.As(err, &ece) {
		t.Fatalf("want *exitCodeError, got %T: %v", err, err)
	}
	return ece.code
}

// ADR-029 makes the exit code the entire consumable output: tier-1 acts on
// the code and never parses prose. Every code an Outcome can carry must
// therefore reach the caller unchanged.
func TestPhaseAOutcome_PropagatesEveryExitCode(t *testing.T) {
	// D9 moved FAILED-terminal to 5; 4 is already the pre-run refusal code.
	for _, code := range []int{0, 1, 2, 3, 5} {
		cmd, _, _ := newTestCmd()
		got := codeOf(t, emitPhaseAOutcome(cmd, code, "msg", nil))
		if got != code {
			t.Fatalf("outcome code %d surfaced as %d", code, got)
		}
	}
}

// A Go-level failure must not collapse into 1. Exit 1 means "items failed",
// which tier-1 retries on; an internal error that reports 1 would be retried
// forever against a fault no retry can clear.
func TestPhaseAOutcome_InternalErrorIsNotItemsFailed(t *testing.T) {
	cmd, _, _ := newTestCmd()
	got := codeOf(t, emitPhaseAOutcome(cmd, 0, "", errors.New("config unreadable")))
	if got == 1 {
		t.Fatal("an internal error exited 1 — tier-1 will retry a fault no retry can clear")
	}
	if got != 2 {
		t.Fatalf("want exit 2 for an internal error, got %d", got)
	}
}

// A nil outcome is the shape where a missing exit code is easiest to invent.
// It must refuse with 2, never report success.
func TestPhaseAOutcome_NilOutcomeNeverReportsSuccess(t *testing.T) {
	cmd, _, _ := newTestCmd()
	if got := codeOf(t, emitPhaseAVerifyOutcome(cmd, nil, errors.New("boom"))); got != 2 {
		t.Fatalf("nil verify outcome exited %d, want 2", got)
	}
	cmd2, _, _ := newTestCmd()
	if got := codeOf(t, emitPhaseARestoreOutcome(cmd2, nil, errors.New("boom"))); got != 2 {
		t.Fatalf("nil restore outcome exited %d, want 2", got)
	}
}

// The verb group must not grow a dispatch subcommand. INV-012 forbids a
// tier-2 binary from invoking an LLM, and a `phasea dispatch` verb is exactly
// how that prohibition would be re-entered after ClaudeRunner is deleted.
func TestPhaseACmd_HasNoDispatchVerb(t *testing.T) {
	root := newPhaseACmd()
	want := map[string]bool{"plan": true, "restore": true, "verify": true}
	for _, sub := range root.Commands() {
		if !want[sub.Name()] {
			t.Fatalf("unexpected phasea subcommand %q — the fan-out belongs to tier-1 (INV-012)", sub.Name())
		}
		delete(want, sub.Name())
	}
	if len(want) != 0 {
		t.Fatalf("missing phasea subcommands: %v", want)
	}
}
