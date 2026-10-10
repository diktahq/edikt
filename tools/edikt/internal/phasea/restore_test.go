package phasea

import (
	"os"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

func (f *verifyFixture) restore(t *testing.T) *RestoreOutcome {
	t.Helper()
	out, err := Restore(RestoreOptions{ProjectRoot: f.root, PlanID: f.planID})
	if out == nil {
		t.Fatalf("Restore() returned a nil outcome: %v", err)
	}
	if out.ExitCode != ExitRestoreOK {
		t.Fatalf("Restore() exit %d: %s", out.ExitCode, out.Message)
	}
	return out
}

// pinnedSnapshot overwrites the plan's captured pin snapshot with a sidecar
// carrying `human_approved_at` on one directive.
//
// It rewrites rather than seeds because the snapshot plan.go captured is the
// STALE prior sidecar, and staleness is what got the item planned — a prior
// sidecar whose anchors already matched the parent would not be in the plan
// at all. The pin machinery does not care about anchors, so the snapshot's
// directive TEXT is what matters here, and this makes it explicit.
func pinnedSnapshot(t *testing.T, f *verifyFixture, directiveText, approvedAt string) {
	t.Helper()
	d := anchoredAtGateQuote(directiveText)
	d.HumanApprovedAt = approvedAt
	writeMarshalledSidecar(t, PinSnapshotPath(f.root, f.planID, f.itemID), &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/ADR-900-test.md",
		Directives:    []sidecar.Directive{d},
	})
}

// ── AC-3.5: TestPinRestore ───────────────────────────────────────────────

func TestPinRestore(t *testing.T) {
	const directive = "The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)"
	const approvedAt = "2026-08-03T00:00:00Z"

	t.Run("restore reinstates human_approved_at the regeneration wiped", func(t *testing.T) {
		f := newStaleFixture(t, anchoredAtStaleQuote(directive))
		pinnedSnapshot(t, f, directive, approvedAt)

		// The extractor never sees the previous sidecar, deliberately — so a
		// regeneration always arrives without the human's stamp.
		f.extractGood(t, anchoredAtGateQuote(directive))
		if sc, _ := sidecar.Load(f.scPath); sc.Directives[0].HumanApprovedAt != "" {
			t.Fatal("fixture: the regenerated sidecar already carries the pin, so nothing is under test")
		}

		out := f.restore(t)
		if out.PinsRestored != 1 {
			t.Fatalf("restore rewrote %d sidecar(s), want 1: %s", out.PinsRestored, out.Message)
		}
		sc, err := sidecar.Load(f.scPath)
		if err != nil {
			t.Fatal(err)
		}
		if sc.Directives[0].HumanApprovedAt != approvedAt {
			t.Fatalf("human_approved_at is %q after restore, want %q", sc.Directives[0].HumanApprovedAt, approvedAt)
		}

		// And the gate then grades it ok — restore and verify agree because
		// they run the SAME comparison, one writing and one only looking.
		v := f.verify(t)
		if it := onlyItem(t, v); it.Status != StatusOK {
			t.Fatalf("verify after a successful restore: %+v — %s", it, v.Message)
		}
		if v.ExitCode != ExitVerifyOK {
			t.Fatalf("got exit %d, want %d: %s", v.ExitCode, ExitVerifyOK, v.Message)
		}
	})

	t.Run("where it cannot, verify reports pins-unrestorable and names the field", func(t *testing.T) {
		f := newStaleFixture(t, anchoredAtStaleQuote("The gate MUST refuse an unsigned payload. (ref: ADR-900)"))
		pinnedSnapshot(t, f, "The gate MUST refuse an unsigned payload. (ref: ADR-900)", approvedAt)

		// A regeneration whose directive has no counterpart — neither an
		// exact-text nor an unambiguous normalized-form match — for the
		// directive the pin was approved against. Attaching it anyway would
		// claim a human approved a rule they never saw it on, so the pin is
		// reported rather than guessed.
		f.extractGood(t, anchoredAtGateQuote(directive))

		r := f.restore(t)
		if len(r.Unrestorable) == 0 {
			t.Fatalf("restore reported nothing unrestorable: %s", r.Message)
		}

		v := f.verify(t)
		it := onlyItem(t, v)
		if it.Status != StatusPinsUnrestorable {
			t.Fatalf("item status %q, want %q: %s", it.Status, StatusPinsUnrestorable, v.Message)
		}
		detail := v.Details[it.ItemID]
		if !strings.Contains(detail, "human_approved_at") {
			t.Errorf("detail %q does not NAME the missing field — \"pins-unrestorable\" alone tells a "+
				"human something a colleague approved is gone without saying what", detail)
		}
		if !strings.Contains(detail, approvedAt) {
			t.Errorf("detail %q does not carry the lost value", detail)
		}
		if v.Receipt.Consumed {
			t.Error("a plan with a lost pin must never be consumed (INV-015)")
		}
	})

	t.Run("verify alone REFUSES rather than restoring", func(t *testing.T) {
		// The seam from the other side: skipping `phasea restore` must not be
		// indistinguishable from running it. A verify that silently
		// reinstated the pin would make the restore verb optional.
		//
		// Strengthened 2026-09-03: verify used to GRADE the item as
		// pins-unrestorable, which distinguished the two but still produced a
		// per-item report describing a corpus missing everything a human
		// pinned. It now refuses outright, because nothing else enforced the
		// contract's order and a documented guarantee ("preserved across
		// regenerations") cannot rest on a step a caller may skip.
		// The receipt the shared fixture plants is removed here: its ABSENCE
		// is the state under test.
		f := withoutReceipt(t, newStaleFixture(t, anchoredAtStaleQuote(directive)))
		pinnedSnapshot(t, f, directive, approvedAt)
		f.extractGood(t, anchoredAtGateQuote(directive))

		v := f.verify(t)
		if v.ExitCode != ExitVerifyRestoreMissing {
			t.Fatalf("verify without a restore must refuse, got exit %d: %s", v.ExitCode, v.Message)
		}
		if !strings.Contains(v.Message, "phasea restore") {
			t.Errorf("the refusal must name the step that writes the receipt:\n%s", v.Message)
		}
		sc, err := sidecar.Load(f.scPath)
		if err != nil {
			t.Fatal(err)
		}
		if sc.Directives[0].HumanApprovedAt != "" {
			t.Fatal("verify wrote the pin back into the sidecar — the gate performed the mutation " +
				"it exists to measure")
		}
	})
}

// ── AC-3.6: TestPinsUnrestorableTerminal ─────────────────────────────────

// TestPinsUnrestorableTerminal pins the distinction between the two terminal
// statuses. failed-terminal is retry EXHAUSTION — an item earns it after
// max_attempts_per_item calls. pins-unrestorable is terminal on FIRST
// observation: the skill contract requires a bounded retry to re-dispatch the
// identical fields, so a second dispatch of the same artifact through the same
// locked prompt produces the same extraction and therefore the same lost pin.
// Retrying would spend two more LLM dispatches to reach a conclusion already
// known on attempt one.
func TestPinsUnrestorableTerminal(t *testing.T) {
	const approvedAt = "2026-08-03T00:00:00Z"
	f := newStaleFixture(t, anchoredAtStaleQuote("The gate MUST refuse an unsigned payload. (ref: ADR-900)"))
	pinnedSnapshot(t, f, "The gate MUST refuse an unsigned payload. (ref: ADR-900)", approvedAt)
	f.extractGood(t, anchoredAtGateQuote("The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)"))
	f.restore(t)

	first := f.verify(t)
	it := onlyItem(t, first)
	if it.Status != StatusPinsUnrestorable || it.Attempts != 1 {
		t.Fatalf("call 1: got %+v, want pins-unrestorable at attempt 1: %s", it, first.Message)
	}
	// Terminal on first observation means no retryable item remains, so the
	// plan is FAILED immediately rather than inviting a re-dispatch.
	if first.ExitCode != ExitVerifyFailed {
		t.Fatalf("call 1: got exit %d, want %d — a lost pin must not be reported as retryable",
			first.ExitCode, ExitVerifyFailed)
	}

	second := f.verify(t)
	if it := onlyItem(t, second); it.Status != StatusPinsUnrestorable || it.Attempts != 1 {
		t.Fatalf("call 2: got %+v — pins-unrestorable must not consume retry budget", it)
	}

	// Even once the pin IS back on disk, the status does not reopen: the loss
	// was observed and recorded against this plan_id, and a status that could
	// un-fail itself would let a hand-edit erase the audit trail of a lost
	// human approval.
	d := anchoredAtGateQuote("The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)")
	d.HumanApprovedAt = approvedAt
	f.extractGood(t, d)
	third := f.verify(t)
	if it := onlyItem(t, third); it.Status != StatusPinsUnrestorable || it.Attempts != 1 {
		t.Fatalf("call 3: got %+v — pins-unrestorable is terminal on first observation", it)
	}
	if third.Receipt.Consumed {
		t.Fatal("a plan that lost a pin became consumed")
	}
}

// ── AC-3.9: TestFailedGateQuarantinesBytes ───────────────────────────────

// TestFailedGateQuarantinesBytes covers what ClaudeRunner's rollbackSidecar
// used to do and what Phase 4 deletes with it. The agent owns the Write tool,
// so Go cannot stop a bad file being written — it can only refuse to let one
// SURVIVE. Without a replacement, a bad extractor write would simply stay in
// the corpus and fail compile for the whole project.
func TestFailedGateQuarantinesBytes(t *testing.T) {
	f := newStaleFixture(t, anchoredAtStaleQuote("A prior rule. (ref: ADR-900)"))

	preDispatch, err := os.ReadFile(PinSnapshotPath(f.root, f.planID, f.itemID))
	if err != nil {
		t.Fatal(err)
	}

	// The exact bytes GL-003's extraction produced: a backtick escaped inside
	// a double-quoted YAML scalar, which is a hard parse error.
	if err := os.WriteFile(f.scPath, []byte(d20InvalidSidecar), 0o644); err != nil {
		t.Fatal(err)
	}

	out := f.restore(t)
	if len(out.Quarantined) != 1 || out.Quarantined[0] != f.itemID {
		t.Fatalf("restore quarantined %v, want [%s]: %s", out.Quarantined, f.itemID, out.Message)
	}

	// The bad write is out of the corpus...
	if _, statErr := os.Stat(f.scPath); !os.IsNotExist(statErr) {
		t.Errorf("the bad write is still in the corpus at %s (stat err %v)", f.scPath, statErr)
	}
	// ...but not destroyed: it is evidence, and INV-015 forbids governance
	// content leaving silently.
	quarantined, err := os.ReadFile(QuarantinePath(f.root, f.planID, f.itemID))
	if err != nil {
		t.Fatalf("the bad bytes were not quarantined: %v", err)
	}
	if string(quarantined) != d20InvalidSidecar {
		t.Error("the quarantined file is not byte-identical to what the extractor wrote")
	}
	// And the pre-dispatch bytes are still recoverable from the plan's own
	// pin snapshot, untouched.
	still, err := os.ReadFile(PinSnapshotPath(f.root, f.planID, f.itemID))
	if err != nil || string(still) != string(preDispatch) {
		t.Errorf("the pre-dispatch bytes are no longer recoverable: %v", err)
	}
	if !strings.Contains(out.Message, "content gate") {
		t.Errorf("restore report %q does not say why the sidecar was quarantined", out.Message)
	}

	// Verify then judges the quarantined write from its quarantine copy
	// (#7, c3-c): classified, retryable, never ok. It used to read the empty
	// expected path as output-missing, which carries no axis and no
	// correction prompt, so the retry was unseeded. It must NOT grade ok —
	// restoring the stale pre-dispatch bytes into place would have made it do
	// exactly that for a stale-migration-preserved item, turning a failed
	// extraction into a green compile.
	v := f.verify(t)
	it := onlyItem(t, v)
	if it.Status == StatusOK {
		t.Fatal("verify graded a quarantined item ok")
	}
	if it.Status != StatusSchemaInvalid || it.DefectAxis != DefectAxisParse {
		t.Fatalf("item status/axis %q/%q, want %q/%q (classified from the quarantine copy): %s",
			it.Status, it.DefectAxis, StatusSchemaInvalid, DefectAxisParse, v.Message)
	}
	if !strings.Contains(it.Diagnostic, "quarantined by restore at") {
		t.Errorf("the diagnostic must name the quarantine copy it was judged from:\n%s", it.Diagnostic)
	}
	if v.ExitCode != ExitVerifyIncomplete {
		t.Fatalf("got exit %d, want %d — a quarantined item is re-dispatchable", v.ExitCode, ExitVerifyIncomplete)
	}
}

// TestRestoreRejectsConsumedPlan — the mutation verb honours the same
// terminal transition the gate does. Re-running the restore against a closed
// transaction would rewrite sidecars no gate will ever look at again.
func TestRestoreRejectsConsumedPlan(t *testing.T) {
	f := newFixture(t, "")
	f.extractGood(t)
	if v := f.verify(t); !v.Receipt.Consumed {
		t.Fatalf("setup: plan was not consumed: %s", v.Message)
	}

	out, err := Restore(RestoreOptions{ProjectRoot: f.root, PlanID: f.planID})
	if err == nil {
		t.Fatal("restoring a consumed plan returned no error")
	}
	if out.ExitCode != ExitRestorePlanRejected {
		t.Fatalf("got exit %d, want %d: %s", out.ExitCode, ExitRestorePlanRejected, out.Message)
	}
}
