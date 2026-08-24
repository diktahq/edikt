package hookmatch

import "testing"

// TestBinaryAbsentOutcomeIsSuppressedAndDistinct — PLAN-v080-reliability-core
// Phase 8, AC-8.7.
//
// The shim itself never writes this record: ADR-060's 2026-08-20 amendment
// prohibits a shim-side journal write (the shim has no state-directory
// contract or lock discipline), and the backfill mechanism that will
// eventually write it — the next successful `bin/edikt` invocation
// reconciling the absence window — is sibling-spec, future scope. Phase 8's
// job is narrower and mechanically checkable on its own: the outcome must
// exist as its own type, distinct from every other outcome, and
// Suppressed() must recognize it, so a future writer of this record needs no
// second decision about whether "binary was absent" counts as governance
// having run.
func TestBinaryAbsentOutcomeIsSuppressedAndDistinct(t *testing.T) {
	if !OutcomeBinaryAbsent.Suppressed() {
		t.Fatal("binary-absent must be Suppressed() — a missing binary is the most literal form of governance not running")
	}

	all := []Outcome{
		OutcomeMatched, OutcomeNoMatch, OutcomeIndexMissing, OutcomeIndexCorrupt,
		OutcomeBadPath, OutcomeIndexEmpty, OutcomeHookRan, OutcomePanicRecovered,
		OutcomeUnregisteredHookName, OutcomePayloadUnparseable, OutcomeBinaryAbsent,
	}
	seen := map[Outcome]bool{}
	for _, o := range all {
		if seen[o] {
			t.Errorf("outcome %q is declared twice — two fail-open classes that collapse to one are indistinguishable", o)
		}
		seen[o] = true
	}
	if len(seen) != len(all) {
		t.Fatalf("%d distinct outcomes for %d declared values", len(seen), len(all))
	}
	if string(OutcomeBinaryAbsent) != "binary-absent" {
		t.Fatalf(`OutcomeBinaryAbsent literal: got %q, want "binary-absent" (the absence banner's own cause tag)`, string(OutcomeBinaryAbsent))
	}
}
