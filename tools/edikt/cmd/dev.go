package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

		// Require templates/ to exist in the source (guards against linking
		// a directory that isn't an edikt payload).
		if _, err := os.Stat(filepath.Join(src, "templates")); os.IsNotExist(err) {
			return fmt.Errorf("dev link: %s/templates/ not found — is this an edikt repo?", src)
		}

		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return err
		}

		devDir := filepath.Join(ediktRoot, "versions", "dev")

		// If versions/dev/ exists and contains regular files, quarantine it
		// rather than silently destroying user content.
		if _, err := os.Lstat(devDir); err == nil {
			hasRegular := false
			if entries, err := os.ReadDir(devDir); err == nil {
				for _, e := range entries {
					if e.Type().IsRegular() {
						hasRegular = true
						break
					}
				}
			}
			if hasRegular {
				quarantine := filepath.Join(
					filepath.Dir(devDir),
					fmt.Sprintf("dev.aborted-%d-%d", time.Now().UnixMilli(), os.Getpid()),
				)
				if err := os.Rename(devDir, quarantine); err != nil {
					return fmt.Errorf("quarantining existing dev dir: %w", err)
				}
				fmt.Fprintf(os.Stderr, "warn: versions/dev/ had user content — quarantined to %s\n", filepath.Base(quarantine))
			} else {
				fmt.Fprintf(os.Stderr, "warn: dev link already exists at %s — relinking\n", devDir)
				os.RemoveAll(devDir)
			}
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

		// Refresh the convenience symlinks (hooks, templates,
		// commands/edikt) so the host agent's slash-command
		// resolution picks up the dev source. Without this, an
		// `edikt use` later would re-link, but `dev link` standalone
		// would leave stale targets.
		repairExternalSymlinks(ediktRoot)

		// Swap the user-facing launcher binary to the dev source's
		// build, if one exists. Without this, the user's PATH-resolved
		// `edikt` keeps running the previously-installed launcher
		// (which won't have any subcommands the dev source added) and
		// the dev link only affects payload paths — see issue #4 from
		// the v0.6.0-rc3 dogfood test where `edikt migrate sidecars`
		// failed despite a successful dev link because the launcher
		// was still v0.5.1. Backup the current launcher first so
		// `dev unlink` can restore it.
		if err := refreshLauncherForDev(ediktRoot, src); err != nil {
			fmt.Fprintf(os.Stderr, "warn: launcher binary not refreshed (%v) — `edikt` on PATH may still be the previous version\n", err)
		}

		emitEvent(ediktRoot, "dev_linked", map[string]interface{}{"source": src})
		fmt.Fprintf(os.Stderr, "dev mode active: %s\n", src)
		return nil
	},
}

// refreshLauncherForDev copies <src>/bin/edikt over $EDIKT_ROOT/bin/edikt
// so the user-facing `edikt` PATH resolution sees the dev source's
// binary and not whatever was last installed via `edikt install` /
// `edikt upgrade`. Backs up the existing launcher to bin/edikt.pre-dev
// for restoration by `dev unlink`. Idempotent: a stale .pre-dev backup
// is overwritten so consecutive `dev link` calls always reflect the
// most recent installed-launcher state.
func refreshLauncherForDev(ediktRoot, src string) error {
	devBin := filepath.Join(src, "bin", "edikt")
	if _, err := os.Stat(devBin); err != nil {
		return fmt.Errorf("dev source has no bin/edikt at %s — build it with `cd %s/tools/edikt && go build -o ../../bin/edikt .`", devBin, src)
	}
	launcherBin := filepath.Join(ediktRoot, "bin", "edikt")
	if err := os.MkdirAll(filepath.Dir(launcherBin), 0o755); err != nil {
		return fmt.Errorf("creating bin dir: %w", err)
	}
	// Backup existing launcher if present (and not already backed up
	// from a previous dev link in this same dev session).
	if _, err := os.Stat(launcherBin); err == nil {
		backup := launcherBin + ".pre-dev"
		// Always overwrite the backup so the most recent installed-
		// launcher state is what dev unlink restores. If a previous
		// dev session left a .pre-dev backup that points at an even
		// earlier launcher, that's stale — we want the most recent.
		_ = os.Remove(backup)
		if err := copyFileBytes(launcherBin, backup); err != nil {
			return fmt.Errorf("backing up launcher: %w", err)
		}
	}
	return copyFileBytes(devBin, launcherBin)
}

// copyFileBytes copies a file by reading then writing, preserving the
// executable bit. Used by refreshLauncherForDev to swap binaries
// without relying on rename (the source and dest can be on different
// volumes when the dev source is on a different mount than ~/.edikt).
func copyFileBytes(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	tmp := dst + fmt.Sprintf(".new.%d", os.Getpid())
	if err := os.WriteFile(tmp, data, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
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

		// Restore the launcher binary backup if dev link captured
		// one. Without this, `edikt` on PATH stays the dev binary
		// even after dev unlink, which makes the unlink confusing.
		launcherBin := filepath.Join(ediktRoot, "bin", "edikt")
		backup := launcherBin + ".pre-dev"
		if _, err := os.Stat(backup); err == nil {
			if err := copyFileBytes(backup, launcherBin); err != nil {
				fmt.Fprintf(os.Stderr, "warn: could not restore launcher from %s: %v\n", backup, err)
			} else {
				_ = os.Remove(backup)
			}
		}

		// Refresh the convenience symlinks for the new active version.
		repairExternalSymlinks(ediktRoot)

		emitEvent(ediktRoot, "dev_unlinked", map[string]interface{}{"reverted_to": fallbackTag})
		fmt.Fprintf(os.Stderr, "dev mode removed; activated %s\n", fallbackTag)
		return nil
	},
}

func init() {
	devCmd.AddCommand(devLinkCmd)
	devCmd.AddCommand(devUnlinkCmd)
	rootCmd.AddCommand(devCmd)
}
