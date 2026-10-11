package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// THE TRAP THIS GUARDS.
//
// RunCriterion's switch had no default, so an unrecognised classification fell
// through and EXECUTED. Adding "owner" to the enum without adding a case would
// therefore have made owner-gated phases run their verifies — the exact
// opposite of what the classification is for, and silently, because the verify
// would simply run and report.
//
// The verify here writes a file. If the criterion is executed the file exists,
// and no amount of status-string assertion can hide it.
func TestRunCriterion_OwnerPhaseIsSkippedNotExecuted(t *testing.T) {
	dir := t.TempDir()
	canary := filepath.Join(dir, "executed.canary")

	phase := Phase{ID: "7", Name: "Owner ruling", Classification: ClassOwner}
	c := Criterion{
		ID:        "AC-7.1",
		Statement: "The owner rules on the posture dial.",
		Verify:    "touch " + canary,
	}

	res := RunCriterion(phase, c, RunOptions{Cwd: dir})

	if _, err := os.Stat(canary); err == nil {
		t.Fatal("OWNER PHASE EXECUTED ITS VERIFY — the switch fell through to RunOne")
	}
	if res.Status != StatusSkippedOwner {
		t.Errorf("status = %q, want %q", res.Status, StatusSkippedOwner)
	}
}

// owner must NOT collapse into informational. Informational means "nothing to
// check"; owner means "a person must act and may not have". A reader scanning a
// report for outstanding human work needs to tell them apart, which is the
// whole reason the classification exists rather than reusing informational.
func TestRunCriterion_OwnerIsDistinctFromInformational(t *testing.T) {
	c := Criterion{ID: "AC-1.1", Statement: "s"}
	owner := RunCriterion(Phase{ID: "1", Classification: ClassOwner}, c, RunOptions{})
	info := RunCriterion(Phase{ID: "1", Classification: ClassInformational}, c, RunOptions{})
	if owner.Status == info.Status {
		t.Fatalf("owner and informational produce the same status %q — the distinction is lost", owner.Status)
	}
}

// An unrecognised classification must FAIL, never execute and never silently
// skip. Executing runs criteria under semantics nobody defined; skipping is a
// control that cannot fail.
func TestRunCriterion_UnknownClassificationFailsRatherThanRunning(t *testing.T) {
	dir := t.TempDir()
	canary := filepath.Join(dir, "executed.canary")
	res := RunCriterion(
		Phase{ID: "1", Classification: "wishful"},
		Criterion{ID: "AC-1.1", Statement: "s", Verify: "touch " + canary},
		RunOptions{Cwd: dir},
	)
	if _, err := os.Stat(canary); err == nil {
		t.Fatal("an unrecognised classification EXECUTED its verify")
	}
	if res.Status != StatusFailed {
		t.Errorf("status = %q, want %q", res.Status, StatusFailed)
	}
	if !strings.Contains(res.StderrExcerpt, "wishful") {
		t.Errorf("the failure does not name the offending classification: %q", res.StderrExcerpt)
	}
}

// Every status must land in a summary bucket. A status missing from
// BuildSummary counts in Total and nothing else, so Total stops equalling the
// sum and the report under-states without saying so.
func TestBuildSummary_OwnerSkipsAreCounted(t *testing.T) {
	r := &Report{Criteria: []Result{
		{ID: "a", Status: StatusPassed},
		{ID: "b", Status: StatusSkippedOwner},
		{ID: "c", Status: StatusSkippedOwner},
	}}
	r.BuildSummary()
	if r.Summary.Skipped != 2 {
		t.Errorf("owner skips not counted: skipped=%d", r.Summary.Skipped)
	}
	sum := r.Summary.Passed + r.Summary.Failed + r.Summary.Timeout + r.Summary.Skipped
	if sum != r.Summary.Total {
		t.Errorf("Total=%d but buckets sum to %d — a status escaped every bucket", r.Summary.Total, sum)
	}
}

// An owner-gated phase must LOAD. Before this, the schema refused `owner` and
// refused any unanticipated key, so the plan could not be parsed at all.
func TestLoadCriteria_AcceptsOwnerPhaseAndPhaseMetadata(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "PLAN-x-criteria.yaml")
	body := `schema_version: 1
plan: PLAN-x
phases:
  - id: "7"
    name: Owner ruling
    classification: owner
    phase_metadata:
      gate: "Owner decides whether the posture dial ships default-deny."
      raised_by: daniel
    criteria:
      - id: AC-7.1
        statement: The owner has ruled on the posture dial.
`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cf, err := LoadCriteria(p)
	if err != nil {
		t.Fatalf("owner-gated phase failed to load: %v", err)
	}
	ph := cf.FindPhase("7")
	if ph == nil {
		t.Fatal("phase 7 not found")
	}
	if ph.Classification != ClassOwner {
		t.Errorf("classification = %q", ph.Classification)
	}
	if got := ph.PhaseMetadata["gate"]; got == nil {
		t.Error("phase_metadata did not survive the load — the text its author wrote is gone")
	}
}
