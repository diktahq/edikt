package sidecar

import "strings"

// Grounding classification for one source_excerpt against its parent .md.
//
// This publishes what excerptStale already decides, plus the two cases
// staleness deliberately treats as "fine" and grounding must not:
//
//   - An EMPTY quote is not stale (there is no anchor to drift against),
//     but it is not grounded either — there is no anchor at all. Counting
//     it as grounded would report a measurement nobody made.
//   - A SELF-QUOTING excerpt (line 1–1, quote == the directive's own text)
//     is skipped by IsStale so Phase A does not dispatch on it forever.
//     For grounding it is the clearest possible miss: the quote did not
//     come from the prose, it came from the directive.
//
// Staleness asks "has the anchor moved?"; grounding asks "is there an
// anchor, and does it point at real prose?". Same matcher, different
// question — which is why the matcher is shared and the verdicts are not.
type ExcerptVerdict string

const (
	// ExcerptGrounded — the quote appears verbatim within line_start..line_end.
	ExcerptGrounded ExcerptVerdict = "grounded"

	// ExcerptGroundedWhitespace — the quote appears only after whitespace
	// normalization. Reported separately from ExcerptGrounded because
	// "verbatim" and "verbatim modulo re-wrapping" are different claims;
	// the compile gate accepts both, a grounding report should say which.
	ExcerptGroundedWhitespace ExcerptVerdict = "grounded_whitespace_normalized"

	// ExcerptSelfQuoting — anchored at line 1–1 with the directive's own
	// text as the quote. The extractor's fallback shape, not an anchor.
	ExcerptSelfQuoting ExcerptVerdict = "self_quoting"

	// ExcerptNoQuote — quote is empty or whitespace only.
	ExcerptNoQuote ExcerptVerdict = "no_quote"

	// ExcerptOutOfRange — line_start/line_end are unusable: non-positive,
	// inverted, or past the end of the parent body.
	ExcerptOutOfRange ExcerptVerdict = "out_of_range"

	// ExcerptNotFound — a real quote and usable lines, but the prose at
	// those lines does not contain it. The anchor points somewhere the
	// quote is not.
	ExcerptNotFound ExcerptVerdict = "not_found"
)

// Grounded reports whether the verdict counts toward the grounded total.
// Whitespace-normalized matches count: the quote is present, the prose was
// re-wrapped. Everything else does not.
func (v ExcerptVerdict) Grounded() bool {
	return v == ExcerptGrounded || v == ExcerptGroundedWhitespace
}

// ClassifyExcerpt classifies one excerpt against the parent's lines.
//
// itemText is the directive's or prohibition's own text, needed only to
// recognise the self-quoting fallback shape. Pass "" when there is none.
//
// The containment test delegates to the same helpers excerptStale uses, so
// the grounding report and the compile staleness gate can never disagree
// about whether a quote matches. Re-implementing the comparison here is
// exactly how two derivations of one fact drift apart.
func ClassifyExcerpt(se SourceExcerpt, itemText string, sourceLines []string) ExcerptVerdict {
	quote := strings.TrimSpace(se.Quote)
	if quote == "" {
		return ExcerptNoQuote
	}
	if itemText != "" && isDefaultFallbackExcerpt(Directive{
		Text:          itemText,
		SourceExcerpt: se,
	}) {
		return ExcerptSelfQuoting
	}
	if se.LineStart < 1 || se.LineEnd < se.LineStart ||
		se.LineStart > len(sourceLines) || se.LineEnd > len(sourceLines) {
		return ExcerptOutOfRange
	}
	passage := strings.Join(sourceLines[se.LineStart-1:se.LineEnd], "\n")
	if strings.Contains(passage, quote) {
		return ExcerptGrounded
	}
	if strings.Contains(normalizeWS(passage), normalizeWS(quote)) {
		return ExcerptGroundedWhitespace
	}
	return ExcerptNotFound
}
