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
	// Smoke: `edikt gov compile --check <repo>` must exit 0 (warnings OK) on
	// the dogfood corpus. The two-phase sidecar path is the only path —
	// ADR-082 (c) deleted the legacy in-body compile and its --legacy escape.
	repoRoot := "../../../.." // tools/edikt/cmd/gov/ → repo root
	buf, err := runGovCmd(t, "gov", "compile", "--check", repoRoot)
	if err != nil {
		if isExitCode(err, 1) {
			t.Fatalf("gov compile --check returned errors:\n%s", buf)
		}
		t.Fatalf("gov compile --check: %v\n%s", err, buf)
	}
}

// stagePreMigrationProject writes one accepted ADR carrying a legacy
// in-body sentinel block and no sidecar — the genuine pre-migration shape.
// The sentinel is what makes it pre-migration: without one there is
// nothing for `migrate sidecars` to lift, and the project is
// never-initialised instead (see stageNeverInitialisedProject).
func stagePreMigrationProject(t *testing.T) string {
	t.Helper()
	work := t.TempDir()
	adrDir := filepath.Join(work, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `# ADR-001 — Test

## Status

Accepted

## Decision

Test rule.

[edikt:directives:start]: #
topic: test
signals:
  - test signal
directives:
  - Test rule MUST hold. (ref: ADR-001)
[edikt:directives:end]: #
`
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-test.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write adr: %v", err)
	}
	return work
}

// stageNeverInitialisedProject writes one accepted ADR with no sidecar AND
// no legacy sentinel — an artifact that predates edikt entirely.
func stageNeverInitialisedProject(t *testing.T) string {
	t.Helper()
	work := t.TempDir()
	adrDir := filepath.Join(work, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "# ADR-001 — Test\n\n## Status\n\nAccepted\n\n## Decision\n\nTest rule.\n"
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-test.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write adr: %v", err)
	}
	return work
}

// TestCompile_PreMigration_FailsHard pins ADR-027 §5: a project with
// governance .md carrying legacy in-body sentinels but no .edikt.yaml
// sidecars must be rejected with the canonical actionable error and a
// non-zero exit. The auto-fallback to legacy compile (review finding #10)
// is forbidden.
func TestCompile_PreMigration_FailsHard(t *testing.T) {
	work := stagePreMigrationProject(t)

	buf, err := runGovCmd(t, "gov", "compile", work)
	if !isExitCode(err, 1) {
		t.Fatalf("expected exit 1; got err=%v\noutput:\n%s", err, buf)
	}
	if !strings.Contains(buf, "pre-migration project state") {
		t.Fatalf("expected canonical pre-migration error string in stderr; got:\n%s", buf)
	}
	if !strings.Contains(buf, "/edikt:upgrade") {
		t.Fatalf("expected slash-command-primary recovery '/edikt:upgrade' in stderr; got:\n%s", buf)
	}
	if !strings.Contains(buf, "github.com/diktahq/edikt/blob/v0.6.0/") {
		t.Fatalf("expected tag-pinned migration-guide URL (INV-008); got:\n%s", buf)
	}
}

// TestCompile_NeverInitialised_DoesNotDemandMigration is the regression
// test for the adoption deadlock: a project whose ADRs predate edikt has
// governance .md and no sidecars, but no legacy sentinels either. Routing
// it to `migrate sidecars` is a dead end — migrate finds nothing to lift
// and exits 0, so the user is sent in a circle.
//
// compile must NOT emit the pre-migration error for this shape, and must
// NOT name migrate/upgrade as the remedy.
func TestCompile_NeverInitialised_DoesNotDemandMigration(t *testing.T) {
	work := stageNeverInitialisedProject(t)

	// --check is asserted rather than a bare run: --check never dispatches
	// a subagent (ADR-028), so the assertion holds without a claude CLI.
	buf, _ := runGovCmd(t, "gov", "compile", "--check", work)

	if strings.Contains(buf, "pre-migration project state") {
		t.Fatalf("never-initialised project must not get the pre-migration error; got:\n%s", buf)
	}
	// `/edikt:upgrade` is the migration entry point. Offering it here is
	// the dead end: it runs migrate, which has nothing to lift.
	if strings.Contains(buf, "/edikt:upgrade") {
		t.Fatalf("must not route a sentinel-free project to the migration flow; got:\n%s", buf)
	}
	// migrate may be NAMED, but only to rule it out.
	if strings.Contains(buf, "migrate sidecars") && !strings.Contains(buf, "nothing for") {
		t.Fatalf("if `migrate sidecars` is mentioned it must be explicitly ruled out; got:\n%s", buf)
	}
	if !strings.Contains(buf, "never been compiled") {
		t.Fatalf("expected the never-initialised diagnosis; got:\n%s", buf)
	}
	// The remedy must name the extraction path that actually works here.
	if !strings.Contains(buf, "/edikt:adr:compile") {
		t.Fatalf("expected per-artifact extraction as the remedy; got:\n%s", buf)
	}
}

// The interactive counterpart — that a never-initialised project actually
// reaches Phase A dispatch and compiles through to governance.md — is
// pinned in internal/govrun with an injected runner, so it exercises the
// full bootstrap→merge flow without spawning a real claude subprocess.
// See TestRunTwoPhase_BootstrapsMissingSidecars.

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
	work := stagePreMigrationProject(t)
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

func TestCompile_RefusesStaleCorpus(t *testing.T) {
	stage := func(t *testing.T, status, blockShape string) string {
		t.Helper()
		work := t.TempDir()
		adrDir := filepath.Join(work, "docs", "architecture", "decisions")
		if err := os.MkdirAll(filepath.Join(work, ".edikt"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(adrDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(work, ".edikt", "config.yaml"), []byte("edikt_version: 0.8.0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		// One migrated artifact, so the corpus is not "pre-migration".
		if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-migrated.md"), []byte("---\ntype: adr\nid: ADR-001\ntitle: Migrated\nstatus: accepted\n---\n# ADR-001: Migrated\n\n## Decision\n\nThe service MUST log every request. (rule)\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-migrated.edikt.yaml"), []byte("schema_version: 2\ntopic: logging\npath: docs/architecture/decisions/ADR-001-migrated.md\nsignals: [logging]\ndirectives:\n  - text: The service MUST log every request. (ref: ADR-001)\n    source_excerpts:\n      - line_start: 10\n        line_end: 10\n        quote: The service MUST log every request. (rule)\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		block := "[edikt:directives:start]: #\nsource_hash: pending\ndirectives:\n  - A legacy rule.\n[edikt:directives:end]: #\n"
		if blockShape == "fenced" {
			block = "```\n" + block + "```\n"
		}
		body := "---\ntype: adr\nid: ADR-002\ntitle: Legacy\nstatus: " + status + "\n---\n# ADR-002: Legacy\n\n**Status:** " + map[string]string{"accepted": "Accepted", "superseded-by-ADR-001": "Superseded by ADR-001"}[status] + "\n\n## Decision\n\nProse.\n\n## Directives\n\n" + block
		if err := os.WriteFile(filepath.Join(adrDir, "ADR-002-legacy.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return work
	}

	t.Run("included artifact with an unfenced block refuses", func(t *testing.T) {
		work := stage(t, "accepted", "unfenced")
		buf, err := runGovCmd(t, "gov", "compile", "--skip-verify", work)
		if !isExitCode(err, 1) {
			t.Fatalf("expected exit 1; got err=%v\n%s", err, buf)
		}
		for _, want := range []string{"unstripped legacy", "ADR-002-legacy.md:17", "/edikt:migrate", "Nothing was compiled"} {
			if !strings.Contains(buf, want) {
				t.Errorf("refusal must contain %q; got:\n%s", want, buf)
			}
		}
		if _, statErr := os.Stat(filepath.Join(work, ".claude", "rules", "governance.md")); statErr == nil {
			t.Fatal("a refused compile must not write the compiled tree")
		}
	})
	t.Run("--check refuses the same way", func(t *testing.T) {
		work := stage(t, "accepted", "unfenced")
		buf, err := runGovCmd(t, "gov", "compile", "--check", work)
		if !isExitCode(err, 1) || !strings.Contains(buf, "/edikt:migrate") {
			t.Fatalf("--check must refuse naming the remedy; err=%v\n%s", err, buf)
		}
	})
	t.Run("skip-listed artifact with a block is not a refusal", func(t *testing.T) {
		work := stage(t, "superseded-by-ADR-001", "unfenced")
		buf, _ := runGovCmd(t, "gov", "compile", "--skip-verify", work)
		if strings.Contains(buf, "unstripped legacy") {
			t.Fatalf("a superseded artifact's block must not refuse the compile; got:\n%s", buf)
		}
	})
	t.Run("fenced mention is not a block", func(t *testing.T) {
		work := stage(t, "accepted", "fenced")
		buf, _ := runGovCmd(t, "gov", "compile", "--skip-verify", work)
		if strings.Contains(buf, "unstripped legacy") {
			t.Fatalf("a fenced mention must not refuse the compile; got:\n%s", buf)
		}
	})
}
