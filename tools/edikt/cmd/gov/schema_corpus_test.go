package gov

// schema_corpus_test.go — N4,
// docs/internal/audits/TRIAGE-2026-08-20-bok-services-governance-projection.md.
//
// schema-check's only exclusion mechanism is skipping directories whose
// basename contains "fixture" — this pins that it covers project-chosen
// naming (sidecar-fixtures/, test-fixtures/), not only the exact literal
// "fixtures", and that a deliberately-invalid fixture there does not fail
// the real corpus's count.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSidecar(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const validCorpusSidecar = `schema_version: 2
topic: hooks
path: docs/architecture/decisions/ADR-100-test.md
signals:
  - hook
directives:
  - text: "Hooks must emit JSON. (ref: ADR-100)"
    source_excerpts:
      - line_start: 1
        line_end: 1
        quote: "Hooks must emit JSON."
`

const deliberatelyInvalidSidecar = `schema_version: 2
topic: hooks
# missing required 'path' field — deliberately invalid, for gate testing
signals:
  - hook
directives: []
`

func TestSchemaCheck_SkipsNamedFixturesDir(t *testing.T) {
	root := t.TempDir()
	writeSidecar(t, filepath.Join(root, "docs/architecture/decisions"), "ADR-100-test.edikt.yaml", validCorpusSidecar)
	writeSidecar(t, filepath.Join(root, "docs/architecture/decisions/fixtures"), "invalid.edikt.yaml", deliberatelyInvalidSidecar)

	out, err := runGovCmd(t, "gov", "schema-check", root)
	if err != nil {
		t.Fatalf("schema-check should pass — the invalid fixture is under fixtures/, got error: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "1 of 1 sidecars valid") {
		t.Errorf("expected exactly 1 real sidecar counted (fixture excluded), got:\n%s", out)
	}
}

// The broadened part of the fix: a project's own naming convention
// (sidecar-fixtures/, not the exact literal "fixtures") must also be
// excluded — this is what N4's report actually used.
func TestSchemaCheck_SkipsProjectNamedFixturesDir(t *testing.T) {
	root := t.TempDir()
	writeSidecar(t, filepath.Join(root, "docs/architecture/decisions"), "ADR-100-test.edikt.yaml", validCorpusSidecar)
	writeSidecar(t, filepath.Join(root, "docs/architecture/decisions/sidecar-fixtures"), "invalid.edikt.yaml", deliberatelyInvalidSidecar)
	writeSidecar(t, filepath.Join(root, "test-fixtures"), "another-invalid.edikt.yaml", deliberatelyInvalidSidecar)

	out, err := runGovCmd(t, "gov", "schema-check", root)
	if err != nil {
		t.Fatalf("schema-check should pass — both invalid fixtures are under *fixture* dirs, got error: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "1 of 1 sidecars valid") {
		t.Errorf("expected exactly 1 real sidecar counted, got:\n%s", out)
	}
}

// Control: a real corpus problem outside any fixture directory must still
// be caught — the broadened match must not become an accidental catch-all.
func TestSchemaCheck_RealInvalidSidecarOutsideFixturesStillFails(t *testing.T) {
	root := t.TempDir()
	writeSidecar(t, filepath.Join(root, "docs/architecture/decisions"), "ADR-100-test.edikt.yaml", validCorpusSidecar)
	writeSidecar(t, filepath.Join(root, "docs/architecture/decisions"), "ADR-101-broken.edikt.yaml", deliberatelyInvalidSidecar)

	out, err := runGovCmd(t, "gov", "schema-check", root)
	if err == nil {
		t.Fatalf("expected schema-check to fail on a real invalid sidecar, got clean output:\n%s", out)
	}
	if !isExitCode(err, 1) {
		t.Errorf("expected exit 1, got: %v", err)
	}
	if !strings.Contains(out, "1 of 2 sidecars valid") {
		t.Errorf("expected 1 of 2 (the fixture-exclusion must not suppress a real corpus defect), got:\n%s", out)
	}
}
