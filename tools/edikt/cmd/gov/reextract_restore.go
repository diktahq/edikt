package gov

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/reextract"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/spf13/cobra"
)

var (
	restorePinnedFrom   string
	restorePinnedDryRun bool
	restorePinnedJSON   bool
)

// restorePinnedCmd repairs sidecars that were regenerated BEFORE the
// preservation step existed.
//
// It is a one-shot repair, not a routine verb: `gov reextract` now preserves
// pinned state inline, so a batch run after this never needs it. It exists
// because the first full-corpus run destroyed 48 pinned fields across 20
// artifacts, and the honest repair reads them back from the commit that
// carried them rather than asking a human to retype approvals the tooling
// lost.
var restorePinnedCmd = &cobra.Command{
	Use:   "restore-pinned [project-root]",
	Short: "Restore human-pinned sidecar fields from a git ref (one-shot repair)",
	Long: `Reads each sidecar as it stood at --from and restores the human-pinned
fields the regeneration dropped: approved paths:, the paths_approval receipt,
and per-directive verify / verify_kind / human_approved_at / fixture paths.

Matching runs exact-text first, then an unambiguous normalized-subject match.
A pin that can be matched neither way is REPORTED, never attached to a best
guess — a sidecar claiming a human approved a command against a rule they never
saw it on is worse than one missing the command.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		if restorePinnedFrom == "" {
			return fmt.Errorf("--from <git-ref> is required: there is nothing to restore pinned state FROM")
		}

		abs, err := filepath.Abs(root)
		if err != nil {
			return err
		}

		var restored, scanned int
		var unrestorable []reextract.PinnedField
		rewritten := map[string]string{}

		files, err := gitLsFiles(abs, "docs")
		if err != nil {
			return err
		}
		for _, rel := range files {
			if !strings.HasSuffix(rel, ".edikt.yaml") {
				continue
			}
			full := filepath.Join(abs, rel)
			if _, statErr := os.Stat(full); statErr != nil {
				continue
			}
			raw, gerr := gitShow(abs, restorePinnedFrom, rel)
			if gerr != nil || strings.TrimSpace(raw) == "" {
				continue // not present at that ref — nothing pinned to carry
			}
			// Loaded through a temp file rather than a bytes helper: Load is
			// the only loader with the project's KnownFields and error
			// reporting, and adding a second entry point for one caller is how
			// a project ends up with two definitions of "valid".
			tmpBefore := filepath.Join(os.TempDir(), "edikt-restore-before.edikt.yaml")
			if werr := os.WriteFile(tmpBefore, []byte(raw), 0o644); werr != nil {
				continue
			}
			before, perr := sidecar.Load(tmpBefore)
			os.Remove(tmpBefore)
			if perr != nil || before == nil {
				continue
			}
			scanned++

			id := artifactIDFromPath(rel)
			if restorePinnedDryRun {
				// Report what WOULD change without touching the tree: the
				// dry-run must not be the thing that performs the repair.
				tmp := filepath.Join(os.TempDir(), "edikt-restore-dryrun.edikt.yaml")
				cur, rerr := os.ReadFile(full)
				if rerr != nil {
					continue
				}
				if werr := os.WriteFile(tmp, cur, 0o644); werr != nil {
					continue
				}
				// beforeLoadErr is always nil here: `before` was already
				// confirmed loadable at line 87 above (perr != nil skips this
				// artifact before reaching this call) — this command IS the
				// manual recovery path A1 exists for, so "the reference
				// commit's sidecar didn't parse" has nowhere further to
				// escalate to; skipping to the next artifact is correct.
				res, rerr2 := reextract.PreservePinned(id, before, nil, tmp)
				os.Remove(tmp)
				if rerr2 != nil {
					fmt.Fprintf(os.Stderr, "  %s: %v\n", id, rerr2)
					continue
				}
				if res.SidecarRewritten {
					restored++
					if !restorePinnedJSON {
						fmt.Fprintf(os.Stdout, "  would restore %s: %d directive pin(s)%s\n",
							id, res.DirectivePins, pathsNote(res.PathsRestored))
					}
				}
				unrestorable = append(unrestorable, res.Unrestorable...)
				continue
			}

			// Same reasoning as the dry-run branch above: `before` is already
			// confirmed loadable by the time this line runs.
			res, rerr := reextract.PreservePinned(id, before, nil, full)
			if rerr != nil {
				fmt.Fprintf(os.Stderr, "  %s: %v\n", id, rerr)
				continue
			}
			if res.SidecarRewritten {
				restored++
				rewritten[id] = full
				if !restorePinnedJSON {
					fmt.Fprintf(os.Stdout, "  restored %s: %d directive pin(s)%s\n",
						id, res.DirectivePins, pathsNote(res.PathsRestored))
				}
			}
			unrestorable = append(unrestorable, res.Unrestorable...)
		}

		// Re-stamp the ledger for what was rewritten. Without this the repair
		// silently un-completes every artifact it fixed, and the next --force
		// re-dispatches work that is already done and correct.
		restamped := 0
		if !restorePinnedDryRun && len(rewritten) > 0 {
			var rerr error
			restamped, rerr = reextract.RestampLedger(abs, rewritten)
			if rerr != nil {
				fmt.Fprintf(os.Stderr, "  warn: ledger re-stamp failed: %v\n", rerr)
			}
		}

		if restorePinnedJSON {
			out := map[string]any{
				"ledger_restamped": restamped,
				"from":             restorePinnedFrom, "scanned": scanned,
				"restored": restored, "unrestorable": unrestorable,
			}
			b, _ := json.MarshalIndent(out, "", "  ")
			fmt.Fprintln(os.Stdout, string(b))
		} else {
			fmt.Fprintf(os.Stdout, "restore-pinned: %d sidecar(s) scanned at %s, %d restored, %d re-stamped in the ledger, %d unrestorable pin(s)\n",
				scanned, restorePinnedFrom, restored, restamped, len(unrestorable))
			for _, u := range unrestorable {
				fmt.Fprintf(os.Stdout, "  UNRESTORABLE %s\n", u.String())
			}
		}

		// Unrestorable pins are a REPORT, not a failure: the command did
		// everything that could be done correctly, and exiting non-zero would
		// make the repair look like it did not run.
		return nil
	},
}

func pathsNote(b bool) string {
	if b {
		return ", approved paths"
	}
	return ""
}

func artifactIDFromPath(rel string) string {
	base := filepath.Base(rel)
	base = strings.TrimSuffix(base, ".edikt.yaml")
	for i, r := range base {
		if r == '-' && i > 2 {
			// ADR-042-no-cross-version -> ADR-042 for the ADR/INV shapes;  edikt-guard:allow
			// guidelines keep their full slug, which is their id.
			if strings.HasPrefix(base, "ADR-") || strings.HasPrefix(base, "INV-") {
				if j := strings.Index(base[4:], "-"); j >= 0 {
					return base[:4+j]
				}
			}
			break
		}
	}
	return base
}

func gitLsFiles(root, path string) ([]string, error) {
	out, err := exec.Command("git", "-C", root, "ls-files", path).Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var files []string
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			files = append(files, l)
		}
	}
	return files, nil
}

func gitShow(root, ref, path string) (string, error) {
	out, err := exec.Command("git", "-C", root, "show", ref+":"+path).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func init() {
	restorePinnedCmd.Flags().StringVar(&restorePinnedFrom, "from", "", "git ref whose sidecars carry the pinned state (required)")
	restorePinnedCmd.Flags().BoolVar(&restorePinnedDryRun, "dry-run", false, "report what would be restored without writing")
	restorePinnedCmd.Flags().BoolVar(&restorePinnedJSON, "json", false, "emit JSON")
	reextractCmd.AddCommand(restorePinnedCmd)
}
