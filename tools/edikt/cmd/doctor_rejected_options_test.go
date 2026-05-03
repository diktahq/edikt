package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// adrWithOptions returns a minimal ADR markdown with N lettered option headings
// under ## Considered Options. optStyle can be "letter" (### A.) or "word"
// (### Option A).
func adrWithOptions(n int, optStyle string) string {
	var sb strings.Builder
	sb.WriteString("---\ntype: adr\nid: ADR-099-test\ntitle: Test\nstatus: accepted\ncreated_at: 2026-05-03T00:00:00Z\n---\n\n")
	sb.WriteString("# ADR-099-test: Test\n\n**Status:** Accepted\n\n## Context\n\nSome context.\n\n")
	sb.WriteString("## Considered Options\n\n")
	letters := []string{"A", "B", "C", "D", "E"}
	for i := 0; i < n && i < len(letters); i++ {
		l := letters[i]
		if optStyle == "word" {
			sb.WriteString("### Option " + l + " Some option\n\nDescription.\n\n")
		} else {
			sb.WriteString("### " + l + ". Some option\n\nDescription.\n\n")
		}
	}
	sb.WriteString("## Decision\n\nWe chose option A.\n")
	return sb.String()
}

// sidecarWithCoverage builds a sidecar yaml. prohibCount = number of
// prohibition entries; manualMustNot = number of manual_directives with
// MUST NOT; manualOther = manual_directives without MUST NOT.
func sidecarWithCoverage(relMdPath string, prohibCount, manualMustNot, manualOther int) string {
	var sb strings.Builder
	sb.WriteString("schema_version: 1\ntopic: test\npath: " + relMdPath + "\nsignals:\n  - test\ndirectives:\n")
	sb.WriteString("  - text: \"Test directive. (ref: ADR-099-test)\"\n    source_excerpt:\n      line_start: 1\n      line_end: 1\n      quote: \"Test directive.\"\n")
	if prohibCount > 0 {
		sb.WriteString("prohibitions:\n")
		for i := 0; i < prohibCount; i++ {
			sb.WriteString("  - text: \"MUST NOT use option B — superseded by ADR-099-test.\"\n    source_excerpt:\n      line_start: 1\n      line_end: 1\n      quote: \"Some option.\"\n    derived_from: \"rejected_option_b\"\n")
		}
	}
	if manualMustNot > 0 || manualOther > 0 {
		sb.WriteString("manual_directives:\n")
		for i := 0; i < manualMustNot; i++ {
			sb.WriteString("  - \"MUST NOT use the in-memory store — superseded by ADR-099-test.\"\n")
		}
		for i := 0; i < manualOther; i++ {
			sb.WriteString("  - \"Prefer option A over alternatives.\"\n")
		}
	}
	return sb.String()
}

// scaffoldRejectedOptionsProject creates a temp project with the decisions dir.
func scaffoldRejectedOptionsProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "docs/architecture/decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return root
}

func writeRejectedOptionsFixture(t *testing.T, root, mdContent, sidecarContent string) (mdPath, sidecarPath string) {
	t.Helper()
	dir := filepath.Join(root, "docs/architecture/decisions")
	mdPath = filepath.Join(dir, "ADR-099-test.md")
	sidecarPath = filepath.Join(dir, "ADR-099-test.edikt.yaml")
	relMd := "docs/architecture/decisions/ADR-099-test.md"
	if err := os.WriteFile(mdPath, []byte(mdContent), 0o644); err != nil {
		t.Fatalf("write md: %v", err)
	}
	if sidecarContent != "" {
		content := strings.ReplaceAll(sidecarContent, "RELPATH", relMd)
		if err := os.WriteFile(sidecarPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write sidecar: %v", err)
		}
	}
	return mdPath, sidecarPath
}

func TestRejectedOptions_FlagsBareADR(t *testing.T) {
	root := scaffoldRejectedOptionsProject(t)
	relMd := "docs/architecture/decisions/ADR-099-test.md"
	writeRejectedOptionsFixture(t, root,
		adrWithOptions(3, "letter"),
		sidecarWithCoverage("RELPATH", 0, 0, 0),
	)
	_ = relMd

	var buf bytes.Buffer
	warns, ran := runRejectedOptionsCheck(root, &buf)
	if !ran {
		t.Fatal("expected ran=true")
	}
	if warns == 0 {
		t.Fatalf("expected ≥1 warn for ADR with 3 options + no coverage; got:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "WARN") {
		t.Fatalf("expected WARN in output; got:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "bin/edikt sidecar add-manual-directive") {
		t.Fatalf("expected remediation hint in output; got:\n%s", buf.String())
	}
}

func TestRejectedOptions_PassesWithProhibitions(t *testing.T) {
	root := scaffoldRejectedOptionsProject(t)
	writeRejectedOptionsFixture(t, root,
		adrWithOptions(3, "letter"),
		sidecarWithCoverage("RELPATH", 1, 0, 0),
	)

	var buf bytes.Buffer
	warns, _ := runRejectedOptionsCheck(root, &buf)
	if warns != 0 {
		t.Fatalf("expected 0 warns for ADR with prohibitions; got %d:\n%s", warns, buf.String())
	}
}

func TestRejectedOptions_PassesWithManualMustNot(t *testing.T) {
	root := scaffoldRejectedOptionsProject(t)
	writeRejectedOptionsFixture(t, root,
		adrWithOptions(2, "letter"),
		sidecarWithCoverage("RELPATH", 0, 1, 0),
	)

	var buf bytes.Buffer
	warns, _ := runRejectedOptionsCheck(root, &buf)
	if warns != 0 {
		t.Fatalf("expected 0 warns for ADR with MUST NOT manual directive; got %d:\n%s", warns, buf.String())
	}
}

func TestRejectedOptions_PassesWithMixedCoverage(t *testing.T) {
	root := scaffoldRejectedOptionsProject(t)
	writeRejectedOptionsFixture(t, root,
		adrWithOptions(3, "letter"),
		sidecarWithCoverage("RELPATH", 1, 1, 0),
	)

	var buf bytes.Buffer
	warns, _ := runRejectedOptionsCheck(root, &buf)
	if warns != 0 {
		t.Fatalf("expected 0 warns for mixed coverage; got %d:\n%s", warns, buf.String())
	}
}

func TestRejectedOptions_SkipsADRsWithFewerThan2Options(t *testing.T) {
	root := scaffoldRejectedOptionsProject(t)
	// 1 option — below threshold
	writeRejectedOptionsFixture(t, root,
		adrWithOptions(1, "letter"),
		sidecarWithCoverage("RELPATH", 0, 0, 0),
	)

	var buf bytes.Buffer
	warns, _ := runRejectedOptionsCheck(root, &buf)
	if warns != 0 {
		t.Fatalf("expected 0 warns for ADR with only 1 option; got %d:\n%s", warns, buf.String())
	}

	// Also verify: 0 options (no ## Considered Options section at all).
	root2 := scaffoldRejectedOptionsProject(t)
	noOptions := "---\ntype: adr\nid: ADR-099-test\ntitle: Test\nstatus: accepted\ncreated_at: 2026-05-03T00:00:00Z\n---\n\n# ADR-099-test\n\n## Decision\n\nWe decided X.\n"
	relMd := "docs/architecture/decisions/ADR-099-test.md"
	if err := os.WriteFile(filepath.Join(root2, relMd), []byte(noOptions), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// No sidecar — should still be silent (below threshold, no sidecar load needed).
	var buf2 bytes.Buffer
	warns2, _ := runRejectedOptionsCheck(root2, &buf2)
	if warns2 != 0 {
		t.Fatalf("expected 0 warns for ADR with no considered options; got %d:\n%s", warns2, buf2.String())
	}
}

func TestRejectedOptions_RemediationMessageDoesNotMentionADREdit(t *testing.T) {
	root := scaffoldRejectedOptionsProject(t)
	writeRejectedOptionsFixture(t, root,
		adrWithOptions(3, "letter"),
		sidecarWithCoverage("RELPATH", 0, 0, 0),
	)

	var buf bytes.Buffer
	warns, _ := runRejectedOptionsCheck(root, &buf)
	if warns == 0 {
		t.Fatal("expected at least 1 warn (test precondition)")
	}

	msg := buf.String()
	forbidden := []string{
		"edit the ADR",
		"modify the ADR",
		"update the .md",
		"rewrite the decision",
	}
	for _, f := range forbidden {
		if strings.Contains(msg, f) {
			t.Errorf("remediation message contains forbidden substring %q (INV-002 violation):\n%s", f, msg)
		}
	}
}

// TestRejectedOptions_FreeFormHeadings covers the dogfood-corpus shape where
// options use free-form titles (e.g. `### Per-concern mechanisms (chosen)`)
// rather than the lettered `### A.` / `### Option A` styles. countConsidered
// Options must still detect both options and trigger the warn when no
// prohibition coverage exists.
func TestRejectedOptions_FreeFormHeadings(t *testing.T) {
	root := scaffoldRejectedOptionsProject(t)
	relMd := "docs/architecture/decisions/ADR-099-test.md"
	mdContent := "---\ntype: adr\nid: ADR-099-test\ntitle: Test\nstatus: accepted\ncreated_at: 2026-05-03T00:00:00Z\n---\n\n" +
		"# ADR-099-test: Test\n\n## Context\n\nSome context.\n\n" +
		"## Considered Options\n\n" +
		"### Unified override model\n\nDescription A.\n\n" +
		"### Per-concern mechanisms (chosen)\n\nDescription B.\n\n" +
		"## Decision\n\nWe chose per-concern mechanisms.\n"
	writeRejectedOptionsFixture(t, root, mdContent, sidecarWithCoverage(relMd, 0, 0, 0))

	var buf bytes.Buffer
	warns, ran := runRejectedOptionsCheck(root, &buf)
	if !ran {
		t.Fatal("expected ran=true")
	}
	if warns != 1 {
		t.Fatalf("expected 1 warn for 2 free-form options + no coverage; got %d:\n%s", warns, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "Per-concern mechanisms") {
		t.Errorf("expected remediation hint to reference rejected option title; got:\n%s", out)
	}
}

func TestRejectedOptions_SkipsWhenSidecarMissing(t *testing.T) {
	root := scaffoldRejectedOptionsProject(t)
	// Write the ADR with 3 options but NO sidecar.
	mdContent := adrWithOptions(3, "letter")
	mdPath := filepath.Join(root, "docs/architecture/decisions/ADR-099-test.md")
	if err := os.WriteFile(mdPath, []byte(mdContent), 0o644); err != nil {
		t.Fatalf("write md: %v", err)
	}

	var buf bytes.Buffer
	warns, ran := runRejectedOptionsCheck(root, &buf)
	if !ran {
		t.Fatal("expected ran=true when decisions dir exists with .md files")
	}
	// No sidecar → INFO emitted, not a WARN counted toward doctor exit code.
	if warns != 0 {
		t.Fatalf("expected 0 warns when sidecar is absent (defers to MISSING check); got %d:\n%s", warns, buf.String())
	}
	if !strings.Contains(buf.String(), "INFO") {
		t.Fatalf("expected INFO line for missing sidecar; got:\n%s", buf.String())
	}
}
