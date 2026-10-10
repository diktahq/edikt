package sidecar

import "testing"

// SR-005 (SPEC-018). The id rule used to consume only digits after the 4-char
// prefix, so anything that was not exactly PREFIX-NNN truncated at the first
// non-digit and two artifacts could collide on one id.
func TestArtifactIDFromName_SegmentRule(t *testing.T) {
	cases := []struct{ name, want string }{
		// Unchanged for the shape the corpus already uses.
		{"ADR-042-where-the-harness-executes.md", "ADR-042"},
		{"INV-002-adr-immutability.md", "INV-002"},

		// THE REGRESSION. Both of these resolved to "ADR-" before SR-005, so
		// they collided on one id and dedup dropped one of them.
		{"ADR-GOLD-001-a-slug.md", "ADR-GOLD-001"},
		{"ADR-GOLD-002-another-slug.md", "ADR-GOLD-002"},

		// A GL id IS the whole basename, slug included — the branch is not
		// entered and the fallthrough returns it whole.
		{"GL-002-verify-coverage.md", "GL-002-verify-coverage"},

		// Degenerate shapes must not panic or over-consume.
		{"ADR-.md", "ADR-"},
		{"ADR-042.md", "ADR-042"},
		{"README.md", "README"},
	}
	for _, c := range cases {
		if got := artifactIDFromName(c.name); got != c.want {
			t.Errorf("artifactIDFromName(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// The collision is the defect, so it is asserted as one rather than left
// implicit in two table rows.
func TestArtifactIDFromName_NonNNNIDsDoNotCollide(t *testing.T) {
	a := artifactIDFromName("ADR-GOLD-001-a.md")
	b := artifactIDFromName("ADR-GOLD-002-b.md")
	if a == b {
		t.Fatalf("two artifacts collided on one id: both resolved to %q — dedup would silently drop one", a)
	}
	if a == "ADR-" || b == "ADR-" {
		t.Fatalf("id truncated to the bare prefix: %q / %q", a, b)
	}
}
