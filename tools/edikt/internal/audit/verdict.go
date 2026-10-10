package audit

import (
	"fmt"
	"sort"
	"strings"
)

// A PRONG VERDICT IS NOT A VERDICT WITHOUT THE SENTENCE THAT SATISFIED IT.
//
// A clean-context auditor passed a draft on the real-alternative prong whose
// two rejected alternatives were "keep relying on the tool" and "keep auditing
// by hand". Both are do-nothing variants: the absence of a design, not a
// competing one. The auditor read the section's presence, which is what a
// checklist gate does unless the evidence is part of the answer.
//
// So a verdict carries the quote or it fails. Directives already carry anchors
// for exactly this reason: a claim about a document that does not quote the
// document is a claim without evidence.

// Verdict is one prong's outcome.
type Verdict string

const (
	// VerdictPass — satisfied, WITH the quote that satisfies it.
	VerdictPass Verdict = "pass"
	// VerdictFail — not satisfied, or satisfied only on the section's presence.
	VerdictFail Verdict = "fail"
	// VerdictUnmeasured — the prong could not be evaluated. Never a pass: a
	// prong nobody could judge is not a prong that held.
	VerdictUnmeasured Verdict = "unmeasured"
)

// ProngResult is what the auditor reports per prong.
type ProngResult struct {
	// Prong names what was tested, e.g. "Q2 prong 1 — a real rejected
	// alternative existed".
	Prong string `json:"prong"`
	// Subject is the thing the prong was tested against: the option's name,
	// the section's heading.
	Subject string `json:"subject,omitempty"`
	// Quote is the sentence from the artifact that satisfies the prong,
	// verbatim. Empty is a failure, not a formatting choice.
	Quote string `json:"quote,omitempty"`
	// Verdict is the outcome. Derived — never authored directly — by
	// GradeProng, so "pass with no quote" cannot be expressed.
	Verdict Verdict `json:"verdict"`
	// Reason says why, in the failing and unmeasured cases.
	Reason string `json:"reason,omitempty"`
}

// doNothingShapes are the openings of an option that describes NOT building
// something. An option the auditor cannot describe as a build does not count
// toward the decision prong: "we considered continuing to rely on the tool" is
// the status quo wearing an alternative's clothes.
var doNothingShapes = []string{
	"keep ", "keeping ", "continue ", "continuing ", "carry on ",
	"do nothing", "leave it", "leaving it", "status quo", "no change",
	"stay with", "staying with", "as today", "as now", "unchanged",
}

// GradeProng is the ONLY way to produce a ProngResult, and it is why a pass
// without a quote cannot exist: the verdict is derived from the evidence
// rather than asserted beside it.
//
// claimed is what the reader of the artifact believes the prong's answer is;
// quote is the sentence they are resting that on. A claimed pass with no quote
// is a fail, with the reason saying so.
func GradeProng(prong, subject, quote string, claimed bool) ProngResult {
	r := ProngResult{Prong: prong, Subject: subject, Quote: strings.TrimSpace(quote)}
	switch {
	case !claimed:
		r.Verdict = VerdictFail
		r.Reason = "the prong was not satisfied"
	case r.Quote == "":
		r.Verdict = VerdictFail
		r.Reason = "claimed satisfied with NO quote — a prong that cites nothing is satisfied by the " +
			"section's presence, which is how a draft whose alternatives were all do-nothing variants passed"
	default:
		r.Verdict = VerdictPass
	}
	return r
}

// GradeDecisionProng is Q2's first prong: a REAL rejected alternative existed.
// It requires the quote AND that the option describes something that would
// have been built. An option whose text is a do-nothing variant fails, with
// the failing prong named.
func GradeDecisionProng(option, quote string) ProngResult {
	r := GradeProng("Q2 prong 1 — a real rejected alternative existed", option, quote, true)
	if r.Verdict != VerdictPass {
		return r
	}
	if isDoNothing(option) || isDoNothing(r.Quote) {
		return ProngResult{
			Prong:   r.Prong,
			Subject: option,
			Quote:   r.Quote,
			Verdict: VerdictFail,
			Reason: "the option describes NOT building something — a do-nothing variant is the absence of a " +
				"design, not a competing one. State what would have been built instead, or accept that this " +
				"is a fact rather than a decision",
		}
	}
	return r
}

func isDoNothing(s string) bool {
	t := strings.ToLower(strings.TrimSpace(s))
	t = strings.TrimPrefix(t, "we ")
	t = strings.TrimPrefix(t, "considered ")
	for _, shape := range doNothingShapes {
		if strings.HasPrefix(t, shape) || strings.Contains(t, " "+strings.TrimSpace(shape)+" ") {
			return true
		}
	}
	return false
}

// Report is the auditor's deterministic half: every prong it graded, and the
// class it concluded.
type Report struct {
	Artifact string        `json:"artifact"`
	Class    string        `json:"class"`
	Prongs   []ProngResult `json:"prongs"`
}

// AnyFailed reports whether a prong failed — the class verdict must not be
// stronger than its evidence.
func (r Report) AnyFailed() bool {
	for _, p := range r.Prongs {
		if p.Verdict != VerdictPass {
			return true
		}
	}
	return false
}

// FailingProngs names the prongs that did not pass, in a stable order, so a
// report says WHICH prong decided the class.
func (r Report) FailingProngs() []string {
	var out []string
	for _, p := range r.Prongs {
		if p.Verdict != VerdictPass {
			out = append(out, fmt.Sprintf("%s (%s)", p.Prong, p.Verdict))
		}
	}
	sort.Strings(out)
	return out
}
