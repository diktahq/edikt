package hookmatch

// F-052 — BounceBudget was documented (this file's own 14-line comment) and
// never wired: AlreadyBounced was a boolean, per-context, bounce-once-forever
// marker, and hookmatch.BounceBudget was read by nothing at all — provable by
// deletion, per the ledger row this test discharges. These tests exercise the
// counted budget: per-context dedup must hold exactly as before (see
// test/unit/hooks/test_inject_dedup_state.sh, AC-3.5, unchanged by this
// change), and a NEW aggregate ceiling must apply across distinct contexts.

import (
	"testing"
)

func testEntries() []Entry {
	return []Entry{{ID: "INV-901:d01", Grade: "must", Text: "test directive"}}
}

func TestAlreadyBounced_PerContextDedupUnchanged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	entries := testEntries()

	r1, err := AlreadyBounced("s1", "parent", entries, 8)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if !r1.Bounce || r1.BudgetExhausted {
		t.Fatalf("first call for a fresh context should bounce, got %+v", r1)
	}

	r2, err := AlreadyBounced("s1", "parent", entries, 8)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if r2.Bounce || r2.BudgetExhausted {
		t.Fatalf("repeat call for the SAME context must be silent (per-context dedup), got %+v", r2)
	}
}

func TestAlreadyBounced_UnknownContextAlwaysDelivers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	entries := testEntries()

	// UNKNOWN MEANS DELIVER, unconditionally, ignoring the budget. Call it
	// enough times to blow past a budget of 1 and confirm it never converts
	// to budget-exhausted or goes silent.
	for i := 0; i < 5; i++ {
		r, err := AlreadyBounced("s1", "", entries, 1)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if !r.Bounce || r.BudgetExhausted {
			t.Fatalf("call %d: an unresolvable context must always bounce, got %+v", i, r)
		}
	}
}

// TestAlreadyBounced_BudgetCapsTheAggregateAcrossContexts is the core F-052
// property: per-context keying bounds ONE context to one bounce, but does
// nothing to bound how many DISTINCT contexts bounce for the same directive
// set in one session. A session that dispatches more subagents than the
// budget against the same governed path must stop denying past the budget —
// and must not go silent either.
func TestAlreadyBounced_BudgetCapsTheAggregateAcrossContexts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	entries := testEntries()
	const budget = 3

	for i := 1; i <= budget; i++ {
		ctx := string(rune('a' + i))
		r, err := AlreadyBounced("s1", ctx, entries, budget)
		if err != nil {
			t.Fatalf("context %s: %v", ctx, err)
		}
		if !r.Bounce || r.BudgetExhausted {
			t.Fatalf("context %s (bounce %d of %d, within budget) should bounce, got %+v", ctx, i, budget, r)
		}
	}

	// The (budget+1)th DISTINCT context still has never been told — per-
	// context dedup alone would bounce it — but the aggregate for this
	// directive set is spent.
	over := string(rune('a' + budget + 1))
	r, err := AlreadyBounced("s1", over, entries, budget)
	if err != nil {
		t.Fatalf("over-budget context: %v", err)
	}
	if r.Bounce {
		t.Fatalf("a bounce past the aggregate budget must NOT deny, got %+v", r)
	}
	if !r.BudgetExhausted {
		t.Fatalf("a bounce past the aggregate budget must be reported as budget-exhausted (proceed LOUDLY), not silently dropped, got %+v", r)
	}

	// And a REPEAT of that same over-budget context must go fully silent —
	// it has now been told (as advisory), so per-context dedup applies.
	r2, err := AlreadyBounced("s1", over, entries, budget)
	if err != nil {
		t.Fatalf("repeat of over-budget context: %v", err)
	}
	if r2.Bounce || r2.BudgetExhausted {
		t.Fatalf("a repeat visit from an already-told context must be silent, got %+v", r2)
	}
}

// TestAlreadyBounced_DifferentDirectiveSetsHaveIndependentBudgets proves the
// budget key is scoped to (session, directive-set content) — a session must
// not have its budget for one MUST-grade rule consumed by bounces on an
// unrelated one.
func TestAlreadyBounced_DifferentDirectiveSetsHaveIndependentBudgets(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const budget = 1
	setA := []Entry{{ID: "INV-901:d01", Grade: "must", Text: "rule A"}}
	setB := []Entry{{ID: "INV-902:d01", Grade: "must", Text: "rule B"}}

	rA, err := AlreadyBounced("s1", "ctx-a", setA, budget)
	if err != nil || !rA.Bounce {
		t.Fatalf("set A first bounce: %+v, %v", rA, err)
	}
	// Spend set A's budget with a second context.
	rA2, err := AlreadyBounced("s1", "ctx-b", setA, budget)
	if err != nil || rA2.Bounce || !rA2.BudgetExhausted {
		t.Fatalf("set A should be budget-exhausted for a second context: %+v, %v", rA2, err)
	}

	// Set B, same session, same contexts even — independent key, independent budget.
	rB, err := AlreadyBounced("s1", "ctx-a", setB, budget)
	if err != nil || !rB.Bounce {
		t.Fatalf("set B must bounce on its own first context, unaffected by set A: %+v, %v", rB, err)
	}
}

// TestAlreadyBounced_F080_UnrelatedRecompileDoesNotReBounce is F-080's own
// property: recompiling gov for a COMPLETELY UNRELATED artifact used to
// change the dedup key (it embedded the whole directive-index file's SHA-256)
// even though this entry's own content — the only thing that changed what the
// agent would be told — was byte-identical. That cost a retry on nearly every
// governed write across two days: any recompile anywhere re-bounced
// everything. The key is now built ONLY from the matched entries' own fields
// (entriesContentHash), so it must stay silent across repeat calls even when
// the caller's index-derived state (simulated here by nothing changing about
// entries at all, mirroring an unrelated recompile) would have moved.
func TestAlreadyBounced_F080_UnrelatedRecompileDoesNotReBounce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	entries := testEntries()

	r1, err := AlreadyBounced("s1", "parent", entries, 8)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if !r1.Bounce {
		t.Fatalf("first call for a fresh context should bounce, got %+v", r1)
	}

	// A second call with byte-identical entries — the only thing an
	// unrelated recompile could not have changed for THIS directive set.
	// Before F-080's fix this was indistinguishable from the first call
	// whenever the (now-removed) whole-index-hash argument differed, which
	// it always did after any recompile.
	r2, err := AlreadyBounced("s1", "parent", entries, 8)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if r2.Bounce {
		t.Fatalf("F-080: unchanged entry content must stay silent (per-context dedup), got %+v", r2)
	}
}

// TestEntriesContentHash_SensitiveToTextChangeAlone proves the key did not
// lose content-sensitivity when the whole-index component was dropped: an
// entry keeping the same ID but changing TEXT (a reworded rule) must still
// produce a different key, so a genuine edit still earns a fresh bounce.
func TestEntriesContentHash_SensitiveToTextChangeAlone(t *testing.T) {
	before := []Entry{{ID: "INV-901:d01", Grade: "must", Text: "original wording"}}
	after := []Entry{{ID: "INV-901:d01", Grade: "must", Text: "reworded — the obligation changed"}}
	if entriesContentHash(before) == entriesContentHash(after) {
		t.Fatalf("entriesContentHash must change when an entry's own text changes, even with the same ID")
	}
}
