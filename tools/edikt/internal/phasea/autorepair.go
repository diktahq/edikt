package phasea

// SPEC-009 Plan A Phase 12 — deterministic anchor repair. Phase A invokes  // edikt-guard:allow
// AutoRepairAnchors before LLM dispatch and skips the dispatch entirely
// when the pure-Go repair resolves every stale anchor.
//
// This file is the integration seam between govrun's Phase A bookkeeping
// and sidecar.AutoRepairAnchors: govrun calls TryAutoRepair for each
// stale pair, persists the rewritten sidecar on success, and only enqueues
// a phasea.Task when TryAutoRepair reports the sidecar is still stale.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// RepairOutcome reports the result of one TryAutoRepair invocation.
type RepairOutcome struct {
	// AnchorsRepaired is the count of directive/prohibition source
	// excerpts whose LineStart/LineEnd were updated by the quote-primary,
	// line-secondary strategy.
	AnchorsRepaired int

	// StillStale is true when at least one anchor remains stale after
	// the repair pass — caller MUST proceed with LLM dispatch.
	StillStale bool

	// Rewrote is true when AutoRepairAnchors mutated the sidecar and
	// the new YAML was persisted back to SidecarPath.
	Rewrote bool
}

// TryAutoRepair reads the parent .md, runs sidecar.AutoRepairAnchors on a
// loaded sidecar, persists the repaired sidecar to disk when any anchor
// was updated, and re-checks IsStale.
//
// Pure-Go, no LLM dispatch. Errors that prevent the repair (parent read,
// sidecar marshal, sidecar write) are surfaced to the caller — phasea
// callers fall back to LLM dispatch on error rather than aborting.
//
// projectRoot resolves the sidecar's relative `path:` field. parentPath is
// the absolute path to the parent .md, when the caller already has it; if
// empty, TryAutoRepair joins projectRoot with sc.Path.
func TryAutoRepair(sc *sidecar.Sidecar, projectRoot, parentPath, sidecarPath string) (RepairOutcome, error) {
	out := RepairOutcome{}
	if sc == nil {
		return out, fmt.Errorf("autorepair: nil sidecar")
	}
	// migration_preserved sidecars are always stale by contract — the
	// LLM extractor must run to synthesise the canonical fields. Skip
	// the repair entirely so we don't mask that signal.
	if sc.MigrationPreserved != nil {
		out.StillStale = true
		return out, nil
	}
	if parentPath == "" {
		parentPath = sc.Path
		if !filepath.IsAbs(parentPath) {
			parentPath = filepath.Join(projectRoot, sc.Path)
		}
	}
	data, err := os.ReadFile(parentPath)
	if err != nil {
		out.StillStale = true
		return out, fmt.Errorf("autorepair: read parent %s: %w", parentPath, err)
	}
	lines := strings.Split(string(data), "\n")

	out.AnchorsRepaired = sidecar.AutoRepairAnchors(sc, lines)
	out.StillStale = sidecar.IsStale(sc, lines)

	if out.AnchorsRepaired > 0 && sidecarPath != "" {
		// Persist the repaired sidecar so subsequent loads see the
		// corrected anchors. Failure to write surfaces to the caller
		// but does not change StillStale — the in-memory repair still
		// stands for the current Phase A invocation.
		// Marshal, NOT Marshal: sc was mutated after Load, and Marshal
		// returns the load-time cached bytes — which silently persisted the
		// ORIGINAL stale file on every repair (the phantom-stale loop:
		// compile reports "fully resolved", disk keeps the drifted anchors,
		// the stop-hook re-reads them and warns again).
		yamlBytes, merr := sidecar.Marshal(sc)
		if merr != nil {
			return out, fmt.Errorf("autorepair: marshal repaired sidecar: %w", merr)
		}
		if werr := os.WriteFile(sidecarPath, yamlBytes, 0o644); werr != nil {
			return out, fmt.Errorf("autorepair: write %s: %w", sidecarPath, werr)
		}
		out.Rewrote = true
	}
	return out, nil
}
