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
//	                        pin-set capture failure (including a pin that
//	                        already exists with different bytes), a
//	                        malformed phasea.* config value, a corpus-path
//	                        plan_id that is already terminal, or an item
//	                        whose latest plan is unclosed and whose sidecar
//	                        has moved since (plan_succession.go). Chosen to match
//	                        SPEC-013's own "4 is the pre-run refusal code"
//	                        convention (D9) rather than invent a second
//	                        refusal code space for a sibling command.
//
// JOURNALING: phasea-journal.v1's `refuse` event carries a `cause` enum
// widened 2026-08-20 (SPEC-013 SR-006, PRD-007 FR-018) from four values to
// nine, specifically so every ExitRefused path below could journal instead
// of silently returning a plain Go error — and widened again 2026-10-04 (#11)
// with the last three below. Every plan-time cause journals exactly one line
// before Plan() returns:
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
//	sidecar-absent-on-request — a requested extraction found no sidecar
//	                           and no --bootstrap declaration (D414).
//	plan-id-terminal        — the corpus path derived the id of a consumed
//	                           or FAILED plan; a requested extraction
//	                           derives a successor instead.
//	unclosed-plan           — the latest plan covering an item was never
//	                           closed and its sidecar has moved since.
//
// invalid-config, prompt-version-unknown and the --artifact/--artifacts
// resolution refusals (sidecar-absent-on-request, an unknown path) journal
// with the PlanIDUnresolved sentinel (plan_types.go): each refuses before a
// real corpus_hash/config_hash pair can be computed at all. Every other cause
// journals a real, content-derived plan_id — plan-id-terminal the terminal
// plan's, unclosed-plan the id the refused plan would have carried. See plan_types.go's Cause* doc
// for the full reasoning.
package phasea

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
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
	// missing/stale work-list, just against the higher ceiling. A caller that
	// needs "replan these regardless of staleness" — which is what
	// `gov reextract` needs after a CONTRACT change, when nothing is stale —
	// passes them in Artifacts and gets reason `requested` per item.
	Force bool

	// Only restricts the plan to these artifact IDs, when non-empty.
	//
	// It exists because `gov reextract --only ADR-003` must still mean what
	// it says once planning moved here: without it the flag was silently
	// dropped and a targeted re-extraction quietly became a whole-corpus
	// one. Filtering happens before the budget check, so a narrow run is
	// checked against the count it will actually dispatch.
	Only []string

	// Artifact plans exactly this one artifact (a path relative to the
	// project root, or absolute) with reason `requested`, whether or not the
	// stale detector would have selected it — SPEC-013 SR-011, the path the
	// per-artifact compile commands take. An unknown path is a refusal.
	Artifact string

	// Artifacts plans exactly these artifacts, each with reason `requested`,
	// whether or not the stale detector would have selected them. It is the
	// plural of Artifact and takes the same path through requestedItem.
	//
	// It exists because `gov reextract --force` did nothing for weeks and said
	// it had. reextract builds its own candidate list, then called Plan with
	// only ProjectRoot/Force/Only — so Plan ignored that list and re-derived a
	// STALE-ONLY work-list. After a CONTRACT change nothing is stale, every
	// anchor still matches, so the plan came back empty and the run reported
	// the candidate count it never dispatched (D425).
	//
	// The comment that used to sit here said a whole-corpus replan "has no
	// representable reason in phasea-dispatch-plan.v1's closed three-value
	// enum". That was true when written and is not now: `requested` was added
	// 2026-09-02 (aa4173d9) and is in the v3 schema's enum. The blocker was
	// removed a fortnight before anyone noticed the verb was still broken —
	// which is its own lesson about comments that record an obstacle and
	// outlive it.
	Artifacts []string

	// Bootstrap declares that the Artifact genuinely has NO prior sidecar —
	// that this is a first extraction, not a re-extraction of one that went
	// missing. It is accepted ONLY with Artifact, and it exists because the
	// two cases are indistinguishable from disk and the difference is
	// destructive (D414).
	//
	// A sidecar's `manual_directives` and `suppressed_directives` are prose a
	// human wrote INTO THE SIDECAR; they appear nowhere in the parent .md, so
	// the extractor cannot regenerate them. pinpreserve carries them forward
	// from the pre-dispatch pin — and when the sidecar is already gone there
	// is no pin to take, so a `requested` extraction over an absent sidecar
	// destroys them and every downstream report still reads clean. bok found
	// exactly that in the field: four directives gone, including a
	// SQL-parameterisation rule, with plan, restore and verify all at exit 0.
	//
	// So the machine does not guess. It refuses and asks, and this flag is
	// the human's answer. It does NOT apply to the corpus path, where
	// ReasonMissing IS the bootstrap case and refusing would break every
	// legitimate first compile.
	Bootstrap bool

	// ContractLead, when set, returns the one-line contract report for the
	// project at the given root. A refusal for an unresolvable extractor leads
	// with it. It is a callback because the report is built in a package that
	// imports this one.
	ContractLead func(root string) string
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
	promptVersion, promptNote, pverr := ResolveExtractorPromptVersionDetail(root)
	if pverr != nil || promptVersion == ExtractorPromptVersionUnknown {
		// Every refusal for an unresolvable extractor is two lines: the contract
		// report's own line alone (the one `edikt doctor --contract-state` prints,
		// which carries the remedy), then the specific detail. The line is supplied by
		// the caller because the report lives in a package that imports this one.
		detail := fmt.Sprintf("refusing to emit a plan: extractor prompt_version resolved to %s (%v); an unpinned extraction contract is never silently baked into a plan",
			ExtractorPromptVersionUnknown, pverr)
		msg := detail
		if opts.ContractLead != nil {
			if lead := opts.ContractLead(root); lead != "" {
				msg = lead + "\n" + detail
			}
		}
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

	// Resolved from the same installed agent file prompt_version was just
	// read from (the project's when it has one, else the profile's), so by the
	// time prompt_version resolved successfully this should too — the
	// "unrecorded" fallback exists for a read racing a concurrent edit
	// between the two resolutions, not as an expected steady-state value on
	// a freshly-emitted plan. Not a separate whole-run refusal gate: unlike
	// an unpinned prompt_version, an unrecorded contract digest does not
	// change what gets dispatched, only what a later drift line can say
	// about it.
	contractDigest, cderr := ResolveExtractorContractDigest(root)
	if cderr != nil {
		contractDigest = ExtractorContractUnrecorded
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
	// --artifact: plan that one artifact regardless of staleness (SR-011).
	// The stale work-list is replaced, not filtered: a fresh artifact is not
	// in it, and "the extractor missed a rule" is exactly the fresh case.
	// Artifacts (plural) is Artifact's own path, once per entry. Any refusal —
	// an unknown path, an excluded artifact, an absent sidecar without
	// --bootstrap — refuses the WHOLE plan rather than silently planning the
	// rest: a batch that quietly drops one subject is the shape D425 is about.
	if len(opts.Artifacts) > 0 {
		reqs := make([]workItem, 0, len(opts.Artifacts))
		for _, a := range opts.Artifacts {
			// A CORPUS-WIDE replan is the bootstrap case for a sidecar-less
			// artifact, not the loss case, so this passes Bootstrap rather than
			// refusing. The D414 refusal guards a TARGETED `--artifact`, where a
			// human named one artifact and an absent sidecar is as likely lost as
			// new. Over a whole corpus it is neither knowable nor rare — every
			// newly-added artifact has no sidecar — and refusing the batch would
			// block re-extraction on any corpus containing one new ADR.
			//
			// The protection is not dropped, it moves to where it can work:
			// buildItem marks each such item BootstrapNoPin, and restore names
			// them — "N of M item(s) had NO prior sidecar, so no pin was taken"
			// — which is D415's ruling that the corpus path REPORTS where it
			// cannot prevent.
			req, rerr := requestedItem(root, cfg.Dirs(), a, promptVersion, true)
			if rerr != nil {
				msg := fmt.Sprintf("refusing to emit a plan: %s: %v", a, rerr)
				if jerr := appendJournal(root, JournalRecord{Event: JournalEventRefuse, PlanID: PlanIDUnresolved, Items: 0, Cause: CauseCorpusLoadError}); jerr != nil {
					msg = msg + fmt.Sprintf(" (additionally, the refusal itself could not be journaled: %v)", jerr)
				}
				return &PlanOutcome{ExitCode: ExitRefused, Message: msg}, errors.New(msg)
			}
			reqs = append(reqs, req)
		}
		items = reqs
		// loadErrs and validationErrs are NOT cleared here, unlike the single
		// --artifact path. A corpus-wide replan is still a corpus read: an
		// unparseable sidecar anywhere is a whole-plan refusal, and discarding
		// those was "a corpus with an unparseable sidecar was planned anyway",
		// caught by the test that exists for exactly that swallow.
	} else if opts.Artifact != "" {
		req, rerr := requestedItem(root, cfg.Dirs(), opts.Artifact, promptVersion, opts.Bootstrap)
		if rerr != nil {
			msg := fmt.Sprintf("refusing to emit a plan: --artifact %s: %v", opts.Artifact, rerr)
			cause := CauseCorpusLoadError
			if errors.Is(rerr, errSidecarAbsentOnRequest) {
				cause = CauseSidecarAbsentOnRequest
			}
			if jerr := appendJournal(root, JournalRecord{Event: JournalEventRefuse, PlanID: PlanIDUnresolved, Items: 0, Cause: cause}); jerr != nil {
				msg = msg + fmt.Sprintf(" (additionally, the refusal itself could not be journaled: %v)", jerr)
			}
			return &PlanOutcome{ExitCode: ExitRefused, Message: msg}, errors.New(msg)
		}
		items = []workItem{req}
		loadErrs, validationErrs = nil, nil
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
		planID := derivePlanID(corpusHash(items), configHash(cfg, promptVersion, contractDigest))
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
		items[i].ContractDigest = contractDigest
	}

	kHash := configHash(cfg, promptVersion, contractDigest)

	// A TERMINAL id is never written again (#11, c1-a/c1-c). A requested
	// extraction — the caller named the artifacts — derives a successor; the
	// corpus path refuses. `requested` comes from the options the caller
	// passed, never from an item's reason.
	requested := opts.Artifact != "" || len(opts.Artifacts) > 0
	succ, serr := resolveTerminalCollision(root, items, kHash, requested)
	cHash, planID := succ.CorpusHash, succ.PlanID
	if serr != nil {
		return refusePlan(root, planID, len(items), CausePlanIDTerminal, serr)
	}

	// The latest unclosed plan per item blocks a re-plan once the sidecar has
	// moved under it (#11, c1-d/c1-f/c1-g). Before pin capture: a refused plan
	// must not have touched a pin.
	if gerr := guardUnclosedPlans(root, planID, succ.Supersedes, items); gerr != nil {
		return refusePlan(root, planID, len(items), CauseUnclosedPlan, gerr)
	}

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
			MaxConcurrency: cfg.MaxConcurrency,
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
		Message:  planMessage(planID, len(items), promptNote) + supersedesMessage(succ),
	}, nil
}

// refusePlan journals one `refuse` line under cause and returns the
// whole-plan refusal. Nothing else is written.
func refusePlan(root, planID string, items int, cause string, why error) (*PlanOutcome, error) {
	msg := "refusing to emit a plan: " + why.Error()
	if jerr := appendJournal(root, JournalRecord{
		Event:  JournalEventRefuse,
		PlanID: planID,
		Items:  items,
		Cause:  cause,
	}); jerr != nil {
		msg = msg + fmt.Sprintf(" (additionally, the refusal itself could not be journaled: %v)", jerr)
	}
	return &PlanOutcome{ExitCode: ExitRefused, Message: msg}, errors.New(msg)
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
// captured, keyed by plan_id. Keying by plan_id does NOT by itself keep two
// plans apart: until 0.9.0 a re-request of an unchanged artifact re-derived a
// consumed plan's id and wrote straight over its pins (#11, c1-a). What keeps
// them apart is the id — corpus_hash covers these very bytes and a terminal id
// is succeeded, never reused (plan_hash.go, plan_succession.go) — and
// capturePinSnapshot's write-once rule, which refuses rather than overwrite.
// Not part of any landed schema — this is plan.go's own private on-disk
// format, consumed by restore.go the same way reextract/snapshot.go's
// writeSnapshot is consumed by reextract's own review step.
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
//
// WRITE-ONCE. A pin is the only copy of what a human pinned before the
// extractor ran. When one already exists for this plan and item, identical
// bytes are a no-op (an idempotent re-plan of an open plan) and different
// bytes are an error — the whole plan is refused under pin-capture-failed
// rather than the pin being replaced with whatever the sidecar holds now.
func capturePinSnapshot(root, planID string, it workItem) error {
	if it.beforeSidecar == nil {
		return nil
	}
	dst := pinSnapshotPath(root, planID, it.ItemID)
	if existing, err := os.ReadFile(dst); err == nil { //nolint:gosec // state path built from a validated id
		if bytes.Equal(existing, it.beforeSidecar) {
			return nil
		}
		return fmt.Errorf("a pin for %s already exists under plan %s at %s with different bytes — pins are "+
			"write-once, and replacing it would discard what that plan must restore", it.ItemID, planID, relPath(root, dst))
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("an existing pin at %s could not be read: %w", relPath(root, dst), err)
	}
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

// errSidecarAbsentOnRequest marks the D414 refusal so the journal can record
// its own cause rather than filing it under a corpus load error.
var errSidecarAbsentOnRequest = errors.New("sidecar absent on a requested extraction")

// errAbsentSidecarOnRequest is the message a human reads when a `requested`
// extraction finds no sidecar. It states what is at risk and BOTH ways forward,
// because a refusal that does not tell you how to proceed is an obstacle rather
// than a control.
func errAbsentSidecarOnRequest(artifact string) error {
	return fmt.Errorf("%w: %s has no sidecar on disk.\n"+
		"  A requested re-extraction rebuilds the sidecar from the parent document — and a sidecar's\n"+
		"  manual_directives and suppressed_directives are prose a human wrote INTO THE SIDECAR. They\n"+
		"  appear nowhere in the parent, so the extractor cannot regenerate them. They are normally\n"+
		"  carried forward from the pre-dispatch pin; with the sidecar already gone there is nothing to\n"+
		"  pin, and they would be destroyed with every downstream report still reading clean.\n"+
		"  Two ways forward, and only you can tell which is true:\n"+
		"    - the sidecar was LOST (deleted, a bad merge, an ignore rule): restore it first, then re-run.\n"+
		"      `git checkout -- <sidecar>` if it is tracked.\n"+
		"    - it NEVER existed and this is a first extraction: re-run with --bootstrap.",
		errSidecarAbsentOnRequest, artifact)
}

// requestedItem builds the one-item work-list for --artifact: the pair whose
// artifact path matches, with reason `requested`, its current sidecar bytes
// (if any) captured as the pre-dispatch baseline so restore/verify treat a
// re-extraction exactly like a stale one.
func requestedItem(root string, dirs []string, artifact, promptVersion string, bootstrap bool) (workItem, error) {
	pairs, err := sidecar.Discover(root, dirs)
	if err != nil {
		return workItem{}, err
	}
	want := artifact
	if !filepath.IsAbs(want) {
		want = filepath.Join(root, filepath.FromSlash(artifact))
	}
	want = filepath.Clean(want)
	for _, p := range pairs {
		if filepath.Clean(p.ParentPath) != want {
			continue
		}
		var before []byte
		if p.SidecarPath != "" {
			if b, rerr := os.ReadFile(p.SidecarPath); rerr == nil {
				before = b
			}
		}
		// A declared bootstrap of a PROPOSED ADR with no sidecar is admitted
		// despite the status exclusion: /edikt:adr:new compiles the ADR it
		// just wrote, and only a clean verify lets it flip to accepted (#14).
		admitProposed := bootstrap && before == nil && sidecar.IsProposedADR(p.Kind, p.ParentPath)
		if excluded, why := statusExcluded(p); excluded && !admitProposed {
			return workItem{}, fmt.Errorf("%s is excluded from the compiled corpus (%s); a requested extraction of an excluded artifact would produce a sidecar nothing reads", artifact, why)
		}
		if before == nil && !bootstrap {
			return workItem{}, errAbsentSidecarOnRequest(artifact)
		}
		return buildItem(root, p, ReasonRequested, promptVersion, before)
	}
	return workItem{}, fmt.Errorf("no governed artifact at that path under %v", dirs)
}

// planMessage is the success line, plus the extractor-resolution note when
// there was more than one definition to choose from (D40): which one the
// plan's prompt_version came from is part of what the plan asserts.
func planMessage(planID string, items int, promptNote string) string {
	msg := fmt.Sprintf("plan %s: %d item(s)", planID, items)
	if promptNote != "" {
		msg += "\nextractor: " + promptNote
	}
	return msg
}
