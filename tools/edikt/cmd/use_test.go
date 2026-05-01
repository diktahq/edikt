package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUseMissingVersion(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)

	_, err := runCmd(t, "use", "v9.9.9")
	if err == nil {
		t.Fatal("expected error for non-installed version, got nil")
	}
}

func TestUseExistingVersion(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)

	for _, v := range []string{"0.4.3", "0.5.0"} {
		if err := os.MkdirAll(filepath.Join(root, "versions", v), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeLock(root, "0.5.0", "test"); err != nil {
		t.Fatal(err)
	}

	buf, err := runCmd(t, "use", "0.4.3")
	if err != nil {
		t.Fatalf("use 0.4.3: %v\n%s", err, buf)
	}

	after, err := readLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if after.Active != "0.4.3" {
		t.Errorf("expected active=0.4.3, got %s", after.Active)
	}
	if after.Previous != "0.5.0" {
		t.Errorf("expected previous=0.5.0, got %s", after.Previous)
	}
}
