package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the active edikt payload version",
	Long: `Reports the currently active edikt version from $EDIKT_ROOT/current/VERSION.
If no version is installed, reports "no version installed".
Use --binary to print the edikt binary version instead.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if binaryFlag, _ := cmd.Flags().GetBool("binary"); binaryFlag {
			fmt.Println(Version)
			return
		}

		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "no version installed")
			return
		}

		// Try lock.yaml first.
		if lf, err := readLock(ediktRoot); err == nil && lf.Active != "" {
			fmt.Println(lf.Active)
			return
		}

		// Fall back to current/VERSION.
		data, err := os.ReadFile(filepath.Join(ediktRoot, "current", "VERSION"))
		if err != nil {
			fmt.Println("no version installed")
			return
		}
		fmt.Println(strings.TrimSpace(string(data)))
	},
}

func init() {
	versionCmd.Flags().Bool("binary", false, "print the edikt binary version instead of the active payload version")
	rootCmd.AddCommand(versionCmd)
}
