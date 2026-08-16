package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/spf13/cobra"
)

// Resolution chains as DATA, not prose.
//
// This exists because a guard kept its own copy of a chain and the chain grew
// underneath it. test/integration/install/_lib.sh sandboxed HOME and unset
// CLAUDE_HOME, on the belief that resolution then fell through to $HOME —
// which was true when the chain had two levels. CLAUDE_CONFIG_DIR was added
// between them, so resolution fell through to the DEVELOPER'S REAL PROFILE
// instead, and `edikt use` wrote a commands symlink into a live home
// directory pointing at a temp dir that was deleted moments later.
//
// The product change was correct. The defect was that the guard's copy of the
// precedence order and the resolver's actual order were two implementations
// of one rule, and only one of them was updated. Same shape as the
// doctor/compile skip predicate and the stop-hook's staleness copy.
//
// So the resolver publishes its chain and every consumer reads it. A new
// variable extends the guard automatically. A consumer that encounters a
// chain entry it has no pin for MUST fail loudly rather than proceed — a
// guard silently ignoring an input it does not recognise is a control
// observing less than it claims (INV-013).

// ChainStep is one rung of a resolution chain, in precedence order.
type ChainStep struct {
	// Env is the environment variable consulted, or "" for a derived step
	// (e.g. a filesystem walk, or a hardcoded default).
	Env string `json:"env"`
	// Kind is "env", "walk", or "default".
	Kind string `json:"kind"`
	// Detail describes a non-env step in one line.
	Detail string `json:"detail"`

	// Input NAMES what the rung reads when it is not an environment
	// variable — "cwd" for the ancestor walk. A guard cannot control what it
	// cannot name, and publishing only the variables left cwd invisible: the
	// walk's input was the process's working directory the whole time, and
	// the chain described it as "(walk)" without saying so.
	Input string `json:"input"`

	// Containment tells a guard HOW to constrain this rung. Some inputs are
	// pinned (set the variable); some can only be checked after the fact
	// (assert the resolved result lands inside the boundary). Without this a
	// guard has no strategy for a non-env rung and silently skips it.
	Containment string `json:"containment"`
}

// claudeRootChain mirrors resolveClaudeRoot, in order. Keep them in lockstep:
// TestResolutionChainMatchesResolvers fails if they drift.
var claudeRootChain = []ChainStep{
	{Env: "CLAUDE_HOME", Kind: "env", Detail: "edikt's explicit override",
		Input: "CLAUDE_HOME", Containment: "pin: set to a path inside the boundary"},
	{Env: "CLAUDE_CONFIG_DIR", Kind: "env", Detail: "Claude Code's own profile selector",
		Input: "CLAUDE_CONFIG_DIR", Containment: "pin: set to a path inside the boundary"},
	{Env: "HOME", Kind: "default", Detail: "$HOME/.claude",
		Input: "HOME", Containment: "pin: set to a path inside the boundary"},
}

// ediktRootChain mirrors resolveEdiktRoot, in order.
var ediktRootChain = []ChainStep{
	{Env: "EDIKT_ROOT", Kind: "env", Detail: "explicit override",
		Input: "EDIKT_ROOT", Containment: "pin: set to a path inside the boundary"},
	{Env: "", Kind: "walk", Detail: "ancestor walk for .edikt/bin/edikt, bounded at the innermost project boundary",
		Input:       "cwd",
		Containment: "assert-result: cwd cannot be pinned to a boundary the process must work outside of — assert the RESOLVED root lands inside instead (edikt debug resolved-roots)"},
	{Env: "EDIKT_HOME", Kind: "env", Detail: "install-root override",
		Input: "EDIKT_HOME", Containment: "pin: set to a path inside the boundary"},
	{Env: "HOME", Kind: "default", Detail: "$HOME/.edikt",
		Input: "HOME", Containment: "pin: set to a path inside the boundary"},
}

// ResolutionChains is the full published set, keyed by what it resolves.
func ResolutionChains() map[string][]ChainStep {
	return map[string][]ChainStep{
		"claude_root": claudeRootChain,
		"edikt_root":  ediktRootChain,
	}
}

// resolutionChainCmd emits the chains so non-Go consumers — the install test
// harness's sandbox guard, chiefly — can pin every input without hardcoding a
// list that will silently fall behind.
var resolutionChainCmd = &cobra.Command{
	Use:    "resolution-chain",
	Short:  "Print the environment-variable precedence chains used for path resolution",
	Hidden: true,
	Long: `Print the precedence chains resolveClaudeRoot and resolveEdiktRoot use.

Consumers that sandbox or guard resolution MUST read this rather than keeping
their own copy: a guard whose list is a proper subset of the live chain has a
silent hole exactly where the newest variable is.

Formats:
  --format=env     one VAR per line, all env-backed steps across both chains
  --format=inputs  every input a guard must handle (pinnable or not), TAB its
                   containment strategy — read this, not env, when building a
                   sandbox guard
  --format=table   human-readable, in precedence order`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		switch resolutionChainFormat {
		case "env":
			seen := map[string]bool{}
			for _, name := range []string{"claude_root", "edikt_root"} {
				for _, step := range ResolutionChains()[name] {
					if step.Env != "" && !seen[step.Env] {
						seen[step.Env] = true
						fmt.Fprintln(out, step.Env)
					}
				}
			}
		case "inputs":
			// Everything a guard must handle, pinnable or not — the env
			// format alone left cwd invisible, which is how a guard that
			// pinned every variable it knew about still escaped.
			seen := map[string]bool{}
			for _, name := range []string{"claude_root", "edikt_root"} {
				for _, step := range ResolutionChains()[name] {
					if step.Input == "" || seen[step.Input] {
						continue
					}
					seen[step.Input] = true
					fmt.Fprintf(out, "%s\t%s\n", step.Input, step.Containment)
				}
			}
		case "table":
			for _, name := range []string{"claude_root", "edikt_root"} {
				fmt.Fprintf(out, "%s:\n", name)
				for i, step := range ResolutionChains()[name] {
					label := step.Env
					if label == "" {
						label = "(" + step.Kind + ")"
					}
					fmt.Fprintf(out, "  %d. %-20s %s\n", i+1, label, step.Detail)
				}
			}
		default:
			return &exitCodeError{code: 3, msg: fmt.Sprintf(
				"resolution-chain: unknown --format %q (want env|inputs|table)", resolutionChainFormat)}
		}
		return nil
	},
}

var resolutionChainFormat string

// resolveClaudeRootVia resolves using the published chain, so the chain is not
// merely documentation of the resolver — it IS the resolver's order. Any drift
// between this and resolveClaudeRoot is a test failure, not a comment rot.
func resolveClaudeRootVia(chain []ChainStep) string {
	for _, step := range chain {
		switch step.Kind {
		case "env":
			if v := os.Getenv(step.Env); v != "" {
				return v
			}
		case "default":
			if v := os.Getenv(step.Env); v != "" {
				return filepath.Join(v, strings.TrimPrefix(step.Detail, "$HOME/"))
			}
		}
	}
	return filepath.Join("/", ".claude")
}

// resolvedRootsCmd prints what resolution ACTUALLY produced, so a guard has an
// outcome to assert and no excuse to model one (INV-014).
//
// The chain command publishes the inputs; this publishes the results. A guard
// that reads only the inputs is a model of resolution and will diverge from
// it — which is how a sandbox that pinned every variable it knew about still
// wrote into a real home directory.
var resolvedRootsCmd = &cobra.Command{
	Use:    "resolved-roots",
	Short:  "Print the paths resolution actually produced",
	Hidden: true,
	Long: `Print the resolved edikt root and Claude root.

Guards MUST assert these values rather than reconstructing them from
environment variables. Reading inputs answers "given my model, where should
this land?"; reading outputs answers "where did it land?" — and only the
second is a boundary check.

Output is KEY=VALUE, one per line, on stdout.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			// Report the failure rather than printing a blank that a caller
			// would read as "resolved to empty" (INV-013).
			fmt.Fprintf(out, "EDIKT_ROOT_ERROR=%s\n", err.Error())
		} else {
			fmt.Fprintf(out, "EDIKT_ROOT=%s\n", ediktRoot)
		}
		fmt.Fprintf(out, "CLAUDE_ROOT=%s\n", resolveClaudeRoot())
		return nil
	},
}

// skipListedCmd exposes the SHARED "does this artifact need a sidecar?"
// predicate to non-Go consumers — shell test suites, chiefly.
//
// Without it, a shell test asserting sidecar presence has to reimplement the
// rule, and that is precisely the divergence that put doctor and compile at
// odds this week: doctor demanded sidecars for retired artifacts while
// compile correctly skipped them. One predicate, consumed everywhere; a third
// copy in bash would be the same defect in a new language.
//
// Exit 0 = skip-listed (no sidecar required), 1 = a sidecar is required.
var skipListedCmd = &cobra.Command{
	Use:          "skip-listed <path-to-artifact.md>",
	Short:        "Exit 0 if the artifact is skip-listed (needs no sidecar)",
	Hidden:       true,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		skip, reason := sidecar.IsSkipListed(args[0])
		if skip {
			fmt.Fprintf(cmd.OutOrStdout(), "skip: %s\n", reason)
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), "required: a sidecar is expected for this artifact")
		return &exitCodeError{code: 1, msg: ""}
	},
}

// debugCmd groups introspection verbs that exist for tooling and tests rather
// than for users. Hidden from help; not part of the tier-1 permit surface.
var debugCmd = &cobra.Command{
	Use:    "debug",
	Short:  "Introspection helpers for tooling and tests",
	Hidden: true,
}

func init() {
	resolutionChainCmd.Flags().StringVar(&resolutionChainFormat, "format", "table",
		"output format: env|inputs|table")
	debugCmd.AddCommand(resolutionChainCmd)
	debugCmd.AddCommand(resolvedRootsCmd)
	debugCmd.AddCommand(skipListedCmd)
	rootCmd.AddCommand(debugCmd)
}
