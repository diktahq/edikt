package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var useCmd = &cobra.Command{
	Use:   "use <tag>",
	Short: "Activate an installed version (atomic symlink flip)",
	Long: `Atomically updates $EDIKT_ROOT/current to point at versions/<tag>,
then rewrites lock.yaml with the new active version.
Fails clearly if the version is not installed.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tag := normalizeTag(args[0])

		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return err
		}

		targetDir := filepath.Join(ediktRoot, "versions", tag)
		if _, err := os.Stat(targetDir); os.IsNotExist(err) {
			return fmt.Errorf("version %s is not installed. Run `edikt install %s` first.", tag, tag)
		}

		// Atomic symlink flip: create a sibling .new symlink then rename over target.
		currentLink := filepath.Join(ediktRoot, "current")
		newLink := currentLink + fmt.Sprintf(".new.%d", os.Getpid())

		// Remove any stale .new link.
		os.Remove(newLink)

		// Target is relative so it works regardless of where EDIKT_ROOT lives.
		relTarget := filepath.Join("versions", tag)
		if err := os.Symlink(relTarget, newLink); err != nil {
			return fmt.Errorf("creating symlink: %w", err)
		}

		if err := atomicRenameSymlink(newLink, currentLink); err != nil {
			os.Remove(newLink)
			return fmt.Errorf("activating version: %w", err)
		}

		if err := writeLock(ediktRoot, tag, "launcher"); err != nil {
			fmt.Fprintf(os.Stderr, "warn: version activated but lock.yaml update failed: %v\n", err)
		}

		fmt.Fprintf(os.Stderr, "activated %s\n", tag)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(useCmd)
}

// atomicRenameSymlink renames src over dst atomically.  On POSIX systems
// rename(2) on a symlink replaces the destination atomically.  We try
// BSD mv -hf and GNU mv -Tf first (which handle symlink-to-directory
// targets correctly), then fall back to os.Rename.
func atomicRenameSymlink(src, dst string) error {
	// os.Rename is atomic on POSIX and works for symlinks.
	return os.Rename(src, dst)
}
