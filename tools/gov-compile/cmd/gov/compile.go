package gov

import (
	"fmt"
	"os"

	"github.com/diktahq/edikt/tools/gov-compile/internal/govrun"
	"github.com/diktahq/edikt/tools/gov-compile/model"
	"github.com/spf13/cobra"
)

var (
	checkFlag bool
	jsonFlag  bool
)

var compileCmd = &cobra.Command{
	Use:   "compile [project-root]",
	Short: "Compile governance directives from ADRs, invariants, and guidelines",
	Long: `Reads all accepted ADRs, active invariants, and guidelines from the project,
groups directives by topic, and writes compiled rule files to .claude/rules/governance/.

Pass --check to validate only (no writes, exits non-zero on errors).
Pass --json for machine-readable output.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot := "."
		if len(args) > 0 {
			projectRoot = args[0]
		}

		clk := model.RealClock{}
		if err := govrun.Run(projectRoot, checkFlag, jsonFlag, clk); err != nil {
			if !jsonFlag {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			}
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	compileCmd.Flags().BoolVar(&checkFlag, "check", false, "validate only — do not write output files, exit non-zero on errors")
	compileCmd.Flags().BoolVar(&jsonFlag, "json", false, "output structured JSON, no prose or progress lines")
	Cmd.AddCommand(compileCmd)
}

