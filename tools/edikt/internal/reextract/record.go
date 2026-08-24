package reextract

// record.go — the second half of re-extraction, after tier-1 has fanned out.
//
// Run() plans and hands off; it no longer dispatches, because dispatching
// means invoking an LLM and INV-012 forbids a tier-2 binary from doing that
// (SPEC-013, PLAN-v080 Phase 4). This is what closes the loop: tier-1 calls
// it once the extractors have finished, and it drives phasea.Restore while
// supplying reextract's own ledger write through the per-item hook.
//
// The ledger stays here rather than moving into phasea for the reason the
// hook is a callback at all: the ledger's on-disk schema is reextract's, and
// phasea should not learn a caller's format to keep a behaviour alive.

import (
	"fmt"
	"strings"
	"path/filepath"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
)

// RecordOptions configures one post-fan-out recording pass.
type RecordOptions struct {
	ProjectRoot string
	PlanID      string
	LedgerPath  string
	Now         func() time.Time

	// OnRecorded, when set, fires after each artifact's ledger entry has been
	// written and flushed to disk.
	//
	// It exists so a test can observe on-disk state at the moment a kill
	// would land — the only way to tell per-completion persistence from an
	// end-of-run save, since both leave identical state after a clean return.
	// A test that checked only the final ledger would pass against the
	// batched implementation this hook replaced.
	OnRecorded func(itemID string, n int)
}

// RecordResult reports what the pass actually did.
type RecordResult struct {
	PlanID    string   `json:"plan_id"`
	Recorded  int      `json:"recorded"`
	Failed    int      `json:"failed"`
	FailedIDs []string `json:"failed_ids,omitempty"`
}

// Record restores pins for a completed plan and writes one ledger entry per
// artifact as each is finished.
//
// Per item, not at the end: a run killed halfway must leave the artifacts it
// already finished recorded, or the resume re-extracts work that is done —
// which is the expensive thing the ledger exists to prevent. That guarantee
// is phasea.Restore's, delivered through OnItemRestored, and is pinned by
// TestLedgerPersistsPerArtifactOnMidBatchKill.
func Record(opts RecordOptions) (*RecordResult, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.LedgerPath == "" {
		opts.LedgerPath = filepath.Join(opts.ProjectRoot, LedgerRelPath)
	}

	ledger := loadLedger(opts.LedgerPath)
	if ledger.Artifacts == nil {
		ledger.Artifacts = map[string]Entry{}
	}

	res := &RecordResult{PlanID: opts.PlanID}
	n := 0

	out, err := phasea.Restore(phasea.RestoreOptions{
		ProjectRoot: opts.ProjectRoot,
		PlanID:      opts.PlanID,
		OnItemRestored: func(item phasea.PlanItem, itemErr error) {
			e := Entry{CompletedAt: opts.Now().UTC().Format(time.RFC3339)}
			if itemErr != nil {
				e.Status = StatusFailed
				e.Err = oneLine(itemErr.Error())
				res.Failed++
				res.FailedIDs = append(res.FailedIDs, item.ItemID)
			} else {
				e.Status = StatusDone
				// Hash AFTER restore, not before: restore reinstates pins and
				// stamps the body digest, both of which rewrite the file. A
				// hash taken earlier would record a digest the sidecar on disk
				// no longer has, and "already regenerated" would then be false
				// for every artifact the moment it was written.
				e.SidecarSHA256 = fileSHA256(filepath.Join(opts.ProjectRoot, item.ExpectedOutput.Path))
				res.Recorded++
			}
			ledger.Artifacts[item.ItemID] = e
			// Persist inside the hook. Deferring to the end would reintroduce
			// exactly the batch-end write this hook replaced.
			if serr := saveLedger(opts.LedgerPath, ledger); serr != nil {
				res.FailedIDs = append(res.FailedIDs, item.ItemID+" (ledger write failed)")
			}
			n++
			if opts.OnRecorded != nil {
				opts.OnRecorded(item.ItemID, n)
			}
		},
	})
	if err != nil {
		return res, fmt.Errorf("restore for plan %s: %w", opts.PlanID, err)
	}
	if out != nil && out.ExitCode != phasea.ExitRestoreOK {
		return res, fmt.Errorf("restore for plan %s exited %d: %s", opts.PlanID, out.ExitCode, out.Message)
	}
	// Per-item failures do not move Restore's exit code — it completed, and
	// the items it could not finish are reported per item. They must still
	// surface here: a recording pass that silently returns nil while four of
	// six artifacts were never extracted would let a partial batch read as a
	// whole one, which is the state the ledger exists to make visible.
	if res.Failed > 0 {
		return res, fmt.Errorf("%d of %d artifact(s) could not be recorded: %s (re-run to resume; recorded artifacts are not re-planned)",
			res.Failed, res.Recorded+res.Failed, strings.Join(res.FailedIDs, ", "))
	}
	return res, nil
}
