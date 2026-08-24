package cmd

// hook_soak.go — `bin/edikt hook soak` (PLAN-v080-reliability-core Phase 15
// group 1 / Phase 19 group 2).
//
// WHAT THIS PROVES, AND WHAT IT DOES NOT
//
// Before the write-deny gates (group 3: pre-tool-use, verify-gate,
// inject-directives-pre) are allowed to migrate to Go, SPEC-014 spec.md's
// "Dogfood soak" requires evidence the SIMPLER groups already migrated
// actually FIRED over a real window of edikt-dev's own daily development —
// not that they compile, not that their unit tests pass, but that they ran.
// This command reads the hook journal (internal/hookmatch/journal.go's
// per-hook `hook_run` records) and reports, per hook in the named group,
// whether that evidence exists.
//
// THE GATE'S REFUSAL LOGIC IS PROVABLE TODAY; ONLY ITS PASSING WAITS ON THE
// CALENDAR. --journal is this command's sole testability seam for that: it
// overrides which journal file is read, so a test can construct a
// synthetic journal (or none at all) and exercise every refusal path
// without depending on this machine's real, 14-day-old journal history.
// --root and --since-commit are the other two inputs a test substitutes;
// see internal/hooksoak's own doc comment for the full design rationale.
//
// EXIT CODES follow the convention already established across tools/edikt/cmd
// (see exitCodeError in root.go, and e.g. verify_all.go's own 0/1/3 split):
//
//	0  the soak PASSED — every hook in the group has clean invocation
//	   evidence in the window
//	1  the soak REFUSED — at least one hook has no evidence, or a
//	   disqualifying outcome was observed. This is a VERDICT, not a usage
//	   error: the gate ran correctly and reported what it found.
//	3  a USAGE or SETUP problem — a bad --group, a missing/malformed
//	   --since-commit, an unloadable registry, or an unresolvable commit.
//	   Deliberately distinct from 1: a caller scripting against this command
//	   must be able to tell "the soak isn't ready yet" from "you called this
//	   wrong" without parsing stderr text.

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/diktahq/edikt/tools/edikt/internal/hook"
	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
	"github.com/diktahq/edikt/tools/edikt/internal/hooksoak"
	"github.com/spf13/cobra"
)

// hookSoakCommitRe matches hook-soak.v1's own `since_commit` pattern
// exactly — the same rule enforced twice would drift, so this is checked
// against the schema in a test.
var hookSoakCommitRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

func newHookSoakCmd() *cobra.Command {
	var group int
	var sinceCommit, root, journalOverride string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "soak",
		Short: "Check whether a migrated hook group has soaked under real use",
		Long: `Report, per hook in the named migration group, whether the hook journal
carries clean invocation evidence within the soak window.

Refuses when any hook in the group has zero journal invocation records, when
any hook's window carries a panic-recovered, unregistered-hook-name, or
binary-absent outcome, or when the journal itself is unreadable or absent —
the last of these is reported as UNKNOWN, never as a passing zero.

elapsed_days is always derived from --since-commit's git commit date via
'git log -1 --format=%cI', never from a local state file, so two engineers on
the same commit compute the identical soak age.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if group != 1 && group != 2 {
				return &exitCodeError{code: 3, msg: fmt.Sprintf(
					"hook soak: --group must be 1 or 2 (got %d) — group 3 (the gate class) has no soak gate of its own", group)}
			}
			if sinceCommit == "" {
				return &exitCodeError{code: 3, msg: "hook soak: --since-commit is required"}
			}
			if !hookSoakCommitRe.MatchString(sinceCommit) {
				return &exitCodeError{code: 3, msg: fmt.Sprintf(
					"hook soak: --since-commit %q is not a git commit hash (want 7-40 lowercase hex chars)", sinceCommit)}
			}
			if root == "" {
				wd, err := os.Getwd()
				if err != nil {
					return &exitCodeError{code: 3, msg: err.Error()}
				}
				root = wd
			}

			regPath, candidates, err := hook.ResolveRegistryPath(root)
			if err != nil {
				return &exitCodeError{code: 3, msg: fmt.Sprintf("hook soak: %v (looked in %v)", err, candidates)}
			}

			journalPath := journalOverride
			if journalPath == "" {
				jp, jerr := hookmatch.JournalPath()
				if jerr != nil {
					return &exitCodeError{code: 3, msg: fmt.Sprintf("hook soak: resolve hook journal path: %v", jerr)}
				}
				journalPath = jp
			}

			rec, err := hooksoak.Check(hooksoak.CheckOptions{
				RegistryPath: regPath,
				Group:        group,
				SinceCommit:  sinceCommit,
				RepoRoot:     root,
				JournalPath:  journalPath,
			})
			if err != nil {
				return &exitCodeError{code: 3, msg: fmt.Sprintf("hook soak: %v", err)}
			}

			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", " ")
				if encErr := enc.Encode(rec); encErr != nil {
					return &exitCodeError{code: 1, msg: encErr.Error()}
				}
			} else {
				fmt.Fprint(cmd.OutOrStdout(), rec.Report())
			}

			if rec.Status == hooksoak.StatusRefused {
				// Empty msg: the record (JSON or Report()) has already said
				// whatever there is to say, and cobra's "error: " prefix on
				// top of that would be noise, matching hook_run.go's own
				// empty-message exitCodeError convention.
				return &exitCodeError{code: 1}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&group, "group", 0, "soak group to check: 1 (injectors) or 2 (multi-call hooks) — required")
	cmd.Flags().StringVar(&sinceCommit, "since-commit", "", "git commit hash the group's soak window counts forward from — required")
	cmd.Flags().StringVar(&root, "root", "", "project root, for registry resolution and the git lookup (defaults to cwd)")
	cmd.Flags().StringVar(&journalOverride, "journal", "",
		"override the hook journal path read for invocation evidence (default: ~/.edikt/state/hook-journal.jsonl) — testability seam only")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON conforming to hook-soak.v1")
	return cmd
}
