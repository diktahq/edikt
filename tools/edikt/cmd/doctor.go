package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run health checks on the edikt installation",
	Long: `Checks:
  - python3 on PATH (ERROR if absent — required by hooks since v0.5.0, INV-003)
  - cosign on PATH (INFO if absent — needed for install/upgrade, ADR-016)
  - $EDIKT_ROOT/current symlink resolves
  - active version manifest exists
  - $CLAUDE_ROOT/commands/edikt symlink resolves

Exits 0 (healthy), 1 (warnings), or 2 (errors).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return err
		}
		claudeRoot := resolveClaudeRoot()

		errN := 0
		warnN := 0

		fmt.Println("edikt doctor")
		fmt.Printf("  EDIKT_ROOT:  %s\n", ediktRoot)
		fmt.Printf("  CLAUDE_ROOT: %s\n", claudeRoot)

		// Check EDIKT_ROOT exists and is writable.
		if _, err := os.Stat(ediktRoot); os.IsNotExist(err) {
			fmt.Println("  ERROR: EDIKT_ROOT does not exist")
			errN++
		} else {
			// Simple writability check: try to open for write.
			testPath := filepath.Join(ediktRoot, ".doctor-write-test")
			if f, err := os.OpenFile(testPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err != nil {
				fmt.Printf("  ERROR: EDIKT_ROOT is not writable (%v)\n", err)
				errN++
			} else {
				f.Close()
				os.Remove(testPath)
			}
		}

		// python3 check (ERROR — required by hooks INV-003 since v0.5.0).
		if pyPath, err := exec.LookPath("python3"); err != nil {
			fmt.Println("  ERROR: python3 not on PATH — edikt hooks require python3 (INV-003 since v0.5.0)")
			errN++
		} else {
			// Get version string.
			out, err := exec.Command(pyPath, "-c", "import sys; print('.'.join(str(x) for x in sys.version_info[:2]))").Output()
			ver := "unknown"
			if err == nil {
				ver = string(out)
				if len(ver) > 0 && ver[len(ver)-1] == '\n' {
					ver = ver[:len(ver)-1]
				}
			}
			fmt.Printf("  python3:     %s (%s)\n", pyPath, ver)
		}

		// cosign check (INFO — optional per ADR-016).
		if cosignPath, err := exec.LookPath("cosign"); err != nil {
			fmt.Println("  INFO: cosign not on PATH — installs require EDIKT_INSTALL_INSECURE=1 without it (ADR-016)")
		} else {
			fmt.Printf("  cosign:      %s\n", cosignPath)
		}

		// current symlink check.
		currentLink := filepath.Join(ediktRoot, "current")
		if info, err := os.Lstat(currentLink); err != nil {
			fmt.Println("  WARN: no active version (current symlink missing)")
			warnN++
		} else if info.Mode()&os.ModeSymlink == 0 {
			fmt.Println("  WARN: current is not a symlink")
			warnN++
		} else {
			// Check that the symlink resolves.
			if _, err := os.Stat(currentLink); err != nil {
				fmt.Println("  ERROR: current symlink does not resolve")
				errN++
			} else {
				link, _ := os.Readlink(currentLink)
				activeTag := filepath.Base(link)
				fmt.Printf("  active:      %s\n", activeTag)

				// Check manifest.
				manifestPath := filepath.Join(ediktRoot, "current", "manifest.yaml")
				if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
					fmt.Println("  WARN: no manifest.yaml in active version")
					warnN++
				} else {
					fmt.Println("  manifest:    present")
				}
			}
		}

		// $CLAUDE_ROOT/commands/edikt symlink check.
		ediktCmds := filepath.Join(claudeRoot, "commands", "edikt")
		if linfo, err := os.Lstat(ediktCmds); err != nil {
			fmt.Printf("  WARN: %s is not a symlink\n", ediktCmds)
			warnN++
		} else if linfo.Mode()&os.ModeSymlink == 0 {
			fmt.Printf("  WARN: %s is not a symlink\n", ediktCmds)
			warnN++
		} else {
			if _, err := os.Stat(ediktCmds); err != nil {
				fmt.Printf("  ERROR: %s does not resolve\n", ediktCmds)
				errN++
			} else {
				fmt.Printf("  commands:    %s (ok)\n", ediktCmds)
			}
		}

		// Summary.
		if errN > 0 {
			fmt.Printf("result: %d errors, %d warnings\n", errN, warnN)
			os.Exit(2)
		} else if warnN > 0 {
			fmt.Printf("result: %d warnings\n", warnN)
			os.Exit(1)
		} else {
			fmt.Println("result: healthy")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
