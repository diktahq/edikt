package sidecar

import (
	"strings"
	"testing"
)

// #7 (c3-c, c3 Part 2): a fault used to carry only a Reason sentence, so a
// correction pass could not tell "re-copy these bytes, the words are right"
// from "this quote is spliced" without parsing prose. Each fault now carries
// a Class, and a whitespace-only fault carries the parent's own bytes for the
// span (Canonical) when that span is unique inside the recorded range.
// Acceptance is unchanged: Class and Canonical describe a rejection, they
// never turn one into a pass (ADR-056/087).

const classParent = "# ADR-900\n" + // 1
	"\n" + // 2
	"- Hooks MUST construct JSON via a\n" + // 3
	"  structured serializer, always.\n" + // 4
	"The gate MUST refuse an unsigned payload.\n" + // 5
	"dup rule text.\n" + // 6
	"dup   rule text.\n" + // 7
	"Spliced first half. Middle sentence. Spliced second half.\n" // 8

func classSidecar(anchors ...SourceExcerpt) *Sidecar {
	return &Sidecar{
		SchemaVersion: 2,
		Directives:    []Directive{{Text: "x", SourceExcerpts: anchors}},
	}
}

func TestVerifyAnchors_FaultClassAndCanonical(t *testing.T) {
	lines := strings.Split(classParent, "\n")

	cases := []struct {
		name          string
		anchor        SourceExcerpt
		wantClass     string
		wantCanonical string
	}{
		{"empty quote", SourceExcerpt{LineStart: 5, LineEnd: 5, Quote: "  "}, "empty", ""},
		{"range before line 1", SourceExcerpt{LineStart: 0, LineEnd: 1, Quote: "x"}, "range", ""},
		{"range inverted", SourceExcerpt{LineStart: 5, LineEnd: 4, Quote: "x"}, "range", ""},
		{"range past the end", SourceExcerpt{LineStart: 5, LineEnd: 99, Quote: "x"}, "range", ""},
		{"off by one", SourceExcerpt{LineStart: 4, LineEnd: 4, Quote: "The gate MUST refuse an unsigned payload."}, "off-by-one", ""},
		{"re-wrapped quote", SourceExcerpt{LineStart: 3, LineEnd: 4,
			Quote: "Hooks MUST construct JSON via a structured serializer, always."},
			"whitespace-only", "Hooks MUST construct JSON via a\n  structured serializer, always."},
		{"duplicate span is not canonicalised", SourceExcerpt{LineStart: 6, LineEnd: 7, Quote: "dup rule  text."},
			"whitespace-only", ""},
		{"spliced quote", SourceExcerpt{LineStart: 8, LineEnd: 8, Quote: "Spliced first half. Spliced second half."},
			"not-verbatim", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := VerifyAnchors(classSidecar(tc.anchor), classParent)
			if len(v.Faults) != 1 {
				t.Fatalf("want exactly one fault, got %d: %+v", len(v.Faults), v.Faults)
			}
			f := v.Faults[0]
			if f.Class != tc.wantClass {
				t.Fatalf("class %q, want %q (reason: %s)", f.Class, tc.wantClass, f.Reason)
			}
			if f.Canonical != tc.wantCanonical {
				t.Fatalf("canonical %q, want %q", f.Canonical, tc.wantCanonical)
			}
			if f.Class == "off-by-one" && (f.FoundStart != 5 || f.FoundEnd != 5) {
				t.Errorf("off-by-one must carry the measured range 5-5, got %d-%d", f.FoundStart, f.FoundEnd)
			}
			if f.Canonical == "" {
				return
			}
			// The canonical bytes are a byte slice of the recorded range, so
			// the anchor rewritten with them passes the unchanged gate.
			fixed := tc.anchor
			fixed.Quote = f.Canonical
			if _, bad := verifyOne("directives", 0, 0, fixed, lines); bad {
				t.Fatalf("the canonical bytes %q do not pass verifyOne at %d-%d", f.Canonical, fixed.LineStart, fixed.LineEnd)
			}
		})
	}

	t.Run("an item with no anchors is class empty", func(t *testing.T) {
		v := VerifyAnchors(classSidecar(), classParent)
		if len(v.Faults) != 1 || v.Faults[0].Class != "empty" {
			t.Fatalf("want one empty-class fault, got %+v", v.Faults)
		}
	})

	t.Run("the class does not change acceptance", func(t *testing.T) {
		v := VerifyAnchors(classSidecar(SourceExcerpt{LineStart: 3, LineEnd: 4,
			Quote: "Hooks MUST construct JSON via a structured serializer, always."}), classParent)
		if v.OK() {
			t.Fatal("a re-wrapped quote must stay a fault — the canonical bytes are a remedy, not an acceptance")
		}
	})
}

func TestCanonicalSliceInRange_UniqueOnly(t *testing.T) {
	passage := "dup rule text.\nfiller\ndup   rule text.\nunique  rule\n  text here."
	if got, ok := canonicalSliceInRange(passage, "dup rule text."); ok {
		t.Fatalf("a span occurring twice must not be canonicalised: got %q", got)
	}
	got, ok := canonicalSliceInRange(passage, "unique rule text")
	if !ok || got != "unique  rule\n  text" {
		t.Fatalf("a unique span maps back to its raw bytes: got %q, %v", got, ok)
	}
	// Overlapping occurrences count as two.
	if got, ok := canonicalSliceInRange("a a a", "a a"); ok {
		t.Fatalf("overlapping occurrences are not unique: got %q", got)
	}
}
