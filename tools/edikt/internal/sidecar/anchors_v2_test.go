package sidecar

import "testing"

var anchorTestLines = []string{
	"# ADR-099",                                       // 1
	"",                                                // 2
	"Compile MUST read sidecars verbatim.",            // 3
	"",                                                // 4
	"\"Verbatim\" means byte-for-byte, including WS.", // 5
}

func threeAnchorDirective() Directive {
	return Directive{
		Text: "Compile MUST read sidecars verbatim. (ref: ADR-099)",
		SourceExcerpts: []SourceExcerpt{
			{LineStart: 3, LineEnd: 3, Quote: "Compile MUST read sidecars verbatim.", Role: "statement"},
			{LineStart: 5, LineEnd: 5, Quote: "\"Verbatim\" means byte-for-byte", Role: "definition"},
			{LineStart: 1, LineEnd: 1, Quote: "# ADR-099", Role: "scope"},
		},
	}
}

// TestAnyAnchorStale_OneStaleAnchorIsStale is the sensitivity case: a directive
// grounded in three spans is WRONG the moment one stops matching, because the
// rule depended on that span.
func TestAnyAnchorStale_OneStaleAnchorIsStale(t *testing.T) {
	d := threeAnchorDirective()
	d.SourceExcerpts[1].Quote = "\"Verbatim\" means roughly the same shape"

	stale, idx := AnyAnchorStale(d, anchorTestLines)
	if !stale {
		t.Fatal("a directive with one non-matching anchor reported NOT stale — the better " +
			"the extraction, the quieter the alarm, which inverts the signal")
	}
	if idx != 1 {
		t.Fatalf("named the wrong anchor: got index %d, want 1 — pointing a reader at prose "+
			"that is fine is worse than saying nothing", idx)
	}
}

// TestAnyAnchorStale_AllFreshIsNotStale is the isolation control. Without it, a
// function returning true unconditionally would pass the test above.
func TestAnyAnchorStale_AllFreshIsNotStale(t *testing.T) {
	if stale, idx := AnyAnchorStale(threeAnchorDirective(), anchorTestLines); stale {
		t.Fatalf("all-fresh 3-anchor control reported stale at anchor %d — the check fires "+
			"on everything and proves nothing", idx)
	}
}

// TestAnyAnchorStale_NoAnchorsIsUnmeasured pins that an ungrounded directive is
// not silently reported as clean. It returns (false, -1), and -1 is the signal
// callers must treat as "nothing was compared" (INV-013).
func TestAnyAnchorStale_NoAnchorsIsUnmeasured(t *testing.T) {
	stale, idx := AnyAnchorStale(Directive{Text: "ungrounded"}, anchorTestLines)
	if stale || idx != -1 {
		t.Fatalf("got (%v, %d), want (false, -1) as the unmeasured signal", stale, idx)
	}
	if len((Directive{Text: "ungrounded"}).Anchors()) != 0 {
		t.Fatal("Anchors() invented an anchor for a directive that has none")
	}
}

// TestAnchorsUnifiesV1AndV2 pins the single read path. Two decode fields that
// must agree are unified by one accessor, or they drift.
func TestAnchorsUnifiesV1AndV2(t *testing.T) {
	v1 := Directive{Text: "x", SourceExcerpt: SourceExcerpt{LineStart: 3, LineEnd: 3, Quote: "Compile MUST read sidecars verbatim."}}
	if got := v1.Anchors(); len(got) != 1 || got[0].LineStart != 3 {
		t.Fatalf("v1 singular did not normalise to one anchor: %+v", got)
	}
	if got := threeAnchorDirective().Anchors(); len(got) != 3 {
		t.Fatalf("v2 plural did not yield 3 anchors: %d", len(got))
	}
	// A v1 sidecar and a v2 sidecar carrying the same anchor must be judged
	// identically — that is the whole point of routing both through Anchors().
	v2 := Directive{Text: "x", SourceExcerpts: []SourceExcerpt{{LineStart: 3, LineEnd: 3, Quote: "Compile MUST read sidecars verbatim."}}}
	s1, _ := AnyAnchorStale(v1, anchorTestLines)
	s2, _ := AnyAnchorStale(v2, anchorTestLines)
	if s1 != s2 {
		t.Fatalf("v1 and v2 forms of the same anchor disagreed: v1=%v v2=%v", s1, s2)
	}
}
