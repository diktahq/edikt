package sidecar

// anchorverify.go — the ADR-056 anchor-verification gate.  edikt-guard:allow
//
// WHY THIS IS CODE AND NOT PROMPT TEXT
//
// Three successive revisions of the extractor prompt tried to make anchors
// correct by instruction. The measured anchor error rate across those
// revisions, on the frozen SPEC-011 fixture set:  edikt-guard:allow
//
//	revision 1  1 ungrounded / 203 anchors
//	revision 2  5 ungrounded / 200 anchors
//	revision 3  1 ungrounded / 184 anchors
//
// It never reached zero, and each revision bought less than the one before.
// Line-number accuracy is not a thing a model self-checks reliably — and the
// failure is invisible on inspection, because the quote is real prose from the
// artifact. Only the recorded RANGE is wrong.
//
// Anchor validity is mechanically decidable: the parent file is right there.
// So it is decided, not requested. INV-011's rule applies directly — stat the  edikt-guard:allow
// promised artifact, do not trust the report of success — and for an anchor
// that has to mean comparing it against the prose it claims to quote.
//
// STRICTER THAN DRIFT DETECTION, DELIBERATELY
//
// grounding.ClassifyExcerpt accepts a whitespace-normalised match
// (ExcerptGroundedWhitespace) because prose gets re-wrapped over a document's
// life and an anchor should survive re-wrapping. That leniency is correct
// there and wrong here. At EXTRACTION time the extractor is reading the exact
// bytes it is quoting; a whitespace difference at that moment means the quote
// was reconstructed rather than copied — the "reads the same but is not a byte
// slice" shape that is a stale anchor from the moment it is written.
//
// Two different questions, so two different strictnesses, kept apart on
// purpose (GL-002): this gate asks "did the extractor copy what it claims to
// have copied", drift asks "does the recorded quote still appear in the prose".
//
// WHAT "BYTE-EXACT" IS MEASURED AGAINST — RULED, D361
//
// The paragraph above used to end "at extraction time there is no legitimate
// source of whitespace difference." That premise is FALSE about markdown, and
// markdown is the only format this corpus is written in. A list item's
// continuation lines carry the item's indentation, and a blockquote's lines
// carry `> `; both belong to the LINE and not to the QUOTE. A multi-line
// sentence lifted out of either therefore differs from the recorded range at
// every interior newline — `"\n  "` or `"\n> "` in the file against `"\n"` in
// the quote — while being, word for word, prose the parent does contain.
// Measured by bok-services on a real corpus, 2026-09-04.
//
// The ruling: an anchor is a LOGICAL QUOTE within a normalised frame, not a
// byte range into the parent. Comparison strips the common leading gutter
// SHARED BY THE RECORDED RANGE from both sides, and is byte-exact inside that
// frame.
//
// DEDENT IS NOT NORMALISE, and getting that backwards would gut the gate.
// normalizeAnchorWS still exists and is still deliberately NOT an acceptance
// path: it powers the diagnostic that names a reconstructed quote. Only a
// leading run shared by every non-blank line of the range is removed. Internal
// whitespace, collapsed runs, re-wrapped prose and joined lines stay rejected
// exactly as before, and a quote the parent does not contain is still refused.

import (
	"fmt"
	"strings"
)

// AnchorFault is one anchor that does not verify against the parent prose.
type AnchorFault struct {
	// Kind is "directives" or "prohibitions"; Index is the 0-based position.
	Kind  string
	Index int
	// Anchor is the 0-based position within that item's source_excerpts.
	Anchor int
	// LineStart / LineEnd are the recorded range, reported verbatim so the
	// message names what to look at rather than describing it.
	LineStart int
	LineEnd   int
	// Reason is a single actionable line.
	Reason string
	// Quote and Actual are truncated for the message; Actual is what really
	// sits at the recorded range, which is the piece that makes an off-by-one
	// obvious to a reader.
	Quote  string
	Actual string
	// Located records that the recorded range WAS resolvable against the
	// parent, so Actual is a measurement rather than an absence.
	//
	// Without it, a range pointing at a blank line produced an empty Actual
	// and the message silently dropped the one part a reader needs — the case
	// where the recorded lines are empty is exactly when "what is actually
	// there" is most worth saying.
	Located bool

	// Class names WHICH failure this is, as a value a caller can branch on
	// instead of a sentence it would have to parse: one of the FaultClass*
	// constants. A correction pass needs it because the remedies differ — a
	// whitespace-only quote has the right words and wants the parent's bytes,
	// a not-verbatim one was spliced or invented, an off-by-one has the right
	// quote at the wrong lines (#7).
	Class string

	// Canonical is set only for FaultClassWhitespaceOnly, and only when the
	// quote's words occur exactly once inside the recorded range: the
	// parent's own bytes for that span. It is a byte slice of the range, so
	// an anchor carrying it passes this gate unchanged. When the words occur
	// more than once nothing is guessed and it stays empty. It describes a
	// remedy; it never accepts the anchor — acceptance is unchanged.
	Canonical string

	// FoundStart / FoundEnd are set only for FaultClassOffByOne: the range
	// the quote actually sits at, measured, so a correction can name it.
	FoundStart int
	FoundEnd   int
}

// Fault classes — AnchorFault.Class.
const (
	// FaultClassEmpty: the quote is blank, or the item has no anchors.
	FaultClassEmpty = "empty"
	// FaultClassRange: the recorded line range cannot be resolved against the
	// parent (before line 1, inverted, or past the last line).
	FaultClassRange = "range"
	// FaultClassOffByOne: the quote is byte-exact one line above or below.
	FaultClassOffByOne = "off-by-one"
	// FaultClassWhitespaceOnly: the words match the recorded range only once
	// whitespace is normalised — reconstructed rather than copied.
	FaultClassWhitespaceOnly = "whitespace-only"
	// FaultClassNotVerbatim: the quote is not at the recorded range at all,
	// even normalised — spliced, paraphrased, or from elsewhere.
	FaultClassNotVerbatim = "not-verbatim"
)

func (f AnchorFault) String() string {
	s := fmt.Sprintf("%s[%d].source_excerpts[%d] (lines %d-%d): %s",
		f.Kind, f.Index, f.Anchor, f.LineStart, f.LineEnd, f.Reason)
	if f.Quote != "" {
		s += fmt.Sprintf("\n      recorded quote: %q", truncateAnchor(f.Quote, 120))
	}
	if f.Located {
		if strings.TrimSpace(f.Actual) == "" {
			s += "\n      actual at those lines: (blank)"
		} else {
			s += fmt.Sprintf("\n      actual at those lines: %q", truncateAnchor(f.Actual, 120))
		}
	}
	return s
}

// AnchorVerification is the measured outcome over one sidecar.
//
// Anchors is the denominator. A report of "0 faults" over 0 anchors is not a
// pass, and Verify's caller can tell the difference because the count travels
// with the verdict (INV-013).  edikt-guard:allow
type AnchorVerification struct {
	Items   int
	Anchors int
	Faults  []AnchorFault
}

// OK reports whether every anchor verified.
func (v AnchorVerification) OK() bool { return len(v.Faults) == 0 }

// VerifyAnchors checks every anchor of every directive and prohibition against
// the parent body.
//
// parentBody is the parent .md's raw content. Every fault is collected — the
// gate names each offending anchor rather than stopping at the first, because
// an extractor that got one range wrong has usually got several.
func VerifyAnchors(sc *Sidecar, parentBody string) AnchorVerification {
	lines := strings.Split(strings.ReplaceAll(parentBody, "\r\n", "\n"), "\n")
	v := AnchorVerification{}

	check := func(kind string, idx int, anchors []SourceExcerpt) {
		v.Items++
		if len(anchors) == 0 {
			v.Faults = append(v.Faults, AnchorFault{
				Kind: kind, Index: idx, Anchor: -1,
				Reason: "no anchors — a directive with zero anchors is ungrounded and cannot be drift-checked",
				Class:  FaultClassEmpty,
			})
			return
		}
		for i, a := range anchors {
			v.Anchors++
			if f, bad := verifyOne(kind, idx, i, a, lines); bad {
				v.Faults = append(v.Faults, f)
			}
		}
	}

	for i := range sc.Directives {
		check("directives", i, sc.Directives[i].Anchors())
	}
	for i := range sc.Prohibitions {
		check("prohibitions", i, sc.Prohibitions[i].Anchors())
	}
	return v
}

func verifyOne(kind string, idx, anchorIdx int, a SourceExcerpt, lines []string) (AnchorFault, bool) {
	// located=false for faults raised BEFORE the range was resolved (a range
	// that cannot be resolved has no "actual" to report); true afterwards.
	fault := func(class, reason, actual string, located bool) (AnchorFault, bool) {
		return AnchorFault{
			Kind: kind, Index: idx, Anchor: anchorIdx,
			LineStart: a.LineStart, LineEnd: a.LineEnd,
			Reason: reason, Quote: a.Quote, Actual: actual, Located: located,
			Class: class,
		}, true
	}

	if strings.TrimSpace(a.Quote) == "" {
		return fault(FaultClassEmpty, "quote is empty", "", false)
	}
	if a.LineStart < 1 {
		return fault(FaultClassRange, fmt.Sprintf("line_start %d is not a 1-indexed line number", a.LineStart), "", false)
	}
	if a.LineEnd < a.LineStart {
		return fault(FaultClassRange, fmt.Sprintf("line_end %d precedes line_start %d", a.LineEnd, a.LineStart), "", false)
	}
	if a.LineEnd > len(lines) {
		return fault(FaultClassRange, fmt.Sprintf("line_end %d is past the parent's last line (%d)", a.LineEnd, len(lines)), "", false)
	}

	actual := strings.Join(lines[a.LineStart-1:a.LineEnd], "\n")

	// Byte-exact containment within the range's gutter frame (D361).
	// Containment rather than equality because a directive legitimately quotes
	// one sentence out of a wider line range; byte-exact inside the frame
	// because the only whitespace an extractor may legitimately drop is the
	// markdown container gutter the range shares.
	if quoteFoundInRange(actual, a.Quote) {
		return AnchorFault{}, false
	}

	// Distinguish the two failures that look identical in a bare "not found",
	// because they call for different fixes. An off-by-one is the extractor
	// mis-numbering a correct quote; a whitespace-only difference is the
	// extractor reconstructing a quote instead of copying it.
	if off, dir := offByOne(a, lines); off {
		f, bad := fault(FaultClassOffByOne, fmt.Sprintf(
			"quote is real prose but the recorded range is one line %s — it sits at lines %d-%d",
			dir, a.LineStart+offset(dir), a.LineEnd+offset(dir)), actual, true)
		f.FoundStart, f.FoundEnd = a.LineStart+offset(dir), a.LineEnd+offset(dir)
		return f, bad
	}
	if strings.Contains(normalizeAnchorWS(actual), normalizeAnchorWS(a.Quote)) {
		f, bad := fault(FaultClassWhitespaceOnly,
			"quote matches only after whitespace normalisation — it was reconstructed, not copied "+
				"(a quote that is not a byte slice of the file is a stale anchor from the moment it is written)",
			actual, true)
		// The parent's own bytes for the span, when they can be named without
		// guessing. Shared with autorepair's exact-span copy (drift.go), so
		// the two never disagree about what "the span" is.
		if span, ok := canonicalSliceInRange(actual, a.Quote); ok {
			f.Canonical = span
		}
		return f, bad
	}
	return fault(FaultClassNotVerbatim, "quote does not appear at the recorded lines", actual, true)
}

// offByOne reports whether the quote sits exactly one line above or below the
// recorded range — the dominant observed failure, and the one whose message is
// most useful when it names the direction.
func offByOne(a SourceExcerpt, lines []string) (bool, string) {
	for _, cand := range []struct {
		delta int
		dir   string
	}{{-1, "one"}, {1, "one"}} {
		s, e := a.LineStart+cand.delta, a.LineEnd+cand.delta
		if s < 1 || e > len(lines) || e < s {
			continue
		}
		if quoteFoundInRange(strings.Join(lines[s-1:e], "\n"), a.Quote) {
			if cand.delta < 0 {
				return true, "too high"
			}
			return true, "too low"
		}
	}
	return false, ""
}

func offset(dir string) int {
	if strings.Contains(dir, "too high") {
		return -1
	}
	return 1
}

// quoteFoundInRange is THE acceptance predicate for an anchor (D361): the
// quote must be a contiguous byte slice of the recorded range, once the
// gutter the range shares has been stripped from both sides.
//
// Raw containment is tried first and unchanged, so nothing that verified
// before this ruling stops verifying — the frame only ever adds acceptances,
// and only ones where the difference is entirely the markdown container the
// quote was lifted out of.
func quoteFoundInRange(actual, quote string) bool {
	if strings.Contains(actual, quote) {
		return true
	}
	frame := anchorGutterFrame(actual)
	if frame == "" {
		// Nothing shared to strip. Falling through here is what keeps the
		// reconstruction diagnostic reachable for un-indented prose.
		return false
	}
	return strings.Contains(stripGutterFrame(actual, frame), stripGutterFrame(quote, frame))
}

// anchorGutterChar reports whether r is markdown container gutter: indentation
// or a blockquote marker.
//
// `>` is in the set because D361's own reasoning names blockquotes alongside
// list indentation — "markdown list and blockquote indentation is part of the
// LINE and not part of the QUOTE" — and a whitespace-only frame cannot see a
// blockquote at all, since `> ` begins at column 0. Nothing else joins the
// set: a list MARKER (`-`, `*`, `1.`) is deliberately excluded, because
// stripping it would let the frame consume prose rather than container.
func anchorGutterChar(r byte) bool { return r == ' ' || r == '\t' || r == '>' }

// anchorGutterFrame returns the longest leading run of gutter characters
// shared by EVERY non-blank line of the recorded range.
//
// Common, not per-line: a per-line dedent would strip indentation that differs
// between lines, which is a real difference between the quote and the file and
// must stay a fault. A blank line carries no gutter and would collapse every
// frame to "", so blank lines are not counted — they are also the lines a
// quote can never disagree about.
//
// THE BOUNDARY THIS DRAWS, stated because an unrecorded semantics limit is one
// nothing will notice being moved: a range that OPENS on the bullet line
// itself shares nothing (`- text` has gutter "", its continuations have "  "),
// so a quote spanning the marker line and its continuations is still refused
// even though the parent contains its words. That is the ruled semantics, not
// an oversight — widening it to a per-line dedent is what would let a quote
// assert a container the parent does not have. Pinned by
// TestVerifyAnchors_FrameIsCommonNotPerLine; reopening it is a ruling, not a
// patch.
func anchorGutterFrame(rangeText string) string {
	frame := ""
	first := true
	for _, line := range strings.Split(rangeText, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := 0
		for n < len(line) && anchorGutterChar(line[n]) {
			n++
		}
		run := line[:n]
		if first {
			frame, first = run, false
			continue
		}
		if len(run) < len(frame) {
			frame = frame[:len(run)]
		}
		for i := 0; i < len(frame); i++ {
			if frame[i] != run[i] {
				frame = frame[:i]
				break
			}
		}
		if frame == "" {
			return ""
		}
	}
	return frame
}

// stripGutterFrame removes frame from the front of every line that carries it.
//
// "that carries it" rather than "every line": the quote side normally arrives
// with the gutter already absent, and the range side always carries it, so the
// same function has to serve both without inventing bytes on either.
func stripGutterFrame(s, frame string) string {
	if frame == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, frame)
	}
	return strings.Join(lines, "\n")
}

func normalizeAnchorWS(s string) string { return strings.Join(strings.Fields(s), " ") }

func truncateAnchor(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
