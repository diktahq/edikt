package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var doctorQuick bool

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

		fmt.Println("edikt doctor")
		fmt.Printf("  EDIKT_ROOT:  %s\n", ediktRoot)
		fmt.Printf("  CLAUDE_ROOT: %s\n", claudeRoot)

		// --quick: only print root paths and exit 0.
		if doctorQuick {
			return nil
		}

		errN := 0
		warnN := 0

		// Check for interrupted migration leftovers.
		if entries, err := os.ReadDir(ediktRoot); err == nil {
			for _, e := range entries {
				n := e.Name()
				if strings.HasPrefix(n, ".migrate-staging-") || strings.HasPrefix(n, ".pre-migration-") {
					fmt.Printf("  ERROR: interrupted migration detected (%s) — run `edikt migrate --abort` to restore\n", n)
					errN++
					break
				}
			}
		}

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
					fmt.Println("  manifest:    OK")
				}

				// Check that the templates symlink resolves through the chain.
				templatesLink := filepath.Join(ediktRoot, "templates")
				if _, err := os.Stat(templatesLink); err != nil {
					fmt.Println("  ERROR: templates symlink does not resolve — active version may be corrupt")
					errN++
				}

				// Verify SHA256SUMS integrity if present.
				sumsPath := filepath.Join(ediktRoot, "current", "SHA256SUMS")
				if _, err := os.Stat(sumsPath); err == nil {
					if tampered, err := checkPayloadIntegrity(filepath.Join(ediktRoot, "current"), sumsPath); err != nil {
						fmt.Printf("  ERROR: manifest integrity check failed: %v\n", err)
						errN++
					} else if len(tampered) > 0 {
						fmt.Printf("  ERROR: manifest integrity check failed: %d file(s) modified\n", len(tampered))
						for _, f := range tampered {
							fmt.Printf("    tampered: %s\n", f)
						}
						errN++
					}
				}
			}
		}

		// $CLAUDE_ROOT/commands/edikt symlink check.
		// Missing = acceptable (fresh install). Exists+not-symlink = WARN.
		// Exists+symlink+broken = ERROR. Exists+symlink+resolves = OK.
		ediktCmds := filepath.Join(claudeRoot, "commands", "edikt")
		linfo, lerr := os.Lstat(ediktCmds)
		if os.IsNotExist(lerr) {
			// Not yet set up — fine for fresh installs, skip.
		} else if lerr != nil {
			fmt.Printf("  WARN: could not stat commands/edikt: %v\n", lerr)
			warnN++
		} else if linfo.Mode()&os.ModeSymlink == 0 {
			fmt.Printf("  WARN: %s is not a symlink\n", ediktCmds)
			warnN++
		} else if _, err := os.Stat(ediktCmds); err != nil {
			fmt.Printf("  ERROR: %s does not resolve\n", ediktCmds)
			errN++
		} else {
			fmt.Printf("  commands:    %s (ok)\n", ediktCmds)
		}

		// Sidecar Health (Phase 7 of PLAN-sidecar-architecture).
		// Walks the project (cwd) for orphan / missing / path-mismatch /
		// schema-invalid / empty-directives sidecars. Silent if cwd is not
		// an edikt project (no artifact dirs visible).
		if cwd, cwdErr := os.Getwd(); cwdErr == nil {
			scErr, scWarn, ran := runSidecarChecks(cwd, os.Stdout)
			if ran {
				errN += scErr
				warnN += scWarn
			}

			// Plan Verification (Phase 12 of PLAN-sidecar-architecture).
			// Soft check — never increments errN.
			vWarn, vRan := runVerifyChecks(cwd, os.Stdout)
			if vRan {
				warnN += vWarn
			}

			// Rejected Options Coverage (Phase 4 of PLAN-v060-governance-accuracy).
			// Warns when an ADR has 2+ considered options but no MUST NOT
			// directives — INV-002 honor: remediation cites manual_directives,
			// never an ADR-body edit.
			roWarn, roRan := runRejectedOptionsCheck(cwd, os.Stdout)
			if roRan {
				warnN += roWarn
			}

			// Orphan Manual Refs (Phase 8 of PLAN-v060-governance-accuracy).
			// Manual directives that cite a non-existent ADR file are
			// surfaced as ORPHAN findings. INV-006: ArtifactID is
			// validated before the filesystem lookup.
			omWarn, omRan := runOrphanManualRefCheck(cwd, os.Stdout)
			if omRan {
				warnN += omWarn
			}
		}

		// settings.json placeholder check — Claude Code does not expand env
		// vars in `command:` strings, so an unsubstituted ${EDIKT_HOOK_DIR}
		// or $HOME makes hooks fail with `/bin/sh: /<hook>.sh: No such file`.
		// Check both global (~/.claude/settings.json) and the project-local
		// (.claude/settings.json) if we're inside a project.
		for _, candidate := range []string{
			filepath.Join(claudeRoot, "settings.json"),
			filepath.Join(".", ".claude", "settings.json"),
		} {
			data, err := os.ReadFile(candidate)
			if err != nil {
				continue
			}
			content := string(data)
			placeholders := []string{"${EDIKT_HOOK_DIR}", "$HOME/.edikt"}
			for _, p := range placeholders {
				if strings.Contains(content, p) {
					fmt.Printf("  ERROR: %s contains unsubstituted placeholder %q — hooks will fail. Re-run /edikt:init to regenerate, or substitute manually.\n",
						candidate, p)
					errN++
					break
				}
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

// checkPayloadIntegrity reads SHA256SUMS and verifies each listed file.
// Returns a list of tampered file paths and any I/O error.
func checkPayloadIntegrity(dir, sumsPath string) ([]string, error) {
	f, err := os.Open(sumsPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var tampered []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			continue
		}
		expected, relPath := parts[0], parts[1]
		absPath := filepath.Join(dir, relPath)
		actual, err := sha256File(absPath)
		if err != nil || actual != expected {
			tampered = append(tampered, relPath)
		}
	}
	return tampered, sc.Err()
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorQuick, "quick", false, "print EDIKT_ROOT and CLAUDE_ROOT only (no checks)")
	rootCmd.AddCommand(doctorCmd)
}
