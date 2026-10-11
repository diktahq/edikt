package cmd

// Red-first record (PLAN-v091 final correction pass, 2026-10-10). Mutation made
// to see each test in this file go red:
//
//	TestPhaseaPlan_RefusalLeadsWithTheContractReport: removed the ContractLead
//	  prefix in phasea.Plan -> red for all four states (first line is the old
//	  "refusing to emit a plan" text); put "(ADR-055)" back into the message ->
//	  red (id check).
//	TestReextract_DispatchRefusalLeadsWithTheContractReport: dropped the
//	  ReportContract(...).Summary prefix from the "refusing to dispatch" error
//	  -> red for v11 and v99; dropped it from the "cannot determine" error ->
//	  red for the unmeasured state.

// Layout pass (2026-10-10, fifth review): the refusal is TWO lines. Line 1 is
// "error: " + the exact Summary, the Summary appears once in all of stderr, and
// line 2 is the detail. Mutation: join the two with "; " again -> red on the
// line-1 equality in every test here.

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// assertSummaryThenDetail checks the two-line layout of a refusal on stderr.
func assertSummaryThenDetail(t *testing.T, stderr, summary string, detailPrefixes ...string) {
	t.Helper()
	lines := strings.Split(strings.TrimLeft(stderr, "\n"), "\n")
	if lines[0] != "error: "+summary {
		t.Errorf("stderr line 1 is not exactly %q\ngot: %q", "error: "+summary, lines[0])
	}
	if n := strings.Count(stderr, summary); n != 1 {
		t.Errorf("the summary appears %d times in stderr, want once:\n%s", n, stderr)
	}
	if len(lines) < 2 {
		t.Fatalf("no detail line after the summary:\n%s", stderr)
	}
	for _, pre := range detailPrefixes {
		if strings.HasPrefix(lines[1], pre) {
			return
		}
	}
	t.Errorf("line 2 %q does not start with any of %q", lines[1], detailPrefixes)
}

var internalIDRe = regexp.MustCompile(`\b(ADR|INV|SPEC|PRD|PLAN)-`)

func TestPhaseaPlan_RefusalLeadsWithTheContractReport(t *testing.T) {
	for _, kind := range unmeasuredKinds {
		t.Run(kind, func(t *testing.T) {
			p := unmeasuredProject(t, kind)
			out, _, _ := p.run(t, "doctor", "--contract-state")
			summary := strings.TrimSpace(out)
			if !strings.Contains(summary, "UNMEASURED") {
				t.Fatalf("precondition: doctor line %q", out)
			}
			perr, code := planRefusal(t, p)
			if code != 4 {
				t.Fatalf("exit %d, want 4\n%s", code, perr)
			}
			assertSummaryThenDetail(t, perr, summary, "refusing to emit a plan: ")
			if m := internalIDRe.FindString(perr); m != "" {
				t.Errorf("refusal carries an internal id %q:\n%s", m, perr)
			}
		})
	}
}

func TestReextract_DispatchRefusalLeadsWithTheContractReport(t *testing.T) {
	cases := []struct {
		name  string
		build func(t *testing.T) *bbProject
	}{
		{"rejected v11", func(t *testing.T) *bbProject { return csProject(t, 11) }},
		{"unrecorded v99", func(t *testing.T) *bbProject { return csProject(t, 99) }},
		{"unmeasured", func(t *testing.T) *bbProject { return unmeasuredProject(t, "no extractor") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.build(t)
			out, _, _ := p.run(t, "doctor", "--contract-state")
			summary := strings.TrimSpace(out)
			if summary == "" || strings.Contains(summary, "\n") {
				t.Fatalf("precondition: doctor line %q", out)
			}
			_, errOut, code := p.run(t, "gov", "reextract", "--force")
			if code == 0 {
				t.Fatalf("dispatch attempt exited 0:\n%s", errOut)
			}
			assertSummaryThenDetail(t, errOut, summary, "refusing to dispatch: ", "cannot determine ", "re-extraction could not emit a dispatch plan: ")
		})
	}
	_ = filepath.Join
}
