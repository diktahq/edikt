package audit

import "testing"

// D118 (3)'s AC: a table of finding-class → subtractive option, with a test
// asserting each listed class prints both moves. This is that test — it does
// NOT grep any message string (that would be the INV-014 mistake the
// coordinator named); it asserts the TABLE itself is complete, which is the
// single source every message-emitting caller is meant to consult.

func TestRemedyTable_EveryEntryHasBothMovesOrARecordedReason(t *testing.T) {
	for _, r := range RemedyTable {
		if r.FindingClass == "" {
			t.Fatalf("a remedy entry with no FindingClass: %+v", r)
		}
		if r.Additive == "" {
			t.Errorf("%s: Additive is empty — every listed class must state the additive move", r.FindingClass)
		}
		if r.Subtractive == "" && r.NoSubtractiveBecause == "" {
			t.Errorf("%s: neither Subtractive nor NoSubtractiveBecause is set — a class with nothing to say "+
				"here is a gap in the table, not a legitimate silence (D118 (3): 'messages with no subtractive "+
				"option stay as they are and are listed as such, so the table is complete rather than partial')",
				r.FindingClass)
		}
		if r.Subtractive != "" && r.NoSubtractiveBecause != "" {
			t.Errorf("%s: both Subtractive and NoSubtractiveBecause are set — exactly one applies", r.FindingClass)
		}
	}
}

// The five classes D118 named explicitly (bok's own findings) must be
// present — this is the "start with the five bok actually walked into"
// requirement, pinned so a future edit can't quietly drop one.
func TestRemedyTable_CoversTheFiveBokFindings(t *testing.T) {
	want := []string{
		"unresolved-citation",
		"pending-topic-description",
		"missing-sidecar-non-accepted",
		"directive-contradicted-by-later-record",
		"anchor-drift",
	}
	for _, fc := range want {
		if _, ok := RemedyFor(fc); !ok {
			t.Errorf("expected RemedyTable to cover %q, it does not", fc)
		}
	}
}

func TestRemedyFor_UnknownClassReportsNotFound(t *testing.T) {
	r, ok := RemedyFor("no-such-finding-class")
	if ok {
		t.Fatalf("expected ok=false for an unlisted class, got a remedy: %+v", r)
	}
}

// The two moves for a given class must actually differ — a table entry that
// says the same thing twice under two field names is not two moves.
func TestRemedyTable_AdditiveAndSubtractiveDiffer(t *testing.T) {
	for _, r := range RemedyTable {
		if r.Subtractive != "" && r.Additive == r.Subtractive {
			t.Errorf("%s: Additive and Subtractive are identical text", r.FindingClass)
		}
	}
}
