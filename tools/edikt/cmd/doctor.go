package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	doctorQuick          bool
	doctorCheckSnapshot  bool
	doctorFailOnDrift    bool
	doctorCreateSnapshot bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run health checks on the edikt installation",
	Long: `Checks:
  - python3 on PATH (ERROR if absent — required by hooks since v0.5.0)
  - cosign on PATH (INFO if absent — needed for install/upgrade signing)
  - $EDIKT_ROOT/current symlink resolves
  - active version manifest exists
  - $CLAUDE_ROOT/commands/edikt symlink resolves

Exits 0 (healthy), 1 (warnings), or 2 (errors).

With --check-snapshot, the standard checks are suppressed and doctor
compares the compiled governance tree (.claude/rules/governance{,/*}.md)
against the project's frozen baseline at
.edikt/snapshot/governance-checksums.txt. Useful for detecting drift
between deliberate governance updates. Use --create-snapshot to write
a fresh baseline; --fail-on-drift makes drift exit 1.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// --check-snapshot suppresses the standard health checks and
		// becomes the sole subject of this invocation. The flag is
		// adopter-facing (SPEC-009 Plan D Phase 7 / SR-015 — the one  // edikt-guard:allow
		// Layer-0 mechanism we expose to userland).
		if doctorCheckSnapshot {
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("doctor --check-snapshot: getwd: %w", err)
			}
			rc := runSnapshotCheck(projectRoot, snapshotOpts{
				FailOnDrift:    doctorFailOnDrift,
				CreateSnapshot: doctorCreateSnapshot,
			}, os.Stdout)
			if rc != 0 {
				os.Exit(rc)
			}
			return nil
		}

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

		// A CLAUDE_HOME override that disagrees with Claude Code's own
		// profile selector means edikt manages files no session under the
		// active profile ever reads — the silent-failure case.
		if ccd, ok := claudeConfigDirMismatch(claudeRoot); ok {
			fmt.Printf("  WARN: CLAUDE_CONFIG_DIR=%s disagrees with resolved CLAUDE_ROOT (CLAUDE_HOME wins) — edikt-managed files may not be read by the active Claude profile\n", ccd)
			warnN++
		}

		// Check for interrupted migration leftovers.
		if entries, err := os.ReadDir(ediktRoot); err == nil {
			for _, e := range entries {
				n := e.Name()
				if strings.HasPrefix(n, ".migrate-staging-") || strings.HasPrefix(n, ".pre-migration-") {
					// --abort is a recovery primitive that has no slash wrapper today.
					// Surface both: slash flow for the normal retry; binary --abort
					// for the explicit "restore from backup and stop" path.
					fmt.Printf("  ERROR: interrupted migration detected (%s) — run /edikt:upgrade in Claude Code to retry, or 'edikt migrate --abort' directly to restore from backup\n", n)
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

		// python3 check (ERROR — required by hooks since v0.5.0).
		if pyPath, err := exec.LookPath("python3"); err != nil {
			fmt.Println("  ERROR: python3 not on PATH — edikt hooks require python3 (since v0.5.0)")
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

		// cosign check (INFO — optional).
		if cosignPath, err := exec.LookPath("cosign"); err != nil {
			fmt.Println("  INFO: cosign not on PATH — installs require EDIKT_INSTALL_INSECURE=1 without it")
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
			// Agent-definition drift: a stale installed copy of a system
			// agent silently shadows the payload template and produces
			// zero-file extractor dispatches (cached at session start).
			warnN += runAgentDriftCheck(ediktRoot, claudeRoot, cwd, os.Stdout)

			// Dual-scope hook registration (ADR-050): detect + OFFER the
			// user-scope removal interactively; never edit silently.
			warnN += runDualRegistrationCheck(claudeRoot, cwd, os.Stdin,
				term.IsTerminal(int(os.Stdin.Fd())), os.Stdout)

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
			// directives — remediation cites manual_directives,
			// never an ADR-body edit.
			roWarn, roRan := runRejectedOptionsCheck(cwd, os.Stdout)
			if roRan {
				warnN += roWarn
			}

			// Orphan Manual Refs (Phase 8 of PLAN-v060-governance-accuracy).
			// Manual directives that cite a non-existent ADR file are
			// surfaced as ORPHAN findings. ArtifactID is validated before
			// the filesystem lookup.
			omWarn, omRan := runOrphanManualRefCheck(cwd, os.Stdout)
			if omRan {
				warnN += omWarn
			}

			// PRD/SPEC sidecar schema drift. Walks paths.prds / paths.specs
			// for sidecar YAML files and flags any that lack schema-required
			// fields (e.g., a PRD sidecar written before the slug or _sync
			// field was added). Soft warning — surfaces drift without
			// blocking doctor's exit. Gov-sidecars are already covered by
			// sidecar.Load()'s KnownFields(true) decoder in the
			// "Sidecar Health" group above.
			sdWarn, sdRan := runSchemaDriftCheck(cwd, os.Stdout)
			if sdRan {
				warnN += sdWarn
			}

			// Sidecar Verify Coverage (Phase 4 of PLAN-v060-completion-evidence).
			// Reports per-sidecar verify health + project-wide coverage as
			// soft warnings. Gov compile owns the hard gate; doctor's role
			// here is informational pressure on low-coverage sidecars and
			// silently-failing verifies.
			vcWarn, vcRan := runVerifyCoverageCheck(cwd, os.Stdout)
			if vcRan {
				warnN += vcWarn
			}

			// Verify Kind Coverage (Phase 10 of SPEC-009 Plan A).  // edikt-guard:allow
			// Per-corpus tally of structural / tooling / behavioral verify_kind
			// values + WARN per behavioral entry lacking human_approved_at.
			// Soft surface — the hard gate lives at Phase B compile and
			// `bin/edikt sidecar approve` (ADR-039).  // edikt-guard:allow
			vkWarn, vkRan := runVerifyKindCoverageCheck(cwd, os.Stdout)
			if vkRan {
				warnN += vkWarn
			}

			// Verify Gate posture (SPEC-009 Plan B Phase 5, ADR-038).  // edikt-guard:allow
			// Reports ENABLED / BYPASSED and tail-counts recent bypass
			// events from .edikt/state/verify-gate.jsonl. Informational.
			vgWarn, vgRan := runVerifyGateCheck(cwd, os.Stdout)
			if vgRan {
				warnN += vgWarn
			}

			// Cheat-rate benchmarks (Phase 10 of SPEC-009 Plan A).  // edikt-guard:allow
			// Reports the contents of .edikt/state/benchmark/. Empty / absent
			// emits a Plan-C-stub line so adopters see the baseline gap.
			cbWarn, cbRan := runCheatRateBenchmarkCheck(cwd, os.Stdout)
			if cbRan {
				warnN += cbWarn
			}

			// Compiled-governance quality grade (SPEC-009 Plan H, SR-016).  // edikt-guard:allow
			// Reports the latest grade under .edikt/state/compile-quality/.
			// Advisory: warns on low-scoring dimensions, never blocks exit.
			cqWarn, cqRan := runCompileQualityCheck(cwd, os.Stdout)
			if cqRan {
				warnN += cqWarn
			}

			// edikt_version vs corpus schema disagreement — backstop for the
			// upgrade-flow gate that withholds the version bump until
			// migration completes (commands/upgrade.md Step 6). Catches a
			// manually edited edikt_version or an upgrade interrupted after
			// the version write.
			warnN += runSchemaPinCheck(cwd, os.Stdout)

			// Orphaned governance surfaces — a .md file under
			// .claude/rules/governance/ that gov compile's manifest-diff
			// cleanup cannot see (fell outside manifest tracking) but that
			// still loads as an ambient rule regardless.
			if osWarn, osRan := runOrphanSurfacesCheck(cwd, os.Stdout); osRan {
				warnN += osWarn
			}

			// Pending topic descriptions — a topic with no approved
			// description renders a non-functional routing placeholder
			// into its skill package and topic file. The one-shot compile
			// summary line reports this once; doctor makes it a standing,
			// repeatable signal so the backlog doesn't go invisible again.
			if tdWarn, tdRan := runTopicDescriptionsCheck(cwd, os.Stdout); tdRan {
				warnN += tdWarn
			}

			// Topic-scope integrity — shadow ambient core (paths: "**") and
			// topics retired to tier 3 with no topic file. Both are
			// authored-source problems (missing paths: globs), but a
			// project can reach either state silently; this is the
			// standing signal the one-shot compile summary line doesn't
			// provide.
			if tsWarn, tsRan := runTopicScopeCheck(cwd, os.Stdout); tsRan {
				warnN += tsWarn
			}

			// Routed sources — replaces the
			// python heredoc previously embedded in commands/doctor.md
			// (Phase 11.5 of PLAN-v060-governance-accuracy). Validates
			// that every cited ADR/INV in the routing surface
			// resolves to a source file under paths.{decisions,invariants}.
			rsErr, _, rsRan := runRoutedSourcesCheck(cwd, os.Stdout)
			if rsRan {
				errN += rsErr
			}

			// statusLine.type validation (Phase 11.5). A missing type
			// field invalidates the WHOLE settings.json from Claude
			// Code's perspective; every hook stops firing too.
			slErr, _, _ := runStatusLineTypeCheck(cwd, os.Stdout)
			errN += slErr
		}

		// settings.json placeholder + hook-path resolution check. Check both
		// global (~/.claude/settings.json) and the project-local
		// (.claude/settings.json) if we're inside a project.
		for _, candidate := range []string{
			filepath.Join(claudeRoot, "settings.json"),
			filepath.Join(".", ".claude", "settings.json"),
		} {
			data, err := os.ReadFile(candidate)
			if err != nil {
				continue
			}
			hpErr, hpWarn := runHookPathCheck(candidate, string(data), os.Stdout)
			errN += hpErr
			warnN += hpWarn
		}

		// Post-flight Scope (post-flight review pipeline).  // edikt-guard:allow
		reportPostFlightScope()

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
// hookCommandRe pulls the command string out of a settings.json hook entry.
var hookCommandRe = regexp.MustCompile(`"command"\s*:\s*"([^"]+)"`)

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
	doctorCmd.Flags().BoolVar(&doctorCheckSnapshot, "check-snapshot", false,
		"compare compiled governance tree against the baseline at .edikt/snapshot/governance-checksums.txt (SR-015)")
	doctorCmd.Flags().BoolVar(&doctorFailOnDrift, "fail-on-drift", false,
		"with --check-snapshot, exit 1 when drift is detected (default: drift is reported but doctor exits 0)")
	doctorCmd.Flags().BoolVar(&doctorCreateSnapshot, "create-snapshot", false,
		"with --check-snapshot, write a fresh baseline from the current compiled governance tree")
	rootCmd.AddCommand(doctorCmd)
}
