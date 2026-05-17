package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

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

// Version is set at build time via ldflags:
//   go build -ldflags "-X 'github.com/diktahq/edikt/tools/edikt/cmd.Version=$(cat ../../VERSION)'" .
//
// The "dev" fallback is intentional: a plain `go build .` produces a binary
// stamped "dev", signaling to the user that they built from source without
// version injection. Never hardcode a version string here — that creates
// drift between the binary's --version output and the VERSION file
// (regression that shipped rc4-stamped binaries through rc5/6/7).
//
// `var` (not `const`) is required for `-X` ldflag injection to work.
var Version = "dev"

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
	// output carries the correct version string. Strip the leading "v"
	// because the render templates already prepend "v" — letting Version
	// pass through unchanged produced "vv0.6.0-rcN" stamps when the
	// ldflag value followed git-tag convention (with "v"), conflicting
	// with the historical CompilerVersion default of "0.1.0" (no "v").
	govrun.CompilerVersion = strings.TrimPrefix(Version, "v")

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

		// Pin warning for non-exempt commands. Walk the parent chain so
		// subcommands (e.g. `gov compile-history`) inherit the parent's
		// exemption — the policy is "any command under `gov` is exempt"
		// not "only the literal `gov` leaf".
		exempt := false
		for c := cmd; c != nil; c = c.Parent() {
			if pinWarnExempt(c.Name()) {
				exempt = true
				break
			}
		}
		if !exempt {
			emitPinWarn(ediktRoot)
		}
		return nil
	}
}
