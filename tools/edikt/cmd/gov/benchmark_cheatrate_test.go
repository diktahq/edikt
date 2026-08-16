package gov

// benchmark_cheatrate_test.go — exit-code contract tests for the
// `bin/edikt gov benchmark cheat-rate` subcommand (SPEC-009 Plan C
// Phase 2 / AC-2.3 / ADR-040).
//
// Contract (mirroring sidecar approve's test layout in
// cmd/sidecar_approve_test.go):
//   0 — run completed
//   1 — sandbox / dispatch error
//   2 — sidecar id specified but no matching fixture / sidecar found
//   3 — invalid or missing arguments
//
// Phase 2 only exercises the no-LLM paths (exit 2 stub miss, exit 3
// missing/invalid args). The full dispatch lands in Phase 3.

import (
	"os"
	"testing"
)

// TestBenchmarkCheatRate_ExitCodes — the AC-2.3 exit-code branches.
func TestBenchmarkCheatRate_ExitCodes(t *testing.T) {
	t.Run("exit2_sidecar_not_found", func(t *testing.T) {
		// Stub mode: an id with no matching fixture must exit 2.
		// Using the regex-valid but unfixtured id "ADR-999" guarantees
		// the lookup miss without tripping the exit-3 validator.
		t.Setenv("EDIKT_CHEAT_RATE_STUB", "1")

		out, err := runGovCmd(t,
			"gov", "benchmark", "cheat-rate", "ADR-999",
		)
		if !isExitCode(err, 2) {
			t.Fatalf("want exit 2, got: %v\noutput: %s", err, out)
		}
	})

	t.Run("exit3_no_args", func(t *testing.T) {
		// No positional id AND no --all → exit 3.
		// Run with stub mode off so we exercise the argv validator,
		// not the stub fixture loader.
		_ = os.Unsetenv("EDIKT_CHEAT_RATE_STUB")

		out, err := runGovCmd(t,
			"gov", "benchmark", "cheat-rate",
		)
		if !isExitCode(err, 3) {
			t.Fatalf("want exit 3, got: %v\noutput: %s", err, out)
		}
	})
}
