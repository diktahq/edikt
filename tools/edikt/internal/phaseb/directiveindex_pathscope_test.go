package phaseb

import (
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// ADR-070 Phase 27 (PRD-010) — the precision fix. BuildDirectiveIndex used to
// fan every directive+prohibition in a sidecar out to every glob the sidecar
// declared, a Cartesian product rather than a filter (confirmed exactly
// against ADR-040's own sidecar: 5 globs x 33 directives = 165 entries).
// These tests pin the two AC-27.1/AC-27.2 properties: a directive-level
// override narrows to its own glob, and a sidecar with no overrides indexes
// byte-identical to the pre-fix behavior.

// fiveGlobSidecar mirrors ADR-040's own shape (5 sidecar-level globs), scaled
// down to a handful of directives so the Cartesian-product arithmetic stays
// easy to verify by hand.
func fiveGlobSidecar(directives []sidecar.Directive) sidecar.Pair {
	return sidecar.Pair{
		ArtifactID: "ADR-901",
		Sidecar: &sidecar.Sidecar{
			Paths:      []string{"a/**/*.go", "b/**/*.go", "c/**/*.go", "d/**/*.go", "e/**/*.go"},
			Directives: directives,
		},
	}
}

// TestBuildDirectiveIndex_PerDirectiveOverrideNarrowsToOwnGlob is AC-27.1: a
// fixture sidecar with 5 sidecar-level globs and one directive declaring a
// 1-entry paths override indexes that directive under exactly 1 glob key,
// not 5.
func TestBuildDirectiveIndex_PerDirectiveOverrideNarrowsToOwnGlob(t *testing.T) {
	pair := fiveGlobSidecar([]sidecar.Directive{
		{
			Text:  "A narrowly-scoped rule. (ref: ADR-901)",
			Paths: []string{"a/**/*.go"},
		},
	})
	idx := BuildDirectiveIndex([]sidecar.Pair{pair})

	matched := 0
	for glob, entries := range idx {
		for _, e := range entries {
			if e.ID == "ADR-901:d01" {
				matched++
				if glob != "a/**/*.go" {
					t.Errorf("overridden directive indexed under unexpected glob %q", glob)
				}
			}
		}
	}
	if matched != 1 {
		t.Fatalf("directive with a 1-entry paths override must index under exactly 1 glob key, indexed under %d", matched)
	}
}

// TestBuildDirectiveIndex_NoOverrideIsByteIdenticalToPreChange is AC-27.2:
// a sidecar where no directive declares its own paths reproduces the exact
// pre-ADR-070 Cartesian-product arithmetic — ADR-040's own measured 5 globs x
// 33 directives = 165 entries, scaled here to 5 globs x 3 directives = 15,
// same multiplication, easier to hand-verify.
func TestBuildDirectiveIndex_NoOverrideIsByteIdenticalToPreChange(t *testing.T) {
	pair := fiveGlobSidecar([]sidecar.Directive{
		{Text: "Rule one. (ref: ADR-901)"},
		{Text: "Rule two. (ref: ADR-901)"},
		{Text: "Rule three. (ref: ADR-901)"},
	})
	idx := BuildDirectiveIndex([]sidecar.Pair{pair})

	if len(idx) != 5 {
		t.Fatalf("want 5 glob keys (unchanged fan-out with no overrides), got %d", len(idx))
	}
	total := 0
	for glob, entries := range idx {
		if len(entries) != 3 {
			t.Errorf("glob %q: want 3 entries (all directives, no override), got %d", glob, len(entries))
		}
		total += len(entries)
	}
	const want = 5 * 3 // ADR-040's own measured shape: globs x directives, exact.
	if total != want {
		t.Fatalf("total entries: want %d (5 globs x 3 directives, ADR-040 arithmetic), got %d", want, total)
	}
}

// TestBuildDirectiveIndex_MixedOverrideAndFallback proves the two behaviors
// coexist correctly within one sidecar: an overridden directive narrows,
// while a sibling with no override still falls back to the full
// sidecar-level glob set exactly as before.
func TestBuildDirectiveIndex_MixedOverrideAndFallback(t *testing.T) {
	pair := fiveGlobSidecar([]sidecar.Directive{
		{Text: "Narrow rule. (ref: ADR-901)", Paths: []string{"a/**/*.go"}},
		{Text: "Broad rule, no override. (ref: ADR-901)"},
	})
	idx := BuildDirectiveIndex([]sidecar.Pair{pair})

	narrowCount, broadCount := 0, 0
	for _, entries := range idx {
		for _, e := range entries {
			switch e.ID {
			case "ADR-901:d01":
				narrowCount++
			case "ADR-901:d02":
				broadCount++
			}
		}
	}
	if narrowCount != 1 {
		t.Errorf("narrowly-scoped directive: want 1 glob, got %d", narrowCount)
	}
	if broadCount != 5 {
		t.Errorf("directive with no override: want all 5 sidecar-level globs (unchanged fallback), got %d", broadCount)
	}
}

// TestResolvePaths_EntryOverrideWinsOverSidecar and its fallback twin pin the
// resolution rule directly, independent of the full index-building path.
func TestResolvePaths_EntryOverrideWinsOverSidecar(t *testing.T) {
	got := resolvePaths([]string{"only/mine.go"}, []string{"a.go", "b.go"})
	if len(got) != 1 || got[0] != "only/mine.go" {
		t.Fatalf("entry override must win over sidecar paths, got %v", got)
	}
}

func TestResolvePaths_FallsBackToSidecarWhenAbsent(t *testing.T) {
	got := resolvePaths(nil, []string{"a.go", "b.go"})
	if len(got) != 2 || got[0] != "a.go" || got[1] != "b.go" {
		t.Fatalf("absent entry override must fall back to sidecar paths unchanged, got %v", got)
	}
}
