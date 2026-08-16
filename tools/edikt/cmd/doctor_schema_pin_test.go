package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfigWithVersion(t *testing.T, root, version string) {
	t.Helper()
	writeFile(t, filepath.Join(root, ".edikt", "config.yaml"), "edikt_version: "+version+"\n")
}

// v2Sidecar returns a schema_version 2 (multi-anchor) sidecar — the shape
// WouldConvertToV2 must NOT flag.
func v2Sidecar(relMdPath string) string {
	return `schema_version: 2
topic: hooks
path: ` + relMdPath + `
signals:
  - hook
directives:
  - text: "Hooks must emit JSON. (ref: INV-003)"
    source_excerpts:
      - line_start: 1
        line_end: 1
        quote: "Hooks must emit JSON."
`
}

func TestSchemaPinCheck_zeroInput_noConfig(t *testing.T) {
	root := scaffoldProject(t)
	var out bytes.Buffer
	if got := runSchemaPinCheck(root, &out); got != 0 {
		t.Fatalf("no .edikt/config.yaml at all: expected 0 warnings, got %d\n%s", got, out.String())
	}
}

func TestSchemaPinCheck_zeroInput_noPin(t *testing.T) {
	root := scaffoldProject(t)
	writeFile(t, filepath.Join(root, ".edikt", "config.yaml"), "base: docs\n")
	var out bytes.Buffer
	if got := runSchemaPinCheck(root, &out); got != 0 {
		t.Fatalf("config.yaml with no edikt_version: expected 0 warnings, got %d\n%s", got, out.String())
	}
}

func TestSchemaPinCheck_belowFloor_v1IsExpected(t *testing.T) {
	root := scaffoldProject(t)
	writeConfigWithVersion(t, root, "0.6.0")
	mdRel := "docs/architecture/decisions/ADR-100-x.md"
	validMd(t, root, mdRel)
	writeFile(t, filepath.Join(root, "docs/architecture/decisions/ADR-100-x.edikt.yaml"), validSidecar(mdRel))

	var out bytes.Buffer
	if got := runSchemaPinCheck(root, &out); got != 0 {
		t.Fatalf("pinned below versionLineFloor: v1 sidecars are expected, not a disagreement; got %d warnings\n%s", got, out.String())
	}
}

func TestSchemaPinCheck_atFloor_v2Clean(t *testing.T) {
	root := scaffoldProject(t)
	writeConfigWithVersion(t, root, "0.7.0-rc3")
	mdRel := "docs/architecture/decisions/ADR-100-x.md"
	validMd(t, root, mdRel)
	writeFile(t, filepath.Join(root, "docs/architecture/decisions/ADR-100-x.edikt.yaml"), v2Sidecar(mdRel))

	var out bytes.Buffer
	if got := runSchemaPinCheck(root, &out); got != 0 {
		t.Fatalf("pinned at floor with a v2-shaped sidecar: expected 0 warnings, got %d\n%s", got, out.String())
	}
}

// This is the positive case: this is exactly the state a premature
// edikt_version bump (finding 2) produces — pinned at the v0.7+ line while a
// sidecar is still schema_version 1 — and the state the upgrade-flow gate in
// commands/upgrade.md Step 6 exists to prevent.
func TestSchemaPinCheck_atFloor_v1Disagreement(t *testing.T) {
	root := scaffoldProject(t)
	writeConfigWithVersion(t, root, "0.7.0-rc3")
	mdRel := "docs/architecture/decisions/ADR-100-x.md"
	validMd(t, root, mdRel)
	writeFile(t, filepath.Join(root, "docs/architecture/decisions/ADR-100-x.edikt.yaml"), validSidecar(mdRel))

	var out bytes.Buffer
	got := runSchemaPinCheck(root, &out)
	if got != 1 {
		t.Fatalf("pinned at v0.7+ with a v1-shaped sidecar: expected 1 warning, got %d\n%s", got, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "ADR-100") {
		t.Fatalf("warning must name the stale artifact:\n%s", s)
	}
	if !strings.Contains(s, "schema_version 1") {
		t.Fatalf("warning must state the disagreement in terms a reader can act on:\n%s", s)
	}
}

func TestSchemaPinCheck_configPathIsDirectory(t *testing.T) {
	root := scaffoldProject(t)
	// config.yaml is a directory, not a file — readPinnedVersion must fail
	// closed (no pin found) rather than panicking on the read.
	if err := os.MkdirAll(filepath.Join(root, ".edikt", "config.yaml"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var out bytes.Buffer
	if got := runSchemaPinCheck(root, &out); got != 0 {
		t.Fatalf("config.yaml as a directory: expected 0 warnings (fail-quiet, not fail-warn), got %d\n%s", got, out.String())
	}
}
