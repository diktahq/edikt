package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SPEC-015 SP-006 — edikt upgrade refuses to jump more than one minor line
// in a single "resolve latest, then activate" pass. EDIKT_RELEASE_TAG lets
// these tests pin what release.ResolveLatestTag returns without hitting the
// network (the same override cmd/release_parity_test.go already relies on).

// seedUpgradeProject builds an EDIKT_ROOT with a lock.yaml recording
// currentV as the active version, and points EDIKT_ROOT/HOME at it so
// resolveEdiktRoot() finds it via the env override.
func seedUpgradeProject(t *testing.T, currentV string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeLock(root, currentV, "launcher", lockSourcePublished); err != nil {
		t.Fatalf("writeLock: %v", err)
	}
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("HOME", t.TempDir()) // unrelated HOME so nothing else is found
	return root
}

func TestUpgradeRefusesSkippedMinorLine(t *testing.T) {
	root := seedUpgradeProject(t, "0.6.5")
	t.Setenv("EDIKT_INSTALL_REPO", "")
	t.Setenv("EDIKT_RELEASE_TAG", "v0.8.0")
	t.Setenv("EDIKT_INSTALL_SOURCE", "") // must never be reached

	prevYes, prevDry := upgradeYes, upgradeDryRun
	upgradeYes, upgradeDryRun = false, false
	t.Cleanup(func() { upgradeYes, upgradeDryRun = prevYes, prevDry })

	err := upgradeCmd.RunE(upgradeCmd, nil)
	if err == nil {
		t.Fatal("expected refusal for a v0.6.5 -> v0.8.0 jump (skips the v0.7 line), got nil")
	}
	got := err.Error()
	for _, want := range []string{"skips a minor line", "v0.6.5", "v0.8.0"} {
		if !strings.Contains(got, want) {
			t.Errorf("refusal message missing %q: %v", want, got)
		}
	}

	// No download: versions/0.8.0 was never created.
	if _, statErr := os.Stat(filepath.Join(root, "versions", "0.8.0")); statErr == nil {
		t.Error("versions/0.8.0 exists — the gate did not stop the download")
	}

	// No activation: lock.yaml's active version is unchanged, and no
	// `current` symlink was ever created (this project never had one).
	lf, rerr := readLock(root)
	if rerr != nil {
		t.Fatalf("readLock: %v", rerr)
	}
	if lf.Active != "0.6.5" {
		t.Errorf("lock.yaml active = %q, want unchanged %q — activation happened despite the refusal", lf.Active, "0.6.5")
	}
	if _, statErr := os.Lstat(filepath.Join(root, "current")); statErr == nil {
		t.Error("current symlink exists — activation happened despite the refusal")
	}
}

func TestUpgradeAllowsAdjacentMinorLine(t *testing.T) {
	seedUpgradeProject(t, "0.7.5")
	t.Setenv("EDIKT_INSTALL_REPO", "")
	t.Setenv("EDIKT_RELEASE_TAG", "v0.8.0")

	prevYes, prevDry := upgradeYes, upgradeDryRun
	// dry-run: proves the gate does not fire for an adjacent-line jump
	// without this test needing to hit the network for the actual download.
	upgradeYes, upgradeDryRun = false, true
	t.Cleanup(func() { upgradeYes, upgradeDryRun = prevYes, prevDry })

	err := upgradeCmd.RunE(upgradeCmd, nil)
	if err != nil {
		t.Fatalf("v0.7.5 -> v0.8.0 (adjacent minor line) should proceed past the gate, got error: %v", err)
	}
}
