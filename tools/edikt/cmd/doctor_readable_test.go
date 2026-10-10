package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/verify"
)

// FR-035: doctor's output must be READABLE — one finding, one line, and the
// active plan only. On the reference repo 159 of 184 warning lines were one
// finding printed once per phase of sixteen historical plans, which buries
// the one signal that matters.

func plantPlan(t *testing.T, root, id string, phases []string, active bool) {
	t.Helper()
	dir := filepath.Join(root, "docs", "internal", "plans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var rows strings.Builder
	rows.WriteString("# Plan: " + id + "\n\n## Progress\n\n| Phase | Status | Attempt | Updated |\n|---|---|---|---|\n")
	for _, p := range phases {
		status := "done"
		if active && p == phases[len(phases)-1] {
			status = "in-progress"
		}
		rows.WriteString("| " + p + " | " + status + " | 1/5 | 2026-05-21 |\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "PLAN-"+id+".md"), []byte(rows.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	crit := map[string]any{"schema_version": 1, "plan": "PLAN-" + id, "phases": []any{}}
	b, _ := json.Marshal(crit)
	if err := os.WriteFile(filepath.Join(dir, "PLAN-"+id+"-criteria.yaml"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// One roll-up line per plan, and the historical ones stay silent.
func TestVerifyChecks_ActivePlanRollup(t *testing.T) {
	root := t.TempDir()
	plantPlan(t, root, "active-one", []string{"1", "2", "3"}, true)
	for i := 0; i < 4; i++ {
		plantPlan(t, root, fmt.Sprintf("historical-%d", i), []string{"1", "2", "3", "4"}, false)
	}
	var out bytes.Buffer
	warns, ran := runVerifyChecks(root, &out)
	if !ran {
		t.Fatal("plans with criteria exist; the check must run")
	}
	s := out.String()
	lines := 0
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, "without a recent passing report") || strings.Contains(l, "no recent passing verification report") {
			lines++
		}
	}
	if lines > 1 {
		t.Fatalf("staleness must be ONE roll-up line for the active plan, got %d lines:\n%s", lines, s)
	}
	if strings.Contains(s, "historical-") {
		t.Errorf("a completed historical plan must stay silent:\n%s", s)
	}
	if warns > 1 {
		t.Errorf("one finding is one warning, got %d", warns)
	}
}

// A verify that RAN and FAILED is the one signal that must not be buried.
// It used to print in the same shade as "never measured" (FR-035).
func TestVerifyCoverage_FailingIsRed(t *testing.T) {
	root := scaffoldProject(t)
	mdRel := "docs/architecture/decisions/ADR-777-red.md"
	validMd(t, root, mdRel)
	writeFile(t, filepath.Join(root, "docs/architecture/decisions/ADR-777-red.edikt.yaml"),
		covSidecar(mdRel, "true"))
	r := verify.NewReport("gov-ADR-777", "all", "deadbeef", []verify.Result{
		{ID: "directives[0]", Statement: "Hooks must emit JSON.", Status: verify.StatusFailed, ExitCode: 1},
	})
	if _, err := verify.WriteReports(filepath.Join(root, ".edikt", "state", "verify"), r); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	errN, _, ran := runVerifyCoverageChecks(root, &out)
	if !ran {
		t.Fatal("a project with sidecars must be measured")
	}
	if errN == 0 {
		t.Fatalf("a failing persisted verify must be RED, not a warning:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "🔴") {
		t.Errorf("the finding must render as RED:\n%s", out.String())
	}
}

// Doctor's cost must not scale with the state directory: the reference repo
// had 129,170 report files and grew by 5–11k a day, and the per-row and
// per-id scans read the directory once per row.
func TestDoctor_StateDirIndexedOnce(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, ".edikt", "state", "verify")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 500; i++ {
		name := fmt.Sprintf("gov-ADR-%03d-all-20260101T000000Z.json", i)
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte(`{"schema_version":1}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	idx, reads := indexVerifyState(stateDir)
	if reads != 1 {
		t.Fatalf("the state directory must be read ONCE per doctor run, got %d reads", reads)
	}
	if len(idx) == 0 {
		t.Fatal("the index is empty — nothing was indexed, which is not the same as an empty directory")
	}
	if got := len(idx["ADR-001"]); got != 1 {
		t.Errorf("the index must group by id, got %d entries for ADR-001", got)
	}
}
