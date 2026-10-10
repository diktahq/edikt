package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// PLAN-v090-release Phase 0: a criterion may carry timeout_s; the runner
// honours it, and the default stays 30 s when it is absent.
func TestCriterionTimeoutS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "P-criteria.yaml")
	if err := os.WriteFile(path, []byte("schema_version: 1\nplan: P\nphases:\n- id: '1'\n  classification: testable\n  criteria:\n  - id: AC-1.1\n    statement: s\n    verify: 'true'\n    timeout_s: 120\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sc, err := LoadCriteria(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := sc.Phases[0].Criteria[0].TimeoutS; got != 120 {
		t.Fatalf("timeout_s = %d, want 120", got)
	}
}

func TestVerifyRunner_TimeoutSHonoured(t *testing.T) {
	if testing.Short() {
		t.Skip("wall-clock test")
	}
	ph := Phase{ID: "1", Classification: ClassTestable}
	// Default (30 s) would pass a 1.5 s sleep; timeout_s: 1 must record timeout.
	res := RunCriterion(ph, Criterion{ID: "AC-1.1", Statement: "s", Verify: "sleep 1.5", TimeoutS: 1}, RunOptions{Cwd: t.TempDir()})
	if res.Status != StatusTimeout {
		t.Fatalf("timeout_s: 1 on a 1.5 s command must record %q, got %q", StatusTimeout, res.Status)
	}
	// And a generous timeout_s lets a 1 s command pass.
	start := time.Now()
	res = RunCriterion(ph, Criterion{ID: "AC-1.2", Statement: "s", Verify: "sleep 1", TimeoutS: 10}, RunOptions{Cwd: t.TempDir()})
	if res.Status != StatusPassed || time.Since(start) > 9*time.Second {
		t.Fatalf("timeout_s: 10 on a 1 s command must pass promptly, got %q after %s", res.Status, time.Since(start))
	}
}

// A criterion subprocess sees EDIKT_INVOKER, so a walled verb it invokes
// is not refused as an undeclared non-terminal call (SPEC-020 SR-009).
func TestVerifyRunner_ExportsInvoker(t *testing.T) {
	os.Unsetenv("EDIKT_INVOKER")
	res := RunOne("AC-1.1", "s", `test "$EDIKT_INVOKER" = /edikt:verify`, RunOptions{Cwd: t.TempDir()})
	if res.Status != StatusPassed {
		t.Fatalf("EDIKT_INVOKER not exported to the criterion: %q %s", res.Status, strings.TrimSpace(res.StderrExcerpt))
	}
	t.Setenv("EDIKT_INVOKER", "ci")
	res = RunOne("AC-1.2", "s", `test "$EDIKT_INVOKER" = ci`, RunOptions{Cwd: t.TempDir()})
	if res.Status != StatusPassed {
		t.Fatalf("a caller-set EDIKT_INVOKER must be inherited unchanged: %q", res.Status)
	}
}
