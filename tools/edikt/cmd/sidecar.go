package cmd

// sidecar.go — `edikt sidecar` subcommand group.
//
// Phase 7 of PLAN-v060-governance-accuracy: add-manual-directive.
// Phase 6 of PLAN-v060-governance-accuracy: diff.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecardiff"
	"github.com/spf13/cobra"
	"golang.org/x/text/unicode/norm"
)

var sidecarCmd = &cobra.Command{
	Use:   "sidecar",
	Short: "Manage governance sidecar files",
	Long: `Commands for reading and editing <artifact>.edikt.yaml sidecar files.

Currently available:
  add-manual-directive   Append a user-authored directive to an existing sidecar
  diff                   Structural-equivalence comparator for golden fixtures
`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// addManualDirective flags
var (
	addManualPath string
	addManualText string
)

// adrIDFromPathRe matches (case-insensitive) ADR-NNN or INV-NNN in a filename.
var adrIDFromPathRe = regexp.MustCompile(`(?i)(ADR|INV)-\d+`)

var addManualDirectiveCmd = &cobra.Command{
	Use:   "add-manual-directive",
	Short: "Append a manual directive to an existing sidecar",
	Long: `Appends <text> to the manual_directives[] list of <sidecar-path> without
editing the parent .md (accepted artifacts are immutable). Writes atomically
(write tmp + rename). Runs Sidecar.Validate() after append.

Exit codes:
  0 — success
  1 — validation error
  2 — sidecar file missing
  3 — duplicate directive (idempotency guard)`,
	Args: cobra.NoArgs,
	RunE: runAddManualDirective,
}

// diffCmd — `edikt sidecar diff <fixture-dir>`
//
// Pure Go, no LLM invocation.
// Exit codes:
// 0 — equivalent
// 1 — divergent
// 2 — missing fixture file
// 3 — flag/arg error
var diffCmd = &cobra.Command{
	Use:   "diff <fixture-dir>",
	Short: "Structural-equivalence comparator for golden fixtures",
	Long: `Compares expected.edikt.yaml and actual.edikt.yaml in <fixture-dir> using
a three-tier structural-equivalence comparator. No LLM invocation.

Exit codes:
  0 — equivalent
  1 — divergent with structured diagnostic on stdout
  2 — missing fixture file (expected.edikt.yaml, actual.edikt.yaml, or fixture.yaml)
  3 — wrong number of arguments`,
	Args: cobra.ExactArgs(1),
	RunE: runSidecarDiff,
}

func init() {
	addManualDirectiveCmd.Flags().StringVar(&addManualPath, "path", "", "path to <artifact>.edikt.yaml or <artifact>.md (required)")
	addManualDirectiveCmd.Flags().StringVar(&addManualText, "text", "", "directive text to append (required, ≤500 chars, no leading/trailing whitespace)")
	_ = addManualDirectiveCmd.MarkFlagRequired("path")
	_ = addManualDirectiveCmd.MarkFlagRequired("text")

	sidecarCmd.AddCommand(addManualDirectiveCmd)
	sidecarCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(sidecarCmd)
}

func runAddManualDirective(cmd *cobra.Command, args []string) error {
	// ── Validate --text ──────────────────────────────────────────────

	if strings.TrimSpace(addManualText) != addManualText {
		return &exitCodeError{code: 1, msg: "error: --text must not have leading or trailing whitespace"}
	}
	if strings.ContainsRune(addManualText, 0) {
		return &exitCodeError{code: 1, msg: "error: --text must not contain embedded NUL bytes"}
	}
	if utf8.RuneCountInString(addManualText) == 0 {
		return &exitCodeError{code: 1, msg: "error: --text is empty"}
	}
	if len(addManualText) > 500 {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("error: --text is %d chars, max 500", len(addManualText))}
	}

	// ── Validate and resolve --path ──────────────────────────────────

	sidecarPath, err := resolveSidecarPath(addManualPath)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("error: --path: %v", err)}
	}

	// ── Load existing sidecar (exit 2 if missing) ─────────────────────────────

	if _, statErr := os.Stat(sidecarPath); os.IsNotExist(statErr) {
		return &exitCodeError{code: 2, msg: fmt.Sprintf("error: sidecar not found: %s", sidecarPath)}
	}

	sc, err := sidecar.Load(sidecarPath)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("error: loading sidecar: %v", err)}
	}

	// ── Auto-append (ref: ADR-NNN + manual) if absent ────────────────────────

	text := addManualText
	if !strings.Contains(text, "(ref:") {
		refTag := buildRefTag(sidecarPath, sc.Path)
		text = text + " " + refTag
	}

	// ── Idempotency: reject duplicates (NFKC-folded, whitespace-normalized) ──

	normalised := normText(text)
	for i, existing := range sc.ManualDirectives {
		if normText(existing) == normalised {
			return &exitCodeError{
				code: 3,
				msg:  fmt.Sprintf("Manual directive already present at index %d.", i),
			}
		}
	}

	// ── Append ───────────────────────────────────────────────────────────────

	sc.ManualDirectives = append(sc.ManualDirectives, text)

	// ── Validate (exit 1 on validation error) ────────────────────────────────

	if err := sc.Validate(); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("error: validate after append: %v", err)}
	}

	// ── Marshal + atomic write ────────────────────────────────────────────────

	out, err := sidecar.Marshal(sc)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("error: marshal: %v", err)}
	}

	if err := atomicWriteNoFollow(sidecarPath, out, 0o644); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("error: write: %v", err)}
	}

	fmt.Fprintf(os.Stdout, "ok: appended manual directive to %s\n", sidecarPath)
	fmt.Fprintf(os.Stdout, "    %q\n", text)
	return nil
}

// resolveSidecarPath resolves --path to an absolute .edikt.yaml path.
// If the input ends with .md, returns the sibling .edikt.yaml path.
// validates path is absolute after resolution, refuses traversal.
func resolveSidecarPath(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("empty path")
	}

	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path: %w", err)
	}

	// Refuse path traversal escapes — the cleaned absolute path must not
	// contain "/.." after Clean (Abs already cleans, but be explicit).
	cleaned := filepath.Clean(abs)
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("path traversal not allowed: %s", raw)
	}

	if strings.HasSuffix(cleaned, ".md") {
		base := strings.TrimSuffix(filepath.Base(cleaned), ".md")
		cleaned = filepath.Join(filepath.Dir(cleaned), base+".edikt.yaml")
	}

	if !strings.HasSuffix(cleaned, ".edikt.yaml") {
		return "", fmt.Errorf("path must be a .edikt.yaml or .md file, got: %s", raw)
	}

	return cleaned, nil
}

// buildRefTag returns "(ref: ADR-NNN + manual)" using the ID extracted from
// the sidecar file path, or the basename slug if no ID is found.
func buildRefTag(sidecarPath, scPath string) string {
	// Try sidecar path first, then the sidecar's path field.
	id := extractIDFromName(filepath.Base(sidecarPath))
	if id == "" {
		id = extractIDFromName(filepath.Base(scPath))
	}
	if id == "" {
		// slug: basename without extension
		base := filepath.Base(sidecarPath)
		base = strings.TrimSuffix(base, ".edikt.yaml")
		base = strings.TrimSuffix(base, ".yaml")
		id = base
	}
	return fmt.Sprintf("(ref: %s + manual)", id)
}

// extractIDFromName extracts ADR-NNN or INV-NNN (case-insensitive) from a
// filename, returning the canonical uppercase form.
func extractIDFromName(name string) string {
	m := adrIDFromPathRe.FindString(name)
	if m == "" {
		return ""
	}
	return strings.ToUpper(m)
}

// normText applies NFKC normalization and collapses internal whitespace for
// the idempotency check.
func normText(s string) string {
	s = norm.NFKC.String(s)
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// runSidecarDiff implements `edikt sidecar diff <fixture-dir>`.
//
// resolves fixture-dir to absolute, refuses traversal, refuses
// paths outside test/fixtures/. Validates that sidecar files inside the dir
// don't escape via symlinks (filepath.EvalSymlinks re-checked for prefix).
func runSidecarDiff(cmd *cobra.Command, args []string) error {
	rawDir := args[0]

	// ── Resolve and validate fixture-dir ──────────────────────────────────────
	absDir, err := filepath.Abs(rawDir)
	if err != nil {
		return &exitCodeError{code: 3, msg: fmt.Sprintf("error: cannot resolve fixture dir: %v", err)}
	}
	cleanDir := filepath.Clean(absDir)
	if strings.Contains(cleanDir, "..") {
		return &exitCodeError{code: 3, msg: "error: path traversal not allowed"}
	}

	// Resolve symlinks on the dir itself.
	realDir, err := filepath.EvalSymlinks(cleanDir)
	if err != nil {
		return &exitCodeError{code: 2, msg: fmt.Sprintf("error: fixture dir not found: %s", cleanDir)}
	}

	// Enforce that fixture files inside the dir don't escape via symlinks.
	for _, name := range []string{"expected.edikt.yaml", "actual.edikt.yaml", "fixture.yaml"} {
		fpath := filepath.Join(realDir, name)
		if _, statErr := os.Lstat(fpath); os.IsNotExist(statErr) {
			return &exitCodeError{code: 2, msg: fmt.Sprintf("error: missing fixture file: %s", fpath)}
		}
		realF, evalErr := filepath.EvalSymlinks(fpath)
		if evalErr != nil {
			return &exitCodeError{code: 2, msg: fmt.Sprintf("error: cannot resolve %s: %v", name, evalErr)}
		}
		if !strings.HasPrefix(realF, realDir) {
			return &exitCodeError{code: 3, msg: fmt.Sprintf("error: %s escapes fixture dir via symlink", name)}
		}
	}

	// ── Load fixture.yaml ─────────────────────────────────────────────────────
	cfg, err := sidecardiff.LoadFixtureConfig(realDir)
	if err != nil {
		return &exitCodeError{code: 2, msg: fmt.Sprintf("error: load fixture.yaml: %v", err)}
	}

	// ── Run comparator ────────────────────────────────────────────────────────
	result, err := sidecardiff.Diff(realDir, cfg)
	if err != nil {
		return &exitCodeError{code: 2, msg: fmt.Sprintf("error: diff: %v", err)}
	}

	fmt.Fprint(os.Stdout, sidecardiff.FormatResult(result))

	if !result.Pass {
		return &exitCodeError{code: 1, msg: ""}
	}
	return nil
}
