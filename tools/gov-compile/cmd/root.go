package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/diktahq/edikt/tools/gov-compile/internal/govrun"
	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags; falls back to the constant.
const Version = "0.1.0"

// shellName is the bash launcher that handles all non-Go subcommands.
const shellName = "edikt-shell"

var rootCmd = &cobra.Command{
	Use:   "edikt",
	Short: "edikt — governance layer for agentic engineering",
	Long: `edikt governs your architecture and compiles your engineering decisions
into automatic enforcement across every AI agent session.`,
	// Silence cobra's built-in error/usage output on unknown subcommands —
	// we delegate those to edikt-shell instead.
	SilenceErrors: true,
	SilenceUsage:  true,
}

// Execute is the entry point called from main.go.
func Execute() {
	// cobra will match known subcommands (gov, version).
	// Unknown args are caught by the RunE below via the custom unknown-command
	// handler registered in init().
	if err := rootCmd.Execute(); err != nil {
		// If cobra could not dispatch, try shell delegation.
		// This handles the case where cobra itself returns an error for an
		// unknown command (shouldn't happen because we set a catch-all, but
		// kept as a safety net).
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Propagate the canonical version into the govrun package so compiled
	// output carries the correct version string.
	govrun.CompilerVersion = Version

	// Catch-all: any arg that doesn't match a registered subcommand is delegated
	// to edikt-shell.  cobra calls this when no other command matches.
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Re-assemble the full argv (subcommand + args).
		return delegateToShell(os.Args[1:])
	}

	// Allow arbitrary trailing args so cobra doesn't reject them before we
	// can delegate.
	rootCmd.Args = cobra.ArbitraryArgs
}

// resolveShellPath walks ancestor directories for bin/edikt-shell, then falls
// back to $EDIKT_ROOT/bin/edikt-shell, then ~/.edikt/bin/edikt-shell.
func resolveShellPath() (string, error) {
	// 1. Explicit env override.
	if root := os.Getenv("EDIKT_ROOT"); root != "" {
		p := filepath.Join(root, "bin", shellName)
		if isExec(p) {
			return p, nil
		}
		return "", fmt.Errorf("edikt-shell not found at %s (EDIKT_ROOT is set)", p)
	}

	// 2. Walk ancestors for bin/edikt-shell (project-mode).
	dir, _ := os.Getwd()
	home := os.Getenv("HOME")
	for dir != "" && dir != "/" && dir != home {
		p := filepath.Join(dir, "bin", shellName)
		if isExec(p) {
			return p, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// 3. Global fallback.
	if home != "" {
		p := filepath.Join(home, ".edikt", "bin", shellName)
		if isExec(p) {
			return p, nil
		}
		return "", fmt.Errorf("edikt-shell not found at %s — run `edikt install` to set up", p)
	}

	return "", fmt.Errorf("edikt-shell not found — run `edikt install` to set up")
}

func isExec(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

// delegateToShell execs edikt-shell with the given argv, inheriting
// stdin/stdout/stderr so interactive prompts work transparently.
func delegateToShell(args []string) error {
	shellPath, err := resolveShellPath()
	if err != nil {
		return err
	}

	// Set EDIKT_SHELL_CALLER=1 so edikt-shell can skip its own root resolution
	// on re-entry (avoids double-launch loops).
	env := append(os.Environ(), "EDIKT_SHELL_CALLER=1")

	c := exec.Command(shellPath, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = env

	if err := c.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}
