package gov

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/diktahq/edikt/tools/edikt/internal/reextract"
	"github.com/spf13/cobra"
)

var (
	reextractForce            bool
	reextractOnly             []string
	reextractConcurrency      int
	reextractCleanTree        bool
	reextractJSON             bool
	reextractStatus           bool
	reextractRestamp          bool
	reextractSkipFixtureProof bool
)

var reextractCmd = &cobra.Command{
	Use:   "reextract [project-root]",
	Short: "Regenerate every sidecar through the locked extractor (requires --force)",
	Long: `Deliberately regenerates the whole live corpus through the locked
sidecar-extractor, behind an explicit --force flag.

WHY A SEPARATE VERB. Phase A regenerates a sidecar when it is STALE, and
staleness is measured from anchors. After an extraction-contract change every
anchor still matches, so nothing is stale and nothing regenerates — while every
sidecar was written by a contract that no longer applies. Re-extraction is
never an implicit side-effect of staleness: it runs behind --force or it does
not run.

RESUMABLE BY CONSTRUCTION. Each completion is written to
.edikt/state/reextract-ledger.json as it happens, so a kill mid-batch costs
only the unfinished work and re-invoking dispatches only what remains. The
ledger is keyed by the extractor's prompt version: a contract change starts a
new batch rather than reporting old work as current.

The ledger records the hash of each sidecar it wrote, so an artifact edited
after its dispatch is re-dispatched rather than counted as covered.

Flags:
  --force        required; without it nothing is dispatched
  --only ID      restrict the batch to named artifacts (repeatable)
  --concurrency  parallel dispatches (default 4)
  --clean-tree   refuse to start unless the working tree is clean, so the
                 batch can land as one reviewable commit
  --status       report ledger state and exit without dispatching
  --json         emit the result summary as JSON
  --skip-fixture-proof  bypass the fixture-validation precondition (rare; see --help)`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}

		if reextractRestamp {
			return reextractRestampDone(root)
		}
		if reextractStatus {
			return reextractReportStatus(root)
		}

		res, err := reextract.Run(reextract.Options{
			ProjectRoot:      root,
			Force:            reextractForce,
			Only:             reextractOnly,
			Concurrency:      reextractConcurrency,
			RequireCleanTree: reextractCleanTree,
			SkipFixtureProof: reextractSkipFixtureProof,
			Stderr:           os.Stderr,
		})
		if res != nil && reextractJSON {
			b, _ := json.MarshalIndent(res, "", "  ")
			fmt.Fprintln(os.Stdout, string(b))
		} else if res != nil {
			fmt.Fprintf(os.Stdout, "re-extraction: %d eligible, %d already done, %d dispatched, %d succeeded, %d failed (prompt %s)\n",
				res.Eligible, res.AlreadyDone, res.Dispatched, res.Succeeded, res.Failed, res.PromptVersion)
		}
		return err
	},
}

// reextractRestampDone re-records the ledger hash for every artifact already
// marked done.
//
// EXPLICIT, NEVER AUTOMATIC. The recorded hash answers "is this sidecar still
// the one this batch produced?", and re-stamping is the operator saying "yes,
// I changed it on purpose" — after an out-of-band repair such as
// `restore-pinned`. Doing it automatically on any mismatch would delete the
// hash's only job, which is noticing a change nobody declared.
func reextractRestampDone(root string) error {
	st, err := reextract.Status(root)
	if err != nil {
		return err
	}
	pairs, err := reextract.DoneSidecarPaths(root)
	if err != nil {
		return err
	}
	n, err := reextract.RestampLedger(root, pairs)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "re-stamped %d ledger entry(ies) of %d done (batch %s)\n", n, st.Done+n, st.PromptVersion)
	return nil
}

func reextractReportStatus(root string) error {
	st, err := reextract.Status(root)
	if err != nil {
		return err
	}
	if reextractJSON {
		b, _ := json.MarshalIndent(st, "", "  ")
		fmt.Fprintln(os.Stdout, string(b))
		return nil
	}
	fmt.Fprintf(os.Stdout, "batch %s: %d/%d regenerated, %d remaining, %d failed\n",
		st.PromptVersion, st.Done, st.Eligible, st.Remaining, st.Failed)
	for _, id := range st.RemainingIDs {
		fmt.Fprintf(os.Stdout, "  pending  %s\n", id)
	}
	for _, id := range st.FailedIDs {
		fmt.Fprintf(os.Stdout, "  failed   %s\n", id)
	}
	return nil
}

func init() {
	reextractCmd.Flags().BoolVar(&reextractForce, "force", false, "required: dispatch the extractor over the corpus")
	reextractCmd.Flags().StringSliceVar(&reextractOnly, "only", nil, "restrict the batch to these artifact IDs")
	reextractCmd.Flags().IntVar(&reextractConcurrency, "concurrency", 4, "parallel extractor dispatches")
	reextractCmd.Flags().BoolVar(&reextractCleanTree, "clean-tree", false, "refuse to start unless the working tree is clean")
	reextractCmd.Flags().BoolVar(&reextractStatus, "status", false, "report ledger state without dispatching")
	reextractCmd.Flags().BoolVar(&reextractRestamp, "restamp-done", false,
		"re-record the ledger hash for artifacts already marked done, after a deliberate out-of-band repair")
	reextractCmd.Flags().BoolVar(&reextractJSON, "json", false, "emit JSON")
	reextractCmd.Flags().BoolVar(&reextractSkipFixtureProof, "skip-fixture-proof", false,
		"bypass the fixture-validation precondition; use only if you are certain the installed "+
			"extraction contract is already validated despite what this binary's embedded record says")
	Cmd.AddCommand(reextractCmd)
}
