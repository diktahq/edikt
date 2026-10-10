package cmd

// plan_active.go — `bin/edikt plan active`, replacing the inline python in
// commands/status.md (PLAN-v080 Phase 24, FR-010).
//
// WHAT IT ANSWERS: which plan is the one still being worked on. The newest
// PLAN-*.md whose Progress table still has a row that is not a completion
// marker. The resolution logic itself lives in internal/activeplan (SR-006,
// gov prune-candidates needed the same answer from a package `cmd` cannot be
// imported by — GL-002 "one rule, one implementation" rather than a second
// copy of the scoping rule).

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/activeplan"
	"github.com/spf13/cobra"
)

func init() {
	planCmd := &cobra.Command{Use: "plan", Short: "Query plan files"}
	planCmd.AddCommand(newPlanActiveCmd(), newPlanLastPhaseCmd())
	rootCmd.AddCommand(planCmd)
}

func newPlanActiveCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "active",
		Short: "Print the path of the newest plan with an incomplete progress row",
		Long: `Print the most recently modified PLAN-*.md whose Progress table still
has at least one row that is not marked complete.

Prints nothing and exits 0 when every plan is complete — that is a real answer
("no plan is active"), not a failure. Exits non-zero only when the directory
cannot be read, so a caller can tell "nothing to do" from "could not look".`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				return fmt.Errorf("--dir is required")
			}
			path, err := activeplan.Resolve(dir)
			if err != nil {
				return err
			}
			if path != "" {
				fmt.Fprintln(cmd.OutOrStdout(), path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "directory holding PLAN-*.md files (required)")
	return cmd
}

var planPhaseRowRe = regexp.MustCompile(`(?i)^\|\s*([0-9]+)\s*\|\s*(done|in-progress)\s*`)

// newPlanLastPhaseCmd prints the LAST phase row marked done or in-progress.
//
// Last, not highest: the progress table is written in phase order, and a plan
// whose rows are out of numerical order is telling you something about its own
// sequencing that a max() would discard.
//
// NO MATCH IS AN ERROR, not an empty line. The caller uses this to decide which
// phase it is reporting on, and a blank answer there becomes a report about
// phase "" — a document that looks complete and describes nothing.
func newPlanLastPhaseCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "last-phase",
		Short: "Print the last phase row marked done or in-progress",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(file) //nolint:gosec // operator-supplied path
			if err != nil {
				return err
			}
			last := ""
			for _, ln := range strings.Split(string(b), "\n") {
				if m := planPhaseRowRe.FindStringSubmatch(ln); m != nil {
					last = m[1]
				}
			}
			if last == "" {
				return fmt.Errorf("%s: no phase row marked done or in-progress", file)
			}
			fmt.Fprintln(cmd.OutOrStdout(), last)
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "plan file (required)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}
