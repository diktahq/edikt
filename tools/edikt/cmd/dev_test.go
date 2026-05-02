package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDevLinkAndUnlink(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	// INV-007: sandbox CLAUDE_HOME. dev link now calls
	// repairExternalSymlinks (rc4 fix for the slash-command symlink
	// not refreshing on dev link); without sandbox the test would
	// touch the host's ~/.claude/commands/edikt.
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	// dev link points at the edikt repo which has templates/, commands/.
	repoRoot := "../../.." // tools/edikt/cmd/ → repo root
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
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	_, err := runCmd(t, "dev", "link", "../../../etc")
	if err == nil {
		t.Fatal("expected error for path traversal in dev link")
	}
}

// TestDevLink_RefreshesLauncherBinary pins the rc4 fix for issue #4
// from the v0.6.0-rc3 dogfood test: dev link now copies <src>/bin/edikt
// over $EDIKT_ROOT/bin/edikt so the user-facing launcher reflects the
// dev source's behavior (subcommands, flags, etc.). Without this fix,
// the user's PATH-resolved `edikt` keeps running whatever was last
// installed via `edikt install` / `edikt upgrade`, and the dev link
// only affects payload paths — which led to "edikt migrate sidecars"
// failing with `unknown command "sidecars"` because the launcher was
// still v0.5.1 even though the dev link pointed at the rc3 source.
//
// dev unlink restores the backup, so the launcher returns to its
// pre-dev state.
func TestDevLink_RefreshesLauncherBinary(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	// Stage a fake dev source directory with a recognizable binary so
	// we can assert dev link copies it into $EDIKT_ROOT/bin/edikt.
	srcDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(srcDir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "commands"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "VERSION"), []byte("0.99.0-dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	devBin := filepath.Join(srcDir, "bin", "edikt")
	devBinContent := []byte("dev-source-binary-v0.99.0\n")
	if err := os.WriteFile(devBin, devBinContent, 0o755); err != nil {
		t.Fatal(err)
	}

	// Stage a different "previously installed" launcher so we can
	// observe the swap.
	launcherDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(launcherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	launcherBin := filepath.Join(launcherDir, "edikt")
	prevContent := []byte("previously-installed-launcher-v0.5.1\n")
	if err := os.WriteFile(launcherBin, prevContent, 0o755); err != nil {
		t.Fatal(err)
	}

	// Run dev link.
	buf, err := runCmd(t, "dev", "link", srcDir)
	if err != nil {
		t.Fatalf("dev link: %v\n%s", err, buf)
	}

	// Assert the launcher was swapped to the dev source's binary.
	gotLauncher, err := os.ReadFile(launcherBin)
	if err != nil {
		t.Fatalf("read launcher after dev link: %v", err)
	}
	if string(gotLauncher) != string(devBinContent) {
		t.Errorf("launcher binary not refreshed.\n got: %q\nwant: %q", gotLauncher, devBinContent)
	}

	// Assert the previous launcher was backed up.
	backup := launcherBin + ".pre-dev"
	gotBackup, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("expected pre-dev backup at %s: %v", backup, err)
	}
	if string(gotBackup) != string(prevContent) {
		t.Errorf("pre-dev backup mismatched.\n got: %q\nwant: %q", gotBackup, prevContent)
	}

	// Now seed a real version + lock so unlink can fall back, then
	// run dev unlink and assert the launcher restored from backup.
	if err := os.MkdirAll(filepath.Join(root, "versions", "0.5.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeLock(root, "0.5.0", "test"); err != nil {
		t.Fatal(err)
	}
	if err := writeLock(root, "dev", "dev-link"); err != nil {
		t.Fatal(err)
	}

	if _, err := runCmd(t, "dev", "unlink"); err != nil {
		t.Fatalf("dev unlink: %v", err)
	}
	gotAfterUnlink, _ := os.ReadFile(launcherBin)
	if string(gotAfterUnlink) != string(prevContent) {
		t.Errorf("launcher not restored after dev unlink.\n got: %q\nwant: %q", gotAfterUnlink, prevContent)
	}
	// Backup should be removed after restore.
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Errorf("pre-dev backup should be removed after dev unlink; stat: %v", err)
	}
}
