package phasea

// plan_worklist.go — the stale-sidecar work-list computation. This is
// deliberately NOT a second detector: it drives the SAME two primitives
// twophase.go's dispatch loop (govrun/twophase.go:283-341) already drives —
// sidecar.Discover's Pair.Sidecar == nil bootstrap branch, and
// (*sidecar.Sidecar).IsStale (sidecar/drift.go:258-301) — through the
// identical package (internal/sidecar), not a reimplementation of either.
//
// Two small wrappers ARE duplicated from govrun/twophase.go, by necessity
// rather than choice: govrun already imports phasea (to build phasea.Task
// for its own dispatch loop), so phasea importing govrun back would be an
// import cycle. Both duplicates delegate to the actual shared decision
// procedure rather than re-deciding anything themselves:
//
//   - statusExcluded (below) is sidecar.StatusExcluded, the one status
//     filter — the same call govrun.statusExcluded makes.
//   - artifactTypeForPair (below) reads sidecar.Pair.Kind, which
//     sidecar.Discover already computed positionally from which configured
//     directory the artifact was found in — govrun.artifactTypeFromPath
//     re-derives the same fact from the path string; this reads the field
//     Discover already set instead of re-deriving it.
//
// A change to the real filter (parse.IsIncluded) or the real classification
// (sidecar.Pair.Kind) is visible to both packages' tests identically,
// because neither package owns a second copy of the decision — only of the
// wrapper around it.

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/diktahq/edikt/tools/edikt/internal/idvalidate"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// workItem is one candidate for the plan, plus the bytes plan.go needs
// in-process but MUST NOT serialize into the schema-governed plan file:
// the pre-dispatch sidecar (for D6's pin-set capture) and the absolute
// artifact path (for reading its bytes / computing its sha256). PlanItem
// carries only the project-root-relative, schema-shaped view.
type workItem struct {
	PlanItem
	absArtifactPath string
	beforeSidecar   []byte // raw pre-dispatch sidecar bytes; nil for "missing"
}

// statusExcluded is sidecar.StatusExcluded applied to a discovered pair —
// the same call govrun.statusExcluded makes. The mirror this used to be is
// gone: the wrapper now lives once, in internal/sidecar (D46 (a)).
func statusExcluded(p sidecar.Pair) (bool, string) {
	return sidecar.StatusExcluded(p.Kind, p.ParentPath)
}

// artifactTypeForPair maps sidecar.Pair.Kind (already computed positionally
// by sidecar.Discover) to the artifact_type vocabulary idvalidate.ArtifactType
// and the plan schema's item_id/agent contract expect. Kind values outside
// the three known ones pass through unchanged so idvalidate rejects them by
// name rather than this function silently coercing them to something valid.
func artifactTypeForPair(p sidecar.Pair) string {
	switch p.Kind {
	case sidecar.KindADR:
		return "adr"
	case sidecar.KindInvariant:
		return "invariant"
	case sidecar.KindGuideline:
		return "guideline"
	default:
		return p.Kind
	}
}

// corpusLoadError is one whole-plan refusal input: either a corrupt/unreadable
// artifact in the governed corpus, or an artifact that failed the idvalidate
// dispatch-boundary check in buildItem. It mirrors govrun's own posture —
// twophase.go accumulates every such error and refuses the ENTIRE run rather
// than silently proceeding with a partial work-list (RunTwoPhase's `if
// len(loadErrs) > 0` check, govrun/twophase.go:391-397) — because a plan that
// silently dropped one bad artifact would be reporting a corpus state nobody
// asked it to report.
//
// computeWorkList returns the two failure classes in SEPARATE slices
// (loadErrs, validationErrs) even though both use this same struct shape,
// because plan.go must journal them under two different phasea-journal.v1
// `cause` values — CauseCorpusLoadError vs CauseItemValidationFailed — and a
// single merged slice would lose which class a given entry belonged to.
type corpusLoadError struct {
	Path string
	Err  error
}

func (e corpusLoadError) String() string {
	return fmt.Sprintf("%s: %v", e.Path, e.Err)
}

// buildItem validates one pair's artifact type/id at the dispatch boundary
// — BEFORE either reaches a plan item, and therefore before either can
// reach a subagent's task prompt — and, on success, builds its PlanItem.
//
// This is the validation boundary point 7 of the phase brief names: once
// Phase 4 deletes ClaudeRunner (whose Resync carried its own idvalidate
// defense-in-depth re-check) and its runner_security_test.go, plan.go
// becomes the SOLE place an artifact id/type is checked before reaching a
// subagent prompt. artifactID/artifactType are validated even though
// sidecar.Discover's own artifactIDFromName is well-behaved for ADR-/INV-
// prefixed files — a guideline's item_id is its bare filename stem, which
// can be anything a filesystem allows, including shell metacharacters,
// instruction-injection text, or Unicode lookalikes that survive on disk
// but must never survive into a plan item.
//
// root is the project root, used to resolve p.ParentPath's sha256 and to
// compute the schema's project-root-relative artifact_path/expected_output.path.
func buildItem(root string, p sidecar.Pair, reason, promptVersion string, beforeSidecar []byte) (workItem, error) {
	artifactType := artifactTypeForPair(p)
	if err := idvalidate.ArtifactType(artifactType); err != nil {
		return workItem{}, fmt.Errorf("%s: %w — refusing dispatch", p.ParentPath, err)
	}
	if err := idvalidate.ArtifactID(p.ArtifactID); err != nil {
		return workItem{}, fmt.Errorf("%s: %w — refusing dispatch", p.ParentPath, err)
	}

	sum, err := sha256Hex(p.ParentPath)
	if err != nil {
		return workItem{}, fmt.Errorf("sha256 %s: %w", p.ParentPath, err)
	}

	item := PlanItem{
		ItemID:         p.ArtifactID,
		ArtifactPath:   relPath(root, p.ParentPath),
		ArtifactSHA256: sum,
		Reason:         reason,
		Agent:          AgentSidecarExtractor,
		PromptVersion:  promptVersion,
		ExpectedOutput: ExpectedOutput{
			Path:               relPath(root, p.SidecarPath),
			Schema:             SchemaGovSidecarV2,
			VerifyParentSHA256: true,
		},
		// No prior sidecar means no pin will be taken for this item, so
		// nothing it carried can be carried forward. Set HERE because
		// buildItem is the one place both the corpus path and the
		// --artifact path converge: setting it on either caller alone is
		// how the first version of this marker came to describe only half
		// the runs that needed it.
		BootstrapNoPin: beforeSidecar == nil,
	}
	return workItem{PlanItem: item, absArtifactPath: p.ParentPath, beforeSidecar: beforeSidecar}, nil
}

// computeWorkList reproduces twophase.go's dispatch-loop decision, pair by
// pair, over the SAME sidecar.Discover output twophase.go itself consumes:
//
//   - p.Skip                       → excluded, no sidecar required (govrun/twophase.go:261)
//   - statusExcluded(p)            → excluded, status disqualifies it (govrun/twophase.go:275)
//   - p.LoadErr != nil             → whole-plan refusal (corpus load error)
//   - p.Sidecar == nil             → reason "missing" (govrun/twophase.go:288-338)
//   - p.Sidecar.IsStale(root)      → reason "stale-quote-drift" or
//     "stale-migration-preserved" (sidecar/drift.go:258-301)
//   - otherwise                    → not stale, excluded
//
// Returns items sorted ascending by item_id byte order — the same order
// corpus_hash's input uses (SAC-002) — plus TWO separate error slices, both
// of which the caller MUST treat as a whole-plan refusal rather than a
// partial work-list (see corpusLoadError's doc): loadErrs for a corrupt or
// unreadable artifact (p.LoadErr, a stale-check I/O error, or a sidecar read
// failure), and validationErrs for an artifact that failed buildItem's
// idvalidate dispatch-boundary check. Kept separate, not merged, because
// plan.go journals the two under different phasea-journal.v1 `cause` values.
//
// Deliberately NOT included here: twophase.go's AutoRepairAnchors pass
// (govrun/twophase.go:345-365). That pass is a pure-Go REPAIR performed
// between "detected stale" and "would dispatch" — the phase brief's own
// Context Needed range (twophase.go:283-341) stops short of it, and the
// plan schema's reason enum has no category for "was stale, autorepair
// silently fixed it, so nothing is planned." Folding it in here would make
// plan.go take a real WRITE action (rewriting anchors) as an implicit side
// effect of what SPEC-013 calls "pure filesystem/hash computation, same as
// every other tier-2 mechanical pass" — Phase 4's rewire is the place to
// decide whether autorepair still runs before planning, not this phase.
func computeWorkList(root string, dirs []string) ([]workItem, []corpusLoadError, []corpusLoadError, error) {
	pairs, err := sidecar.Discover(root, dirs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("discover sidecars: %w", err)
	}

	var items []workItem
	var loadErrs []corpusLoadError
	var validationErrs []corpusLoadError

	for _, p := range pairs {
		if p.Skip {
			continue
		}
		if excluded, _ := statusExcluded(p); excluded {
			continue
		}
		if p.LoadErr != nil {
			loadErrs = append(loadErrs, corpusLoadError{Path: p.SidecarPath, Err: p.LoadErr})
			continue
		}

		if p.Sidecar == nil {
			items = append(items, mustBuildOrError(root, p, ReasonMissing, nil, &validationErrs))
			continue
		}

		stale, _, ierr := p.Sidecar.IsStale(root)
		if ierr != nil {
			loadErrs = append(loadErrs, corpusLoadError{Path: p.SidecarPath, Err: ierr})
			continue
		}
		if !stale {
			continue
		}
		reason := ReasonStaleQuoteDrift
		if p.Sidecar.MigrationPreserved != nil {
			reason = ReasonStaleMigrationPreserved
		}
		before, rerr := readFileOrNil(p.SidecarPath)
		if rerr != nil {
			loadErrs = append(loadErrs, corpusLoadError{Path: p.SidecarPath, Err: rerr})
			continue
		}
		items = append(items, mustBuildOrError(root, p, reason, before, &validationErrs))
	}

	// items built via mustBuildOrError that failed validation are omitted
	// (nil PlanItem.ItemID); strip them now that every pair has been walked,
	// so the caller sees only real items plus the accumulated errors.
	filtered := items[:0]
	for _, it := range items {
		if it.ItemID != "" {
			filtered = append(filtered, it)
		}
	}
	items = filtered

	sort.Slice(items, func(i, j int) bool { return items[i].ItemID < items[j].ItemID })

	return items, loadErrs, validationErrs, nil
}

// mustBuildOrError calls buildItem and, on failure, folds the error into
// validationErrs (so a single bad artifact refuses the whole plan, mirroring
// twophase.go's own loadErrs aggregation) rather than aborting the walk —
// every other pair still gets evaluated so the refusal message reports the
// FULL set of problems in one pass, not one-at-a-time. Named validationErrs
// (not loadErrs) at the call site because buildItem's only failure mode is
// idvalidate rejection, not a corpus read/parse error — plan.go journals the
// two under different phasea-journal.v1 `cause` values.
func mustBuildOrError(root string, p sidecar.Pair, reason string, before []byte, validationErrs *[]corpusLoadError) workItem {
	item, err := buildItem(root, p, reason, "", before)
	if err != nil {
		*validationErrs = append(*validationErrs, corpusLoadError{Path: p.ParentPath, Err: err})
		return workItem{}
	}
	return item
}

// relPath renders path relative to root for the schema's project-root-relative
// fields, falling back to path unchanged when root is empty or the relation
// cannot be computed (e.g. different volumes on Windows).
func relPath(root, path string) string {
	if root == "" {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
