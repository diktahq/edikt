package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCore lays down a compiled core of an exact byte length.
func writeCore(t *testing.T, root string, nbytes int) {
	t.Helper()
	dir := filepath.Join(root, ".claude", "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "governance.md"),
		bytes.Repeat([]byte("x"), nbytes), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestUnmeasuredIsNotAPass. NEGATIVE CONTROL, and the one that matters most:
// with no compiled core there is nothing to judge, and the check must decline
// to report rather than print "ok" (INV-013).
func TestSurfaceBudgetUnmeasuredIsNotAPass(t *testing.T) {
	root := t.TempDir()
	var w bytes.Buffer
	warns, ran := runSurfaceBudgetCheck(root, &w)
	if ran {
		t.Errorf("no compiled core exists; check must report unmeasured, not run")
	}
	if warns != 0 {
		t.Errorf("warns=%d on an unmeasured project", warns)
	}
	if strings.Contains(w.String(), "ok") {
		t.Errorf("a project with no core must never be reported ok:\n%s", w.String())
	}
}

// TestCoreUnderTargetIsQuietButStillReportsTheNumber.
func TestSurfaceBudgetCoreUnderTarget(t *testing.T) {
	root := t.TempDir()
	writeCore(t, root, 4000) // 1,000 est-tokens
	var w bytes.Buffer
	warns, ran := runSurfaceBudgetCheck(root, &w)
	if !ran {
		t.Fatal("a core exists; the check must run")
	}
	if warns != 0 {
		t.Errorf("1,000 est-tokens is well under target; warns=%d\n%s", warns, w.String())
	}
	if !strings.Contains(w.String(), "1000 est-tokens") {
		t.Errorf("the number must print even when healthy, so drift is visible "+
			"before it becomes a breach:\n%s", w.String())
	}
}

// TestCoreOverTargetWarns is the POSITIVE CONTROL for the budget half.
//
// The scenario it stands in for: eleven topic descriptions each at a legal 500
// take the core to ~3,744 est-tokens. Every field is inside its cap and the
// assembled surface is over budget, which is the case no per-field cap can
// express.
func TestSurfaceBudgetCoreOverTargetWarns(t *testing.T) {
	root := t.TempDir()
	writeCore(t, root, 3744*4) // the eleven-at-500 projection
	var w bytes.Buffer
	warns, ran := runSurfaceBudgetCheck(root, &w)
	if !ran {
		t.Fatal("a core exists; the check must run")
	}
	if warns == 0 {
		t.Fatalf("3,744 est-tokens is over the %d target and must warn:\n%s",
			coreTokenTarget, w.String())
	}
	out := w.String()
	for _, want := range []string{"3744 est-tokens", "over the 3500", "EVERY edit"} {
		if !strings.Contains(out, want) {
			t.Errorf("warning must contain %q; got:\n%s", want, out)
		}
	}
}

// TestBoundaryIsNotOffByOne pins the comparison itself.
func TestSurfaceBudgetBoundary(t *testing.T) {
	for _, tc := range []struct {
		tok       int
		wantWarns int
	}{
		{coreTokenTarget - 1, 0},
		{coreTokenTarget, 0}, // "over the target" means strictly greater
		{coreTokenTarget + 1, 1},
	} {
		root := t.TempDir()
		writeCore(t, root, tc.tok*4)
		var w bytes.Buffer
		warns, ran := runSurfaceBudgetCheck(root, &w)
		if !ran {
			t.Fatalf("%d tok: check must run", tc.tok)
		}
		if warns != tc.wantWarns {
			t.Errorf("%d est-tokens: warns=%d want %d\n%s", tc.tok, warns, tc.wantWarns, w.String())
		}
	}
}

// TestSoftCapFor pins the field->soft-cap mapping against the single source in
// internal/sidecar, so a cap changed there and not here fails rather than
// silently reporting against a stale number.
func TestSoftCapFor(t *testing.T) {
	for _, f := range []string{"text", "intent", "falsifying_observation",
		"evidence", "proposed_topic_description"} {
		if c, ok := softCapFor(f); !ok || c <= 0 {
			t.Errorf("softCapFor(%q) = (%d, %v); every capped field needs a soft target", f, c, ok)
		}
	}
	if _, ok := softCapFor("not_a_capped_field"); ok {
		t.Error("an uncapped field must not report a soft cap")
	}
	// text's soft cap must stay BELOW the hard cap, or "soft" means nothing.
	soft, _ := softCapFor("text")
	if soft >= 2000 {
		t.Errorf("soft cap %d is not below the 2000 hard cap", soft)
	}
}
