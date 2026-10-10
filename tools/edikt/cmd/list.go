package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var listVerbose bool
var listGlobal bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed edikt versions",
	Long:  `Scans $EDIKT_ROOT/versions/ and prints installed version directories sorted alphabetically. The active version is marked with *.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return err
		}

		listRoot := ediktRoot
		if listGlobal {
			home := os.Getenv("HOME")
			if ediktHome := os.Getenv("EDIKT_HOME"); ediktHome != "" {
				listRoot = ediktHome
			} else if home != "" {
				listRoot = filepath.Join(home, ".edikt")
			}
		}

		versionsDir := filepath.Join(listRoot, "versions")
		if _, err := os.Stat(versionsDir); os.IsNotExist(err) {
			printNoVersionsInstalled()
			return nil
		}

		// Determine active version from lock.yaml or current symlink.
		activeTag := ""
		if lf, err := readLock(listRoot); err == nil && lf.Active != "" {
			activeTag = lf.Active
		} else if link, err := os.Readlink(filepath.Join(listRoot, "current")); err == nil {
			activeTag = filepath.Base(link)
		}

		// Collect installed versions.
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
		sort.Strings(tags)

		if len(tags) == 0 {
			printNoVersionsInstalled()
			return nil
		}

		for _, tag := range tags {
			marker := " "
			if tag == activeTag || tag == "v"+activeTag || "v"+tag == activeTag {
				marker = "*"
			}
			if listVerbose {
				dir := filepath.Join(versionsDir, tag)
				installedAt := "-"
				manifestPath := filepath.Join(dir, "manifest.yaml")
				if data, err := os.ReadFile(manifestPath); err == nil {
					for _, line := range strings.Split(string(data), "\n") {
						if strings.HasPrefix(line, "installed_at:") {
							val := strings.TrimSpace(strings.TrimPrefix(line, "installed_at:"))
							installedAt = strings.Trim(val, `"`)
							break
						}
					}
				}
				fmt.Printf("%s %s\t%s\n", marker, tag, installedAt)
			} else {
				fmt.Printf("%s %s\n", marker, tag)
			}
		}
		return nil
	},
}

func init() {
	listCmd.Flags().BoolVarP(&listVerbose, "verbose", "v", false, "show installed_at timestamp")
	listCmd.Flags().BoolVar(&listGlobal, "global", false, "list from global ~/.edikt/versions/")
	rootCmd.AddCommand(listCmd)
}

// printNoVersionsInstalled explains the empty state instead of stating it.
//
// `brew install edikt` ships the LAUNCHER ONLY — the payload it runs is a
// separate download. A user who has just installed via Homebrew reasonably
// believes edikt is installed, runs `list`, and gets "(no versions installed)"
// with nowhere to go. One reporter had to discover `edikt install` unaided.
//
// Documentation cannot fix this: the user is in a terminal, not reading the
// README. `install` already handles its own equivalent moment correctly —
// "installed %s (not activated — run `edikt use %s` to activate)" — and this is
// the same treatment for the other end of the flow.
func printNoVersionsInstalled() {
	fmt.Println("(no versions installed)")
	fmt.Println()
	fmt.Println("The launcher is installed but no payload is. If you installed with")
	fmt.Println("Homebrew, that is expected — a formula ships the binary alone.")
	fmt.Println()
	fmt.Println("  edikt install v0.8.0    # fetch the payload")
	fmt.Println("  edikt use v0.8.0        # activate it and link the /edikt:* commands")
}
