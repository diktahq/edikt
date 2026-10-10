package phasea

import (
	"os"
	"strings"
	"testing"
)

// RESTORE WAS SKIPPABLE, AND NOTHING SAID SO.
//
// The dispatch contract is plan → fan out → restore → verify. Nothing coupled
// verify to restore: a run that went fan-out → verify got per-item results and
// correction prompts with no sign that the pinned state had never been
// reinstated. And `gov compile` documents suppression as "preserved across
// sidecar regenerations" — that preservation IS restore, so an optional
// restore makes a documented guarantee optional.
//
// verify REFUSES rather than running restore itself: a verify that mutates the
// thing it is judging is the shape closed in the read-only compile work.

// The shared fixture plants a receipt so the item-verdict tests can run; the
// coupling tests remove it, because the state under test is its ABSENCE.
func withoutReceipt(t *testing.T, f *verifyFixture) *verifyFixture {
	t.Helper()
	if err := os.Remove(RestoreReceiptPath(f.root, f.planID)); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return f
}

func TestRestore_WritesAReceiptVerifyCanRequire(t *testing.T) {
	f := withoutReceipt(t, newLiftedFixture(t, []string{preservedRule}))
	if _, err := os.Stat(RestoreReceiptPath(f.root, f.planID)); err == nil {
		t.Fatal("no receipt may exist before restore runs")
	}

	out, err := Restore(RestoreOptions{ProjectRoot: f.root, PlanID: f.planID})
	if err != nil || out == nil || out.ExitCode != ExitRestoreOK {
		t.Fatalf("restore must succeed on the fixture: %v %+v", err, out)
	}

	raw, rerr := os.ReadFile(RestoreReceiptPath(f.root, f.planID))
	if rerr != nil {
		t.Fatalf("restore must leave a receipt: %v", rerr)
	}
	for _, want := range []string{f.planID, "restored_at"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the receipt must carry %q:\n%s", want, raw)
		}
	}
}

func TestVerify_RefusesWhenRestoreHasNotRun(t *testing.T) {
	f := withoutReceipt(t, newLiftedFixture(t, []string{preservedRule}))
	liftedOutput(t, f, preservedRule)

	out, err := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: f.planID})
	if err == nil {
		t.Fatal("verify must refuse when the plan has no restore receipt")
	}
	if out == nil || out.ExitCode != ExitVerifyRestoreMissing {
		code := -1
		if out != nil {
			code = out.ExitCode
		}
		t.Fatalf("the refusal needs its own exit code: got %d, want %d", code, ExitVerifyRestoreMissing)
	}
	msg := out.Message
	if !strings.Contains(msg, "restore") || !strings.Contains(msg, RestoreReceiptPath(f.root, f.planID)) {
		t.Errorf("the refusal must name the missing receipt and the step that writes it:\n%s", msg)
	}
	// It refuses; it does not perform the restore. A verify that mutates the
	// thing it judges cannot be re-run to check the mutation.
	if _, serr := os.Stat(RestoreReceiptPath(f.root, f.planID)); serr == nil {
		t.Error("verify must NOT run restore itself")
	}
}

func TestVerify_ProceedsOnceTheReceiptExists(t *testing.T) {
	f := newLiftedFixture(t, []string{preservedRule})
	if _, err := Restore(RestoreOptions{ProjectRoot: f.root, PlanID: f.planID}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	liftedOutput(t, f, preservedRule)

	out, _ := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: f.planID})
	if out == nil {
		t.Fatal("verify produced no outcome")
	}
	if out.ExitCode == ExitVerifyRestoreMissing {
		t.Fatalf("with a receipt present verify must proceed, got the restore refusal:\n%s", out.Message)
	}
}

// A receipt for a DIFFERENT plan is not evidence for this one.
func TestVerify_ReceiptMustBelongToThisPlan(t *testing.T) {
	f := withoutReceipt(t, newLiftedFixture(t, []string{preservedRule}))
	liftedOutput(t, f, preservedRule)
	// Plant a receipt under another plan id.
	other := "pap-000000000000"
	if err := os.MkdirAll(stateDirFor(f.root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(RestoreReceiptPath(f.root, other), []byte(`{"plan_id":"`+other+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: f.planID})
	if err == nil || out == nil || out.ExitCode != ExitVerifyRestoreMissing {
		t.Fatalf("another plan's receipt is not evidence for this one: %+v", out)
	}
}
