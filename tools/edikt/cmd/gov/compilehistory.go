package gov

// compilehistory.go — `bin/edikt gov compile-history` subcommand.
//
// Pure deterministic state-machine wrapper around internal/orphan.
// Replaces the ~200-line Python heredoc previously embedded in
// commands/gov/compile.md (Phase 11.5 of PLAN-v060-governance-accuracy).
//
// Contract:
//   --orphans         comma-separated list of orphan IDs (ADR/INV/GUIDE).
//                     Empty string means "no orphans this run".
//   --history-path    absolute or project-relative path to the
//                     compile-history.json state file. Validated under
//                     INV-006: must stay within the project root and
//                     contain no `..` segments.
//   --edikt-version   optional version stamp written into the file.
//
// Exit codes:
//   0 — clean (warn or no-op)
//   1 — Consecutive scenario (BLOCK — same orphan set as the prior run)
//   2 — flag/argument validation error (INV-006 refusal)

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/orphan"
	"github.com/spf13/cobra"
)

// orphanIDPattern is INV-006's allowlist for the `--orphans` argv:
// `(ADR|INV|GUIDE)-[0-9]+`. The user-facing markdown injects the IDs
// from a sentinel-discovery pass; an attacker controlling the source
// document headers cannot break out of the regex without first
// corrupting the YAML frontmatter (caught upstream).
var orphanIDPattern = regexp.MustCompile(`^(ADR|INV|GUIDE)-\d+$`)

var (
	chOrphans       string
	chHistoryPath   string
	chEdiktVersion  string
	chQuiet         bool
)

var compileHistoryCmd = &cobra.Command{
	Use:   "compile-history",
	Short: "Apply the orphan-set transition rules and write compile-history.json",
	Long: `Implements the five-rule orphan-detection state machine for
gov:compile (FR-004 / AC-003 / AC-003b / AC-017 / AC-018):

  1. First detection   — no prior history. Warn, exit 0, write history.
  2. Consecutive       — prior set == current set. BLOCK (exit 1), do
                         not overwrite history.
  3. Subset / resolved — current ⊂ prior. Warn, write, exit 0.
  4. Superset          — current ⊃ prior. Warn, write, exit 0.
  5. Different reset   — sets differ but neither sub/superset. Warn,
                         write, exit 0.

Writes to .edikt/state/compile-history.json by default. Atomic: tmp +
rename, so a crash mid-write leaves the previous state file intact.

Per ADR-020 + ADR-030 this command is pure Go — no LLM dispatch, no
shell-out. Two consecutive invocations with the same inputs and a
pinned timestamp produce byte-equal output.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := runCompileHistory(cmd, args)
		exitFromExitErr(err)
		return err
	},
}

func init() {
	compileHistoryCmd.Flags().StringVar(&chOrphans, "orphans", "",
		"comma-separated orphan IDs (e.g. 'ADR-012,INV-003'). Empty = no orphans.")
	compileHistoryCmd.Flags().StringVar(&chHistoryPath, "history-path",
		".edikt/state/compile-history.json",
		"path to the compile-history JSON file (validated to stay under project root)")
	compileHistoryCmd.Flags().StringVar(&chEdiktVersion, "edikt-version", "",
		"optional version string written to the edikt_version field")
	compileHistoryCmd.Flags().BoolVar(&chQuiet, "quiet", false,
		"suppress prose output; emit nothing on success")
	Cmd.AddCommand(compileHistoryCmd)
}

func runCompileHistory(cmd *cobra.Command, args []string) error {
	root, err := os.Getwd()
	if err != nil {
		return &exitErr{code: 2, msg: fmt.Sprintf("getwd: %v", err)}
	}

	// INV-006: validate --history-path. Resolve absolute, refuse traversal,
	// keep the resolved path under the project root.
	if strings.Contains(chHistoryPath, "..") {
		return &exitErr{code: 2, msg: "--history-path must not contain '..'"}
	}
	historyAbs := chHistoryPath
	if !filepath.IsAbs(historyAbs) {
		historyAbs = filepath.Join(root, historyAbs)
	}
	historyAbs, err = filepath.Abs(historyAbs)
	if err != nil {
		return &exitErr{code: 2, msg: fmt.Sprintf("invalid --history-path: %v", err)}
	}
	if !strings.HasPrefix(historyAbs, root+string(os.PathSeparator)) && historyAbs != root {
		return &exitErr{code: 2, msg: fmt.Sprintf("--history-path must be under project root: %s", historyAbs)}
	}

	// INV-006: validate --orphans. Reject anything outside the allowlist.
	current, err := parseOrphanIDs(chOrphans)
	if err != nil {
		return &exitErr{code: 2, msg: err.Error()}
	}

	// Read prior history (corrupt → treat as absent and warn).
	prior, ok, _ := orphan.ReadStateDetailed(historyAbs)
	if !ok {
		if !chQuiet {
			fmt.Println("[WARN] compile-history.json is unparseable — treating as absent (first detection)")
		}
	}
	var priorList *[]string
	if ok && prior != nil {
		// Empty prior set is meaningfully different from missing
		// prior set: missing = first-detection scenario, empty =
		// "previous compile had no orphans". Surface that with
		// pointer-to-slice semantics matching internal/orphan.
		priorList = &prior.OrphanADRs
	}

	scenario, block, writeHist := orphan.Decide(current, priorList)

	// Emit warnings or block messages.
	if !chQuiet {
		emitDecision(os.Stdout, current, scenario, block)
	}

	// Atomic write of new history (skip when block — preserve baseline so
	// the next compile can detect "same set" again).
	if writeHist {
		if err := orphan.WriteState(historyAbs, current, chEdiktVersion); err != nil {
			// Per the python heredoc, write failures degrade to a
			// warning and exit 0 (the file is best-effort).
			if !chQuiet {
				fmt.Fprintf(os.Stderr, "[WARN] could not atomically write %s: %v\n", historyAbs, err)
			}
			return nil
		}
	}

	if block {
		return &exitErr{code: 1, msg: ""}
	}
	return nil
}

// parseOrphanIDs splits the comma-separated argv, trims whitespace,
// validates each non-empty entry against the allowlist, and returns a
// deduplicated sorted slice. Empty input yields a nil slice.
func parseOrphanIDs(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, tok := range strings.Split(s, ",") {
		id := strings.TrimSpace(tok)
		if id == "" {
			continue
		}
		if !orphanIDPattern.MatchString(id) {
			return nil, fmt.Errorf("invalid orphan ID %q: must match %s", id, orphanIDPattern.String())
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

// emitDecision prints the warning/block prose. Format matches the
// python heredoc's output line-for-line so downstream test fixtures
// and human callers don't drift.
func emitDecision(w *os.File, current []string, sc orphan.Scenario, block bool) {
	bw := bufio.NewWriter(w)
	defer bw.Flush()

	if len(current) == 0 {
		// No orphans this run — silent success (matches python behaviour).
		return
	}

	if block {
		fmt.Fprintln(bw, "")
		fmt.Fprintln(bw, "### Orphan ADR BLOCK")
		fmt.Fprintln(bw, "")
		for _, id := range current {
			fmt.Fprintf(bw, "[BLOCK] %s: consecutive compile with same orphan set — compilation blocked\n", id)
		}
		fmt.Fprintln(bw, "")
		fmt.Fprintln(bw, "Fix options for each blocked ADR/INV:")
		fmt.Fprintln(bw, "  1. Add directives to the sentinel block and run /edikt:gov:compile")
		fmt.Fprintln(bw, "  2. Add `no-directives: \"<reason ≥ 10 chars>\"` to the frontmatter")
		fmt.Fprintln(bw, "  3. Revert the ADR to draft status if the decision is not yet ready")
		fmt.Fprintln(bw, "")
		fmt.Fprintln(bw, "The orphan set has not changed since the last compile. Compilation is blocked.")
		return
	}

	fmt.Fprintln(bw, "")
	fmt.Fprintln(bw, "### Orphan ADR warnings")
	fmt.Fprintln(bw, "")
	for _, id := range current {
		fmt.Fprintf(bw, "[WARN] %s: accepted ADR/INV has zero directives and no no-directives reason\n", id)
	}
	fmt.Fprintln(bw, "")
	fmt.Fprintln(bw, "Fix options for each orphan ADR/INV:")
	fmt.Fprintln(bw, "  1. Add directives to the sentinel block and run /edikt:gov:compile")
	fmt.Fprintln(bw, "  2. Add `no-directives: \"<reason ≥ 10 chars>\"` to the frontmatter")
	fmt.Fprintln(bw, "  3. Revert the ADR to draft status if the decision is not yet ready")
	fmt.Fprintln(bw, "")
	fmt.Fprintf(bw, "Scenario: %s. This compile exits 0 — the SAME orphan set on the next compile will block.\n",
		scenarioNote(sc))
}

func scenarioNote(sc orphan.Scenario) string {
	switch sc {
	case orphan.FirstDetection:
		return "first detection"
	case orphan.SubsetResolved:
		return "changed (subset) → reset to first-detection"
	case orphan.SupersetAdded:
		return "changed (superset) → first-detection"
	case orphan.DifferentReset:
		return "changed (different set) → first-detection"
	case orphan.NoOrphans:
		return "no orphans"
	case orphan.Consecutive:
		return "consecutive"
	default:
		return string(sc)
	}
}

// MarshalScenario lets external tooling render a scenario to a stable
// JSON tag without import-cycling on the orphan package.
func MarshalScenario(sc orphan.Scenario) ([]byte, error) { return json.Marshal(string(sc)) }

// errBadFlag is sentinel error for INV-006 refusals.
var errBadFlag = errors.New("invalid flag value")
