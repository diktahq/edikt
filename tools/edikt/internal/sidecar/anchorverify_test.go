package sidecar

import (
	"strings"
	"testing"
)

// parentFixture is the prose every case in this file anchors against. Line
// numbers are 1-indexed and stated here so each case can name a range without
// the reader counting.
//
//	1: # ADR-900 — Example
//	2: (blank)
//	3: ## Decision
//	4: (blank)
//	5: Body drift reports THAT something changed, never WHAT.
//	6: It cannot distinguish an added MUST from a fixed typo, and it MUST NOT try.
//	7: (blank)
//	8: The recorded digest MUST NOT be relocated out of the sidecar.
const parentFixture = `# ADR-900 — Example

## Decision

Body drift reports THAT something changed, never WHAT.
It cannot distinguish an added MUST from a fixed typo, and it MUST NOT try.

The recorded digest MUST NOT be relocated out of the sidecar.`

func scWithAnchors(anchors ...SourceExcerpt) *Sidecar {
	return &Sidecar{
		SchemaVersion: 2, Topic: "compile", Path: "docs/x.md",
		Directives: []Directive{{
			Text:           "Body drift MUST report THAT something changed and never WHAT. (ref: ADR-900)",
			SourceExcerpts: anchors,
		}},
	}
}

// TestVerifyAnchorsIsolation is the control that keeps every case below
// meaningful: a CORRECT sidecar must produce zero faults.
//
// Without it, a verifier that rejected everything would pass every sensitivity
// case in this file and the suite would read as thorough while proving only
// that the gate says no.
func TestVerifyAnchorsIsolation(t *testing.T) {
	sc := scWithAnchors(
		SourceExcerpt{LineStart: 5, LineEnd: 5, Quote: "Body drift reports THAT something changed, never WHAT."},
		SourceExcerpt{LineStart: 6, LineEnd: 6, Quote: "It cannot distinguish an added MUST from a fixed typo, and it MUST NOT try."},
	)
	v := VerifyAnchors(sc, parentFixture)
	if !v.OK() {
		for _, f := range v.Faults {
			t.Errorf("unexpected fault: %s", f)
		}
	}
	if v.Anchors != 2 {
		t.Errorf("examined %d anchors, want 2 — the denominator must reflect what was checked", v.Anchors)
	}
	if v.Items != 1 {
		t.Errorf("examined %d items, want 1", v.Items)
	}
}

// TestVerifyAnchorsMultiLineSpanAccepted pins that a quote spanning a real
// multi-line range verifies. Multi-line ranges are where every measured
// off-by-one occurred, so a gate that simply rejected them would score well on
// the failure cases and break legitimate extraction.
func TestVerifyAnchorsMultiLineSpanAccepted(t *testing.T) {
	sc := scWithAnchors(SourceExcerpt{
		LineStart: 5, LineEnd: 6,
		Quote: "Body drift reports THAT something changed, never WHAT.\nIt cannot distinguish an added MUST from a fixed typo, and it MUST NOT try.",
	})
	if v := VerifyAnchors(sc, parentFixture); !v.OK() {
		for _, f := range v.Faults {
			t.Errorf("multi-line span rejected: %s", f)
		}
	}
}

// TestVerifyAnchorsOffByOne is the SENSITIVITY case for the dominant measured
// failure: the quote is real prose from the artifact, and only the recorded
// range is wrong. It is invisible on inspection, which is exactly why it needs
// a mechanical check.
//
// The assertion pins the DIRECTION in the message. A gate that detected the
// fault but reported a generic "not found" would leave the reader doing the
// diagnosis the gate had already done.
func TestVerifyAnchorsOffByOne(t *testing.T) {
	for _, tc := range []struct {
		name    string
		anchor  SourceExcerpt
		wantDir string
	}{
		{"recorded one line too high", SourceExcerpt{LineStart: 6, LineEnd: 6,
			Quote: "Body drift reports THAT something changed, never WHAT."}, "too high"},
		{"recorded one line too low", SourceExcerpt{LineStart: 4, LineEnd: 4,
			Quote: "Body drift reports THAT something changed, never WHAT."}, "too low"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := VerifyAnchors(scWithAnchors(tc.anchor), parentFixture)
			if v.OK() {
				t.Fatal("off-by-one anchor accepted — the quote is real prose, so nothing else would catch this")
			}
			msg := v.Faults[0].String()
			if !strings.Contains(msg, tc.wantDir) {
				t.Errorf("fault does not name the direction (%s):\n%s", tc.wantDir, msg)
			}
			// The actual lines must be in the message: an anchor error cannot
			// be seen from the quote alone, because the quote is correct.
			if !strings.Contains(msg, "actual at those lines") {
				t.Errorf("fault does not show what is actually at the recorded range:\n%s", msg)
			}
		})
	}
}

// TestVerifyAnchorsWhitespaceReconstruction is the case that separates this
// gate from drift detection. ClassifyExcerpt ACCEPTS a whitespace-normalised
// match, because prose gets re-wrapped over a document's life. At extraction
// time there is no such excuse: the extractor is reading the exact bytes, so a
// whitespace difference means the quote was reconstructed rather than copied.
//
// If this ever starts passing, the gate has collapsed into drift detection and
// the strictness the generation boundary needs is gone.
func TestVerifyAnchorsWhitespaceReconstruction(t *testing.T) {
	sc := scWithAnchors(SourceExcerpt{
		LineStart: 5, LineEnd: 6,
		// Wrapped lines joined with a space instead of the file's newline.
		Quote: "Body drift reports THAT something changed, never WHAT. It cannot distinguish an added MUST from a fixed typo, and it MUST NOT try.",
	})
	v := VerifyAnchors(sc, parentFixture)
	if v.OK() {
		t.Fatal("a reconstructed quote was accepted — the generation gate has collapsed into drift's leniency")
	}
	if !strings.Contains(v.Faults[0].String(), "whitespace normalisation") {
		t.Errorf("fault does not name reconstruction as the cause:\n%s", v.Faults[0])
	}

	// Control: drift detection still accepts it. The two are SUPPOSED to
	// disagree here, and a test asserting only the strict side would not
	// notice if the lenient side were tightened by accident.
	verdict := ClassifyExcerpt(sc.Directives[0].SourceExcerpts[0], sc.Directives[0].Text,
		strings.Split(parentFixture, "\n"))
	if !verdict.Grounded() {
		t.Errorf("drift detection also rejected it (%s) — the two checks are meant to differ in strictness, "+
			"and if they now agree the lenient one has been tightened", verdict)
	}
}

// TestVerifyAnchorsRangeFaults covers the ranges that cannot be right
// regardless of the quote.
func TestVerifyAnchorsRangeFaults(t *testing.T) {
	cases := map[string]SourceExcerpt{
		"line_start below 1": {LineStart: 0, LineEnd: 1, Quote: "anything"},
		"inverted range":     {LineStart: 6, LineEnd: 5, Quote: "anything"},
		"past end of parent": {LineStart: 5, LineEnd: 900, Quote: "anything"},
		"empty quote":        {LineStart: 5, LineEnd: 5, Quote: "   "},
	}
	for name, a := range cases {
		t.Run(name, func(t *testing.T) {
			if v := VerifyAnchors(scWithAnchors(a), parentFixture); v.OK() {
				t.Error("accepted an anchor whose range or quote cannot be valid")
			}
		})
	}
}

// TestVerifyAnchorsZeroAnchorsIsAFault — a directive with no anchors is
// ungrounded by construction. It must be a fault and not a silent skip: an
// item nothing checked, reported as checked, is the INV-013 shape.
func TestVerifyAnchorsZeroAnchorsIsAFault(t *testing.T) {
	v := VerifyAnchors(scWithAnchors(), parentFixture)
	if v.OK() {
		t.Fatal("a directive with zero anchors was accepted")
	}
	if v.Items != 1 {
		t.Errorf("items = %d, want 1 — an unanchored item must still be counted", v.Items)
	}
}

// TestVerifyAnchorsReportsEveryFault — an extractor that got one range wrong
// has usually got several. Stopping at the first turns one fix into N
// dispatch cycles.
func TestVerifyAnchorsReportsEveryFault(t *testing.T) {
	sc := &Sidecar{
		SchemaVersion: 2, Topic: "compile", Path: "docs/x.md",
		Directives: []Directive{
			{Text: "A. (ref: ADR-900)", SourceExcerpts: []SourceExcerpt{{LineStart: 6, LineEnd: 6, Quote: "Body drift reports THAT something changed, never WHAT."}}},
			{Text: "B. (ref: ADR-900)", SourceExcerpts: []SourceExcerpt{{LineStart: 1, LineEnd: 1, Quote: "nowhere in this file"}}},
		},
		Prohibitions: []Prohibition{
			{Text: "MUST NOT relocate. (ref: ADR-900)", SourceExcerpts: []SourceExcerpt{{LineStart: 900, LineEnd: 901, Quote: "x"}}},
		},
	}
	v := VerifyAnchors(sc, parentFixture)
	if len(v.Faults) != 3 {
		t.Fatalf("reported %d fault(s), want 3 — the gate must name every offending anchor, not the first", len(v.Faults))
	}
	kinds := map[string]int{}
	for _, f := range v.Faults {
		kinds[f.Kind]++
	}
	if kinds["prohibitions"] != 1 {
		t.Errorf("prohibitions were not verified (%v) — they carry their own anchors and drift the same way", kinds)
	}
}

// TestValidateRawAgainstDeclaredSchemaDispatch pins the version dispatch.
//
// The regression it guards is concrete: both validating call sites hardcoded
// the v1 schema, so migrating the corpus to v2 made 70 of 82 valid sidecars
// report as schema failures.
func TestValidateRawAgainstDeclaredSchemaDispatch(t *testing.T) {
	v1 := []byte(`schema_version: 1
topic: compile
path: docs/x.md
signals: [example thing]
directives:
  - text: "A MUST b. (ref: ADR-900)"
    source_excerpt:
      line_start: 5
      line_end: 5
      quote: "A must b."
`)
	v2 := []byte(`schema_version: 2
topic: compile
path: docs/x.md
signals: [example thing]
directives:
  - text: "A MUST b. (ref: ADR-900)"
    source_excerpts:
      - line_start: 5
        line_end: 5
        quote: "A must b."
`)
	if err := ValidateRawAgainstDeclaredSchema(v1); err != nil {
		t.Errorf("a valid v1 sidecar was rejected: %v", err)
	}
	if err := ValidateRawAgainstDeclaredSchema(v2); err != nil {
		t.Errorf("a valid v2 sidecar was rejected: %v", err)
	}

	// Sensitivity: the dispatch must actually pick a different schema, not
	// accept both shapes under one permissive check. A v2-shaped body
	// DECLARING v1 must fail.
	mislabelled := []byte(strings.Replace(string(v2), "schema_version: 2", "schema_version: 1", 1))
	if err := ValidateRawAgainstDeclaredSchema(mislabelled); err == nil {
		t.Error("a v2-shaped body declaring schema_version 1 was accepted — the dispatch is not selecting a schema")
	}

	// Fail closed on an undeclared or unknown version: neither is validatable,
	// and neither may fall back to whichever schema is older.
	for name, body := range map[string]string{
		"undeclared": "topic: compile\npath: docs/x.md\nsignals: []\ndirectives: []\n",
		"unknown":    "schema_version: 99\ntopic: compile\npath: docs/x.md\nsignals: []\ndirectives: []\n",
	} {
		if err := ValidateRawAgainstDeclaredSchema([]byte(body)); err == nil {
			t.Errorf("%s schema_version was accepted — an unvalidatable document must not report as valid", name)
		}
	}
}
