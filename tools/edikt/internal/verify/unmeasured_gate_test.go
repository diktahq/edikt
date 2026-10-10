package verify

import "testing"

// unmeasured_gate_test.go — D334. `AnyFailures()` counted failed and timeout
// and nothing else, so a report whose every criterion was UNMEASURED answered
// "no failures" and the phase gate above it exited 0. A phase could be marked
// done having measured nothing: INV-013 ("absence is never a pass") broken by
// the runner that enforces INV-013 across the rest of the corpus.
//
// These tests pin BOTH halves of the fix — the new question is asked, and the
// old question is unchanged, because the scope decision lives in keeping them
// apart. `verify gov|prd|spec|all` still gate on AnyFailures() alone.

// TestAnyUnmeasured_AllUnmeasuredIsNotAPass is the defect, verbatim: every
// criterion unmeasured, nothing failed, and the report must still refuse to
// read as clean.
func TestAnyUnmeasured_AllUnmeasuredIsNotAPass(t *testing.T) {
	r := NewReport("demo", "1", "sha", []Result{
		{ID: "1.1", Status: StatusUnmeasured, StderrExcerpt: "subject not readable: src/gone.go"},
		{ID: "1.2", Status: StatusUnmeasured, StderrExcerpt: "subject not readable: test/gone.sh"},
	})
	if r.Summary.Unmeasured != 2 {
		t.Fatalf("unmeasured: got %d, want 2", r.Summary.Unmeasured)
	}
	if r.Summary.Passed != 0 || r.Summary.Failed != 0 {
		t.Fatalf("nothing passed and nothing failed; got %+v", r.Summary)
	}
	if !r.AnyUnmeasured() {
		t.Error("AnyUnmeasured: got false on an all-unmeasured report — this is the defect")
	}
	// The old question must still answer honestly. Unmeasured is not a
	// failure, and pretending it is would be the same conflation one step over.
	if r.AnyFailures() {
		t.Error("AnyFailures: got true on an all-unmeasured report — unmeasured is not a failed assertion")
	}
}

// TestAnyUnmeasured_GenuinePassIsStillAPass is the must-NOT-fail direction. A
// gate that cannot pass is not a gate.
func TestAnyUnmeasured_GenuinePassIsStillAPass(t *testing.T) {
	r := NewReport("demo", "1", "sha", []Result{
		{ID: "1.1", Status: StatusPassed},
		{ID: "1.2", Status: StatusPassed},
		{ID: "1.3", Status: StatusSkippedInformational},
		{ID: "1.4", Status: StatusSkippedOwner},
	})
	if r.AnyUnmeasured() {
		t.Errorf("AnyUnmeasured: got true with zero unmeasured criteria: %+v", r.Summary)
	}
	if r.AnyFailures() {
		t.Errorf("AnyFailures: got true on an all-pass report: %+v", r.Summary)
	}
}

// TestAnyUnmeasured_OneAmongPassesStillGates — the partial case. Three green
// and one that measured nothing is not "3 of 4 done"; the criterion that did
// not measure is the phase's promise nobody checked.
func TestAnyUnmeasured_OneAmongPassesStillGates(t *testing.T) {
	r := NewReport("demo", "1", "sha", []Result{
		{ID: "1.1", Status: StatusPassed},
		{ID: "1.2", Status: StatusPassed},
		{ID: "1.3", Status: StatusPassed},
		{ID: "1.4", Status: StatusUnmeasured, StderrExcerpt: "subject not readable: test/never-written.sh"},
	})
	if !r.AnyUnmeasured() {
		t.Error("AnyUnmeasured: got false with one unmeasured criterion among passes")
	}
	if r.AnyFailures() {
		t.Error("AnyFailures: got true — nothing failed here")
	}
}

// TestUnmeasuredCriteria_NamesWhatWasNotMeasured. The gate must say what was
// not measured, not just that something was not: a reader told only "exit 1"
// goes looking for a broken assertion that does not exist.
func TestUnmeasuredCriteria_NamesWhatWasNotMeasured(t *testing.T) {
	r := NewReport("demo", "1", "sha", []Result{
		{ID: "1.1", Status: StatusPassed, Statement: "fine"},
		{ID: "1.2", Status: StatusUnmeasured, Statement: "the gate exists",
			StderrExcerpt: "subject not readable: templates/hooks/absent.sh"},
		{ID: "1.3", Status: StatusFailed, Statement: "broken"},
		{ID: "1.4", Status: StatusUnmeasured, Statement: "the test exists",
			StderrExcerpt: "subject not readable: test/absent.sh"},
	})
	got := r.UnmeasuredCriteria()
	if len(got) != 2 {
		t.Fatalf("UnmeasuredCriteria: got %d entries, want 2 (%+v)", len(got), got)
	}
	if got[0].ID != "1.2" || got[1].ID != "1.4" {
		t.Errorf("UnmeasuredCriteria: got ids %q,%q — want 1.2,1.4 in report order", got[0].ID, got[1].ID)
	}
	// The subject is the actionable half. A list of criterion ids with no
	// subject sends the reader back to the report file to learn the one thing
	// the message existed to tell them.
	if got[0].StderrExcerpt == "" || got[1].StderrExcerpt == "" {
		t.Error("UnmeasuredCriteria: dropped the subject excerpt — the caller has nothing to name")
	}
	if len(r.UnmeasuredCriteria()) != 2 || r.Summary.Failed != 1 {
		t.Errorf("failed criteria must not be folded into unmeasured: %+v", r.Summary)
	}
}

// TestUnmeasuredCriteria_EmptyWhenNothingUnmeasured — nil, not a one-entry
// slice of a zero Result. A caller that ranges over this must print nothing.
func TestUnmeasuredCriteria_EmptyWhenNothingUnmeasured(t *testing.T) {
	r := NewReport("demo", "1", "sha", []Result{{ID: "1.1", Status: StatusPassed}})
	if got := r.UnmeasuredCriteria(); len(got) != 0 {
		t.Errorf("UnmeasuredCriteria: got %d entries on an all-pass report, want 0", len(got))
	}
}
