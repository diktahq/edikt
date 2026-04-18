package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var upgradePinCmd = &cobra.Command{
	Use:   "upgrade-pin",
	Short: "Update edikt_version in the nearest .edikt/config.yaml to the active version",
	Long: `Walks up from the current directory to find .edikt/config.yaml.
Updates (or appends) the edikt_version field to match the currently active version.
All other content is preserved byte-for-byte.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return err
		}

		// Determine active version.
		lf, err := readLock(ediktRoot)
		if err != nil || lf.Active == "" {
			return fmt.Errorf("no active version — run `edikt install <tag>` first")
		}
		active := normalizeTag(lf.Active)

		// Find project config.
		configPath := findProjectConfig()
		if configPath == "" {
			return &exitCodeError{code: 1, msg: "not inside an edikt project (no .edikt/config.yaml found in ancestor directories)"}
		}

		if err := updateOrAppendPin(configPath, active); err != nil {
			return fmt.Errorf("updating config: %w", err)
		}

		emitEvent(ediktRoot, "version_pinned", map[string]interface{}{
			"version": active,
			"config":  configPath,
		})

		fmt.Fprintf(os.Stderr, "pinned edikt_version: %q in %s\n", active, configPath)
		return nil
	},
}

// updateOrAppendPin rewrites configPath, replacing the edikt_version line
// (if present) with the new value, or appending it (if absent).
// All other bytes are preserved.
func updateOrAppendPin(configPath, version string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	found := false
	newLine := fmt.Sprintf("edikt_version: %q", version)
	for i, line := range lines {
		if strings.HasPrefix(line, "edikt_version:") {
			lines[i] = newLine
			found = true
			break
		}
	}

	if !found {
		// Append at end (ensure trailing newline).
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = append(lines[:len(lines)-1], newLine, "")
		} else {
			lines = append(lines, newLine)
		}
	}

	result := strings.Join(lines, "\n")
	return os.WriteFile(configPath, []byte(result), 0o644)
}


func init() {
	rootCmd.AddCommand(upgradePinCmd)
}
