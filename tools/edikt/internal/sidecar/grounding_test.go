package sidecar

import "testing"

var groundingLines = []string{
	"# ADR-001 — Test",                       // 1
	"",                                       // 2
	"## Decision",                            // 3
	"",                                       // 4
	"Services MUST NOT share mutable state.", // 5
	"",                                       // 6
	"A wrapped sentence that the extractor",  // 7
	"joined when it recorded the quote.",     // 8
}

func TestClassifyExcerpt_Verdicts(t *testing.T) {
	cases := []struct {
		name string
		se   SourceExcerpt
		text string
		want ExcerptVerdict
	}{
		{
			name: "exact match at the recorded lines",
			se:   SourceExcerpt{LineStart: 5, LineEnd: 5, Quote: "Services MUST NOT share mutable state."},
			want: ExcerptGrounded,
		},
		{
			name: "match only after whitespace normalization",
			se: SourceExcerpt{LineStart: 7, LineEnd: 8,
				Quote: "A wrapped sentence that the extractor joined when it recorded the quote."},
			want: ExcerptGroundedWhitespace,
		},
		{
			// The case IsStale skips so Phase A stops re-dispatching. For
			// grounding it is the clearest miss there is: the quote came
			// from the directive, not the prose.
			name: "self-quoting fallback at line 1-1",
			se:   SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "Some rule MUST hold."},
			text: "Some rule MUST hold.",
			want: ExcerptSelfQuoting,
		},
		{
			// The case IsStale calls not-stale because there is no anchor
			// to drift. Grounding must not read "nothing to check" as
			// "checked and fine".
			name: "empty quote",
			se:   SourceExcerpt{LineStart: 5, LineEnd: 5, Quote: ""},
			want: ExcerptNoQuote,
		},
		{
			name: "whitespace-only quote",
			se:   SourceExcerpt{LineStart: 5, LineEnd: 5, Quote: "   \n  "},
			want: ExcerptNoQuote,
		},
		{
			name: "lines past the end of the parent",
			se:   SourceExcerpt{LineStart: 90, LineEnd: 95, Quote: "Services MUST NOT share mutable state."},
			want: ExcerptOutOfRange,
		},
		{
			name: "inverted range",
			se:   SourceExcerpt{LineStart: 5, LineEnd: 3, Quote: "Services MUST NOT share mutable state."},
			want: ExcerptOutOfRange,
		},
		{
			name: "zero line_start",
			se:   SourceExcerpt{LineStart: 0, LineEnd: 0, Quote: "Services MUST NOT share mutable state."},
			want: ExcerptOutOfRange,
		},
		{
			// The quote is real prose from the document, but the anchor
			// points at the wrong lines. Grounding is about the anchor,
			// not about the quote existing somewhere.
			name: "real quote, wrong lines",
			se:   SourceExcerpt{LineStart: 3, LineEnd: 3, Quote: "Services MUST NOT share mutable state."},
			want: ExcerptNotFound,
		},
		{
			name: "quote absent from the document entirely",
			se:   SourceExcerpt{LineStart: 5, LineEnd: 5, Quote: "A rule nobody ever wrote."},
			want: ExcerptNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyExcerpt(tc.se, tc.text, groundingLines)
			if got != tc.want {
				t.Errorf("ClassifyExcerpt = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestClassifyExcerpt_GroundedSetIsExactlyTheTwoMatches keeps Grounded()
// from quietly widening. If a future verdict is added and defaults into
// the grounded set, the corpus number moves without anyone deciding it
// should.
func TestClassifyExcerpt_GroundedSetIsExactlyTheTwoMatches(t *testing.T) {
	grounded := map[ExcerptVerdict]bool{
		ExcerptGrounded:           true,
		ExcerptGroundedWhitespace: true,
		ExcerptSelfQuoting:        false,
		ExcerptNoQuote:            false,
		ExcerptOutOfRange:         false,
		ExcerptNotFound:           false,
	}
	for v, want := range grounded {
		if v.Grounded() != want {
			t.Errorf("%q.Grounded() = %v, want %v", v, v.Grounded(), want)
		}
	}
}

// TestClassifyExcerpt_DivergesFromStalenessWhereItMust is the load-bearing
// test for the whole check. Grounding shares excerptStale's matcher but
// answers a different question, and these two inputs are where the answers
// must differ. If grounding ever agreed with IsStale on both, it would be
// a second name for the staleness gate rather than a measurement of
// extraction health.
func TestClassifyExcerpt_DivergesFromStalenessWhereItMust(t *testing.T) {
	noQuote := SourceExcerpt{LineStart: 5, LineEnd: 5, Quote: ""}
	if excerptStale(noQuote, groundingLines) {
		t.Fatal("precondition changed: an empty quote is no longer treated as not-stale")
	}
	if ClassifyExcerpt(noQuote, "", groundingLines).Grounded() {
		t.Error("an excerpt with no quote at all was counted as grounded")
	}

	selfQuote := Directive{
		Text:          "Some rule MUST hold.",
		SourceExcerpt: SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "Some rule MUST hold."},
	}
	if !isDefaultFallbackExcerpt(selfQuote) {
		t.Fatal("precondition changed: the self-quoting shape is no longer recognised")
	}
	if ClassifyExcerpt(selfQuote.SourceExcerpt, selfQuote.Text, groundingLines).Grounded() {
		t.Error("a self-quoting fallback excerpt was counted as grounded")
	}
}
