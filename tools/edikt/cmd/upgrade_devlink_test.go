package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// A dev-linked install must be detected from the FILESYSTEM, not from a
// recorded version. Its version parses as 0.0.0, so every release compares
// greater and upgrade would replace the developer's working-tree link with a
// stable tarball — a silent downgrade. Today it 404s only because the asset
// does not exist at the computed URL, which is luck rather than a guard.
func TestIsDevLinked(t *testing.T) {
	mkroot := func(t *testing.T) string {
		d, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return d
	}

	t.Run("dev link is detected", func(t *testing.T) {
		root := mkroot(t)
		if err := os.MkdirAll(filepath.Join(root, "versions", "dev"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("versions", "dev"), filepath.Join(root, "current")); err != nil {
			t.Fatal(err)
		}
		if ok, target := isDevLinked(root); !ok {
			t.Fatalf("dev link not detected (target=%q) — upgrade would proceed and clobber it", target)
		}
	})

	t.Run("a normal versioned install is NOT flagged", func(t *testing.T) {
		root := mkroot(t)
		if err := os.MkdirAll(filepath.Join(root, "versions", "0.6.0"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("versions", "0.6.0"), filepath.Join(root, "current")); err != nil {
			t.Fatal(err)
		}
		if ok, _ := isDevLinked(root); ok {
			t.Fatal("versioned install flagged as dev-linked — upgrade would refuse legitimately-upgradable installs")
		}
	})

	t.Run("no current symlink at all is NOT flagged", func(t *testing.T) {
		root := mkroot(t)
		if ok, _ := isDevLinked(root); ok {
			t.Fatal("empty root flagged as dev-linked")
		}
	})
}

// The version string a dev install records is exactly why the model-based
// check fails: it compares LESS than every release, so "is an upgrade
// available?" answers yes for a tree that must not be touched.
func TestDevVersionComparesLessThanEveryRelease(t *testing.T) {
	devV := normalizeTag("dev")
	for _, rel := range []string{"0.4.5", "0.6.0", "1.0.0"} {
		if !semverGreater(rel, devV) {
			t.Errorf("semverGreater(%q, dev=%q) = false; expected true — "+
				"this is the comparison that made the downgrade possible", rel, devV)
		}
	}
}
