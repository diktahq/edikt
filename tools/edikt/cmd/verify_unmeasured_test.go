package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/internal/verify"
)

// verify_unmeasured_test.go — D334, end to end through the real binary.
//
// THE DEFECT: `edikt verify <plan-id> --phase N` gated its exit code on
// AnyFailures(), which counted `failed` and `timeout` and not `unmeasured`.
// A phase whose criteria ALL reported "unmeasured: subject unreadable" —
// every verify pointing at an artifact that does not exist — exited 0, and
// commands/sdlc/plan.md flips the phase row to `done` on exit 0. Work could
// be marked done having measured nothing.
//
// The tests below fix the SCOPE as much as the behaviour. The plan verb gates
// on unmeasured; `verify gov|prd|spec|all` deliberately do not, because their
// exit code answers "is a live rule violated", not "is this phase done", and
// an absent subject there is usually work not yet built.

// TestVerify_AllUnmeasuredPhaseFails is the defect verbatim. Every criterion
// names a file that does not exist; nothing fails, nothing passes; the phase
// must not exit 0.
func TestVerify_AllUnmeasuredPhaseFails(t *testing.T) {
	root := scaffoldVerifyProject(t, "unmeas", `phases:
  - id: "1"
    name: nothing measured
    classification: testable
    criteria:
      - id: 1.1
        statement: the forbidden pattern is absent from the source
        verify: "! grep -q FORBIDDEN src/never-created.go"
      - id: 1.2
        statement: the gate script exists and passes
        verify: "bash test/never-written.sh"
`)
	out, code := runVerify(t, root, "unmeas", "--phase", "1")
	if code == 0 {
		t.Fatalf("exit code: got 0 on an all-unmeasured phase — this is the defect D334 ruled\n%s", out)
	}
	if code != 1 {
		t.Fatalf("exit code: got %d, want 1 (the phase-gate code plan.md handles)\n%s", code, out)
	}

	// NAMED, not generic. A bare exit 1 sends the reader hunting for a failing
	// assertion that does not exist; the message has to say what was not
	// measured and which subject was missing.
	if !strings.Contains(out, "UNMEASURED") {
		t.Errorf("output does not announce UNMEASURED — it reads as a generic failure\n%s", out)
	}
	for _, want := range []string{"1.1", "1.2", "src/never-created.go", "test/never-written.sh"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not name %q — the reader cannot act on it\n%s", want, out)
		}
	}
	// And it must not be reported as a failure, which would be the same
	// conflation one column over.
	if !strings.Contains(out, "0 failed") {
		t.Errorf("summary should still report 0 failed — unmeasured is not a failed assertion\n%s", out)
	}
	if !strings.Contains(out, "2 unmeasured") {
		t.Errorf("summary should report 2 unmeasured\n%s", out)
	}
}

// TestVerify_GenuinePassStillExitsZero is the must-NOT-fail direction. A gate
// that cannot pass is not a gate, and this is the property that would break a
// real done phase.
func TestVerify_GenuinePassStillExitsZero(t *testing.T) {
	root := scaffoldVerifyProject(t, "green", `phases:
  - id: "1"
    name: real work
    classification: testable
    criteria:
      - id: 1.1
        statement: the criteria file itself is readable
        verify: "test -f docs/internal/plans/PLAN-green-criteria.yaml"
      - id: 1.2
        statement: a plain shell assertion holds
        verify: "exit 0"
`)
	out, code := runVerify(t, root, "green", "--phase", "1")
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 on a genuinely passing phase\n%s", code, out)
	}
	if strings.Contains(out, "UNMEASURED") {
		t.Errorf("clean run announced UNMEASURED\n%s", out)
	}
}

// TestVerify_UnmeasuredAmongPassesStillFails — the partial case, and the one a
// real plan hits. Three green and one that measured nothing is not "3 of 4
// done": the fourth criterion is a promise nobody checked.
func TestVerify_UnmeasuredAmongPassesStillFails(t *testing.T) {
	root := scaffoldVerifyProject(t, "partial", `phases:
  - id: "2"
    name: mostly done
    classification: testable
    criteria:
      - id: 2.1
        statement: holds
        verify: "exit 0"
      - id: 2.2
        statement: holds too
        verify: "exit 0"
      - id: 2.3
        statement: the new suite exists
        verify: "bash test/not-yet-written.sh"
`)
	out, code := runVerify(t, root, "partial", "--phase", "2")
	if code != 1 {
		t.Fatalf("exit code: got %d, want 1 — one unmeasured criterion among passes still gates\n%s", code, out)
	}
	if !strings.Contains(out, "2.3") || !strings.Contains(out, "test/not-yet-written.sh") {
		t.Errorf("output must name the one criterion that measured nothing\n%s", out)
	}
	if !strings.Contains(out, "2 passed") {
		t.Errorf("the three that did run must still be reported as measured\n%s", out)
	}
}

// TestVerify_AllowFailuresSuppressesUnmeasuredExit — --allow-failures is the
// documented report-only escape hatch for the same question ("may this phase
// flip?"), so it covers unmeasured too. It must suppress the EXIT and never
// the REPORT: silencing the message would rebuild the defect behind a flag.
func TestVerify_AllowFailuresSuppressesUnmeasuredExit(t *testing.T) {
	root := scaffoldVerifyProject(t, "allowed", `phases:
  - id: "1"
    name: nothing measured
    classification: testable
    criteria:
      - id: 1.1
        statement: the absent thing
        verify: "bash test/absent.sh"
`)
	out, code := runVerify(t, root, "allowed", "--phase", "1", "--allow-failures")
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 under --allow-failures\n%s", code, out)
	}
	if !strings.Contains(out, "UNMEASURED") || !strings.Contains(out, "test/absent.sh") {
		t.Errorf("--allow-failures must suppress the exit, not the report\n%s", out)
	}
}

// TestVerify_UnmeasuredProgressLineIsNotAQuestionMark. The per-criterion
// progress mark for unmeasured was "?", the same mark emitProgress gives a
// status it has never heard of — so the class that now gates the phase read as
// the class nobody had classified.
func TestVerify_UnmeasuredProgressLineIsNotAQuestionMark(t *testing.T) {
	root := scaffoldVerifyProject(t, "mark", `phases:
  - id: "1"
    name: nothing measured
    classification: testable
    criteria:
      - id: 1.1
        statement: absent subject
        verify: "bash test/absent.sh"
`)
	out, _ := runVerify(t, root, "mark", "--phase", "1")
	if !strings.Contains(out, "U [phase 1] 1.1") {
		t.Errorf("expected the unmeasured progress mark 'U' for criterion 1.1\n%s", out)
	}
}

// TestVerify_UnmeasuredGateIsScopedToThePlanVerb — THE SCOPE GUARD, and the
// reason this fix is not `|| Unmeasured > 0` in AnyFailures().
//
// `verify gov <id>` runs over a governance corpus where a subject may be
// legitimately absent: the rule describes work not yet built. Measured on this
// repo when the change landed, 48 unmeasured items sat across 17 sidecars, and
// gating on them would have flipped 8 sidecars carrying ZERO failures from
// green to red. That is noise, not enforcement, and `/edikt:adr:new` and
// `/edikt:gov:compile` consume those exit codes.
//
// This asserts the resolved outcome the gov verb gates on — AnyFailures() over
// a real runGovVerifies result — rather than restating its expression.
func TestVerify_UnmeasuredGateIsScopedToThePlanVerb(t *testing.T) {
	root := t.TempDir()
	sc := &sidecar.Sidecar{
		Directives: []sidecar.Directive{
			{Text: "the release workflow exists", Verify: "test -f .github/workflows/never-added.yml"},
			{Text: "a rule that does hold", Verify: "exit 0"},
		},
	}
	results := runGovVerifies(sc, root)
	if results[0].Status != verify.StatusUnmeasured {
		t.Fatalf("fixture did not produce an unmeasured result: got %q — the guard would be vacuous",
			results[0].Status)
	}

	report := verify.NewReport("gov-ADR-999", "all", "", results)
	if report.Summary.Unmeasured != 1 {
		t.Fatalf("unmeasured: got %d, want 1", report.Summary.Unmeasured)
	}
	// The gov verb's gate. It must stay false: this sidecar names no violated
	// rule, only an unbuilt one.
	if report.AnyFailures() {
		t.Error("verify gov would now exit non-zero on an unmeasured directive — " +
			"the plan-phase gate has leaked into the corpus sweep")
	}
	// And the report must still SAY nothing was measured (INV-013), which is
	// what makes the narrower exit-code scope honest rather than a hole.
	if !strings.Contains(report.Summary.Line(), "1 unmeasured") {
		t.Errorf("summary line hides the unmeasured count: %q", report.Summary.Line())
	}
	if !report.AnyUnmeasured() {
		t.Error("AnyUnmeasured false on a report holding one unmeasured item")
	}
}

// TestVerify_UnmeasuredPersistsToTheReportFile — the exit code is consumed by
// plan.md, but post-flight (commands/sdlc/post-flight.md) reads the persisted
// JSON. The status has to survive into the file, or the two consumers disagree.
func TestVerify_UnmeasuredPersistsToTheReportFile(t *testing.T) {
	root := scaffoldVerifyProject(t, "persist", `phases:
  - id: "1"
    name: nothing measured
    classification: testable
    criteria:
      - id: 1.1
        statement: absent subject
        verify: "bash test/absent.sh"
`)
	out, _ := runVerify(t, root, "persist", "--phase", "1")
	matches, err := filepath.Glob(filepath.Join(root, ".edikt", "state", "verify", "persist-phase-1-*.json"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no persisted report under .edikt/state/verify/ (err=%v)\n%s", err, out)
	}
	body, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !strings.Contains(string(body), `"unmeasured": 1`) {
		t.Errorf("persisted summary does not carry the unmeasured count:\n%s", body)
	}
	if !strings.Contains(string(body), verify.StatusUnmeasured) {
		t.Errorf("persisted criterion does not carry status %q:\n%s", verify.StatusUnmeasured, body)
	}
}
