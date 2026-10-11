package govrun

// Red-first record (PLAN-v091 phase 1, 2026-10-10). Mutation made to see
// TestRunTwoPhase_PlanRefusalLeadsWithTheContractReport go red: stop passing
// opts.ContractLead into phasea.Plan (and drop the lift in the refusal) -> red
// (the refusal names no report line); join with "; " instead of a newline -> red
// (line-1 equality); lift the line to the end of the message
// instead of the front -> red (prefix check).

import (
	"bytes"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/model"
)

func TestRunTwoPhase_PlanRefusalLeadsWithTheContractReport(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")
	t.Setenv("CLAUDE_HOME", t.TempDir()) // no profile extractor either
	root := stageNeverInitialised(t)     // an accepted ADR with no sidecar, no extractor installed
	const lead = "extraction contract state is UNMEASURED: no extractor is installed; run /edikt:upgrade to install it"
	var errBuf, outBuf bytes.Buffer

	_, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot:  root,
		Stderr:       &errBuf,
		Stdout:       &outBuf,
		OnLoss:       "accept",
		ContractLead: func(string) string { return lead },
	}, model.RealClock{})
	if err == nil {
		t.Fatalf("a compile with nothing installed to extract must refuse\nstderr:\n%s", errBuf.String())
	}
	if !strings.HasPrefix(err.Error(), lead+"\n") {
		t.Errorf("the refusal does not lead with the contract report line:\n%v", err)
	}
	if !strings.Contains(err.Error(), "phase A: could not emit a dispatch plan") || !strings.Contains(err.Error(), "extractor definition not found") {
		t.Errorf("the refusal lost its detail:\n%v", err)
	}
	if first := strings.SplitN(errBuf.String(), "\n", 2)[0]; !strings.Contains(errBuf.String(), "error: "+lead+"\n") && first != "error: "+lead {
		t.Errorf("stderr's error line is not exactly the report line:\n%s", errBuf.String())
	}
	if lines := strings.Split(err.Error(), "\n"); len(lines) < 2 || lines[0] != lead || !strings.HasPrefix(lines[1], "phase A: could not emit a dispatch plan: refusing to emit a plan: ") {
		t.Errorf("the refusal is not summary on line 1 and phase text on line 2:\n%v", err)
	}
	if strings.Count(err.Error(), lead) != 1 {
		t.Errorf("the report line appears %d times, want once:\n%v", strings.Count(err.Error(), lead), err)
	}
}
