package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	// INV-007: sandbox EDIKT_ROOT + CLAUDE_HOME so this test reads
	// from a controlled fake-version, not the user's actual install.
	// Without this, a developer running the suite while their global
	// edikt is dev-linked sees `dev\n` (no semver dot) and the assert
	// fails on the local dev's machine while passing in CI.
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	// Stage a fake versioned install so the version command has
	// something semver-shaped to read.
	if err := os.MkdirAll(filepath.Join(root, "versions", "0.5.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "versions", "0.5.0", "VERSION"), []byte("0.5.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeLock(root, "0.5.0", "test"); err != nil {
		t.Fatal(err)
	}

	buf, err := runCmd(t, "version")
	if err != nil {
		t.Fatalf("version: %v\n%s", err, buf)
	}
	if !strings.Contains(buf, ".") {
		t.Errorf("version output should contain a semver dot, got: %q", buf)
	}
}
