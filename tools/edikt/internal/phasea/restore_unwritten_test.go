package phasea

// restore_unwritten_test.go — PLAN-v090-issue-fixes Phase 3 (#8, c2-b).
//
// THE DEFECT. An extractor that wrote nothing — a turn-exhausted dispatch is
// the field case — left the PRIOR sidecar on disk. Restore could not tell it
// from a fresh extraction: a gate-passing prior cleared the content gate, had
// its pins "reinstated" onto itself and its body digest stamped against the
// CURRENT parent, and verify then graded it ok and consumed the plan. The
// ADR-053 body-drift signal went quiet over prose no extraction ever read, and
// `gov reextract` recorded the item done.
//
// THE FIX, asserted here: restore compares the live bytes with the plan's pin
// and with what the previous restore of this plan left, BEFORE it mutates
// anything. An unwritten item is not gated, not stamped, listed in the restore
// receipt, and ledgered failed; verify grades it output-missing (stalled under
// --stalled — verify_unwritten_test.go).

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func containsID(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func mustRestoreReceipt(t *testing.T, root, planID string) *RestoreReceipt {
	t.Helper()
	rr := restoreReceiptFor(root, planID)
	if rr == nil {
		t.Fatalf("no restore receipt for %s", planID)
	}
	return rr
}

// restoreHooked runs Restore and returns the per-item errors its hook saw.
func restoreHooked(t *testing.T, root, planID string) (*RestoreOutcome, map[string]error) {
	t.Helper()
	seen := map[string]error{}
	out, _ := Restore(RestoreOptions{
		ProjectRoot: root, PlanID: planID,
		OnItemRestored: func(item PlanItem, err error) { seen[item.ItemID] = err },
	})
	if out == nil || out.ExitCode != ExitRestoreOK {
		t.Fatalf("Restore(%s): %+v", planID, out)
	}
	return out, seen
}

// ── AC-3.1 ───────────────────────────────────────────────────────────────

// TestRestore_UnwrittenSidecarIsNotStamped — the c2-b repro at the restore
// step: a gate-passing prior, a requested plan, and an extractor that wrote
// nothing.
func TestRestore_UnwrittenSidecarIsNotStamped(t *testing.T) {
	f := newRequestedFixture(t, "")
	p := f.mustRequest(t)
	before := readBytes(t, f.sc)

	out, seen := restoreHooked(t, f.root, p.PlanID)

	if !bytes.Equal(before, readBytes(t, f.sc)) {
		t.Fatal("restore rewrote a sidecar the extractor never wrote — the prior is now stamped as a fresh extraction")
	}
	if err, ok := seen[f.itemID]; !ok || err == nil {
		t.Errorf("the unwritten item must reach OnItemRestored with an error (ledgered failed); got %v (called: %v)", err, ok)
	}
	if !containsID(out.Unwritten, f.itemID) {
		t.Errorf("RestoreOutcome.Unwritten = %v, want it to name %s", out.Unwritten, f.itemID)
	}
	if !strings.Contains(out.Message, "no write observed") {
		t.Errorf("the restore report must say no write was observed:\n%s", out.Message)
	}
	rr := mustRestoreReceipt(t, f.root, p.PlanID)
	if rr.SchemaVersion != 2 {
		t.Errorf("restore receipt schema_version = %d, want 2", rr.SchemaVersion)
	}
	if !containsID(rr.Unwritten, f.itemID) {
		t.Errorf("restore receipt unwritten = %v, want it to name %s", rr.Unwritten, f.itemID)
	}
	if _, ok := rr.OutputsWrittenAt[f.itemID]; ok {
		t.Error("an item nothing wrote was given an extractor write time")
	}
	if _, err := os.Stat(QuarantinePath(f.root, p.PlanID, f.itemID)); err == nil {
		t.Error("an unwritten item was quarantined — there is no bad write to move aside")
	}
}

// TestRestore_RerunAfterStalledRetryComparesLastRestoredDigest — a retry round
// whose dispatch stalls leaves the sidecar as the PREVIOUS restore left it,
// which no longer equals the pin. The comparison is against that digest.
func TestRestore_RerunAfterStalledRetryComparesLastRestoredDigest(t *testing.T) {
	// Budget for a third verify, so the stalled retry's verdict is the
	// classification itself and not retry exhaustion.
	f := newRequestedFixture(t, "phasea:\n  max_attempts_per_item: 3\n")
	p := f.mustRequest(t)
	f.extract(t, "The first extraction MUST hold. (ref: ADR-900)")
	r1, _ := restoreHooked(t, f.root, p.PlanID)
	if containsID(r1.Unwritten, f.itemID) {
		t.Fatalf("setup: a real extraction was reported unwritten:\n%s", r1.Message)
	}

	// A retryable verdict on a sidecar that is on disk: the installed contract
	// moved under the plan. Then the contract is put back for the retry.
	writeExtractorAgent(t, f.root, 4)
	if v := f.verify(t, p.PlanID); v.ExitCode != ExitVerifyIncomplete || onlyItem(t, v).Status != StatusPromptVersionMismatch {
		t.Fatalf("setup: want a retryable prompt-version-mismatch, got exit %d\n%s", v.ExitCode, v.Message)
	}
	writeExtractorAgent(t, f.root, 3)
	restoredBytes := readBytes(t, f.sc)

	// The retry dispatch stalls: nothing is written.
	r2, seen := restoreHooked(t, f.root, p.PlanID)
	if !containsID(r2.Unwritten, f.itemID) {
		t.Fatalf("restore after a stalled retry did not see that nothing was written since the last restore:\n%s", r2.Message)
	}
	if seen[f.itemID] == nil {
		t.Error("the unwritten retry must ledger as failed")
	}
	if !bytes.Equal(restoredBytes, readBytes(t, f.sc)) {
		t.Error("restore rewrote the unwritten sidecar on the retry round")
	}
}

// TestRestore_UnreadableSnapshotFallsThroughNotUnwritten — a pin that exists
// but cannot be read is NOT evidence that nothing was written. The comparison
// is unmeasured, said so, and the item goes through the ordinary path.
func TestRestore_UnreadableSnapshotFallsThroughNotUnwritten(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a mode-000 file; the unreadable state cannot be built")
	}
	f := newRequestedFixture(t, "")
	p := f.mustRequest(t)
	pin := PinSnapshotPath(f.root, p.PlanID, f.itemID)
	if err := os.Chmod(pin, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(pin, 0o644) })
	if _, err := os.ReadFile(pin); err == nil {
		t.Skip("the filesystem ignores mode 000; the unreadable state cannot be built")
	}

	out, _ := restoreHooked(t, f.root, p.PlanID)
	if containsID(out.Unwritten, f.itemID) {
		t.Fatal("an unreadable pin was read as proof that nothing was written")
	}
	if !strings.Contains(out.Message, f.itemID) || !strings.Contains(out.Message, "not measured") {
		t.Errorf("the restore report must say the unwritten check for %s was not measured:\n%s", f.itemID, out.Message)
	}
}
