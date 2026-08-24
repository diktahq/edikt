package cmd

// plan_active.go — `bin/edikt plan active`, replacing the inline python in
// commands/status.md (PLAN-v080 Phase 24, FR-010).
//
// WHAT IT ANSWERS: which plan is the one still being worked on. The newest
// PLAN-*.md whose Progress table still has a row that is not a completion
// marker.
//
// THE SCOPING IS THE WHOLE ALGORITHM. A plan body contains several tables —
// cost estimates, phase prompts, dependency lists — and rows in those look
// exactly like progress rows. Counting them made finished plans look active,
// which is why the search is bounded to the `## Progress` section rather than
// run over the whole file. That bound is preserved here verbatim, including
// its failure mode: a plan with no Progress heading contributes nothing rather
// than being treated as fully complete.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var (
	planCompleteRe = regexp.MustCompile(`(?i)\b(done|pass(ed)?|complete(d)?)\b|✅`)
	planRowRe      = regexp.MustCompile(`^\| *\d+[a-z]?\s*\|`)
	planProgressRe = regexp.MustCompile(`(?mi)^##+ +Progress\b`)
	planNextHeadRe = regexp.MustCompile(`(?m)^##+ +`)
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
			entries, err := os.ReadDir(dir)
			if err != nil {
				return err
			}
			type cand struct {
				path string
				mod  int64
			}
			var files []cand
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				if !strings.HasPrefix(name, "PLAN-") || !strings.HasSuffix(name, ".md") {
					continue
				}
				if strings.Contains(name, ".SUPERSEDED") {
					continue
				}
				info, ierr := e.Info()
				if ierr != nil {
					continue
				}
				files = append(files, cand{filepath.Join(dir, name), info.ModTime().UnixNano()})
			}
			sort.Slice(files, func(i, j int) bool { return files[i].mod > files[j].mod })

			for _, f := range files {
				body, rerr := os.ReadFile(f.path) //nolint:gosec // path built from ReadDir
				if rerr != nil {
					continue
				}
				rows := progressRows(string(body))
				if len(rows) == 0 {
					continue
				}
				for _, r := range rows {
					if !planCompleteRe.MatchString(r) {
						fmt.Fprintln(cmd.OutOrStdout(), f.path)
						return nil
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "directory holding PLAN-*.md files (required)")
	return cmd
}

// progressRows returns the table rows inside the `## Progress` section only.
func progressRows(body string) []string {
	m := planProgressRe.FindStringIndex(body)
	if m == nil {
		return nil
	}
	rest := body[m[1]:]
	if nxt := planNextHeadRe.FindStringIndex(rest); nxt != nil {
		rest = rest[:nxt[0]]
	}
	var out []string
	for _, ln := range strings.Split(rest, "\n") {
		if planRowRe.MatchString(ln) {
			out = append(out, ln)
		}
	}
	return out
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
