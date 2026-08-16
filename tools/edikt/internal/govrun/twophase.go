package govrun

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/contradiction"
	"github.com/diktahq/edikt/tools/edikt/internal/idvalidate"
	"github.com/diktahq/edikt/tools/edikt/internal/lossless"
	"github.com/diktahq/edikt/tools/edikt/internal/parse"
	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
	"github.com/diktahq/edikt/tools/edikt/internal/phaseb"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/internal/topicregistry"
	"github.com/diktahq/edikt/tools/edikt/model"
	"golang.org/x/term"
)

// TwoPhaseOptions configures one invocation of the sidecar-aware compile.
type TwoPhaseOptions struct {
	ProjectRoot string
	CheckOnly   bool
	NoWait      bool
	JSONMode    bool          // suppress prose UI on stdout; caller emits JSON
	Runner      phasea.Runner // injectable for tests; defaults to ClaudeRunner
	Concurrency int           // defaults to 8
	Stderr      io.Writer
	Stdout      io.Writer

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

	// BootstrapSidecars is the subset of StaleSidecars that had no sidecar
	// on disk at all (as opposed to a sidecar whose anchors drifted). Kept
	// separate so callers can distinguish "first compile of a project that
	// predates edikt" from "routine resync" — the two have very different
	// cost profiles and the JSON consumer should be able to tell them apart.
	BootstrapSidecars []string `json:"bootstrap_sidecars,omitempty"`

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

	// RenderDrift is AC-2.9's report: rendered surfaces that do not match a
	// fresh render of the current sidecars. Empty and non-nil means the check
	// RAN and found nothing; nil means it did not run — a distinction a
	// consumer must be able to make (INV-013).
	RenderDrift []string `json:"render_drift,omitempty"`
}

// LossArtifactRec is a JSON-friendly per-artifact loss record.
type LossArtifactRec struct {
	SidecarPath string          `json:"sidecar_path"`
	Losses      []lossless.Loss `json:"losses"`
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

// statusExcluded reports whether a discovered pair's parent artifact is
// disqualified from compiling by its STATUS — a `proposed` ADR, a `revoked`
// invariant — and returns a human-readable reason on a hit.
//
// ADR-020:d03 is the requirement: the tier-2 compile helper filters source  edikt-guard:allow
// documents by status (accepted ADRs, active invariants, all guidelines).
// Nothing on the sidecar path enforced it. sidecar.Discover carries a
// DENYLIST (skip `superseded` / `deprecated` / migration:skip) and
// parse.IsIncluded carries the ALLOWLIST the ADR describes, and the denylist
// — the weaker one — is the one on the path that runs, so every other
// status walked straight through: ADR-038, `status: proposed`, compiled 165  edikt-guard:allow
// directive-index entries and the write-time tier delivered them as
// MUST-grade denies (F-069).
//
// The decision is NOT re-implemented here. It delegates to parse.IsIncluded
// because a second copy of a decision procedure is the defect, not the fix
// (GL-002): the two copies that already existed are what drifted. In
// particular, an allowlist written fresh against frontmatter alone would
// take 344 index entries dark rather than 165 — ADR-001, ADR-007, ADR-010  edikt-guard:allow
// and ADR-060 carry no frontmatter status and declare acceptance in a  edikt-guard:allow
// `**Status:** Accepted` body line, which is exactly the fallback
// parse.IsIncluded already implements and a third copy would forget.
//
// The filter deliberately does NOT live in sidecar.isSkipListed. That
// predicate also feeds `migrate sidecars`, doctor coverage, verify-all and
// legacy-sentinel detection — none of which are asking "should these
// directives compile". Widening it would answer four questions with the
// answer to one.
func statusExcluded(p sidecar.Pair) (bool, string) {
	var kind string
	switch p.Kind {
	case sidecar.KindADR:
		kind = "adr"
	case sidecar.KindInvariant:
		kind = "inv"
	case sidecar.KindGuideline:
		kind = "guideline"
	default:
		// Parent found in a directory the caller gave no class for. This
		// filter's job is to drop artifacts whose status is known and
		// disqualifying, not artifacts whose class could not be read.
		return false, ""
	}

	doc, err := parse.LoadDocument(p.ParentPath)
	if err != nil {
		// An unreadable parent is not a status verdict. The LoadErr and
		// bootstrap paths in RunTwoPhase report it on their own terms.
		return false, ""
	}
	if doc.IsIncluded(kind) {
		return false, ""
	}

	status := strings.TrimSpace(doc.Frontmatter.Status)
	if status == "" {
		status = strings.TrimSpace(doc.BoldStatus)
	}
	if status == "" {
		status = "no status"
	}
	return true, fmt.Sprintf("status: %s — only accepted ADRs and active invariants compile", status)
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
		opts.Concurrency = 8
	}
	if opts.Runner == nil {
		opts.Runner = &phasea.ClaudeRunner{}
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
			repaired, rerr := phasea.TryAutoRepair(p.Sidecar, opts.ProjectRoot, p.ParentPath, p.SidecarPath)
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
			}
			if rerr == nil && !repaired.StillStale {
				continue
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

	if opts.CheckOnly && len(tasks) > 0 {
		// --check mode is gate-friendly (called by CI and by tier-1 markdown).
		// Lead with the slash command for human readers; mention the binary
		// form parenthetically for CI/scripting contexts that don't have a
		// Claude Code session.
		fmt.Fprintf(opts.Stderr, "error: %d sidecar(s) stale — run /edikt:gov:compile in Claude Code to resync (or 'edikt gov compile' directly in CI)\n", len(tasks))
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
			fmt.Fprintln(opts.Stderr, "Run `bin/edikt migrate to-v2` first — it converts every sidecar in one deterministic pass.")
			return res, fmt.Errorf("phase A: %d v1-shaped sidecar(s); run `edikt migrate to-v2`", len(legacy))
		}

		// Fail fast when the dispatch target is unreachable. Without this,
		// a project with 42 uncompiled artifacts fans out 42 tasks that
		// each die with `exec: "claude": executable file not found` — 42
		// identical stack-shaped errors instead of one actionable line.
		if err := opts.Runner.Preflight(); err != nil {
			fmt.Fprintf(opts.Stderr, "error: cannot dispatch the sidecar extractor for %d artifact(s): %v\n", len(tasks), err)
			fmt.Fprintln(opts.Stderr, "Phase A runs the per-artifact `:compile` command in a Claude Code session.")
			fmt.Fprintln(opts.Stderr, "Run /edikt:gov:compile from inside Claude Code, or install the `claude` CLI and re-run.")
			return res, fmt.Errorf("phase A: extractor unavailable")
		}

		if n := len(res.BootstrapSidecars); n > 0 {
			fmt.Fprintf(opts.Stderr, "Phase A: bootstrapping %d sidecar(s) from prose (first compile of these artifacts)\n", n)
		}

		ctx := context.Background()
		dis := &phasea.Dispatcher{
			Runner:       opts.Runner,
			Concurrency:  opts.Concurrency,
			ProgressOut:  opts.Stderr,
			ErrorLogPath: filepath.Join(opts.ProjectRoot, ".edikt", "state", "compile-errors.log"),
		}
		// Record and announce the pinned model. A run whose output nobody
		// can attribute to a model is a measurement that cannot be
		// compared to any other — which is what every extraction baseline
		// taken before the pin now is.
		//
		// Resolved from the ClaudeRunner when that is the runner in play;
		// a caller that injected its own runner (tests, future
		// harnesses) is not assumed to dispatch to any model at all, so
		// the field stays empty rather than reporting a model that never
		// ran. Reporting the default here regardless would be exactly the
		// unmeasured-as-measured shape this pass has been removing.
		//
		// ADR-055: read the model from the EXTRACTOR AGENT'S frontmatter,
		// not from the CLI --model value. --model pins the session that
		// runs the slash command; that session forks a subagent per
		// artifact (ADR-027) and the subagent's frontmatter governs. The
		// two disagreed for three months — the CLI said claude-opus-5 while
		// every sidecar in the corpus was written by sonnet, and the log
		// line reported the CLI's value (D27).
		//
		// On failure this reports UNKNOWN and never substitutes the CLI
		// value or DefaultExtractorModel. Substituting one is precisely how
		// an unmeasured value came to be rendered as a measured one.
		if _, ok := opts.Runner.(*phasea.ClaudeRunner); ok {
			m, merr := phasea.ResolveExtractorAgentModel(opts.ProjectRoot)
			res.ExtractorModel = m
			if merr != nil {
				fmt.Fprintf(opts.Stderr,
					"Phase A — extractor model: %s (could not read %s: %v)\n",
					phasea.ExtractorModelUnknown, phasea.ExtractorAgentRelPath, merr)
			} else {
				fmt.Fprintf(opts.Stderr, "Phase A — extractor model: %s\n", m)
			}
		}

		paRes := dis.Run(ctx, tasks)
		res.PhaseADone = true
		res.PhaseAFailures = len(paRes.Failures)
		fmt.Fprintf(opts.Stderr, "Phase A done: %d ok, %d failed in %s\n",
			paRes.TaskCount-len(paRes.Failures), len(paRes.Failures), paRes.Wall.Round(time.Millisecond))

		if len(paRes.Failures) > 0 {
			fmt.Fprintf(opts.Stderr, "Phase A failures (logged to .edikt/state/compile-errors.log):\n")
			for _, f := range paRes.Failures {
				fmt.Fprintf(opts.Stderr, "  %s: %v\n", f.Task.ArtifactID, f.Err)
				res.PhaseAErrors = append(res.PhaseAErrors, PhaseAErrorRec{
					ArtifactID:   f.Task.ArtifactID,
					ArtifactType: f.Task.ArtifactType,
					Err:          f.Err.Error(),
				})
			}
			return res, fmt.Errorf("phase A: %d failure(s); not running phase B", len(paRes.Failures))
		}

		// ADR-053 — stamp the body-drift baseline.
		//
		// This is the ONLY honest moment to do it: the extractor has just
		// read this parent body and produced a sidecar from it, so the digest
		// recorded here is genuinely "what extraction saw". Stamping anywhere
		// else — a backfill pass over the whole corpus, say — would write a
		// baseline claiming an extraction that never happened, converting the
		// signal from "this sidecar may be incomplete" into a confident lie.
		// That is strictly worse than the unmeasured state it replaces, which
		// is why pre-ADR-053 sidecars stay unmeasured until they are next
		// regenerated rather than being backfilled to look clean.
		//
		// Reached only when every Phase A task succeeded (the early return
		// above), so no failed extraction can leave a baseline behind.
		for _, t := range tasks {
			if err := sidecar.StampBodyDigest(t.SidecarPath, t.ParentPath,
				os.ReadFile,
				func(p string, b []byte) error { return os.WriteFile(p, b, 0o644) },
			); err != nil {
				// Non-fatal: the extraction itself succeeded, and a missing
				// baseline degrades to UNMEASURED, which is a safe state. A
				// failure to stamp must not discard good extraction work.
				fmt.Fprintf(opts.Stderr,
					"  warn: %s — could not record body-drift baseline: %v (reported as unmeasured)\n",
					t.ArtifactID, err)
			}
		}

		// Reload sidecars now that subagents have regenerated them. Two
		// paths (Phase 7 of PLAN-sidecar-review-fixes #41):
		//
		// • Default — full re-Discover. Conservative; matches v0.6.0
		// RC behavior. Stays the fallback so any incremental-mode
		// regression can be ruled out by clearing the env var.
		//
		// • EDIKT_PHASE_B_INCREMENTAL=1 — reload only the sidecars
		// Phase A actually rewrote. Phase A never adds new pairs to
		// the set: `pairs` is derived from the parent .md files, and
		// a bootstrap task writes a sidecar for a pair that is
		// already a member (it just had a nil Sidecar). So the
		// original `pairs` slice stays authoritative for membership;
		// only contents need refreshing. Reuse-with-incremental-reload
		// is opt-in for v0.6.0 and slated to default on in v0.6.1 once
		// soak data is in.
		if os.Getenv("EDIKT_PHASE_B_INCREMENTAL") == "1" {
			staleIDs := make(map[string]bool, len(tasks))
			for _, t := range tasks {
				staleIDs[t.ArtifactID] = true
			}
			for i := range pairs {
				if !staleIDs[pairs[i].ArtifactID] {
					continue
				}
				sc, lerr := sidecar.Load(pairs[i].SidecarPath)
				if lerr != nil {
					pairs[i].Sidecar = nil
					pairs[i].LoadErr = lerr
					continue
				}
				pairs[i].Sidecar = sc
				pairs[i].LoadErr = nil
			}
		} else {
			pairs, err = sidecar.Discover(opts.ProjectRoot, dirs)
			if err != nil {
				return res, fmt.Errorf("rediscover after phase A: %w", err)
			}
		}
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
				"edikt gov compile --check: anchor drift: 0 stale (%d sidecar(s))\n", len(pairs))
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
			fmt.Fprintln(opts.Stderr, "  Run `bin/edikt gov compile` to re-render.")
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

		// (1) Lossless check
		if mp := pairs[i].Sidecar.MigrationPreserved; len(mp.Directives) > 0 {
			losses := lossless.CheckLosslessAgainstDirectives(mp.Directives, pairs[i].Sidecar)
			if len(losses) > 0 {
				res.LosslessReport = append(res.LosslessReport, LossArtifactRec{
					SidecarPath: pairs[i].SidecarPath,
					Losses:      losses,
				})
			}
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
		if werr := os.WriteFile(pairs[i].SidecarPath, out, 0o644); werr != nil {
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
	}

	// F-115/A3 — a reachability REGRESSION, reported distinctly from the
	// routine tier-3 line above. The line above answers "which topics are
	// skill-only right now" (true on a fresh clone too, nothing wrong). This
	// answers "which topics had write-time delivery a moment ago and lost
	// it because of THIS run" — found live: a topic reassignment left a
	// project's core domain silently unreachable, with nothing warning at
	// compile time; the only existing signal was doctor's own scope check,
	// a separately-invoked, point-in-time snapshot with no notion of "did
	// this compile cause it" and no attribution to which reassignment did.
	if len(pbRes.TopicsNewlyUnreachable) > 0 {
		fmt.Fprintf(summaryOut,
			"⚠ REACHABILITY REGRESSION — %d topic(s) had write-time delivery before this compile and do not after it: %s\n"+
				"  Each had a .claude/rules/governance/<topic>.md file an instant ago; this run removed it because "+
				"every contributing sidecar is now undeclared (no paths:). If an artifact was just reassigned to "+
				"one of these topics, that reassignment is very likely why. Declare paths: on at least one "+
				"contributing sidecar (bin/edikt sidecar approve --kind paths) to restore delivery, or confirm the "+
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
