package phaseb

import (
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/compile"
)

func unionFixture() *topicGroup {
	return &topicGroup{
		Directives: []compile.Rule{
			{Text: "Zeta MUST hold. (ref: ADR-002)", Source: "ADR-002"},
			{Text: "Alpha MUST hold. (ref: ADR-001)", Source: "ADR-001"},
		},
		Manual: []manualEntry{
			{Text: "Beta MUST hold.", Source: "ADR-001"},
			{Text: "Gamma MUST hold. (ref: ADR-003)", Source: "ADR-003"},
		},
	}
}

// TestUnionMatchesBuildRegionLines is the DRIFT GUARD, and the reason
// buildRegionLines was not refactored to share this code: the topic file's
// bytes must not shift, so the two implementations stay separate and a test
// pins them together. Asserts the resolved outcome — identical rendered text
// in identical order — rather than restating the annotation logic (INV-014).
func TestUnionMatchesBuildRegionLines(t *testing.T) {
	g := unionFixture()
	want, _, _ := buildRegionLines(g)
	got := unionDirectiveRules(g)
	if len(got) != len(want) {
		t.Fatalf("length: union %d, region %d — the two surfaces would carry different rule sets", len(got), len(want))
	}
	for i := range want {
		if got[i].Text != want[i] {
			t.Errorf("index %d differs:\n  region: %q\n  union : %q\n"+
				"the skill and the topic file would render the same rule differently", i, want[i], got[i].Text)
		}
	}
}

// TestUnionCarriesManual — POSITIVE CONTROL for the bug itself. Before the fix
// a retired topic's skill received g.Directives raw; every manual entry was
// dropped. On bok-services that was 61 of 63 manual directives reaching no
// compiled surface at all.
func TestUnionCarriesManual(t *testing.T) {
	g := unionFixture()
	got := unionDirectiveRules(g)
	if len(got) != 4 {
		t.Fatalf("union must carry 2 extracted + 2 manual, got %d", len(got))
	}
	var manual int
	for _, r := range got {
		if strings.Contains(r.Text, "*(manual)*") {
			manual++
		}
	}
	if manual != 2 {
		t.Errorf("both manual entries must be present and marked; found %d", manual)
	}
	// The un-unioned field is what the bug passed. If these were equal the
	// fix would be a no-op.
	if len(got) == len(g.Directives) {
		t.Error("union is the same size as g.Directives alone — manual was not added")
	}
}

// TestUnionAnnotatesManualWithoutRefTag pins the ref synthesis, which is the
// half a caller would most plausibly reimplement differently.
func TestUnionAnnotatesManualWithoutRefTag(t *testing.T) {
	g := &topicGroup{Manual: []manualEntry{{Text: "Delta MUST hold.", Source: "ADR-009"}}}
	got := unionDirectiveRules(g)
	if len(got) != 1 {
		t.Fatalf("got %d rules", len(got))
	}
	if !strings.Contains(got[0].Text, "(ref: ADR-009 + manual)") {
		t.Errorf("a manual entry with no ref tag must gain one: %q", got[0].Text)
	}
	if !strings.Contains(got[0].Text, "*(manual)*") {
		t.Errorf("manual marker missing: %q", got[0].Text)
	}
	if got[0].Source != "ADR-009" {
		t.Errorf("Source must survive for attribution; got %q", got[0].Source)
	}
}

// TestUnionEmptyIsEmpty — a topic with neither must not synthesise a rule.
func TestUnionEmptyIsEmpty(t *testing.T) {
	if got := unionDirectiveRules(&topicGroup{}); len(got) != 0 {
		t.Errorf("empty topic must yield no rules; got %v", got)
	}
}
