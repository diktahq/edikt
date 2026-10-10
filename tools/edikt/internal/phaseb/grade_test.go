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

// SR-003 (SPEC-018) REVERSED THIS. ADR-071's class table promoted an ADR
// prohibition to must; that rule put 112 of 238 write-time MUSTs into the deny
// channel as rejected options, 34.8% of the delivered payload, and Stage 0
// measured dropping them at -34.9% est-tokens per deny.
//
// The test is kept and inverted rather than deleted: the default is the thing
// that changed, so it is the thing that needs pinning. An ADR prohibition that
// must still interrupt now says so with `enforcement: deny`, which the test
// below asserts still works — the override became the mechanism.
func TestGradeFor_ADRProhibitionDefaultsAdvisory(t *testing.T) {
	if got := gradeFor("adr", true, ""); got != "advisory" {
		t.Fatalf("ADR prohibition with no enforcement override must grade advisory since SR-003, got %q", got)
	}
}

func TestGradeFor_ADRProhibitionEnforcementDenyStillMust(t *testing.T) {
	// The escape hatch SR-003 leaves open. Without this, removing the class-table
	// promotion would silently remove the ability to promote at all.
	if got := gradeFor("adr", true, "deny"); got != "must" {
		t.Fatalf("ADR prohibition with enforcement:deny must still grade must, got %q", got)
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

// TestDeliveryMap_ADRSidecarDirectiveAndProhibitionGradeDiffer proves the
// grade decision is made per entry, not per sidecar: the delivery map carries
// a Grade on each entry, and an ADR's directive and prohibition can differ
// (enforcement:deny promotes the prohibition alone). Since ADR-083 the map is
// the only place grade is pinned — there is no index to read it from.
func TestDeliveryMap_ADRSidecarDirectiveAndProhibitionGradeDiffer(t *testing.T) {
	pair := sidecar.Pair{
		ArtifactID: "ADR-901",
		Sidecar: &sidecar.Sidecar{
			Topic:      "architecture",
			Paths:      []string{"a/**/*.go"},
			Directives: []sidecar.Directive{{Text: "An ordinary architectural decision. (ref: ADR-901)"}},
			Prohibitions: []sidecar.Prohibition{
				{Text: "MUST NOT do the rejected thing. (ref: ADR-901)", Enforcement: "deny"},
			},
		},
	}
	grades := map[string]string{}
	for _, entries := range BuildDeliveryMap([]sidecar.Pair{pair}, map[string]bool{}) {
		for _, e := range entries {
			grades[e.ID] = e.Grade
		}
	}
	if grades["ADR-901:d01"] != gradeAdvisory {
		t.Fatalf("ADR directive must grade advisory by default (SR-003), got %q", grades["ADR-901:d01"])
	}
	if grades["ADR-901:p01"] != gradeMust {
		t.Fatalf("enforcement:deny on a prohibition must promote it to must, got %q", grades["ADR-901:p01"])
	}
	if gradeFor("adr", false, "") != gradeAdvisory || gradeFor("adr", true, "") != gradeAdvisory {
		t.Fatal("both ADR kinds must grade advisory by default")
	}
}
