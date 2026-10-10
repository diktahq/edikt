package phasea

// divergence_test.go — the CONTROL for the divergence detector.
//
// A detector that has never been shown to fire is indistinguishable from one
// that cannot. This file stages the measured production case — an artifact
// whose retry gets WORSE — and asserts the run terminates instead of spending
// the rest of its budget.
//
// The staged shape is guideline-event-transport's, verbatim in structure:
//
//	attempt 1   1 over-cap directive   (cap+72)
//	attempt 2   4 over-cap directives  (cap+51 / +164 / +66 / +78)
//	anchors     valid throughout — length was the ONLY defect
//
// The incident's absolute lengths were 572 and 551/664/566/578 against the
// 500 cap of the day. They are expressed as OFFSETS FROM MaxItemTextLen since
// 2026-08-30, when the hard cap moved to 2000: as literals they silently
// stopped being over-cap, and a fixture that no longer reproduces the defect
// makes its test agree with a state it was written to catch. The shape — one
// defect, then four, anchors valid throughout — is the part under test.
//
// max_attempts_per_item is set to 5, NOT the default 2. That is the whole point
// of the control: with a budget of 2, an item stopping after attempt 2 proves
// nothing, because exhaustion would stop it anyway and the two causes are
// indistinguishable. With 5, a stop at attempt 2 can only be divergence.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// overCap builds a directive whose text exceeds MaxItemTextLen by construction,
// anchored at prose that really is in the parent — so the ONLY defect is length.
func overCap(n int, seed string) sidecar.Directive {
	body := seed + " " + strings.Repeat("obligation and consequence, ", 40)
	if len(body) < n {
		body += strings.Repeat("x", n-len(body))
	}
	return anchoredAtGateQuote(body[:n])
}

func TestDivergenceDetectorFires(t *testing.T) {
	f := newFixture(t, "phasea:\n  max_attempts_per_item: 5\n")

	// ── attempt 1: one over-cap directive, anchors valid ──
	f.extractGood(t, overCap(MaxItemTextLen+72, "The service MUST publish every domain event."))
	out1 := f.verify(t)
	it1 := onlyItem(t, out1)

	if it1.Status != StatusLengthExceeded {
		t.Fatalf("attempt 1 status = %q, want %q — a pure length failure must not be "+
			"collapsed into schema-invalid, or a four-sentence split looks like a failed extraction",
			it1.Status, StatusLengthExceeded)
	}
	if it1.DefectAxis != DefectAxisLength || it1.DefectCount != 1 {
		t.Fatalf("attempt 1 defect = %s/%d, want length/1", it1.DefectAxis, it1.DefectCount)
	}
	if it1.AnchorsValid != 1 || it1.AnchorsBroken != 0 {
		t.Errorf("attempt 1 anchors = %d valid / %d broken, want 1/0 — anchor state must be "+
			"recorded even when length already failed", it1.AnchorsValid, it1.AnchorsBroken)
	}
	if it1.Convergence != ConvergenceFirstAttempt {
		t.Errorf("attempt 1 convergence = %q, want %q", it1.Convergence, ConvergenceFirstAttempt)
	}

	// THE ROOT-CAUSE FIX: the diagnostic must be ON THE RECEIPT, not only in
	// memory. Everything downstream — the retry included — reads the receipt.
	if it1.Diagnostic == "" {
		t.Fatal("receipt carries no diagnostic — this is the whole defect: the retry reads the " +
			"receipt, so a diagnostic held only in VerifyOutcome.Details dies with the process " +
			"and the re-dispatch is unseeded")
	}
	if it1.CorrectionPrompt == "" {
		t.Fatal("no correction_prompt persisted — the re-dispatch would resend the original " +
			"prompt verbatim, which is the unseeded retry that produced the measured regression")
	}
	for _, want := range []string{
		fmt.Sprintf("%d", MaxItemTextLen+72), fmt.Sprintf("%d", MaxItemTextLen),
		"CORRECTION PASS", "sidecar-extractor.md:742"} {
		if !strings.Contains(it1.CorrectionPrompt, want) {
			t.Errorf("correction_prompt omits %q — it must name the actual length, the ceiling, "+
				"and the remedy the extractor already carries:\n%s", want, it1.CorrectionPrompt)
		}
	}

	// ── attempt 2: FOUR over-cap directives — strictly worse on the same axis ──
	f.extractGood(t,
		overCap(MaxItemTextLen+51, "The service MUST publish every domain event."),
		overCap(MaxItemTextLen+164, "The consumer MUST acknowledge within the window."),
		overCap(MaxItemTextLen+66, "The transport MUST NOT drop an event silently."),
		overCap(MaxItemTextLen+78, "The outbox MUST be drained in order."),
	)
	out2 := f.verify(t)
	it2 := onlyItem(t, out2)

	if it2.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", it2.Attempts)
	}
	if it2.DefectCount != 4 {
		t.Fatalf("attempt 2 defect count = %d, want 4 (the staged regression)", it2.DefectCount)
	}
	if it2.Convergence != ConvergenceDiverged {
		t.Fatalf("convergence = %q, want %q — 1 defect to 4 on the same axis is divergence, "+
			"and a policy that cannot see it keeps paying for it",
			it2.Convergence, ConvergenceDiverged)
	}

	// THE CONTROL'S POINT: it stopped, and it stopped for the RIGHT REASON.
	if it2.Status != StatusDiverged {
		t.Fatalf("status = %q, want %q — the detector did not fire", it2.Status, StatusDiverged)
	}
	if it2.Status == StatusFailedTerminal {
		t.Fatal("stopped on budget exhaustion, not divergence — with max_attempts 5 this would " +
			"mean the detector never fired and exhaustion merely looked like it did")
	}
	if !IsTerminalStatus(it2.Status) {
		t.Fatal("diverged is not terminal — the receipt would say 'diverged' while the retry " +
			"loop re-dispatched it anyway, making the detector decorative")
	}

	// The exit code must distinguish this from exhaustion. Exit 5's contract
	// says "the retry budget is spent"; telling a caller that about an item
	// with 3 attempts remaining is false, and the two states need opposite
	// responses from a human.
	if out2.ExitCode != ExitVerifyDiverged {
		t.Errorf("exit code = %d, want %d (diverged) — folding this into %d would tell the "+
			"caller the budget was exhausted when it was deliberately left unspent",
			out2.ExitCode, ExitVerifyDiverged, ExitVerifyFailed)
	}

	// Budget deliberately unspent: 3 of 5 attempts remain.
	if !strings.Contains(it2.Diagnostic, "deliberately NOT spent") {
		t.Errorf("diagnostic does not record that the remaining budget was left unspent:\n%s",
			it2.Diagnostic)
	}
}

// A retry that IMPROVES must not be mistaken for divergence, or a seeded
// correction pass would be killed on the attempt where it is working. This is
// the acceptance signal for the fix as a whole.
func TestConvergenceImprovedIsNotStopped(t *testing.T) {
	f := newFixture(t, "phasea:\n  max_attempts_per_item: 5\n")

	f.extractGood(t,
		overCap(MaxItemTextLen+51, "one"), overCap(MaxItemTextLen+164, "two"), overCap(MaxItemTextLen+66, "three"), overCap(MaxItemTextLen+78, "four"))
	if it := onlyItem(t, f.verify(t)); it.DefectCount != 4 {
		t.Fatalf("setup: defect count = %d, want 4", it.DefectCount)
	}

	// The correction pass lands: four over-cap texts become one.
	f.extractGood(t, overCap(MaxItemTextLen+72, "one"))
	it := onlyItem(t, f.verify(t))

	if it.Convergence != ConvergenceImproved {
		t.Fatalf("convergence = %q, want %q — 4 defects to 1 is improvement",
			it.Convergence, ConvergenceImproved)
	}
	if it.Status == StatusDiverged {
		t.Fatal("an improving retry was killed as divergent — the correction pass would never " +
			"be allowed to finish")
	}
	if IsTerminalStatus(it.Status) {
		t.Fatalf("status %q is terminal on an improving retry with 3 attempts left", it.Status)
	}
}

// Different axes are not comparable. Reporting a length count against an anchor
// count as though one were "worse" would fire the detector on noise.
func TestConvergenceUnknownAcrossAxes(t *testing.T) {
	f := newFixture(t, "phasea:\n  max_attempts_per_item: 5\n")

	f.extractGood(t, overCap(MaxItemTextLen+72, "length defect"))
	if it := onlyItem(t, f.verify(t)); it.DefectAxis != DefectAxisLength {
		t.Fatalf("setup: axis = %q, want length", it.DefectAxis)
	}

	// Now fail on anchors instead — two broken anchors, no length problem.
	f.extractGood(t,
		anchoredAtStaleQuote("A short directive. (ref: ADR-900)"),
		anchoredAtStaleQuote("Another short one. (ref: ADR-900)"))
	it := onlyItem(t, f.verify(t))

	if it.DefectAxis != DefectAxisAnchor {
		t.Fatalf("axis = %q, want anchor", it.DefectAxis)
	}
	if it.Convergence != ConvergenceUnknown {
		t.Errorf("convergence = %q, want %q — 1 length defect vs 2 anchor defects is not a "+
			"comparison, and treating it as divergence would fire the detector on noise",
			it.Convergence, ConvergenceUnknown)
	}
	if it.Status == StatusDiverged {
		t.Error("fired the divergence stop on an incomparable axis change")
	}
}

// An item that goes from failing to OK after a seeded correction must be
// recorded as improved. This is the acceptance signal for the whole fix, and it
// does NOT fall out of the axis comparison: a successful attempt has axis
// "none" and the failing one had axis "length", which compares as incomparable.
// Left unhandled, a correction pass that worked perfectly would be filed as
// "unknown" and the fix would look like it had never converged once.
func TestConvergenceReachingOKCountsAsImproved(t *testing.T) {
	f := newFixture(t, "phasea:\n  max_attempts_per_item: 5\n")

	f.extractGood(t, overCap(MaxItemTextLen+72, "The service MUST publish every domain event."))
	if it := onlyItem(t, f.verify(t)); it.Status != StatusLengthExceeded {
		t.Fatalf("setup: status = %q, want length-exceeded", it.Status)
	}

	// The correction pass lands: the over-cap text is split and fits.
	f.extractGood(t)
	it := onlyItem(t, f.verify(t))

	if it.Status != StatusOK {
		t.Fatalf("status = %q, want ok — the corrected sidecar should pass", it.Status)
	}
	if it.Convergence != ConvergenceImproved {
		t.Errorf("convergence = %q, want %q — reaching ok after a failure IS the improvement, "+
			"and recording it as anything else means a working correction pass is never counted",
			it.Convergence, ConvergenceImproved)
	}
}

// A NON-CORRECTABLE AXIS MUST YIELD AN EMPTY CORRECTION PROMPT.
//
// This is the guard that stops a re-dispatch from being counted as "seeded"
// when it carries nothing. Inventing an instruction for an axis with no known
// remedy produces a retry that LOOKS seeded and is not, which is worse than an
// unseeded retry: the metric that exists to catch unseeded retries reports it
// as fixed.
//
// AMENDED. This test originally listed DefectAxisParse among the permanently
// non-correctable axes, on the stated reasoning that a parse failure "has no
// known mechanical remedy" the way length ("split at the seam") and anchor
// ("re-quote that span") do. That reasoning held until the remedy was measured:
// a contract emitted `text:` values as bare scalars, and because every
// directive body ends in a "(ref: ...)" tail carrying a colon-space, a bare
// scalar cannot parse by construction. The parser names the line. That is a
// remedy, and withholding it left the one retry to re-roll the same prompt
// blind.
//
// The GUARD is unchanged and is what this test still protects: a parse error
// that does NOT localise yields "" exactly as before. What changed is that
// "parse" stopped being a permanent classification and became a conditional
// one — correctable when the error says where, non-correctable when it does
// not. The mirror case is TestCorrectionPromptSeedsLocalisedParseFailure.
func TestCorrectionPromptEmptyOnNonCorrectableAxes(t *testing.T) {
	for _, axis := range []string{DefectAxisSchema, DefectAxisNone} {
		rep := DefectReport{Axis: axis, AnchorsValid: 12}
		if got := rep.CorrectionPrompt(); got != "" {
			t.Errorf("axis %q produced a correction prompt %q — a caller could send this and "+
				"count the re-dispatch as seeded when nothing actionable was communicated",
				axis, got)
		}
	}
	// The parse axis keeps the guard wherever the error cannot be located.
	for _, unlocalised := range []string{
		"", "yaml: did not find expected key", "unmarshal error", "line: not a number",
	} {
		rep := DefectReport{Axis: DefectAxisParse, ParseError: unlocalised, AnchorsValid: 12}
		if got := rep.CorrectionPrompt(); got != "" {
			t.Errorf("parse error %q has no line and still produced %q — a correction that "+
				"cannot say WHERE is the case this guard exists for", unlocalised, got)
		}
	}
}

// A LOCALISED PARSE FAILURE MUST SEED THE RETRY.
//
// The mirror of the guard above. Measured field failure: a sidecar with 11 of
// 13 directives intact failed to load because two `text:` values were written
// as bare scalars, and the retry budget was spent re-sending the original
// prompt because this axis returned "".
func TestCorrectionPromptSeedsLocalisedParseFailure(t *testing.T) {
	// The exact string this project's YAML parser produces.
	rep := DefectReport{
		Axis:       DefectAxisParse,
		ParseError: "yaml: line 130: mapping values are not allowed in this context",
	}
	got := rep.CorrectionPrompt()
	if got == "" {
		t.Fatal("a parse error naming a line produced no correction — the retry would " +
			"re-roll the same prompt blind, which is the defect this case removes")
	}
	if !strings.Contains(got, "line 130") {
		t.Errorf("the correction does not name the line the parser reported; an instruction "+
			"that omits WHERE is not actionable: %q", got)
	}
	// It must NOT claim anchors survived. looseParse failed, so nothing in the
	// document was checked, and saying otherwise would report a measurement
	// nobody made (INV-013) inside the instruction itself.
	for _, forbidden := range []string{"anchor(s) quote the parent", "already-valid anchor"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("the correction claims anchors are valid in a document that never "+
				"loaded: %q", got)
		}
	}
}

// The correctable axes must produce a NON-empty, SPECIFIC instruction. The
// mirror of the test above: an empty prompt on a length failure would silently
// turn a correction pass back into the unseeded re-run this whole change exists
// to remove.
func TestCorrectionPromptPopulatedOnCorrectableAxes(t *testing.T) {
	length := DefectReport{
		Axis:          DefectAxisLength,
		LengthDefects: []LengthDefect{{Kind: "directive", Line: 64, Len: 551, ParentLines: "12-14", Head: "The outbox MUST"}},
		AnchorsValid:  83,
	}
	if p := length.CorrectionPrompt(); p == "" {
		t.Error("length axis produced no correction prompt")
	} else {
		// The location is the PARENT range and the text's head, not the
		// sidecar line (64): the extractor never sees the sidecar (D-I).
		for _, want := range []string{"12-14", "The outbox MUST", "551", fmt.Sprintf("%d", MaxItemTextLen), "83"} {
			if !strings.Contains(p, want) {
				t.Errorf("length prompt omits %q — it must name the location, the actual "+
					"length, the ceiling, and the anchors to preserve:\n%s", want, p)
			}
		}
	}

	anchor := DefectReport{Axis: DefectAxisAnchor, AnchorsValid: 80, AnchorsBroken: 3}
	if p := anchor.CorrectionPrompt(); p == "" {
		t.Error("anchor axis produced no correction prompt")
	} else if !strings.Contains(p, "3") || !strings.Contains(p, "83") {
		t.Errorf("anchor prompt does not name how many of how many are broken:\n%s", p)
	}
}

// An item that is NOT ok must never reach a caller with an empty diagnostic AND
// an empty correction prompt on a correctable axis — that combination is what
// made the original retry blind.
func TestNonOKItemAlwaysCarriesADiagnostic(t *testing.T) {
	f := newFixture(t, "phasea:\n  max_attempts_per_item: 5\n")
	f.extractGood(t, overCap(MaxItemTextLen+72, "The service MUST publish every domain event."))
	it := onlyItem(t, f.verify(t))

	if it.Status == StatusOK {
		t.Fatal("fixture did not fail as intended")
	}
	if it.Diagnostic == "" {
		t.Error("a non-ok item reached the receipt with no diagnostic — the retry reads the " +
			"receipt, so this is the blind-retry defect reappearing")
	}
	if it.DefectAxis == DefectAxisLength && it.CorrectionPrompt == "" {
		t.Error("a length failure carries no correction prompt, so its re-dispatch would be " +
			"unseeded while still counting against the retry budget")
	}
}

// THE MEASURED REGRESSION CASE.
//
// guideline-event-transport is the artifact that motivated the whole change:
// four over-cap directives (551/664/566/578) against a 500 ceiling, 83 valid
// anchors, ~280k tokens and ~3.4 hours across a budget of 2, zero output. A
// human fixed it by hand in minutes by splitting each directive at its
// semicolon seam, each half anchored where the original was — which is the
// instruction the receipt now carries.
//
// The property under test is the one the fix promises: a SEEDED retry produces
// AT MOST attempt 1's violation count, never more. Before seeding, this same
// shape went 1 -> 4.
func TestSeededRetryConvergesOnTheMeasuredShape(t *testing.T) {
	f := newFixture(t, "phasea:\n  max_attempts_per_item: 5\n")

	// ── attempt 1: the four over-cap directives, at their real lengths ──
	f.extractGood(t,
		semicolonSeam(MaxItemTextLen+51, "The transport MUST publish every domain event"),
		semicolonSeam(MaxItemTextLen+164, "The consumer MUST acknowledge within the window"),
		semicolonSeam(MaxItemTextLen+66, "The transport MUST NOT drop an event silently"),
		semicolonSeam(MaxItemTextLen+78, "The outbox MUST be drained in order"),
	)
	first := onlyItem(t, f.verify(t))

	if first.Status != StatusLengthExceeded || first.DefectAxis != DefectAxisLength {
		t.Fatalf("attempt 1 = %s/%s, want length-exceeded/length", first.Status, first.DefectAxis)
	}
	if first.DefectCount != 4 {
		t.Fatalf("attempt 1 defect count = %d, want 4 (the measured shape)", first.DefectCount)
	}
	if first.AnchorsBroken != 0 {
		t.Errorf("attempt 1 reports %d broken anchors; the measured case had ALL anchors valid "+
			"and failed only on length", first.AnchorsBroken)
	}

	// The instruction a human would have written, now produced automatically.
	wantLens := fmt.Sprintf("%d/%d/%d/%d", MaxItemTextLen+51, MaxItemTextLen+164,
		MaxItemTextLen+66, MaxItemTextLen+78)
	if !strings.Contains(first.CorrectionPrompt, wantLens) {
		t.Errorf("correction prompt does not name the four actual lengths:\n%s",
			first.CorrectionPrompt)
	}
	// The anchors are named by their parent lines, not by "the same
	// source_excerpts" of a sidecar the extractor never sees (D-I).
	if !strings.Contains(first.CorrectionPrompt, "parent lines 5-5") ||
		!strings.Contains(first.CorrectionPrompt, "each anchored to the parent lines it states") {
		t.Errorf("correction prompt does not tell the retry where each text is grounded, which is "+
			"what makes it a split rather than a re-extraction:\n%s", first.CorrectionPrompt)
	}

	// ── attempt 2: the seeded correction — each split at its semicolon seam ──
	var split []sidecar.Directive
	for _, seed := range []string{
		"The transport MUST publish every domain event",
		"The consumer MUST acknowledge within the window",
		"The transport MUST NOT drop an event silently",
		"The outbox MUST be drained in order",
	} {
		a, b := splitAtSeam(seed)
		split = append(split, anchoredAtGateQuote(a), anchoredAtGateQuote(b))
	}
	f.extractGood(t, split...)
	second := onlyItem(t, f.verify(t))

	// THE PROMISE: at most attempt 1's count, never more.
	if second.DefectCount > first.DefectCount {
		t.Fatalf("a SEEDED retry got WORSE: %d -> %d defects. That is the pre-fix behaviour "+
			"(1 -> 4) reappearing with an instruction attached, which is worse than no "+
			"instruction because the convergence metric would report it as corrected",
			first.DefectCount, second.DefectCount)
	}
	if second.DefectCount != 0 {
		t.Errorf("split directives still over cap: %d remain — the seam split did not fit",
			second.DefectCount)
	}
	if second.Status != StatusOK {
		t.Fatalf("status after the correction = %q, want ok: %s", second.Status, second.Diagnostic)
	}
	if second.Convergence != ConvergenceImproved {
		t.Errorf("convergence = %q, want %q — a correction that resolved every defect must "+
			"register as improvement or the fix's acceptance criterion never fires",
			second.Convergence, ConvergenceImproved)
	}
	if second.Status == StatusDiverged {
		t.Fatal("the divergence detector killed a converging correction pass")
	}
}

// semicolonSeam builds an over-cap directive of exactly n bytes carrying a
// semicolon seam, so a split at that seam yields two halves that each fit.
func semicolonSeam(n int, head string) sidecar.Directive {
	tail := "; and the operator MUST record the outcome in the audit ledger before returning"
	filler := strings.Repeat("with bounded retry and explicit acknowledgement, ", 20)
	body := head + " " + filler
	half := n / 2
	if len(body) < half {
		body += strings.Repeat("x", half-len(body))
	}
	out := body[:half] + tail + " " + strings.Repeat("y", 0)
	if len(out) < n {
		out += strings.Repeat("z", n-len(out))
	}
	return anchoredAtGateQuote(out[:n])
}

// splitAtSeam produces the two atomic statements a human would write, each
// comfortably under the cap. Three obligations in one 664-character sentence
// enforce worse than three atomic ones — which is why the cap is not raised.
func splitAtSeam(head string) (string, string) {
	return head + " with bounded retry and explicit acknowledgement. (ref: ADR-900)",
		"The operator MUST record the outcome in the audit ledger before returning. (ref: ADR-900)"
}
