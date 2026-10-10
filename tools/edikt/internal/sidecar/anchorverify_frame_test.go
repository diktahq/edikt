package sidecar

import (
	"strings"
	"testing"
)

// anchorverify_frame_test.go — D361: an anchor is a LOGICAL QUOTE within a
// normalised frame, not a byte range into the parent.
//
// THE DEFECT, measured by bok-services on a real corpus (77 sidecars, 26
// artifacts) 2026-09-04 and re-verified at the code by the coordinator: the
// gate compared the quote against the recorded range byte-for-byte, justified
// by "at extraction time there is no legitimate source of whitespace
// difference". Markdown falsifies that premise. A list item's continuation
// lines carry the item's indentation and a blockquote's lines carry `> `, so
// `actual` holds "\n  " or "\n> " at every interior newline where the quote
// holds "\n" — and the quote is, word for word, prose the parent contains.
//
// THE RULING strips the common leading gutter SHARED BY THE RECORDED RANGE
// from both sides and compares byte-exactly inside that frame. DEDENT IS NOT
// NORMALISE: routing acceptance through normalizeAnchorWS would accept the
// reconstructed quotes that function exists to name, turning a two-way
// diagnostic into a one-way accept. That is what TestVerifyAnchors_
// StillRejectsWrongQuote is for, and it is the clause the other three are
// bought with if nobody writes it.
//
// FALSIFICATION RECORD — 2026-09-07. Each mutation was applied to
// anchorverify.go alone, `go test ./internal/sidecar/ -run TestVerifyAnchors`
// was run, the mutation was reverted, and the file was diffed back to its
// pre-mutation bytes. Every RED below is a transcribed observation, not a
// prediction; where a mutation killed FEWER cases than expected that is
// recorded too, because the point of the record is what it actually kills.
//
//	M1  quoteFoundInRange → plain strings.Contains(actual, quote)
//	    (the pre-ruling gate, restored)
//	    → RED  _IndentedMultiline/multi_line_quote_inside_a_list_item
//	    → RED  _IndentedMultiline/quote_spanning_a_blank_line_inside_a_list_item
//	    → RED  _Blockquote/multi_line_quote_inside_a_blockquote
//
//	M2  stripGutterFrame → per-line dedent (strip each line's OWN gutter run
//	    instead of the run every line shares)
//	    → RED  _StillRejectsWrongQuote/blockquote_marker_invented_on_the_quote_side
//	    NOT red: _FrameIsCommonNotPerLine. Its range shares no gutter, so
//	    quoteFoundInRange returns at the frame == "" guard and never reaches
//	    stripGutterFrame. M7 is the mutation that reaches it.
//
//	M3  quoteFoundInRange → strings.Contains(normalizeAnchorWS(actual),
//	    normalizeAnchorWS(quote)) — dedent replaced by normalise, the one
//	    substitution D361 singles out as gutting the gate
//	    → RED  _StillRejectsWrongQuote/re_wrapped_inside_a_list_item
//	    → RED  _StillRejectsWrongQuote/internal_whitespace_is_not_gutter
//	    → RED  _FrameIsCommonNotPerLine
//	    → RED  TestVerifyAnchorsWhitespaceReconstruction (the pre-existing guard)
//	    → RED  _Blockquote/multi_line_quote_inside_a_blockquote — normalise is
//	           not a superset of dedent: it leaves the `>` markers as tokens,
//	           so it refuses a quote the frame accepts
//
//	M4  anchorGutterChar → drop '>' from the gutter set (whitespace only,
//	    the narrowest literal reading of "common leading whitespace")
//	    → RED  _Blockquote/multi_line_quote_inside_a_blockquote
//
//	M5  quoteFoundInRange → strip the frame from the quote side only
//	    → RED  _IndentedMultiline/multi_line_quote_inside_a_list_item
//	    → RED  _IndentedMultiline/quote_spanning_a_blank_line_inside_a_list_item
//	    → RED  _Blockquote/multi_line_quote_inside_a_blockquote
//
//	M6  anchorGutterFrame → count blank lines when computing the shared run
//	    (a blank line carries no gutter, so every frame collapses to "")
//	    → RED  _IndentedMultiline/quote_spanning_a_blank_line_inside_a_list_item
//
//	M7  anchorGutterFrame → keep the LONGEST run seen instead of the run every
//	    line shares (the frame mutation M2 could not reach)
//	    → RED  _FrameIsCommonNotPerLine

// indentedParentFixture is markdown with the two containers the ruling is
// about. Line numbers are 1-indexed and stated so each case names a range
// without the reader counting.
//
//	 1: # ADR-901 — Indentation is part of the line, not part of the quote
//	 2: (blank)
//	 3: ## Decision
//	 4: (blank)
//	 5: - The gate is byte-exact within a frame:
//	 6:   Body drift reports THAT something changed, never WHAT.
//	 7:   It cannot distinguish an added MUST from a fixed typo.
//	 8: (blank)
//	 9: > The recorded digest MUST NOT be relocated out of the sidecar,
//	10: > because a digest that travels separately is a digest nothing checks.
//	11: (blank)
//	12:   - An indented single-line rule MUST verify exactly as it does today.
//	13: (blank)
//	14: - A rule with a paragraph break:
//	15: (blank)
//	16:   First half of the sentence sits here,
//	17: (blank)
//	18:   and the second half sits after a blank line.
const indentedParentFixture = `# ADR-901 — Indentation is part of the line, not part of the quote

## Decision

- The gate is byte-exact within a frame:
  Body drift reports THAT something changed, never WHAT.
  It cannot distinguish an added MUST from a fixed typo.

> The recorded digest MUST NOT be relocated out of the sidecar,
> because a digest that travels separately is a digest nothing checks.

  - An indented single-line rule MUST verify exactly as it does today.

- A rule with a paragraph break:

  First half of the sentence sits here,

  and the second half sits after a blank line.
`

// acceptedAnchor runs the shipped gate over one anchor and fails loudly with
// the gate's own message, which is the part a reader needs when a legitimate
// quote is refused.
func acceptedAnchor(t *testing.T, a SourceExcerpt) {
	t.Helper()
	v := VerifyAnchors(scWithAnchors(a), indentedParentFixture)
	if v.Anchors != 1 {
		t.Fatalf("examined %d anchors, want 1 — the denominator must reflect what was checked", v.Anchors)
	}
	if !v.OK() {
		for _, f := range v.Faults {
			t.Errorf("a quote the parent contains was rejected: %s", f)
		}
	}
}

// rejectedAnchor asserts the gate refused, and (when wantReason is non-empty)
// that it refused for the stated reason. A rejection for the wrong reason is a
// gate that got the right answer by accident.
func rejectedAnchor(t *testing.T, a SourceExcerpt, wantReason string) {
	t.Helper()
	v := VerifyAnchors(scWithAnchors(a), indentedParentFixture)
	if v.OK() {
		t.Fatal("a quote the parent does not contain was ACCEPTED — the frame has been widened into a normaliser")
	}
	if wantReason != "" && !strings.Contains(v.Faults[0].String(), wantReason) {
		t.Errorf("fault does not name %q as the cause:\n%s", wantReason, v.Faults[0])
	}
}

// TestVerifyAnchors_IndentedMultiline — AC-3.3, clauses 1 and 3: a multi-line
// quote inside a markdown list item verifies, and a single-line quote inside a
// list still verifies exactly as it did before the ruling.
func TestVerifyAnchors_IndentedMultiline(t *testing.T) {
	t.Run("multi line quote inside a list item", func(t *testing.T) {
		// Lines 6-7 carry two spaces of list-item continuation indent; the
		// extractor recorded the sentence without it. This is the exact shape
		// bok hit, and it was RED before D361.
		acceptedAnchor(t, SourceExcerpt{
			LineStart: 6, LineEnd: 7,
			Quote: "Body drift reports THAT something changed, never WHAT.\n" +
				"It cannot distinguish an added MUST from a fixed typo.",
		})
	})

	t.Run("quote spanning a blank line inside a list item", func(t *testing.T) {
		// A blank line carries no gutter. If it counted toward the shared run
		// the frame would collapse to "" and this quote would be refused, so
		// the paragraph-break case is the one that pins blank-line handling.
		acceptedAnchor(t, SourceExcerpt{
			LineStart: 16, LineEnd: 18,
			Quote: "First half of the sentence sits here,\n\n" +
				"and the second half sits after a blank line.",
		})
	})

	t.Run("single line quote inside a list verifies as it does today", func(t *testing.T) {
		q := "An indented single-line rule MUST verify exactly as it does today."
		acceptedAnchor(t, SourceExcerpt{LineStart: 12, LineEnd: 12, Quote: q})

		// "As it does today" is asserted, not asserted-by-implication: raw
		// containment already accepts this, so the frame is inert here and a
		// future change to the frame cannot be what keeps this case green.
		line12 := strings.Split(indentedParentFixture, "\n")[11]
		if !strings.Contains(line12, q) {
			t.Fatalf("fixture drift: line 12 %q no longer contains the single-line quote", line12)
		}
		if got := anchorGutterFrame(line12); got != "  " {
			t.Errorf("frame for line 12 = %q, want %q — the fixture must keep the leading indent this clause is about", got, "  ")
		}
	})
}

// TestVerifyAnchors_Blockquote — AC-3.3, clause 2: a multi-line quote inside a
// blockquote verifies.
//
// This is the clause a whitespace-only frame cannot reach. `> ` begins at
// column 0, so there is no common leading WHITESPACE to strip; the gutter set
// has to include the blockquote marker, which is what D361's own reasoning
// names ("markdown list and blockquote indentation is part of the LINE and not
// part of the QUOTE").
func TestVerifyAnchors_Blockquote(t *testing.T) {
	t.Run("multi line quote inside a blockquote", func(t *testing.T) {
		acceptedAnchor(t, SourceExcerpt{
			LineStart: 9, LineEnd: 10,
			Quote: "The recorded digest MUST NOT be relocated out of the sidecar,\n" +
				"because a digest that travels separately is a digest nothing checks.",
		})
	})

	t.Run("quote that kept the blockquote gutter still verifies", func(t *testing.T) {
		// The extractor is not required to strip the gutter — a byte-exact
		// copy of the range is still a byte-exact copy. Stripping the frame
		// from both sides must not break the case that never needed it.
		acceptedAnchor(t, SourceExcerpt{
			LineStart: 9, LineEnd: 10,
			Quote: "> The recorded digest MUST NOT be relocated out of the sidecar,\n" +
				"> because a digest that travels separately is a digest nothing checks.",
		})
	})
}

// TestVerifyAnchors_StillRejectsWrongQuote — AC-3.3, clause 4, and the clause
// the first three are bought with if it is not written. The gate must not be
// loosened into one that accepts a quote the parent does not contain.
func TestVerifyAnchors_StillRejectsWrongQuote(t *testing.T) {
	t.Run("prose absent from the parent at an indented range", func(t *testing.T) {
		rejectedAnchor(t, SourceExcerpt{
			LineStart: 6, LineEnd: 7,
			Quote: "Body drift reports THAT nothing changed, never WHY.\n" +
				"It cannot distinguish a removed MUST from a fixed typo.",
		}, "quote does not appear at the recorded lines")
	})

	t.Run("re wrapped inside a list item", func(t *testing.T) {
		// The two lines joined with a space instead of the file's newline.
		// Dedent does not reach this: the difference is not the container.
		// If this ever passes, acceptance has been routed through
		// normalizeAnchorWS and the reconstruction diagnostic has become a
		// one-way accept.
		rejectedAnchor(t, SourceExcerpt{
			LineStart: 6, LineEnd: 7,
			Quote: "Body drift reports THAT something changed, never WHAT. " +
				"It cannot distinguish an added MUST from a fixed typo.",
		}, "whitespace normalisation")
	})

	t.Run("internal whitespace is not gutter", func(t *testing.T) {
		// A doubled space in the middle of a line. Only a LEADING run shared
		// by the whole range is frame; everything else is content.
		rejectedAnchor(t, SourceExcerpt{
			LineStart: 6, LineEnd: 7,
			Quote: "Body drift reports THAT something  changed, never WHAT.\n" +
				"It cannot distinguish an added MUST from a fixed typo.",
		}, "whitespace normalisation")
	})

	t.Run("blockquote marker invented on the quote side", func(t *testing.T) {
		// Lines 6-7 are a list item, not a blockquote. Stripping the frame
		// must not let the quote assert a container the parent does not have —
		// which is precisely what a per-line dedent would allow.
		rejectedAnchor(t, SourceExcerpt{
			LineStart: 6, LineEnd: 7,
			Quote: "> Body drift reports THAT something changed, never WHAT.\n" +
				"> It cannot distinguish an added MUST from a fixed typo.",
		}, "quote does not appear at the recorded lines")
	})

	t.Run("real prose recorded at the wrong container", func(t *testing.T) {
		// The quote is genuine prose from lines 6-7, recorded against the
		// blockquote at 9-10. The frame must not make one container's prose
		// verify against another's.
		rejectedAnchor(t, SourceExcerpt{
			LineStart: 9, LineEnd: 10,
			Quote: "Body drift reports THAT something changed, never WHAT.\n" +
				"It cannot distinguish an added MUST from a fixed typo.",
		}, "quote does not appear at the recorded lines")
	})
}

// TestVerifyAnchors_FrameIsCommonNotPerLine pins the BOUNDARY of D361, so the
// boundary is a recorded decision rather than an accident nobody would notice
// being moved.
//
// The ruling strips the run every line of the range SHARES. A range that opens
// on the bullet line itself (`- text`, gutter "") and continues on indented
// lines (gutter "  ") shares nothing, so such a quote is still refused even
// though the parent contains its words. That is not a defect judgement about
// the quote — it is the ruled semantics, and widening it to a per-line dedent
// is the change that would let the previous test's invented `> ` through.
func TestVerifyAnchors_FrameIsCommonNotPerLine(t *testing.T) {
	if got := anchorGutterFrame("- The gate is byte-exact within a frame:\n  Body drift reports THAT something changed, never WHAT."); got != "" {
		t.Errorf("frame = %q, want \"\" — a bullet line and its continuation share no leading gutter", got)
	}
	rejectedAnchor(t, SourceExcerpt{
		LineStart: 5, LineEnd: 7,
		Quote: "The gate is byte-exact within a frame:\n" +
			"Body drift reports THAT something changed, never WHAT.\n" +
			"It cannot distinguish an added MUST from a fixed typo.",
	}, "")
}
