package phaseb

import "testing"

// resolvePaths is the per-directive path override rule (ADR-070 Phase 27,
// PRD-010): a directive that names its own paths: is scoped to those globs
// alone; one that does not inherits the sidecar's. It once fed the write-time
// index; since ADR-083 it feeds the delivery map, and the rule is unchanged.

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
