package grounding

import (
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

var lines = []string{
	"# ADR-099",                            // 1
	"",                                     // 2
	"Compile MUST read sidecars verbatim.", // 3
	"",                                     // 4
	"Verbatim means byte-for-byte.",        // 5
}

// TestPerAnchorGroundingEmitsOneVerdictPerAnchor is the sensitivity case: an
// ungrounded SECOND anchor must not hide behind a grounded first. The second
// anchor is usually the definition the rule actually depends on.
func TestPerAnchorGroundingEmitsOneVerdictPerAnchor(t *testing.T) {
	d := sidecar.Directive{
		Text: "Compile MUST read sidecars verbatim. (ref: ADR-099)",
		SourceExcerpts: []sidecar.SourceExcerpt{
			{LineStart: 3, LineEnd: 3, Quote: "Compile MUST read sidecars verbatim."},
			{LineStart: 5, LineEnd: 5, Quote: "Verbatim means approximately the same"}, // not in prose
		},
	}
	verdicts := classifyAll(d.Anchors(), d.Text, lines)
	if len(verdicts) != 2 {
		t.Fatalf("got %d verdict(s) for a 2-anchor directive, want 2 — collapsing to one "+
			"verdict is how an ungrounded anchor disappears", len(verdicts))
	}
	if !verdicts[0].Grounded() {
		t.Errorf("anchor 0 should be grounded, got %v", verdicts[0])
	}
	if verdicts[1].Grounded() {
		t.Errorf("anchor 1 quotes text absent from the parent but was reported grounded (%v)", verdicts[1])
	}
}

// TestPerAnchorGroundingAllGroundedControl is the isolation control: without it,
// a classifier that returned "not grounded" for everything would pass above.
func TestPerAnchorGroundingAllGroundedControl(t *testing.T) {
	d := sidecar.Directive{
		Text: "Compile MUST read sidecars verbatim. (ref: ADR-099)",
		SourceExcerpts: []sidecar.SourceExcerpt{
			{LineStart: 3, LineEnd: 3, Quote: "Compile MUST read sidecars verbatim."},
			{LineStart: 5, LineEnd: 5, Quote: "Verbatim means byte-for-byte."},
		},
	}
	for i, v := range classifyAll(d.Anchors(), d.Text, lines) {
		if !v.Grounded() {
			t.Errorf("anchor %d of an all-grounded control reported %v", i, v)
		}
	}
}

// TestPerAnchorGroundingCountsAnchorlessItem pins that an item with no anchors
// still reaches the denominator. Dropping it would turn an ungrounded directive
// into one that was never counted at all (INV-013).
func TestPerAnchorGroundingCountsAnchorlessItem(t *testing.T) {
	v := classifyAll(sidecar.Directive{Text: "ungrounded"}.Anchors(), "ungrounded", lines)
	if len(v) != 1 {
		t.Fatalf("anchorless item produced %d verdict(s), want exactly 1 so it stays in the denominator", len(v))
	}
	if v[0].Grounded() {
		t.Fatalf("anchorless item reported grounded (%v) — absence rendered as a pass", v[0])
	}
}
