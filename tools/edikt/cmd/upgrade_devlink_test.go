package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/versioncmp"
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

// THE FLIP RECORDED IN docs/internal/plans/verdicts/SPEC-024-flip-list.md,
// ENTRY (a). This test used to assert the OPPOSITE — that every release
// compares GREATER than `dev` — and it was green because that is what the
// truncating parser answered: semverParts("dev") found no leading digits and
// returned [0,0,0], so a developer's own checkout sorted below everything ever
// published. The test was a faithful record of the defect, not a mistake, and
// its expected value was written down before this change so meeting it red
// could not be resolved by deleting it.
//
// EXPECTED VALUE NOW: incomparable. A channel name has no position in a
// release ordering; `dev` is neither ahead of nor behind 0.4.5, and any total
// order placed on it is a fabrication.
//
// THE SUBJECT DID NOT CHANGE: the same three release strings against the same
// `dev`, so this is a change of ANSWER and not a change of question.
func TestDevVersionIsIncomparableWithEveryRelease(t *testing.T) {
	devV := normalizeTag("dev")
	for _, rel := range []string{"0.4.5", "0.6.0", "1.0.0"} {
		got := versioncmp.Compare(rel, devV)
		if got != versioncmp.Incomparable {
			t.Errorf("versioncmp.Compare(%q, dev=%q) = %q; expected incomparable — "+
				"an ordering between a release and a channel name is the fabrication "+
				"that made the downgrade possible", rel, devV, got)
		}
	}
}
