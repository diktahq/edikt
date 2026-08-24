package phaseb

import (
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// ADR-071 (amends ADR-064) — gradeFor stopped deriving grade from modal-verb
// detection in directive text (98.2% of the corpus graded must, because ADR
// prose is overwhelmingly written as plain declaratives that extraction
// correctly promotes to MUST-phrased text — the modal check downstream had
// almost nothing left to discriminate on). Grade is now class-weighted:
//
//	INV-*, directive or prohibition  -> must
//	ADR-*, prohibition               -> must
//	ADR-*, directive                 -> advisory
//	GL-*,  directive or prohibition  -> advisory
//
// `enforcement` (ADR-070) overrides the class default in either direction
// when set. These tests replace directiveindex_grade_test.go's prior
// ADR-064-era assertions (bare-MUST-derives-must, MUST-vs-SHOULD moves
// grade, etc.), which tested a mechanism this ADR retires — keeping them
// would assert behavior ADR-071 deliberately changed.

func TestGradeFor_InvariantDefaultsMust(t *testing.T) {
	// ADR-071 Confirmation bullet 1: a fixture invariant directive with no
	// enforcement field grades must.
	if got := gradeFor("invariant", false, ""); got != "must" {
		t.Fatalf("invariant directive with no enforcement override must grade must, got %q", got)
	}
	if got := gradeFor("invariant", true, ""); got != "must" {
		t.Fatalf("invariant prohibition with no enforcement override must grade must, got %q", got)
	}
}

func TestGradeFor_ADRDirectiveDefaultsAdvisory(t *testing.T) {
	// ADR-071 Confirmation bullet 2: a fixture ADR directive with no
	// enforcement field grades advisory.
	if got := gradeFor("adr", false, ""); got != "advisory" {
		t.Fatalf("ADR directive with no enforcement override must grade advisory, got %q", got)
	}
}

func TestGradeFor_ADRDirectiveEnforcementDenyEscalates(t *testing.T) {
	// ADR-071 Confirmation bullet 2 (continued): the same fixture with
	// enforcement: deny grades must.
	if got := gradeFor("adr", false, "deny"); got != "must" {
		t.Fatalf("ADR directive with enforcement:deny must grade must, got %q", got)
	}
}

func TestGradeFor_ADRProhibitionDefaultsMust(t *testing.T) {
	// ADR-071 Confirmation bullet 3: a fixture ADR prohibition with no
	// enforcement field grades must.
	if got := gradeFor("adr", true, ""); got != "must" {
		t.Fatalf("ADR prohibition with no enforcement override must grade must, got %q", got)
	}
}

func TestGradeFor_ADRProhibitionEnforcementAdvisoryDemotes(t *testing.T) {
	// ADR-071 Confirmation bullet 3 (continued): the same fixture with
	// enforcement: advisory grades advisory.
	if got := gradeFor("adr", true, "advisory"); got != "advisory" {
		t.Fatalf("ADR prohibition with enforcement:advisory must grade advisory, got %q", got)
	}
}

func TestGradeFor_GuidelineDefaultsAdvisory(t *testing.T) {
	if got := gradeFor("guideline", false, ""); got != "advisory" {
		t.Fatalf("guideline directive with no enforcement override must grade advisory, got %q", got)
	}
	if got := gradeFor("guideline", true, ""); got != "advisory" {
		t.Fatalf("guideline prohibition with no enforcement override must grade advisory, got %q", got)
	}
}

func TestGradeFor_EnforcementOverridesInvariantInEitherDirection(t *testing.T) {
	// ADR-071 Decision: "enforcement: advisory forces advisory on an
	// invariant... that a reviewer judges doesn't [warrant blocking]" —
	// enforcement always takes priority over the class default when set,
	// even for the invariant class. Distinct from the default-unset case
	// (TestGradeFor_InvariantDefaultsMust), which stays must unconditionally.
	if got := gradeFor("invariant", false, "advisory"); got != "advisory" {
		t.Fatalf("enforcement:advisory must override even the invariant class default, got %q", got)
	}
}

func TestGradeFor_UnknownClassDefaultsAdvisory(t *testing.T) {
	// classFor's own fallback ("unknown", for an unrecognized ID prefix)
	// must not panic or silently escalate — same fail-open posture as the
	// rest of the write-time tier.
	if got := gradeFor("unknown", false, ""); got != "advisory" {
		t.Fatalf("unrecognized class with no enforcement override must grade advisory, got %q", got)
	}
}

// TestIndexEntriesFor_ADRSidecarDirectiveAndProhibitionGradeDiffer proves the
// real call site distinguishes directive vs prohibition within the SAME ADR
// sidecar — gradeFor's isProhibition parameter must be wired from the actual
// list an entry came from, not hardcoded or guessed from text shape.
func TestIndexEntriesFor_ADRSidecarDirectiveAndProhibitionGradeDiffer(t *testing.T) {
	pair := fiveGlobSidecar([]sidecar.Directive{
		{Text: "An ordinary architectural decision. (ref: ADR-901)"},
	})
	pair.Sidecar.Prohibitions = []sidecar.Prohibition{
		{Text: "MUST NOT do the rejected thing. (ref: ADR-901)"},
	}
	dirs, prohs := IndexEntriesFor(pair)
	if len(dirs) != 1 || dirs[0].Grade != "advisory" {
		t.Fatalf("ADR directive with no enforcement override must grade advisory, got %+v", dirs)
	}
	if len(prohs) != 1 || prohs[0].Grade != "must" {
		t.Fatalf("ADR prohibition with no enforcement override must grade must, got %+v", prohs)
	}
}
