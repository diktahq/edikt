package gov

// gitignorebootstrap.go — `bin/edikt gov gitignore-bootstrap` subcommand.
//
// Replaces the ~30-line python heredoc in commands/gov/compile.md
// (Phase 11.5 of PLAN-v060-governance-accuracy). Side-effect-only:
// appends `.edikt/state/` to the project's .gitignore unless an
// equivalent line variant is already present. Trailing-slash variants
// are deduplicated.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/gitignore"
	"github.com/spf13/cobra"
)

var (
	gbProjectRoot string
	gbEntry       string
	gbQuiet       bool
)

var gitignoreBootstrapCmd = &cobra.Command{
	Use:   "gitignore-bootstrap",
	Short: "Ensure .edikt/state/ is .gitignore'd on first compile",
	Long: `Idempotent: appends '.edikt/state/' to the project's
.gitignore unless an equivalent variant is already present (handles
both trailing-slash and stripped-slash forms). Creates .gitignore
when absent.

Per ADR-029 Rule 2 the only contract is the exit code; output is
informational. Per ADR-030 this is pure Go — no LLM dispatch.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := runGitignoreBootstrap(cmd, args)
		exitFromExitErr(err)
		return err
	},
}

func init() {
	gitignoreBootstrapCmd.Flags().StringVar(&gbProjectRoot, "project-root", "",
		"project root (default: cwd; INV-006: validated to exist)")
	gitignoreBootstrapCmd.Flags().StringVar(&gbEntry, "entry", ".edikt/state/",
		"line to ensure (default '.edikt/state/'; allowlist-validated)")
	gitignoreBootstrapCmd.Flags().BoolVar(&gbQuiet, "quiet", false, "suppress prose output")
	Cmd.AddCommand(gitignoreBootstrapCmd)
}

func runGitignoreBootstrap(cmd *cobra.Command, args []string) error {
	root := gbProjectRoot
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return &exitErr{code: 2, msg: fmt.Sprintf("getwd: %v", err)}
		}
	}

	// INV-006: refuse traversal in --project-root and --entry. The default
	// values are safe; user overrides are validated.
	if strings.Contains(gbProjectRoot, "..") || strings.Contains(gbEntry, "..") {
		return &exitErr{code: 2, msg: "--project-root and --entry must not contain '..'"}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return &exitErr{code: 2, msg: fmt.Sprintf("resolve project root: %v", err)}
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return &exitErr{code: 2, msg: fmt.Sprintf("project root not a directory: %s", abs)}
	}
	if strings.TrimSpace(gbEntry) == "" {
		return &exitErr{code: 2, msg: "--entry cannot be empty"}
	}

	out, err := gitignore.EnsureEntry(abs, gbEntry)
	if err != nil {
		// Match the python heredoc: write failures are warnings, not blockers.
		fmt.Fprintf(os.Stderr, "[WARN] gitignore-bootstrap: %v\n", err)
		return nil
	}
	if !gbQuiet {
		switch out {
		case gitignore.AlreadyPresent:
			// Silent — no-op.
		case gitignore.Appended:
			fmt.Printf("[OK] Appended '%s' to .gitignore\n", gbEntry)
		case gitignore.Created:
			fmt.Printf("[OK] Created .gitignore with '%s' entry\n", gbEntry)
		}
	}
	return nil
}
