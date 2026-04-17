package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDevLinkAndUnlink(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)

	// dev link points at the edikt repo which has templates/, commands/.
	repoRoot := "../../../../.." // tools/gov-compile/cmd/ → repo root
	abs, _ := filepath.Abs(repoRoot)

	buf, err := runCmd(t, "dev", "link", abs)
	if err != nil {
		t.Fatalf("dev link: %v\n%s", err, buf)
	}

	// dev link creates versions/dev/ as a directory with internal symlinks.
	devDir := filepath.Join(root, "versions", "dev")
	if _, err := os.Stat(devDir); err != nil {
		t.Fatalf("expected versions/dev to exist after dev link: %v", err)
	}
	entries, _ := os.ReadDir(devDir)
	hasSymlink := false
	for _, e := range entries {
		info, _ := e.Info()
		if info != nil && info.Mode()&os.ModeSymlink != 0 {
			hasSymlink = true
			break
		}
	}
	if !hasSymlink {
		t.Errorf("expected at least one symlink inside versions/dev/, entries: %v", entries)
	}

	// Seed a real version so unlink has something to fall back to.
	if err := os.MkdirAll(filepath.Join(root, "versions", "0.5.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeLock(root, "0.5.0", "test"); err != nil {
		t.Fatal(err)
	}
	if err := writeLock(root, "dev", "dev-link"); err != nil {
		t.Fatal(err)
	}

	buf, err = runCmd(t, "dev", "unlink")
	if err != nil {
		t.Fatalf("dev unlink: %v\n%s", err, buf)
	}
	if _, err := os.Lstat(devDir); !os.IsNotExist(err) {
		t.Errorf("expected dev dir removed after unlink")
	}
}

func TestDevLinkRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)

	_, err := runCmd(t, "dev", "link", "../../../etc")
	if err == nil {
		t.Fatal("expected error for path traversal in dev link")
	}
}
