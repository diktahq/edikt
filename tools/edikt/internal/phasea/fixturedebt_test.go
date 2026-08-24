package phasea

import (
	"strings"
	"testing"
)

// The debt notice must fire on the shape that actually bit a consumer project:
// behavioral verifies introduced by a regeneration with no fixtures written.
func TestFixtureDebtNotice_NamesArtifactsAndTotal(t *testing.T) {
	got := fixtureDebtNotice([]fixtureDebt{
		{ArtifactID: "ADR-014-claim-state", Missing: 4},
		{ArtifactID: "INV-007-attestation", Missing: 6},
	})
	for _, want := range []string{
		"10 behavioral verify(s) without a fixture pair",
		"ADR-014-claim-state — 4",
		"INV-007-attestation — 6",
		"positive AND a negative script",
		"edikt.dev/governance/behavioral-fixtures",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("notice missing %q:\n%s", want, got)
		}
	}
}

// A clean regeneration must say nothing. A warning that fires every run is the
// unread channel — the notice earns attention by being rare.
func TestFixtureDebtNotice_SilentWhenNoDebt(t *testing.T) {
	if got := fixtureDebtNotice(nil); got != "" {
		t.Errorf("expected silence on zero debt, got:\n%s", got)
	}
	if got := fixtureDebtNotice([]fixtureDebt{}); got != "" {
		t.Errorf("expected silence on empty slice, got:\n%s", got)
	}
}

// Both spellings must count. The extractor prompt now pins the bare form, but a
// corpus written before that pin carries the quoted one — and under-reporting
// the exact debt this exists to surface would be the defect, not a rounding
// error. The reporting project's corpus carried both.
func TestIsBehavioral_CountsBothSpellings(t *testing.T) {
	for _, k := range []string{"behavioral", `"behavioral"`, "'behavioral'", " behavioral ", "Behavioral"} {
		if !isBehavioral(k) {
			t.Errorf("isBehavioral(%q) = false; a spelling variant must still count", k)
		}
	}
	for _, k := range []string{"", "tooling", "structural", "behaviour"} {
		if isBehavioral(k) {
			t.Errorf("isBehavioral(%q) = true; only behavioral counts", k)
		}
	}
}

// A pair is complete only with BOTH arms. A positive with no negative is a
// check that has never been shown to fail.
func TestIncompletePair_RequiresBothArms(t *testing.T) {
	cases := []struct {
		pos, neg string
		want     bool
	}{
		{"", "", true},
		{"a.sh", "", true},
		{"", "b.sh", true},
		{"  ", "b.sh", true},
		{"a.sh", "b.sh", false},
	}
	for _, c := range cases {
		if got := incompletePair(c.pos, c.neg); got != c.want {
			t.Errorf("incompletePair(%q,%q) = %v, want %v", c.pos, c.neg, got, c.want)
		}
	}
}
