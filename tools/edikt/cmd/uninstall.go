package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var uninstallYes bool

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove EDIKT_ROOT entirely and unlink commands symlink",
	Long: `Removes $EDIKT_ROOT and $CLAUDE_ROOT/commands/edikt after confirmation.

Guards:
  - Refuses if $EDIKT_ROOT is a symlink (would follow link and nuke the target).
  - Refuses if $EDIKT_ROOT is not under $HOME (misconfigured root safety check).
  - Refuses if $EDIKT_ROOT does not look like an edikt root.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return err
		}
		claudeRoot := resolveClaudeRoot()

		// Guard: refuse if EDIKT_ROOT is a symlink.
		if linfo, err := os.Lstat(ediktRoot); err == nil && linfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to uninstall: %s is a symlink.\nRemove it manually after verifying the target.", ediktRoot)
		}

		// Guard: refuse if EDIKT_ROOT is not under $HOME.
		home := os.Getenv("HOME")
		if home != "" {
			cleanRoot := filepath.Clean(ediktRoot)
			cleanHome := filepath.Clean(home)
			if !strings.HasPrefix(cleanRoot, cleanHome+string(os.PathSeparator)) && cleanRoot != cleanHome {
				return fmt.Errorf("refusing to uninstall: %s is not under $HOME (%s) — safety guard", ediktRoot, home)
			}
		}

		// Guard: must look like an edikt root.
		base := filepath.Base(ediktRoot)
		_, hasLock := os.Stat(filepath.Join(ediktRoot, "lock.yaml"))
		_, hasVersion := os.Stat(filepath.Join(ediktRoot, "VERSION"))
		_, hasVersions := os.Stat(filepath.Join(ediktRoot, "versions"))
		if base != ".edikt" && os.IsNotExist(hasLock) && os.IsNotExist(hasVersion) && os.IsNotExist(hasVersions) {
			return fmt.Errorf("refusing to uninstall: %s does not look like an edikt root\n(basename is not '.edikt' and no lock.yaml/VERSION/versions/ found)", ediktRoot)
		}

		if !uninstallYes {
			fmt.Printf("Remove %s and unlink %s/commands/edikt? [y/N]: ", ediktRoot, claudeRoot)
			var reply string
			fmt.Scanln(&reply)
			switch strings.ToLower(strings.TrimSpace(reply)) {
			case "y", "yes":
			default:
				fmt.Fprintln(os.Stderr, "aborted")
				return nil
			}
		}

		// Remove Claude commands symlink.
		ediktCmds := filepath.Join(claudeRoot, "commands", "edikt")
		if linfo, err := os.Lstat(ediktCmds); err == nil && linfo.Mode()&os.ModeSymlink != 0 {
			os.Remove(ediktCmds)
		}

		if err := os.RemoveAll(ediktRoot); err != nil {
			return fmt.Errorf("removing %s: %w", ediktRoot, err)
		}

		fmt.Fprintln(os.Stderr, "uninstalled")
		return nil
	},
}

func init() {
	uninstallCmd.Flags().BoolVarP(&uninstallYes, "yes", "y", false, "skip confirmation prompt")
	rootCmd.AddCommand(uninstallCmd)
}
