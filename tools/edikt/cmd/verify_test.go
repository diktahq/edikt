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
