package cmd

// upgrade_stale_plans.go — an unconsumed plan written by an older
// binary's schema can never be verified or consumed once the binary moves
// on (phasea verify refuses it by name — see the stale-schema-version
// message in internal/phasea/verify.go). Left in place it is a dead file a
// user has no way to distinguish from one still in flight. Project
// reconciliation removes it and says so, one line per plan.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
)

// stalePlanFile is the minimal shape read from a plan file — just enough to
// decide staleness, never the full DispatchPlan (this is advisory
// housekeeping, not a load-bearing parse).
type stalePlanFile struct {
	SchemaVersion int    `json:"schema_version"`
	PlanID        string `json:"plan_id"`
}

type stalePlanReceipt struct {
	Consumed bool `json:"consumed"`
}

// removeStalePlans deletes unconsumed phasea plan files (and their sibling
// receipt / restore-receipt / pin-snapshot state) under
// .edikt/state/phasea/ whose schema_version is older than the current
// binary writes.
//
// A plan already consumed (its receipt has consumed:true) is left alone —
// it is finished business, not stale, and its state is the audit record of
// completed work. A plan at the current schema_version is left alone too —
// it may still be mid-fan-out. A malformed plan file is skipped, not
// fatal: this is advisory housekeeping over runtime state, not a
// load-bearing parse.
func removeStalePlans(projectRoot string, out io.Writer) (int, error) {
	dir := filepath.Join(projectRoot, ".edikt", "state", "phasea")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	removed := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".plan.json") {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(dir, name))
		if rerr != nil {
			continue
		}
		var plan stalePlanFile
		if jerr := json.Unmarshal(data, &plan); jerr != nil || plan.PlanID == "" {
			continue
		}
		if plan.SchemaVersion >= phasea.SchemaVersionPlan {
			continue
		}

		receiptPath := phasea.ReceiptFilePath(projectRoot, plan.PlanID)
		if rdata, rerr2 := os.ReadFile(receiptPath); rerr2 == nil {
			var receipt stalePlanReceipt
			if json.Unmarshal(rdata, &receipt) == nil && receipt.Consumed {
				continue
			}
		}

		if err := os.Remove(phasea.PlanFilePath(projectRoot, plan.PlanID)); err != nil {
			return removed, fmt.Errorf("removing stale plan %s: %w", plan.PlanID, err)
		}
		// Best-effort: a receipt/restore-receipt/pin-snapshot set with no
		// plan behind it is orphaned, not useful. Their absence is not an
		// error — a plan may never have reached restore or verify.
		_ = os.Remove(receiptPath)
		_ = os.Remove(phasea.RestoreReceiptPath(projectRoot, plan.PlanID))
		_ = os.RemoveAll(filepath.Join(dir, plan.PlanID+".pins"))

		fmt.Fprintf(out, "  removed stale plan %s (schema v%d, this binary writes v%d)\n",
			plan.PlanID, plan.SchemaVersion, phasea.SchemaVersionPlan)
		removed++
	}
	return removed, nil
}
