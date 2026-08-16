package phaseb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

func mustRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

// TestOrphanCleanup_RemovesSurfacesOfADisappearedTopic is the AC-2.5 core.
//
// When a topic's last artifact is deleted, the topic file and skill package it
// rendered last time are still on disk. Nothing else would remove them: the
// merge loop only visits topics that still EXIST, so an orphan is invisible to
// the code that would otherwise notice it.
//
// A lingering orphan is not cosmetic. A tier-2 rules file keeps loading
// whenever its globs match, delivering directives from an artifact the project
// deliberately removed — governance that outlives its own source, with nothing
// reporting that it did.
func TestOrphanCleanup_RemovesSurfacesOfADisappearedTopic(t *testing.T) {
	root := t.TempDir()

	keep := mkPair(t, root, "ADR-001-test", "keeper", []sidecar.Directive{
		{Text: "Keeper rule. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
	})
	keep.ArtifactID = "ADR-001"
	doomed := mkPair(t, root, "ADR-002-test", "doomed", []sidecar.Directive{
		{Text: "Doomed rule. (ref: ADR-002)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"}},
	})
	doomed.ArtifactID = "ADR-002"

	if _, err := Merge(root, []sidecar.Pair{keep, doomed}, Options{
		CompiledAt: "2026-08-12T00:00:00Z", CompilerVersion: "0.6.0-test",
	}); err != nil {
		t.Fatalf("first merge: %v", err)
	}

	doomedTopic := filepath.Join(root, ".claude", "rules", "governance", "doomed.md")
	doomedSkill := filepath.Join(root, ".claude", "skills", "edikt-doomed", "SKILL.md")
	// PIN THE SUBJECT: if the first compile never wrote these, the deletion
	// assertions below would pass against files that never existed.
	for _, p := range []string{doomedTopic, doomedSkill} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("precondition: first compile did not write %s: %v", p, err)
		}
	}

	res, err := Merge(root, []sidecar.Pair{keep}, Options{
		CompiledAt: "2026-08-12T00:00:00Z", CompilerVersion: "0.6.0-test",
	})
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}

	for _, p := range []string{doomedTopic, doomedSkill} {
		if _, serr := os.Stat(p); serr == nil {
			t.Errorf("orphaned surface survived the compile that dropped its topic: %s", p)
		}
	}
	// The empty skill DIRECTORY must go too — an edikt-doomed/ with no
	// SKILL.md still registers as a skill with no content.
	if _, serr := os.Stat(filepath.Dir(doomedSkill)); serr == nil {
		t.Errorf("empty skill directory survived: %s", filepath.Dir(doomedSkill))
	}

	// Reported, never silent (INV-013).
	if len(res.OrphansRemoved) == 0 {
		t.Error("orphan removal was not reported; a file disappearing with nothing " +
			"naming it is indistinguishable from a compile that lost it")
	}
}

// TestOrphanCleanup_LeavesEveryOtherSurfaceByteIdentical is the other half of
// AC-2.5, and the half a naive implementation fails.
//
// "Delete what is gone" is easy to satisfy by re-rendering everything, which
// would also churn every surviving surface. The criterion says the surviving
// topics stay BYTE-IDENTICAL — because a cleanup that rewrites the whole tree
// makes every compile a diff, and a governance tree that is permanently dirty
// in git is one nobody reviews.
func TestOrphanCleanup_LeavesEveryOtherSurfaceByteIdentical(t *testing.T) {
	root := t.TempDir()

	keep := mkPair(t, root, "ADR-001-test", "keeper", []sidecar.Directive{
		{Text: "Keeper rule. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
	})
	keep.ArtifactID = "ADR-001"
	doomed := mkPair(t, root, "ADR-002-test", "doomed", []sidecar.Directive{
		{Text: "Doomed rule. (ref: ADR-002)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"}},
	})
	doomed.ArtifactID = "ADR-002"

	if _, err := Merge(root, []sidecar.Pair{keep, doomed}, Options{
		CompiledAt: "2026-08-12T00:00:00Z", CompilerVersion: "0.6.0-test",
	}); err != nil {
		t.Fatalf("first merge: %v", err)
	}

	keeperTopic := filepath.Join(root, ".claude", "rules", "governance", "keeper.md")
	keeperSkill := filepath.Join(root, ".claude", "skills", "edikt-keeper", "SKILL.md")
	beforeTopic := mustRead(t, keeperTopic)
	beforeSkill := mustRead(t, keeperSkill)

	if _, err := Merge(root, []sidecar.Pair{keep}, Options{
		CompiledAt: "2026-08-12T00:00:00Z", CompilerVersion: "0.6.0-test",
	}); err != nil {
		t.Fatalf("second merge: %v", err)
	}

	if got := mustRead(t, keeperTopic); got != beforeTopic {
		t.Errorf("surviving topic file changed bytes during orphan cleanup;\nbefore:\n%s\nafter:\n%s",
			beforeTopic, got)
	}
	if got := mustRead(t, keeperSkill); got != beforeSkill {
		t.Errorf("surviving skill package changed bytes during orphan cleanup")
	}
}

// TestOrphanCleanup_DoesNotDeleteUnownedFiles is the isolation control.
//
// Cleanup reads paths from a file on disk and deletes them. Without this, an
// implementation that deleted whatever the manifest names would pass both
// tests above while turning a text edit into a delete primitive — point a
// hand-edited manifest at anything and compile removes it.
func TestOrphanCleanup_DoesNotDeleteUnownedFiles(t *testing.T) {
	root := t.TempDir()

	keep := mkPair(t, root, "ADR-001-test", "keeper", []sidecar.Directive{
		{Text: "Keeper rule. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
	})
	keep.ArtifactID = "ADR-001"
	if _, err := Merge(root, []sidecar.Pair{keep}, Options{
		CompiledAt: "2026-08-12T00:00:00Z", CompilerVersion: "0.6.0-test",
	}); err != nil {
		t.Fatalf("first merge: %v", err)
	}

	victim := filepath.Join(root, "PRECIOUS.md")
	if err := os.WriteFile(victim, []byte("not edikt's file\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Forge a manifest entry pointing at it, with a kind compile does not own.
	manPath := filepath.Join(root, ".claude", "rules", "governance", ManifestName)
	man := mustRead(t, manPath)
	man += "  - path: \"PRECIOUS.md\"\n    kind: \"something-else\"\n    sha256: \"deadbeef\"\n"
	if err := os.WriteFile(manPath, []byte(man), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Merge(root, []sidecar.Pair{keep}, Options{
		CompiledAt: "2026-08-12T00:00:00Z", CompilerVersion: "0.6.0-test",
	}); err != nil {
		t.Fatalf("second merge: %v", err)
	}

	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("compile deleted a file it does not own, named by a hand-edited manifest: %v", err)
	}
	if body := mustRead(t, victim); !strings.Contains(body, "not edikt's file") {
		t.Error("unowned file was modified")
	}
}
