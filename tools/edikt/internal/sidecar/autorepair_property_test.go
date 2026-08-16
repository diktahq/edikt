package sidecar

// autorepair_property_test.go — SPEC-009 Plan A Phase 12 / AC-12.3.
//
// Property-based tests for AutoRepairAnchors. Three properties pinned:
//
//   - shift-by-N: every directive whose anchor has been shifted N lines
//     down is repaired back to the correct line.
//   - quote-absent fallthrough: directives without a SourceExcerpt.Quote
//     are left untouched (no false repairs).
//   - no-op on fresh: AutoRepairAnchors on an already-correct sidecar
//     reports 0 repairs.
//
// Uses math/rand with a fixed seed for determinism — matches the existing
// migration_property_test.go convention.

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// buildDocument synthesises a numLines-line markdown document whose every
// line contains a unique sentinel "ANCHOR-<i>" so quote lookups are exact.
// Returns the document text and its split lines.
func buildDocument(numLines int) (string, []string) {
	var b strings.Builder
	b.WriteString("# Title\n\n")
	for i := 1; i <= numLines; i++ {
		fmt.Fprintf(&b, "Line %d body with ANCHOR-%d sentinel.\n", i, i)
	}
	text := b.String()
	return text, strings.Split(text, "\n")
}

// makeSidecar builds a sidecar with N directives whose anchors point at
// the line containing each ANCHOR-i sentinel. shiftBy applies a uniform
// offset to every LineStart/LineEnd so the anchors no longer match.
func makeSidecar(n int, lineOffsetForAnchorI int, shiftBy int) *Sidecar {
	sc := &Sidecar{
		SchemaVersion: 1,
		Topic:         "test",
		Path:          "docs/architecture/decisions/ADR-XXX.md",
	}
	for i := 1; i <= n; i++ {
		correctLine := i + lineOffsetForAnchorI
		sc.Directives = append(sc.Directives, Directive{
			Text: fmt.Sprintf("Directive %d.", i),
			SourceExcerpt: SourceExcerpt{
				LineStart: correctLine + shiftBy,
				LineEnd:   correctLine + shiftBy,
				Quote:     fmt.Sprintf("ANCHOR-%d", i),
			},
		})
	}
	return sc
}

// TestPropertyAutoRepair pins the three AutoRepairAnchors properties over
// a randomized corpus.
func TestPropertyAutoRepair(t *testing.T) {
	t.Run("shift_by_N_repairs_every_directive", func(t *testing.T) {
		r := rand.New(rand.NewSource(20260523))
		const N = 200
		// Document head: "# Title\n\n" — 2 lines before content starts.
		// Line containing ANCHOR-i is at index (1-indexed): 2 + i.
		const lineOffset = 2

		for trial := 0; trial < N; trial++ {
			numDirectives := 1 + r.Intn(10)        // 1..10 directives
			docLines := numDirectives + r.Intn(20) // doc has at least numDirectives lines
			shift := -3 + r.Intn(7)                // shift ∈ [-3, +3]
			if shift == 0 {
				shift = 1 // ensure we exercise the repair path
			}

			_, lines := buildDocument(docLines)
			sc := makeSidecar(numDirectives, lineOffset, shift)

			repaired := AutoRepairAnchors(sc, lines)
			if repaired != numDirectives {
				t.Errorf("trial %d (n=%d, shift=%+d): repaired=%d, want %d",
					trial, numDirectives, shift, repaired, numDirectives)
			}

			// Every directive's LineStart should now point at the
			// correct line (containing its ANCHOR-i sentinel).
			for i, d := range sc.Directives {
				wantLine := (i + 1) + lineOffset
				if d.SourceExcerpt.LineStart != wantLine {
					t.Errorf("trial %d directive[%d]: LineStart=%d, want %d (shift=%+d)",
						trial, i, d.SourceExcerpt.LineStart, wantLine, shift)
				}
				if d.SourceExcerpt.LineEnd != wantLine {
					t.Errorf("trial %d directive[%d]: LineEnd=%d, want %d (single-line span preserved)",
						trial, i, d.SourceExcerpt.LineEnd, wantLine)
				}
			}

			// Post-condition: sidecar is no longer stale.
			if IsStale(sc, lines) {
				t.Errorf("trial %d: sidecar still stale after repair", trial)
			}
		}
	})

	t.Run("quote_absent_fallthrough", func(t *testing.T) {
		r := rand.New(rand.NewSource(20260524))
		const N = 200
		const lineOffset = 2

		for trial := 0; trial < N; trial++ {
			numDirectives := 1 + r.Intn(10)
			docLines := numDirectives + r.Intn(20)
			_, lines := buildDocument(docLines)
			sc := makeSidecar(numDirectives, lineOffset, 0)

			// Empty out every directive's Quote — the repair has no
			// data to work with and must leave LineStart/LineEnd intact.
			originalAnchors := make([][2]int, len(sc.Directives))
			for i := range sc.Directives {
				originalAnchors[i] = [2]int{
					sc.Directives[i].SourceExcerpt.LineStart,
					sc.Directives[i].SourceExcerpt.LineEnd,
				}
				sc.Directives[i].SourceExcerpt.Quote = ""
			}
			// Also shift them so we can be sure no false repair happened.
			for i := range sc.Directives {
				sc.Directives[i].SourceExcerpt.LineStart += 5
				sc.Directives[i].SourceExcerpt.LineEnd += 5
			}

			repaired := AutoRepairAnchors(sc, lines)
			if repaired != 0 {
				t.Errorf("trial %d: repaired=%d, want 0 (quote-absent)", trial, repaired)
			}
			for i, d := range sc.Directives {
				wantStart := originalAnchors[i][0] + 5
				wantEnd := originalAnchors[i][1] + 5
				if d.SourceExcerpt.LineStart != wantStart {
					t.Errorf("trial %d directive[%d]: LineStart drifted from %d to %d on quote-absent input",
						trial, i, wantStart, d.SourceExcerpt.LineStart)
				}
				if d.SourceExcerpt.LineEnd != wantEnd {
					t.Errorf("trial %d directive[%d]: LineEnd drifted from %d to %d on quote-absent input",
						trial, i, wantEnd, d.SourceExcerpt.LineEnd)
				}
			}
		}
	})

	t.Run("no_op_on_fresh_sidecar", func(t *testing.T) {
		r := rand.New(rand.NewSource(20260525))
		const N = 200
		const lineOffset = 2

		for trial := 0; trial < N; trial++ {
			numDirectives := 1 + r.Intn(10)
			docLines := numDirectives + r.Intn(20)
			_, lines := buildDocument(docLines)
			sc := makeSidecar(numDirectives, lineOffset, 0) // shift=0 → already correct

			repaired := AutoRepairAnchors(sc, lines)
			if repaired != 0 {
				t.Errorf("trial %d (n=%d): repaired=%d, want 0 (fresh sidecar)",
					trial, numDirectives, repaired)
			}
			if IsStale(sc, lines) {
				t.Fatalf("trial %d: fresh sidecar reported stale", trial)
			}
		}
	})

	t.Run("line_secondary_tiebreaker_picks_closest", func(t *testing.T) {
		// Three directives share a common ANCHOR quote at lines 5, 10, 15.
		// Initial LineStart values are 4, 9, 14 (each off-by-one). Repair
		// must route each directive to its nearest match — NOT collapse
		// all three onto the first occurrence (the ADR-038 motivating
		// case from the AutoRepairAnchors comment block).
		lines := make([]string, 20)
		for i := range lines {
			lines[i] = fmt.Sprintf("Plain line %d.", i+1)
		}
		lines[4] = "Shared content with ANCHOR sentinel and tail A."
		lines[9] = "Shared content with ANCHOR sentinel and tail B."
		lines[14] = "Shared content with ANCHOR sentinel and tail C."

		sc := &Sidecar{
			SchemaVersion: 1,
			Topic:         "test",
			Path:          "docs/architecture/decisions/ADR-038.md",
			Directives: []Directive{
				{Text: "d1", SourceExcerpt: SourceExcerpt{LineStart: 4, LineEnd: 4, Quote: "ANCHOR"}},
				{Text: "d2", SourceExcerpt: SourceExcerpt{LineStart: 9, LineEnd: 9, Quote: "ANCHOR"}},
				{Text: "d3", SourceExcerpt: SourceExcerpt{LineStart: 14, LineEnd: 14, Quote: "ANCHOR"}},
			},
		}
		repaired := AutoRepairAnchors(sc, lines)
		if repaired != 3 {
			t.Errorf("repaired=%d, want 3 (all three off-by-one)", repaired)
		}
		want := []int{5, 10, 15}
		for i, d := range sc.Directives {
			if d.SourceExcerpt.LineStart != want[i] {
				t.Errorf("directive[%d]: LineStart=%d, want %d (line-secondary tiebreaker should pick closest match)",
					i, d.SourceExcerpt.LineStart, want[i])
			}
		}
	})
}
