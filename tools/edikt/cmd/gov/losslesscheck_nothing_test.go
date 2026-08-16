package gov

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runLosslessIn runs `gov lossless-check` with the working directory set
// to dir. The command resolves its project root from os.Getwd(), so cwd is
// the input — runGovCmd inherits the test's cwd and would run against the
// edikt repo itself.
func runLosslessIn(t *testing.T, dir string) (string, error) {
	t.Helper()
	c := exec.Command(buildBinary(t), "gov", "lossless-check")
	c.Dir = dir
	c.Env = append(os.Environ(), "EDIKT_SKIP_VERSION_GATE=1", "EDIKT_VERIFY_TRUST=1")
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	// Run BEFORE reading the buffer. `return buf.String(), c.Run()` would
	// evaluate the operands left to right and hand back an empty string.
	err := c.Run()
	return buf.String(), err
}

// stageLosslessProject builds the minimum a real lossless-check run needs:
// a config, the three artifact dirs, and the v0.4.3 baseline dir whose
// absence is already exit 2. Sidecars are added by the caller.
func stageLosslessProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{
		"docs/architecture/decisions",
		"docs/architecture/invariants",
		"docs/guidelines",
		"test/fixtures/sidecar-baseline-v043",
		".edikt/state",
		".edikt",
	} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	cfg := "edikt_version: 0.6.0\npaths:\n" +
		"  decisions: docs/architecture/decisions\n" +
		"  invariants: docs/architecture/invariants\n" +
		"  guidelines: docs/guidelines\n"
	if err := os.WriteFile(filepath.Join(root, ".edikt/config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return root
}

// TestLosslessCheck_NoArtifactsIsNotClean pins the vacuous pass.
//
// The verdict was `if Summary.Failed > 0 { exit 1 }` and nothing else, so
// a project where zero artifacts were ever compared exited 0 — the same
// code as a project where every artifact was compared and every one held.
// The documented contract reads "0 — clean (all artifacts pass)", which is
// vacuously true over an empty set and is exactly how the reassurance gets
// manufactured: a CI job running this gate on a corpus it could not find
// concludes the migration was lossless.
//
// Nothing here is stubbed. A project with the baseline dir present and no
// sidecars yet is the ordinary state between `edikt init` and the first
// compile.
func TestLosslessCheck_NoArtifactsIsNotClean(t *testing.T) {
	root := stageLosslessProject(t)

	out, err := runLosslessIn(t, root)

	if err == nil {
		t.Fatalf("exited 0 having compared nothing — indistinguishable from a "+
			"corpus that was compared and held:\n%s", out)
	}
	if !isExitCode(err, 4) {
		t.Errorf("expected the nothing-compared exit code 4, got %v:\n%s", err, out)
	}
	if !strings.Contains(out, "UNMEASURED") {
		t.Errorf("output does not name the result as unmeasured:\n%s", out)
	}
}

// TestLosslessCheck_AllSkippedIsNotClean is the same defect with subjects
// present. Artifacts exist, every one is skipped — superseded, sidecar
// missing, or no v0.4.3 baseline — so passed=0, failed=0, and the check
// exited 0 while having verified nothing.
//
// This is the shape the audit actually observed: skipped=8, total_losses=0,
// exit 0.
func TestLosslessCheck_AllSkippedIsNotClean(t *testing.T) {
	root := stageLosslessProject(t)

	// A sidecar with no v0.4.3 baseline — the "created post-migration"
	// skip path. Discover needs the parent .md to pair against.
	adrDir := filepath.Join(root, "docs/architecture/decisions")
	md := "---\nstatus: accepted\n---\n\n# ADR-001 — Test\n\n## Decision\n\nTest rule.\n"
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-test.md"), []byte(md), 0o644); err != nil {
		t.Fatalf("write adr: %v", err)
	}
	sc := "schema_version: 1\ntopic: \"testing\"\n" +
		"path: \"docs/architecture/decisions/ADR-001-test.md\"\n" +
		"signals:\n  - \"sig\"\n" +
		"directives:\n  - text: \"A rule MUST hold. (ref: ADR-001)\"\n" +
		"    source_excerpt:\n      line_start: 9\n      line_end: 9\n      quote: \"Test rule.\"\n"
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-test.edikt.yaml"), []byte(sc), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	out, err := runLosslessIn(t, root)

	if err == nil {
		t.Fatalf("exited 0 with every artifact skipped and none compared:\n%s", out)
	}
	if !isExitCode(err, 4) {
		t.Errorf("expected exit 4, got %v:\n%s", err, out)
	}
}
