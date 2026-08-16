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
}

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
	fault := func(reason, actual string, located bool) (AnchorFault, bool) {
		return AnchorFault{
			Kind: kind, Index: idx, Anchor: anchorIdx,
			LineStart: a.LineStart, LineEnd: a.LineEnd,
			Reason: reason, Quote: a.Quote, Actual: actual, Located: located,
		}, true
	}

	if strings.TrimSpace(a.Quote) == "" {
		return fault("quote is empty", "", false)
	}
	if a.LineStart < 1 {
		return fault(fmt.Sprintf("line_start %d is not a 1-indexed line number", a.LineStart), "", false)
	}
	if a.LineEnd < a.LineStart {
		return fault(fmt.Sprintf("line_end %d precedes line_start %d", a.LineEnd, a.LineStart), "", false)
	}
	if a.LineEnd > len(lines) {
		return fault(fmt.Sprintf("line_end %d is past the parent's last line (%d)", a.LineEnd, len(lines)), "", false)
	}

	actual := strings.Join(lines[a.LineStart-1:a.LineEnd], "\n")

	// Byte-exact containment: the quote must be a contiguous slice of the
	// bytes at the recorded range. Containment rather than equality because a
	// directive legitimately quotes one sentence out of a wider line range;
	// byte-exact because at extraction time there is no legitimate source of
	// whitespace difference.
	if strings.Contains(actual, a.Quote) {
		return AnchorFault{}, false
	}

	// Distinguish the two failures that look identical in a bare "not found",
	// because they call for different fixes. An off-by-one is the extractor
	// mis-numbering a correct quote; a whitespace-only difference is the
	// extractor reconstructing a quote instead of copying it.
	if off, dir := offByOne(a, lines); off {
		return fault(fmt.Sprintf(
			"quote is real prose but the recorded range is one line %s — it sits at lines %d-%d",
			dir, a.LineStart+offset(dir), a.LineEnd+offset(dir)), actual, true)
	}
	if strings.Contains(normalizeAnchorWS(actual), normalizeAnchorWS(a.Quote)) {
		return fault(
			"quote matches only after whitespace normalisation — it was reconstructed, not copied "+
				"(a quote that is not a byte slice of the file is a stale anchor from the moment it is written)",
			actual, true)
	}
	return fault("quote does not appear at the recorded lines", actual, true)
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
		if strings.Contains(strings.Join(lines[s-1:e], "\n"), a.Quote) {
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

func normalizeAnchorWS(s string) string { return strings.Join(strings.Fields(s), " ") }

func truncateAnchor(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
