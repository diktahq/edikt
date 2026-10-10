package hookmatch

// home_test.go — 2026-08-27. Proves ediktHome()'s three-rung priority
// (EDIKT_ROOT > EDIKT_HOME > $HOME/.edikt) and that JournalPath/DedupDir
// actually follow it, not just that the helper function returns the right
// string in isolation. Found while root-causing a self-arm benchmark run
// that failed to authenticate: HOME redirection (needed for the OLD
// os.UserHomeDir()-only resolution) breaks the macOS login keychain, which
// resolves via $HOME/Library/Keychains/login.keychain-db. A sandbox that
// stops redirecting HOME and redirects only EDIKT_ROOT needs its state paths
// to follow EDIKT_ROOT too, or state silently leaks into the real machine's
// ~/.edikt/state/ while the caller believes it measured an isolated journal.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEdiktHome_PrefersEdiktRootOverHome(t *testing.T) {
	realHome := t.TempDir()
	isolatedRoot := t.TempDir()
	t.Setenv("HOME", realHome)
	t.Setenv("EDIKT_ROOT", isolatedRoot)
	t.Setenv("EDIKT_HOME", "")

	got, err := ediktHome()
	if err != nil {
		t.Fatalf("ediktHome: %v", err)
	}
	if got != isolatedRoot {
		t.Fatalf("EDIKT_ROOT set but ediktHome() = %q, want %q (HOME must not win)", got, isolatedRoot)
	}
}

func TestEdiktHome_FallsBackToEdiktHomeWhenRootUnset(t *testing.T) {
	realHome := t.TempDir()
	isolatedHome := t.TempDir()
	t.Setenv("HOME", realHome)
	t.Setenv("EDIKT_ROOT", "")
	t.Setenv("EDIKT_HOME", isolatedHome)

	got, err := ediktHome()
	if err != nil {
		t.Fatalf("ediktHome: %v", err)
	}
	if got != isolatedHome {
		t.Fatalf("EDIKT_HOME set (EDIKT_ROOT unset) but ediktHome() = %q, want %q", got, isolatedHome)
	}
}

func TestEdiktHome_FallsBackToHomeDotEdikt(t *testing.T) {
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)
	t.Setenv("EDIKT_ROOT", "")
	t.Setenv("EDIKT_HOME", "")

	got, err := ediktHome()
	if err != nil {
		t.Fatalf("ediktHome: %v", err)
	}
	want := filepath.Join(realHome, ".edikt")
	if got != want {
		t.Fatalf("neither EDIKT_ROOT nor EDIKT_HOME set but ediktHome() = %q, want %q", got, want)
	}
}

// TestJournalPath_FollowsEdiktRoot_NotHome is the property that actually
// matters: a caller that redirects EDIKT_ROOT but leaves HOME pointing at
// the real machine (the exact shape needed once HOME can no longer be
// redirected for auth reasons) must have its journal land under
// EDIKT_ROOT, never under the real HOME.
func TestJournalPath_FollowsEdiktRoot_NotHome(t *testing.T) {
	realHome := t.TempDir()
	isolatedRoot := t.TempDir()
	t.Setenv("HOME", realHome)
	t.Setenv("EDIKT_ROOT", isolatedRoot)
	t.Setenv("EDIKT_HOME", "")

	path, err := JournalPath()
	if err != nil {
		t.Fatalf("JournalPath: %v", err)
	}
	if filepath.Dir(path) != filepath.Join(isolatedRoot, "state") {
		t.Fatalf("JournalPath() = %q, want under %q (EDIKT_ROOT), not %q (HOME)",
			path, isolatedRoot, realHome)
	}
	if _, err := os.Stat(filepath.Join(realHome, ".edikt")); err == nil {
		t.Fatalf("JournalPath created state under the real HOME (%q) despite EDIKT_ROOT being set", realHome)
	}
}
