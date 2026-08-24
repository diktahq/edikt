package phaseb

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// ADR-072 (amends ADR-066) Phase 29 — a directive narrowed by its OWN paths
// override (ADR-070) renders a compact digest into its topic file's ambient
// body, IN ADDITION TO full directive-index.yaml delivery, not instead of
// it. A directive relying only on the sidecar-level paths (no override)
// stays index-only, unchanged from ADR-066 — no regression to full
// pre-ADR-066 duplication for the rest of the corpus.

// mkScopedPairWithOverride is mkScopedPair's sibling: the sidecar declares 5
// broad sidecar-level globs (so the "no override" directive/prohibition
// still resolve against a real multi-glob set, matching ADR-070's own
// fixture shape), and the FIRST directive/prohibition carries its own
// narrower paths override.
func mkScopedPairWithOverride(t *testing.T, projectRoot, basename, topic string) sidecar.Pair {
	t.Helper()
	p := mkPair(t, projectRoot, basename, topic, nil)
	p.Sidecar.Paths = []string{"a/**/*.go", "b/**/*.go", "c/**/*.go", "d/**/*.go", "e/**/*.go"}
	p.Sidecar.Directives = []sidecar.Directive{
		{
			Text:          "Narrowly-scoped directive body. (ref: " + basename[:7] + ")",
			SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"},
			Paths:         []string{"a/**/*.go"},
		},
		{
			Text:          "Broadly-scoped directive body, no override. (ref: " + basename[:7] + ")",
			SourceExcerpt: sidecar.SourceExcerpt{LineStart: 2, LineEnd: 2, Quote: "z"},
		},
	}
	p.Sidecar.Prohibitions = []sidecar.Prohibition{
		{
			Text:          "MUST NOT do the narrowly-scoped thing. (ref: " + basename[:7] + ")",
			SourceExcerpt: sidecar.SourceExcerpt{LineStart: 3, LineEnd: 3, Quote: "y"},
			Paths:         []string{"a/**/*.go"},
		},
	}
	return p
}

// TestMerge_NarrowedDirectiveRendersDigestAndStaysIndexed is AC-29.1/AC-29.2:
// the directive with its own paths override renders into the topic file's
// body AND still appears in directive-index.yaml — dual channel confirmed.
func TestMerge_NarrowedDirectiveRendersDigestAndStaysIndexed(t *testing.T) {
	root := t.TempDir()
	pairs := []sidecar.Pair{mkScopedPairWithOverride(t, root, "ADR-001-test", "architecture")}

	if _, err := Merge(root, pairs, Options{
		CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test",
	}); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	body := readFileString(t, filepath.Join(root, ".claude", "rules", "governance", "architecture.md"))
	if !strings.Contains(body, "Narrowly-scoped directive body") {
		t.Errorf("topic file body missing the narrowed directive's digest:\n%s", body)
	}
	if !strings.Contains(body, "MUST NOT do the narrowly-scoped thing") {
		t.Errorf("topic file body missing the narrowed prohibition's digest:\n%s", body)
	}

	idx := BuildDirectiveIndex(pairs)
	found := false
	for glob, entries := range idx {
		for _, e := range entries {
			if strings.Contains(e.Text, "Narrowly-scoped directive body") {
				found = true
				if glob != "a/**/*.go" {
					t.Errorf("narrowed directive indexed under unexpected glob %q", glob)
				}
			}
		}
	}
	if !found {
		t.Fatal("narrowed directive dropped from directive-index.yaml — dual channel requires BOTH surfaces, not just the topic file")
	}
}

// TestMerge_UnNarrowedDirectiveStaysIndexOnly is AC-29.3: the sibling
// directive with no override — relying on the sidecar-level paths — must NOT
// appear in the topic file body. No regression to full pre-ADR-066
// duplication for the rest of the corpus.
func TestMerge_UnNarrowedDirectiveStaysIndexOnly(t *testing.T) {
	root := t.TempDir()
	pairs := []sidecar.Pair{mkScopedPairWithOverride(t, root, "ADR-001-test", "architecture")}

	if _, err := Merge(root, pairs, Options{
		CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test",
	}); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	body := readFileString(t, filepath.Join(root, ".claude", "rules", "governance", "architecture.md"))
	if strings.Contains(body, "Broadly-scoped directive body") {
		t.Errorf("un-narrowed directive (no paths override) leaked into the topic file body — ADR-066's index-only delivery must still hold for it:\n%s", body)
	}

	idx := BuildDirectiveIndex(pairs)
	globCount := 0
	for _, entries := range idx {
		for _, e := range entries {
			if strings.Contains(e.Text, "Broadly-scoped directive body") {
				globCount++
			}
		}
	}
	if globCount != 5 {
		t.Fatalf("un-narrowed directive: want all 5 sidecar-level globs in the index (unchanged fallback), got %d", globCount)
	}
}

// TestMerge_NarrowedEntryClearsTheEmptyByDesignNote is AC-29.5: once a topic
// has at least one narrowed entry rendering into its body, the "empty by
// design" note (which only fires when the Directives region is genuinely
// empty — deliveryNoteFor's own len(directiveLines) > 0 guard) must not
// render a stale claim of emptiness alongside real content.
func TestMerge_NarrowedEntryClearsTheEmptyByDesignNote(t *testing.T) {
	root := t.TempDir()
	pairs := []sidecar.Pair{mkScopedPairWithOverride(t, root, "ADR-001-test", "architecture")}

	if _, err := Merge(root, pairs, Options{
		CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test",
	}); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	body := readFileString(t, filepath.Join(root, ".claude", "rules", "governance", "architecture.md"))
	if strings.Contains(body, "empty by design") {
		t.Errorf("delivery note claims the Directives region is empty by design, but it contains a narrowed entry's digest:\n%s", body)
	}
}
