package phasea

import (
	"strings"
	"testing"
)

// A VERDICT WITH NO LABEL, AND AN INSTRUCTION THAT CONTRADICTED THE EXIT CODE.
//
// The summary built its verdict from a map holding three entries while the
// verb returns seven. An exit-2 run fell through to an empty label, and the
// switch beneath it had cases only for two codes, so a REJECTED plan printed
// no instruction of its own — leaving the reader with the retry line that
// belongs to exit 1. The documented table says 1 is retryable and 2 is not,
// so the message and the contract told the caller opposite things: re-dispatch
// what must never be re-dispatched.
//
// This is the absence-shaped bug one function away from the summary line that
// hid seven unmeasured criteria: a lookup that misses renders as nothing, and
// nothing reads as fine.

// documentedVerifyExitCodes is the contract as the command's help states it.
// Kept here as a literal list rather than derived from the map under test —
// a test that reads the same structure it is checking cannot catch an
// omission in it.
var documentedVerifyExitCodes = []int{
	ExitVerifyOK,
	ExitVerifyIncomplete,
	ExitVerifyPlanRejected,
	ExitVerifyMalformed,
	ExitVerifyFailed,
	ExitVerifyDiverged,
	ExitVerifyRestoreMissing,
}

func TestVerifyVerdicts_EveryDocumentedExitCodeHasALabel(t *testing.T) {
	for _, code := range documentedVerifyExitCodes {
		v, ok := verifyVerdicts[code]
		if !ok {
			t.Errorf("exit %d has no verdict entry — it would render as a blank label", code)
			continue
		}
		if strings.TrimSpace(v.label) == "" {
			t.Errorf("exit %d has an empty label", code)
		}
	}
	if len(verifyVerdicts) != len(documentedVerifyExitCodes) {
		t.Errorf("the table has %d entries for %d documented codes — one of the two is out of date",
			len(verifyVerdicts), len(documentedVerifyExitCodes))
	}
}

// Every code EXCEPT success carries an instruction. Success is the one case
// where "do nothing" is the whole answer.
func TestVerifyVerdicts_EveryFailingCodeHasItsOwnNextStep(t *testing.T) {
	for _, code := range documentedVerifyExitCodes {
		v := verifyVerdicts[code]
		if code == ExitVerifyOK {
			if v.nextStep != "" {
				t.Errorf("a consumed plan needs no instruction, got %q", v.nextStep)
			}
			continue
		}
		if strings.TrimSpace(v.nextStep) == "" {
			t.Errorf("exit %d has no next step — the reader is left acting on the line printed above it", code)
		}
	}
}

// The rejection's instruction must CONTRADICT the retry line rather than
// merely differ from it: this is the sentence bok read and correctly
// disbelieved.
func TestVerifySummary_RejectionSaysDoNotReDispatch(t *testing.T) {
	r := &Receipt{Items: []ReceiptItem{{ItemID: "adr-900-d0", Status: StatusParentHashMismatch, Attempts: 1}}}
	msg := verifySummary("pap-000000000000", r, map[string]string{"adr-900-d0": "the artifact changed after planning"}, ExitVerifyPlanRejected)

	if strings.Contains(msg, "re-dispatch each non-terminal item") {
		t.Fatalf("a rejected plan must NOT carry the retry instruction:\n%s", msg)
	}
	for _, want := range []string{"REJECTED", "fresh plan_id", "do NOT re-dispatch"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the rejection must say %q:\n%s", want, msg)
		}
	}
}

// And the retryable case keeps its own instruction, so the two remain
// distinguishable in the transcript.
func TestVerifySummary_IncompleteKeepsTheRetryInstruction(t *testing.T) {
	r := &Receipt{Items: []ReceiptItem{{ItemID: "adr-900-d0", Status: StatusOutputMissing, Attempts: 1}}}
	msg := verifySummary("pap-000000000000", r, map[string]string{"adr-900-d0": "no sidecar on disk"}, ExitVerifyIncomplete)
	if !strings.Contains(msg, "INCOMPLETE") || !strings.Contains(msg, "re-dispatch each non-terminal item") {
		t.Fatalf("the retryable case must keep its instruction:\n%s", msg)
	}
}

// An exit code with no entry renders LOUDLY, not blankly. This is the
// falsification: delete an entry from the table and the summary says so
// instead of printing a gap.
func TestVerifySummary_AnUnlabelledCodeSaysSo(t *testing.T) {
	r := &Receipt{Items: []ReceiptItem{{ItemID: "adr-900-d0", Status: StatusOK, Attempts: 1}}}
	msg := verifySummary("pap-000000000000", r, map[string]string{}, 99)
	if !strings.Contains(msg, "UNLABELLED VERDICT") || !strings.Contains(msg, "99") {
		t.Fatalf("an exit code with no entry must be named as a defect in the table:\n%s", msg)
	}
}

// #7 (c3-c): the INCOMPLETE line told the caller to re-dispatch "verbatim"
// while the receipt carried a correction_prompt for exactly that retry, so a
// caller following the summary sent the unseeded re-run the receipt exists
// to prevent. It now names the correction pass, and says what to do with an
// item that has no prompt.
func TestVerifyVerdicts_IncompleteNeverSaysVerbatim(t *testing.T) {
	next := verifyVerdicts[ExitVerifyIncomplete].nextStep
	if strings.Contains(strings.ToLower(next), "verbatim") {
		t.Fatalf("the INCOMPLETE next step still says verbatim: %q", next)
	}
	for _, want := range []string{"correction pass", "correction_prompt", "empty correction_prompt is reported, not re-dispatched"} {
		if !strings.Contains(next, want) {
			t.Errorf("the INCOMPLETE next step omits %q: %q", want, next)
		}
	}
}
