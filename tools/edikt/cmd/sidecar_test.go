package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// minimalSidecarYAML returns a valid minimal sidecar YAML for test use.
// The topic and path are passed in to allow per-test variation.
func minimalSidecarYAML(topic, path string) string {
	return "schema_version: 1\n" +
		"topic: " + topic + "\n" +
		"path: " + path + "\n" +
		"signals: []\n" +
		"directives:\n" +
		"  - text: MUST follow the rules\n" +
		"    source_excerpt:\n" +
		"      line_start: 1\n" +
		"      line_end: 1\n" +
		"      quote: MUST follow the rules\n"
}

// minimalSidecarWithManual returns a minimal sidecar YAML with one manual directive.
func minimalSidecarWithManual(topic, path, manual string) string {
	return minimalSidecarYAML(topic, path) +
		"manual_directives:\n" +
		"  - " + yamlQuote(manual) + "\n"
}

// yamlQuote wraps a string in single quotes for safe inline YAML scalar embedding.
func yamlQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// writeTempSidecar writes YAML to a temp .edikt.yaml file and returns its path.
func writeTempSidecar(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name+".edikt.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTempSidecar: %v", err)
	}
	return p
}

// TestAddManual_FreshSidecar — sidecar with no manual_directives; append succeeds.
func TestAddManual_FreshSidecar(t *testing.T) {
	dir := t.TempDir()
	sc := writeTempSidecar(t, dir, "ADR-001-test",
		minimalSidecarYAML("architecture", "docs/architecture/decisions/ADR-001-test.md"))

	out, err := runCmd(t,
		"sidecar", "add-manual-directive",
		"--path", sc,
		"--text", "MUST enable prompt caching (ref: ADR-001 + manual)",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !contains(out, "ok: appended") {
		t.Errorf("expected 'ok: appended' in output, got: %s", out)
	}

	loaded, err := sidecar.Load(sc)
	if err != nil {
		t.Fatalf("load after append: %v", err)
	}
	if len(loaded.ManualDirectives) != 1 {
		t.Fatalf("want 1 manual directive, got %d", len(loaded.ManualDirectives))
	}
	if !strings.Contains(loaded.ManualDirectives[0], "MUST enable prompt caching") {
		t.Errorf("unexpected directive text: %q", loaded.ManualDirectives[0])
	}
}

// TestAddManual_ExistingManualList — sidecar with 1 existing entry; append produces 2.
func TestAddManual_ExistingManualList(t *testing.T) {
	dir := t.TempDir()
	sc := writeTempSidecar(t, dir, "ADR-002-test",
		minimalSidecarWithManual("architecture", "docs/architecture/decisions/ADR-002-test.md",
			"MUST use canonical YAML (ref: ADR-002 + manual)"))

	out, err := runCmd(t,
		"sidecar", "add-manual-directive",
		"--path", sc,
		"--text", "MUST NOT use JSON for sidecar storage (ref: ADR-002 + manual)",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}

	loaded, err := sidecar.Load(sc)
	if err != nil {
		t.Fatalf("load after append: %v", err)
	}
	if len(loaded.ManualDirectives) != 2 {
		t.Fatalf("want 2 manual directives, got %d: %v", len(loaded.ManualDirectives), loaded.ManualDirectives)
	}
}

// TestAddManual_DedupRejection — exit 3 + exact message on duplicate text.
func TestAddManual_DedupRejection(t *testing.T) {
	dir := t.TempDir()
	existing := "MUST use canonical YAML (ref: ADR-003 + manual)"
	sc := writeTempSidecar(t, dir, "ADR-003-test",
		minimalSidecarWithManual("architecture", "docs/architecture/decisions/ADR-003-test.md", existing))

	out, err := runCmd(t,
		"sidecar", "add-manual-directive",
		"--path", sc,
		"--text", existing,
	)
	if !isExitCode(err, 3) {
		t.Fatalf("want exit 3, got: %v\noutput: %s", err, out)
	}
	if !contains(out, "Manual directive already present at index 0.") {
		t.Errorf("expected dedup message, got: %s", out)
	}
}

// TestAddManual_AutoRefTag — text without (ref: ...) gets (ref: ADR-NNN + manual) appended.
func TestAddManual_AutoRefTag(t *testing.T) {
	dir := t.TempDir()
	sc := writeTempSidecar(t, dir, "ADR-027-sidecar-architecture",
		minimalSidecarYAML("architecture", "docs/architecture/decisions/ADR-027-sidecar-architecture.md"))

	out, err := runCmd(t,
		"sidecar", "add-manual-directive",
		"--path", sc,
		"--text", "MUST co-locate sidecar with parent artifact",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}

	loaded, err := sidecar.Load(sc)
	if err != nil {
		t.Fatalf("load after append: %v", err)
	}
	if len(loaded.ManualDirectives) != 1 {
		t.Fatalf("want 1 directive, got %d", len(loaded.ManualDirectives))
	}
	got := loaded.ManualDirectives[0]
	if !strings.Contains(got, "(ref: ADR-027 + manual)") {
		t.Errorf("expected auto ref tag '(ref: ADR-027 + manual)' in %q", got)
	}
}

// TestAddManual_RejectsMissingSidecar — exit 2 when sidecar path doesn't exist.
func TestAddManual_RejectsMissingSidecar(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "ADR-099-nonexistent.edikt.yaml")

	out, err := runCmd(t,
		"sidecar", "add-manual-directive",
		"--path", missing,
		"--text", "MUST never exist (ref: ADR-099 + manual)",
	)
	if !isExitCode(err, 2) {
		t.Fatalf("want exit 2, got: %v\noutput: %s", err, out)
	}
	if !contains(out, "sidecar not found") {
		t.Errorf("expected 'sidecar not found' message, got: %s", out)
	}
}

// TestAddManual_RejectsInvalidYAML — non-zero exit when the file is malformed YAML.
func TestAddManual_RejectsInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	// Write a file that is not valid YAML at all
	p := filepath.Join(dir, "ADR-100-bad.edikt.yaml")
	if err := os.WriteFile(p, []byte("this: is: broken: yaml: :::"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t,
		"sidecar", "add-manual-directive",
		"--path", p,
		"--text", "MUST validate (ref: ADR-100 + manual)",
	)
	if err == nil {
		t.Fatalf("expected non-zero exit, got success\noutput: %s", out)
	}
}

// TestAddManual_PostAddValidatePasses — post-append Load + Validate round-trip succeeds.
func TestAddManual_PostAddValidatePasses(t *testing.T) {
	dir := t.TempDir()
	sc := writeTempSidecar(t, dir, "ADR-005-test",
		minimalSidecarYAML("architecture", "docs/architecture/decisions/ADR-005-test.md"))

	_, err := runCmd(t,
		"sidecar", "add-manual-directive",
		"--path", sc,
		"--text", "SHOULD prefer immutable data structures (ref: ADR-005 + manual)",
	)
	if err != nil {
		t.Fatalf("append failed: %v", err)
	}

	loaded, loadErr := sidecar.Load(sc)
	if loadErr != nil {
		t.Fatalf("Load after append failed: %v", loadErr)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatalf("Validate after append failed: %v", err)
	}
}

// TestAddManual_MdPathResolvesToSibling — --path foo/ADR-001.md resolves to foo/ADR-001.edikt.yaml.
func TestAddManual_MdPathResolvesToSibling(t *testing.T) {
	dir := t.TempDir()
	// Write the sidecar (not the .md — we don't need the actual ADR body)
	sidecarPath := writeTempSidecar(t, dir, "ADR-001-architecture",
		minimalSidecarYAML("architecture", "docs/architecture/decisions/ADR-001-architecture.md"))

	// Pass the .md path — expect resolution to the .edikt.yaml sibling
	mdPath := strings.TrimSuffix(sidecarPath, ".edikt.yaml") + ".md"

	out, err := runCmd(t,
		"sidecar", "add-manual-directive",
		"--path", mdPath,
		"--text", "MUST use the sidecar pattern (ref: ADR-001 + manual)",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}

	loaded, err := sidecar.Load(sidecarPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.ManualDirectives) != 1 {
		t.Fatalf("want 1 manual directive, got %d", len(loaded.ManualDirectives))
	}
}

// TestAddManual_ParentMdUnchanged — INV-002 regression: the parent .md is
// byte-equal before and after add-manual-directive.
func TestAddManual_ParentMdUnchanged(t *testing.T) {
	dir := t.TempDir()
	sc := writeTempSidecar(t, dir, "ADR-006-test",
		minimalSidecarYAML("architecture", "docs/architecture/decisions/ADR-006-test.md"))

	// Write a fake .md next to the sidecar
	mdPath := strings.TrimSuffix(sc, ".edikt.yaml") + ".md"
	mdContent := []byte("# ADR-006\n\nSome immutable content.\n")
	if err := os.WriteFile(mdPath, mdContent, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := runCmd(t,
		"sidecar", "add-manual-directive",
		"--path", sc,
		"--text", "MUST preserve immutable prose (ref: ADR-006 + manual)",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(mdContent) {
		t.Errorf("parent .md was modified\nbefore: %q\nafter:  %q", mdContent, after)
	}
}

// TestAddManual_INVSidecar — case-insensitive: INV-NNN sidecars get (ref: INV-NNN + manual).
func TestAddManual_INVSidecar(t *testing.T) {
	dir := t.TempDir()
	sc := writeTempSidecar(t, dir, "INV-002-adr-immutability",
		minimalSidecarYAML("governance", "docs/architecture/invariants/INV-002-adr-immutability.md"))

	out, err := runCmd(t,
		"sidecar", "add-manual-directive",
		"--path", sc,
		"--text", "MUST NEVER edit accepted ADR prose",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}

	loaded, err := sidecar.Load(sc)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.ManualDirectives) != 1 {
		t.Fatalf("want 1 directive, got %d", len(loaded.ManualDirectives))
	}
	got := loaded.ManualDirectives[0]
	if !strings.Contains(got, "(ref: INV-002 + manual)") {
		t.Errorf("expected '(ref: INV-002 + manual)' in %q", got)
	}
}
