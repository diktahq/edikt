package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
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
	//
	// D119b: err=nil for every code, including the failure ones, was the
	// defect — every real phasea.Plan/.Restore/.Verify return that carries a
	// non-OK ExitCode ALSO returns a non-nil error (verify.go's
	// `return out, nil` fires only on ExitVerifyOK), so an err=nil call for
	// codes 1..5 tested a combination production never produces. This test
	// used to pass with the exit-code clobber still in place; it does not
	// anymore.
	cases := []struct {
		code int
		err  error
	}{
		{0, nil},
		{1, errors.New("items incomplete")},
		{2, errors.New("plan rejected")},
		{3, errors.New("malformed")},
		{5, errors.New("failed terminal")},
	}
	for _, c := range cases {
		cmd, _, _ := newTestCmd()
		got := codeOf(t, emitPhaseAOutcome(cmd, c.code, "msg", c.err))
		if got != c.code {
			t.Fatalf("outcome code %d (err=%v) surfaced as %d", c.code, c.err, got)
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
	// prune-stale is pure file deletion over .edikt/state/phasea/ — no LLM
	// dispatch, no fan-out — so it belongs on this allowlist alongside the
	// three original verbs, not excluded by it.
	want := map[string]bool{"plan": true, "restore": true, "verify": true, "prune-stale": true}
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

// A normal outcome prints ONCE, on stdout, and nothing re-prints it as an
// error. `plan` with a current corpus used to print its sentence twice — the
// second time on stderr prefixed "error:" — so a caller reading the transcript
// saw a failure where the answer was "there is nothing to do" (D81 c). The
// exit code is unchanged: tier-1 branches on it.
func TestPhaseAPlan_NothingToPlanPrintsOnceAndIsNotAnError(t *testing.T) {
	var out, errOut bytes.Buffer
	c := &cobra.Command{Use: "phasea"}
	c.SetOut(&out)
	c.SetErr(&errOut)

	err := emitPhaseAOutcomeNormal(c, phasea.ExitNothingToPlan, "nothing to plan — no missing or stale sidecars in the governed corpus", nil, phasea.ExitNothingToPlan)

	if got := strings.Count(out.String(), "nothing to plan"); got != 1 {
		t.Fatalf("the message must print exactly once on stdout, got %d:\n%s", got, out.String())
	}
	var ece *exitCodeError
	if !errors.As(err, &ece) {
		t.Fatalf("the exit code must survive as an exitCodeError, got %v", err)
	}
	if ece.code != phasea.ExitNothingToPlan {
		t.Fatalf("exit code %d, want %d", ece.code, phasea.ExitNothingToPlan)
	}
	if ece.msg != "" {
		t.Fatalf("a normal outcome must carry no error text (it is printed as `error: …` otherwise), got %q", ece.msg)
	}
}

// A failure prints once too — on stderr, through the exit-code error, never
// on stdout where a caller reads results.
//
// D119b: err was nil here too. Passing the real shape — a non-nil error
// alongside the failure code — is what actually exercises the branch that
// fires in production; with err=nil this test could not have caught the
// exit-code clobber.
func TestPhaseAOutcome_FailurePrintsOnceOnStderr(t *testing.T) {
	var out, errOut bytes.Buffer
	c := &cobra.Command{Use: "phasea"}
	c.SetOut(&out)
	c.SetErr(&errOut)

	msg := "item ADR-900-d0 failed terminally"
	err := emitPhaseAOutcomeNormal(c, phasea.ExitVerifyFailed, msg, errors.New(msg), phasea.ExitNothingToPlan)

	if out.Len() != 0 {
		t.Fatalf("a failure must not print on stdout:\n%s", out.String())
	}
	var ece *exitCodeError
	if !errors.As(err, &ece) {
		t.Fatalf("want an exitCodeError, got %v", err)
	}
	if ece.code != phasea.ExitVerifyFailed || ece.msg == "" {
		t.Fatalf("a failure keeps its code and its message: code=%d msg=%q", ece.code, ece.msg)
	}
}

// The exit table lives on the verb group, once. A caller that assumes 1 means
// the same thing on plan and on verify retries a refusal or accepts a loss.
func TestPhaseA_ExitTableIsDocumentedOnTheGroup(t *testing.T) {
	help := newPhaseAPlanCmd().Long
	// "restore receipt" pins exit 7 (D119): it existed as ExitVerifyRestoreMissing
	// but was absent from this table entirely.
	for _, want := range []string{"EXIT CODES", "nothing to plan", "failed terminally", "diverged", "restore receipt"} {
		if !strings.Contains(help, want) {
			t.Errorf("the phasea exit table does not mention %q", want)
		}
	}
}
