package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture helpers ─────────────────────────────────────────────────────────

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// validSidecar returns a known-good sidecar yaml whose `path` field points
// at relPath (relative to project root).
func validSidecar(relMdPath string) string {
	return `schema_version: 1
topic: hooks
path: ` + relMdPath + `
signals:
  - hook
directives:
  - text: "Hooks must emit JSON. (ref: INV-003)"
    source_excerpt:
      line_start: 1
      line_end: 1
      quote: "Hooks must emit JSON."
`
}

func emptyDirectivesSidecar(relMdPath string) string {
	return `schema_version: 1
topic: prose
path: ` + relMdPath + `
signals: []
directives: []
`
}

func invalidSchemaSidecar(relMdPath string) string {
	// Missing required `topic` field.
	return `schema_version: 1
path: ` + relMdPath + `
signals: []
directives: []
`
}

func validMd(t *testing.T, root, rel string) string {
	t.Helper()
	full := filepath.Join(root, rel)
	writeFile(t, full, "# "+filepath.Base(rel)+"\n\nHooks must emit JSON.\n")
	return full
}

// scaffoldProject lays out the minimum project tree the sidecar checks
// expect: docs/architecture/{decisions,invariants}/, docs/guidelines/.
func scaffoldProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{
		"docs/architecture/decisions",
		"docs/architecture/invariants",
		"docs/guidelines",
	} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	return root
}

// tests ───────────────────────────────────────────────────────────────────

func TestSidecarChecks_clean(t *testing.T) {
	root := scaffoldProject(t)
	mdRel := "docs/architecture/decisions/ADR-100-x.md"
	validMd(t, root, mdRel)
	writeFile(t,
		filepath.Join(root, "docs/architecture/decisions/ADR-100-x.edikt.yaml"),
		validSidecar(mdRel),
	)

	var buf bytes.Buffer
	errs, warns, ran := runSidecarChecks(root, &buf)
	if !ran {
		t.Fatal("expected sidecar checks to run on scaffolded project")
	}
	if errs != 0 || warns != 0 {
		t.Fatalf("clean fixture: expected 0/0, got errs=%d warns=%d\n%s", errs, warns, buf.String())
	}
	if !strings.Contains(buf.String(), "All sidecar checks passed.") {
		t.Fatalf("expected pass banner; got:\n%s", buf.String())
	}
}

func TestSidecarChecks_orphan(t *testing.T) {
	root := scaffoldProject(t)
	// Sidecar with no sibling .md.
	writeFile(t,
		filepath.Join(root, "docs/architecture/decisions/ADR-200-orphan.edikt.yaml"),
		validSidecar("docs/architecture/decisions/ADR-200-orphan.md"),
	)

	var buf bytes.Buffer
	errs, _, ran := runSidecarChecks(root, &buf)
	if !ran {
		t.Fatal("expected ran=true")
	}
	if errs == 0 {
		t.Fatalf("orphan fixture should emit at least 1 error; got:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "ORPHAN") {
		t.Fatalf("expected ORPHAN diagnostic; got:\n%s", buf.String())
	}
}

func TestSidecarChecks_missing(t *testing.T) {
	root := scaffoldProject(t)
	mdRel := "docs/architecture/decisions/ADR-300-missing.md"
	validMd(t, root, mdRel)
	// No sidecar.

	var buf bytes.Buffer
	errs, _, _ := runSidecarChecks(root, &buf)
	if errs == 0 {
		t.Fatalf("missing fixture should emit error; got:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "MISSING") ||
		!strings.Contains(buf.String(), "/edikt:adr:compile") {
		t.Fatalf("expected MISSING + adr:compile hint; got:\n%s", buf.String())
	}
}

func TestSidecarChecks_missing_skipsByMarker(t *testing.T) {
	// Phase 6 of PLAN-sidecar-review-fixes #16 replaced the hardcoded
	// ADR-008-/ADR-009-/SPEC- prefix list with an opt-in declaration on
	// the artifact: a `<!-- edikt:migration:skip reason="…" -->` marker
	// or `migration: skip` frontmatter. Files that opt in MUST NOT
	// trigger a MISSING error; plain files MUST still error.
	root := scaffoldProject(t)

	// Opt-in via marker comment.
	withMarker := func(rel, reason string) {
		full := filepath.Join(root, rel)
		writeFile(t, full,
			"<!-- edikt:migration:skip reason=\""+reason+"\" -->\n\n"+
				"# "+filepath.Base(rel)+"\n\nHooks must emit JSON.\n")
	}
	withMarker("docs/architecture/decisions/ADR-008-foo.md",
		"documents the legacy three-list directive schema")
	withMarker("docs/architecture/decisions/ADR-009-bar.md",
		"documents the v0.4.3 invariant-record convention")

	// Opt-in via YAML frontmatter (documents_legacy_format: true).
	writeFile(t,
		filepath.Join(root, "docs/architecture/decisions/SPEC-001-baz.md"),
		"---\ndocuments_legacy_format: true\n---\n\n# SPEC-001-baz.md\n\nHooks must emit JSON.\n",
	)

	var buf bytes.Buffer
	errs, _, _ := runSidecarChecks(root, &buf)
	if errs != 0 {
		t.Fatalf("opt-in skip files should not error; got %d errs:\n%s", errs, buf.String())
	}

	// Sanity: a plain ADR-008-named file with no marker MUST still
	// error MISSING under the new opt-in regime — the prefix-based
	// auto-skip is gone.
	plainRoot := scaffoldProject(t)
	validMd(t, plainRoot, "docs/architecture/decisions/ADR-008-plain.md")
	var buf2 bytes.Buffer
	errs2, _, _ := runSidecarChecks(plainRoot, &buf2)
	if errs2 == 0 {
		t.Fatalf("plain ADR-008-named file without marker should still error MISSING; got:\n%s", buf2.String())
	}
}

func TestSidecarChecks_pathMismatch(t *testing.T) {
	root := scaffoldProject(t)
	mdRel := "docs/architecture/decisions/ADR-400-foo.md"
	validMd(t, root, mdRel)
	// Sidecar is co-located with the .md but its `path` field points at a
	// different file — this is the path-mismatch failure mode.
	writeFile(t,
		filepath.Join(root, "docs/architecture/decisions/ADR-400-foo.edikt.yaml"),
		validSidecar("docs/architecture/decisions/SOMETHING-ELSE.md"),
	)

	var buf bytes.Buffer
	errs, _, _ := runSidecarChecks(root, &buf)
	if errs == 0 {
		t.Fatalf("path-mismatch fixture should emit error; got:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "PATH MISMATCH") {
		t.Fatalf("expected PATH MISMATCH diagnostic; got:\n%s", buf.String())
	}
}

func TestSidecarChecks_schemaInvalid(t *testing.T) {
	root := scaffoldProject(t)
	mdRel := "docs/architecture/decisions/ADR-500-foo.md"
	validMd(t, root, mdRel)
	writeFile(t,
		filepath.Join(root, "docs/architecture/decisions/ADR-500-foo.edikt.yaml"),
		invalidSchemaSidecar(mdRel),
	)

	var buf bytes.Buffer
	errs, _, _ := runSidecarChecks(root, &buf)
	if errs == 0 {
		t.Fatalf("schema-invalid fixture should emit error; got:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "SCHEMA INVALID") {
		t.Fatalf("expected SCHEMA INVALID diagnostic; got:\n%s", buf.String())
	}
}

func TestSidecarChecks_emptyDirectives_isSoftWarning(t *testing.T) {
	root := scaffoldProject(t)
	mdRel := "docs/architecture/decisions/ADR-600-empty.md"
	validMd(t, root, mdRel)
	writeFile(t,
		filepath.Join(root, "docs/architecture/decisions/ADR-600-empty.edikt.yaml"),
		emptyDirectivesSidecar(mdRel),
	)

	var buf bytes.Buffer
	errs, warns, _ := runSidecarChecks(root, &buf)
	if errs != 0 {
		t.Fatalf("empty directives must NOT error; got %d errs:\n%s", errs, buf.String())
	}
	if warns == 0 {
		t.Fatalf("empty directives should warn; got 0 warns:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "NEEDS REVIEW") {
		t.Fatalf("expected NEEDS REVIEW soft warning; got:\n%s", buf.String())
	}
}

func TestSidecarChecks_skipsWhenNotProject(t *testing.T) {
	// Directory with no docs/ tree — checks should be silent.
	root := t.TempDir()
	var buf bytes.Buffer
	errs, warns, ran := runSidecarChecks(root, &buf)
	if ran {
		t.Fatal("expected ran=false outside an edikt project")
	}
	if errs != 0 || warns != 0 {
		t.Fatalf("expected 0/0 outside a project; got %d/%d", errs, warns)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected silent output; got:\n%s", buf.String())
	}
}
