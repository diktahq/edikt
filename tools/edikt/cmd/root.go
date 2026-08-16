package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/diktahq/edikt/tools/edikt/internal/verify"
	"github.com/spf13/cobra"
)

// isVerifyCommand reports whether cmd is `edikt verify` or any subcommand of
// it (all / gov / prd / spec). Walks the parent chain rather than matching the
// leaf name, so a future `verify <something>` is covered without edits.
func isVerifyCommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "verify" {
			return true
		}
	}
	return false
}

// exitCodeError carries a specific process exit code distinct from cobra's
// default exit-1-on-error. Commands that need exit codes 2, 3, 5 etc. return
// this type so Execute() can call os.Exit with the right code.
type exitCodeError struct {
	code int
	msg  string
}

func (e *exitCodeError) Error() string { return e.msg }

// Version is set at build time via ldflags:
// go build -ldflags "-X 'github.com/diktahq/edikt/tools/edikt/cmd.Version=$(cat ../../VERSION)'" .
//
// The "dev" fallback is intentional: a plain `go build .` produces a binary
// stamped "dev", signaling to the user that they built from source without
// version injection. Never hardcode a version string here — that creates
// drift between the binary's --version output and the VERSION file
// (regression that shipped rc4-stamped binaries through rc5/6/7).
//
// `var` (not `const`) is required for `-X` ldflag injection to work.
var Version = "dev"

// compilerVersionStamp maps the build's Version into the string stamped
// into every compiled artifact, and reports whether it came from a real
// release. Returns the version with any leading "v" stripped for a
// release; a self-describing marker otherwise.
//
// The stamp used to be `strings.TrimPrefix(Version, "v")` unconditionally,
// so a plain `go build .` produced artifacts marked `gov-compile vdev`.
//
// Refusing to compile without an injected version was the alternative and
// is the wrong trade: `go build . && ./edikt gov compile` is how a
// contributor tests a compile change, and making that fatal would push
// everyone to discover the ldflag that silences it — a gate whose first
// effect is teaching people to bypass it. A dev build should run.
//
// What it must not do is claim provenance it doesn't have. The defect was
// never the word "dev"; it was that "vdev" is SHAPED like a version — same
// field, same leading "v" — so nothing reading an artifact could tell a
// release-built compile from someone's working copy. The stamp is now
// unmistakably not a version, which keeps the loop working and makes the
// artifact honest about what produced it.
func compilerVersionStamp(v string) (stamp string, released bool) {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" || trimmed == "dev" {
		return "unversioned-build (built from source; not a release — provenance unknown)", false
	}
	return strings.TrimPrefix(trimmed, "v"), true
}

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
	govrun.CompilerVersion, _ = compilerVersionStamp(Version)

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

		// Verify re-entry gate. A sidecar `verify:` command is arbitrary
		// shell and may invoke `edikt verify` again; when it addresses the
		// artifact being verified (or two artifacts address each other) the
		// runner re-enters without bound and forks until the process table
		// is exhausted. Refusing here — at the entry point of a nested run,
		// before any sidecar is loaded or any command spawned — is what
		// makes a missed cycle inert rather than fatal.
		//
		// Deliberately NOT a verifyCmd PersistentPreRunE: cobra runs only the
		// closest one in the chain, so defining it there would silently
		// disable this hook's version gate (ADR-042) and lock acquisition for
		// every verify command.
		if isVerifyCommand(cmd) && verify.DepthExceeded() {
			return &exitCodeError{code: 3, msg: fmt.Sprintf(
				"refusing to re-enter the verify runner (depth %d, max %d).\n"+
					"A sidecar verify: command invoked `edikt verify` from inside a verify run.\n"+
					"Self-referential verifies are also circular — verifying X cannot be evidence\n"+
					"that verifying X works. Repoint the verify: at an independent check.",
				verify.CurrentDepth(), verify.MaxDepth)}
		}

		// Version-line gate (ADR-042): refuse to operate on a project that  // edikt-guard:allow
		// predates the current architecture line. Version-management verbs
		// (migrate/upgrade/rollback/…) and read-only commands are exempt.
		if versionGateApplies(cmd) {
			if err := ensureVersionLine(); err != nil {
				return err
			}
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
