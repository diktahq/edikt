package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scaffoldGovProject writes a minimal .edikt/config.yaml plus a single
// gov sidecar under docs/architecture/decisions. Returns the project root.
func scaffoldGovProject(t *testing.T, id, sidecarBody string) string {
	t.Helper()
	root := t.TempDir()
	ediktDir := filepath.Join(root, ".edikt")
	if err := os.MkdirAll(ediktDir, 0o755); err != nil {
		t.Fatalf("mkdir .edikt: %v", err)
	}
	// Minimal config — paths default to docs/architecture/decisions etc.,
	// which is what we use below.
	if err := os.WriteFile(
		filepath.Join(ediktDir, "config.yaml"),
		[]byte("paths:\n  decisions: docs/architecture/decisions\n"),
		0o644,
	); err != nil {
		t.Fatalf("write config: %v", err)
	}
	decisionsDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(decisionsDir, 0o755); err != nil {
		t.Fatalf("mkdir decisions: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(decisionsDir, id+".edikt.yaml"),
		[]byte(sidecarBody),
		0o644,
	); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	return root
}

// scaffoldPRDProject writes config + a PRD sidecar under docs/product/prds.
func scaffoldPRDProject(t *testing.T, id, sidecarBody string) string {
	t.Helper()
	root := t.TempDir()
	ediktDir := filepath.Join(root, ".edikt")
	if err := os.MkdirAll(ediktDir, 0o755); err != nil {
		t.Fatalf("mkdir .edikt: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(ediktDir, "config.yaml"),
		[]byte("paths:\n  prds: docs/product/prds\n"),
		0o644,
	); err != nil {
		t.Fatalf("write config: %v", err)
	}
	prdsDir := filepath.Join(root, "docs", "product", "prds")
	if err := os.MkdirAll(prdsDir, 0o755); err != nil {
		t.Fatalf("mkdir prds: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(prdsDir, id+".yaml"),
		[]byte(sidecarBody),
		0o644,
	); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	return root
}

// scaffoldSPECProject writes config + a SPEC sidecar under docs/product/specs.
func scaffoldSPECProject(t *testing.T, id, sidecarBody string) string {
	t.Helper()
	root := t.TempDir()
	ediktDir := filepath.Join(root, ".edikt")
	if err := os.MkdirAll(ediktDir, 0o755); err != nil {
		t.Fatalf("mkdir .edikt: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(ediktDir, "config.yaml"),
		[]byte("paths:\n  specs: docs/product/specs\n"),
		0o644,
	); err != nil {
		t.Fatalf("write config: %v", err)
	}
	specsDir := filepath.Join(root, "docs", "product", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(specsDir, id+".yaml"),
		[]byte(sidecarBody),
		0o644,
	); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	return root
}

func runVerifySub(t *testing.T, root string, args ...string) (string, int) {
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

// ── gov ──────────────────────────────────────────────────────────────────

func TestVerifyGov_passOnAllPass(t *testing.T) {
	root := scaffoldGovProject(t, "ADR-001", `schema_version: 1
topic: demo
path: docs/architecture/decisions/ADR-001-x.md
signals: []
directives:
  - text: do thing
    source_excerpt: {line_start: 1, line_end: 1, quote: q}
    verify: "exit 0"
`)
	out, code := runVerifySub(t, root, "gov", "ADR-001")
	if code != 0 {
		t.Fatalf("exit: got %d, want 0\n%s", code, out)
	}
	if !strings.Contains(out, "1 passed") {
		t.Errorf("expected '1 passed' in summary, got:\n%s", out)
	}
}

func TestVerifyGov_failOnAnyFail(t *testing.T) {
	root := scaffoldGovProject(t, "ADR-002", `schema_version: 1
topic: demo
path: docs/architecture/decisions/ADR-002-x.md
signals: []
directives:
  - text: bad
    source_excerpt: {line_start: 1, line_end: 1, quote: q}
    verify: "exit 1"
`)
	out, code := runVerifySub(t, root, "gov", "ADR-002")
	if code != 1 {
		t.Fatalf("exit: got %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "1 failed") {
		t.Errorf("expected '1 failed' in summary, got:\n%s", out)
	}
}

func TestVerifyGov_skippedOnMissingVerify(t *testing.T) {
	root := scaffoldGovProject(t, "ADR-003", `schema_version: 1
topic: demo
path: docs/architecture/decisions/ADR-003-x.md
signals: []
directives:
  - text: no verify
    source_excerpt: {line_start: 1, line_end: 1, quote: q}
verification:
  - "[ ] bare-string legacy form"
  - text: "[ ] structured form with no verify"
`)
	out, code := runVerifySub(t, root, "gov", "ADR-003")
	if code != 0 {
		t.Fatalf("exit: got %d, want 0 (no failures, all skipped)\n%s", code, out)
	}
	if !strings.Contains(out, "3 skipped") {
		t.Errorf("expected '3 skipped', got:\n%s", out)
	}
}

func TestVerifyGov_invalidID(t *testing.T) {
	root := scaffoldGovProject(t, "ADR-004", `schema_version: 1
topic: demo
path: docs/architecture/decisions/ADR-004.md
signals: []
directives: []
`)
	out, code := runVerifySub(t, root, "gov", "not a valid id")
	if code != 3 {
		t.Fatalf("exit: got %d, want 3 (invalid args)\n%s", code, out)
	}
}

func TestVerifyGov_missingSidecar(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".edikt", "config.yaml"),
		[]byte("paths:\n  decisions: docs/architecture/decisions\n"),
		0o644,
	); err != nil {
		t.Fatalf("write config: %v", err)
	}
	out, code := runVerifySub(t, root, "gov", "ADR-999")
	if code != 2 {
		t.Fatalf("exit: got %d, want 2 (missing sidecar)\n%s", code, out)
	}
}

// ── prd ──────────────────────────────────────────────────────────────────

func TestVerifyPRD_passAndSkip(t *testing.T) {
	root := scaffoldPRDProject(t, "PRD-001", `schema_version: "1.0"
type: prd
id: PRD-001
title: Demo
status: in-progress
rigor: solo
author: test
created_at: "2026-01-01T00:00:00Z"
requirements:
  - id: FR-001
    text: do thing
    status: shipped
    verify: "exit 0"
  - id: FR-002
    text: undecided
    status: accepted
acceptance_criteria:
  - id: AC-001-1
    fr: FR-001
    given: g
    when: w
    then: t
    status: shipped
    verify: "exit 0"
`)
	out, code := runVerifySub(t, root, "prd", "PRD-001")
	if code != 0 {
		t.Fatalf("exit: got %d, want 0\n%s", code, out)
	}
	if !strings.Contains(out, "2 passed") || !strings.Contains(out, "1 skipped") {
		t.Errorf("expected '2 passed' and '1 skipped' in summary, got:\n%s", out)
	}
}

func TestVerifyPRD_failOnFR(t *testing.T) {
	root := scaffoldPRDProject(t, "PRD-002", `schema_version: "1.0"
type: prd
id: PRD-002
title: Demo
status: in-progress
rigor: solo
author: test
created_at: "2026-01-01T00:00:00Z"
requirements:
  - id: FR-001
    text: bad
    status: accepted
    verify: "exit 7"
`)
	out, code := runVerifySub(t, root, "prd", "PRD-002")
	if code != 1 {
		t.Fatalf("exit: got %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "1 failed") {
		t.Errorf("expected '1 failed', got:\n%s", out)
	}
}

func TestVerifyPRD_invalidID(t *testing.T) {
	root := scaffoldPRDProject(t, "PRD-003", `schema_version: "1.0"
type: prd
id: PRD-003
title: Demo
status: in-progress
rigor: solo
author: test
created_at: "2026-01-01T00:00:00Z"
`)
	_, code := runVerifySub(t, root, "prd", "not-a-prd")
	if code != 3 {
		t.Fatalf("exit: got %d, want 3", code)
	}
}

// ── spec ─────────────────────────────────────────────────────────────────

func TestVerifySPEC_passOnSRsAndACs(t *testing.T) {
	root := scaffoldSPECProject(t, "SPEC-001", `schema_version: "1.0"
type: spec
id: SPEC-001
title: Demo
status: in-progress
author: test
created_at: "2026-01-01T00:00:00Z"
source_prompt: "demo"
requirements:
  - id: SR-001
    text: do thing
    verify: "exit 0"
acceptance_criteria:
  - id: SAC-001
    source: spec
    given: g
    when: w
    then: t
    verify: "exit 0"
`)
	out, code := runVerifySub(t, root, "spec", "SPEC-001")
	if code != 0 {
		t.Fatalf("exit: got %d, want 0\n%s", code, out)
	}
	if !strings.Contains(out, "2 passed") {
		t.Errorf("expected '2 passed', got:\n%s", out)
	}
}

func TestVerifySPEC_failOnSR(t *testing.T) {
	root := scaffoldSPECProject(t, "SPEC-002", `schema_version: "1.0"
type: spec
id: SPEC-002
title: Demo
status: in-progress
author: test
created_at: "2026-01-01T00:00:00Z"
source_prompt: "demo"
requirements:
  - id: SR-001
    text: bad
    verify: "exit 9"
`)
	out, code := runVerifySub(t, root, "spec", "SPEC-002")
	if code != 1 {
		t.Fatalf("exit: got %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "1 failed") {
		t.Errorf("expected '1 failed', got:\n%s", out)
	}
}

func TestVerifySPEC_allowFailures(t *testing.T) {
	root := scaffoldSPECProject(t, "SPEC-003", `schema_version: "1.0"
type: spec
id: SPEC-003
title: Demo
status: in-progress
author: test
created_at: "2026-01-01T00:00:00Z"
source_prompt: "demo"
requirements:
  - id: SR-001
    text: bad
    verify: "exit 1"
`)
	out, code := runVerifySub(t, root, "spec", "SPEC-003", "--allow-failures")
	if code != 0 {
		t.Fatalf("exit: got %d, want 0 (allow-failures suppresses exit 1)\n%s", code, out)
	}
}
