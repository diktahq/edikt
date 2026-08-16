package gov

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/diktahq/edikt/tools/edikt/internal/phaseb"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/internal/trust"
	"github.com/diktahq/edikt/tools/edikt/model"
	"github.com/spf13/cobra"
)

var (
	checkFlag      bool
	dryRunFlag     bool
	jsonFlag       bool
	noWaitFlag     bool
	onLossFlag     string
	legacyFlag     bool
	skipVerifyFlag bool
	trustFlag      bool
)

var compileCmd = &cobra.Command{
	Use:   "compile [project-root]",
	Short: "Compile governance directives from ADRs, invariants, and guidelines",
	Long: `Reads all accepted ADRs, active invariants, and guidelines from the project,
groups directives by topic, and writes compiled rule files to .claude/rules/governance/.

Two-phase mode (default):
  Phase A — conditional resync of stale sidecars via subagent dispatch.
  Phase B — pure deterministic merge over the sidecar set.

Two states look alike — governance .md present, no .edikt.yaml sidecars —
and are handled differently:

  Pre-migration      legacy in-body sentinel blocks are present. Rejected
                     with an actionable error directing the user to
                     ` + "`edikt migrate sidecars`" + ` to lift them into
                     co-located sidecars.
  Never-initialised  no sentinels anywhere: the artifacts predate edikt,
                     so there is nothing to migrate. Phase A bootstraps a
                     sidecar for each artifact by extracting from its
                     prose. --check and EDIKT_HEADLESS=1 cannot dispatch,
                     so they report the artifacts and exit non-zero.

Flags:
  --check     validate only; in two-phase mode refuses on stale sidecars.
  --dry-run   alias for --check; aligned with migrate sidecars / verify
              flag conventions.
  --json      structured JSON output. In two-phase mode emits a single
              JSON object with phase_a / phase_b summaries; in legacy
              mode emits the existing legacy report shape.
  --no-wait   fail fast instead of waiting on a held compile.lock.
  --legacy    DEPRECATED — force legacy in-body compile even when sidecars
              exist. Slated for removal in v0.7.0; preserved for the
              v0.5.x→v0.6.0 transition window only.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot := "."
		if len(args) > 0 {
			projectRoot = args[0]
		}

		// --dry-run is an alias for --check. Either flag enables check-only
		// mode; passing both is a no-op (both set the same internal state).
		if dryRunFlag {
			checkFlag = true
		}

		clk := model.RealClock{}

		// Forced legacy path: only when --legacy is explicit. v0.6.0+ never
		// falls back to in-body sentinel parsing — the only path to legacy
		// is opt-in.
		if legacyFlag {
			if err := govrun.Run(projectRoot, checkFlag, jsonFlag, clk); err != nil {
				if !jsonFlag {
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
				}
				os.Exit(1)
			}
			return nil
		}

		// Resolve paths from .edikt/config.yaml — NEVER hardcode defaults
		// at this call site. Hardcoding silently no-ops projects with
		// customized paths.* (regression that shipped in v0.6.0-rc≤7).
		dirs := govrun.GovernanceDirs(projectRoot)
		hasSidecars := sidecar.HasAnySidecar(projectRoot, dirs)
		hasMarkdown := sidecar.HasAnyGovernanceMarkdown(projectRoot, dirs)

		// Governance .md exists but no sidecars yet. Two distinct states
		// hide behind that shape, and they need opposite remedies:
		//
		//   PRE-MIGRATION      legacy in-body sentinels present. `migrate
		//                      sidecars` lifts them. Hard refusal —
		//                      NEVER fall back to in-body parsing.
		//   NEVER-INITIALISED  no sentinels anywhere: the artifacts predate
		//                      edikt. Nothing to migrate. The sidecars must
		//                      be extracted from prose, which is exactly
		//                      what Phase A dispatch does.
		//
		// Before this split, both states got the pre-migration refusal, so
		// a project adopting edikt on existing ADRs could not reach a
		// compiled state by any documented path: compile said "run
		// migrate", migrate reported "0 sidecars to create" and exited 0,
		// and compile still refused.
		if !hasSidecars && hasMarkdown {
			// PRE-MIGRATION vs NEVER-INITIALISED. Both present as "markdown
			// but no sidecars"; only the first has anything to migrate.
			// Removed in bc5bbd6 while sidecar.HasAnyLegacySentinel was
			// uncommitted; restored now that discover.go has landed, which
			// was the trigger that commit named.
			if sidecar.HasAnyLegacySentinel(projectRoot, dirs) {
				fmt.Fprintln(os.Stderr, preMigrationError())
				os.Exit(1)
			}

			// Neither --check (ADR-028: never dispatch a subagent) nor a  // edikt-guard:allow
			// headless run (commands/gov/compile.md §12) can bootstrap.
			// Say so, name the remedy, exit non-zero.
			if checkFlag || os.Getenv("EDIKT_HEADLESS") == "1" {
				if !jsonFlag {
					fmt.Fprintln(os.Stderr, neverInitialisedCheckError())
				}
				os.Exit(1)
			}
			if !jsonFlag {
				fmt.Fprintln(os.Stderr, "edikt gov compile: no sidecars found and no legacy sentinels to migrate —")
				fmt.Fprintln(os.Stderr, "  bootstrapping sidecars from prose via the per-artifact extractor.")
			}
		}

		// Empty project: no governance .md at all. Compile is a no-op,
		// but emit a hint listing the paths that were checked so a
		// misconfigured paths.* in .edikt/config.yaml doesn't silently
		// no-op (regression from rc≤7 where this branch was silent).
		if !hasSidecars && !hasMarkdown {
			if !jsonFlag {
				fmt.Fprintln(os.Stderr, "edikt gov compile: no governance .md or .edikt.yaml sidecars found at:")
				for _, d := range dirs {
					if d == "" {
						continue
					}
					fmt.Fprintf(os.Stderr, "  - %s\n", d)
				}
				fmt.Fprintln(os.Stderr, "If you customized paths.* in .edikt/config.yaml, verify they match this list.")
			}
			return nil
		}

		// Trust gate (ADR-041): the post-compile verify step executes this  // edikt-guard:allow
		// repo's verify: shell commands. The posture (warn / block / disabled)
		// is decided by internal/trust.Evaluate. --check / --json / --skip-verify
		// never run the gate, so they never trigger it. --trust always records.
		if trustFlag {
			if rerr := trust.Record(projectRoot); rerr != nil {
				fmt.Fprintf(os.Stderr, "error: could not record trust for %s: %v\n", trust.Realpath(projectRoot), rerr)
				os.Exit(1)
			}
		}
		if !checkFlag && !jsonFlag && !skipVerifyFlag {
			switch decision, tmsg := trust.Evaluate(projectRoot, trustFlag); decision {
			case trust.Refuse:
				// Compile-specific refusal — also names --skip-verify (compile
				// can still produce rules without running the gate).
				fmt.Fprintln(os.Stderr, untrustedCompileMessage(projectRoot))
				os.Exit(4)
			case trust.ProceedWithWarning:
				fmt.Fprintln(os.Stderr, tmsg)
			}
		}

		// Sidecar-present: two-phase mode.
		res, err := govrun.RunTwoPhase(govrun.TwoPhaseOptions{
			ProjectRoot: projectRoot,
			CheckOnly:   checkFlag,
			NoWait:      noWaitFlag,
			JSONMode:    jsonFlag,
			OnLoss:      onLossFlag,
		}, clk)
		if jsonFlag {
			emitTwoPhaseJSON(res, err)
		}
		if err != nil {
			if !jsonFlag {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			}
			os.Exit(1)
		}

		// Completion-evidence gate: after a successful Phase B, walk every
		// sidecar and run its verify: commands. Failures here mean the
		// merge wrote bytes whose claimed directives don't actually hold,
		// so the gate blocks the success path. Skipped in --check mode
		// (no Phase B ran) and in --json mode (the gate's text output
		// would corrupt the structured payload — JSON consumers can call
		// `edikt verify all --json` themselves).
		if !checkFlag && !jsonFlag && !skipVerifyFlag {
			if rc := runPostCompileVerify(projectRoot); rc != 0 {
				os.Exit(rc)
			}
		}
		return nil
	},
}

// runPostCompileVerify execs `bin/edikt verify all --gov-only` in a subprocess against
// the same binary, scoped to projectRoot. Returns the subprocess exit
// code: 0 = clean, 1 = at least one verify failed, 2 = sidecar loading
// problem, 3 = invocation problem.
//
// Exec-subprocess (rather than an in-process call) is the right shape
// here: it keeps the gate honest (same surface a slash command or e2e
// test would call), avoids cyclic imports between this package and the
// cmd package, and lets the user inspect the gate's stdout independently.
func runPostCompileVerify(projectRoot string) int {
	self, err := os.Executable()
	if err != nil {
		// If we can't even find our own binary, we can't gate — and a
		// gate that did not run must not return the code that means it
		// ran clean. This used to warn and return 0, which the caller
		// (`if rc != 0`) read as a pass: compile exited 0 announcing
		// verified output nothing had verified. Exit 3 is the code this
		// function's own contract already reserves for an invocation
		// problem (INV-011: absence of output is never evidence of
		// completion).
		fmt.Fprintf(os.Stderr,
			"error: post-compile verify gate could not run — could not resolve edikt binary path: %v\n", err)
		return 3
	}
	// --gov-only: the gate guards the governance output it just wrote, so
	// only ADR / INV / guideline verifies participate. prd / spec / plan
	// sidecars routinely carry verifies for deliberately-unbuilt future
	// work; gating compile on those turned every WIP-project compile into
	// an error exit. They keep their own runners (`edikt verify prd|spec|<plan>`).
	cmd := exec.Command(self, "verify", "all", "--gov-only")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			if ee.ExitCode() == 1 {
				fmt.Fprintln(os.Stderr,
					"\nerror: gov compile produced output, but the verify gate found failing sidecar(s).")
				fmt.Fprintln(os.Stderr,
					"Re-run gov compile after fixing the failing directives or removing their verify commands.")
			}
			return ee.ExitCode()
		}
		// Not an ExitError: the subprocess never started, so there is no
		// child exit code to report. A missing binary, an unreadable
		// project root, a permission denial — the gate observed nothing.
		// That is an invocation problem (3), not a pass (0). Reporting 0
		// here let a compile whose gate could not launch exit exactly
		// like one whose gate launched and found every verify holding.
		fmt.Fprintf(os.Stderr, "error: post-compile verify gate could not run: %v\n", err)
		return 3
	}
	return 0
}

// emitTwoPhaseJSON writes the canonical --json shape for two-phase mode
// to stdout. The exit code is the primary contract; this output is the
// secondary contract surface for tier-2 callers (shape documented at
// website/commands/gov/compile.md).
func emitTwoPhaseJSON(res *govrun.TwoPhaseResult, runErr error) {
	type phaseAOut struct {
		Dispatched int `json:"dispatched"`
		Stale      int `json:"stale"`
		// StaleIDs names the stale artifacts, not just how many. Consumers
		// that need to report WHICH sidecars drifted — templates/hooks/
		// stop-hook.sh — would otherwise have to reimplement the staleness
		// predicate to find out, and a second implementation of a rule is
		// how the two answers diverge (it did: the hook's copy reported
		// ADR-026 and ADR-040 stale when this predicate said 0).
		StaleIDs []string `json:"stale_ids"`
		// Bootstrap counts artifacts that had NO sidecar at all, as opposed
		// to one whose anchors drifted. A large value means this is a first
		// compile of a project adopting edikt on pre-existing docs — very
		// different cost from a routine resync, so consumers get to tell
		// the two apart.
		Bootstrap int                     `json:"bootstrap"`
		Errors    []govrun.PhaseAErrorRec `json:"errors"`
	}
	type phaseBOut struct {
		Ran             bool     `json:"ran"`
		TopicsRendered  []string `json:"topics_rendered"`
		TopicsUnchanged []string `json:"topics_unchanged"`
		IndexWritten    bool     `json:"index_written"`
		TotalDirectives int      `json:"total_directives"`
		// Surfaces is the render's own enumeration of what it produced —
		// the answer consumers read INSTEAD of assuming .claude/rules/
		// directory layout. Always an array: [] means Phase B ran and
		// produced nothing, which is different from the key being absent
		// because Phase B never ran (`"ran": false` says that).
		Surfaces []phaseb.Surface `json:"surfaces"`
	}
	out := struct {
		Status string    `json:"status"`
		PhaseA phaseAOut `json:"phase_a"`
		// PhaseB is ALWAYS populated. {"ran": false, ...} when --check mode
		// skipped phase B; {"ran": true, ...} otherwise. Never null — that
		// was the rc≤7 contract violation (consumers couldn't distinguish
		// "phase B produced no changes" from "phase B never ran" from
		// "compile is broken").
		PhaseB phaseBOut `json:"phase_b"`
		// LosslessReport is the per-sidecar list of dropped directives
		// the post-extractor verification gate found. Empty array when
		// no losses (the happy path). Consumers can use this to surface
		// extractor regressions without parsing the prose summary.
		LosslessReport []govrun.LossArtifactRec `json:"lossless_report"`
		Error          string                   `json:"error,omitempty"`
	}{
		Status: "ok",
		PhaseB: phaseBOut{
			TopicsRendered:  []string{},
			TopicsUnchanged: []string{},
			Surfaces:        []phaseb.Surface{},
		},
		LosslessReport: []govrun.LossArtifactRec{},
	}
	if res != nil {
		out.PhaseA.Stale = len(res.StaleSidecars)
		out.PhaseA.StaleIDs = res.StaleSidecars
		if out.PhaseA.StaleIDs == nil {
			out.PhaseA.StaleIDs = []string{}
		}
		out.PhaseA.Bootstrap = len(res.BootstrapSidecars)
		if res.PhaseADone {
			out.PhaseA.Dispatched = len(res.StaleSidecars)
		}
		out.PhaseA.Errors = res.PhaseAErrors
		if out.PhaseA.Errors == nil {
			out.PhaseA.Errors = []govrun.PhaseAErrorRec{}
		}
		if res.PhaseB != nil {
			out.PhaseB = phaseBOut{
				Ran:             true,
				TopicsRendered:  res.PhaseB.TopicsRendered,
				TopicsUnchanged: res.PhaseB.TopicsUnchanged,
				IndexWritten:    res.PhaseB.IndexWritten,
				TotalDirectives: res.PhaseB.TotalDirectives,
				Surfaces:        res.PhaseB.Surfaces,
			}
			if out.PhaseB.Surfaces == nil {
				out.PhaseB.Surfaces = []phaseb.Surface{}
			}
			if out.PhaseB.TopicsRendered == nil {
				out.PhaseB.TopicsRendered = []string{}
			}
			if out.PhaseB.TopicsUnchanged == nil {
				out.PhaseB.TopicsUnchanged = []string{}
			}
		}
		if len(res.LosslessReport) > 0 {
			out.LosslessReport = res.LosslessReport
		}
	}
	if runErr != nil {
		out.Status = "error"
		out.Error = runErr.Error()
	}
	body, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(os.Stdout, string(body))
}

// preMigrationError returns the canonical refusal string emitted when a
// project still has legacy in-body sentinels and no co-located sidecars.
// The migration guide URL is tag-pinned to v0.6.0 — the release that
// introduces the hard requirement.
func preMigrationError() string {
	return `error: pre-migration project state — v0.6.0 requires co-located .edikt.yaml sidecars.

  In Claude Code:  /edikt:upgrade   (runs the full v0.4→v0.6 migration flow)

See https://github.com/diktahq/edikt/blob/v0.6.0/website/guides/sidecar-migration.md for details.`
}

// neverInitialisedCheckError is the actionable refusal for a project whose
// governance artifacts predate edikt entirely — no sidecars, and no legacy
// in-body sentinels to migrate. `migrate sidecars` is deliberately NOT
// offered here: it has nothing to lift and would exit 0 without doing
// anything, which is the loop this message exists to break.
//
// Only --check and headless runs reach this. An interactive run bootstraps
// the sidecars itself via Phase A dispatch.
func neverInitialisedCheckError() string {
	return `error: no .edikt.yaml sidecars — this project's governance artifacts have never been compiled.

There are no legacy in-body sentinels, so there is nothing for ` + "`edikt migrate sidecars`" + ` to lift.
Sidecars must be extracted from the artifact prose:

  In Claude Code:  /edikt:gov:compile   (bootstraps every missing sidecar, then merges)
  Per artifact:    /edikt:adr:compile / /edikt:invariant:compile / /edikt:guideline:compile

--check and EDIKT_HEADLESS=1 never dispatch a subagent, so neither can bootstrap.
Run compile interactively, without --check, to generate the sidecars.`
}

// untrustedCompileMessage is the actionable refusal printed when gov compile
// would run the post-compile verify gate in a project the user has not
// approved (ADR-041). It offers both the approve path and the no-gate path.  // edikt-guard:allow
func untrustedCompileMessage(projectRoot string) string {
	return fmt.Sprintf(`error: %s is not an approved edikt project — refusing to run the post-compile verify gate.

The verify gate executes arbitrary shell from this repo's sidecar verify: fields. To approve this repo once:
  bin/edikt gov compile --trust

To compile without running the gate (no repo shell executed):
  bin/edikt gov compile --skip-verify

For CI or a one-off run, set EDIKT_VERIFY_TRUST=1.`, trust.Realpath(projectRoot))
}

func init() {
	compileCmd.Flags().BoolVar(&checkFlag, "check", false, "validate only — do not write output files, exit non-zero on errors")
	compileCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "alias for --check (aligned with migrate sidecars / verify flag conventions)")
	compileCmd.Flags().BoolVar(&jsonFlag, "json", false, "output structured JSON (phase_a / phase_b summary in two-phase mode)")
	compileCmd.Flags().BoolVar(&noWaitFlag, "no-wait", false, "fail fast on a held compile.lock instead of blocking")
	compileCmd.Flags().BoolVar(&legacyFlag, "legacy", false, "force legacy in-body sentinel compile even when sidecars exist")
	compileCmd.Flags().BoolVar(&skipVerifyFlag, "skip-verify", false,
		"skip the post-compile verify gate (use sparingly — disabling the gate "+
			"defeats the completion-evidence discipline)")
	compileCmd.Flags().BoolVar(&trustFlag, "trust", false,
		"approve this project to run its repo-defined verify: commands and record "+
			"it in ~/.edikt/state")
	compileCmd.Flags().StringVar(&onLossFlag, "on-loss", "auto",
		`policy when Phase A's post-extractor lossless check finds directives dropped from MigrationPreserved:
"abort"  — exit non-zero with per-sidecar report (recommended for CI)
"accept" — warn and continue (explicit opt-in to silent loss)
"auto"   — abort if stdin is not a TTY, accept otherwise (default)`)
	Cmd.AddCommand(compileCmd)
}
