package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListEmpty(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)

	buf, err := runCmd(t, "list")
	if err != nil {
		t.Fatalf("list on empty root: %v\n%s", err, buf)
	}
}

func TestListShowsVersions(t *testing.T) {
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

	buf, err := runCmd(t, "list")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, buf)
	}
	if !contains(buf, "0.4.3") || !contains(buf, "0.5.0") {
		t.Errorf("expected both versions in output, got:\n%s", buf)
	}
	if !contains(buf, "*") {
		t.Errorf("expected active version marked with *, got:\n%s", buf)
	}
}
