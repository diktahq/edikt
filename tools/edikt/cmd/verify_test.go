package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scaffoldVerifyProject builds a minimal project tree at root and writes
// a criteria sidecar with the given phases body. Returns the project root.
func scaffoldVerifyProject(t *testing.T, planID, phasesBody string) string {
	t.Helper()
	root := t.TempDir()
	plansDir := filepath.Join(root, "docs", "internal", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "plan: " + planID + "\nschema_version: 1\n" + phasesBody
	p := filepath.Join(plansDir, "PLAN-"+planID+"-criteria.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write criteria: %v", err)
	}
	return root
}

// runVerify execs the built binary with `verify` plus args, scoped to
// root as the working directory. Returns combined output and exit code.
func runVerify(t *testing.T, root string, args ...string) (string, int) {
	t.Helper()
	bin := buildBinary(t)
	full := append([]string{"verify"}, args...)
	cmd := exec.Command(bin, full...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return string(out), ee.ExitCode()
	}
	t.Fatalf("exec: %v", err)
	return "", -1
}

func TestVerifyCmd_exitCodes(t *testing.T) {
	t.Run("exit 0 on all-pass", func(t *testing.T) {
		root := scaffoldVerifyProject(t, "demo", `phases:
  - id: "1"
    name: pass
    classification: testable
    criteria:
      - id: 1.1
        statement: ok
        verify: "exit 0"
`)
		out, code := runVerify(t, root, "demo", "--phase", "1")
		if code != 0 {
			t.Fatalf("exit code: got %d, want 0\n%s", code, out)
		}
	})

	t.Run("exit 1 on failure", func(t *testing.T) {
		root := scaffoldVerifyProject(t, "demo", `phases:
  - id: "1"
    name: fail
    classification: testable
    criteria:
      - id: 1.1
        statement: bad
        verify: "exit 1"
`)
		out, code := runVerify(t, root, "demo", "--phase", "1")
		if code != 1 {
			t.Fatalf("exit code: got %d, want 1\n%s", code, out)
		}
	})

	t.Run("exit 2 when sidecar missing", func(t *testing.T) {
		root := t.TempDir()
		// No criteria sidecar exists.
		out, code := runVerify(t, root, "ghost")
		if code != 2 {
			t.Fatalf("exit code: got %d, want 2\n%s", code, out)
		}
		if !strings.Contains(out, "no criteria sidecar") {
			t.Errorf("expected 'no criteria sidecar' in output: %s", out)
		}
	})

	t.Run("exit 3 on invalid plan-id", func(t *testing.T) {
		root := t.TempDir()
		out, code := runVerify(t, root, "..//bad")
		if code != 3 {
			t.Fatalf("exit code: got %d, want 3\n%s", code, out)
		}
	})

	t.Run("exit 3 on unknown phase", func(t *testing.T) {
		root := scaffoldVerifyProject(t, "demo", `phases:
  - id: "1"
    name: ok
    classification: testable
    criteria:
      - id: 1.1
        statement: ok
        verify: "exit 0"
`)
		out, code := runVerify(t, root, "demo", "--phase", "99")
		if code != 3 {
			t.Fatalf("exit code: got %d, want 3\n%s", code, out)
		}
	})
}

func TestVerifyCmd_phaseFilter(t *testing.T) {
	root := scaffoldVerifyProject(t, "demo", `phases:
  - id: "1"
    name: phase one
    classification: testable
    criteria:
      - id: 1.1
        statement: from-phase-1
        verify: "exit 0"
  - id: "2"
    name: phase two
    classification: testable
    criteria:
      - id: 2.1
        statement: from-phase-2
        verify: "exit 0"
`)
	out, code := runVerify(t, root, "demo", "--phase", "1")
	if code != 0 {
		t.Fatalf("exit code: %d\n%s", code, out)
	}
	if !strings.Contains(out, "1.1") {
		t.Errorf("expected 1.1 in output: %s", out)
	}
	if strings.Contains(out, "2.1") {
		t.Errorf("phase filter should exclude 2.1: %s", out)
	}
}

func TestVerifyCmd_allowFailures(t *testing.T) {
	root := scaffoldVerifyProject(t, "demo", `phases:
  - id: "1"
    name: mixed
    classification: testable
    criteria:
      - id: 1.1
        statement: pass
        verify: "exit 0"
      - id: 1.2
        statement: fail
        verify: "exit 5"
`)
	// Without --allow-failures: exit 1.
	_, code := runVerify(t, root, "demo", "--phase", "1")
	if code != 1 {
		t.Fatalf("without flag: got %d, want 1", code)
	}
	// With --allow-failures: exit 0 but report still records failure.
	out, code := runVerify(t, root, "demo", "--phase", "1", "--allow-failures")
	if code != 0 {
		t.Fatalf("with flag: got %d, want 0\n%s", code, out)
	}
	// Confirm the report records the failure.
	reports, err := filepath.Glob(filepath.Join(root, ".edikt", "state", "verify", "demo-phase-1-*.json"))
	if err != nil || len(reports) == 0 {
		t.Fatalf("report glob: %v / %v", err, reports)
	}
	body, err := os.ReadFile(reports[len(reports)-1])
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var r struct {
		Summary struct {
			Failed int `json:"failed"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("parse report: %v", err)
	}
	if r.Summary.Failed != 1 {
		t.Errorf("report should record the failure: %s", body)
	}
}

func TestVerifyCmd_jsonFlag(t *testing.T) {
	root := scaffoldVerifyProject(t, "demo", `phases:
  - id: "1"
    name: ok
    classification: testable
    criteria:
      - id: 1.1
        statement: ok
        verify: "exit 0"
`)
	out, code := runVerify(t, root, "demo", "--phase", "1", "--json")
	if code != 0 {
		t.Fatalf("exit: %d\n%s", code, out)
	}
	// --json should emit the report as JSON to stdout. Find a parseable
	// JSON object embedded in output (pin warning may precede it).
	idx := strings.Index(out, "{\n  \"plan_id\":")
	if idx < 0 {
		t.Fatalf("no JSON report in output: %s", out)
	}
	var r struct {
		PlanID string `json:"plan_id"`
		Phase  string `json:"phase"`
	}
	if err := json.Unmarshal([]byte(out[idx:][:strings.LastIndex(out[idx:], "}")+1]), &r); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, out)
	}
	if r.PlanID != "demo" {
		t.Errorf("plan_id: %q", r.PlanID)
	}
	if r.Phase != "1" {
		t.Errorf("phase: %q", r.Phase)
	}
}

func TestVerifyCmd_writesStateDir(t *testing.T) {
	root := scaffoldVerifyProject(t, "demo", `phases:
  - id: "1"
    name: ok
    classification: testable
    criteria:
      - id: 1.1
        statement: ok
        verify: "exit 0"
`)
	_, code := runVerify(t, root, "demo", "--phase", "1")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	stateDir := filepath.Join(root, ".edikt", "state", "verify")
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatalf("read state dir: %v", err)
	}
	gotJSON, gotTxt := false, false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			gotJSON = true
		}
		if strings.HasSuffix(e.Name(), ".txt") {
			gotTxt = true
		}
	}
	if !gotJSON || !gotTxt {
		t.Errorf("expected json+txt reports, got: %v", entries)
	}
}

// ── retired-parent sidecars must not gate verify all (bok field bug #2) ─────

func TestRunVerifyAll_SkipsRetiredParentSidecars(t *testing.T) {
	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Retired parent + leftover sidecar with an always-failing verify.
	parent := "---\nstatus: superseded\nsuperseded_by: GL-004\n---\n\n# ADR-002\n"
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-002-retired.md"), []byte(parent), 0o644); err != nil {
		t.Fatal(err)
	}
	sc := `schema_version: 1
topic: "testing"
path: "docs/architecture/decisions/ADR-002-retired.md"
signals: []
directives:
  - text: "Dead rule MUST hold. (ref: ADR-002)"
    source_excerpt:
      line_start: 1
      line_end: 1
      quote: "x"
    verify: "false"
    verify_kind: structural
`
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-002-retired.edikt.yaml"), []byte(sc), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := runVerifyAll(root, false)
	if err != nil {
		t.Fatalf("runVerifyAll: %v", err)
	}
	if report.Summary.Failed != 0 {
		t.Errorf("retired-parent sidecar verifies must be skipped, got %d failed", report.Summary.Failed)
	}
	for _, s := range report.Sidecars {
		if strings.Contains(s.Path, "ADR-002") {
			t.Errorf("retired-parent sidecar must not appear in the report: %s", s.Path)
		}
	}
}

// ── gate scoping: gov-only walk (bok field bug #4) ───────────────────────────

func TestRunVerifyAll_GovOnlyExcludesPrdAndSpec(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{
		"docs/architecture/decisions",
		"docs/product/prds",
		"docs/product/specs",
	} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Passing gov sidecar with live parent.
	parent := "---\nstatus: accepted\n---\n\n# ADR-001\n"
	if err := os.WriteFile(filepath.Join(root, "docs/architecture/decisions/ADR-001-live.md"), []byte(parent), 0o644); err != nil {
		t.Fatal(err)
	}
	gov := `schema_version: 1
topic: "testing"
path: "docs/architecture/decisions/ADR-001-live.md"
signals: []
directives:
  - text: "Rule MUST hold. (ref: ADR-001)"
    source_excerpt:
      line_start: 1
      line_end: 1
      quote: "x"
    verify: "true"
    verify_kind: structural
`
	if err := os.WriteFile(filepath.Join(root, "docs/architecture/decisions/ADR-001-live.edikt.yaml"), []byte(gov), 0o644); err != nil {
		t.Fatal(err)
	}
	// Failing PRD sidecar — WIP work whose verify targets unbuilt binaries.
	prd := "id: PRD-001\nrequirements:\n  - id: FR-001\n    text: \"future work\"\n    verify: \"false\"\n"
	if err := os.WriteFile(filepath.Join(root, "docs/product/prds/PRD-001.yaml"), []byte(prd), 0o644); err != nil {
		t.Fatal(err)
	}

	full, err := runVerifyAll(root, false)
	if err != nil {
		t.Fatalf("runVerifyAll(full): %v", err)
	}
	if full.Summary.Failed == 0 {
		t.Fatal("full walk should report the failing PRD verify (fixture broken?)")
	}

	govOnly, err := runVerifyAll(root, true)
	if err != nil {
		t.Fatalf("runVerifyAll(gov-only): %v", err)
	}
	if govOnly.Summary.Failed != 0 {
		t.Errorf("gov-only walk must not run prd/spec verifies, got %d failed", govOnly.Summary.Failed)
	}
	for _, s := range govOnly.Sidecars {
		if s.Kind != "gov" {
			t.Errorf("gov-only walk returned non-gov sidecar: %s/%s", s.Kind, s.ID)
		}
	}
}

// ── plan-id prefix tolerance (bok-services field bug #6: double PLAN- prefix) ─

func TestLocateCriteriaSidecar_AcceptsBothIDForms(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "docs", "internal", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	p := filepath.Join(plansDir, "PLAN-SPEC-032-skill-catalog-criteria.yaml")
	if err := os.WriteFile(p, []byte("plan: x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Bare form.
	got, err := locateCriteriaSidecar(root, "SPEC-032-skill-catalog")
	if err != nil || got != p {
		t.Fatalf("bare id: got (%q, %v), want %q", got, err, p)
	}
	// PLAN-prefixed form MUST NOT double-prefix.
	got, err = locateCriteriaSidecar(root, "PLAN-SPEC-032-skill-catalog")
	if err != nil || got != p {
		t.Fatalf("PLAN-prefixed id: got (%q, %v), want %q", got, err, p)
	}
}
