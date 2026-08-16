package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifestFile(t *testing.T, govDir string, trackedNames ...string) {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("schema_version: 1\nsurfaces:\n")
	for _, name := range trackedNames {
		b.WriteString("  - path: \".claude/rules/governance/" + name + "\"\n")
		b.WriteString("    kind: \"topic-file\"\n")
		b.WriteString("    sha256: \"deadbeef\"\n")
	}
	if len(trackedNames) == 0 {
		b.Reset()
		b.WriteString("schema_version: 1\nsurfaces: []\n")
	}
	writeFile(t, filepath.Join(govDir, "manifest.yaml"), b.String())
}

func TestOrphanSurfacesCheck_zeroInput_noManifest(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	warn, ran := runOrphanSurfacesCheck(root, &out)
	if ran {
		t.Fatalf("expected ran=false when no manifest exists yet, got ran=true warn=%d\n%s", warn, out.String())
	}
}

func TestOrphanSurfacesCheck_clean(t *testing.T) {
	root := t.TempDir()
	govDir := filepath.Join(root, ".claude", "rules", "governance")
	if err := os.MkdirAll(govDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(govDir, "hooks.md"), "# hooks\n")
	writeManifestFile(t, govDir, "hooks.md")

	var out bytes.Buffer
	warn, ran := runOrphanSurfacesCheck(root, &out)
	if !ran {
		t.Fatal("expected ran=true")
	}
	if warn != 0 {
		t.Fatalf("expected 0 warnings when every .md is tracked, got %d\n%s", warn, out.String())
	}
}

// This is the positive case: a real .md file sitting in the governance dir
// with no entry in the current manifest — the shape a lost/reset manifest
// or pre-manifest-tracking file leaves behind, confirmed live
// (.claude/rules/governance/deployment.md, compile_schema_version: 2, 8
// directives, absent from the render manifest).
func TestOrphanSurfacesCheck_orphanDetected(t *testing.T) {
	root := t.TempDir()
	govDir := filepath.Join(root, ".claude", "rules", "governance")
	if err := os.MkdirAll(govDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(govDir, "hooks.md"), "# hooks\n")
	writeFile(t, filepath.Join(govDir, "deployment.md"), "# deployment (orphaned)\n")
	writeManifestFile(t, govDir, "hooks.md")

	var out bytes.Buffer
	warn, ran := runOrphanSurfacesCheck(root, &out)
	if !ran {
		t.Fatal("expected ran=true")
	}
	if warn != 1 {
		t.Fatalf("expected 1 warning, got %d\n%s", warn, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "deployment.md") {
		t.Fatalf("warning must name the orphaned file:\n%s", s)
	}
	if strings.Contains(s, "orphan: hooks.md") {
		t.Fatalf("must not flag a tracked file as orphaned:\n%s", s)
	}
}
