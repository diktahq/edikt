package phaseb

import "testing"

// ADR-064 — gradeFor used to check only negative-polarity markers
// ("MUST NOT", "NEVER", "NO EXCEPTIONS"), so a bare positive MUST from a
// non-invariant artifact fell through to advisory. Corpus-measured: 404 of
// 420 currently-advisory directive-index entries (96%) carried exactly this
// shape. Fixed to derive from sidecar.DirectiveModal (RFC-2119 modal force)
// instead of a hand-rolled keyword list.

func TestGradeFor_BarePositiveMustDerivesMust(t *testing.T) {
	// The live instance that made this fix non-hypothetical: ADR-055
	// clause 2, extracted verbatim.
	got := gradeFor("ADR-055", "The extractor model MUST be `sonnet`. (ref: ADR-055)")
	if got != "must" {
		t.Fatalf("bare positive MUST from a non-invariant artifact must derive to must-grade, got %q", got)
	}
}

func TestGradeFor_NegativeMarkersStillDeriveMust(t *testing.T) {
	// Mode 2 (MUST NOT/NEVER -> positive MUST) must stay closed exactly as
	// before this fix. "NO EXCEPTIONS" alone (no accompanying RFC-2119
	// modal) is deliberately NOT special-cased here: corpus-checked, the
	// one real instance without an accompanying modal (INV-012:d03, "No
	// exceptions outside that file.") is an invariant and derives to must
	// unconditionally regardless of wording — no non-invariant directive in
	// the corpus relies on "no exceptions" alone to qualify, so adding a
	// second keyword check for it would reintroduce exactly the parallel-
	// implementation drift this fix removes, for a case nothing needs.
	cases := []string{
		"The compiler MUST NOT emit a warning. (ref: ADR-001)",
		"This behavior is NEVER permitted. (ref: ADR-001)",
		"The gate MUST fail closed; no exceptions. (ref: ADR-001)",
	}
	for _, text := range cases {
		if got := gradeFor("ADR-001", text); got != "must" {
			t.Fatalf("negative-marker text must still derive to must-grade, got %q for %q", got, text)
		}
	}
}

func TestGradeFor_F023Mode3_MustToShouldMovesGrade(t *testing.T) {
	must := gradeFor("ADR-058", "The pattern MUST be folded into stage 2. (ref: ADR-058)")
	should := gradeFor("ADR-058", "The pattern SHOULD be folded into stage 2. (ref: ADR-058)")
	if must != "must" {
		t.Fatalf("MUST-worded directive should derive to must-grade, got %q", must)
	}
	if should == must {
		t.Fatalf("F-023 mode 3: rewording must not stay invisible — MUST and SHOULD derived the same grade (%q)", should)
	}
	if should != "advisory" {
		t.Fatalf("SHOULD-worded directive should derive to advisory, got %q", should)
	}
}

func TestGradeFor_MayLevelStaysAdvisory(t *testing.T) {
	got := gradeFor("ADR-001", "Teams MAY override the default. (ref: ADR-001)")
	if got != "advisory" {
		t.Fatalf("MAY-level text must derive to advisory, got %q", got)
	}
}

func TestGradeFor_InvariantsAlwaysMustRegardlessOfWording(t *testing.T) {
	// Structural, not textual: an invariant is must-grade even with no
	// RFC-2119 modal at all.
	got := gradeFor("INV-009", "Completion claims require fresh evidence in the same turn.")
	if got != "must" {
		t.Fatalf("invariant text with no modal keyword must still derive to must-grade, got %q", got)
	}
}

func TestGradeFor_NoModalDerivesAdvisory(t *testing.T) {
	got := gradeFor("ADR-001", "This section explains the background for the decision.")
	if got != "advisory" {
		t.Fatalf("descriptive text with no RFC-2119 modal must derive to advisory, got %q", got)
	}
}
