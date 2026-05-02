package gov

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompileCheckCleanProject(t *testing.T) {
	// Smoke: `edikt gov compile --check <repo>` must exit 0 (warnings OK).
	// The dogfood project is pre-migration (governance .md without sidecars),
	// so per ADR-027 §5 / Phase 1 of PLAN-sidecar-review-fixes, the
	// non-legacy path hard-fails. Until the dogfood project itself is
	// migrated (Phase 8 ship-gate), this smoke test exercises the legacy
	// in-body sentinel path via --legacy. Once Phase 8's `migrate sidecars
	// --apply` runs on the dogfood, drop --legacy and the test will
	// exercise the two-phase compile path natively.
	repoRoot := "../../../.." // tools/edikt/cmd/gov/ → repo root
	buf, err := runGovCmd(t, "gov", "compile", "--legacy", "--check", repoRoot)
	if err != nil {
		if isExitCode(err, 1) {
			t.Fatalf("gov compile --legacy --check returned errors:\n%s", buf)
		}
		t.Fatalf("gov compile --legacy --check: %v\n%s", err, buf)
	}
}

// TestCompile_PreMigration_FailsHard pins ADR-027 §5: a project with
// governance .md but no .edikt.yaml sidecars must be rejected with the
// canonical actionable error and a non-zero exit. The auto-fallback to
// legacy compile (review finding #10) is forbidden.
func TestCompile_PreMigration_FailsHard(t *testing.T) {
	work := t.TempDir()

	// Stage one accepted ADR with no sidecar — minimal pre-migration shape.
	adrDir := filepath.Join(work, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `# ADR-001 — Test

## Status

Accepted

## Decision

Test rule.
`
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-test.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write adr: %v", err)
	}

	buf, err := runGovCmd(t, "gov", "compile", work)
	if !isExitCode(err, 1) {
		t.Fatalf("expected exit 1; got err=%v\noutput:\n%s", err, buf)
	}
	if !strings.Contains(buf, "pre-migration project state") {
		t.Fatalf("expected canonical pre-migration error string in stderr; got:\n%s", buf)
	}
	if !strings.Contains(buf, "edikt migrate sidecars") {
		t.Fatalf("expected actionable directive 'edikt migrate sidecars' in stderr; got:\n%s", buf)
	}
	if !strings.Contains(buf, "github.com/diktahq/edikt/blob/v0.6.0/") {
		t.Fatalf("expected tag-pinned migration-guide URL (INV-008); got:\n%s", buf)
	}
}

// TestCompile_EmptyProject_NoOp pins the empty-project case: no
// governance .md at all → compile exits 0 with no error.
func TestCompile_EmptyProject_NoOp(t *testing.T) {
	work := t.TempDir()
	// No governance dirs at all.
	buf, err := runGovCmd(t, "gov", "compile", work)
	if err != nil {
		t.Fatalf("expected exit 0 on empty project; got err=%v\noutput:\n%s", err, buf)
	}
}

// TestCompile_DryRunFlag_AliasesCheck pins the --dry-run alias from
// finding #29: it must behave identically to --check (validate-only;
// no writes; non-zero exit on stale sidecars). Smoke-test on a clean
// pre-migration project: --dry-run must surface the canonical pre-
// migration error and exit non-zero, exactly like --check would.
func TestCompile_DryRunFlag_AliasesCheck(t *testing.T) {
	work := t.TempDir()
	adrDir := filepath.Join(work, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "# ADR-001 — Test\n\n## Status\n\nAccepted\n\n## Decision\n\nTest rule.\n"
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-test.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write adr: %v", err)
	}
	buf, err := runGovCmd(t, "gov", "compile", "--dry-run", work)
	if !isExitCode(err, 1) {
		t.Fatalf("expected exit 1; got err=%v\noutput:\n%s", err, buf)
	}
	if !strings.Contains(buf, "pre-migration project state") {
		t.Fatalf("expected canonical pre-migration error; got:\n%s", buf)
	}
}

// TestCompile_TwoPhaseJSON pins finding #32: gov compile --json in
// two-phase mode emits a single JSON object on stdout with phase_a /
// phase_b sub-objects. Run on a project that already has a sidecar so
// the two-phase path is taken; assert the JSON shape.
func TestCompile_TwoPhaseJSON(t *testing.T) {
	bin := buildBinary(t)

	work := t.TempDir()
	adrDir := filepath.Join(work, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mdBody := `# ADR-001 — Test

## Status

Accepted

## Decision

A directive sentence.
`
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-test.md"), []byte(mdBody), 0o644); err != nil {
		t.Fatalf("write adr: %v", err)
	}
	// Co-located sidecar with a directive whose source_excerpt matches the
	// .md body verbatim, so Phase A sees nothing stale and Phase B has a
	// deterministic merge to perform.
	sidecarBody := `schema_version: 1
topic: test
path: docs/architecture/decisions/ADR-001-test.md
signals:
  - test
directives:
  - text: "A directive sentence."
    source_excerpt:
      line_start: 9
      line_end: 9
      quote: "A directive sentence."
`
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-test.edikt.yaml"), []byte(sidecarBody), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	cmd := exec.Command(bin, "gov", "compile", "--json", work)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run()

	// stdout MUST be a single JSON object with phase_a and phase_b keys.
	var parsed struct {
		Status string `json:"status"`
		PhaseA struct {
			Dispatched int `json:"dispatched"`
			Stale      int `json:"stale"`
		} `json:"phase_a"`
		PhaseB *struct {
			TopicsRendered  []string `json:"topics_rendered"`
			TopicsUnchanged []string `json:"topics_unchanged"`
		} `json:"phase_b"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("--json output not parseable: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if parsed.Status == "" {
		t.Fatalf("status missing; stdout:\n%s", stdout.String())
	}
	// Stale sidecar count is observable; must be 0 because we constructed
	// a matching .md/sidecar pair.
	if parsed.PhaseA.Stale != 0 {
		t.Fatalf("phase_a.stale: want 0, got %d (sidecar should match)\nstdout:\n%s\nstderr:\n%s",
			parsed.PhaseA.Stale, stdout.String(), stderr.String())
	}
	// Phase B must have run (the sidecar set is healthy).
	if parsed.PhaseB == nil {
		t.Fatalf("phase_b absent; expected merge to run\nstdout:\n%s", stdout.String())
	}
}

// TestCompile_LegacyFlag_Allowed pins the deprecated --legacy escape
// hatch: when set, in-body parsing runs (with whatever errors it would
// have produced). The flag is preserved through v0.7.0 per ADR-027 §5
// commentary.
func TestCompile_LegacyFlag_Allowed(t *testing.T) {
	work := t.TempDir()
	adrDir := filepath.Join(work, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-test.md"), []byte("# ADR-001 — Test\n\n## Status\n\nAccepted\n"), 0o644); err != nil {
		t.Fatalf("write adr: %v", err)
	}
	// --legacy must NOT emit the pre-migration error. The legacy run may
	// still fail for other reasons (no sentinel) but the failure must be
	// distinct from the migration-required path.
	buf, _ := runGovCmd(t, "gov", "compile", "--legacy", work)
	if strings.Contains(buf, "pre-migration project state") {
		t.Fatalf("--legacy must not surface the pre-migration error; got:\n%s", buf)
	}
}
