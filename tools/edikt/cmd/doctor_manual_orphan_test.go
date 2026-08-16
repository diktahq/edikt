package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDoctor_FlagsOrphanRef — a manual_directive that cites ADR-999 (a
// non-existent ADR file) emits an ORPHAN finding. References to extant ADRs
// are silently passed through. INV-006: ArtifactID is validated before the
// filesystem lookup; a Unicode-lookalike or shell-metachar ID is rejected
// without surfacing a finding (the validator path treats it as "skip").
func TestDoctor_FlagsOrphanRef(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs/architecture/decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Real ADR-001 file.
	if err := os.WriteFile(filepath.Join(dir, "ADR-001-real.md"),
		[]byte("# ADR-001 real\n\n**Status:** Accepted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Sidecar for ADR-001 with one orphan ref (ADR-999) and one valid ref
	// (ADR-001).
	sidecarBody := `schema_version: 1
topic: test
path: docs/architecture/decisions/ADR-001-real.md
signals:
  - x
directives:
  - text: "Real directive. (ref: ADR-001)"
    source_excerpt:
      line_start: 1
      line_end: 1
      quote: "Real directive."
manual_directives:
  - "MUST NOT do X. (ref: ADR-999)"
  - "Stay aligned. (ref: ADR-001)"
`
	if err := os.WriteFile(filepath.Join(dir, "ADR-001-real.edikt.yaml"), []byte(sidecarBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	warns, ran := runOrphanManualRefCheck(root, &buf)
	if !ran {
		t.Fatal("expected check to run; got ran=false")
	}
	if warns != 1 {
		t.Errorf("want 1 orphan warn, got %d. Output:\n%s", warns, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "ORPHAN: manual directive in") {
		t.Errorf("expected ORPHAN line, got:\n%s", out)
	}
	if !strings.Contains(out, "cites ADR-999 which does not exist") {
		t.Errorf("expected ADR-999 citation, got:\n%s", out)
	}
	// The path of the sidecar in the orphan line legitimately contains
	// "ADR-001-real.edikt.yaml", but the cited ID in the warning must NOT
	// be ADR-001.
	if strings.Contains(out, "cites ADR-001") {
		t.Errorf("ADR-001 reference should not warn, got:\n%s", out)
	}
}

// TestDoctor_OrphanRef_NoSidecars — ran=false when no sidecars exist.
func TestDoctor_OrphanRef_NoSidecars(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs/architecture/decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	_, ran := runOrphanManualRefCheck(root, &buf)
	if ran {
		t.Error("expected ran=false when no sidecars exist")
	}
}
