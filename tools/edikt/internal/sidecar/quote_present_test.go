package sidecar

import (
	"strings"
	"testing"
)

// QuotePresent answers "is this passage still somewhere in the parent?" in the
// same normalised frame canonicalSliceInRange works in: every run of
// whitespace, line breaks included, is one space. verify's survival check asks
// it of a prior anchor's quote, so a wrapped or re-indented quote must still
// count as present, and an empty one must never count as present.
func TestQuotePresent_NormalisedFrame(t *testing.T) {
	parent := strings.Split("# Title\n\n"+
		"Every hook MUST emit JSON on stdout,\n"+
		"  wrapped across two lines.\n\n"+
		"A second paragraph.\n", "\n")

	cases := []struct {
		name  string
		quote string
		want  bool
	}{
		{"exact single line", "A second paragraph.", true},
		{"inner slice of a line", "MUST emit JSON", true},
		{"spans a wrap, joined with one space", "Every hook MUST emit JSON on stdout, wrapped across two lines.", true},
		{"spans a wrap, own whitespace", "Every hook MUST emit JSON on stdout,\n\t wrapped", true},
		{"spans a blank line", "two lines. A second", true},
		{"absent prose", "Every hook MAY emit YAML.", false},
		{"different words in the same place", "Every hook MUST emit XML", false},
		{"empty quote", "", false},
		{"whitespace-only quote", " \n\t ", false},
	}
	for _, tc := range cases {
		if got := QuotePresent(tc.quote, parent); got != tc.want {
			t.Errorf("%s: QuotePresent(%q) = %v, want %v", tc.name, tc.quote, got, tc.want)
		}
	}

	// Containment between two quotes is the same question with a one-line
	// "parent": verify uses it to decide whether an output anchor and a prior
	// anchor quote the same passage.
	if !QuotePresent("emit JSON", []string{"Every hook MUST\nemit JSON."}) {
		t.Error("a quote contained in another, across its line break, must be present")
	}
	if QuotePresent("emit JSON", nil) {
		t.Error("nothing is present in an empty parent")
	}
}
