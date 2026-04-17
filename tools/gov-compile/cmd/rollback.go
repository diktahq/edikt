package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var rollbackCmd = &cobra.Command{
	Use:   "rollback [version]",
	Short: "Re-activate the previous version (or a specific version)",
	Long: `Flips the current symlink back to the previous version recorded in lock.yaml.
Optionally accepts a specific version tag to roll back to.

Special form: edikt rollback v0.5.0 — delegates to edikt-shell to restore
pre-v0.5.0 host files from the backup install.sh wrote during the 0.4.x migration.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Special v0.5.0 host-file rollback was previously handled by edikt-shell.
		// edikt-shell has been removed (ADR-022 Phase 3). The versioned layout is
		// the only supported layout from v0.5.0 onward; a host-file rollback to the
		// flat pre-v0.5.0 layout is not supported via this command.
		// Use 'edikt migrate --abort' instead if a migration is in progress.

		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return err
		}

		lf, err := readLock(ediktRoot)
		if err != nil {
			return fmt.Errorf("reading lock.yaml: %w", err)
		}

		// Determine target version.
		var targetTag string
		if len(args) == 1 {
			targetTag = normalizeTag(args[0])
		} else {
			if lf.Previous == "" {
				return fmt.Errorf("no previous version recorded in lock.yaml")
			}
			targetTag = normalizeTag(lf.Previous)
		}

		targetDir := filepath.Join(ediktRoot, "versions", targetTag)
		if _, err := os.Stat(targetDir); os.IsNotExist(err) {
			return fmt.Errorf("previous version %s is not installed", targetTag)
		}

		currentTag := lf.Active

		// Atomic symlink flip.
		currentLink := filepath.Join(ediktRoot, "current")
		newLink := currentLink + fmt.Sprintf(".new.%d", os.Getpid())
		os.Remove(newLink)

		relTarget := filepath.Join("versions", targetTag)
		if err := os.Symlink(relTarget, newLink); err != nil {
			return fmt.Errorf("creating symlink: %w", err)
		}
		if err := os.Rename(newLink, currentLink); err != nil {
			os.Remove(newLink)
			return fmt.Errorf("flipping current: %w", err)
		}

		if err := writeLock(ediktRoot, targetTag, "launcher"); err != nil {
			fmt.Fprintf(os.Stderr, "warn: rolled back but lock.yaml update failed: %v\n", err)
		}

		fmt.Fprintf(os.Stderr, "rolled back to %s (was %s)\n", targetTag, currentTag)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(rollbackCmd)
}
