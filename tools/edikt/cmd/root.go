package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/spf13/cobra"
)

// exitCodeError carries a specific process exit code distinct from cobra's
// default exit-1-on-error. Commands that need exit codes 2, 3, 5 etc. return
// this type so Execute() can call os.Exit with the right code.
type exitCodeError struct {
	code int
	msg  string
}

func (e *exitCodeError) Error() string { return e.msg }

// Version is set at build time via ldflags; falls back to the constant.
const Version = "0.6.0-rc3"

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
		var ece *exitCodeError
		if errors.As(err, &ece) {
			if ece.msg != "" {
				fmt.Fprintf(os.Stderr, "error: %v\n", ece.msg)
			}
			os.Exit(ece.code)
		}
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

	// PersistentPreRunE: lock acquisition for mutating commands + pin warning.
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		name := cmd.Name()

		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return nil // can't resolve root — let the command handle it
		}

		// Acquire lock for mutating commands.
		if mutatingCommands[name] {
			_, unlock, lerr := acquireLock(ediktRoot)
			if lerr != nil {
				return lerr
			}
			// unlock is called at process exit (fd held open via cmd scope).
			_ = unlock
		}

		// Pin warning for non-exempt commands.
		if !pinWarnExempt(name) {
			emitPinWarn(ediktRoot)
		}
		return nil
	}
}
