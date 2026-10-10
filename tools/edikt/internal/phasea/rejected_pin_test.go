package phasea

import (
	"strings"
	"testing"
)

// SPEC-020 SR-010 amendment (D57) — a host-rejected explicit extractor.model
// pin is failed-terminal on FIRST observation, quoting the host's error, and
// is never retried: the pin the host refused is the same input a retry would
// resubmit.
func TestRejectedPin(t *testing.T) {
	t.Run("recorded failed-terminal quoting the host error", func(t *testing.T) {
		f := newFixture(t, "")
		// No sidecar written: the dispatch never produced output because
		// the host refused the pin before extraction ran.
		out, _ := Verify(VerifyOptions{
			ProjectRoot: f.root, PlanID: f.planID,
			RejectedPin: map[string]string{f.itemID: "model 'claude-nonexistent-9' is not available"},
		})
		it := onlyItem(t, out)
		if it.Status != StatusFailedTerminal {
			t.Fatalf("status = %q, want %q", it.Status, StatusFailedTerminal)
		}
		if !strings.Contains(it.Diagnostic, "claude-nonexistent-9") {
			t.Fatalf("diagnostic must quote the host's error; got: %s", it.Diagnostic)
		}
		if !IsTerminalStatus(it.Status) {
			t.Fatal("a rejected pin must be terminal")
		}
		if out.ExitCode != ExitVerifyFailed {
			t.Fatalf("exit = %d, want %d (ExitVerifyFailed)", out.ExitCode, ExitVerifyFailed)
		}
	})

	t.Run("terminal on first observation, never carries a retry-exhaustion message", func(t *testing.T) {
		f := newFixture(t, "")
		out, _ := Verify(VerifyOptions{
			ProjectRoot: f.root, PlanID: f.planID,
			RejectedPin: map[string]string{f.itemID: "rejected"},
		})
		it := onlyItem(t, out)
		if it.Attempts != 1 {
			t.Fatalf("attempts = %d, want 1 — a rejected pin never retries", it.Attempts)
		}
		if strings.Contains(it.Diagnostic, "retry budget exhausted") {
			t.Fatalf("a first-observation rejected pin must not read as retry exhaustion: %s", it.Diagnostic)
		}
	})

	t.Run("the diagnostic is length-capped", func(t *testing.T) {
		f := newFixture(t, "")
		huge := strings.Repeat("x", rejectedPinDiagnosticCap*2)
		out, _ := Verify(VerifyOptions{
			ProjectRoot: f.root, PlanID: f.planID,
			RejectedPin: map[string]string{f.itemID: huge},
		})
		it := onlyItem(t, out)
		if len(it.Diagnostic) >= len(huge) {
			t.Fatalf("diagnostic (%d chars) was not capped against a %d-char host error", len(it.Diagnostic), len(huge))
		}
	})

	// Output on disk is the evidence, the rejection report is not — same
	// rule as Stalled (TestVerify_StalledReportDoesNotOverrideOutput).
	t.Run("output on disk overrides the rejection report", func(t *testing.T) {
		f := newFixture(t, "")
		f.extractGood(t)
		out, _ := Verify(VerifyOptions{
			ProjectRoot: f.root, PlanID: f.planID,
			RejectedPin: map[string]string{f.itemID: "rejected"},
		})
		it := onlyItem(t, out)
		if it.Status != StatusOK {
			t.Fatalf("status = %q, want ok — a rejection report must not override real output", it.Status)
		}
	})
}
