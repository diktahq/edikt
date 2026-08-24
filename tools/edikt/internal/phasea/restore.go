// restore.go — `bin/edikt phasea restore`, the MUTATING half of SPEC-013's
// D6 seam (Phase 3 of PLAN-v080-reliability-core).
//
// WHY THIS IS ITS OWN VERB. Pin restoration needs the sidecar as it stood
// BEFORE the extractor overwrote it (pinpreserve.PreservePinned's `before`
// argument). By the time `phasea verify` runs, tier-1 has already fanned out
// and every stale sidecar has been rewritten — the `before` is gone from the
// tree. The pin-set therefore has to be captured at PLAN time (plan.go's
// capturePinSnapshot) and reinstated by a verb that runs BETWEEN the fan-out
// and the gate. That verb is this one. The skill's Phase A sequence is:
//
//	plan  →  fan out one sidecar-extractor per item  →  restore  →  verify
//
// Folding this work into verify would put a write inside the gate, which is
// the "did the write happen" / "make the write happen" fusion SPEC-013 exists
// to undo: a gate that repairs what it measures can no longer report on it,
// and a green result would no longer distinguish "the extractor got it right"
// from "the gate fixed it up on the way past".
//
// TWO MUTATIONS, AND NOTHING ELSE:
//
//  1. REINSTATE PINS. Every field the plan's pre-dispatch snapshot captured —
//     approved `paths:`, a `paths_approval` receipt, a directive's approved
//     `verify`/`verify_kind`/`human_approved_at`/fixture paths — is carried
//     back into the regenerated sidecar. What cannot be reattached is
//     REPORTED, never guessed (pinpreserve's own contract).
//
//  2. QUARANTINE A BAD WRITE. See quarantineSidecar.
//
// Exit codes mirror verify's, minus the outcomes that only a gate can have:
//
//	0  ExitRestoreOK           — the restore pass ran over every item.
//	2  ExitRestorePlanRejected — plan_id unknown or already consumed.
//	3  ExitRestoreMalformed    — malformed invocation or unusable plan file.
//
// A per-item failure does NOT change the exit code. Restore is
// continue-on-error by design, matching Dispatcher.Run's contract: it is not
// the gate, and a non-zero exit here would make the skill abort before the
// verb whose job is to decide whether anything actually failed.
package phasea

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/pinpreserve"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// Exit codes — see the file doc above.
const (
	ExitRestoreOK           = 0
	ExitRestorePlanRejected = 2
	ExitRestoreMalformed    = 3
)

// RestoreOptions configures one Restore() call.
type RestoreOptions struct {
	// ProjectRoot is the tree to restore into. Empty means the current
	// working directory.
	ProjectRoot string

	// PlanID names the plan whose pin snapshots are reinstated.
	PlanID string

	// OnItemRestored, when set, is called once per plan item immediately
	// after that item's sidecar is final — pins reinstated and body digest
	// stamped — and before the loop moves on.
	//
	// This is D4's named home for the per-completion ledger hook that lived
	// on the deleted dispatcher's recordingRunner. It is a callback rather
	// than a ledger write inlined here on purpose: the ledger format belongs
	// to whoever is driving the restore (reextract has its own), and phasea
	// should not learn a caller's on-disk schema to keep a hook alive.
	//
	// It is called PER ITEM, not once at the end. That is the property the
	// hook existed for: a run killed halfway must leave the items it already
	// finished recorded, so a resume does not re-extract them. A batch-end
	// write loses exactly the work that was most expensive to do.
	OnItemRestored func(item PlanItem, err error)
}

// RestoreOutcome is Restore()'s result.
type RestoreOutcome struct {
	ExitCode int
	Message  string

	// PinsRestored counts the sidecars this pass actually rewrote. It counts
	// WORK DONE, not items walked: a run that reports the same number every
	// time — including runs where every pin was already in place — is
	// reporting work it did not do, and a number that cannot go to zero is
	// one nobody can use to tell whether the restore is finished.
	PinsRestored int

	// Quarantined names the item_ids whose regenerated sidecar failed the
	// content gate and was moved aside.
	Quarantined []string

	// Unrestorable carries every pinned field that could not be reattached.
	// verify recomputes this independently — this field is a report, never
	// an input to the gate.
	Unrestorable []pinpreserve.PinnedField
}

// Restore reinstates the plan's captured pin-set into the regenerated
// sidecars and quarantines any regenerated sidecar that fails the content
// gate. See the file doc.
func Restore(opts RestoreOptions) (*RestoreOutcome, error) {
	root := opts.ProjectRoot
	planID := opts.PlanID

	if !planIDPattern.MatchString(planID) {
		msg := fmt.Sprintf(
			"refusing to restore: %q is not a valid plan_id — expected the "+
				"phasea-dispatch-plan.v1 shape `pap-` followed by 12 lowercase hex characters", planID)
		return &RestoreOutcome{ExitCode: ExitRestoreMalformed, Message: msg}, fmt.Errorf("%s", msg)
	}

	plan, code, msg := loadPlanForVerb(root, planID)
	if plan == nil {
		return &RestoreOutcome{ExitCode: code, Message: msg}, fmt.Errorf("%s", msg)
	}

	// A consumed plan is terminal for BOTH verbs. Re-running the mutation
	// against one would rewrite sidecars belonging to a closed transaction,
	// with no gate left that will ever look at the result.
	prior, perr := loadReceipt(root, planID)
	if perr != nil {
		msg := fmt.Sprintf("refusing to restore: the prior receipt for %s could not be read: %v", planID, perr)
		return &RestoreOutcome{ExitCode: ExitRestoreMalformed, Message: msg}, fmt.Errorf("%s", msg)
	}
	if prior != nil && prior.Consumed {
		msg := fmt.Sprintf(
			"refusing to restore: plan %s is already consumed (verified_at %s) — a consumed plan is terminal",
			planID, prior.VerifiedAt)
		return &RestoreOutcome{ExitCode: ExitRestorePlanRejected, Message: msg}, fmt.Errorf("%s", msg)
	}

	out := &RestoreOutcome{ExitCode: ExitRestoreOK}
	var notes []string

	// restoreOne does one item's work and returns why it did not complete, if
	// it did not. The loop below notifies EXACTLY ONCE per item, whatever
	// happened — the notification cannot be skipped by an early return the
	// way it could when each `continue` had to remember to fire it.
	//
	// That is not hypothetical tidiness: the first version of this hook was
	// bypassed by three of the five exit paths, including the quarantine one.
	// An item that failed the content gate would have gone unrecorded, and a
	// resume reads an absent ledger entry as "never attempted" — so the item
	// that most needed attention is the one that would have been silently
	// re-extracted forever.
	restoreOne := func(item PlanItem) error {
		artifactPath := underRoot(root, item.ArtifactPath)
		sidecarPath := underRoot(root, item.ExpectedOutput.Path)

		if _, err := os.Stat(sidecarPath); err != nil {
			// Nothing was written for this item. There is no bad write to
			// quarantine and nothing to restore pins INTO; verify will report
			// output-missing, which is retryable and correct.
			notes = append(notes, fmt.Sprintf("%s: no sidecar at %s — nothing to restore", item.ItemID, item.ExpectedOutput.Path))
			return fmt.Errorf("%s: no sidecar written", item.ItemID)
		}

		if gerr := contentGate(sidecarPath, artifactPath); gerr != nil {
			qpath, qerr := quarantineSidecar(root, planID, item.ItemID, sidecarPath)
			if qerr != nil {
				// Worse than the bad extraction: a sidecar that fails the
				// gate is still on disk and could not be moved. Say so
				// loudly — a silent half-quarantine is how a broken compile
				// gets blamed on the next change instead of this one.
				notes = append(notes, fmt.Sprintf(
					"%s: failed the content gate (%v) and QUARANTINE ALSO FAILED (%v) — %s is bad on "+
						"disk and must be moved by hand", item.ItemID, gerr, qerr, item.ExpectedOutput.Path))
				return fmt.Errorf("%s: content gate failed (%v) and quarantine failed (%v)", item.ItemID, gerr, qerr)
			}
			out.Quarantined = append(out.Quarantined, item.ItemID)
			notes = append(notes, fmt.Sprintf(
				"%s: failed the content gate (%v) — the bad write is at %s; the pre-dispatch bytes "+
					"remain at %s", item.ItemID, gerr, relPath(root, qpath), relPath(root, PinSnapshotPath(root, planID, item.ItemID))))
			return fmt.Errorf("%s: content gate failed: %v", item.ItemID, gerr)
		}

		snap := PinSnapshotPath(root, planID, item.ItemID)
		if _, err := os.Stat(snap); err != nil {
			// No snapshot means no prior sidecar existed — the "missing"
			// reason's ordinary bootstrap case, with genuinely nothing pinned
			// to lose. Not a gap.
			//
			// It still gets a body-digest baseline. A bootstrapped artifact
			// was just as freshly extracted as a re-extracted one, and ADR-053
			// ties the stamp to "an extraction that actually read this parent
			// body", not to whether a prior sidecar happened to exist. Leaving
			// it unstamped would make every first-compile artifact permanently
			// unmeasured for body drift.
			return stampBody(sidecarPath, artifactPath, item.ItemID, &notes)
		}
		before, loadErr := sidecar.Load(snap)
		if loadErr != nil {
			// The snapshot exists but will not parse. That is NOT the same
			// claim as "nothing was pinned": there might have been anything
			// in it. PreservePinned's beforeLoadErr parameter exists to keep
			// the two apart, so pass it through rather than passing nil.
			before = nil
		}

		var itemErr error

		res, rerr := pinpreserve.PreservePinned(item.ItemID, before, loadErr, sidecarPath)
		if rerr != nil {
			itemErr = rerr
			notes = append(notes, fmt.Sprintf("%s: pin restore failed: %v", item.ItemID, rerr))
		}
		if res == nil {
			if itemErr == nil {
				itemErr = fmt.Errorf("%s: pin restore returned no result", item.ItemID)
			}
			return itemErr
		}
		if res.SidecarRewritten {
			out.PinsRestored++
			notes = append(notes, fmt.Sprintf(
				"%s: reinstated %d directive pin(s)%s%s", item.ItemID, res.DirectivePins,
				flag(res.PathsRestored, ", approved paths"), flag(res.ApprovalRestored, ", approval receipt")))
		}
		out.Unrestorable = append(out.Unrestorable, res.Unrestorable...)

		// ADR-053 — stamp the body-drift baseline, here and nowhere else.
		//
		// This moved from twophase.go's post-dispatch loop, which became
		// unreachable when Phase A stopped dispatching from tier-2 (D4
		// requires every behaviour the deleted dispatcher carried to have a
		// named new home before it goes; this is that home for the stamp).
		//
		// Restore is the correct home and verify is not: the stamp is a
		// WRITE, and D6 keeps verify read-only so it can be re-run without
		// changing what it judges. Restore is already the post-extraction
		// mutation point, and by here the sidecar is final — the extractor
		// wrote it and PreservePinned has reinstated the human-pinned state
		// on top.
		//
		// ADR-053's constraint still governs: a digest recorded anywhere
		// other than immediately after an extraction that actually read this
		// parent body would claim an extraction that never happened, turning
		// "this sidecar may be incomplete" into a confident lie. That is why
		// this runs per item inside the loop and not as a corpus-wide
		// backfill.
		if serr := stampBody(sidecarPath, artifactPath, item.ItemID, &notes); serr != nil {
			// Non-fatal, matching the behaviour this replaces: a missing
			// baseline degrades to UNMEASURED, which is a safe state. A
			// failed stamp must not discard good extraction work.
			itemErr = serr
		}

		return itemErr
	}

	for _, item := range plan.Items {
		itemErr := restoreOne(item)
		if opts.OnItemRestored != nil {
			opts.OnItemRestored(item, itemErr)
		}
	}

	sort.Slice(out.Unrestorable, func(i, j int) bool {
		if out.Unrestorable[i].ArtifactID != out.Unrestorable[j].ArtifactID {
			return out.Unrestorable[i].ArtifactID < out.Unrestorable[j].ArtifactID
		}
		return out.Unrestorable[i].Field < out.Unrestorable[j].Field
	})

	var b strings.Builder
	fmt.Fprintf(&b, "restore %s: %d item(s), %d sidecar(s) rewritten, %d quarantined, %d unrestorable pin(s)",
		planID, len(plan.Items), out.PinsRestored, len(out.Quarantined), len(out.Unrestorable))
	for _, n := range notes {
		fmt.Fprintf(&b, "\n  %s", n)
	}
	for _, u := range out.Unrestorable {
		fmt.Fprintf(&b, "\n  UNRESTORABLE %s", u.String())
	}
	out.Message = b.String()
	return out, nil
}

// QuarantinePath is .edikt/state/phasea/<plan_id>.quarantine/<item_id>.yaml —
// where a regenerated sidecar that fails the content gate is moved.
func QuarantinePath(root, planID, itemID string) string {
	return filepath.Join(root, ".edikt", "state", "phasea", planID+".quarantine", itemID+".yaml")
}

// quarantineSidecar moves a bad regenerated sidecar out of the corpus and
// into .edikt/state/phasea/<plan_id>.quarantine/.
//
// WHY THIS EXISTS. ClaudeRunner's rollbackSidecar restored the prior bytes
// byte-for-byte after a failed gate, so "the write does not happen" was true
// in effect: the agent owns the Write tool, so Go cannot stop a bad file
// being written — it can only refuse to let one SURVIVE. ClaudeRunner and
// rollbackSidecar are deleted in Phase 4, and without a replacement a bad
// extractor write would simply stay in the corpus and fail compile for the
// whole project. This is the replacement.
//
// WHY IT MOVES THE FILE ASIDE RATHER THAN RESTORING THE PRE-DISPATCH BYTES.
// The pre-dispatch bytes are the STALE sidecar — that staleness is why the
// item was planned in the first place. Writing them back would leave a file
// that parses, validates, and (for a stale-migration-preserved item) anchors
// cleanly, so the very next `phasea verify` would grade the item `ok` and
// consume the plan. A failed extraction would land as a green compile: the
// exact absence-rendered-as-a-measurement shape INV-013 forbids. Leaving the
// sidecar ABSENT instead makes verify report `output-missing`, which is
// truthful and retryable. Nothing is lost either way — the bad write is in
// quarantine and the pre-dispatch bytes are still in the plan's .pins/
// snapshot, both named in the restore report (INV-015).
func quarantineSidecar(root, planID, itemID, sidecarPath string) (string, error) {
	dst := QuarantinePath(root, planID, itemID)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(sidecarPath, dst); err == nil {
		return dst, nil
	}
	// Rename fails across devices (a project whose docs/ is a mount point).
	// Copy-then-remove rather than give up: the point is that the bad write
	// does not survive in the corpus.
	if err := copyFile(sidecarPath, dst); err != nil {
		return "", err
	}
	if err := os.Remove(sidecarPath); err != nil {
		return "", fmt.Errorf("copied to quarantine but could not remove the original: %w", err)
	}
	return dst, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func flag(on bool, text string) string {
	if !on {
		return ""
	}
	return text
}

// stampBody records the ADR-053 body-drift baseline for one restored item.
//
// Non-fatal by contract: a missing baseline degrades to UNMEASURED, which is
// a safe state, and a failed stamp must not discard good extraction work. The
// failure is still reported rather than swallowed — an unmeasured artifact
// nobody was told about is the shape INV-013 names.
func stampBody(sidecarPath, artifactPath, itemID string, notes *[]string) error {
	if err := sidecar.StampBodyDigest(sidecarPath, artifactPath,
		os.ReadFile,
		func(p string, b []byte) error { return os.WriteFile(p, b, 0o644) },
	); err != nil {
		*notes = append(*notes, fmt.Sprintf(
			"%s: could not record body-drift baseline: %v (reported as unmeasured)", itemID, err))
		return err
	}
	return nil
}
