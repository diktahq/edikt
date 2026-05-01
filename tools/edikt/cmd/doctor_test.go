package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDoctorMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nonexistent")
	t.Setenv("EDIKT_ROOT", root)

	_, err := runCmd(t, "doctor")
	if err == nil {
		t.Fatal("expected doctor to fail when EDIKT_ROOT does not exist")
	}
}

func TestDoctorMinimalLayout(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)

	// INV-007: isolate from host ~/.claude — never read user state in tests.
	claudeRoot := t.TempDir()
	t.Setenv("CLAUDE_HOME", claudeRoot)
	if err := os.MkdirAll(filepath.Join(claudeRoot, "commands"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "versions", "0.5.0", "commands"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(
		filepath.Join(root, "versions", "0.5.0", "commands"),
		filepath.Join(claudeRoot, "commands", "edikt"),
	); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(
		filepath.Join(root, "versions", "0.5.0"),
		filepath.Join(root, "current"),
	); err != nil {
		t.Fatal(err)
	}
	if err := writeLock(root, "0.5.0", "test"); err != nil {
		t.Fatal(err)
	}

	buf, err := runCmd(t, "doctor")
	// exit 0 (healthy) or exit 1 (warnings) are both acceptable.
	// Only exit 2 (errors about the layout itself) is a test failure.
	if err != nil && isExitCode(err, 2) {
		t.Fatalf("doctor returned errors (exit 2) for valid layout:\n%s", buf)
	}
}
