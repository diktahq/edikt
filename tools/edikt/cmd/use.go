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

		// Minimum payload version gate: payloads below 0.5.0 are not supported.
		const minPayloadVersion = "0.5.0"
		if isSemverTag(tag) && !semverGreater(tag, minPayloadVersion) && tag != minPayloadVersion {
			return &exitCodeError{code: 2, msg: fmt.Sprintf(
				"version %s is below the minimum supported payload version (%s)",
				tag, minPayloadVersion,
			)}
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

		// Repair external symlinks: hooks/ and templates/ inside EDIKT_ROOT,
		// and commands/edikt inside CLAUDE_ROOT.
		repairExternalSymlinks(ediktRoot)

		emitEvent(ediktRoot, "version_activated", map[string]interface{}{"version": tag})

		fmt.Fprintf(os.Stderr, "activated %s\n", tag)
		return nil
	},
}

// repairExternalSymlinks ensures that convenience symlinks outside the versioned
// directory resolve through the current/ symlink chain after a version switch.
//
// $EDIKT_ROOT/hooks      → current/hooks
// $EDIKT_ROOT/templates  → current/templates
// $CLAUDE_ROOT/commands/edikt → $EDIKT_ROOT/current/commands/edikt
func repairExternalSymlinks(ediktRoot string) {
	claudeRoot := resolveClaudeRoot()

	// Local convenience symlinks (relative, so they work wherever EDIKT_ROOT is).
	// Layout detection for hooks: v0.4.x had hooks/ at payload root, v0.5.x
	// keeps hooks under templates/hooks/.
	templatesLink := filepath.Join(ediktRoot, "templates")
	replaceSymlink(templatesLink, filepath.Join("current", "templates"))

	hooksLink := filepath.Join(ediktRoot, "hooks")
	hooksRootTarget := filepath.Join(ediktRoot, "current", "hooks")
	hooksTemplatesTarget := filepath.Join(ediktRoot, "current", "templates", "hooks")
	hooksTargetRel := filepath.Join("current", "hooks")
	if _, err := os.Stat(hooksRootTarget); err != nil {
		if _, err := os.Stat(hooksTemplatesTarget); err == nil {
			hooksTargetRel = filepath.Join("current", "templates", "hooks")
		}
	}
	replaceSymlink(hooksLink, hooksTargetRel)

	// Claude commands symlink (absolute path required across directory boundaries).
	// Detect payload layout:
	//   nested (v0.4.x): current/commands/edikt/ is a directory → target it
	//   flat   (v0.5.x): current/commands/*.md directly         → target current/commands
	ediktCmds := filepath.Join(claudeRoot, "commands", "edikt")
	nestedTarget := filepath.Join(ediktRoot, "current", "commands", "edikt")
	flatTarget := filepath.Join(ediktRoot, "current", "commands")
	ediktCmdsTarget := flatTarget
	if info, err := os.Stat(nestedTarget); err == nil && info.IsDir() {
		ediktCmdsTarget = nestedTarget
	}
	if err := os.MkdirAll(filepath.Join(claudeRoot, "commands"), 0o755); err == nil {
		replaceSymlink(ediktCmds, ediktCmdsTarget)
	}
}

// replaceSymlink atomically replaces (or creates) a symlink at dst pointing to target.
func replaceSymlink(dst, target string) {
	tmp := dst + fmt.Sprintf(".new.%d", os.Getpid())
	os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
	}
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
