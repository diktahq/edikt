package cmd

// sidecar.go — `edikt sidecar` subcommand group.
//
// Phase 7 of PLAN-v060-governance-accuracy.
//
// Registers the `sidecar` parent cobra command at root, plus the
// `add-manual-directive` subcommand. Designed so `diff` (Phase 6)
// can be added later without restructuring.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/spf13/cobra"
	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
)

var sidecarCmd = &cobra.Command{
	Use:   "sidecar",
	Short: "Manage governance sidecar files",
	Long: `Commands for reading and editing <artifact>.edikt.yaml sidecar files.

Currently available:
  add-manual-directive   Append a user-authored directive to an existing sidecar
`,
	Args:    cobra.NoArgs,
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
editing the parent .md (INV-002). Writes atomically (write tmp + rename).
Runs Sidecar.Validate() after append.

Exit codes:
  0 — success
  1 — validation error
  2 — sidecar file missing
  3 — duplicate directive (idempotency guard)`,
	Args: cobra.NoArgs,
	RunE: runAddManualDirective,
}

func init() {
	addManualDirectiveCmd.Flags().StringVar(&addManualPath, "path", "", "path to <artifact>.edikt.yaml or <artifact>.md (required)")
	addManualDirectiveCmd.Flags().StringVar(&addManualText, "text", "", "directive text to append (required, ≤500 chars, no leading/trailing whitespace)")
	_ = addManualDirectiveCmd.MarkFlagRequired("path")
	_ = addManualDirectiveCmd.MarkFlagRequired("text")

	sidecarCmd.AddCommand(addManualDirectiveCmd)
	rootCmd.AddCommand(sidecarCmd)
}

func runAddManualDirective(cmd *cobra.Command, args []string) error {
	// ── INV-006: validate --text ──────────────────────────────────────────────

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

	// ── INV-006: validate and resolve --path ──────────────────────────────────

	sidecarPath, err := resolveSidecarPath(addManualPath)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("error: --path: %v", err)}
	}

	// ── Load existing sidecar (exit 2 if missing) ─────────────────────────────

	if _, statErr := os.Stat(sidecarPath); os.IsNotExist(statErr) {
		return &exitCodeError{code: 2, msg: fmt.Sprintf("error: sidecar not found: %s", sidecarPath)}
	}

	// Decode without cache so Marshal after mutation emits the mutated state,
	// not the pre-mutation cached bytes that sidecar.Load would capture.
	sc, err := loadForMutation(sidecarPath)
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
// INV-006: validates path is absolute after resolution, refuses traversal.
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

// loadForMutation decodes a sidecar from disk and runs Validate, but does NOT
// prime the marshal cache. This ensures that after mutating the struct and
// calling sidecar.Marshal, the updated fields are serialized rather than the
// pre-mutation cache that sidecar.Load would have captured.
func loadForMutation(path string) (*sidecar.Sidecar, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(bufio.NewReader(f))
	dec.KnownFields(true)

	var sc sidecar.Sidecar
	if err := dec.Decode(&sc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	sc.SourcePath = path
	if err := sc.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return &sc, nil
}
