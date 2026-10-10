package phasea

// SPEC-009 Plan A Phase 12 — deterministic anchor repair. Phase A invokes  // edikt-guard:allow
// AutoRepairAnchors before LLM dispatch and skips the dispatch entirely
// when the pure-Go repair resolves every stale anchor.
//
// This file is the integration seam between govrun's Phase A bookkeeping
// and sidecar.AutoRepairAnchors. PHASEA NEVER PERSISTS A SIDECAR: it repairs
// the sidecar it is handed in memory and RETURNS the repaired bytes. Whether
// those bytes reach disk is govrun's decision, made through its one writer
// (writeSourceSidecar), which refuses under --check / --dry-run and keeps
// the `$schema` header the file opened with.
//
// It used to write here (#9, c4-a): a direct file write reachable from every
// caller, including the non-writing flags, which re-marshalled the file and
// dropped its header. A write that lives in the callee cannot be gated by a
// flag the callee is never told about.

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
	// excerpts whose LineStart/LineEnd (or quote) were updated by the
	// quote-primary, line-secondary strategy.
	AnchorsRepaired int

	// StillStale is true when at least one anchor remains stale after
	// the repair pass — caller MUST proceed with LLM dispatch.
	StillStale bool

	// Repaired is the canonical YAML of the repaired sidecar, set only when
	// AnchorsRepaired > 0. Nil means there is nothing to persist. It carries
	// no `$schema` header: the caller's writer re-attaches the one the file
	// opened with.
	Repaired []byte
}

// TryAutoRepair reads the parent .md, runs sidecar.AutoRepairAnchors on sc
// IN MEMORY, and re-checks staleness. It never writes a file.
//
// Pure-Go, no LLM dispatch. Errors that prevent the repair (parent read,
// sidecar marshal) are surfaced to the caller — phasea callers fall back to
// LLM dispatch on error rather than aborting.
//
// projectRoot resolves the sidecar's relative `path:` field. parentPath is
// the absolute path to the parent .md, when the caller already has it; if
// empty, TryAutoRepair joins projectRoot with sc.Path.
func TryAutoRepair(sc *sidecar.Sidecar, projectRoot, parentPath string) (RepairOutcome, error) {
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

	if out.AnchorsRepaired > 0 {
		// Marshal, NOT the load-time cache: sc was mutated after Load, and
		// returning the cached bytes is what once persisted the ORIGINAL
		// stale file on every repair (the phantom-stale loop: compile
		// reported "fully resolved", disk kept the drifted anchors).
		yamlBytes, merr := sidecar.Marshal(sc)
		if merr != nil {
			return out, fmt.Errorf("autorepair: marshal repaired sidecar: %w", merr)
		}
		out.Repaired = yamlBytes
	}
	return out, nil
}
