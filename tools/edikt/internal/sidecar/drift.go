package sidecar

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IsStale reports whether the sidecar's recorded directive quotes still
// match the live parent .md prose, per ADR-028's drift contract:
// the sidecar is stale if any directive's source_excerpt.quote no longer
// appears at line_start..line_end in the parent body.
//
// projectRoot is used to resolve sidecar.path when it is relative.
// reason is empty when not stale; populated with the first violation found
// when stale (the dispatcher uses it for log lines, not control flow).
func (s *Sidecar) IsStale(projectRoot string) (stale bool, reason string, err error) {
	parentPath := s.Path
	if !filepath.IsAbs(parentPath) {
		parentPath = filepath.Join(projectRoot, s.Path)
	}
	data, err := os.ReadFile(parentPath)
	if err != nil {
		return false, "", fmt.Errorf("read parent %s: %w", parentPath, err)
	}
	lines := strings.Split(string(data), "\n")

	for i, d := range s.Directives {
		// Phase 5 → Phase 8 carry-over: when migrate's findDirectiveSource
		// failed to anchor the legacy sentinel directive text against
		// differently-phrased prose in the parent .md, applyArtifact wrote
		// a default-fallback excerpt with line_start=line_end=1 and
		// quote=directive_text. Treat that pattern as "no source anchor
		// available" rather than "stale" — there's no anchor to drift
		// against, so drift detection is undefined. Phase 9's golden corpus
		// expansion provides full anchor coverage; this is a transitional
		// weakening for the v0.6.0 mechanical-migration carry-over.
		if isDefaultFallbackExcerpt(d) {
			continue
		}
		if d.SourceExcerpt.LineStart > len(lines) || d.SourceExcerpt.LineEnd > len(lines) {
			return true, fmt.Sprintf("directive[%d]: lines %d-%d outside body length %d",
				i, d.SourceExcerpt.LineStart, d.SourceExcerpt.LineEnd, len(lines)), nil
		}
		sliceStart := d.SourceExcerpt.LineStart - 1
		sliceEnd := d.SourceExcerpt.LineEnd
		passage := strings.Join(lines[sliceStart:sliceEnd], "\n")
		if !strings.Contains(passage, strings.TrimSpace(d.SourceExcerpt.Quote)) {
			return true, fmt.Sprintf("directive[%d]: quote not found at lines %d-%d",
				i, d.SourceExcerpt.LineStart, d.SourceExcerpt.LineEnd), nil
		}
	}
	return false, "", nil
}

// isDefaultFallbackExcerpt reports whether d's source_excerpt looks like the
// "no anchor available" default produced by migrate's findDirectiveSource
// when sentinel text didn't anchor to differently-phrased prose. Two shapes:
//
//  1. Full-fallback: line_start == line_end == 1 AND quote == directive text
//     (when directive text ≤ 200 chars; migrate writes it verbatim).
//  2. Truncated-fallback: line_start == line_end == 1 AND len(quote) == 200
//     AND directive text starts with quote (when directive text > 200 chars
//     migrate truncates the quote to fit the schema's source_excerpt bounds).
//
// Both patterns have no real source anchor, so drift detection is undefined
// and we skip it. See applyArtifact in tools/edikt/cmd/migrate_sidecars.go.
func isDefaultFallbackExcerpt(d Directive) bool {
	if d.SourceExcerpt.LineStart != 1 || d.SourceExcerpt.LineEnd != 1 {
		return false
	}
	q := strings.TrimSpace(d.SourceExcerpt.Quote)
	t := strings.TrimSpace(d.Text)
	if q == t {
		return true
	}
	if len(d.SourceExcerpt.Quote) == 200 && strings.HasPrefix(d.Text, d.SourceExcerpt.Quote) {
		return true
	}
	return false
}
