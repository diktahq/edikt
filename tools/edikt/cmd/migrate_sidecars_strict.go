package cmd

// migrate_sidecars_strict.go — --strict and --report-json flags for Phase 3
// of PLAN-v060-governance-accuracy.
//
// Diffs legacy sentinel content against the newly-written sidecar. No LLM
// invocations — pure Go string comparison + regex (ADR-030).
//
// Exit codes when --strict is set:
//   exit 1 — LOST or FACTUAL items present
//   exit 2 — DEGRADED items only (warning level)
//   exit 0 — clean
//   exit 3 — system error (parse failure)

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/parse"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// strictDiffPair is one (sentinel, sidecar) pair collected during apply.
type strictDiffPair struct {
	mdPath      string
	sidecarPath string
	sentinel    parse.Sentinel
}

// StrictCategory classifies a regression item.
type StrictCategory string

const (
	CategoryLOST     StrictCategory = "LOST"
	CategoryDEGRADED StrictCategory = "DEGRADED"
	CategoryFACTUAL  StrictCategory = "FACTUAL"
)

// StrictSeverity rates the impact of a regression item.
type StrictSeverity string

const (
	SeverityHigh   StrictSeverity = "high"
	SeverityMedium StrictSeverity = "medium"
	SeverityLow    StrictSeverity = "low"
)

// StrictSourceExcerpt mirrors the sidecar SourceExcerpt shape in the manifest.
type StrictSourceExcerpt struct {
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	Quote     string `json:"quote"`
}

// StrictItem is one regression finding in the manifest.
type StrictItem struct {
	Path          string              `json:"path"`
	Category      StrictCategory      `json:"category"`
	Severity      StrictSeverity      `json:"severity"`
	Field         string              `json:"field"`
	Expected      string              `json:"expected"`
	Actual        string              `json:"actual"`
	SourceExcerpt StrictSourceExcerpt `json:"source_excerpt"`
}

// StrictSummary is the top-level counts in the manifest.
type StrictSummary struct {
	Lost           int `json:"lost"`
	Degraded       int `json:"degraded"`
	Factual        int `json:"factual"`
	TotalArtifacts int `json:"total_artifacts"`
}

// StrictManifest is the JSON manifest written by --report-json.
type StrictManifest struct {
	Summary StrictSummary `json:"summary"`
	Items   []StrictItem  `json:"items"`
}

// greppableToken matches file paths, test names, grep, and HTTP verbs that
// make a verification item concrete (not abstract).
var greppableToken = regexp.MustCompile(
	`\b[\w/]+\.(go|md|sh|py|yaml|json)\b` +
		`|\bgrep\b` +
		`|\b(GET|POST|PUT|DELETE|PATCH)\b`,
)

// conditionalPrefix matches the fallback / contingency patterns that should
// NOT be promoted to MUST when converting from legacy to new.
var conditionalPrefix = regexp.MustCompile(
	`(?i)(^Fallback:|^Alternatively:|^Optionally:|^If\s+\w|^As a fallback,)`,
)

// modality extracts the first modal word from a directive text.
// Returns "" if no recognized modal is present.
func modality(text string) string {
	upper := strings.ToUpper(text)
	switch {
	case strings.Contains(upper, "MUST NOT"):
		return "MUST NOT"
	case strings.Contains(upper, "MUST"):
		return "MUST"
	case strings.Contains(upper, "SHOULD NOT"):
		return "SHOULD NOT"
	case strings.Contains(upper, "SHOULD"):
		return "SHOULD"
	case strings.Contains(upper, "MAY"):
		return "MAY"
	default:
		return ""
	}
}

// isMandatory reports whether modal is MUST or MUST NOT.
func isMandatory(modal string) bool {
	return modal == "MUST" || modal == "MUST NOT"
}

// normalizeDirective lowercases, collapses whitespace, and strips trailing
// "(ref: …)" tags so two semantically-equal directives compare equal.
func normalizeDirective(s string) string {
	// Strip ref tag: " (ref: XYZ)"
	if i := strings.LastIndex(s, "(ref:"); i > 0 {
		s = strings.TrimRight(strings.TrimSpace(s[:i]), ".")
	}
	s = strings.ToLower(s)
	// Collapse all whitespace runs to a single space.
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// normalizeDirectiveNoModal is like normalizeDirective but also strips
// recognized modal keywords so two directives that differ only in
// MUST/SHOULD/MAY compare equal. Used for FACTUAL detection matching.
var modalRe = regexp.MustCompile(`(?i)\bmust not\b|\bmust\b|\bshould not\b|\bshould\b|\bmay\b|\bnever\b`)

func normalizeDirectiveNoModal(s string) string {
	// Strip ref tag first.
	if i := strings.LastIndex(s, "(ref:"); i > 0 {
		s = strings.TrimRight(strings.TrimSpace(s[:i]), ".")
	}
	s = strings.ToLower(s)
	s = modalRe.ReplaceAllString(s, "")
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// buildStrictManifest diffs each (sentinel, sidecar) pair and returns the
// aggregated manifest. Pure Go — no LLM invocation (ADR-030).
func buildStrictManifest(pairs []strictDiffPair) (*StrictManifest, error) {
	manifest := &StrictManifest{
		Items: []StrictItem{},
	}

	for _, p := range pairs {
		sc, err := sidecar.Load(p.sidecarPath)
		if err != nil {
			return nil, fmt.Errorf("load sidecar %s: %w", p.sidecarPath, err)
		}

		items := diffSentinelSidecar(p.mdPath, &p.sentinel, sc)
		manifest.Items = append(manifest.Items, items...)
		manifest.Summary.TotalArtifacts++
	}

	// Sort deterministically: path → category → field.
	sort.Slice(manifest.Items, func(i, j int) bool {
		a, b := manifest.Items[i], manifest.Items[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		return a.Field < b.Field
	})

	for _, item := range manifest.Items {
		switch item.Category {
		case CategoryLOST:
			manifest.Summary.Lost++
		case CategoryDEGRADED:
			manifest.Summary.Degraded++
		case CategoryFACTUAL:
			manifest.Summary.Factual++
		}
	}

	return manifest, nil
}

// diffSentinelSidecar computes the diff for one artifact. Returns the items
// found; empty slice means no regressions.
func diffSentinelSidecar(mdPath string, sent *parse.Sentinel, sc *sidecar.Sidecar) []StrictItem {
	var items []StrictItem

	// ── LOST: directives present in sentinel but absent from sidecar ──────────
	newDirSet := make(map[string]bool, len(sc.Directives))
	for _, d := range sc.Directives {
		newDirSet[normalizeDirective(d.Text)] = true
	}
	// Also include manual_directives in the "present" set — they're carried over
	// intentionally and are not a regression.
	for _, d := range sc.ManualDirectives {
		newDirSet[normalizeDirective(d)] = true
	}

	for _, legacyText := range sent.Directives {
		norm := normalizeDirective(legacyText)
		if !newDirSet[norm] {
			items = append(items, StrictItem{
				Path:          mdPath,
				Category:      CategoryLOST,
				Severity:      SeverityHigh,
				Field:         "directives",
				Expected:      legacyText,
				Actual:        "",
				SourceExcerpt: excerptFromText(legacyText),
			})
		}
	}

	// ── LOST: paths present in sentinel but absent from sidecar ──────────────
	newPathSet := make(map[string]bool, len(sc.Paths))
	for _, p := range sc.Paths {
		newPathSet[p] = true
	}
	for _, legacyPath := range sent.Paths {
		if !newPathSet[legacyPath] {
			items = append(items, StrictItem{
				Path:          mdPath,
				Category:      CategoryLOST,
				Severity:      SeverityMedium,
				Field:         "paths",
				Expected:      legacyPath,
				Actual:        "",
				SourceExcerpt: excerptFromText(legacyPath),
			})
		}
	}

	// ── LOST: scope phases present in sentinel but absent from sidecar ────────
	newScopeSet := make(map[string]bool, len(sc.Scope))
	for _, s := range sc.Scope {
		newScopeSet[s] = true
	}
	for _, legacyScope := range sent.Scope {
		if !newScopeSet[legacyScope] {
			items = append(items, StrictItem{
				Path:          mdPath,
				Category:      CategoryLOST,
				Severity:      SeverityMedium,
				Field:         "scope",
				Expected:      legacyScope,
				Actual:        "",
				SourceExcerpt: excerptFromText(legacyScope),
			})
		}
	}

	// ── LOST: prohibitions — legacy NEVER/MUST NOT directives not in new sidecar
	newProhibSet := make(map[string]bool, len(sc.Prohibitions))
	for _, p := range sc.Prohibitions {
		newProhibSet[normalizeDirective(p.Text)] = true
	}
	// Also check new directives for NEVER/MUST NOT.
	for _, d := range sc.Directives {
		upper := strings.ToUpper(d.Text)
		if strings.Contains(upper, "NEVER") || strings.Contains(upper, "MUST NOT") {
			newProhibSet[normalizeDirective(d.Text)] = true
		}
	}

	for _, legacyText := range sent.Directives {
		upper := strings.ToUpper(legacyText)
		if !strings.Contains(upper, "NEVER") && !strings.Contains(upper, "MUST NOT") {
			continue
		}
		norm := normalizeDirective(legacyText)
		if !newProhibSet[norm] {
			items = append(items, StrictItem{
				Path:          mdPath,
				Category:      CategoryLOST,
				Severity:      SeverityHigh,
				Field:         "prohibitions",
				Expected:      legacyText,
				Actual:        "",
				SourceExcerpt: excerptFromText(legacyText),
			})
		}
	}

	// ── FACTUAL: modality drift — contingency language promoted to MUST ───────
	// Build a modal-stripped normalized-text → new sidecar directive map.
	// Modal stripping is required because FACTUAL detection compares directives
	// that have the same semantic content but different modal keywords
	// (e.g. legacy "SHOULD" vs new "MUST"). A standard normalize would never
	// find the match since the strings differ by the modal word.
	newDirByNoModal := make(map[string]string, len(sc.Directives))
	for _, d := range sc.Directives {
		newDirByNoModal[normalizeDirectiveNoModal(d.Text)] = d.Text
	}

	for _, legacyText := range sent.Directives {
		// Only flag directives that contain a conditional/fallback prefix.
		if !conditionalPrefix.MatchString(legacyText) {
			continue
		}
		legacyModal := modality(legacyText)
		noModalNorm := normalizeDirectiveNoModal(legacyText)
		newText, found := newDirByNoModal[noModalNorm]
		if !found {
			// Already flagged as LOST above if truly missing; skip double-reporting.
			continue
		}
		newModal := modality(newText)
		// Flag if new is MUST/MUST NOT but legacy was not mandatory.
		if isMandatory(newModal) && !isMandatory(legacyModal) {
			items = append(items, StrictItem{
				Path:          mdPath,
				Category:      CategoryFACTUAL,
				Severity:      SeverityHigh,
				Field:         "directives",
				Expected:      legacyModal,
				Actual:        newModal,
				SourceExcerpt: excerptFromText(legacyText),
			})
		}
	}

	// ── DEGRADED: verification items became abstract ───────────────────────────
	// Match legacy and new verification items positionally (best-effort) or by
	// checking whether any new item covers the same greppable token.
	for i, legacyVerif := range sent.Verification {
		if !greppableToken.MatchString(legacyVerif) {
			// Legacy was already abstract — not a regression.
			continue
		}
		// Find corresponding new item: try same index, then substring match.
		newVerif := ""
		if i < len(sc.Verification) {
			newVerif = sc.Verification[i]
		} else if len(sc.Verification) > 0 {
			// Fall back to the last new item.
			newVerif = sc.Verification[len(sc.Verification)-1]
		}
		if newVerif != "" && greppableToken.MatchString(newVerif) {
			// New item is still concrete.
			continue
		}
		// Check whether any new verification item covers this greppable token.
		token := greppableToken.FindString(legacyVerif)
		covered := false
		for _, nv := range sc.Verification {
			if strings.Contains(nv, token) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		items = append(items, StrictItem{
			Path:          mdPath,
			Category:      CategoryDEGRADED,
			Severity:      SeverityLow,
			Field:         "verification",
			Expected:      legacyVerif,
			Actual:        newVerif,
			SourceExcerpt: excerptFromText(legacyVerif),
		})
	}

	return items
}

// excerptFromText synthesises a SourceExcerpt from a text string for manifest
// items where we don't have exact line numbers (the sentinel content is
// already extracted as a flat string by the parse package).
func excerptFromText(text string) StrictSourceExcerpt {
	truncated := text
	if len(truncated) > 200 {
		truncated = truncated[:200]
	}
	return StrictSourceExcerpt{LineStart: 1, LineEnd: 1, Quote: truncated}
}

// writeStrictManifest marshals and writes the manifest to path.
// Uses json.MarshalIndent for deterministic, human-readable output.
func writeStrictManifest(path string, m *StrictManifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// strictExit enforces the exit-code contract for --strict mode.
// Callers invoke os.Exit directly so the cobra error-return path
// does not print a redundant usage message.
func strictExit(m *StrictManifest) {
	if m.Summary.Lost > 0 || m.Summary.Factual > 0 {
		fmt.Fprintf(os.Stderr, "strict: %d LOST, %d FACTUAL, %d DEGRADED — exit 1\n",
			m.Summary.Lost, m.Summary.Factual, m.Summary.Degraded)
		os.Exit(1)
	}
	if m.Summary.Degraded > 0 {
		fmt.Fprintf(os.Stderr, "strict: %d DEGRADED — exit 2\n", m.Summary.Degraded)
		os.Exit(2)
	}
}
