package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A testable phase is one whose criteria each carry a falsifier and a declared
// evidence kind: mechanical, observed or judged. Only mechanical evidence has a
// command. commands/sdlc/plan.md tells the author never to invent one, and until
// this test the loader refused the plan that instruction produces
// ("testable phases require a verify command"), so a plan could follow the
// command or load, not both.
//
// These tests pin both halves of the repair: the plan loads, and the criterion
// nobody measured is never counted as passed.

const testableMixedPlan = `schema_version: 1
plan: PLAN-mixed
phases:
  - id: "1"
    name: one mechanical, one observed
    classification: testable
    criteria:
      - id: AC-1.1
        statement: "Intent: the tool runs. Procedure: run it. Expected: exit 0. Falsifier: non-zero exit."
        verify: "true"
      - id: AC-1.2
        statement: "Intent: the user sees the warning. Procedure: the owner runs the command in a fresh session. Expected: the warning is on screen. Falsifier: it is not. Evidence: observed."
`

func loadMixed(t *testing.T) Phase {
	t.Helper()
	p := filepath.Join(t.TempDir(), "PLAN-mixed-criteria.yaml")
	if err := os.WriteFile(p, []byte(testableMixedPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadCriteria(p)
	if err != nil {
		t.Fatalf("a testable phase holding an observed criterion with no verify must load: %v", err)
	}
	ph := c.FindPhase("1")
	if ph == nil {
		t.Fatal("phase 1 not found after load")
	}
	if len(ph.Criteria) != 2 || ph.Criteria[1].Verify != "" {
		t.Fatalf("decode changed the criteria: %+v", ph.Criteria)
	}
	return *ph
}

func TestTestablePhase_CriterionWithoutVerifyLoadsAndIsUnmeasured(t *testing.T) {
	ph := loadMixed(t)
	opts := RunOptions{Cwd: t.TempDir()}

	withCmd := RunCriterion(ph, ph.Criteria[0], opts)
	if withCmd.Status != StatusPassed {
		t.Fatalf("the mechanical criterion must still run and pass, got %q", withCmd.Status)
	}

	noCmd := RunCriterion(ph, ph.Criteria[1], opts)
	if noCmd.Status != StatusUnmeasuredNoCommand {
		t.Fatalf("a testable criterion with no verify must be %q, got %q", StatusUnmeasuredNoCommand, noCmd.Status)
	}
	if !IsUnmeasured(noCmd.Status) {
		t.Fatal("IsUnmeasured does not recognise the no-command status")
	}
	if noCmd.Status == StatusPassed || strings.HasPrefix(noCmd.Status, "skipped") {
		t.Fatalf("status %q reads as passed or skipped; absence is never a pass", noCmd.Status)
	}
	if !strings.Contains(noCmd.StderrExcerpt, "no verify command declared") {
		t.Errorf("the report does not say why nothing was measured: %q", noCmd.StderrExcerpt)
	}
}

// The phase gate. `edikt verify <plan> --phase N` exits non-zero on
// AnyFailures() || AnyUnmeasured(), and its exit 0 is what marks a phase done.
// One unmeasured criterion among passes must keep the phase open.
func TestTestablePhase_CriterionWithoutVerifyGatesThePhase(t *testing.T) {
	ph := loadMixed(t)
	opts := RunOptions{Cwd: t.TempDir()}
	r := &Report{}
	for _, c := range ph.Criteria {
		r.Criteria = append(r.Criteria, RunCriterion(ph, c, opts))
	}
	r.BuildSummary()

	if r.Summary.Passed != 1 || r.Summary.Unmeasured != 1 || r.Summary.Skipped != 0 {
		t.Fatalf("want 1 passed, 1 unmeasured, 0 skipped; got %+v", r.Summary)
	}
	if got := r.Summary.Passed + r.Summary.Failed + r.Summary.Timeout + r.Summary.Unmeasured + r.Summary.Skipped; got != r.Summary.Total {
		t.Fatalf("the buckets sum to %d of %d: a status is counted nowhere", got, r.Summary.Total)
	}
	if !r.AnyUnmeasured() {
		t.Fatal("AnyUnmeasured is false, so the phase would be marked done with a criterion nobody measured")
	}
	if r.AnyFailures() {
		t.Fatal("an unmeasured criterion is not a failure")
	}
	un := r.UnmeasuredCriteria()
	if len(un) != 1 || un[0].ID != "AC-1.2" {
		t.Fatalf("the report does not name the unmeasured criterion: %+v", un)
	}
}

// A phase made only of observed or judged criteria is testable too (D242), and
// must not come out as an empty pass.
func TestTestablePhase_AllWithoutVerifyIsNotAPass(t *testing.T) {
	ph := Phase{ID: "1", Classification: ClassTestable, Criteria: []Criterion{
		{ID: "AC-1.1", Statement: "judged, with a falsifier"},
		{ID: "AC-1.2", Statement: "observed, with a falsifier"},
	}}
	r := &Report{}
	for _, c := range ph.Criteria {
		r.Criteria = append(r.Criteria, RunCriterion(ph, c, RunOptions{Cwd: t.TempDir()}))
	}
	r.BuildSummary()
	if r.Summary.Passed != 0 || r.Summary.Unmeasured != 2 || !r.AnyUnmeasured() {
		t.Fatalf("an all-judged testable phase must be 2 unmeasured and gated; got %+v", r.Summary)
	}
}
