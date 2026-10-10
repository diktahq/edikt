package versioncmp

// versioncmp_test.go — SPEC-024 phase 2, the helper side of SR-002.
//
// The fixture table (docs/product/specs/SPEC-024-.../fixtures.yaml) is the
// authority for what this package must answer. Its rows are not read as
// intent alone: each carries what the SHIPPING code answered when the row was
// written, so a row recorded `agrees_today: false` cannot be satisfied by
// preserving the behaviour that was there. The version-ordering group is
// driven directly here; cmd re-drives the same rows through its own harness.

import (
	"strings"
	"testing"

	fx "github.com/diktahq/edikt/tools/edikt/internal/spec024fixtures"
)

// excludedRows names every fixture row this package's gate does NOT assert,
// with the reason. See TestFixtures_VersionOrdering.
var excludedRows = map[string]string{
	"VO-12": "an empty version intends `error` — the caller must be FORCED to handle an absence. Compare answers `incomparable`, which is safe at every consumer and is not the same thing; the requirement that closes it guards an empty LATEST_VERSION in the tier-1 shell block and belongs to a later phase",
}

// TestFixtures_VersionOrdering — AC-2.5.
//
// Every row of the version-ordering group except VO-12, asserted against the
// helper directly. VO-11 is called out by name below: it is the row that
// catches a rewrite comparing pre-release suffixes as STRINGS, which gets
// beta.2 against beta.10 backwards and passes every other row in the group.
func TestFixtures_VersionOrdering(t *testing.T) {
	tbl, err := fx.Load()
	if err != nil {
		t.Fatalf("loading the fixture table: %v", err)
	}
	rows := tbl.Group("version-ordering")
	if len(rows) == 0 {
		t.Fatal("the version-ordering group is empty — this gate would report a pass having measured nothing")
	}

	measured := map[string]bool{}
	for _, row := range rows {
		if _, skip := excludedRows[row.ID]; skip {
			continue
		}
		left, _ := row.Fields["left"].(string)
		right, _ := row.Fields["right"].(string)
		intended, _ := row.Fields["intended"].(string)
		if intended == "" {
			t.Errorf("%s records no `intended` verdict — there is nothing to hold the helper to", row.ID)
			continue
		}
		measured[row.ID] = true

		got := Compare(left, right)
		if string(got) != intended {
			t.Errorf("%s: Compare(%q, %q) = %q, intended %q", row.ID, left, right, got, intended)
		}

		// ANTISYMMETRY, asserted on every row rather than in a case of its
		// own: an ordering that answers left_newer in one direction and
		// anything but right_newer in the other is not an ordering.
		rev := Compare(right, left)
		want := map[Ordering]Ordering{
			LeftNewer:    RightNewer,
			RightNewer:   LeftNewer,
			Equal:        Equal,
			Incomparable: Incomparable,
		}[got]
		if rev != want {
			t.Errorf("%s: Compare(%q,%q)=%q but the reverse is %q, expected %q — the relation is not antisymmetric",
				row.ID, left, right, got, rev, want)
		}
	}

	if len(measured) != len(rows)-len(excludedRows) {
		t.Errorf("measured %d rows of %d (%d excluded)", len(measured), len(rows), len(excludedRows))
	}
	if !measured["VO-11"] {
		t.Error("VO-11 was not measured — it is the row that catches a string comparison of pre-release identifiers, and every other row in the group passes under that wrong reading")
	}

	// The exclusion list may hold exactly the row it claims, with a reason.
	if len(excludedRows) != 1 {
		t.Errorf("exactly one row may be excluded; %d are: %v", len(excludedRows), excludedRows)
	}
	for id, why := range excludedRows {
		if strings.TrimSpace(why) == "" {
			t.Errorf("row %s is excluded with no reason stated", id)
		}
		row, ok := tbl.Lookup(id)
		if !ok {
			t.Errorf("excluded row %s is not in the table", id)
			continue
		}
		if row.Agreement != fx.AgreeNo {
			t.Errorf("excluded row %s records agrees_today: %q — it is excluded BECAUSE it still disagrees, so a flip means it was closed by a phase that does not own it", id, row.Agreement)
		}
	}
}

// TestUpgradeGate_AllConsumersUseHelper — AC-2.2, the helper's half.
//
// cmd asserts that its four call sites go through this package. What is
// asserted here is that going through it MAKES THE OLD DEFECT UNREPRESENTABLE:
// there is no value of any exported function that reports a channel name as a
// version, as major 0, or as ordered against a release. A consumer cannot
// reintroduce the defect by calling this package correctly.
func TestUpgradeGate_AllConsumersUseHelper(t *testing.T) {
	channels := []string{"dev", "nightly", "", "  ", "0.8", "v", "1.0.0.0", "0.8.x"}

	t.Run("no channel name parses as a version", func(t *testing.T) {
		for _, c := range channels {
			if v, ok := Parse(c); ok {
				t.Errorf("Parse(%q) = %+v, true — a string that is not a release version must say so", c, v)
			}
		}
	})

	t.Run("no channel name has a major version", func(t *testing.T) {
		for _, c := range channels {
			if n, ok := Major(c); ok {
				t.Errorf("Major(%q) = %d, true — reading a channel as major 0 is what made a 1.x release look like a cross-major upgrade from a dev checkout (VO-10)", c, n)
			}
		}
	})

	t.Run("no channel name is ordered against a release", func(t *testing.T) {
		for _, c := range channels {
			for _, rel := range []string{"0.4.5", "0.8.0", "0.9.0", "1.0.0"} {
				if got := Compare(c, rel); got != Incomparable {
					t.Errorf("Compare(%q, %q) = %q, want incomparable", c, rel, got)
				}
				if Greater(rel, c) {
					t.Errorf("Greater(%q, %q) is true — this is the comparison the already-up-to-date refusal read, and it is why a dev install was never refused there", rel, c)
				}
				if Greater(c, rel) {
					t.Errorf("Greater(%q, %q) is true", c, rel)
				}
			}
		}
	})

	t.Run("the one comparison on a channel that IS answerable", func(t *testing.T) {
		if got := Compare("dev", "dev"); got != Equal {
			t.Errorf("Compare(dev, dev) = %q, want equal — the helper is not allowed to refuse a question it can answer (VO-05)", got)
		}
		if got := Compare("", ""); got != Incomparable {
			t.Errorf("Compare(\"\", \"\") = %q, want incomparable — an empty string is an absence, not a name, and two failed fetches are not the same version", got)
		}
	})
}

// The semver rules, each with the wrong answer it exists to exclude.
func TestCompare_SemverRules(t *testing.T) {
	cases := []struct {
		left, right string
		want        Ordering
		why         string
	}{
		{"0.8.0", "0.8.0", Equal, "reflexivity"},
		{"v0.8.0", "0.8.0", Equal, "the leading v is presentation, not version (§2)"},
		{"0.8.0+build.7", "0.8.0", Equal, "build metadata takes no part in ordering (§11.2)"},
		{"1.0.0", "0.9.0", LeftNewer, "major outranks minor"},
		{"0.10.0", "0.9.0", LeftNewer, "minor compares NUMERICALLY — lexically 10 < 9"},
		{"0.9.10", "0.9.9", LeftNewer, "patch compares numerically too"},
		{"0.8.0-rc1", "0.8.0", RightNewer, "a pre-release sorts before its release (§11.3)"},
		{"0.8.0", "0.8.0-rc1", LeftNewer, "and the same in the other direction"},
		{"0.9.0-beta.2", "0.9.0-beta.10", RightNewer, "numeric identifiers compare numerically: 10 > 2 (§11.4.1)"},
		{"0.9.0-alpha", "0.9.0-beta", RightNewer, "alphanumeric identifiers compare in ASCII order (§11.4.2)"},
		{"0.9.0-1", "0.9.0-alpha", RightNewer, "a numeric identifier sorts BEFORE an alphanumeric one (§11.4.3)"},
		{"0.9.0-alpha", "0.9.0-alpha.1", RightNewer, "a larger set of identifiers sorts higher when all preceding are equal (§11.4.4)"},
		{"0.9.0-beta.2", "0.8.0", LeftNewer, "the numeric core decides before any pre-release rule is consulted (VO-01)"},
	}
	for _, c := range cases {
		if got := Compare(c.left, c.right); got != c.want {
			t.Errorf("Compare(%q, %q) = %q, want %q — %s", c.left, c.right, got, c.want, c.why)
		}
	}
}

// The full §11 ordering, asserted as a CHAIN rather than as pairs. A set of
// pairwise assertions can all pass while the relation they describe is not a
// total order; walking a published example (§11.4) end to end cannot.
func TestCompare_SemverSpecExampleChain(t *testing.T) {
	chain := []string{
		"1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta",
		"1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0",
	}
	for i := 0; i+1 < len(chain); i++ {
		if got := Compare(chain[i], chain[i+1]); got != RightNewer {
			t.Errorf("Compare(%q, %q) = %q, want right_newer — semver 2.0.0 §11.4's own worked example, in order",
				chain[i], chain[i+1], got)
		}
	}
	// Transitivity across the whole chain, not only between neighbours.
	if got := Compare(chain[len(chain)-1], chain[0]); got != LeftNewer {
		t.Errorf("Compare(%q, %q) = %q, want left_newer", chain[len(chain)-1], chain[0], got)
	}
}

func TestParse_RejectsWhatIsNotAVersion(t *testing.T) {
	bad := []string{
		"", " ", "dev", "nightly", "latest", "v", "0", "0.8", "0.8.0.1",
		"0.8.0-", "0.8.0-rc..1", "0.8.0-rc 1", "x.y.z", "0.8.z", "-1.0.0",
	}
	for _, s := range bad {
		if v, ok := Parse(s); ok {
			t.Errorf("Parse(%q) = %+v, true — the truncating parser this replaced accepted every one of these as a number", s, v)
		}
	}
	good := map[string]Version{
		"0.9.0":            {Major: 0, Minor: 9, Patch: 0},
		"v1.2.3":           {Major: 1, Minor: 2, Patch: 3},
		" 0.9.0 ":          {Major: 0, Minor: 9, Patch: 0},
		"0.9.0+build.1":    {Major: 0, Minor: 9, Patch: 0},
		"10.20.30":         {Major: 10, Minor: 20, Patch: 30},
		"0.9.0-rc.1+meta":  {Major: 0, Minor: 9, Patch: 0, Pre: []string{"rc", "1"}},
		"0.9.0-beta.10":    {Major: 0, Minor: 9, Patch: 0, Pre: []string{"beta", "10"}},
		"0.9.0-x-y-z.--":   {Major: 0, Minor: 9, Patch: 0, Pre: []string{"x-y-z", "--"}},
		"0.9.0-0.3.7":      {Major: 0, Minor: 9, Patch: 0, Pre: []string{"0", "3", "7"}},
		"0.9.0-alpha.beta": {Major: 0, Minor: 9, Patch: 0, Pre: []string{"alpha", "beta"}},
	}
	for s, want := range good {
		got, ok := Parse(s)
		if !ok {
			t.Errorf("Parse(%q) = _, false — this IS a release version", s)
			continue
		}
		if got.Major != want.Major || got.Minor != want.Minor || got.Patch != want.Patch {
			t.Errorf("Parse(%q) core = %d.%d.%d, want %d.%d.%d", s, got.Major, got.Minor, got.Patch, want.Major, want.Minor, want.Patch)
		}
		if strings.Join(got.Pre, ".") != strings.Join(want.Pre, ".") {
			t.Errorf("Parse(%q) pre-release = %v, want %v", s, got.Pre, want.Pre)
		}
		if got.IsPreRelease() != (len(want.Pre) > 0) {
			t.Errorf("Parse(%q).IsPreRelease() = %v, want %v", s, got.IsPreRelease(), len(want.Pre) > 0)
		}
	}
}

// Greater is a convenience over Compare and MUST NOT be used where the two
// not-newer cases differ. This test states that in code so the limitation is
// asserted rather than only documented.
func TestGreater_CannotDistinguishOlderFromIncomparable(t *testing.T) {
	older := Greater("0.8.0", "0.9.0")
	channel := Greater("0.8.0", "dev")
	if older || channel {
		t.Fatalf("Greater says true where it must not: older=%v channel=%v", older, channel)
	}
	// Same bool, different worlds — which is exactly why upgrade's refusals
	// switch on Compare instead.
	if Compare("0.8.0", "0.9.0") == Compare("0.8.0", "dev") {
		t.Error("Compare collapses `older` and `not comparable` onto one answer, which is the representation defect SR-002 exists to remove")
	}
}
