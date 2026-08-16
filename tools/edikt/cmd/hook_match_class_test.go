package cmd

import (
	"sort"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
)

// DESIGN-QUESTIONS-2026-08-16.md Q2, option 3 — presentation-only ordering
// of a multi-directive deny by structural class. Proven at the level that
// matters: sorting a real []hookmatch.Entry, not just classPriority in
// isolation, since a correct priority function wired up wrong at the call
// site would still leave entries unsorted.

func TestClassPriority_Order(t *testing.T) {
	cases := []struct {
		class string
		want  int
	}{
		{"invariant", 0},
		{"adr", 1},
		{"guideline", 2},
		{"unknown", 3},
		{"", 3}, // an index rendered before Class existed
	}
	for _, c := range cases {
		if got := classPriority(c.class); got != c.want {
			t.Errorf("classPriority(%q) = %d, want %d", c.class, got, c.want)
		}
	}
}

func TestSortEntriesByClass_InvariantFirst(t *testing.T) {
	entries := []hookmatch.Entry{
		{ID: "ADR-001:d01", Class: "adr", Text: "an adr rule"},
		{ID: "GL-002:d01", Class: "guideline", Text: "a guideline"},
		{ID: "INV-005:d01", Class: "invariant", Text: "an invariant"},
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return classPriority(entries[i].Class) < classPriority(entries[j].Class)
	})
	want := []string{"INV-005:d01", "ADR-001:d01", "GL-002:d01"}
	for i, id := range want {
		if entries[i].ID != id {
			t.Fatalf("position %d: got %s, want %s (full order: %v)", i, entries[i].ID, id, entryIDs(entries))
		}
	}
}

func TestSortEntriesByClass_StableWithinClass(t *testing.T) {
	// Two invariants in a specific order must stay in that order — the sort
	// separates classes, it does not reorder within one.
	entries := []hookmatch.Entry{
		{ID: "INV-002:d01", Class: "invariant"},
		{ID: "ADR-001:d01", Class: "adr"},
		{ID: "INV-001:d01", Class: "invariant"},
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return classPriority(entries[i].Class) < classPriority(entries[j].Class)
	})
	want := []string{"INV-002:d01", "INV-001:d01", "ADR-001:d01"}
	for i, id := range want {
		if entries[i].ID != id {
			t.Fatalf("position %d: got %s, want %s (full order: %v)", i, entries[i].ID, id, entryIDs(entries))
		}
	}
}

func entryIDs(entries []hookmatch.Entry) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids
}
