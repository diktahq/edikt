package gov

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/model"
	"github.com/spf13/cobra"
)

var (
	checkFlag    bool
	dryRunFlag   bool
	jsonFlag     bool
	noWaitFlag   bool
	legacyFlag   bool
)

var compileCmd = &cobra.Command{
	Use:   "compile [project-root]",
	Short: "Compile governance directives from ADRs, invariants, and guidelines",
	Long: `Reads all accepted ADRs, active invariants, and guidelines from the project,
groups directives by topic, and writes compiled rule files to .claude/rules/governance/.

Two-phase mode (default per ADR-027 / ADR-028):
  Phase A — conditional resync of stale sidecars via subagent dispatch.
  Phase B — pure deterministic merge over the sidecar set.

A pre-migration project (governance .md present but no .edikt.yaml
sidecars) is rejected with a single-line actionable error per ADR-027
§5. The user is directed to ` + "`edikt migrate sidecars`" + ` to lift legacy
in-body sentinel blocks into co-located sidecars.

Flags:
  --check     validate only; in two-phase mode refuses on stale sidecars.
  --dry-run   alias for --check; aligned with migrate sidecars / verify
              flag conventions (ADR-029 / Phase 5 of PLAN-sidecar-review-fixes).
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

		// Forced legacy path: only when --legacy is explicit. This honors
		// ADR-027 §5's "NEVER fall back to in-body sentinel parsing" — the
		// auto-fallback is gone; the only path to legacy is opt-in.
		if legacyFlag {
			if err := govrun.Run(projectRoot, checkFlag, jsonFlag, clk); err != nil {
				if !jsonFlag {
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
				}
				os.Exit(1)
			}
			return nil
		}

		dirs := governanceDirs()
		hasSidecars := sidecar.HasAnySidecar(projectRoot, dirs)
		hasMarkdown := sidecar.HasAnyGovernanceMarkdown(projectRoot, dirs)

		// Pre-migration: governance .md exists but no sidecars yet. ADR-027 §5
		// requires a hard, actionable refusal. NEVER fall back to in-body
		// parsing.
		if !hasSidecars && hasMarkdown {
			fmt.Fprintln(os.Stderr, preMigrationError())
			os.Exit(1)
		}

		// Empty project: no governance .md at all. Compile is a no-op.
		if !hasSidecars && !hasMarkdown {
			return nil
		}

		// Sidecar-present: two-phase mode (ADR-028).
		res, err := govrun.RunTwoPhase(govrun.TwoPhaseOptions{
			ProjectRoot: projectRoot,
			CheckOnly:   checkFlag,
			NoWait:      noWaitFlag,
			JSONMode:    jsonFlag,
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
		return nil
	},
}

// emitTwoPhaseJSON writes the canonical --json shape for two-phase mode
// to stdout (ADR-029 §"exit-code-only contract" — output is the contract
// surface for tier-2 callers; the shape is documented in
// website/commands/gov/compile.md).
func emitTwoPhaseJSON(res *govrun.TwoPhaseResult, runErr error) {
	type phaseAOut struct {
		Dispatched int                      `json:"dispatched"`
		Stale      int                      `json:"stale"`
		Errors     []govrun.PhaseAErrorRec  `json:"errors"`
	}
	type phaseBOut struct {
		TopicsRendered  []string `json:"topics_rendered"`
		TopicsUnchanged []string `json:"topics_unchanged"`
		IndexWritten    bool     `json:"index_written"`
		TotalDirectives int      `json:"total_directives"`
	}
	out := struct {
		Status string     `json:"status"`
		PhaseA phaseAOut  `json:"phase_a"`
		PhaseB *phaseBOut `json:"phase_b,omitempty"`
		Error  string     `json:"error,omitempty"`
	}{
		Status: "ok",
	}
	if res != nil {
		out.PhaseA.Stale = len(res.StaleSidecars)
		if res.PhaseADone {
			out.PhaseA.Dispatched = len(res.StaleSidecars)
		}
		out.PhaseA.Errors = res.PhaseAErrors
		if out.PhaseA.Errors == nil {
			out.PhaseA.Errors = []govrun.PhaseAErrorRec{}
		}
		if res.PhaseB != nil {
			out.PhaseB = &phaseBOut{
				TopicsRendered:  res.PhaseB.TopicsRendered,
				TopicsUnchanged: res.PhaseB.TopicsUnchanged,
				IndexWritten:    res.PhaseB.IndexWritten,
				TotalDirectives: res.PhaseB.TotalDirectives,
			}
		}
	}
	if runErr != nil {
		out.Status = "error"
		out.Error = runErr.Error()
	}
	body, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(os.Stdout, string(body))
}

func governanceDirs() []string {
	return []string{
		"docs/architecture/decisions",
		"docs/architecture/invariants",
		"docs/guidelines",
	}
}

// preMigrationError returns the canonical ADR-027 §5 error string. The
// migration guide URL is tag-pinned per INV-008 to v0.6.0 — the release
// that introduces the hard requirement.
func preMigrationError() string {
	return `error: pre-migration project state — run 'edikt migrate sidecars --dry-run' followed by 'edikt migrate sidecars --apply' to generate the .edikt.yaml sidecars required by v0.6.0 compile.
See https://github.com/diktahq/edikt/blob/v0.6.0/website/guides/sidecar-migration.md for details.`
}

func init() {
	compileCmd.Flags().BoolVar(&checkFlag, "check", false, "validate only — do not write output files, exit non-zero on errors")
	compileCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "alias for --check (aligned with migrate sidecars / verify flag conventions)")
	compileCmd.Flags().BoolVar(&jsonFlag, "json", false, "output structured JSON (phase_a / phase_b summary in two-phase mode)")
	compileCmd.Flags().BoolVar(&noWaitFlag, "no-wait", false, "fail fast on a held compile.lock instead of blocking")
	compileCmd.Flags().BoolVar(&legacyFlag, "legacy", false, "force legacy in-body sentinel compile even when sidecars exist")
	Cmd.AddCommand(compileCmd)
}
