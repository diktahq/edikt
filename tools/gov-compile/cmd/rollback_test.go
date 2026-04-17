package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRollbackNoPrevious(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)

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
