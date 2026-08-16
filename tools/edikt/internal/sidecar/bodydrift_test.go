package sidecar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// THE CASE THIS WHOLE SIGNAL EXISTS FOR.
//
// A governance rule is ADDED to an artifact after extraction. Every recorded
// quote still matches — nothing that was extracted has changed — so anchor
// drift reports clean and compile prints "0 stale". The new rule is never
// extracted and never enforced, and nothing in the pipeline says so.
//
// This test asserts the two signals DISAGREE on that input. If they ever
// agree, the second signal has collapsed into the first and the hole is back.
//
// WHY IT IS SHAPED THIS WAY, because it is the model for the next one:
// it asserts a PROPERTY (these two instruments must diverge on this class of
// input) rather than a VALUE (this call returns that constant). The
// difference is what it fails on. A value test fails when a number moves —
// which is usually a rename, a reformat, or a legitimate change, so it cries
// wolf and eventually gets updated to match whatever the code now does. This
// one fails when the DESIGN degrades: the only way to make it go red is to
// make body drift stop seeing something anchor drift cannot see, which is the
// precise moment the second signal has stopped being a second signal.
//
// It also cannot be satisfied by weakening one side. Deleting body drift
// fails it; making anchor drift fire on added prose fails it; collapsing both
// into one verdict fails it. A test that survives every wrong fix and only
// passes on the right one is doing the work a review would otherwise have to
// do every time this file is touched.
//
// Note the explicit precondition check below. It asserts the fixture still
// has the property the test is ABOUT — that anchor drift really is blind
// here — so that if that ever changes, this reports a broken premise rather
// than silently becoming a test of nothing (GL-002 rule 11: a probe is
// itself an instrument).
func TestAddedProseIsInvisibleToAnchorDriftAndVisibleToBodyDrift(t *testing.T) {
	before := "# INV-999\n\n## Statement\n\nEvery widget MUST be frobnicated.\n"
	recorded := BodyDigest(before)

	after := before + "\nA control with no subject MUST stay silent.\n"

	// Anchor drift: the one recorded quote still matches live prose.
	sc := &Sidecar{Directives: []Directive{{
		Text: "Every widget MUST be frobnicated.",
		SourceExcerpt: SourceExcerpt{
			LineStart: 5, LineEnd: 5,
			Quote: "Every widget MUST be frobnicated.",
		},
	}}}
	if IsStale(sc, strings.Split(after, "\n")) {
		t.Fatal("precondition failed: anchor drift fired on added prose — " +
			"this test's premise is that it does NOT, so the fixture is wrong")
	}

	// Body drift: sees it.
	got := CheckBodyDrift("INV-999", recorded, after)
	if got.Verdict != BodyDrifted {
		t.Fatalf("body drift = %s, want drifted — added prose must be visible to "+
			"the second signal, or it is not a second signal", got.Verdict)
	}
}

// Whitespace normalization: a reflow must NOT read as a content change. A
// digest that fired on re-wrapping would be ignored within a week.
func TestReflowIsNotBodyDrift(t *testing.T) {
	original := "## Statement\n\nEvery widget MUST be frobnicated before shipping.\n"
	reflowed := "## Statement\n\nEvery widget MUST be\nfrobnicated   before shipping.\n"

	got := CheckBodyDrift("INV-999", BodyDigest(original), reflowed)
	if got.Verdict != BodyUnchanged {
		t.Fatalf("body drift = %s on a pure reflow, want unchanged", got.Verdict)
	}
}

// CRLF is a line-ending change, not a content change.
func TestCRLFIsNotBodyDrift(t *testing.T) {
	lf := "## Statement\n\nEvery widget MUST be frobnicated.\n"
	crlf := strings.ReplaceAll(lf, "\n", "\r\n")
	if got := CheckBodyDrift("X", BodyDigest(lf), crlf); got.Verdict != BodyUnchanged {
		t.Fatalf("body drift = %s on CRLF conversion, want unchanged", got.Verdict)
	}
}

// A real word change IS drift — the counterpart to the reflow test. Without
// this, a normalization bug that collapsed everything to "" would make every
// reflow test pass and every drift undetectable.
func TestWordChangeIsBodyDrift(t *testing.T) {
	original := "Every widget MUST be frobnicated.\n"
	changed := "Every widget MUST NOT be frobnicated.\n"
	if got := CheckBodyDrift("X", BodyDigest(original), changed); got.Verdict != BodyDrifted {
		t.Fatalf("body drift = %s on an inverted MUST, want drifted", got.Verdict)
	}
}

// Deletion is drift too. Anchor drift catches deletion of an EXTRACTED quote;
// body drift must catch deletion of anything.
func TestDeletionOfUnextractedProseIsBodyDrift(t *testing.T) {
	original := "Every widget MUST be frobnicated.\n\nAn aside nobody extracted.\n"
	shortened := "Every widget MUST be frobnicated.\n"
	if got := CheckBodyDrift("X", BodyDigest(original), shortened); got.Verdict != BodyDrifted {
		t.Fatalf("body drift = %s on deleted unextracted prose, want drifted", got.Verdict)
	}
}

// INV-013 applied to this signal itself: no baseline is UNMEASURED, never
// "unchanged". A bool return type would have made this state unrepresentable
// and forced it into the clean count.
func TestNoRecordedDigestIsUnmeasuredNotAPass(t *testing.T) {
	got := CheckBodyDrift("X", "", "any body at all")
	if got.Verdict != BodyUnmeasured {
		t.Fatalf("verdict = %s with no recorded digest, want unmeasured — "+
			"absence of a baseline is not evidence of no change", got.Verdict)
	}
	if got.Reason == "" {
		t.Error("unmeasured verdict carries no reason — 'I could not check' " +
			"is only information when it says why")
	}
}

func TestWhitespaceOnlyRecordedDigestIsUnmeasured(t *testing.T) {
	if got := CheckBodyDrift("X", "   \t ", "body"); got.Verdict != BodyUnmeasured {
		t.Fatalf("verdict = %s for a whitespace-only recorded digest, want unmeasured", got.Verdict)
	}
}

// The summary must carry its denominator, or a run over zero artifacts is
// indistinguishable from a clean run over many.
func TestSummaryCarriesDenominatorAndSeparatesUnmeasured(t *testing.T) {
	s := SummarizeBodyDrift([]BodyDriftResult{
		{ArtifactID: "a", Verdict: BodyUnchanged},
		{ArtifactID: "b", Verdict: BodyDrifted},
		{ArtifactID: "c", Verdict: BodyUnmeasured},
		{ArtifactID: "d", Verdict: BodyUnchanged},
	})
	if s.Total != 4 || s.Unchanged != 2 || s.Drifted != 1 || s.Unmeasured != 1 {
		t.Fatalf("summary = %+v, want total 4 / unchanged 2 / drifted 1 / unmeasured 1", s)
	}
	// Unmeasured must never be folded into the clean count.
	if s.Unchanged+s.Drifted == s.Total {
		t.Error("unmeasured was absorbed into another bucket — that is the defect")
	}
	line := s.ReportLine()
	for _, want := range []string{"of 4", "unmeasured", "NOT a pass"} {
		if !strings.Contains(line, want) {
			t.Errorf("report line missing %q:\n%s", want, line)
		}
	}
}

// The zero-input case, per INV-013's own falsifiability recipe: remove every
// input and confirm the control reports that it measured nothing rather than
// reporting success.
func TestZeroArtifactsReportsNothingToCheckNotClean(t *testing.T) {
	line := SummarizeBodyDrift(nil).ReportLine()
	if !strings.Contains(line, "no artifacts to check") {
		t.Fatalf("zero-artifact report line = %q; it must say there was nothing "+
			"to check rather than implying a clean corpus", line)
	}
}

// THE CEILING must be stated in the output, not just in the source comments.
// A reader who takes "body drift: 3" as "3 rules are missing" has been misled
// by a signal that only ever knew "3 bodies changed".
func TestDriftedReportStatesItsCeiling(t *testing.T) {
	line := SummarizeBodyDrift([]BodyDriftResult{
		{ArtifactID: "a", Verdict: BodyDrifted},
	}).ReportLine()
	if !strings.Contains(line, "never WHAT") {
		t.Errorf("drift report does not state that it cannot say WHAT changed:\n%s", line)
	}
	if !strings.Contains(line, "INCOMPLETE") {
		t.Errorf("drift report does not name the meaning (sidecar may be incomplete):\n%s", line)
	}
}

// The two signals must not share a normalization that could disagree. This
// pins them to the same function rather than to equal-looking copies.
func TestBodyNormalizationMatchesAnchorNormalization(t *testing.T) {
	for _, s := range []string{
		"a  b\tc\n\nd", "  leading and trailing  ", "MUST\r\nNOT", "",
	} {
		if NormalizeBody(s) != normalizeWS(s) {
			t.Errorf("NormalizeBody(%q) diverges from anchor drift's normalizeWS — "+
				"two definitions of 'the same body' is one too many", s)
		}
	}
}

func TestDigestIsDeterministic(t *testing.T) {
	body := "## Statement\n\nEvery widget MUST be frobnicated.\n"
	// Two calls separated by other work, compared via variables.
	//
	// This was `if BodyDigest(body) != BodyDigest(body)`, which staticcheck
	// correctly flags (SA4000): identical expressions either side of !=. The
	// compiler is free to fold it, so the assertion could hold without either
	// call running — GL-002 rule 17's species, in a test written to prove
	// determinism. A test of determinism that never re-invokes the function
	// is a test of nothing.
	first := BodyDigest(body)
	_ = BodyDigest("some other body entirely")
	second := BodyDigest(body)
	if first != second {
		t.Fatalf("BodyDigest is not deterministic: %q then %q", first, second)
	}
	if len(BodyDigest(body)) != 64 {
		t.Fatalf("digest length = %d, want 64 hex chars", len(BodyDigest(body)))
	}
}

// An absent JSON field decodes to 0, and 0 here is BodyUnmeasured — which is
// on INV-013's own enumerated list. The string Status field is what consumers
// read; this pins that it is always populated.
func TestStatusStringAlwaysPopulated(t *testing.T) {
	cases := []BodyDriftResult{
		CheckBodyDrift("a", "", "body"),
		CheckBodyDrift("b", BodyDigest("body"), "body"),
		CheckBodyDrift("c", BodyDigest("body"), "other"),
	}
	for _, c := range cases {
		if c.Status == "" {
			t.Errorf("%s: Status string empty — a consumer decoding the enum would "+
				"read an absent field as unmeasured", c.ArtifactID)
		}
		if c.Status != c.Verdict.String() {
			t.Errorf("%s: Status %q disagrees with Verdict %s", c.ArtifactID, c.Status, c.Verdict)
		}
	}
}

// End-to-end round trip: stamp a baseline, then confirm the signal reports
// unchanged — and that appending one line flips it to drifted.
//
// This is the pair the reporting path depends on. Without the second half,
// a StampBodyDigest that wrote a constant would pass the first half forever.
func TestStampThenDetect(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "INV-900.md")
	side := filepath.Join(dir, "INV-900.edikt.yaml")

	body := "# INV-900\n\n## Statement\n\nWidgets MUST be frobnicated.\n"
	if err := os.WriteFile(parent, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	scYAML := "schema_version: 1\ntopic: demo\npath: INV-900.md\nsignals: [demo]\ndirectives: []\n"
	if err := os.WriteFile(side, []byte(scYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	read := os.ReadFile
	write := func(p string, b []byte) error { return os.WriteFile(p, b, 0o644) }

	if err := StampBodyDigest(side, parent, read, write); err != nil {
		t.Fatalf("StampBodyDigest: %v", err)
	}
	sc, err := Load(side)
	if err != nil {
		t.Fatalf("reload after stamp: %v", err)
	}
	if sc.BodyDigest == "" {
		t.Fatal("stamp wrote no body_digest — the baseline was not established")
	}

	// Unchanged parent → unchanged verdict.
	if got := sc.CheckBodyDriftAgainstParent(dir, read); got.Verdict != BodyUnchanged {
		t.Fatalf("verdict = %s immediately after stamping, want unchanged", got.Verdict)
	}

	// Append a rule → drifted. This is the case the whole ADR exists for:
	// no recorded quote is invalidated, so anchor drift stays silent.
	if err := os.WriteFile(parent, []byte(body+"\nSprockets MUST NOT be gilded.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := sc.CheckBodyDriftAgainstParent(dir, read); got.Verdict != BodyDrifted {
		t.Fatalf("verdict = %s after appending a rule, want drifted", got.Verdict)
	}

	// Stamping is idempotent against an unchanged parent.
	if err := StampBodyDigest(side, parent, read, write); err != nil {
		t.Fatalf("re-stamp: %v", err)
	}
	first, _ := Load(side)
	if err := StampBodyDigest(side, parent, read, write); err != nil {
		t.Fatalf("re-stamp 2: %v", err)
	}
	second, _ := Load(side)
	if first.BodyDigest != second.BodyDigest {
		t.Error("StampBodyDigest is not idempotent on an unchanged parent")
	}
}

// An unreadable parent is UNMEASURED, not unchanged — the rule applied to
// this function's own failure path.
func TestUnreadableParentIsUnmeasured(t *testing.T) {
	sc := &Sidecar{Path: "does-not-exist.md", BodyDigest: BodyDigest("x")}
	got := sc.CheckBodyDriftAgainstParent(t.TempDir(), os.ReadFile)
	if got.Verdict != BodyUnmeasured {
		t.Fatalf("verdict = %s for an unreadable parent, want unmeasured", got.Verdict)
	}
	if got.Reason == "" {
		t.Error("unmeasured verdict names no reason")
	}
}
