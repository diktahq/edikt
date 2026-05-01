package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Development mode commands (link, unlink)",
}

var devLinkCmd = &cobra.Command{
	Use:   "link <path>",
	Short: "Create $EDIKT_ROOT/versions/dev/ as a symlink to <path> and activate it",
	Long: `Creates $EDIKT_ROOT/versions/dev/ with symlinks into <path> and activates dev
mode by flipping $EDIKT_ROOT/current to point at versions/dev.

Validation:
  - <path> must be an existing directory
  - <path> must not contain '..'
  - Warns if dev is already linked.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		src := args[0]

		// Reject paths containing ..
		if strings.Contains(src, "..") {
			return fmt.Errorf("dev link: path must not contain '..'")
		}

		// Resolve to absolute.
		if !filepath.IsAbs(src) {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting cwd: %w", err)
			}
			src = filepath.Join(cwd, src)
		}
		src = filepath.Clean(src)

		if _, err := os.Stat(src); os.IsNotExist(err) {
			return fmt.Errorf("dev link: path does not exist: %s", src)
		}

		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return err
		}

		devDir := filepath.Join(ediktRoot, "versions", "dev")

		// Warn if dev link already exists.
		if _, err := os.Lstat(devDir); err == nil {
			fmt.Fprintf(os.Stderr, "warn: dev link already exists at %s — relinking\n", devDir)
			os.RemoveAll(devDir)
		}

		if err := os.MkdirAll(filepath.Join(ediktRoot, "versions"), 0o755); err != nil {
			return fmt.Errorf("creating versions dir: %w", err)
		}
		if err := os.MkdirAll(devDir, 0o755); err != nil {
			return fmt.Errorf("creating dev dir: %w", err)
		}

		// Create symlinks into the source tree.
		links := map[string]string{
			"VERSION":   filepath.Join(src, "VERSION"),
			"hooks":     filepath.Join(src, "templates", "hooks"),
			"templates": filepath.Join(src, "templates"),
			"commands":  filepath.Join(src, "commands"),
		}
		for name, target := range links {
			linkPath := filepath.Join(devDir, name)
			os.Remove(linkPath)
			if err := os.Symlink(target, linkPath); err != nil {
				return fmt.Errorf("creating symlink %s → %s: %w", linkPath, target, err)
			}
		}

		// Optional CHANGELOG.md.
		changelogSrc := filepath.Join(src, "CHANGELOG.md")
		if _, err := os.Stat(changelogSrc); err == nil {
			changelogLink := filepath.Join(devDir, "CHANGELOG.md")
			os.Remove(changelogLink)
			os.Symlink(changelogSrc, changelogLink)
		}

		// Write DEV_SOURCE file.
		devSourcePath := filepath.Join(devDir, "DEV_SOURCE")
		if err := os.WriteFile(devSourcePath, []byte(src+"\n"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "warn: could not write DEV_SOURCE: %v\n", err)
		}

		// Flip current to versions/dev.
		currentLink := filepath.Join(ediktRoot, "current")
		newLink := currentLink + fmt.Sprintf(".new.%d", os.Getpid())
		os.Remove(newLink)
		if err := os.Symlink(filepath.Join("versions", "dev"), newLink); err != nil {
			return fmt.Errorf("creating current symlink: %w", err)
		}
		if err := os.Rename(newLink, currentLink); err != nil {
			os.Remove(newLink)
			return fmt.Errorf("flipping current: %w", err)
		}

		if err := writeLock(ediktRoot, "dev", "dev-link"); err != nil {
			fmt.Fprintf(os.Stderr, "warn: dev linked but lock.yaml update failed: %v\n", err)
		}

		fmt.Fprintf(os.Stderr, "dev mode active: %s\n", src)
		return nil
	},
}

var devUnlinkCmd = &cobra.Command{
	Use:   "unlink",
	Short: "Remove dev mode, revert to the previous version from lock.yaml",
	Long: `Removes $EDIKT_ROOT/versions/dev (the dev symlink directory) and
reactivates the previous version from lock.yaml if available.
Idempotent — safe to call when dev mode is not active.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return err
		}

		devDir := filepath.Join(ediktRoot, "versions", "dev")
		if _, err := os.Lstat(devDir); os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "no dev link active")
			return nil
		}

		// Remove the dev dir.
		if err := os.RemoveAll(devDir); err != nil {
			return fmt.Errorf("removing dev dir: %w", err)
		}

		// Read lock.yaml to find what to fall back to.
		lf, _ := readLock(ediktRoot)
		fallbackTag := normalizeTag(lf.Previous)

		if fallbackTag == "" {
			// Walk versions/ for best non-dev tag.
			versionsDir := filepath.Join(ediktRoot, "versions")
			if entries, err := os.ReadDir(versionsDir); err == nil {
				for _, e := range entries {
					if e.IsDir() && e.Name() != "dev" {
						fallbackTag = e.Name()
					}
				}
			}
		}

		if fallbackTag == "" {
			fmt.Fprintln(os.Stderr, "error: no tagged version to fall back to — run 'edikt install <tag>'")
			os.Exit(1)
		}

		// Flip current to the fallback.
		currentLink := filepath.Join(ediktRoot, "current")
		newLink := currentLink + fmt.Sprintf(".new.%d", os.Getpid())
		os.Remove(newLink)
		relTarget := filepath.Join("versions", fallbackTag)
		if err := os.Symlink(relTarget, newLink); err != nil {
			return fmt.Errorf("creating symlink: %w", err)
		}
		if err := os.Rename(newLink, currentLink); err != nil {
			os.Remove(newLink)
			return fmt.Errorf("flipping current: %w", err)
		}

		if err := writeLock(ediktRoot, fallbackTag, "launcher"); err != nil {
			fmt.Fprintf(os.Stderr, "warn: dev unlinked but lock.yaml update failed: %v\n", err)
		}

		fmt.Fprintf(os.Stderr, "dev mode removed; activated %s\n", fallbackTag)
		return nil
	},
}

func init() {
	devCmd.AddCommand(devLinkCmd)
	devCmd.AddCommand(devUnlinkCmd)
	rootCmd.AddCommand(devCmd)
}
