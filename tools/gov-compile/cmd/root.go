package cmd

import (
	"fmt"
	"os"

	"github.com/diktahq/edikt/tools/gov-compile/internal/govrun"
	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags; falls back to the constant.
const Version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:   "edikt",
	Short: "edikt — governance layer for agentic engineering",
	Long: `edikt governs your architecture and compiles your engineering decisions
into automatic enforcement across every AI agent session.`,
	SilenceErrors: true,
	SilenceUsage:  true,
}

// Execute is the entry point called from main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Propagate the canonical version into the govrun package so compiled
	// output carries the correct version string.
	govrun.CompilerVersion = Version

	// Unknown subcommand: print error and usage.
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(os.Args) > 1 {
			fmt.Fprintf(os.Stderr, "error: unknown subcommand: %s\n", os.Args[1])
		}
		return cmd.Help()
	}

	// Allow arbitrary trailing args so cobra doesn't reject them before RunE.
	rootCmd.Args = cobra.ArbitraryArgs
}
