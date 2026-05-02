package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUseMissingVersion(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	// INV-007: sandbox CLAUDE_HOME so any code path that resolves it
	// (today: only repairExternalSymlinks; defensive for future
	// refactors) cannot escape into the host's ~/.claude.
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	_, err := runCmd(t, "use", "v9.9.9")
	if err == nil {
		t.Fatal("expected error for non-installed version, got nil")
	}
}

func TestUseExistingVersion(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	// INV-007: sandbox CLAUDE_HOME. Without this, repairExternalSymlinks
	// resolves to $HOME/.claude and writes a `commands/edikt` symlink
	// pointing into the test's temp EDIKT_ROOT — when the temp dir is
	// cleaned up at test end, the user's ~/.claude/commands/edikt
	// dangles and breaks Claude Code's slash-command resolution.
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	// Both versions must be ≥ 0.5.0 (minimum supported payload version).
	for _, v := range []string{"0.5.0", "0.5.1"} {
		if err := os.MkdirAll(filepath.Join(root, "versions", v), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeLock(root, "0.5.1", "test"); err != nil {
		t.Fatal(err)
	}

	buf, err := runCmd(t, "use", "0.5.0")
	if err != nil {
		t.Fatalf("use 0.5.0: %v\n%s", err, buf)
	}

	after, err := readLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if after.Active != "0.5.0" {
		t.Errorf("expected active=0.5.0, got %s", after.Active)
	}
	if after.Previous != "0.5.1" {
		t.Errorf("expected previous=0.5.1, got %s", after.Previous)
	}
}
