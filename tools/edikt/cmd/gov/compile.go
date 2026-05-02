package gov

import (
	"fmt"
	"os"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/model"
	"github.com/spf13/cobra"
)

var (
	checkFlag    bool
	jsonFlag     bool
	noWaitFlag   bool
	legacyFlag   bool
)

var compileCmd = &cobra.Command{
	Use:   "compile [project-root]",
	Short: "Compile governance directives from ADRs, invariants, and guidelines",
	Long: `Reads all accepted ADRs, active invariants, and guidelines from the project,
groups directives by topic, and writes compiled rule files to .claude/rules/governance/.

Two-phase mode (default when sidecars are present, per ADR-027 / ADR-028):
  Phase A — conditional resync of stale sidecars via subagent dispatch.
  Phase B — pure deterministic merge over the sidecar set.

Legacy in-body sentinel mode runs automatically when no .edikt.yaml sidecars
exist yet (v0.6.0-dev transition; Phase 6b will replace this with a hard
"migration required" error).

Flags:
  --check     validate only; in two-phase mode refuses on stale sidecars.
  --json      structured JSON output (legacy mode only).
  --no-wait   fail fast instead of waiting on a held compile.lock.
  --legacy    force legacy in-body compile even when sidecars exist.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot := "."
		if len(args) > 0 {
			projectRoot = args[0]
		}

		clk := model.RealClock{}

		useTwoPhase := !legacyFlag && sidecarsPresent(projectRoot)
		if useTwoPhase {
			_, err := govrun.RunTwoPhase(govrun.TwoPhaseOptions{
				ProjectRoot: projectRoot,
				CheckOnly:   checkFlag,
				NoWait:      noWaitFlag,
			}, clk)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return nil
		}

		if err := govrun.Run(projectRoot, checkFlag, jsonFlag, clk); err != nil {
			if !jsonFlag {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			}
			os.Exit(1)
		}
		return nil
	},
}

func sidecarsPresent(root string) bool {
	dirs := []string{
		"docs/architecture/decisions",
		"docs/architecture/invariants",
		"docs/guidelines",
	}
	return sidecar.HasAnySidecar(root, dirs)
}

func init() {
	compileCmd.Flags().BoolVar(&checkFlag, "check", false, "validate only — do not write output files, exit non-zero on errors")
	compileCmd.Flags().BoolVar(&jsonFlag, "json", false, "output structured JSON (legacy mode only)")
	compileCmd.Flags().BoolVar(&noWaitFlag, "no-wait", false, "fail fast on a held compile.lock instead of blocking")
	compileCmd.Flags().BoolVar(&legacyFlag, "legacy", false, "force legacy in-body sentinel compile even when sidecars exist")
	Cmd.AddCommand(compileCmd)
}
