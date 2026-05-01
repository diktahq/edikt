package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var pruneKeep int
var pruneDryRun bool
var pruneYes bool

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove old installed versions",
	Long: `Remove version directories under $EDIKT_ROOT/versions/ that are neither
active nor previous. Versions are sorted lexically; the N most recent are kept
(default N=3). The active and previous versions recorded in lock.yaml are
always protected regardless of sort position.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return err
		}

		versionsDir := filepath.Join(ediktRoot, "versions")
		if _, err := os.Stat(versionsDir); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "prune: no versions directory — nothing to prune\n")
			return nil
		}

		lf, _ := readLock(ediktRoot)
		activeTag := normalizeTag(lf.Active)
		previousTag := normalizeTag(lf.Previous)

		entries, err := os.ReadDir(versionsDir)
		if err != nil {
			return fmt.Errorf("reading versions dir: %w", err)
		}

		var tags []string
		for _, e := range entries {
			if e.IsDir() {
				tags = append(tags, e.Name())
			}
		}

		if len(tags) == 0 {
			fmt.Fprintf(os.Stderr, "prune: no versions found\n")
			return nil
		}

		// Sort lexically descending (newest first).
		sort.Sort(sort.Reverse(sort.StringSlice(tags)))

		var toKeep []string
		var toPrune []string
		for i, tag := range tags {
			if i < pruneKeep {
				toKeep = append(toKeep, tag)
			} else {
				toPrune = append(toPrune, tag)
			}
		}

		if len(toPrune) == 0 {
			fmt.Fprintf(os.Stderr, "prune: nothing to prune (total=%d, keep=%d)\n", len(tags), pruneKeep)
			return nil
		}

		// If not --yes and not --dry-run, show plan and confirm.
		if !pruneYes && !pruneDryRun {
			fmt.Printf("Would remove %d version(s):\n", len(toPrune))
			for _, t := range toPrune {
				tn := normalizeTag(t)
				if tn == activeTag || tn == previousTag {
					fmt.Printf("  (skip protected) %s\n", t)
				} else {
					fmt.Printf("  %s\n", t)
				}
			}
			fmt.Print("Proceed? [y/N]: ")
			var reply string
			fmt.Scanln(&reply)
			switch strings.ToLower(strings.TrimSpace(reply)) {
			case "y", "yes":
			default:
				fmt.Fprintln(os.Stderr, "aborted")
				return nil
			}
		}

		anyFailed := false
		for _, tag := range toPrune {
			tn := normalizeTag(tag)
			if activeTag != "" && (tn == activeTag || tag == activeTag) {
				fmt.Fprintf(os.Stderr, "prune: skipping active version %s\n", tag)
				continue
			}
			if previousTag != "" && (tn == previousTag || tag == previousTag) {
				fmt.Fprintf(os.Stderr, "prune: skipping previous version %s\n", tag)
				continue
			}

			if pruneDryRun {
				fmt.Printf("would prune: %s\n", tag)
			} else {
				fmt.Fprintf(os.Stderr, "prune: removing %s\n", tag)
				if err := os.RemoveAll(filepath.Join(versionsDir, tag)); err != nil {
					fmt.Fprintf(os.Stderr, "error: prune: failed to remove %s: %v\n", tag, err)
					anyFailed = true
				}
			}
		}

		if pruneDryRun {
			fmt.Fprintf(os.Stderr, "prune: dry-run complete (no changes)\n")
			return nil
		}

		if anyFailed {
			return fmt.Errorf("prune: one or more versions could not be removed")
		}
		return nil
	},
}

func init() {
	pruneCmd.Flags().IntVar(&pruneKeep, "keep", 3, "number of recent versions to keep")
	pruneCmd.Flags().BoolVar(&pruneDryRun, "dry-run", false, "print which versions would be removed without removing anything")
	pruneCmd.Flags().BoolVarP(&pruneYes, "yes", "y", false, "skip confirmation prompt")
	rootCmd.AddCommand(pruneCmd)
}
