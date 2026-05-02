package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRollbackNoPrevious(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	// INV-007: sandbox CLAUDE_HOME so any code path that resolves it
	// (rollback's repairExternalSymlinks, defensive against future
	// refactors) cannot escape into the host's ~/.claude. Same class
	// of leak fixed for TestUseExistingVersion.
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	if err := os.MkdirAll(filepath.Join(root, "versions", "0.5.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeLock(root, "0.5.0", "test"); err != nil {
		t.Fatal(err)
	}

	_, err := runCmd(t, "rollback")
	if err == nil {
		t.Fatal("expected error when no previous version exists")
	}
}

func TestRollbackToPrevious(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	// INV-007: sandbox CLAUDE_HOME. Without this, rollback's
	// repairExternalSymlinks resolves to $HOME/.claude and writes a
	// commands/edikt symlink pointing into the test's temp EDIKT_ROOT.
	// When the temp dir is cleaned up at test end, the host's
	// ~/.claude/commands/edikt dangles and breaks Claude Code's
	// slash-command resolution for /edikt:* until manually relinked.
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	for _, v := range []string{"0.4.3", "0.5.0"} {
		if err := os.MkdirAll(filepath.Join(root, "versions", v), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Set up 0.5.0 active with 0.4.3 as previous.
	if err := writeLock(root, "0.4.3", "test"); err != nil {
		t.Fatal(err)
	}
	if err := writeLock(root, "0.5.0", "test"); err != nil {
		t.Fatal(err)
	}

	buf, err := runCmd(t, "rollback")
	if err != nil {
		t.Fatalf("rollback: %v\n%s", err, buf)
	}

	after, err := readLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if after.Active != "0.4.3" {
		t.Errorf("expected active=0.4.3 after rollback, got %s", after.Active)
	}
}
