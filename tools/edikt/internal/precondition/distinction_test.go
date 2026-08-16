package precondition

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/verify"
)

// TestRunnerCannotDistinguish_PreconditionAbsentFromRuleViolated is the
// defect, written as an executable claim rather than as prose.
//
// Two verify commands are run for real through internal/verify:
//
//	A. the file exists, and the asserted pattern is not in it → RULE VIOLATED
//	B. the file does not exist at all                         → PRECONDITION ABSENT
//
// The runner returns the SAME status for both. That is why greenfield
// extraction's `rg -q '…' internal/rag/chunk.go` over a missing file
// produced a FAIL and COMPILE_EXIT=1: a file that was never there was
// reported as a governance violation, and nothing in the output could
// separate the two.
//
// The test then shows this package DOES separate them from the same inputs.
// If a future change teaches the runner the difference, the first assertion
// fails and this test must be rewritten to pin the new contract — which is
// the point: the conflation is pinned, so it cannot be quietly half-fixed.
func TestRunnerCannotDistinguish_PreconditionAbsentFromRuleViolated(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "present.go")
	if err := os.WriteFile(real, []byte("package rag\n// no Chunk here\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	violated := verify.RunOne("A", "rule violated",
		"grep -q 'func Chunk' present.go", verify.RunOptions{Cwd: dir})
	absent := verify.RunOne("B", "precondition absent",
		"grep -q 'func Chunk' missing.go", verify.RunOptions{Cwd: dir})

	// Guard the experiment before reading it. If either command somehow
	// passed, the comparison below would be vacuous.
	if violated.Status == verify.StatusPassed || absent.Status == verify.StatusPassed {
		t.Fatalf("neither command should pass; got A=%q B=%q", violated.Status, absent.Status)
	}

	if violated.Status != absent.Status {
		t.Fatalf("the runner now distinguishes these states (A=%q, B=%q).\n"+
			"That is an improvement, not a failure — but this test pins the "+
			"conflation, so update it to pin the new contract instead.",
			violated.Status, absent.Status)
	}
	t.Logf("runner reports %q for BOTH a violated rule and an absent precondition",
		violated.Status)

	// Same two commands, through this package. These must differ.
	exists := present("present.go")
	vA, _, _, _, _ := CheckCommand("grep -q 'func Chunk' present.go", exists)
	vB, missingB, _, _, _ := CheckCommand("grep -q 'func Chunk' missing.go", exists)

	if vA != Satisfied {
		t.Errorf("A: verdict = %q, want %q — the file is there, so the "+
			"precondition holds and the FAIL is about the rule", vA, Satisfied)
	}
	if vB != Absent {
		t.Errorf("B: verdict = %q, want %q — the file is absent, so the "+
			"command reports on nothing", vB, Absent)
	}
	if len(missingB) != 1 || missingB[0] != "missing.go" {
		t.Errorf("B: missing = %v, want [missing.go] — naming the absent path "+
			"is what makes the state actionable", missingB)
	}
}
