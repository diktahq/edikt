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
      line_start: 3
      line_end: 3
      quote: "Hooks must emit JSON."
`
}

// driftedSidecar is validSidecar with the anchor pointed at the heading
// instead of the sentence it quotes — structurally valid, semantically stale.
// Every check except anchor drift passes on it, which is the whole point.
func driftedSidecar(relMdPath string) string {
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
	// Accepted, the way the real corpus declares it when frontmatter carries
	// no status: the bold Status line. A fixture with NO status is excluded
	// by the same filter compile applies, and would report nothing here.
	// The quote stays on line 3, where the anchored fixtures point.
	writeFile(t, full, "# "+filepath.Base(rel)+"\n\nHooks must emit JSON.\n\n**Status:** Accepted\n")
	return full
}

// statusMd writes an artifact whose frontmatter declares the given status.
func statusMd(t *testing.T, root, rel, status string) string {
	t.Helper()
	full := filepath.Join(root, rel)
	writeFile(t, full, "---\nstatus: "+status+"\n---\n# "+filepath.Base(rel)+"\n\nHooks must emit JSON.\n")
	return full
}

// scaffoldProject lays out the minimum project tree the sidecar checks
// expect: docs/architecture/{decisions,invariants}/, docs/guidelines/.
func scaffoldProject(t *testing.T) string {
	t.Helper()
	isolateDoctorEnv(t, true)
	root := t.TempDir()
	installValidatedExtractor(t, root)
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

// N2 (docs/internal/audits/TRIAGE-2026-08-20-bok-services-governance-projection.md):
// a proposed ADR/invariant explicitly opted out via `sidecar: skip` must
// not trigger MISSING. The control below used to assert the opposite for a
// proposed artifact with NO opt-out ("still must", because ADR-063 showed a
// proposed artifact CAN carry a sidecar). D46 (a) reversed that: can is not
// must, compile ignores a proposed artifact's sidecar either way, and doctor
// demanding one it would never use sent bok to "run /edikt:adr:compile" for
// artifacts compile --check correctly filtered. The control now asserts the
// skip is reported with its status, never silent.
func TestSidecarChecks_missing_skipsBySidecarSkipMarker(t *testing.T) {
	root := scaffoldProject(t)

	writeFile(t, filepath.Join(root, "docs/architecture/decisions/ADR-200-pending.md"),
		"---\nstatus: proposed\nsidecar: skip\nreason: \"awaiting acceptance gate\"\n---\n\n"+
			"# ADR-200-pending.md\n\nHooks must emit JSON.\n")

	var buf bytes.Buffer
	errs, _, _ := runSidecarChecks(root, &buf)
	if errs != 0 {
		t.Fatalf("explicit sidecar: skip opt-out should not error; got %d errs:\n%s", errs, buf.String())
	}

	// Control (D46 (a)): status: proposed alone, no opt-out, is not MISSING
	// — it is reported as skipped with its status.
	plainRoot := scaffoldProject(t)
	writeFile(t, filepath.Join(plainRoot, "docs/architecture/decisions/ADR-201-pending.md"),
		"---\nstatus: proposed\n---\n\n# ADR-201-pending.md\n\nHooks must emit JSON.\n")
	var buf2 bytes.Buffer
	errs2, _, _ := runSidecarChecks(plainRoot, &buf2)
	if errs2 != 0 || !strings.Contains(buf2.String(), "skipped: docs/architecture/decisions/ADR-201-pending.md") ||
		!strings.Contains(buf2.String(), "status: proposed") {
		t.Fatalf("status: proposed alone is skipped with its status, not MISSING (D46 a); got %d errs:\n%s", errs2, buf2.String())
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

func TestSidecarChecks_anchorDrift(t *testing.T) {
	root := scaffoldProject(t)
	mdRel := "docs/architecture/decisions/ADR-600-drift.md"
	validMd(t, root, mdRel)
	writeFile(t,
		filepath.Join(root, "docs/architecture/decisions/ADR-600-drift.edikt.yaml"),
		driftedSidecar(mdRel),
	)

	var buf bytes.Buffer
	errs, _, ran := runSidecarChecks(root, &buf)
	if !ran {
		t.Fatal("expected sidecar checks to run")
	}
	if errs != 1 {
		t.Fatalf("drifted anchor: expected exactly 1 error, got %d\n%s", errs, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "ANCHOR DRIFT") {
		t.Fatalf("expected ANCHOR DRIFT finding; got:\n%s", out)
	}
	// The denominator must travel with the verdict (INV-013): a fault count
	// alone does not say what it was drawn from.
	if !strings.Contains(out, "1 of 1 anchor(s)") {
		t.Fatalf("expected the anchor denominator in the message; got:\n%s", out)
	}
	// The message must name what is actually at the recorded lines — that is
	// what makes an off-by-one obvious rather than merely reported.
	if !strings.Contains(out, "# ADR-600-drift.md") {
		t.Fatalf("expected the actual line content in the message; got:\n%s", out)
	}
}

// A sidecar whose parent is missing must be reported ONCE, by the orphan
// check, not a second time as drift.
func TestSidecarChecks_anchorDrift_doesNotDoubleCountOrphan(t *testing.T) {
	root := scaffoldProject(t)
	writeFile(t,
		filepath.Join(root, "docs/architecture/decisions/ADR-601-orphan.edikt.yaml"),
		validSidecar("docs/architecture/decisions/ADR-601-orphan.md"),
	)

	var buf bytes.Buffer
	errs, _, _ := runSidecarChecks(root, &buf)
	if errs != 1 {
		t.Fatalf("orphan with no parent: expected exactly 1 error, got %d\n%s", errs, buf.String())
	}
	if strings.Contains(buf.String(), "ANCHOR DRIFT") {
		t.Fatalf("missing parent must not also be reported as drift; got:\n%s", buf.String())
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

// D46 (a): doctor demands a sidecar only from artifacts compile would govern.
// A proposed or rejected ADR without a sidecar is not MISSING; an accepted one
// still is; a rejected ADR's absence is reported as skipped with its status,
// never silently (INV-013).
func TestSidecarChecks_missing_skipsExcludedStatus(t *testing.T) {
	root := scaffoldProject(t)
	statusMd(t, root, "docs/architecture/decisions/ADR-301-rejected.md", "rejected")
	statusMd(t, root, "docs/architecture/decisions/ADR-302-proposed.md", "proposed")
	statusMd(t, root, "docs/architecture/decisions/ADR-303-accepted.md", "accepted")

	var buf bytes.Buffer
	errs, _, _ := runSidecarChecks(root, &buf)
	out := buf.String()
	if strings.Contains(out, "MISSING: docs/architecture/decisions/ADR-301-rejected.md") ||
		strings.Contains(out, "MISSING: docs/architecture/decisions/ADR-302-proposed.md") {
		t.Fatalf("a rejected/proposed ADR must not be reported MISSING:\n%s", out)
	}
	if !strings.Contains(out, "MISSING: docs/architecture/decisions/ADR-303-accepted.md") {
		t.Fatalf("an accepted ADR without a sidecar must still be MISSING:\n%s", out)
	}
	if errs != 1 {
		t.Fatalf("expected exactly one MISSING error, got %d:\n%s", errs, out)
	}
	if !strings.Contains(out, "skipped: docs/architecture/decisions/ADR-301-rejected.md") ||
		!strings.Contains(out, "status: rejected") {
		t.Fatalf("the rejected ADR's skip must be reported with its status:\n%s", out)
	}
}

// An invariant with no status is active (legacy), so it is still MISSING.
func TestSidecarChecks_missing_noStatusInvariantIsActive(t *testing.T) {
	root := scaffoldProject(t)
	full := filepath.Join(root, "docs/architecture/invariants/INV-900-legacy.md")
	writeFile(t, full, "# INV-900\n\nHooks must emit JSON.\n")
	var buf bytes.Buffer
	errs, _, _ := runSidecarChecks(root, &buf)
	if errs != 1 || !strings.Contains(buf.String(), "MISSING: docs/architecture/invariants/INV-900-legacy.md") {
		t.Fatalf("a status-less invariant is active and its missing sidecar is MISSING:\n%s", buf.String())
	}
}

// D46 (b): doctor's anchor check is status-filtered like compile. A
// superseded ADR whose sidecar anchors are deliberately broken is skipped
// with its status, not an ANCHOR DRIFT error; an accepted one with the same
// broken anchors still is.
func TestSidecarChecks_anchorDrift_statusFiltered(t *testing.T) {
	root := scaffoldProject(t)
	sup := "docs/architecture/decisions/ADR-600-superseded.md"
	writeFile(t, filepath.Join(root, sup), "---\nstatus: superseded\n---\n# ADR-600\n\nHooks must emit JSON.\n")
	writeFile(t, filepath.Join(root, "docs/architecture/decisions/ADR-600-superseded.edikt.yaml"), driftedSidecar(sup))
	acc := "docs/architecture/decisions/ADR-601-accepted.md"
	writeFile(t, filepath.Join(root, acc), "---\nstatus: accepted\n---\n# ADR-601\n\nHooks must emit JSON.\n")
	writeFile(t, filepath.Join(root, "docs/architecture/decisions/ADR-601-accepted.edikt.yaml"), driftedSidecar(acc))

	var buf bytes.Buffer
	errs, _, _ := runSidecarChecks(root, &buf)
	out := buf.String()
	if strings.Contains(out, "ANCHOR DRIFT: docs/architecture/decisions/ADR-600-superseded.edikt.yaml") {
		t.Fatalf("a superseded artifact's anchors are not a finding:\n%s", out)
	}
	if !strings.Contains(out, "skipped: docs/architecture/decisions/ADR-600-superseded.edikt.yaml anchors not checked") {
		t.Fatalf("the skip must be said with its status:\n%s", out)
	}
	if !strings.Contains(out, "ANCHOR DRIFT: docs/architecture/decisions/ADR-601-accepted.edikt.yaml") || errs != 1 {
		t.Fatalf("the accepted artifact's broken anchors are still the finding (errs=%d):\n%s", errs, out)
	}
}

// bok round 6 (build 9937d03a): doctor raised ANCHOR DRIFT, with "run
// /edikt:<kind>:compile to regenerate", on superseded artifacts that compile
// itself skips as "existing sidecar ignored; safe to delete". Compile skips by
// sidecar.IsSkipListed (a superseded guideline, an ADR whose body status line
// reads "Superseded by"), which the status filter alone does not cover:
// every guideline is "included", whatever its status. Doctor must skip what
// compile skips.
func TestSidecarChecks_anchorDrift_skipsWhatCompileSkips(t *testing.T) {
	root := scaffoldProject(t)
	gl := "docs/guidelines/GL-700-superseded.md"
	writeFile(t, filepath.Join(root, gl), "---\nstatus: superseded\n---\n# GL-700\n\nHooks must emit JSON.\n")
	writeFile(t, filepath.Join(root, "docs/guidelines/GL-700-superseded.edikt.yaml"), driftedSidecar(gl))
	adr := "docs/architecture/decisions/ADR-701-body-superseded.md"
	writeFile(t, filepath.Join(root, adr), "---\nstatus: accepted\n---\n# ADR-701\n\n**Status:** Superseded by ADR-702\n\nHooks must emit JSON.\n")
	writeFile(t, filepath.Join(root, "docs/architecture/decisions/ADR-701-body-superseded.edikt.yaml"), driftedSidecar(adr))

	var buf bytes.Buffer
	_, _, _ = runSidecarChecks(root, &buf)
	out := buf.String()
	for _, sc := range []string{"docs/guidelines/GL-700-superseded.edikt.yaml", "docs/architecture/decisions/ADR-701-body-superseded.edikt.yaml"} {
		if strings.Contains(out, "ANCHOR DRIFT: "+sc) {
			t.Errorf("compile skips %s, so its anchors are not a doctor finding:\n%s", sc, out)
		}
		if !strings.Contains(out, "skipped: "+sc+" anchors not checked") {
			t.Errorf("the skip of %s must be said, not silent:\n%s", sc, out)
		}
	}
}
