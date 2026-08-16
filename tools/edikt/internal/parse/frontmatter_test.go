package parse

import "testing"

func TestStatusWord(t *testing.T) {
	cases := map[string]string{
		"Accepted":                              "accepted",
		"Accepted.":                              "accepted",
		"Accepted, superseded in part by ADR-99": "accepted",
		"Accepted. Supersedes ADR-0026 §6, amends the storage clause": "accepted",
		"  accepted  ":       "accepted",
		"":                   "",
		"Superseded by ADR-9": "superseded",
		"Active":              "active",
	}
	for in, want := range cases {
		if got := statusWord(in); got != want {
			t.Errorf("statusWord(%q) = %q, want %q", in, got, want)
		}
	}
}

// This is the positive case: an ADR whose status line carries trailing
// prose after the status word — the shape found in a real downstream
// consumer's corpus ("Accepted. Supersedes ADR-0026 §6…") — must still be
// included. An exact-match comparison against the whole trailing sentence
// silently dropped it.
func TestIsIncluded_ADR_TrailingProseAfterStatus(t *testing.T) {
	d := &Document{
		BoldStatus: "Accepted. Supersedes ADR-0026 §6, amends the storage clause.",
	}
	if !d.IsIncluded("adr") {
		t.Fatal("expected an ADR with trailing prose after 'Accepted' to be included")
	}
}

func TestIsIncluded_ADR_PlainAccepted(t *testing.T) {
	d := &Document{BoldStatus: "Accepted"}
	if !d.IsIncluded("adr") {
		t.Fatal("expected a plain 'Accepted' status to be included")
	}
}

func TestIsIncluded_ADR_Superseded_Excluded(t *testing.T) {
	d := &Document{BoldStatus: "Superseded by ADR-0099"}
	if d.IsIncluded("adr") {
		t.Fatal("expected a superseded ADR to be excluded")
	}
}

func TestIsIncluded_ADR_FrontmatterStatusWins(t *testing.T) {
	d := &Document{}
	d.Frontmatter.Status = "accepted"
	if !d.IsIncluded("adr") {
		t.Fatal("expected frontmatter status: accepted to be included regardless of BoldStatus")
	}
}

func TestIsIncluded_INV_TrailingProseAfterStatus(t *testing.T) {
	d := &Document{BoldStatus: "Active. Amends INV-0002 in one clause."}
	if !d.IsIncluded("inv") {
		t.Fatal("expected an invariant with trailing prose after 'Active' to be included")
	}
}

func TestIsIncluded_INV_NoStatusAtAll_LegacyIncluded(t *testing.T) {
	d := &Document{}
	if !d.IsIncluded("inv") {
		t.Fatal("expected an invariant with no status anywhere to be included (legacy: no status means active)")
	}
}

func TestIsIncluded_Guideline_AlwaysIncluded(t *testing.T) {
	d := &Document{BoldStatus: "whatever, guidelines have no status gate"}
	if !d.IsIncluded("guideline") {
		t.Fatal("expected a guideline to always be included")
	}
}
