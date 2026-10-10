package phasea

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RESTORE IS NOT OPTIONAL, AND UNTIL NOW NOTHING SAID SO.
//
// The dispatch contract is plan → fan out → restore → verify. Nothing coupled
// the last two: a run that went fan-out → verify produced per-item results and
// correction prompts with no sign that the pinned state had never been
// reinstated. `gov compile` documents suppression as "preserved across sidecar
// regenerations" — that preservation IS this step, so a skippable restore made
// a documented guarantee optional.
//
// The receipt is the coupling. Restore writes one; verify REFUSES without it,
// naming the file and the step that writes it. Verify does not run restore
// itself: a verify that mutates the thing it is judging cannot be re-run to
// check the mutation, which is the shape the read-only compile work closed.

// SchemaVersionRestoreReceipt is the restore receipt's shape version.
//
//	1 — plan_id, restored_at (RFC3339, whole seconds), pins_restored,
//	    quarantined.
//	2 — adds unwritten, restored_digests, outputs_written_at and
//	    verify_receipt_sha256; restored_at becomes RFC3339Nano. Additive: a v1
//	    receipt still reads, and every v2 field is absent from it, which the
//	    readers treat as "not measured" rather than as "nothing happened".
//
// There is no published schema for this file. It is runtime state that only
// restore writes and only restore and verify read.
const SchemaVersionRestoreReceipt = 2

// RestoreReceipt records that restore ran for a plan, and what it did.
type RestoreReceipt struct {
	SchemaVersion int      `json:"schema_version"`
	PlanID        string   `json:"plan_id"`
	RestoredAt    string   `json:"restored_at"`
	PinsRestored  int      `json:"pins_restored"`
	Quarantined   []string `json:"quarantined,omitempty"`

	// Unwritten names the items whose sidecar was byte-identical to the plan's
	// pin, or to what the previous restore of this plan left, when this
	// restore looked: the extractor wrote nothing. They were not gated,
	// stamped or touched. Verify grades them output-missing.
	Unwritten []string `json:"unwritten,omitempty"`

	// RestoredDigests is sha256 of each item's sidecar as this restore left
	// it (carried forward for an item this run did not finish). The next
	// restore of the same plan compares against it: a retry that wrote
	// nothing leaves exactly these bytes, which no longer equal the pin.
	RestoredDigests map[string]string `json:"restored_digests,omitempty"`

	// OutputsWrittenAt is each item's sidecar mtime as the EXTRACTOR left it,
	// RFC3339Nano, captured before restore wrote anything. Restore's own
	// stamp moves every mtime to the moment it ran, so an mtime read after
	// restore measures restore. An item nothing rewrote since the previous
	// restore keeps the time recorded then.
	OutputsWrittenAt map[string]string `json:"outputs_written_at,omitempty"`

	// VerifyReceiptSHA256 is sha256 of the plan's verify receipt as it stood
	// when this restore ran ("" when there was none). A later restore that
	// finds the same value is a re-run inside the same round: nothing was
	// re-dispatched in between, so an item this run restored is not
	// "unwritten" merely because its bytes have not moved since.
	VerifyReceiptSHA256 string `json:"verify_receipt_sha256,omitempty"`
}

// RestoreReceiptPath is where a plan's restore receipt lives, beside the plan
// and its pins.
func RestoreReceiptPath(root, planID string) string {
	return filepath.Join(stateDirFor(root), planID+".restore.json")
}

// stateDirFor is the phasea state directory.
func stateDirFor(root string) string {
	return filepath.Join(root, ".edikt", "state", "phasea")
}

// writeRestoreReceipt records a completed restore. A failure to write is
// returned rather than warned about: the receipt is what the next step
// requires, so a restore whose receipt did not land has not finished.
func writeRestoreReceipt(root, planID string, out *RestoreOutcome) error {
	if err := os.MkdirAll(stateDirFor(root), 0o755); err != nil {
		return fmt.Errorf("restore receipt: %w", err)
	}
	r := RestoreReceipt{
		SchemaVersion: SchemaVersionRestoreReceipt,
		PlanID:        planID,
		// Nanoseconds, not whole seconds: the next restore compares sidecar
		// mtimes against this instant, and a run inside the same second as
		// the previous one must not read the previous run's own writes as
		// new extractor output.
		RestoredAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if out != nil {
		r.PinsRestored = out.PinsRestored
		r.Quarantined = out.Quarantined
		r.Unwritten = out.Unwritten
		r.RestoredDigests = out.RestoredDigests
		r.OutputsWrittenAt = out.OutputsWrittenAt
		r.VerifyReceiptSHA256 = out.verifyReceiptSHA256
	}
	body, err := json.MarshalIndent(&r, "", "  ")
	if err != nil {
		return fmt.Errorf("restore receipt: %w", err)
	}
	return os.WriteFile(RestoreReceiptPath(root, planID), append(body, '\n'), 0o644) //nolint:gosec // state file, world-readable by design
}

// restoreReceiptFor reads a plan's receipt, or nil when restore has not run
// for THIS plan. A receipt naming another plan is not evidence for this one.
func restoreReceiptFor(root, planID string) *RestoreReceipt {
	raw, err := os.ReadFile(RestoreReceiptPath(root, planID)) //nolint:gosec // state path
	if err != nil {
		return nil
	}
	var r RestoreReceipt
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil
	}
	if r.PlanID != planID {
		return nil
	}
	return &r
}
