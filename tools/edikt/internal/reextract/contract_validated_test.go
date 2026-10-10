package reextract

import (
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
)

// Reported from the field 2026-09-18 against 0.9.0: `gov reextract --status`
// said `prompt_version v9, eligible 87, done 0` while a dispatch of those same
// 87 refused outright, and `/edikt:upgrade` §7 offered the work with no hint it
// would be refused. Two paths, one record, opposite answers.
//
// The fix is not a second check kept in sync by hand. ContractValidated is ONE
// function; the dispatcher refuses on it and Status reports it. These tests pin
// that arrangement, because a duplicated rule is what produced the divergence.

func TestContractValidated_RejectedIsNotTheSameFactAsUnrecorded(t *testing.T) {
	// v5 is in the shipped record's `rejected:` list; v999 is in neither list.
	// A reader needs to know which — one was judged and refused, the other was
	// never put to the question — so the two must not produce the same message.
	okRejected, whyRejected := ContractValidated("v5")
	okUnknown, whyUnknown := ContractValidated("v999")

	if okRejected || okUnknown {
		t.Fatalf("neither v5 (rejected) nor v999 (unrecorded) may validate: got %v / %v", okRejected, okUnknown)
	}
	if !strings.Contains(whyRejected, "REJECTED") {
		t.Errorf("a rejected contract must say so — it is a decision already taken, not a gap.\ngot: %s", whyRejected)
	}
	if strings.Contains(whyUnknown, "REJECTED") {
		t.Errorf("an unrecorded contract must NOT be reported as rejected.\ngot: %s", whyUnknown)
	}
	if whyRejected == whyUnknown {
		t.Error("rejected and unrecorded produced identical text; the distinction is the point")
	}
}

func TestContractValidated_AcceptsOnlyWhatTheRecordValidates(t *testing.T) {
	// v4 is the one validated entry in the shipped record. If this ever fails,
	// the record changed and every other assertion here needs re-reading.
	if ok, why := ContractValidated("v4"); !ok {
		t.Fatalf("v4 is recorded as validated and must pass: %s", why)
	}
	if ok, _ := ContractValidated(""); ok {
		t.Error("an empty contract version must never validate")
	}
	if ok, _ := ContractValidated(phasea.ExtractorPromptVersionUnknown); ok {
		t.Error("an UNRESOLVABLE contract version must never validate — not knowing is not proof")
	}
}

func TestContractValidated_DoesNotSubstringMatchTheRejectedProse(t *testing.T) {
	// The original check greped the file and matched "v4" inside the PROSE of
	// the rejected-v5 entry, letting an unrecorded contract through. The record
	// still carries that prose, so this is a live regression guard, not a
	// hypothetical: a substring implementation passes v4 for the wrong reason
	// and would also pass any version whose string appears anywhere in it.
	for _, v := range []string{"v3", "v5", "v40", "v04"} {
		if ok, _ := ContractValidated(v); ok {
			t.Errorf("%s is not under `validated:` and must not pass — a substring match on the "+
				"record's prose is how this broke before", v)
		}
	}
}
