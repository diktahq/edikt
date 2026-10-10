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
// FIRST IT LOOKS, THEN IT MUTATES. Each item ends in exactly one of three
// outcomes, decided in this order:
//
//  1. UNWRITTEN — the extractor wrote nothing. The live sidecar is
//     byte-identical to the plan's pre-dispatch pin, or to what the previous
//     restore of this plan left (a retry round whose dispatch stalled). It is
//     not gated, not pinned onto itself, not stamped and not quarantined: it
//     is left exactly as found, listed in the receipt's `unwritten`, and
//     reported to OnItemRestored as a failure. Verify grades it
//     output-missing. Before this check existed, a turn-exhausted dispatch
//     over a gate-passing prior had the prior stamped as a fresh extraction
//     and consumed as ok: the body-drift baseline moved over prose nothing
//     had read. The comparison happens HERE and not in verify because only
//     restore sees the extractor's bytes before its own stamp rewrites them.
//
//  2. QUARANTINED — something was written and it fails the content gate. See
//     quarantineSidecar.
//
//  3. RESTORED — something was written and it passes. Pins are reinstated:
//     every field the plan's pre-dispatch snapshot captured — approved
//     `paths:`, a `paths_approval` receipt, a directive's approved
//     `verify`/`verify_kind`/`human_approved_at`/fixture paths — is carried
//     back into the regenerated sidecar, and what cannot be reattached is
//     REPORTED, never guessed (pinpreserve's own contract). Unreceipted
//     `paths:` are carried too, and labelled unreceipted, never approved. A
//     `paths:` the extractor wrote on a sidecar whose pre-dispatch state had
//     none is dropped: scope enters only through `proposed_paths` and
//     `edikt sidecar approve --kind paths`. Then the body-drift baseline is
//     stamped.
//
// Before any of that, each written item's mtime is recorded as the
// extractor's write time. Restore's own writes move every mtime to the moment
// it ran, so this is the last point at which the mtime still says when the
// extractor finished — the evidence verify's concurrency report reads.
//
// Exit codes mirror verify's, minus the outcomes that only a gate can have:
//
//	0  ExitRestoreOK           — the restore pass ran over every item.
//	2  ExitRestorePlanRejected — plan_id unknown, already consumed, or FAILED
//	                           (terminal: every item at a terminal status).
//	3  ExitRestoreMalformed    — malformed invocation or unusable plan file.
//
// A per-item failure does NOT change the exit code. Restore is
// continue-on-error by design, matching Dispatcher.Run's contract: it is not
// the gate, and a non-zero exit here would make the skill abort before the
// verb whose job is to decide whether anything actually failed.
package phasea

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

	// PathsDropped names the items whose extractor-introduced `paths:` this
	// pass dropped. A drop is a rewrite too, so the summary's "sidecar(s)
	// rewritten" counts a sidecar that only had its paths dropped; it is
	// kept out of PinsRestored because no pin was restored.
	PathsDropped []string

	// UnpinnedItems names every item this plan took NO pin for, because no
	// sidecar existed for it when the plan was built. Nothing those artifacts
	// carried could be carried forward, and nothing downstream can tell
	// whether they carried anything.
	//
	// It is a LIST, not a boolean, and that is the whole point. The first
	// version of this was `NothingPinned bool`, true only when EVERY item was
	// unpinned — so a single ordinary stale sidecar planned alongside a
	// deleted one silenced it completely, which is the common case in any real
	// corpus and was exactly the run that needed to speak. A per-item fact
	// reported as a whole-plan boolean is not the same measurement.
	//
	// It is also stated as a COUNT and NAMES rather than as an alarm. On a
	// greenfield first compile every item is unpinned and nothing is lost;
	// crying wolf there teaches people to ignore the line that matters when
	// 1 of 90 is unpinned. INV-013: say what was and was not measured.
	UnpinnedItems []string

	// Quarantined names the item_ids whose regenerated sidecar failed the
	// content gate and was moved aside.
	Quarantined []string

	// Unwritten names the item_ids whose sidecar the extractor did not write
	// (see the file doc). Persisted in the restore receipt; verify reads it.
	Unwritten []string

	// RestoredDigests and OutputsWrittenAt are what the restore receipt
	// records for the next restore and for verify — see RestoreReceipt.
	RestoredDigests  map[string]string
	OutputsWrittenAt map[string]string

	// verifyReceiptSHA256 is the verify receipt this run saw; see
	// RestoreReceipt.VerifyReceiptSHA256.
	verifyReceiptSHA256 string

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
				"phasea-dispatch-plan.v2 shape `pap-` followed by 12 lowercase hex characters", planID)
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
	// A FAILED plan is terminal too (#11, c1-f). Every item is at a terminal
	// status, so no verify call against this id will ever look at a sidecar
	// again: restoring into it would stamp a new extraction as fresh with
	// nothing left to gate it, and verify would carry the old verdict forward
	// over it.
	if receiptFailedTerminal(prior) {
		msg := fmt.Sprintf(
			"refusing to restore: plan %s is FAILED (terminal; verified_at %s) — no verify call against it will "+
				"evaluate an item again, so a restore here stamps output nothing will gate. Restore the pin "+
				"(%s) over the sidecar, or `git checkout -- <sidecar>`, and re-request the artifact to get a "+
				"successor plan",
			planID, prior.VerifiedAt, relPath(root, filepath.Join(stateDirFor(root), planID+".pins")))
		return &RestoreOutcome{ExitCode: ExitRestorePlanRejected, Message: msg}, fmt.Errorf("%s", msg)
	}

	// WHAT THIS RUN COMPARES AGAINST, read once before any item is touched:
	// the previous restore of this plan (what it left, and when), and whether
	// a verify has run since it. No verify since means this is a re-run of the
	// same round and nothing was re-dispatched in between.
	priorRestore := restoreReceiptFor(root, planID)
	verifySum := fileSHA256OrEmpty(ReceiptFilePath(root, planID))
	sameRound := priorRestore != nil && priorRestore.SchemaVersion >= 2 &&
		priorRestore.VerifyReceiptSHA256 == verifySum
	cutoff := writeTimeCutoff(priorRestore, prior)
	// An item already terminal in the verify receipt is carried forward by
	// verify and is never re-dispatched against this plan, so "nothing was
	// written since" is its expected state, not a finding.
	priorTerminal := map[string]bool{}
	if prior != nil {
		for _, it := range prior.Items {
			if IsTerminalStatus(it.Status) {
				priorTerminal[it.ItemID] = true
			}
		}
	}

	out := &RestoreOutcome{
		ExitCode:            ExitRestoreOK,
		RestoredDigests:     map[string]string{},
		OutputsWrittenAt:    map[string]string{},
		verifyReceiptSHA256: verifySum,
	}
	if priorRestore != nil {
		for id, sum := range priorRestore.RestoredDigests {
			out.RestoredDigests[id] = sum
		}
	}
	var notes []string
	// pinnedAndDropped counts sidecars both re-pinned and stripped, so the
	// summary's rewritten count does not count one sidecar twice.
	pinnedAndDropped := 0

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

		st, err := os.Stat(sidecarPath)
		if err != nil {
			// Still quarantined from an earlier restore in THIS round: the
			// path is empty because restore moved the write aside, not
			// because nothing was written. Keep it listed, or verify would
			// lose the quarantine copy it classifies the item from.
			if sameRound && containsString(priorRestore.Quarantined, item.ItemID) {
				if _, qerr := os.Stat(QuarantinePath(root, planID, item.ItemID)); qerr == nil {
					out.Quarantined = append(out.Quarantined, item.ItemID)
					notes = append(notes, fmt.Sprintf("%s: still quarantined by this round's earlier restore at %s",
						item.ItemID, relPath(root, QuarantinePath(root, planID, item.ItemID))))
					return fmt.Errorf("%s: quarantined (content gate failed)", item.ItemID)
				}
			}
			// Nothing was written for this item. There is no bad write to
			// quarantine and nothing to restore pins INTO; verify will report
			// output-missing, which is retryable and correct.
			notes = append(notes, fmt.Sprintf("%s: no sidecar at %s — nothing to restore", item.ItemID, item.ExpectedOutput.Path))
			return fmt.Errorf("%s: no sidecar written", item.ItemID)
		}

		// LOOK BEFORE MUTATING (outcome 1 in the file doc). An unreadable
		// sidecar falls through to the content gate, which quarantines it.
		if live, rerr := os.ReadFile(sidecarPath); rerr == nil && !priorTerminal[item.ItemID] {
			unwritten, basis, unmeasured := unwrittenSincePlan(root, planID, item.ItemID, live, priorRestore, sameRound)
			if unwritten {
				out.Unwritten = append(out.Unwritten, item.ItemID)
				notes = append(notes, fmt.Sprintf(
					"%s: no write observed since plan %s — %s is byte-identical to %s. Not gated, not stamped, "+
						"left as found; verify grades it output-missing (stalled under --stalled). If the "+
						"extractor has written it since, run restore again",
					item.ItemID, planID, item.ExpectedOutput.Path, basis))
				return fmt.Errorf("%s: no write observed since plan %s (the sidecar is byte-identical to %s)",
					item.ItemID, planID, basis)
			}
			if unmeasured != nil {
				notes = append(notes, fmt.Sprintf(
					"%s: the pin snapshot could not be read (%v), so whether the extractor wrote anything was "+
						"not measured against it — the item is restored as written", item.ItemID, unmeasured))
			}
		}

		// The extractor's write time, captured before restore's own writes
		// move the mtime. A sidecar no newer than the previous restore (or
		// the verify after it) was not rewritten since: keep the time
		// recorded then.
		if priorRestore == nil || st.ModTime().After(cutoff) {
			out.OutputsWrittenAt[item.ItemID] = st.ModTime().UTC().Format(time.RFC3339Nano)
		} else if prev, ok := priorRestore.OutputsWrittenAt[item.ItemID]; ok {
			out.OutputsWrittenAt[item.ItemID] = prev
		}

		// From here the item is restore's; record what it is left as, so
		// the next restore of this plan can tell a stalled retry from a
		// write.
		final := true
		defer func() {
			if !final {
				return
			}
			if b, berr := os.ReadFile(sidecarPath); berr == nil {
				sum := sha256.Sum256(b)
				out.RestoredDigests[item.ItemID] = hex.EncodeToString(sum[:])
			}
		}()

		if gerr := contentGate(root, sidecarPath, artifactPath); gerr != nil {
			final = false
			delete(out.RestoredDigests, item.ItemID)
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
			//
			// No prior sidecar also means no `paths:` existed before this
			// dispatch, so any the output carries were introduced by the
			// extractor and are dropped before the stamp.
			stripped, serr := stripIntroducedPaths(nil, sidecarPath, item.ItemID, &notes)
			if stripped {
				out.PathsDropped = append(out.PathsDropped, item.ItemID)
			}
			if stErr := stampBody(sidecarPath, artifactPath, item.ItemID, &notes); serr == nil {
				serr = stErr
			}
			return serr
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
				"%s: reinstated %d entry pin(s)%s%s", item.ItemID, res.DirectivePins,
				res.PathsLabel(), flag(res.ApprovalRestored, ", approval receipt")))
		}
		out.Unrestorable = append(out.Unrestorable, res.Unrestorable...)

		// After the pins are back, so the pre-dispatch state is final: a
		// `paths:` the snapshot did not carry is the extractor's and is
		// dropped. A snapshot that would not parse says nothing about what
		// it carried, so that case is noted and left as written.
		if loadErr == nil {
			stripped, serr := stripIntroducedPaths(before, sidecarPath, item.ItemID, &notes)
			if stripped {
				out.PathsDropped = append(out.PathsDropped, item.ItemID)
				if res.SidecarRewritten {
					// Already counted as rewritten by the pin restore.
					pinnedAndDropped++
				}
			}
			if serr != nil && itemErr == nil {
				itemErr = serr
			}
		} else if live, lerr := sidecar.Load(sidecarPath); lerr == nil && len(live.Paths) > 0 {
			notes = append(notes, fmt.Sprintf(
				"%s: paths: [%s] left as written — the pin snapshot could not be read, so whether the extractor "+
					"introduced them was not checked. Scope enters only via proposed_paths and "+
					"`edikt sidecar approve --kind paths`; review them by hand",
				item.ItemID, strings.Join(live.Paths, ", ")))
		}

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

	// INV-013 (D414). Decide whether this plan TOOK any pins at all. A run
	// that pinned nothing cannot have protected anything, and the summary
	// below would otherwise report that state in exactly the words it uses
	// for a clean, fully-restored one.
	for _, item := range plan.Items {
		if _, serr := os.Stat(PinSnapshotPath(root, planID, item.ItemID)); serr != nil {
			out.UnpinnedItems = append(out.UnpinnedItems, item.ItemID)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "restore %s: %d item(s), %d sidecar(s) rewritten, %d quarantined, %d unwritten, %d unrestorable pin(s)",
		planID, len(plan.Items), out.PinsRestored+len(out.PathsDropped)-pinnedAndDropped, len(out.Quarantined), len(out.Unwritten), len(out.Unrestorable))
	if n := len(out.UnpinnedItems); n > 0 {
		fmt.Fprintf(&b, "\n  %d of %d item(s) had NO prior sidecar, so no pin was taken and nothing they carried could"+
			"\n  be carried forward: %s", n, len(plan.Items), strings.Join(out.UnpinnedItems, ", "))
		if n < len(plan.Items) {
			fmt.Fprintf(&b, "\n  If any of those had a sidecar before, its manual_directives and suppressed_directives"+
				"\n  are gone — they live only in the sidecar and the extractor cannot rebuild them.")
		}
	}
	for _, n := range notes {
		fmt.Fprintf(&b, "\n  %s", n)
	}
	for _, u := range out.Unrestorable {
		fmt.Fprintf(&b, "\n  UNRESTORABLE %s", u.String())
	}
	out.Message = b.String()

	// The receipt is what verify requires. A restore whose receipt did not
	// land has not finished: the next step would refuse anyway, and it should
	// refuse for the real reason rather than silently proceeding.
	if out.ExitCode == ExitRestoreOK {
		if werr := writeRestoreReceipt(root, planID, out); werr != nil {
			out.ExitCode = ExitRestoreMalformed
			out.Message = fmt.Sprintf("restore completed but its receipt could not be written: %v", werr)
			return out, fmt.Errorf("%s", out.Message)
		}
	}

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
// sidecar ABSENT instead means nothing at the expected path can pass, and
// verify — which reads this receipt's `quarantined` list — judges the item
// from the quarantine copy: anchor-invalid, length-exceeded or schema-invalid,
// retryable, with a correction prompt (#7). Nothing is lost either way — the
// bad write is in quarantine and the pre-dispatch bytes are still in the
// plan's .pins/ snapshot, both named in the restore report (INV-015).
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

// stripIntroducedPaths drops an artifact-level `paths:` the extractor wrote
// on a sidecar whose pre-dispatch state had none (see
// pinpreserve.StripUnapprovedPaths), and says so. before is the parsed pin
// snapshot, or nil when the item had no prior sidecar; a snapshot that would
// not parse must not reach here.
//
// stripped reports a rewrite, which the caller records in PathsDropped.
func stripIntroducedPaths(before *sidecar.Sidecar, sidecarPath, itemID string, notes *[]string) (stripped bool, err error) {
	res, err := pinpreserve.StripUnapprovedPaths(before, sidecarPath)
	if err != nil {
		*notes = append(*notes, fmt.Sprintf(
			"%s: COULD NOT DROP extractor-introduced paths (%v) — they are still on disk as enforced scope "+
				"no human approved; remove them by hand", itemID, err))
		return false, fmt.Errorf("%s: drop extractor-introduced paths: %w", itemID, err)
	}
	if len(res.Dropped) == 0 {
		return false, nil
	}
	receipt := ""
	if res.ReceiptDropped {
		receipt = " (and the approval receipt that came with them — a receipt the pre-dispatch sidecar never carried is not an approval)"
	}
	*notes = append(*notes, fmt.Sprintf(
		"%s: dropped paths: [%s]%s — the extractor wrote them on a sidecar that had none, and no approval "+
			"receipt covers them; scope enters only via proposed_paths + `edikt sidecar approve --kind paths`",
		itemID, strings.Join(res.Dropped, ", "), receipt))
	return true, nil
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

// unwrittenSincePlan reports whether the live sidecar shows NO extractor write
// since the plan: its bytes equal the plan's pre-dispatch pin, or equal what
// the previous restore of this plan left (a retry round whose dispatch wrote
// nothing). basis names which, for the report.
//
// A re-run inside the same round is not a stalled retry: when no verify has
// run since the previous restore, an item that restore already restored is
// still restored, even though its bytes have not moved.
//
// A pin that exists but cannot be read is "not measured" — returned as
// unmeasured, never read as a match. Absence of a pin (an item with no prior
// sidecar) is not a gap: there is nothing to compare, and any sidecar there
// now was written by someone.
func unwrittenSincePlan(root, planID, itemID string, live []byte, priorRestore *RestoreReceipt, sameRound bool) (unwritten bool, basis string, unmeasured error) {
	sum := sha256.Sum256(live)
	liveSum := hex.EncodeToString(sum[:])
	priorSum := ""
	priorUnwritten := false
	if priorRestore != nil {
		priorSum = priorRestore.RestoredDigests[itemID]
		for _, id := range priorRestore.Unwritten {
			if id == itemID {
				priorUnwritten = true
			}
		}
	}
	if sameRound && !priorUnwritten && priorSum != "" && priorSum == liveSum {
		return false, "", nil
	}

	pin, perr := os.ReadFile(PinSnapshotPath(root, planID, itemID))
	switch {
	case perr == nil:
		if bytes.Equal(live, pin) {
			return true, "the pre-dispatch pin", nil
		}
	case !os.IsNotExist(perr):
		unmeasured = perr
	}
	if priorSum != "" && priorSum == liveSum {
		return true, fmt.Sprintf("what restore left at %s", priorRestore.RestoredAt), unmeasured
	}
	return false, "", unmeasured
}

// writeTimeCutoff is the instant after which a sidecar mtime counts as a new
// extractor write: one second past the later of the previous restore and the
// verify after it. Both are restore's or verify's own write moments, and
// verified_at is recorded in whole seconds, hence the margin. Zero when there
// was no previous restore.
func writeTimeCutoff(priorRestore *RestoreReceipt, priorVerify *Receipt) time.Time {
	var cutoff time.Time
	for _, stamp := range []string{restoredAtOf(priorRestore), verifiedAtOf(priorVerify)} {
		if stamp == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339Nano, stamp); err == nil && t.Add(time.Second).After(cutoff) {
			cutoff = t.Add(time.Second)
		}
	}
	return cutoff
}

func restoredAtOf(r *RestoreReceipt) string {
	if r == nil {
		return ""
	}
	return r.RestoredAt
}

func verifiedAtOf(r *Receipt) string {
	if r == nil {
		return ""
	}
	return r.VerifiedAt
}

// fileSHA256OrEmpty is sha256 of the file, or "" when it cannot be read.
func fileSHA256OrEmpty(path string) string {
	sum, err := sha256Hex(path)
	if err != nil {
		return ""
	}
	return sum
}

func containsString(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
