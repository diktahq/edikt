// Package phasea's plan.go is `bin/edikt phasea plan`'s deterministic half
// (SPEC-013, Phase 2 of PLAN-v080-reliability-core). It computes the
// stale-sidecar work-list (plan_worklist.go, reusing the SAME detection
// twophase.go's dispatch loop already performs), derives a content-addressed
// plan_id (plan_hash.go, ADR-020), captures the pre-dispatch pin-set every
// item needs for D6's later restore, and — only when the work-list is
// within budget — writes a schema-valid plan to
// .edikt/state/phasea/<plan_id>.plan.json and appends one line to
// .edikt/state/phasea-dispatch.jsonl.
//
// PURITY (INV-012): this file imports no os/exec, no net/http, and performs
// no LLM dispatch of any kind. Every operation here is filesystem read,
// hash, and a deterministic filesystem write — the same discipline
// internal/phaseb already holds Phase B to.
//
// Cobra wiring (the `bin/edikt phasea plan` verb itself) is Phase 4's job,
// per the phase brief. Plan() is the library that verb will call.
//
// EXIT CODES (documented and asserted by TestPlanExitCodes):
//
//	0  ExitPlanEmitted    — a schema-valid plan was written.
//	1  ExitNothingToPlan  — the work-list is empty; not an error, there is
//	                        simply nothing stale or missing to plan.
//	4  ExitRefused        — a whole-plan refusal: budget ceiling exceeded,
//	                        extractor prompt_version UNKNOWN, a corpus load
//	                        error, an item that failed idvalidate, a
//	                        pin-set capture failure, or a malformed
//	                        phasea.* config value. Chosen to match
//	                        SPEC-013's own "4 is the pre-run refusal code"
//	                        convention (D9) rather than invent a second
//	                        refusal code space for a sibling command.
//
// JOURNALING: phasea-journal.v1's `refuse` event carries a `cause` enum
// widened 2026-08-20 (SPEC-013 SR-006, PRD-007 FR-018) from four values to
// nine, specifically so every ExitRefused path below could journal instead
// of silently returning a plain Go error. All seven plan-time causes now
// journal exactly one line before Plan() returns:
//
//	invalid-config          — LoadConfig failed (missing/malformed
//	                           .edikt/config.yaml or a non-positive
//	                           phasea.* value).
//	prompt-version-unknown  — the extractor prompt_version could not be
//	                           resolved.
//	corpus-load-error       — sidecar.Discover failed outright, or one or
//	                           more pairs failed to load/parse.
//	item-validation-failed  — one or more items failed buildItem's
//	                           idvalidate check.
//	ceiling-exceeded        — stale count exceeds phasea.ceiling.
//	force-ceiling-exceeded  — stale count exceeds phasea.force_ceiling on
//	                           a --force run.
//	pin-capture-failed      — D6's mandatory pin-set capture failed.
//
// invalid-config and prompt-version-unknown journal with the
// PlanIDUnresolved sentinel (plan_types.go): both refuse before a real
// corpus_hash/config_hash pair can be computed at all. Every other cause
// journals a real, content-derived plan_id. See plan_types.go's Cause* doc
// for the full reasoning.
package phasea

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Exit codes — see the package doc above.
const (
	ExitPlanEmitted   = 0
	ExitNothingToPlan = 1
	ExitRefused       = 4
)

// PlanOptions configures one Plan() call.
type PlanOptions struct {
	// ProjectRoot is the tree to plan against. Empty means the current
	// working directory (mirrors phasea.Task.ProjectRoot's own convention).
	ProjectRoot string

	// Force selects phasea.force_ceiling instead of phasea.ceiling as the
	// budget this run's item count is checked against (D3) — the higher
	// budget an explicitly human-invoked forced run is checked against.
	//
	// Force does NOT change which items are planned: computeWorkList's
	// stale-sidecar detection is the one detector this file has (see the
	// phase brief's point 1), so a forced run still plans exactly the
	// missing/stale work-list, just against the higher ceiling. A truly
	// whole-corpus "replan every artifact regardless of staleness" mode —
	// the shape `gov reextract --force` will eventually need — has no
	// representable `reason` in phasea-dispatch-plan.v1's closed three-value
	// enum (plan_types.go's Reason* doc) and is left to reextract's own
	// Phase 4 rewire, which owns its own ledger-based candidate list and can
	// call the lower-level helpers in this file directly rather than
	// through the stale-only Plan() entry point.
	Force bool

	// Only restricts the plan to these artifact IDs, when non-empty.
	//
	// It exists because `gov reextract --only ADR-003` must still mean what
	// it says once planning moved here: without it the flag was silently
	// dropped and a targeted re-extraction quietly became a whole-corpus
	// one. Filtering happens before the budget check, so a narrow run is
	// checked against the count it will actually dispatch.
	Only []string
}

// PlanOutcome is Plan()'s result. Plan is non-nil only when ExitCode ==
// ExitPlanEmitted.
type PlanOutcome struct {
	ExitCode int
	Plan     *DispatchPlan
	Message  string
}

// Plan computes the stale-sidecar work-list and, when it is within budget,
// writes a schema-valid plan file and journals the emission. See the
// package doc for exit codes and journaling scope.
func Plan(opts PlanOptions) (*PlanOutcome, error) {
	root := opts.ProjectRoot

	cfg, err := LoadConfig(root)
	if err != nil {
		msg := err.Error()
		// LoadConfig failed, so there is no Config to hash — journal with
		// the sentinel plan_id (see PlanIDUnresolved's doc).
		if jerr := appendJournal(root, JournalRecord{
			Event:  JournalEventRefuse,
			PlanID: PlanIDUnresolved,
			Items:  0,
			Cause:  CauseInvalidConfig,
		}); jerr != nil {
			msg = msg + fmt.Sprintf(" (additionally, the refusal itself could not be journaled: %v)", jerr)
		}
		return &PlanOutcome{ExitCode: ExitRefused, Message: msg}, errors.New(msg)
	}

	// Refuse before doing any corpus work when the extraction contract
	// itself is unpinned (point 6 / ADR-055 §4 by analogy). An UNKNOWN
	// prompt_version must never be silently baked into a plan that fans out
	// real subagent dispatches downstream.
	promptVersion, pverr := ResolveExtractorPromptVersion(root)
	if pverr != nil || promptVersion == ExtractorPromptVersionUnknown {
		msg := fmt.Sprintf(
			"refusing to emit a plan: extractor prompt_version resolved to %s (%v) — "+
				"an unpinned extraction contract is never silently baked into a plan (ADR-055 §4 by analogy)",
			ExtractorPromptVersionUnknown, pverr)
		// This refusal happens BEFORE any corpus work (deliberately — see
		// the comment above), so there is no corpus_hash to derive a real
		// plan_id from — journal with the sentinel.
		if jerr := appendJournal(root, JournalRecord{
			Event:  JournalEventRefuse,
			PlanID: PlanIDUnresolved,
			Items:  0,
			Cause:  CausePromptVersionUnknown,
		}); jerr != nil {
			msg = msg + fmt.Sprintf(" (additionally, the refusal itself could not be journaled: %v)", jerr)
		}
		return &PlanOutcome{ExitCode: ExitRefused, Message: msg}, errors.New(msg)
	}

	items, loadErrs, validationErrs, werr := computeWorkList(root, cfg.Dirs())
	if werr != nil {
		msg := werr.Error()
		// sidecar.Discover itself failed before any items could be built —
		// no items exist at all, so journal with the sentinel plan_id.
		if jerr := appendJournal(root, JournalRecord{
			Event:  JournalEventRefuse,
			PlanID: PlanIDUnresolved,
			Items:  0,
			Cause:  CauseCorpusLoadError,
		}); jerr != nil {
			msg = msg + fmt.Sprintf(" (additionally, the refusal itself could not be journaled: %v)", jerr)
		}
		return &PlanOutcome{ExitCode: ExitRefused, Message: msg}, errors.New(msg)
	}
	// Narrow to --only BEFORE the budget check, so a targeted run is checked
	// against what it will actually dispatch rather than the whole corpus.
	if len(opts.Only) > 0 {
		want := make(map[string]bool, len(opts.Only))
		for _, id := range opts.Only {
			want[id] = true
		}
		kept := items[:0]
		for _, it := range items {
			if want[it.ItemID] {
				kept = append(kept, it)
			}
		}
		items = kept
	}

	if len(loadErrs) > 0 || len(validationErrs) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "refusing to emit a plan: %d corpus error(s), %d item validation failure(s) — "+
			"every artifact in the governed corpus must load and validate before any plan is written, "+
			"mirroring twophase.go's own whole-run refusal on a load error:\n", len(loadErrs), len(validationErrs))
		for _, e := range loadErrs {
			fmt.Fprintf(&b, "  %s\n", e.String())
		}
		for _, e := range validationErrs {
			fmt.Fprintf(&b, "  %s\n", e.String())
		}
		msg := b.String()

		// A real, content-derived plan_id from whatever DID successfully
		// build (items holds only the pairs that both loaded and validated)
		// — reproducible for the same corpus/config state, unlike the
		// pre-corpus-work sentinel above. loadErrs takes priority over
		// validationErrs when both are non-empty: a corpus that does not
		// even load is the more fundamental problem, and the message above
		// already names every individual failure either way.
		cause := CauseItemValidationFailed
		if len(loadErrs) > 0 {
			cause = CauseCorpusLoadError
		}
		planID := derivePlanID(corpusHash(items), configHash(cfg, promptVersion))
		if jerr := appendJournal(root, JournalRecord{
			Event:  JournalEventRefuse,
			PlanID: planID,
			Items:  len(items),
			Cause:  cause,
		}); jerr != nil {
			msg = msg + fmt.Sprintf(" (additionally, the refusal itself could not be journaled: %v)", jerr)
		}
		return &PlanOutcome{ExitCode: ExitRefused, Message: msg}, errors.New(msg)
	}

	if len(items) == 0 {
		return &PlanOutcome{
			ExitCode: ExitNothingToPlan,
			Message:  "nothing to plan — no missing or stale sidecars in the governed corpus",
		}, nil
	}

	for i := range items {
		items[i].PromptVersion = promptVersion
	}

	cHash := corpusHash(items)
	kHash := configHash(cfg, promptVersion)
	planID := derivePlanID(cHash, kHash)

	ceiling := cfg.Ceiling
	configKey := "phasea.ceiling"
	applied := AppliedCeilingCeiling
	cause := CauseCeilingExceeded
	if opts.Force {
		ceiling = cfg.ForceCeiling
		configKey = "phasea.force_ceiling"
		applied = AppliedCeilingForce
		cause = CauseForceCeilingExceeded
	}

	if len(items) > ceiling {
		msg := fmt.Sprintf(
			"refusing to emit a plan: %d stale/missing sidecar(s) exceeds %s (configured value %d, observed count %d)",
			len(items), configKey, ceiling, len(items))
		if jerr := appendJournal(root, JournalRecord{
			Event:  JournalEventRefuse,
			PlanID: planID,
			Items:  len(items),
			Cause:  cause,
		}); jerr != nil {
			msg = msg + fmt.Sprintf(" (additionally, the refusal itself could not be journaled: %v)", jerr)
		}
		return &PlanOutcome{ExitCode: ExitRefused, Message: msg}, errors.New(msg)
	}

	// D6 — mandatory pin-set capture. A capture failure refuses the WHOLE
	// plan; nothing is written. Every item is attempted (not short-circuited
	// on the first failure) so the refusal message names every affected
	// item in one pass.
	var captureErrs []string
	for _, it := range items {
		if cerr := capturePinSnapshot(root, planID, it); cerr != nil {
			captureErrs = append(captureErrs, fmt.Sprintf("%s: %v", it.ItemID, cerr))
		}
	}
	if len(captureErrs) > 0 {
		msg := fmt.Sprintf(
			"refusing to emit a plan: pin-set capture failed for %d item(s) — D6 requires the "+
				"pre-dispatch pin-set to be recorded for every item before a plan is emitted, because "+
				"restore.go needs the sidecar as it stood BEFORE the extractor overwrites it:\n  %s",
			len(captureErrs), strings.Join(captureErrs, "\n  "))
		// planID was already derived above (cHash/kHash cover the full
		// work-list, computed before this D6 pass ever runs) — the SAME id
		// the plan file would have carried had capture succeeded.
		if jerr := appendJournal(root, JournalRecord{
			Event:  JournalEventRefuse,
			PlanID: planID,
			Items:  len(items),
			Cause:  CausePinCaptureFailed,
		}); jerr != nil {
			msg = msg + fmt.Sprintf(" (additionally, the refusal itself could not be journaled: %v)", jerr)
		}
		return &PlanOutcome{ExitCode: ExitRefused, Message: msg}, errors.New(msg)
	}

	plan := &DispatchPlan{
		SchemaVersion: SchemaVersionPlan,
		PlanID:        planID,
		CreatedFrom:   CreatedFrom{CorpusHash: cHash, ConfigHash: kHash},
		Budget: Budget{
			Items:          len(items),
			Ceiling:        cfg.Ceiling,
			ForceCeiling:   cfg.ForceCeiling,
			AppliedCeiling: applied,
		},
		Items: toPlanItems(items),
	}

	if err := writePlanFile(root, plan); err != nil {
		msg := fmt.Sprintf("refusing to emit a plan: could not write plan file: %v", err)
		return &PlanOutcome{ExitCode: ExitRefused, Message: msg}, errors.New(msg)
	}

	// The plan is now real and on disk — a genuine deliverable regardless
	// of what happens next. A journal-append failure here is reported
	// (INV-013: absence of a measured journal line must not read as a
	// clean success) but does NOT retract the plan file: rolling back a
	// successfully-written, schema-valid, byte-stable artifact over an
	// unrelated journal I/O failure would destroy real, usable output to
	// "fix" an audit-trail gap that is itself fully visible in the returned
	// error.
	if jerr := appendJournal(root, JournalRecord{
		Event:  JournalEventPlan,
		PlanID: planID,
		Items:  len(items),
	}); jerr != nil {
		return &PlanOutcome{ExitCode: ExitPlanEmitted, Plan: plan,
				Message: fmt.Sprintf("plan %s emitted (%d item(s)) but the journal line could not be written: %v", planID, len(items), jerr)},
			fmt.Errorf("plan %s emitted but journal append failed: %w", planID, jerr)
	}

	return &PlanOutcome{
		ExitCode: ExitPlanEmitted,
		Plan:     plan,
		Message:  fmt.Sprintf("plan %s: %d item(s)", planID, len(items)),
	}, nil
}

// toPlanItems strips workItem's in-process-only fields (absArtifactPath,
// beforeSidecar) down to the schema-shaped PlanItem the file actually
// serializes. Order is preserved — computeWorkList already sorted
// ascending by item_id (SAC-002).
func toPlanItems(items []workItem) []PlanItem {
	out := make([]PlanItem, len(items))
	for i, it := range items {
		out[i] = it.PlanItem
	}
	return out
}

// PlanFilePath is .edikt/state/phasea/<plan_id>.plan.json.
func PlanFilePath(root, planID string) string {
	return filepath.Join(root, ".edikt", "state", "phasea", planID+".plan.json")
}

// writePlanFile serializes plan deterministically (fixed struct field
// order, items already sorted ascending by item_id) and writes it via
// temp-file-then-rename so a concurrent reader never observes a partial
// file.
func writePlanFile(root string, plan *DispatchPlan) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dst := PlanFilePath(root, plan.PlanID)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".plan.*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// pinSnapshotPath is where an item's pre-dispatch sidecar bytes are
// captured, keyed by plan_id so a later plan (a different corpus/config
// state) never collides with an earlier one's snapshots. Not part of any
// landed schema (phasea-dispatch-plan.v1/phasea-journal.v1 are the only
// schemas this phase's plan output must validate against) — this is
// plan.go's own private on-disk format, consumed by restore.go (Phase 3)
// the same way reextract/snapshot.go's writeSnapshot already is consumed
// by reextract's own review step.
func pinSnapshotPath(root, planID, itemID string) string {
	return filepath.Join(root, ".edikt", "state", "phasea", planID+".pins", itemID+".yaml")
}

// PinSnapshotPath is the exported form, for Phase 3's restore.go to
// construct the identical path independently.
func PinSnapshotPath(root, planID, itemID string) string {
	return pinSnapshotPath(root, planID, itemID)
}

// capturePinSnapshot records item's pre-dispatch sidecar bytes to disk so
// restore.go can reconstitute the exact `before` sidecar PreservePinned
// needs (reextract/preserve.go:159), after the real extractor dispatch has
// already overwritten the live sidecar. A nil beforeSidecar (the "missing"
// reason — no prior sidecar existed) is a real, valid case with nothing to
// capture, per PreservePinned's own contract ("before == nil" means
// "nothing to preserve, the ordinary bootstrap case") — it succeeds
// vacuously rather than writing an empty file.
func capturePinSnapshot(root, planID string, it workItem) error {
	if it.beforeSidecar == nil {
		return nil
	}
	dst := pinSnapshotPath(root, planID, it.ItemID)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".pin.*.yaml.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(it.beforeSidecar); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
