package phasea

// verify_unwritten_test.go — PLAN-v090-issue-fixes Phase 3 (#8, c2-b), the
// verify half: an item restore observed the extractor never wrote is graded
// output-missing (stalled under --stalled), never ok. See
// restore_unwritten_test.go for the defect.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// ── AC-3.2 ───────────────────────────────────────────────────────────────

// TestVerify_UnwrittenPriorSidecarIsOutputMissing — the scratch repro, which
// graded ok and CONSUMED the plan at HEAD.
func TestVerify_UnwrittenPriorSidecarIsOutputMissing(t *testing.T) {
	f := newRequestedFixture(t, "")
	p := f.mustRequest(t)
	pin := readBytes(t, PinSnapshotPath(f.root, p.PlanID, f.itemID))
	f.restore(t, p.PlanID)

	v := f.verify(t, p.PlanID)
	it := onlyItem(t, v)
	if it.Status != StatusOutputMissing || v.ExitCode != ExitVerifyIncomplete || v.Receipt.Consumed {
		t.Fatalf("an unwritten requested item: status %q exit %d consumed %v, want %q / %d / false\n%s",
			it.Status, v.ExitCode, v.Receipt.Consumed, StatusOutputMissing, ExitVerifyIncomplete, v.Message)
	}
	if !strings.Contains(it.Diagnostic, "no write since plan "+p.PlanID) {
		t.Errorf("the diagnostic must say restore observed no write since the plan:\n%s", it.Diagnostic)
	}
	if !bytes.Equal(pin, readBytes(t, f.sc)) {
		t.Error("the unwritten sidecar was stamped on the way through restore and verify")
	}
}

func TestVerify_UnwrittenPriorSidecarWithStalledReportIsStalled(t *testing.T) {
	f := newRequestedFixture(t, "")
	p := f.mustRequest(t)
	f.restore(t, p.PlanID)

	v, _ := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: p.PlanID, Stalled: []string{f.itemID}})
	if it := onlyItem(t, v); it.Status != StatusStalled || v.ExitCode != ExitVerifyIncomplete {
		t.Fatalf("an unwritten item the dispatcher reported stalled: status %q exit %d, want %q / %d\n%s",
			it.Status, v.ExitCode, StatusStalled, ExitVerifyIncomplete, v.Message)
	}
}

// TestVerify_IdenticalGenuineRewriteOfLegacyStampedPriorIsOK — the reason the
// comparison lives in restore and not in verify. A prior carrying every stamp,
// rewritten by a real extraction with identical content, is restored and
// stamped back to bytes identical to the pin. Restore sees the extractor's
// unstamped bytes and knows a write happened; a verify-time comparison would
// see only the end state and grade a genuine extraction output-missing.
func TestVerify_IdenticalGenuineRewriteOfLegacyStampedPriorIsOK(t *testing.T) {
	f := newRequestedFixture(t, "")
	const same = "The stable extraction MUST hold. (ref: ADR-900)"
	f.cycle(t, same)
	stamped := readBytes(t, f.sc)
	if sc, err := sidecar.Load(f.sc); err != nil || sc.BodyDigest == "" {
		t.Fatalf("setup: the prior is not stamped (err %v)", err)
	}

	p := f.mustRequest(t)
	f.extract(t, same)
	if bytes.Equal(stamped, readBytes(t, f.sc)) {
		t.Fatal("setup: the extractor's output equals the stamped prior, so nothing distinguishes a write")
	}
	if r, _ := restoreHooked(t, f.root, p.PlanID); containsID(r.Unwritten, f.itemID) {
		t.Fatalf("a genuine rewrite was reported unwritten:\n%s", r.Message)
	}
	v := f.verify(t, p.PlanID)
	if v.ExitCode != ExitVerifyOK || onlyItem(t, v).Status != StatusOK {
		t.Fatalf("a genuine identical rewrite: exit %d\n%s", v.ExitCode, v.Message)
	}
	if !bytes.Equal(stamped, readBytes(t, f.sc)) {
		t.Log("note: the end state differs from the stamped prior; the false-positive shape was not reached")
	}
}

// ── AC-3.3 (phasea half) ─────────────────────────────────────────────────

// TestPlan_UnwrittenFailedPlanReRequestGetsSuccessor — the c1 guard and the
// c2 detection together. An unwritten item leaves sidecar == pin, so the plan
// goes FAILED without the unclosed-plan guard firing, and a re-request then
// derives a successor that names it.
func TestPlan_UnwrittenFailedPlanReRequestGetsSuccessor(t *testing.T) {
	f := newRequestedFixture(t, "phasea:\n  max_attempts_per_item: 1\n")
	p := f.mustRequest(t)
	pin := readBytes(t, PinSnapshotPath(f.root, p.PlanID, f.itemID))
	f.restore(t, p.PlanID)
	if v := f.verify(t, p.PlanID); v.ExitCode != ExitVerifyFailed {
		t.Fatalf("an unwritten item with a budget of 1: exit %d, want FAILED (%d)\n%s", v.ExitCode, ExitVerifyFailed, v.Message)
	}
	if !bytes.Equal(pin, readBytes(t, f.sc)) {
		t.Fatal("the FAILED plan's unwritten sidecar no longer equals its pin")
	}

	out := f.request(t)
	if out.ExitCode != ExitPlanEmitted || out.Plan == nil {
		t.Fatalf("re-request after an unwritten FAILED plan: exit %d\n%s", out.ExitCode, out.Message)
	}
	if out.Plan.PlanID == p.PlanID {
		t.Fatalf("the re-request reused FAILED %s", p.PlanID)
	}
	if !strings.Contains(out.Message, p.PlanID) {
		t.Errorf("the successor must name the FAILED plan it supersedes (%s):\n%s", p.PlanID, out.Message)
	}
}
