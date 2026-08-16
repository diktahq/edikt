package cmd

import (
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/internal/verify"
)

// TestRunGovVerifies_SuppressedDirectiveIsSkippedNotFailed pins the defect
// found 2026-08-14: runGovVerifies iterated sc.Directives raw and ran every
// verify: unconditionally, never consulting suppressed_directives the way
// the render path (phaseb/merge.go) does. A suppressed directive carrying a
// verify: that would fail (checking a rule that no longer compiles) made
// `bin/edikt verify gov <id>` exit 1 even though the directive was correctly
// excluded from every rendered surface -- exactly what happened to ADR-017
// this session before it was patched by hand. This is the general fix.
func TestRunGovVerifies_SuppressedDirectiveIsSkippedNotFailed(t *testing.T) {
	sc := &sidecar.Sidecar{
		SuppressedDirectives: []string{"suppressed rule text"},
		Directives: []sidecar.Directive{
			{Text: "suppressed rule text", Verify: "exit 1"},           // would fail if run
			{Text: "live rule text", Verify: "exit 0"},                 // must still run and pass
			{Text: "live rule with no verify"},                         // must still be skipped:operational
		},
	}

	results := runGovVerifies(sc, ".")
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	suppressed := results[0]
	if suppressed.Status != verify.StatusSkippedSuppressed {
		t.Errorf("suppressed directive: got status %q, want %q", suppressed.Status, verify.StatusSkippedSuppressed)
	}
	if suppressed.Statement != "suppressed rule text" {
		t.Errorf("suppressed directive: statement not preserved, got %q", suppressed.Statement)
	}

	live := results[1]
	if live.Status != verify.StatusPassed {
		t.Errorf("live directive: got status %q, want %q — the fix must not suppress non-suppressed directives", live.Status, verify.StatusPassed)
	}

	noVerify := results[2]
	if noVerify.Status != verify.StatusSkippedOperational {
		t.Errorf("no-verify directive: got status %q, want %q", noVerify.Status, verify.StatusSkippedOperational)
	}

	report := verify.NewReport("gov-test", "all", "", results)
	if report.AnyFailures() {
		t.Errorf("report has failures, want none: %+v", report.Summary)
	}
	if report.Summary.Skipped != 2 {
		t.Errorf("expected 2 skipped (1 suppressed + 1 no-verify), got %d", report.Summary.Skipped)
	}
	if report.Summary.Passed != 1 {
		t.Errorf("expected 1 passed, got %d", report.Summary.Passed)
	}
}

// TestRunGovVerifies_NoSuppressionIsUnaffected is the control: with an empty
// SuppressedDirectives list, behavior is unchanged from before this fix.
func TestRunGovVerifies_NoSuppressionIsUnaffected(t *testing.T) {
	sc := &sidecar.Sidecar{
		Directives: []sidecar.Directive{
			{Text: "rule one", Verify: "exit 0"},
			{Text: "rule two", Verify: "exit 1"},
		},
	}
	results := runGovVerifies(sc, ".")
	if results[0].Status != verify.StatusPassed {
		t.Errorf("rule one: got %q, want passed", results[0].Status)
	}
	if results[1].Status != verify.StatusFailed {
		t.Errorf("rule two: got %q, want failed", results[1].Status)
	}
}
