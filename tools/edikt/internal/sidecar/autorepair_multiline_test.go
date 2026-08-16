package sidecar

import (
	"strings"
	"testing"
)

// Field bug (bok-services 2026-08-07, issue 5b/5c): extractor quote anchors
// were frequently not byte-exact — wrapped lines joined into one string,
// bullets/indentation stripped — so the staleness checker flagged them and
// the LLM re-anchor loop became whack-a-mole (three artifacts needed manual
// re-anchoring). The deterministic autorepair must resolve the whole class:
// whitespace-normalized matching across multi-line windows, all anchors in
// one pass.

func src(lines ...string) []string { return lines }

func TestAutoRepair_WrappedLineJoinedQuote(t *testing.T) {
	lines := src(
		"# ADR-015",   // 1
		"",            // 2
		"## Decision", // 3
		"",            // 4
		"The compile pipeline MUST write generated", // 5
		"directive metadata only to the sidecar.",   // 6
	)
	sc := &Sidecar{
		SchemaVersion: 1,
		Directives: []Directive{{
			Text: "x",
			SourceExcerpt: SourceExcerpt{
				LineStart: 12, LineEnd: 12,
				// Extractor joined the two wrapped lines with a space.
				Quote: "The compile pipeline MUST write generated directive metadata only to the sidecar.",
			},
		}},
	}
	n := AutoRepairAnchors(sc, lines)
	if n != 1 {
		t.Fatalf("expected 1 repaired anchor, got %d", n)
	}
	se := sc.Directives[0].SourceExcerpt
	if se.LineStart != 5 || se.LineEnd != 6 {
		t.Errorf("expected anchor 5-6, got %d-%d", se.LineStart, se.LineEnd)
	}
	if IsStale(sc, lines) {
		t.Error("sidecar must not be stale after wrapped-line repair")
	}
}

func TestAutoRepair_BulletAndIndentStrippedQuote(t *testing.T) {
	lines := src(
		"## Statement",                      // 1
		"",                                  // 2
		"- Hooks MUST construct JSON via a", // 3
		"  structured serializer, always.",  // 4
	)
	sc := &Sidecar{
		SchemaVersion: 1,
		Directives: []Directive{{
			Text: "x",
			SourceExcerpt: SourceExcerpt{
				LineStart: 9, LineEnd: 9,
				// Bullet marker and indentation stripped, lines joined.
				Quote: "Hooks MUST construct JSON via a structured serializer, always.",
			},
		}},
	}
	if n := AutoRepairAnchors(sc, lines); n != 1 {
		t.Fatalf("expected 1 repaired anchor, got %d", n)
	}
	se := sc.Directives[0].SourceExcerpt
	if se.LineStart != 3 || se.LineEnd != 4 {
		t.Errorf("expected anchor 3-4, got %d-%d", se.LineStart, se.LineEnd)
	}
	if IsStale(sc, lines) {
		t.Error("sidecar must not be stale after bullet-strip repair")
	}
}

func TestAutoRepair_AllAnchorsInOnePass(t *testing.T) {
	lines := src(
		"## Decision",            // 1
		"Alpha rule holds here.", // 2
		"Beta rule wraps across", // 3
		"two source lines.",      // 4
		"Gamma rule holds too.",  // 5
	)
	sc := &Sidecar{
		SchemaVersion: 1,
		Directives: []Directive{
			{Text: "a", SourceExcerpt: SourceExcerpt{LineStart: 40, LineEnd: 40, Quote: "Alpha rule holds here."}},
			{Text: "b", SourceExcerpt: SourceExcerpt{LineStart: 41, LineEnd: 41, Quote: "Beta rule wraps across two source lines."}},
			{Text: "c", SourceExcerpt: SourceExcerpt{LineStart: 42, LineEnd: 42, Quote: "Gamma rule holds too."}},
		},
	}
	if n := AutoRepairAnchors(sc, lines); n != 3 {
		t.Fatalf("one pass must repair all 3 drifted anchors, got %d", n)
	}
	if IsStale(sc, lines) {
		t.Error("sidecar must be fully fresh after one repair pass")
	}
}

func TestExcerptStale_NormalizedFallbackAtRecordedLines(t *testing.T) {
	// Quote differs from source only in collapsed whitespace, anchored at
	// the right lines already — must NOT be reported stale (and so must
	// not trigger an LLM dispatch at all).
	lines := src(
		"## Decision",
		"The  rule   MUST hold.", // double spaces in source
	)
	sc := &Sidecar{
		SchemaVersion: 1,
		Directives: []Directive{{
			Text:          "x",
			SourceExcerpt: SourceExcerpt{LineStart: 2, LineEnd: 2, Quote: "The rule MUST hold."},
		}},
	}
	if IsStale(sc, lines) {
		t.Error("whitespace-only quote drift at the recorded lines must not be stale")
	}
}

func TestAutoRepair_AbsentQuoteStillUntouched(t *testing.T) {
	lines := src("## Decision", "Something else entirely.")
	sc := &Sidecar{
		SchemaVersion: 1,
		Directives: []Directive{{
			Text:          "x",
			SourceExcerpt: SourceExcerpt{LineStart: 2, LineEnd: 2, Quote: "This text no longer exists."},
		}},
	}
	if n := AutoRepairAnchors(sc, lines); n != 0 {
		t.Fatalf("absent quote must not be 'repaired', got %d", n)
	}
	if !IsStale(sc, lines) {
		t.Error("genuinely missing quote must remain stale (LLM dispatch required)")
	}
}

func TestAutoRepair_TiebreakerStillNearest(t *testing.T) {
	// Repeated quote: nearest-to-existing-LineStart still wins (ADR-038 case).
	lines := src(
		"dup rule text.", // 1
		"filler",         // 2
		"dup rule text.", // 3
	)
	sc := &Sidecar{
		SchemaVersion: 1,
		Directives: []Directive{{
			Text:          "x",
			SourceExcerpt: SourceExcerpt{LineStart: 4, LineEnd: 4, Quote: "dup rule text."},
		}},
	}
	if n := AutoRepairAnchors(sc, lines); n != 1 {
		t.Fatalf("expected repair, got %d", n)
	}
	if got := sc.Directives[0].SourceExcerpt.LineStart; got != 3 {
		t.Errorf("nearest match to line 4 is line 3, got %d", got)
	}
}

func TestNormalizeWS(t *testing.T) {
	got := normalizeWS("  - The\trule   MUST   hold. ")
	if got != "- The rule MUST hold." && !strings.Contains(got, "MUST hold.") {
		t.Errorf("normalizeWS collapsed unexpectedly: %q", got)
	}
}
