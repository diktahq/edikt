package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/spf13/cobra"
)

// nextIDCmd is the user-invisible helper that powers the !`...` pre-execution
// block at the top of the /edikt:sdlc:{prd,spec,plan}, /edikt:adr:new, and
// /edikt:invariant:new slash commands.
//
// It exists because Claude Code's static permission analyzer cannot evaluate
// the multi-stage shell pipelines those slash commands previously inlined
// (case statements, command substitution, find|xargs|sed chains). A single
// binary invocation IS analyzable, so all five slash commands get a single
// permission rule.
//
// Output is wrapped in <!-- edikt:live --> markers so the slash-command body
// can locate it deterministically when constructing Claude's initial prompt.
// The wrapper text is the exact byte sequence the shell pipelines used to
// emit — preserved verbatim so downstream skill prompt logic doesn't change.
var nextIDCmd = &cobra.Command{
	Use:   "next-id <kind>",
	Short: "Emit next available artifact ID + live context (for slash-command pre-exec)",
	Long: `Used internally by the SDLC slash commands to inject the next available
artifact ID into Claude's initial prompt.

Kinds:
  spec      — next SPEC-NNN + list of existing SPEC dirs
  prd       — next PRD-NNN + list of existing PRD files
  adr       — next ADR-NNN + list of existing ADR files
  inv       — next INV-NNN + list of existing INV files
  discovery — next DISCOVERY-NNN + list of existing DISCOVERY files
  plan      — most recently-edited plan + its in-progress phase row

If .edikt/config.yaml is absent (no project context), emits the minimal
"001 / none yet" form for the listing kinds and exits 0 silently for plan.

This subcommand replaces inline ` + "`!`" + `bash -c '...'` + "`" + ` blocks in the
slash command pre-execution, so they pass Claude Code's static permission
analyzer (single-binary invocations are always analyzable; multi-stage
shell pipelines are not).`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runNextID(os.Stdout, args[0])
	},
}

func init() {
	rootCmd.AddCommand(nextIDCmd)
}

// findProjectRoot walks up from cwd looking for .edikt/config.yaml. Returns
// the directory holding .edikt/ (i.e. the project root). Returns empty
// string + nil if not found — the no-config case is normal and produces
// the "001 / none yet" fallback output.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, ".edikt", "config.yaml")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil // hit filesystem root
		}
		dir = parent
	}
}

func runNextID(out io.Writer, kind string) error {
	root, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}

	// No-config fallback paths — match the shell pipelines' behavior exactly.
	if root == "" {
		return emitNoConfigFallback(out, kind)
	}

	cfg, err := govrun.LoadConfig(root)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	switch kind {
	case "spec":
		return emitListing(out, root, cfg.Paths.Specs, listingSpec)
	case "prd":
		return emitListing(out, root, cfg.Paths.Prds, listingPRD)
	case "adr":
		return emitListing(out, root, cfg.Paths.Decisions, listingADR)
	case "inv":
		return emitListing(out, root, cfg.Paths.Invariants, listingINV)
	case "discovery":
		return emitListing(out, root, cfg.Paths.Discovery, listingDiscovery)
	case "plan":
		return emitActivePlan(out, root, cfg.Paths.Plans)
	default:
		return fmt.Errorf("unknown kind %q (want one of: spec, prd, adr, inv, discovery, plan)", kind)
	}
}

// emitNoConfigFallback matches the bash pipelines' behavior when
// .edikt/config.yaml is absent: a minimal "001 / none yet" listing for
// spec/prd/adr/inv, and silent exit for plan (no active plan to report).
func emitNoConfigFallback(out io.Writer, kind string) error {
	switch kind {
	case "spec":
		fmt.Fprint(out, wrapLive("Next SPEC number: SPEC-001\nExisting specs: (none yet)\n"))
	case "prd":
		fmt.Fprint(out, wrapLive("Next PRD number: PRD-001\nExisting PRDs: (none yet)\n"))
	case "adr":
		fmt.Fprint(out, wrapLive("Next ADR number: ADR-001\nExisting ADRs: (none yet)\n"))
	case "inv":
		fmt.Fprint(out, wrapLive("Next INV number: INV-001\nExisting invariants: (none yet)\n"))
	case "discovery":
		fmt.Fprint(out, wrapLive("Next DISCOVERY number: DISCOVERY-001\nExisting discoveries: (none yet)\n"))
	case "plan":
		// Plan emits nothing when no config — matches the shell.
	default:
		return fmt.Errorf("unknown kind %q (want one of: spec, prd, adr, inv, discovery, plan)", kind)
	}
	return nil
}

// listingKind packages the per-kind details for the listing flavor of
// the command (spec/prd/adr/inv share the same shape).
type listingKind struct {
	idPrefix string // "SPEC", "PRD", "ADR", "INV"
	dirGlob  bool   // true for spec (SPEC-NNN is a directory containing spec.md)
	header   string // label for "Next X number:"
	footer   string // label for "Existing Xs:"
}

var (
	listingSpec      = listingKind{idPrefix: "SPEC", dirGlob: true, header: "Next SPEC number", footer: "Existing specs"}
	listingPRD       = listingKind{idPrefix: "PRD", dirGlob: false, header: "Next PRD number", footer: "Existing PRDs"}
	listingADR       = listingKind{idPrefix: "ADR", dirGlob: false, header: "Next ADR number", footer: "Existing ADRs"}
	listingINV       = listingKind{idPrefix: "INV", dirGlob: false, header: "Next INV number", footer: "Existing invariants"}
	listingDiscovery = listingKind{idPrefix: "DISCOVERY", dirGlob: false, header: "Next DISCOVERY number", footer: "Existing discoveries"}
)

func emitListing(out io.Writer, root, relDir string, k listingKind) error {
	absDir := relDir
	if !filepath.IsAbs(absDir) {
		absDir = filepath.Join(root, relDir)
	}

	names, err := listArtifacts(absDir, k)
	if err != nil {
		return err
	}

	next := fmt.Sprintf("%s-%03d", k.idPrefix, len(names)+1)
	existing := "(none yet)"
	if len(names) > 0 {
		existing = strings.Join(names, ",")
	}
	body := fmt.Sprintf("%s: %s\n%s: %s\n", k.header, next, k.footer, existing)
	fmt.Fprint(out, wrapLive(body))
	return nil
}

// listArtifacts returns the sorted list of artifact names matching the
// kind's pattern. For dirGlob=true (spec), matches subdirectories named
// SPEC-* that contain a spec.md. For dirGlob=false, matches *.md files
// named <prefix>-* and strips the .md extension.
func listArtifacts(absDir string, k listingKind) ([]string, error) {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	prefix := k.idPrefix + "-"
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if k.dirGlob {
			if !e.IsDir() {
				continue
			}
			// Require spec.md inside (matches the shell's SPEC-*/spec.md glob).
			if _, err := os.Stat(filepath.Join(absDir, name, "spec.md")); err != nil {
				continue
			}
			names = append(names, name)
		} else {
			if e.IsDir() {
				continue
			}
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			names = append(names, strings.TrimSuffix(name, ".md"))
		}
	}
	sort.Strings(names)
	return names, nil
}

// emitActivePlan implements the `plan` kind: find the most recently-modified
// .md under plans dir (fallback to docs/product/plans), extract the first
// in-progress phase row from its progress table, emit the live block.
// Emits nothing when no plan found — matches the shell pipeline's exit-0
// silent behavior.
func emitActivePlan(out io.Writer, root, relPlansDir string) error {
	absDir := relPlansDir
	if !filepath.IsAbs(absDir) {
		absDir = filepath.Join(root, relPlansDir)
	}
	plan := mostRecentPlan(absDir)
	if plan == "" {
		// Legacy fallback — pre-v0.6 projects sometimes kept plans under docs/product/plans.
		plan = mostRecentPlan(filepath.Join(root, "docs", "product", "plans"))
	}
	if plan == "" {
		return nil // no plan, no output (matches the shell)
	}
	phase := firstInProgressPhase(plan)
	if phase == "" {
		phase = "(none in progress)"
	}
	fmt.Fprint(out, wrapLive(fmt.Sprintf("Active plan: %s\nCurrent phase status: %s\n", filepath.Base(plan), phase)))
	return nil
}

func mostRecentPlan(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var best string
	var bestMtime int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		if mt := info.ModTime().Unix(); mt > bestMtime {
			bestMtime = mt
			best = path
		}
	}
	return best
}

// firstInProgressPhase reads the plan file and returns the trimmed text of
// the first table row whose cells include an "in_progress" / "in progress" /
// "in-progress" marker (case-insensitive). Matches the shell pipeline's
// `grep -iE "\|.*in[_ -]progress"` + `tr -d "|"` + `xargs` behavior.
var inProgressRe = regexp.MustCompile(`(?i)\|.*in[_\- ]progress`)

func firstInProgressPhase(planPath string) string {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if inProgressRe.MatchString(line) {
			cleaned := strings.ReplaceAll(line, "|", "")
			cleaned = strings.TrimSpace(cleaned)
			cleaned = collapseSpaces(cleaned)
			return cleaned
		}
	}
	return ""
}

func collapseSpaces(s string) string {
	var b strings.Builder
	var prevSpace bool
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if !prevSpace && b.Len() > 0 {
				b.WriteRune(' ')
			}
			prevSpace = true
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return strings.TrimRight(b.String(), " ")
}

// wrapLive surrounds body in the <!-- edikt:live --> / <!-- /edikt:live -->
// markers exactly as the shell pipelines did — preserved verbatim so slash
// command prompt logic that locates this block doesn't need to change.
func wrapLive(body string) string {
	return "<!-- edikt:live -->\n" + body + "<!-- /edikt:live -->\n"
}
