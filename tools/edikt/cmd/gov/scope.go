package gov

// scope.go — `edikt gov scope`: resolve a file set into review units.
//
// This is the shared engine both review surfaces call: the diff-scoped one that
// runs in the post-flight loop, and the whole-tree one. One engine, two file-set
// sources — so a fix to how directives are resolved cannot land in one and miss
// the other, which is how review-time coverage fell to 5 of 805 in the first
// place.
//
// The resolver is hookmatch.MatchGoverning over the delivery map — the one
// path->directive scope source since ADR-083 retired the write-time index.  edikt-guard:allow
// That is deliberate and is the whole point: if a directive stops reaching the
// hook it stops reaching review, and a single coverage number describes both.
// A second matcher here would be a second definition of "which rules govern
// this file", and the two would drift.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
	"github.com/spf13/cobra"
)

func newScopeCmd() *cobra.Command {
	var (
		root      string
		all       bool
		subtree   string
		pathsFrom string
		asJSON    bool
	)

	cmd := &cobra.Command{
		Use:   "scope",
		Short: "Resolve a file set into review units for semantic review",
		Long: `Resolve files into REVIEW UNITS — batches of files that share an identical
directive set — and report the scale of a review before one is run.

File-set sources (exactly one):
  --all              every tracked file in the repository
  --subtree <path>   every tracked file under <path>
  --paths-from <f>   newline-separated paths from a file, or - for stdin

Batching is by matched directive set, not by file: files governed by the same
directives are one unit, because the question asked of a reviewer is identical
for all of them. The files-to-units collapse is the cost driver and is reported.

Directives resolve through the delivery map, the one path->directive scope
source, so no two consumers can disagree about what governs a path.

An empty or unresolvable scope reports UNMEASURED and exits 3. It never reports
an empty result as a clean review — that is the defect this verb exists to
remove.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if root == "" {
				wd, err := os.Getwd()
				if err != nil {
					return err
				}
				root = wd
			}
			abs, err := filepath.Abs(root)
			if err != nil {
				return err
			}
			root = abs

			sources := 0
			for _, on := range []bool{all, subtree != "", pathsFrom != ""} {
				if on {
					sources++
				}
			}
			if sources != 1 {
				return errors.New("exactly one of --all, --subtree or --paths-from is required")
			}

			files, err := collectFiles(root, all, subtree, pathsFrom)
			if err != nil {
				return err
			}

			// GOVERNANCE, not injection. This command answers "what governs
			// this file", and since ADR-079 that is no longer the same question
			// as "what does PreToolUse inject" — advisory is delivered by the
			// topic file. Resolving through hookmatch.Match counted only the
			// write-time surface and under-reported the corpus by 552
			// directives, which check-review-coverage-equivalence caught as a
			// 790 -> 238 shrink. Its baseline was not lowered; this call was
			// corrected instead.
			//
			// Text is intentionally absent: the routing table carries no
			// directive bodies, and this command's only consumer reads
			// `directives_in_scope`, a count of distinct ids. Adding text here
			// would mean duplicating every directive body into a second file
			// that nothing delivers — the drift risk ADR-079 removed.
			rep := BuildReviewUnits(files, func(p string) ([]Directive, error) {
				res := hookmatch.MatchGoverning(root, p)
				switch res.Outcome {
				case hookmatch.OutcomeIndexMissing, hookmatch.OutcomeIndexCorrupt:
					return nil, fmt.Errorf("%s: %s", res.Outcome, res.Detail)
				}
				out := make([]Directive, 0, len(res.Entries))
				for _, e := range res.Entries {
					out = append(out, Directive{
						ID:      e.ID,
						Grade:   e.Grade,
						Class:   e.Class,
						Surface: e.Surface,
					})
				}
				return out, nil
			})

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(rep); err != nil {
					return err
				}
			} else {
				printScopeSummary(cmd, rep)
			}

			if rep.Unmeasured {
				return &scopeUnmeasured{reason: rep.Reason}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&root, "root", "", "project root (defaults to cwd)")
	cmd.Flags().BoolVar(&all, "all", false, "every tracked file in the repository")
	cmd.Flags().StringVar(&subtree, "subtree", "", "every tracked file under this path")
	cmd.Flags().StringVar(&pathsFrom, "paths-from", "", "read newline-separated paths from a file, or - for stdin")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the full report as JSON")
	return cmd
}

// scopeUnmeasured carries exit code 3 — distinct from a usage error (1) and
// from a clean run (0), so a caller can tell "I could not measure" apart from
// "I measured and found nothing".
type scopeUnmeasured struct{ reason string }

func (e *scopeUnmeasured) Error() string { return "UNMEASURED: " + e.reason }
func (e *scopeUnmeasured) ExitCode() int { return 3 }

// collectFiles resolves the requested file-set source. Tracked files only: an
// untracked build artifact is not part of the codebase under review, and
// including it would inflate every cost estimate.
func collectFiles(root string, all bool, subtree, pathsFrom string) ([]string, error) {
	if pathsFrom != "" {
		var raw []byte
		var err error
		if pathsFrom == "-" {
			raw, err = readAllStdin()
		} else {
			raw, err = os.ReadFile(pathsFrom)
		}
		if err != nil {
			return nil, err
		}
		var out []string
		for _, l := range strings.Split(string(raw), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				out = append(out, l)
			}
		}
		return out, nil
	}

	args := []string{"ls-files"}
	if subtree != "" {
		args = append(args, "--", subtree)
	}
	c := exec.Command("git", args...)
	c.Dir = root
	out, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w (a whole-tree scope needs a git repository)", err)
	}
	var files []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	sort.Strings(files)
	return files, nil
}

func readAllStdin() ([]byte, error) {
	st, err := os.Stdin.Stat()
	if err != nil {
		return nil, err
	}
	if st.Mode()&os.ModeCharDevice != 0 {
		return nil, errors.New("--paths-from - was given but stdin is a terminal")
	}
	return io.ReadAll(os.Stdin)
}

// printScopeSummary is the pre-flight a caller shows BEFORE spending anything.
// A whole-tree LLM pass that starts without telling the user its size is not
// shippable, so the numbers come first and the units come second.
func printScopeSummary(cmd *cobra.Command, rep ScopeReport) {
	w := cmd.OutOrStdout()

	if rep.Unmeasured {
		fmt.Fprintf(w, "scope: UNMEASURED — %s\n", rep.Reason)
		fmt.Fprintf(w, "  files scanned: %d, of which governed by nothing: %d\n", rep.FilesScanned, rep.FilesUnmapped)
		fmt.Fprintln(w, "  This is not a clean review. Nothing was examined against any directive.")
		return
	}

	collapse := "n/a"
	if rep.ReviewUnits > 0 {
		collapse = fmt.Sprintf("%.1fx", float64(rep.FilesMatched)/float64(rep.ReviewUnits))
	}

	fmt.Fprintln(w, "scope:")
	fmt.Fprintf(w, "  files scanned        %d\n", rep.FilesScanned)
	fmt.Fprintf(w, "  files governed       %d\n", rep.FilesMatched)
	fmt.Fprintf(w, "  files ungoverned     %d\n", rep.FilesUnmapped)
	fmt.Fprintf(w, "  review units         %d   (%s collapse — units are what a review costs, not files)\n", rep.ReviewUnits, collapse)
	fmt.Fprintf(w, "  directives in scope  %d\n", rep.DirectivesTot)
}
