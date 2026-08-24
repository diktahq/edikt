package gov

import (
	"os"
	"path/filepath"
	"testing"
)

// INV-018/PRD-010 Phase 31 — runInjectionBudgetReport is wired into gov
// compile as a NON-BLOCKING report: it must never change compile's exit
// code, whether the underlying script is absent, exits clean, or reports a
// regression (exit 1). These tests exercise the function directly, the same
// pattern TestRunPostCompileVerify_LaunchFailureIsNotAPass uses for the
// fail-CLOSED gate — here proving the opposite property for the fail-OPEN
// one.

// TestRunInjectionBudgetReport_AbsentScriptIsSilentNoOp is AC-31.3's
// no-script-shipped case: an installed user project has no
// test/test-injection-budget.sh (INV-017 keeps test/ private to edikt-dev),
// and the function must not panic, error, or otherwise misbehave — it is
// simply not applicable there.
func TestRunInjectionBudgetReport_AbsentScriptIsSilentNoOp(t *testing.T) {
	root := t.TempDir()
	// No test/ directory at all — the common shape for an installed project.
	runInjectionBudgetReport(root) // must not panic
}

// TestRunInjectionBudgetReport_RegressionExitDoesNotPropagate is AC-31.2: a
// script that reports a regression (exit 1, matching
// test/test-injection-budget.sh's real contract when it finds one) must not
// cause any observable failure from this function — there is nothing to
// assert on a void return, which is itself the point: the caller in
// compile.go never inspects a result, by construction.
func TestRunInjectionBudgetReport_RegressionExitDoesNotPropagate(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "test")
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(testDir, "test-injection-budget.sh")
	// A fake detector that behaves like the real one on a regression: prints
	// a report and exits 1.
	fake := "#!/bin/sh\necho 'PASS-through fixture: REGRESSION on some/glob'\nexit 1\n"
	if err := os.WriteFile(script, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	// Must not panic, and must return normally regardless of the script's
	// exit code — that IS the non-blocking contract this function exists
	// to implement.
	runInjectionBudgetReport(root)
}

// TestRunInjectionBudgetReport_RunsScriptWhenPresent is the isolation
// control: without it, TestRunInjectionBudgetReport_AbsentScriptIsSilentNoOp
// passing trivially (because the function always no-ops) would go
// undetected. Proves the script actually executes by having it create a
// marker file as a side effect.
func TestRunInjectionBudgetReport_RunsScriptWhenPresent(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "test")
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(testDir, "ran.marker")
	script := filepath.Join(testDir, "test-injection-budget.sh")
	fake := "#!/bin/sh\ntouch \"" + marker + "\"\nexit 0\n"
	if err := os.WriteFile(script, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	runInjectionBudgetReport(root)
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("script did not run — marker file was not created")
	}
}
