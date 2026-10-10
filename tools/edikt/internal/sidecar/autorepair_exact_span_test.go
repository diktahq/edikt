package sidecar

import (
	"strings"
	"testing"
)

// #7 (c3-b): a quote found only by normalised match used to be replaced by
// the WHOLE passage at its new line range. When the matched line carries a
// second sentence, that sentence was copied into the quote too — the anchor
// claimed prose the directive was never grounded in, and every repair grew
// the quote to whole lines. The repair copies the file's own bytes for the
// matched span, and nothing around it.

func TestAutoRepair_NormalisedMatchCopiesExactSpan(t *testing.T) {
	lines := src(
		"# ADR-015",   // 1
		"",            // 2
		"## Decision", // 3
		"",            // 4
		"Hooks MUST construct JSON via a structured",               // 5
		"serializer. Shell concatenation is NEVER permitted here.", // 6
	)
	sc := &Sidecar{
		SchemaVersion: 2,
		Directives: []Directive{{
			Text: "x",
			SourceExcerpts: []SourceExcerpt{{
				LineStart: 2, LineEnd: 3,
				// Extractor joined the wrapped line: found only by normalised match.
				Quote: "Hooks MUST construct JSON via a structured serializer.",
			}},
		}},
	}
	if n := AutoRepairAnchors(sc, lines); n != 1 {
		t.Fatalf("expected 1 repaired anchor, got %d", n)
	}
	se := sc.Directives[0].SourceExcerpts[0]
	if se.LineStart != 5 || se.LineEnd != 6 {
		t.Fatalf("expected anchor 5-6, got %d-%d", se.LineStart, se.LineEnd)
	}
	want := "Hooks MUST construct JSON via a structured\nserializer."
	if se.Quote != want {
		t.Fatalf("the repaired quote must be the exact matched span of the file's bytes\n got: %q\nwant: %q", se.Quote, want)
	}
	if strings.Contains(se.Quote, "Shell concatenation") {
		t.Fatalf("the second sentence on the matched line leaked into the quote: %q", se.Quote)
	}
	if IsStale(sc, lines) {
		t.Fatal("the exact-span quote must be byte-exact at its recorded lines (not stale)")
	}
}

func TestCanonicalSliceInRange_WrappedAndIndented(t *testing.T) {
	passage := "- Hooks MUST construct JSON via a\n  structured serializer, always. Trailing words."
	got, ok := canonicalSliceInRange(passage, "Hooks MUST construct JSON via a structured serializer, always.")
	if !ok {
		t.Fatal("a unique normalised occurrence must map back to a raw span")
	}
	if want := "Hooks MUST construct JSON via a\n  structured serializer, always."; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if !strings.Contains(passage, got) {
		t.Fatalf("the slice must be a byte slice of the passage: %q", got)
	}
}

func TestCanonicalSliceInRange_MultibyteBoundaries(t *testing.T) {
	// Non-ASCII runes on both edges of the span, and a non-breaking space
	// (U+00A0, which strings.Fields treats as whitespace) inside it.
	passage := "Préface — «Les règles doivent  tenir» — après."
	got, ok := canonicalSliceInRange(passage, "«Les règles doivent tenir»")
	if !ok {
		t.Fatal("expected a unique match")
	}
	if want := "«Les règles doivent  tenir»"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if !strings.Contains(passage, got) {
		t.Fatalf("not a byte slice of the passage: %q", got)
	}
}

func TestCanonicalSliceInRange_NotUniqueFallsBack(t *testing.T) {
	passage := "dup rule text.\nfiller\ndup   rule text."
	if got, ok := canonicalSliceInRange(passage, "dup rule text."); ok {
		t.Fatalf("a repeated span is ambiguous and must not be picked: got %q", got)
	}
}

func TestCanonicalSliceInRange_AbsentOrEmpty(t *testing.T) {
	if _, ok := canonicalSliceInRange("Some prose.", "Other prose."); ok {
		t.Fatal("an absent quote has no span")
	}
	if _, ok := canonicalSliceInRange("Some prose.", "   "); ok {
		t.Fatal("an empty quote has no span")
	}
}

// Whatever the repair writes into the quote is a byte slice of the range it
// now points at — the exact span when it is unique, the whole passage when
// it is not.
func TestAutoRepair_RepairedQuoteIsAlwaysAByteSlice(t *testing.T) {
	lines := src(
		"## Decision",            // 1
		"Rules MUST hold. Rules", // 2
		"MUST hold. Rules MUST",  // 3
	)
	sc := &Sidecar{
		SchemaVersion: 2,
		Directives: []Directive{{
			Text: "x",
			SourceExcerpts: []SourceExcerpt{{
				LineStart: 9, LineEnd: 9,
				Quote: "hold. Rules MUST hold.",
			}},
		}},
	}
	AutoRepairAnchors(sc, lines)
	se := sc.Directives[0].SourceExcerpts[0]
	passage := strings.Join(lines[se.LineStart-1:se.LineEnd], "\n")
	if !strings.Contains(passage, se.Quote) {
		t.Fatalf("whatever the repair writes must be a byte slice of the range: %q", se.Quote)
	}
}

func TestCanonicalSliceInRange_InvalidUTF8ByteKeepsOffsets(t *testing.T) {
	passage := "lead \xff rule MUST hold. tail"
	got, ok := canonicalSliceInRange(passage, "rule MUST hold.")
	if !ok || got != "rule MUST hold." {
		t.Fatalf("an invalid byte before the span must not shift its offsets: %q %v", got, ok)
	}
}
