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
			fmt.Println("(no versions installed)")
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
			fmt.Println("(no versions installed)")
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
