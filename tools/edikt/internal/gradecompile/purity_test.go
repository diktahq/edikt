// purity_test.go — Phase B ↔ grader isolation gate (ADR-028).
//
// SPEC-009 Plan H. The compile-quality grader is a POST-COMPILE,
// LLM-dispatching step. ADR-028 requires Phase B (the deterministic
// compile merge) to stay pure — no subagent dispatch, no shell-out, no
// LLM. This test asserts the grader package is NOT in Phase B's
// transitive import closure, so the grader can never be pulled onto a
// compile code path. It uses `go list -deps` so it sees the full import
// closure, not just phaseb's own source. Mirrors the shape of
// internal/phaseb/purity_test.go.
package gradecompile_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestGradeCompilePurity(t *testing.T) {
	const phasebPath = "github.com/diktahq/edikt/tools/edikt/internal/phaseb"
	const gradecompilePath = "github.com/diktahq/edikt/tools/edikt/internal/gradecompile"

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", phasebPath)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list -deps %s: %v\nstderr: %s", phasebPath, err, stderr.String())
	}

	deps := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	sawPhaseb := false
	for _, line := range deps {
		imp := strings.TrimSpace(line)
		if imp == phasebPath {
			sawPhaseb = true
		}
		if imp == gradecompilePath {
			t.Errorf("phaseb has forbidden transitive import %q — the compile-quality grader must never be reachable from Phase B (ADR-028 purity)", gradecompilePath)
		}
	}
	if !sawPhaseb {
		t.Fatalf("go list -deps did not return %s; stdout:\n%s", phasebPath, stdout.String())
	}
}
