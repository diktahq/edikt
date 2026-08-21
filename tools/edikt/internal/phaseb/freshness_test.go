package phaseb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

func freshnessOpts(root string) Options {
	return Options{
		CompiledAt:      "2026-08-12T00:00:00Z",
		CompilerVersion: "0.6.0-test",
		OutDir:          filepath.Join(root, ".claude", "rules", "governance"),
		IndexPath:       filepath.Join(root, ".claude", "rules", "governance.md"),
	}
}

// TestRenderFreshness_CleanTreeReportsNoDrift is the ISOLATION half, and it
// has to come first: a checker that reports drift unconditionally would
// satisfy every sensitivity test ever written. If this fails, the signal is a
// constant and the other test proves nothing.
func TestRenderFreshness_CleanTreeReportsNoDrift(t *testing.T) {
	root := t.TempDir()
	p := mkPair(t, root, "ADR-001-test", "alpha", []sidecar.Directive{
		{Text: "Alpha rule. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
	})
	p.ArtifactID = "ADR-001"
	pairs := []sidecar.Pair{p}

	if _, err := Merge(root, pairs, freshnessOpts(root)); err != nil {
		t.Fatalf("merge: %v", err)
	}

	drifts, err := CheckRenderFreshness(root, pairs, freshnessOpts(root))
	if err != nil {
		t.Fatalf("freshness check errored on a freshly rendered tree: %v", err)
	}
	if len(drifts) != 0 {
		var lines []string
		for _, d := range drifts {
			lines = append(lines, d.String())
		}
		t.Fatalf("freshly rendered tree reported as stale — the signal is a constant "+
			"and cannot evidence anything:\n  %s", strings.Join(lines, "\n  "))
	}
}

// TestRenderFreshness_StaleTreeFailsNamingTheDrift is the SENSITIVITY half and
// the PC-8 case exactly: every sidecar is fresh against its parent, but the
// rendered tree came from an earlier sidecar state. --check used to report
// clean here, which is the specific hole this closes (SR-020 / SAC-003).
func TestRenderFreshness_StaleTreeFailsNamingTheDrift(t *testing.T) {
	root := t.TempDir()

	// Render from the OLD corpus.
	old := mkPair(t, root, "ADR-001-test", "alpha", []sidecar.Directive{
		{Text: "Alpha rule. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
	})
	old.ArtifactID = "ADR-001"
	if _, err := Merge(root, []sidecar.Pair{old}, freshnessOpts(root)); err != nil {
		t.Fatalf("merge (old state): %v", err)
	}

	// The sidecars now say MORE than the tree does. Nothing re-rendered.
	now := mkPair(t, root, "ADR-001-test", "alpha", []sidecar.Directive{
		{Text: "Alpha rule. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		{Text: "Second alpha rule. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 2, LineEnd: 2, Quote: "y"}},
	})
	now.ArtifactID = "ADR-001"

	drifts, err := CheckRenderFreshness(root, []sidecar.Pair{now}, freshnessOpts(root))
	if err != nil {
		t.Fatalf("freshness check: %v", err)
	}
	if len(drifts) == 0 {
		t.Fatal("stale rendered tree reported CLEAN — this is the exact PC-8 failure: " +
			"sidecars fresh, surfaces rendered from an earlier state, --check silent")
	}

	// NAMED, not bare. The whole point of PC-8 is that "stale" with no
	// specifics gives the reader nothing to pull on when re-running compile
	// does not fix it.
	joined := ""
	for _, d := range drifts {
		joined += d.String() + "\n"
	}
	if !strings.Contains(joined, "alpha.md") {
		t.Errorf("drift does not name the surface that moved:\n%s", joined)
	}
	if !strings.Contains(joined, "directive count") {
		t.Errorf("drift does not name the measurable delta (directive count):\n%s", joined)
	}
	if strings.Contains(joined, "stale\n") && !strings.Contains(joined, "->") {
		t.Errorf("drift reported bare, with no direction:\n%s", joined)
	}
}

// TestRenderFreshness_DetectsAnOrphanedSurface covers the other direction: a
// surface on disk that a fresh render would not produce at all. Content
// comparison alone cannot see it — there is nothing to compare it against —
// so it needs its own enumeration of what is actually on disk.
func TestRenderFreshness_DetectsAnOrphanedSurface(t *testing.T) {
	root := t.TempDir()
	p := mkPair(t, root, "ADR-001-test", "alpha", []sidecar.Directive{
		{Text: "Alpha rule. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
	})
	p.ArtifactID = "ADR-001"
	pairs := []sidecar.Pair{p}
	if _, err := Merge(root, pairs, freshnessOpts(root)); err != nil {
		t.Fatalf("merge: %v", err)
	}

	// A topic file from a corpus state that no longer exists.
	ghost := filepath.Join(root, ".claude", "rules", "governance", "ghost.md")
	if err := os.WriteFile(ghost, []byte("---\npaths:\n  - \"x/**\"\n---\n\n- Ghost rule. (ref: ADR-099)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	drifts, err := CheckRenderFreshness(root, pairs, freshnessOpts(root))
	if err != nil {
		t.Fatalf("freshness check: %v", err)
	}
	found := false
	for _, d := range drifts {
		if strings.Contains(d.Path, "ghost.md") && d.Kind == "unexpected" {
			found = true
		}
	}
	if !found {
		t.Errorf("orphaned surface not reported; a rules file with no source in the corpus "+
			"keeps loading and delivering governance nothing stands behind. got: %v", drifts)
	}
}

// TestRenderFreshness_DetectsAnOrphanedSurface_RealCallShape is N1
// (docs/internal/audits/TRIAGE-2026-08-20-bok-services-governance-projection.md):
// the ACTUAL production call site (govrun/twophase.go, around the
// `phaseb.CheckRenderFreshness` call) constructs its Options literal with only
// CompiledAt/CompilerVersion/Excluded/TopicDescriptions set — never OutDir or
// IndexPath, unlike every test above via freshnessOpts(). Before the fix,
// liveSurfaces() silently scanned an empty path and orphan detection never
// fired in production, even though the identical scenario above (fed through
// freshnessOpts()) always caught it. This pins the real call shape, not the
// test helper's.
func TestRenderFreshness_DetectsAnOrphanedSurface_RealCallShape(t *testing.T) {
	root := t.TempDir()
	p := mkPair(t, root, "ADR-001-test", "alpha", []sidecar.Directive{
		{Text: "Alpha rule. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
	})
	p.ArtifactID = "ADR-001"
	pairs := []sidecar.Pair{p}
	if _, err := Merge(root, pairs, freshnessOpts(root)); err != nil {
		t.Fatalf("merge: %v", err)
	}

	// The N1 scenario exactly: an orphaned compiled file from a topic no
	// sidecar declares anymore, sitting directly in the governance output
	// directory.
	orphan := filepath.Join(root, ".claude", "rules", "governance", "classification.md")
	if err := os.WriteFile(orphan, []byte("---\npaths:\n  - \"x/**\"\n---\n\n- Stale rule. (ref: ADR-099)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// THE REAL CALL SHAPE — no OutDir, no IndexPath, matching
	// govrun/twophase.go's actual Options{} literal.
	realCallOpts := Options{
		CompiledAt:      "2026-08-20T00:00:00Z",
		CompilerVersion: "0.7.2-test",
	}

	drifts, err := CheckRenderFreshness(root, pairs, realCallOpts)
	if err != nil {
		t.Fatalf("freshness check: %v", err)
	}
	found := false
	for _, d := range drifts {
		if strings.Contains(d.Path, "classification.md") && d.Kind == "unexpected" {
			found = true
		}
	}
	if !found {
		t.Errorf("N1 regression: orphaned surface not reported when Options is constructed "+
			"the way production code actually constructs it (OutDir/IndexPath unset). got: %v", drifts)
	}
}
