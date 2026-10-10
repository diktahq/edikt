package govrun

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/contradiction"
	"github.com/diktahq/edikt/tools/edikt/internal/idvalidate"
	"github.com/diktahq/edikt/tools/edikt/internal/migrate"
	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
	"github.com/diktahq/edikt/tools/edikt/internal/phaseb"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/internal/topicregistry"
	"github.com/diktahq/edikt/tools/edikt/model"
	"golang.org/x/term"
)

// writeSourceSidecar is the ONE way this package writes a source artifact.
//
// The rule: every write of a source sidecar goes through this function, and
// nothing below govrun writes one. phasea's autorepair returns bytes and never
// persists (it used to, and that write was reachable from `--check`: #9).
// Two properties follow from the rule being structural rather than incidental:
//
//  1. NO PATH REACHABLE FROM A DOCUMENTED NON-WRITING FLAG WRITES. `--check`,
//     `--check --json` and `--dry-run` all set CheckOnly, and the refusal is
//     on the write itself, so a caller reordered above the check branch's
//     return still cannot write under a flag whose contract is that it does
//     not.
//  2. The `# yaml-language-server: $schema=` header the file opened with is
//     re-attached: Marshal emits canonical bytes with no comments, so a
//     strip-and-rewrite silently drops the header the templates ship and the
//     validator demands.
func writeSourceSidecar(opts TwoPhaseOptions, path string, data []byte) error {
	if opts.CheckOnly {
		return fmt.Errorf("refusing to write %s: --check / --dry-run must not modify a source artifact", path)
	}
	var header string
	if prev, rerr := os.ReadFile(path); rerr == nil { //nolint:gosec // path from the discovered corpus
		header = sidecar.SchemaHeaderOf(prev)
	}
	return os.WriteFile(path, sidecar.WithSchemaHeader(header, data), 0o644) //nolint:gosec // sidecars are world-readable by design
}

// checkOnlyAutorepair classifies one stale pair under --check / --dry-run by
// running the autorepair on an independent copy loaded from disk.
//
// handled=true: the copy is fully resolved. The pair is recorded once in
// StaleSidecars and once in RepairableSidecars, a `stale:` line is printed,
// and the caller must skip its task path. handled=false: the pair still needs
// extraction (or the repair could not run); residual, when non-empty, is the
// staleness reason re-measured on the repaired copy, which the caller prints
// instead of the pre-repair one. Nothing is written either way.
func checkOnlyAutorepair(opts TwoPhaseOptions, p sidecar.Pair, res *TwoPhaseResult) (handled bool, residual string) {
	cp, lerr := sidecar.Load(p.SidecarPath)
	if lerr != nil {
		fmt.Fprintf(opts.Stderr, "  autorepair: %s — could not load a copy to measure (%v); counted as needing extraction\n", p.ArtifactID, lerr)
		return false, ""
	}
	out, rerr := phasea.TryAutoRepair(cp, opts.ProjectRoot, p.ParentPath)
	if rerr != nil {
		fmt.Fprintf(opts.Stderr, "  autorepair: %s — %v; counted as needing extraction\n", p.ArtifactID, rerr)
		return false, ""
	}
	if out.AnchorsRepaired == 0 {
		return false, ""
	}
	if !out.StillStale {
		res.StaleSidecars = append(res.StaleSidecars, p.ArtifactID)
		res.RepairableSidecars = append(res.RepairableSidecars, p.ArtifactID)
		fmt.Fprintf(opts.Stderr,
			"  stale: %s — anchor drift, %d anchor(s) mechanically repairable; not written (--check) — run edikt gov compile\n",
			p.ArtifactID, out.AnchorsRepaired)
		return true, ""
	}
	fmt.Fprintf(opts.Stderr, "  autorepair: %s — %d anchor(s) repairable, residual drift needs extraction; not written (--check)\n",
		p.ArtifactID, out.AnchorsRepaired)
	return false, residualReason(cp, opts.ProjectRoot, "")
}

// residualReason re-measures a partially repaired sidecar and returns the
// first anchor that is STILL stale. Reporting the pre-repair reason would
// name an anchor the repair already fixed and send the reader to prose that
// is fine. fallback is returned when the re-measure finds nothing it can name
// (the residual sits outside the directive anchors the reason covers).
func residualReason(sc *sidecar.Sidecar, projectRoot, fallback string) string {
	if stale, reason, err := sc.IsStale(projectRoot); err == nil && stale && reason != "" {
		return reason
	}
	if fallback == "" {
		return "residual anchor drift after autorepair"
	}
	return fallback
}

// TwoPhaseOptions configures one invocation of the sidecar-aware compile.
type TwoPhaseOptions struct {
	ProjectRoot string
	CheckOnly   bool
	NoWait      bool
	JSONMode    bool // suppress prose UI on stdout; caller emits JSON
	Concurrency int  // defaults to DefaultDispatchConcurrency
	// DispatchTimeout bounds ONE artifact's extraction. Defaults to
	// DefaultDispatchTimeout; a negative value disables the bound and is
	// only meaningful to tests that drive a runner they control.
	DispatchTimeout time.Duration
	Stderr          io.Writer
	Stdout          io.Writer

	// OnLoss governs what happens when the post-extractor lossless
	// check finds directives in MigrationPreserved that the extractor
	// didn't carry into the canonical output. Values: "abort" (exit
	// non-zero), "accept" (warn + continue), "auto" (abort in non-TTY,
	// accept otherwise; the default). Empty string is treated as "auto".
	OnLoss string
}

// TwoPhaseResult summarizes the run for both prose UI and --json output.
type TwoPhaseResult struct {
	StaleSidecars []string `json:"stale_sidecars,omitempty"`

	// RepairableSidecars is the subset of StaleSidecars whose drift the
	// deterministic autorepair resolves completely. Populated only under
	// --check / --dry-run, where the repair is computed on a copy and never
	// written: these are the ids `edikt gov compile` would fix without an
	// extraction. A partially repairable sidecar is NOT listed — it still
	// needs the extractor.
	RepairableSidecars []string `json:"repairable_sidecars,omitempty"`

	// BootstrapSidecars is the subset of StaleSidecars that had no sidecar
	// on disk at all (as opposed to a sidecar whose anchors drifted). Kept
	// separate so callers can distinguish "first compile of a project that
	// predates edikt" from "routine resync" — the two have very different
	// cost profiles and the JSON consumer should be able to tell them apart.
	BootstrapSidecars []string `json:"bootstrap_sidecars,omitempty"`

	// PhaseAPlanID names the dispatch plan emitted when Phase A found work.
	// Empty when Phase A had nothing to do. It is how a caller — and the
	// tier-1 skill — names the plan to fan out and then verify.
	PhaseAPlanID string `json:"phase_a_plan_id,omitempty"`

	// BodyDrift is the ADR-053 second signal: how many artifact bodies
	// changed since their sidecar was extracted. Separate from
	// StaleSidecars (anchor drift) because a WRONG sidecar and an
	// INCOMPLETE one are different problems with different urgencies.
	BodyDrift *sidecar.BodyDriftSummary `json:"body_drift,omitempty"`

	// ExtractorModel is the model id Phase A pinned for this run. Recorded
	// so a measurement taken from this compile is attributable to a model
	// — without it, comparing two extraction baselines cannot separate a
	// prompt change from a model that moved underneath both. Empty when
	// Phase A did not run (nothing was stale), which is a real distinction:
	// no dispatch happened, so no model produced anything.
	ExtractorModel string `json:"extractor_model,omitempty"`

	// RegistryCoverage is the measured state of .edikt/topics.yaml against
	// the topics this corpus actually uses: approved, pending, orphaned, and
	// edited-since-approval, each with the corpus-topic denominator.
	//
	// It is a REPORT, not a gate. During the migration window an upgrading
	// project has pending entries by construction, and refusing to compile
	// until every one is approved would brick the upgrade it is supposed to
	// enable. What is forbidden is inventing a description or staying quiet
	// about a missing one — hence a count that always travels with a
	// denominator. nil means the registry was not measured at all.
	RegistryCoverage *topicregistry.Coverage `json:"registry_coverage,omitempty"`

	// ProposalRouting records what the pre-Phase-B routing step queued for
	// human approval and which topics it opened empty slots for.
	ProposalRouting *ProposalRoutingReport `json:"proposal_routing,omitempty"`

	PhaseADone     bool             `json:"phase_a_done"`
	PhaseAFailures int              `json:"phase_a_failures"`
	PhaseAErrors   []PhaseAErrorRec `json:"phase_a_errors,omitempty"`
	PhaseB         *phaseb.Result   `json:"phase_b,omitempty"`

	// Conflicts is SPEC-010 phase 9's contradiction report (AC-9.1): every  edikt-guard:allow
	// same-topic, same-noun-phrase, opposing-modality directive pair found
	// across the live (already-supersession-filtered) corpus. Report only —
	// see internal/contradiction's package doc for the precedence rule
	// (AC-9.2): supersession is the one rule this corpus has, already
	// applied upstream, so every entry here has no further rule and MUST be
	// surfaced to a human, never silently resolved.
	Conflicts []contradiction.Conflict `json:"conflicts,omitempty"`

	// Restatements is AC-4.2's advisory duplicate report: the same rule
	// asserted by two different artifacts. It travels beside Conflicts
	// because it is the same posture — reported, never auto-resolved — but
	// it is a distinct relation: Conflicts finds artifacts that DISAGREE,
	// this finds artifacts that AGREE without either owning the rule.
	Restatements []contradiction.Restatement `json:"restatements,omitempty"`

	// LosslessReport summarizes any post-extractor directives that
	// went missing relative to MigrationPreserved.Directives. Per
	// the OnLoss policy, a non-empty report may have failed the run.
	LosslessReport []LossArtifactRec `json:"lossless_report,omitempty"`

	// LosslessUnmeasured names artifacts the lossless check could NOT
	// evaluate, and exists because the check's denominator can be zero.
	// CheckLosslessAgainstDirectives compares the extractor's output against
	// MigrationPreserved.Directives; when that list is empty it compares
	// against nothing, finds nothing missing, and the artifact passes a gate
	// that never measured it. INV-013: absence is never a passing measurement.
	//
	// Same nil/empty contract as RenderDrift below. Empty and non-nil means
	// the Phase B pass RAN and every artifact carrying a preservation block
	// was measurable; nil means the pass did not run. Entries name artifacts
	// whose extraction is unverified, not artifacts known to have lost data.
	LosslessUnmeasured []UnmeasuredRec `json:"lossless_unmeasured,omitempty"`

	// RenderDrift is AC-2.9's report: rendered surfaces that do not match a
	// fresh render of the current sidecars. Empty and non-nil means the check
	// RAN and found nothing; nil means it did not run — a distinction a
	// consumer must be able to make (INV-013).
	RenderDrift []string `json:"render_drift,omitempty"`
}

// UnmeasuredRec names an artifact whose lossless comparison had no baseline
// to compare against. Reason distinguishes the two ways that happens so a
// reader can tell an unparseable sentinel apart from a sidecar written by a
// migrator that destroyed its own baseline.
type UnmeasuredRec struct {
	SidecarPath string `json:"sidecar_path"`
	Reason      string `json:"reason"`
}

// losslessMeasurable reports whether a preservation block gives the lossless
// check a baseline it can actually compare the extractor's output against.
//
// Only Directives qualify. CheckLosslessAgainstDirectives matches on a
// (modality, ref_id, normalized noun-phrase) tuple per legacy directive
// string; RawBody is unparsed prose by definition and yields no such tuple,
// and SchemaDetected is audit metadata rather than content.
func losslessMeasurable(mp *sidecar.MigrationPreserved) bool {
	return mp != nil && len(mp.Directives) > 0
}

// losslessUnmeasurableReason names WHY an artifact could not be measured, so
// the report distinguishes an unparseable sentinel that was preserved from a
// baseline that was destroyed before the gate ever saw it. Callers must only
// use it when losslessMeasurable is false.
func losslessUnmeasurableReason(mp *sidecar.MigrationPreserved) string {
	if mp.HasContent() {
		return "migration_preserved carries only raw_body (unparsed sentinel); extractor output cannot be compared per-directive"
	}
	return "migration_preserved is empty (no directives, no raw_body); the baseline was lost before this gate could measure it"
}

// LossArtifactRec is a JSON-friendly per-artifact loss record.
type LossArtifactRec struct {
	SidecarPath string         `json:"sidecar_path"`
	Losses      []migrate.Loss `json:"losses"`
}

// resolveLossPolicy maps a string OnLoss flag value into a concrete
// "abort" or "accept" decision. "auto" (or empty) inspects stdin:
// TTY → accept (user can read the warning and decide), non-TTY → abort
// (CI / scripts must opt-in to accepting silent loss). Unknown values
// fall back to the safer "abort" path.
func resolveLossPolicy(flag string, stdin *os.File) string {
	switch strings.ToLower(flag) {
	case "abort":
		return "abort"
	case "accept":
		return "accept"
	case "auto", "":
		if stdin != nil && term.IsTerminal(int(stdin.Fd())) {
			return "accept"
		}
		return "abort"
	default:
		return "abort"
	}
}

// PhaseAErrorRec is a JSON-friendly view of a Phase A subagent failure.
type PhaseAErrorRec struct {
	ArtifactID   string `json:"artifact_id"`
	ArtifactType string `json:"artifact_type"`
	Err          string `json:"error"`
}

// statusExcluded is sidecar.StatusExcluded applied to a discovered pair —
// the one status filter (ADR-020:d03, decision procedure parse.IsIncluded).
// The history of why it is one function and not a second copy is on
// sidecar.StatusExcluded; the F-069 finding (a `status: proposed` ADR
// compiled 165 MUST-grade directives because only the weaker denylist ran)
// is what it exists to prevent.
func statusExcluded(p sidecar.Pair) (bool, string) {
	return sidecar.StatusExcluded(p.Kind, p.ParentPath)
}

// RunTwoPhase is the sidecar-aware compile path.
//
// Order of operations:
// 1. acquire .edikt/state/compile.lock (blocks unless --no-wait)
// 2. discover artifact pairs and detect stale sidecars
// 3. if --check && stale → emit actionable error, exit 1, no LLM dispatch
// 4. if stale && !--check → run Phase A (parallel resync, continue-on-error)
// 5. if Phase A had failures → exit 1, do NOT run Phase B
// 6. run Phase B (deterministic merge)
func RunTwoPhase(opts TwoPhaseOptions, clk model.Clock) (*TwoPhaseResult, error) {
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = DefaultDispatchConcurrency
	}
	if opts.DispatchTimeout <= 0 {
		opts.DispatchTimeout = DefaultDispatchTimeout
	}

	cfg, err := loadConfig(opts.ProjectRoot)
	if err != nil {
		return nil, err
	}
	dirs := []string{cfg.Paths.Decisions, cfg.Paths.Invariants, cfg.Paths.Guidelines}

	release, err := acquireCompileLock(opts.ProjectRoot, opts.NoWait, opts.Stderr)
	if err != nil {
		return nil, err
	}
	defer release()

	pairs, err := sidecar.Discover(opts.ProjectRoot, dirs)
	if err != nil {
		return nil, fmt.Errorf("discover sidecars: %w", err)
	}

	res := &TwoPhaseResult{}
	var tasks []phasea.Task
	var loadErrs []string

	for _, p := range pairs {
		if p.Skip {
			// Superseded ADRs and migration:skip-marked artifacts opt out of
			// sidecar coverage. : no body edit required to
			// suppress them — the supersession status line was present at
			// acceptance time. A leftover sidecar next to a retired artifact
			// (pre-retirement compile output, or a hand-written empty
			// placeholder) is tolerated and ignored — announced so the user
			// knows the file is inert and can delete it.
			if _, serr := os.Stat(p.SidecarPath); serr == nil {
				fmt.Fprintf(opts.Stderr, "  skip: %s — %s (existing sidecar ignored; safe to delete)\n",
					p.ArtifactID, p.SkipReason)
			}
			continue
		}
		if excluded, _ := statusExcluded(p); excluded {
			// Not dispatched: an artifact whose status keeps it out of the
			// merge has nothing to gain from a resync, and dispatching it
			// spends an LLM call on directives that will be discarded a few
			// hundred lines below. Silent here on purpose — the filter at
			// the Phase B site announces it once, and announcing at both
			// sites would report one exclusion twice.
			continue
		}
		if p.LoadErr != nil {
			loadErrs = append(loadErrs, fmt.Sprintf("  %s: %v", p.SidecarPath, p.LoadErr))
			continue
		}
		if p.Sidecar == nil {
			// Missing sidecar — bootstrap it. Extraction-from-prose is the
			// same operation Phase A already performs for a stale sidecar
			// (the per-artifact `:compile` command writes the file whether
			// or not one existed), so a missing sidecar is dispatched
			// through the identical path rather than reported as a fatal
			// load error.
			//
			// Reporting it as an error is what made a never-edikt-managed
			// project uncompilable: every artifact failed the same way and
			// the only documented remedy (`migrate sidecars`) had nothing
			// to migrate.
			//
			// Two cases still refuse rather than bootstrap:
			//
			//   --check         must never dispatch a subagent (ADR-028).  // edikt-guard:allow
			//   EDIKT_HEADLESS  the documented opt-out — commands/gov/compile.md
			//                   §12 already specifies "disable the auto-chain,
			//                   print the explicit run-these list, exit
			//                   non-zero" for headless runs. Honouring it here
			//                   is what keeps the binary and the slash command
			//                   agreeing, and it keeps CI and the golden test
			//                   from spawning LLM subprocesses.
			//
			// Both report the missing sidecar as a load error, exactly as
			// every missing sidecar did before bootstrap existed.
			if opts.CheckOnly || os.Getenv("EDIKT_HEADLESS") == "1" {
				loadErrs = append(loadErrs, fmt.Sprintf("  %s: sidecar missing — run /edikt:%s:compile", p.ParentPath, artifactTypeFromPath(p.ParentPath, dirs)))
				continue
			}
			artifactType := artifactTypeFromPath(p.ParentPath, dirs)
			if err := idvalidate.ArtifactType(artifactType); err != nil {
				loadErrs = append(loadErrs, fmt.Sprintf("  %s: %v — refusing dispatch", p.ParentPath, err))
				continue
			}
			if err := idvalidate.ArtifactID(p.ArtifactID); err != nil {
				loadErrs = append(loadErrs, fmt.Sprintf("  %s: %v — refusing dispatch", p.ParentPath, err))
				continue
			}
			res.BootstrapSidecars = append(res.BootstrapSidecars, p.ArtifactID)
			res.StaleSidecars = append(res.StaleSidecars, p.ArtifactID)
			tasks = append(tasks, phasea.Task{
				ArtifactType: artifactType,
				ArtifactID:   p.ArtifactID,
				ParentPath:   p.ParentPath,
				SidecarPath:  p.SidecarPath,
				ProjectRoot:  opts.ProjectRoot, // F-029
			})
			fmt.Fprintf(opts.Stderr, "  bootstrap: %s — no sidecar, extracting from prose\n", p.ArtifactID)
			continue
		}
		stale, reason, err := p.Sidecar.IsStale(opts.ProjectRoot)
		if err != nil {
			loadErrs = append(loadErrs, fmt.Sprintf("  %s: %v", p.SidecarPath, err))
			continue
		}
		if stale {
			// SPEC-009 Plan A Phase 12 — deterministic anchor repair.  // edikt-guard:allow
			// Before paying the LLM-dispatch cost, run AutoRepairAnchors
			// to see if pure-Go quote-primary / line-secondary scanning
			// can resolve the drift. Skip dispatch entirely when the
			// repair removes every stale anchor.
			//
			// Under --check / --dry-run the repair runs on an INDEPENDENT
			// copy loaded from disk, never on p.Sidecar: the check is asked
			// to measure the corpus, and a repair applied to the pairs it
			// goes on to measure (contradictions, render freshness) would
			// report the state it produced instead of the state it found.
			// Repairable drift is drift — it is counted stale, printed with
			// the `stale:` prefix the review commands grep for, and fails
			// the check (#9, c4-b). Nothing is written (c4-a).
			if opts.CheckOnly {
				if handled, residual := checkOnlyAutorepair(opts, p, res); handled {
					continue
				} else if residual != "" {
					reason = residual
				}
			} else {
				// p is a range-variable COPY of the pair, but p.Sidecar is a
				// pointer into pairs: the in-memory repair below reaches
				// Phase B through it. Never reassign p.Sidecar here — a new
				// pointer would be written to disk while Phase B compiled
				// the drifted original.
				repaired, rerr := phasea.TryAutoRepair(p.Sidecar, opts.ProjectRoot, p.ParentPath)
				if rerr == nil && repaired.Repaired != nil {
					rerr = writeSourceSidecar(opts, p.SidecarPath, repaired.Repaired)
				}
				if rerr != nil {
					fmt.Fprintf(opts.Stderr, "  autorepair: %s — %v (falling back to LLM dispatch)\n", p.ArtifactID, rerr)
				} else if repaired.AnchorsRepaired > 0 {
					fmt.Fprintf(opts.Stderr, "  autorepair: %s — repaired %d anchor(s)%s\n",
						p.ArtifactID, repaired.AnchorsRepaired,
						func() string {
							if repaired.StillStale {
								return " (residual drift, dispatching extractor)"
							}
							return " (fully resolved, skipping LLM dispatch)"
						}())
					if repaired.StillStale {
						reason = residualReason(p.Sidecar, opts.ProjectRoot, reason)
					}
				}
				if rerr == nil && !repaired.StillStale {
					continue
				}
			}
			// validate artifact ID + type at the dispatch boundary
			// before they flow into a `claude -p` argv string. A name that
			// slipped past discovery (unusual filename, future glob) MUST
			// NOT reach the locked extractor prompt.
			artifactType := artifactTypeFromPath(p.ParentPath, dirs)
			if err := idvalidate.ArtifactType(artifactType); err != nil {
				loadErrs = append(loadErrs, fmt.Sprintf("  %s: %v — refusing dispatch", p.ParentPath, err))
				continue
			}
			if err := idvalidate.ArtifactID(p.ArtifactID); err != nil {
				loadErrs = append(loadErrs, fmt.Sprintf("  %s: %v — refusing dispatch", p.ParentPath, err))
				continue
			}
			res.StaleSidecars = append(res.StaleSidecars, p.ArtifactID)
			tasks = append(tasks, phasea.Task{
				ArtifactType: artifactType,
				ArtifactID:   p.ArtifactID,
				ParentPath:   p.ParentPath,
				SidecarPath:  p.SidecarPath,
				ProjectRoot:  opts.ProjectRoot, // F-029
			})
			fmt.Fprintf(opts.Stderr, "  stale: %s — %s\n", p.ArtifactID, reason)
		}
	}

	if len(loadErrs) > 0 {
		fmt.Fprintln(opts.Stderr, "Sidecar errors:")
		for _, e := range loadErrs {
			fmt.Fprintln(opts.Stderr, e)
		}
		return res, fmt.Errorf("%d sidecar error(s)", len(loadErrs))
	}

	if opts.CheckOnly && len(res.StaleSidecars) > 0 {
		// --check mode is gate-friendly (called by CI and by tier-1 markdown).
		// Every stale sidecar counts, repairable or not: a check that passed
		// over drift it could have repaired reported a clean corpus that was
		// not one. The two remedies differ, so the line names both — the
		// binary applies a mechanical repair, extraction needs Claude Code.
		fmt.Fprintf(opts.Stderr,
			"error: %d sidecar(s) stale — %d mechanically repairable (run edikt gov compile), %d need extraction (run /edikt:gov:compile in Claude Code)\n",
			len(res.StaleSidecars), len(res.RepairableSidecars), len(tasks))
		return res, fmt.Errorf("stale sidecars in --check mode")
	}

	if len(tasks) > 0 {
		// ATOMICITY PRE-FLIGHT — refuse to dispatch while the corpus still
		// holds v1-shaped sidecars.
		//
		// The extractor vNext writes gov-sidecar.v2 (source_excerpts[]). If it
		// runs against a corpus that has not been migrated, one ordinary prose
		// edit is enough to produce a v2-shaped sidecar sitting next to
		// v1-shaped ones — a corpus in two shapes, converted by accident, one
		// artifact at a time, with no record of which is which. The migrate
		// verb converts the whole set in one deterministic pass; a partial
		// conversion by dispatch is not the same operation.
		//
		// It refuses BEFORE the first dispatch, not after: a run that
		// converted twenty artifacts and then stopped would leave exactly the
		// half-migrated state this exists to prevent.
		if legacy := v1ShapedSidecars(pairs); len(legacy) > 0 {
			fmt.Fprintf(opts.Stderr,
				"error: refusing to dispatch the extractor — %d of %d sidecar(s) still carry the v1 single-anchor shape:\n",
				len(legacy), countLoaded(pairs))
			for _, id := range legacy {
				fmt.Fprintf(opts.Stderr, "  %s\n", id)
			}
			fmt.Fprintln(opts.Stderr, "The extractor writes gov-sidecar.v2 (source_excerpts[]). Dispatching now would convert")
			fmt.Fprintln(opts.Stderr, "the corpus one artifact at a time, by accident, leaving it in two shapes at once.")
			fmt.Fprintln(opts.Stderr, "Run `edikt migrate lift` first — it converts every sidecar in one deterministic pass.")
			return res, fmt.Errorf("phase A: %d v1-shaped sidecar(s); run `edikt migrate lift`", len(legacy))
		}

		// Phase A no longer dispatches from here, and must not.
		//
		// Fanning extractors out means invoking an LLM, which INV-012
		// forbids a tier-2 binary from doing. ADR-030 carried an exemption
		// for this exact call site with a removal deadline that has already
		// slipped once; SPEC-013 replaces the dispatch with a contract —
		// tier-2 emits a schema-valid plan, tier-1 fans out one
		// sidecar-extractor per item, tier-2 verifies the result.
		//
		// So this returns a plan and a refusal rather than doing the work.
		// The refusal is the point: Phase B must not run against sidecars
		// that are known stale, and the only path that clears them now runs
		// through tier-1.
		if n := len(res.BootstrapSidecars); n > 0 {
			fmt.Fprintf(opts.Stderr, "Phase A: %d sidecar(s) need bootstrapping from prose (first compile of these artifacts)\n", n)
		}

		// Render the installed frontmatter from extractor.model config
		// BEFORE resolving it, so the resolution immediately below always
		// reflects the current config rather than
		// whatever the file happened to carry from a prior render or a
		// hand edit. A config load failure here is reported but does not
		// abort the plan — the render is best-effort; ResolveExtractorAgentModel
		// still runs against whatever is on disk.
		if planCfg, cfgErr := phasea.LoadConfig(opts.ProjectRoot); cfgErr == nil {
			if rerr := phasea.RenderInstalledExtractorFrontmatter(opts.ProjectRoot, planCfg.ExtractorModel); rerr != nil {
				fmt.Fprintf(opts.Stderr, "Phase A — could not render extractor frontmatter from config: %v\n", rerr)
			}
		} else {
			fmt.Fprintf(opts.Stderr, "Phase A — could not load config for extractor.model: %v\n", cfgErr)
		}

		// The extractor model is reported unconditionally now. It used to be
		// gated on `opts.Runner.(*phasea.ClaudeRunner)` — "only report a
		// model when we are the ones dispatching". With dispatch gone that
		// condition can never be true, so the gate would have silently
		// turned this into dead code and the field into a permanent empty
		// string. ADR-055 still governs WHERE the value comes from: the
		// extractor agent's frontmatter, never a CLI --model flag, and
		// UNKNOWN on failure rather than a substituted default.
		m, merr := phasea.ResolveExtractorAgentModel(opts.ProjectRoot)
		res.ExtractorModel = m
		if merr != nil {
			fmt.Fprintf(opts.Stderr,
				"Phase A — extractor model: %s (could not read %s: %v)\n",
				phasea.ExtractorModelUnknown, phasea.ExtractorAgentRelPath, merr)
		} else {
			fmt.Fprintf(opts.Stderr, "Phase A — extractor model: %s\n", m)
		}

		planOut, planErr := phasea.Plan(phasea.PlanOptions{ProjectRoot: opts.ProjectRoot})
		if planErr != nil || planOut == nil || planOut.ExitCode != phasea.ExitPlanEmitted {
			msg := "phase A: could not emit a dispatch plan"
			if planOut != nil && planOut.Message != "" {
				msg = msg + ": " + planOut.Message
			} else if planErr != nil {
				msg = msg + ": " + planErr.Error()
			}
			fmt.Fprintln(opts.Stderr, "error: "+msg)
			return res, errors.New(msg)
		}

		res.PhaseAPlanID = planOut.Plan.PlanID
		fmt.Fprintf(opts.Stderr,
			"Phase A: %d artifact(s) need extraction — plan %s emitted at %s\n",
			len(planOut.Plan.Items), planOut.Plan.PlanID,
			phasea.PlanFilePath(opts.ProjectRoot, planOut.Plan.PlanID))
		fmt.Fprintln(opts.Stderr, "Run /edikt:gov:compile from inside Claude Code: it fans out one")
		fmt.Fprintln(opts.Stderr, "sidecar-extractor per plan item, then runs `edikt phasea verify`.")
		return res, fmt.Errorf("phase A: %d artifact(s) await tier-1 extraction (plan %s); not running phase B",
			len(planOut.Plan.Items), planOut.Plan.PlanID)
	}

	// Skip-listed artifacts (superseded / deprecated / migration:skip) are
	// excluded from EVERYTHING downstream — the lossless check, the
	// migration_preserved strip, and Phase B's merge. Without this, a
	// retired ADR whose sidecar still existed on disk kept compiling its
	// directives into governance as duplicates of their replacement.
	//
	// The filter is also the only place the retired count exists. Downstream
	// of here the retired pairs are gone, so Phase B has no way to report
	// how many artifacts it did not compile — which is why the index header
	// used to assert "N accepted, 0 superseded" while never having counted
	// either. Tally by kind on the way past and hand the result to Merge as
	// data. Initialised non-nil: an empty map is a measured zero, and a nil
	// map is what an unmeasuring caller passes.
	//
	// Status exclusion (ADR-020:d03) rides the same filter, for the same  edikt-guard:allow
	// reason: an artifact that is not accepted must not reach the lossless
	// check, the migration_preserved strip, or the merge. It is announced
	// rather than dropped quietly — INV-015, governance content never leaves  edikt-guard:allow
	// silently. 165 index entries going dark with no line of output is the
	// same class of event as them arriving with no line of output.
	excludedByKind := map[string]int{}
	active := pairs[:0:0]
	for _, p := range pairs {
		if p.Skip {
			excludedByKind[p.Kind]++
			continue
		}
		if excluded, reason := statusExcluded(p); excluded {
			excludedByKind[p.Kind]++
			fmt.Fprintf(opts.Stderr, "  skip: %s — %s\n", p.ArtifactID, reason)
			continue
		}
		active = append(active, p)
	}
	pairs = active

	// BODY DRIFT — the second staleness signal (ADR-053).
	//
	// Reported for every active pair, in --check and in a full run alike, and
	// named separately from anchor drift because the two demand different
	// responses. Anchor drift means the sidecar is WRONG (a recorded quote no
	// longer matches: directives claim support they have lost) and is urgent.
	// Body drift means the sidecar may be INCOMPLETE (prose changed that
	// extraction never saw) and calls for a regeneration.
	//
	// Body drift is NOT folded into the stale verdict and MUST NOT gate the
	// run. Doing so would make every prose edit dispatch a full LLM
	// re-extraction and collapse ADR-028's Phase A / Phase B separation. It is
	// a report, and its remedy is a regeneration the operator chooses.
	bodyResults := make([]sidecar.BodyDriftResult, 0, len(pairs))
	for i := range pairs {
		if pairs[i].Sidecar == nil {
			continue
		}
		bodyResults = append(bodyResults,
			pairs[i].Sidecar.CheckBodyDriftAgainstParent(opts.ProjectRoot, os.ReadFile))
	}
	bodySummary := sidecar.SummarizeBodyDrift(bodyResults)
	res.BodyDrift = &bodySummary

	if opts.CheckOnly {
		// Emit a verdict line even when no stale sidecars were found —
		// silent exit-0 in --check mode (regression rc≤7) made CI checks
		// indistinguishable from "compile is broken" vs "all good".
		//
		// Both signals are named. A bare "up-to-date, 0 stale" answered a
		// narrower question than it appeared to: it meant "nothing extracted
		// has changed" and read as "the corpus is in sync". Those differ by
		// prose ADDED since extraction, which is invisible to anchor drift by
		// construction — and that gap once hid thirteen unenforced governance
		// rules across four commits while this line reported clean.
		// SPEC-010 phase 9 (AC-9.1): cheap, deterministic, read-only — runs  edikt-guard:allow
		// in --check mode too, not just full compile. See the non-check
		// path below for the shared reporting shape and the precedence-rule
		// reasoning (AC-9.2), documented once in internal/contradiction's
		// package doc rather than duplicated at both call sites.
		res.Conflicts = contradiction.Detect(pairs)
		res.Restatements = contradiction.DetectRestatements(pairs)

		if !opts.JSONMode {
			fmt.Fprintf(opts.Stderr,
				"edikt gov compile --check: anchor drift: %d stale (%d sidecar(s))\n", len(res.StaleSidecars), len(pairs))
			fmt.Fprintf(opts.Stderr, "%s\n", bodySummary.ReportLine())
			if len(res.Conflicts) == 0 {
				fmt.Fprintln(opts.Stderr, "conflicts: none detected (same-topic, same-subject, opposing-modality pairs)")
			} else {
				fmt.Fprintf(opts.Stderr, "conflicts: %d detected. NOT auto-resolved; each needs a human decision:\n", len(res.Conflicts))
				for _, c := range res.Conflicts {
					fmt.Fprintf(opts.Stderr, "  - [%s] %s (%s) vs %s (%s) — %q\n",
						c.Topic, c.A.Source, c.A.Modality, c.B.Source, c.B.Modality, c.NounPhrase)
				}
			}
			if len(res.Restatements) == 0 {
				fmt.Fprintf(opts.Stderr, "restatements: none detected across %d sidecar(s) (same subject, same modality, different artifacts)\n", len(pairs))
			} else {
				fmt.Fprint(opts.Stderr, contradiction.RestatementReport(res.Restatements))
			}
		}

		// RENDER FRESHNESS (AC-2.9 / PC-8). Every check above asks whether the
		// SIDECARS are current. None of them asks whether the RENDERED TREE
		// matches those sidecars — so a tree rendered from an earlier sidecar
		// state passed --check while serving stale governance, and the check
		// reported clean at the moment it was most wrong (SR-020 / SAC-003).
		//
		// It runs LAST because it is the most expensive check here (a full
		// shadow render), and it changes the exit code, so it must not be
		// short-circuited by the cheaper ones reporting clean.
		// Descriptions are a REAL render input (they appear in the ambient
		// topic index, topic files and skill frontmatter), so the shadow
		// render must be given the same ones the live tree was built from.
		// A shadow render with empty descriptions would differ from the live
		// tree on every topic and report universal drift — a check that always
		// fails is as useless as one that never does.
		checkRegistry, cregErr := topicregistry.LoadOrEmpty(
			filepath.Join(opts.ProjectRoot, ".edikt", "topics.yaml"))
		if cregErr != nil {
			return res, fmt.Errorf("render freshness: load topic registry: %w", cregErr)
		}
		checkDescriptions := make(map[string]string, len(checkRegistry))
		for topic, entry := range checkRegistry {
			checkDescriptions[topic] = entry.Description
		}
		drifts, ferr := phaseb.CheckRenderFreshness(opts.ProjectRoot, pairs, phaseb.Options{
			CompiledAt:        clk.Now().Format(time.RFC3339),
			CompilerVersion:   CompilerVersion,
			Excluded:          excludedByKind,
			ContractBlock:     phaseb.ContractBlockFor(opts.ProjectRoot, "governance-authoring"),
			TopicDescriptions: checkDescriptions,
		})
		if ferr != nil {
			// UNMEASURED, and therefore a failure. A freshness check that could
			// not run is not a fresh tree (INV-011: fail closed).  edikt-guard:allow
			fmt.Fprintf(opts.Stderr, "render freshness: UNMEASURED — %v\n", ferr)
			fmt.Fprintln(opts.Stderr, "  A check that could not run is not a clean result.")
			return res, fmt.Errorf("render freshness check failed to run: %w", ferr)
		}
		res.RenderDrift = make([]string, 0, len(drifts))
		for _, d := range drifts {
			res.RenderDrift = append(res.RenderDrift, d.String())
		}
		if len(drifts) == 0 {
			if !opts.JSONMode {
				fmt.Fprintln(opts.Stderr, "render freshness: rendered surfaces match a fresh render of the current sidecars")
			}
			return res, nil
		}
		if !opts.JSONMode {
			fmt.Fprintf(opts.Stderr, "render freshness: %d surface(s) STALE relative to current sidecars:\n", len(drifts))
			for _, d := range drifts {
				fmt.Fprintf(opts.Stderr, "  - %s\n", d.String())
			}
			for _, line := range phaseb.RemedyFor(drifts) {
				fmt.Fprintf(opts.Stderr, "  %s\n", line)
			}
		}
		return res, fmt.Errorf("%d rendered surface(s) stale relative to current sidecars", len(drifts))
	}

	// Two-phase migration: Phase B post-extractor verification + cleanup.
	//
	// (1) Lossless check: for every sidecar that still carries
	// migration_preserved (extractor just ran on it, or it was
	// loaded fresh), compare MigrationPreserved.Directives against
	// the canonical Directives + Prohibitions + ManualDirectives
	// the extractor produced. Any unmatched (modality, ref_id,
	// normalized noun-phrase) tuple is a loss — the extractor
	// dropped or rephrased preserved baseline content.
	//
	// (2) Strip: after the check captures the comparison data,
	// deterministically remove migration_preserved from the
	// sidecar so steady-state sidecars don't carry vestigial
	// v0.4-era data forever.
	//
	// Order matters: check BEFORE strip so we have access to the
	// preserved baseline to compare against.
	for i := range pairs {
		if pairs[i].Sidecar == nil || pairs[i].Sidecar.MigrationPreserved == nil {
			continue
		}

		// (1) Lossless check.
		//
		// The pass has now run for at least one artifact, so the report
		// lists become "ran and found nothing" rather than "did not run".
		if res.LosslessUnmeasured == nil {
			res.LosslessUnmeasured = []UnmeasuredRec{}
		}
		mp := pairs[i].Sidecar.MigrationPreserved
		switch {
		case losslessMeasurable(mp):
			losses := migrate.CheckLosslessAgainstDirectives(mp.Directives, pairs[i].Sidecar)
			if len(losses) > 0 {
				res.LosslessReport = append(res.LosslessReport, LossArtifactRec{
					SidecarPath: pairs[i].SidecarPath,
					Losses:      losses,
				})
			}
		default:
			res.LosslessUnmeasured = append(res.LosslessUnmeasured, UnmeasuredRec{
				SidecarPath: pairs[i].SidecarPath,
				Reason:      losslessUnmeasurableReason(mp),
			})
		}

		// (2) Strip + rewrite
		pairs[i].Sidecar.MigrationPreserved = nil
		// Marshal, NOT Marshal. Load primes a canonical-bytes cache on
		// the struct and Marshal returns it verbatim, so this strip was a
		// NO-OP: migration_preserved survived every compile while the code
		// read as though it removed it, violating ADR-034's requirement that
		// steady-state sidecars carry no transient field.
		//
		// Found by StampBodyDigest hitting the identical trap one function
		// away, and confirmed empirically rather than by reading.
		out, merr := sidecar.Marshal(pairs[i].Sidecar)
		if merr != nil {
			return res, fmt.Errorf("strip migration_preserved from %s: %w", pairs[i].SidecarPath, merr)
		}
		if werr := writeSourceSidecar(opts, pairs[i].SidecarPath, out); werr != nil {
			return res, fmt.Errorf("rewrite %s after migration_preserved strip: %w", pairs[i].SidecarPath, werr)
		}
	}

	// Enforce the OnLoss policy when the lossless check found any
	// dropped directives. Closes the silent-loss class — the extractor
	// could have rephrased or dropped preserved baseline content, and
	// without this gate that loss would just appear in governance.md
	// silently. Default behavior (TTY → accept, non-TTY → abort) makes
	// CI catch regressions while interactive use stays low-friction.
	if len(res.LosslessReport) > 0 {
		policy := resolveLossPolicy(opts.OnLoss, os.Stdin)
		fmt.Fprintln(opts.Stderr)
		fmt.Fprintf(opts.Stderr, "lossless check: %d sidecar(s) lost directives during Phase A extraction:\n", len(res.LosslessReport))
		for _, rec := range res.LosslessReport {
			fmt.Fprintf(opts.Stderr, "  %s — %d loss(es):\n", filepath.Base(rec.SidecarPath), len(rec.Losses))
			for _, l := range rec.Losses {
				excerpt := l.LegacyText
				if len(excerpt) > 100 {
					excerpt = excerpt[:97] + "..."
				}
				fmt.Fprintf(opts.Stderr, "    - %s: %s\n", l.Type, excerpt)
			}
		}
		switch policy {
		case "abort":
			fmt.Fprintln(opts.Stderr)
			fmt.Fprintln(opts.Stderr, "abort (per --on-loss=abort). The sidecar-extractor dropped or rephrased content")
			fmt.Fprintln(opts.Stderr, "from migration_preserved that the user explicitly chose to carry forward on upgrade.")
			fmt.Fprintln(opts.Stderr, "Options:")
			fmt.Fprintln(opts.Stderr, "  - Re-run with --on-loss=accept to keep the extractor's output as-is.")
			fmt.Fprintln(opts.Stderr, "  - Edit the affected sidecar(s) by hand to restore dropped entries.")
			fmt.Fprintln(opts.Stderr, "  - File an extractor-regression bug if the loss is unexpected.")
			return res, fmt.Errorf("lossless check: %d sidecar(s) with loss after Phase A", len(res.LosslessReport))
		case "accept":
			fmt.Fprintln(opts.Stderr)
			fmt.Fprintln(opts.Stderr, "continuing (per --on-loss=accept). Loss documented above is now persisted.")
		}
	}

	// Surface artifacts the check could not measure. This is deliberately
	// separate from the loss report above: a loss is a known regression, an
	// unmeasured artifact is an unknown. Reporting them together would let the
	// second hide inside the first's count.
	//
	// It routes through the SAME OnLoss policy rather than a softer one. The
	// policy's whole premise is that CI must opt in to accepting silent loss,
	// and an unverifiable extraction is silent loss that has not been proven
	// either way — INV-013's "absence is never a passing measurement" applies
	// to the gate's own denominator, not only to what it reports.
	if len(res.LosslessUnmeasured) > 0 {
		policy := resolveLossPolicy(opts.OnLoss, os.Stdin)
		fmt.Fprintln(opts.Stderr)
		fmt.Fprintf(opts.Stderr, "lossless check: UNMEASURED for %d sidecar(s) — the check ran but had no baseline to compare against:\n", len(res.LosslessUnmeasured))
		for _, rec := range res.LosslessUnmeasured {
			fmt.Fprintf(opts.Stderr, "  %s\n    %s\n", filepath.Base(rec.SidecarPath), rec.Reason)
		}
		switch policy {
		case "abort":
			fmt.Fprintln(opts.Stderr)
			fmt.Fprintln(opts.Stderr, "abort (per --on-loss=abort). These artifacts passed the lossless gate without")
			fmt.Fprintln(opts.Stderr, "being measured by it; that is not the same as passing it. Options:")
			fmt.Fprintln(opts.Stderr, "  - Re-run with --on-loss=accept to proceed with the extraction unverified.")
			fmt.Fprintln(opts.Stderr, "  - Restore the artifact's sentinel from version control and re-run migrate,")
			fmt.Fprintln(opts.Stderr, "    which now preserves unparseable blocks verbatim into raw_body.")
			fmt.Fprintln(opts.Stderr, "  - Review the .md against its sidecar by hand and confirm the rules survived.")
			return res, fmt.Errorf("lossless check: %d sidecar(s) unmeasured after Phase A", len(res.LosslessUnmeasured))
		case "accept":
			fmt.Fprintln(opts.Stderr)
			fmt.Fprintln(opts.Stderr, "continuing (per --on-loss=accept). These extractions remain unverified.")
		}
	}

	// ── Topic registry (.edikt/topics.yaml) ─────────────────────────────────
	//
	// The registry is INPUT to the render, never output. A missing file is the
	// legitimate no-subject case (a project that has never run the approval
	// ceremony); every other read error fails, because a registry that exists
	// but will not parse must not degrade into "zero topics approved" — that
	// would silently drop every pinned description and render the corpus as if
	// nobody had ever approved anything.
	registryPath := topicregistry.PathFor(opts.ProjectRoot)
	registry, regErr := topicregistry.LoadOrEmpty(registryPath)
	if regErr != nil {
		return res, fmt.Errorf("topic registry %s: %w", registryPath, regErr)
	}
	corpusTopics := make([]string, 0, len(pairs))
	for _, p := range pairs {
		if p.Sidecar != nil {
			corpusTopics = append(corpusTopics, p.Sidecar.Topic)
		}
	}
	// Route extraction-time proposals into the approval queue and open a slot
	// for every topic still without an approved description. This runs BEFORE
	// Measure so the coverage line reports the state a human will act on.
	//
	// It writes only to .edikt/state/ and to sidecars (stripping transient
	// fields); it never writes .edikt/topics.yaml, which is why an upgrade can
	// enumerate its whole backlog without any description being invented.
	routing, routeErr := RouteProposals(opts.ProjectRoot, pairs, registry, clk.Now())
	if routeErr != nil {
		return res, fmt.Errorf("route proposals: %w", routeErr)
	}
	res.ProposalRouting = &routing

	registryCoverage := topicregistry.Measure(registry, corpusTopics)
	descriptions := make(map[string]string, len(registry))
	for topic, entry := range registry {
		descriptions[topic] = entry.Description
	}
	res.RegistryCoverage = &registryCoverage

	pbRes, err := phaseb.Merge(opts.ProjectRoot, pairs, phaseb.Options{
		CompiledAt:        clk.Now().Format(time.RFC3339),
		CompilerVersion:   CompilerVersion,
		Excluded:          excludedByKind,
		ContractBlock:     phaseb.ContractBlockFor(opts.ProjectRoot, "governance-authoring"),
		TopicDescriptions: descriptions,
	})
	if err != nil {
		return res, fmt.Errorf("phase B: %w", err)
	}
	res.PhaseB = pbRes

	// In JSON mode the caller emits a single JSON object on stdout; the
	// prose summary line is routed to stderr at a lower verbosity to avoid
	// breaking machine-readable consumers.
	summaryOut := opts.Stdout
	if opts.JSONMode {
		summaryOut = opts.Stderr
	}
	fmt.Fprintf(summaryOut, "Phase B — merged %d sidecar(s) into %d topic file(s) (%d rendered, %d unchanged), index %s\n",
		len(pairs),
		len(pbRes.TopicsRendered)+len(pbRes.TopicsUnchanged),
		len(pbRes.TopicsRendered),
		len(pbRes.TopicsUnchanged),
		writtenLabel(pbRes.IndexWritten),
	)

	// Registry coverage. Reported on EVERY compile, not only when something
	// is wrong: the migration window's whole contract is that compile renders
	// what is approved and REPORTS what is pending, with counts and a
	// denominator, rather than either inventing a description or failing.
	// A corpus with zero topics stays silent — no subject, no announcement.
	if registryCoverage.Total() > 0 {
		fmt.Fprintln(summaryOut, registryCoverage.Report())
	}
	if !routing.Empty() {
		fmt.Fprintln(summaryOut, routing.Report())
	}

	// Scope coverage. Without a reported fraction the unscoped topics sit
	// unscoped permanently and nobody learns it is fixable — so the line
	// names the sidecars holding them open, which is the clearing
	// condition. A project with no topics at all stays silent: announcing
	// non-coverage of a subject that does not exist is noise (INV-013).
	//
	// The CONSEQUENCE clause changed with SPEC-011 stage 1 and the wording  edikt-guard:allow
	// had to follow it. An unscoped topic used to render a tier-2 rules file
	// globbed at `**/*` — loaded on every edit, which is what put ~44k tokens
	// of scoped governance into the ambient budget. It now retires to tier 3
	// and is reached by skill invocation instead. Leaving the old "(**/*)"
	// text would have described a mechanism that no longer exists, which is
	// the stale-report class: a line that still prints, still looks
	// authoritative, and is silently wrong.
	if total := len(pbRes.TopicsScoped) + len(pbRes.TopicsUnscoped); total > 0 {
		if len(pbRes.TopicsUnscoped) == 0 {
			fmt.Fprintf(summaryOut,
				"Scope — all %d topic file(s) scoped by declared globs\n", total)
		} else {
			fmt.Fprintf(summaryOut,
				"Scope — %d of %d topic(s) scoped to tier 2; %d retired to tier 3 (skill-invoked, not ambient) pending glob declarations on %d sidecar(s): %s\n",
				len(pbRes.TopicsScoped), total, len(pbRes.TopicsUnscoped),
				len(pbRes.UndeclaredSources), strings.Join(pbRes.UndeclaredSources, ", "))
		}
	}

	// Retirement, named per topic. The scope line above gives the FRACTION;
	// this gives the topics and where each one went. Both are needed: a
	// reader who sees "6 retired" and cannot find out WHICH has been told a
	// number, not an outcome — and a topic that vanished from
	// `.claude/rules/` with nothing naming it reads as a compile that lost
	// it rather than one that moved it on purpose.
	if len(pbRes.TopicsRetiredToSkill) > 0 {
		fmt.Fprintf(summaryOut,
			"Tier 3 — %d topic(s) retired from .claude/rules/ to skill packages: %s\n",
			len(pbRes.TopicsRetiredToSkill), strings.Join(pbRes.TopicsRetiredToSkill, ", "))

		// THE CONSEQUENCE, said out loud. The line above reports where the
		// topics went; this reports what it COST, which is the half a reader
		// cannot derive. Retiring a topic reaches every sidecar in it — a
		// scoped sidecar with perfectly good globs still loses path-triggered
		// delivery because a SIBLING declared none.
		//
		// Judgement routing is the right destination for a rule with no file
		// scope: a process rule or a naming convention has nothing to trigger
		// on, and for those the skill package's description IS the enforcement
		// mechanism, not a nicety — it is the only thing that can select the
		// rule before any file is touched. This line is not a defect queue.
		// What it makes visible is the OTHER case: a directive routed to
		// judgement because path inference came up empty used to be
		// indistinguishable from one routed there deliberately.
		if pbRes.JudgementRoutedDirectives > 0 {
			fmt.Fprintf(summaryOut,
				"         %d directive(s) are consequently reachable by judgement only — no path match delivers them.\n",
				pbRes.JudgementRoutedDirectives)
			fmt.Fprintf(summaryOut,
				"         Path inference is owned by PRD-004 (extractor quality); PRD-005 records it as DF-7. Not fixed by declaring globs by hand.\n")
		}
	}

	// F-115/A3 — a reachability REGRESSION, reported distinctly from the
	// routine tier-3 line above. The line above answers "which topics are
	// skill-only right now" (true on a fresh clone too, nothing wrong). This
	// answers "which topics had path-triggered delivery a moment ago and lost
	// it because of THIS run" — found live: a topic reassignment left a
	// project's core domain silently unreachable, with nothing warning at
	// compile time; the only existing signal was doctor's own scope check,
	// a separately-invoked, point-in-time snapshot with no notion of "did
	// this compile cause it" and no attribution to which reassignment did.
	if len(pbRes.TopicsNewlyUnreachable) > 0 {
		fmt.Fprintf(summaryOut,
			"⚠ REACHABILITY REGRESSION — %d topic(s) had path-triggered delivery before this compile and do not after it: %s\n"+
				"  Each had a .claude/rules/governance/<topic>.md file an instant ago; this run removed it because "+
				"every contributing sidecar is now undeclared (no paths:). If an artifact was just reassigned to "+
				"one of these topics, that reassignment is very likely why. Declare paths: on at least one "+
				"contributing sidecar (edikt sidecar approve --kind paths) to restore delivery, or confirm the "+
				"topic dropping to tier 3 (skill-invoked only) is intended.\n",
			len(pbRes.TopicsNewlyUnreachable), strings.Join(pbRes.TopicsNewlyUnreachable, ", "))
	}

	// SPEC-010 phase 9 (AC-9.1/9.2): contradiction detection, report only.  edikt-guard:allow
	// pairs is already supersession-filtered by this point (retired
	// artifacts are excluded before Phase A/B ever see them), so every
	// conflict found here is between two currently-accepted directives with
	// no precedence rule between them — surfaced, never auto-resolved.
	res.Conflicts = contradiction.Detect(pairs)
	res.Restatements = contradiction.DetectRestatements(pairs)
	if len(res.Conflicts) == 0 {
		fmt.Fprintln(summaryOut, "Conflicts — none detected (same-topic, same-subject, opposing-modality pairs)")
	} else {
		fmt.Fprintf(summaryOut, "Conflicts — %d detected. NOT auto-resolved; each needs a human decision:\n", len(res.Conflicts))
		for _, c := range res.Conflicts {
			fmt.Fprintf(summaryOut, "  - [%s] %s (%s) vs %s (%s) — %q\n",
				c.Topic, c.A.Source, c.A.Modality, c.B.Source, c.B.Modality, c.NounPhrase)
		}
	}

	// Restatements are ADVISORY and never block: two artifacts agreeing is
	// not a broken corpus, it is an ownership question. The count carries its
	// denominator so "none" is readable as a measurement over a scanned
	// corpus rather than as a check that did not run.
	if len(res.Restatements) == 0 {
		fmt.Fprintf(summaryOut, "Restatements — none detected across %d sidecar(s) (same subject, same modality, different artifacts)\n", len(pairs))
	} else {
		fmt.Fprint(summaryOut, contradiction.RestatementReport(res.Restatements))
	}
	return res, nil
}

// artifactTypeFromPath returns "adr", "invariant", or "guideline" based on
// which configured dir the parent .md sits under.
func artifactTypeFromPath(parentPath string, dirs []string) string {
	abs, _ := filepath.Abs(parentPath)
	if strings.Contains(abs, string(os.PathSeparator)+"decisions"+string(os.PathSeparator)) {
		return "adr"
	}
	if strings.Contains(abs, string(os.PathSeparator)+"invariants"+string(os.PathSeparator)) {
		return "invariant"
	}
	if strings.Contains(abs, string(os.PathSeparator)+"guidelines"+string(os.PathSeparator)) {
		return "guideline"
	}
	for _, d := range dirs {
		if strings.Contains(parentPath, d) {
			if strings.Contains(d, "decisions") {
				return "adr"
			}
			if strings.Contains(d, "invariants") {
				return "invariant"
			}
			if strings.Contains(d, "guidelines") {
				return "guideline"
			}
		}
	}
	return "adr"
}

func writtenLabel(b bool) string {
	if b {
		return "rewritten"
	}
	return "unchanged"
}

// acquireCompileLock takes an advisory flock on .edikt/state/compile.lock.
// When noWait is true, contention returns an error immediately rather than
// blocking. The returned func releases the lock and closes the file
// descriptor; the lock file itself is intentionally left in place
// (advisory locks survive across runs and the file is purely a lock
// token — no consumer reads its contents). Removing it after release
// would race against any concurrent process that just opened the same
// path; benign today on POSIX but a real bug if the file ever doubles
// as a PID record.
func acquireCompileLock(projectRoot string, noWait bool, stderr io.Writer) (func(), error) {
	if stderr == nil {
		stderr = os.Stderr
	}
	lockDir := filepath.Join(projectRoot, ".edikt", "state")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir lock dir: %w", err)
	}
	lockPath := filepath.Join(lockDir, "compile.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	// Try non-blocking first so contention is VISIBLE. The advisory flock
	// self-releases when its holder dies, so the file on disk is never the
	// stale artifact — but a silent blocking wait here looked exactly like
	// a hung compile in the field (killed run + orphaned claude child
	// still holding the lock). Announce, then wait.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		return flockRelease(f), nil
	}
	if noWait {
		f.Close()
		return nil, fmt.Errorf("compile already running (lock held); --no-wait set, exiting")
	}
	fmt.Fprintf(stderr, "compile.lock is held by another process — waiting for it to finish (Ctrl-C to abort, --no-wait to fail fast)\n")
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquire compile.lock: %w", err)
	}
	return flockRelease(f), nil
}

func flockRelease(f *os.File) func() {
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}
}
