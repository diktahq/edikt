package cmd

import "testing"

// TestArtifactIDFromEveryRealShape. These are the shapes that actually occur;
// each one broke, or would have broken, one of the three implementations this
// file replaced.
func TestArtifactIDFromEveryRealShape(t *testing.T) {
	for in, want := range map[string]string{
		"INV-010:d03": "INV-010", // index entry id
		"INV-010-directive-metadata-lands-in-sidecars.md": "INV-010",  // artifact filename
		"SPEC-018-enforcement-reach":                      "SPEC-018", // spec DIRECTORY, 4-letter prefix
		"PRD-011-governance-delivery.md":                  "PRD-011",
		"GL-002-verify-coverage.md":                       "GL-002",   // 2-letter prefix
		"INV-021.md":                                      "INV-021",  // no slug
		"ADR-057":                                         "ADR-057",  // bare id
		"INV-0100-four-digit.md":                          "INV-0100", // all digits consumed
	} {
		got, ok := artifactIDFrom(in)
		if !ok || got != want {
			t.Errorf("artifactIDFrom(%q) = (%q, %v), want (%q, true)", in, got, ok, want)
		}
	}
}

// TestArtifactIDRejectsNonIDs. ok=false, never a partial or empty id: a ""
// silently grouped with other ""s is how an unparsed name becomes a fake
// duplicate in doctor_duplicate_ids.
func TestArtifactIDRejectsNonIDs(t *testing.T) {
	for _, in := range []string{"NOTANID", "inv-010", "ADR-", "-010", "ADR-02", "", "README.md", "d03"} {
		if got, ok := artifactIDFrom(in); ok {
			t.Errorf("artifactIDFrom(%q) = (%q, true); must not match", in, got)
		}
	}
}

// TestFailureModeOne — THE FALSE ZERO.
//
// `^([A-Z]{2,3}-\d{3})(?::|$)` was written for index entry ids and reused on
// filenames. A hyphen follows the id in a filename, not a colon, so it matched
// NOTHING: the enumeration counted zero artifacts and the check reported "ok —
// every routed artifact reaches a glob" having measured nothing at all. On that
// corpus 66 of 86 were unrouted.
//
// One rule must handle BOTH shapes, or the next reuse repeats it.
func TestFailureModeOne_IndexRuleReusedOnFilenames(t *testing.T) {
	const entry = "INV-010:d03"
	const file = "INV-010-directive-metadata-lands-in-sidecars.md"
	e, eok := artifactIDFrom(entry)
	f, fok := artifactIDFrom(file)
	if !eok || !fok {
		t.Fatalf("both shapes must resolve: entry ok=%v, filename ok=%v", eok, fok)
	}
	if e != f {
		t.Errorf("the same artifact must yield the same id from either shape: %q vs %q", e, f)
	}
}

// TestFailureModeTwo — THE TRUNCATED ID.
//
// A hand-rolled IndexAny trim in strandedProposals produced "ADR-" instead of
// "ADR-002". A truncated id is worse than no id: it is non-empty, so it passes
// every emptiness check and then groups every ADR under one key.
func TestFailureModeTwo_TruncatedID(t *testing.T) {
	for _, in := range []string{
		"ADR-002-claude-md-safe-merge.md",
		"ADR-069-rename-no-directives-key.md",
	} {
		got, ok := artifactIDFrom(in)
		if !ok {
			t.Fatalf("artifactIDFrom(%q) did not match", in)
		}
		if got == "ADR-" || len(got) < len("ADR-000") {
			t.Errorf("artifactIDFrom(%q) = %q — truncated id", in, got)
		}
	}
	// And the two must not collapse to one key.
	a, _ := artifactIDFrom("ADR-002-x.md")
	b, _ := artifactIDFrom("ADR-069-y.md")
	if a == b {
		t.Errorf("two artifacts collapsed to one id: %q", a)
	}
}

// TestPrefixWidthCoversSpec. GL is two letters and SPEC is four. A {2,3} rule
// silently drops all 18 specs from duplicate-id detection — a check reporting
// nothing where there is something.
func TestPrefixWidthCoversSpec(t *testing.T) {
	for _, in := range []string{"GL-002-x.md", "ADR-001-x.md", "SPEC-018-x", "PRD-011-x.md"} {
		if _, ok := artifactIDFrom(in); !ok {
			t.Errorf("artifactIDFrom(%q) must match; prefix width regression", in)
		}
	}
}
