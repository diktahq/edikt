package sidecar

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AutoRepairAnchors attempts to repair stale line/column anchors in each
// directive and prohibition by re-scanning the source document text.
//
// Strategy (quote-primary, line-secondary):
//  1. If SourceExcerpt.Quote is non-empty, scan sourceLines for an exact
//     substring match. When a unique match is found, update LineStart (and
//     LineEnd when it was previously a single-line anchor) to that line.
//  2. When multiple lines match the quote, pick the one closest to the
//     existing LineStart (line-secondary tiebreaker). This disambiguates
//     repeated phrases that share a common prefix.
//  3. If no line matches, fall back to the existing LineStart when it is
//     in range — leave the anchor untouched.
//
// This is a pure-Go deterministic function — no LLM dispatch.
//
// Motivating case: ADR-038 directive[40..42] — three directives sharing a  // edikt-guard:allow
// common SourceExcerpt prefix caused the LLM to re-anchor all three to the
// first occurrence. AutoRepairAnchors resolves this by disambiguating with
// the existing LineStart as a tiebreaker when multiple quote matches exist,
// so the deterministic repair preserves the original ordering instead of
// collapsing onto the first match.
//
// Returns the number of anchors that were updated.
func AutoRepairAnchors(sc *Sidecar, sourceLines []string) int {
	if sc == nil {
		return 0
	}
	repaired := 0
	for i := range sc.Directives {
		d := &sc.Directives[i]
		for ai := range sc.Directives[i].SourceExcerpts {
			if repairExcerpt(&sc.Directives[i].SourceExcerpts[ai], sourceLines) {
				repaired++
			}
		}
		if repairExcerpt(&d.SourceExcerpt, sourceLines) {
			repaired++
		}
	}
	for i := range sc.Prohibitions {
		p := &sc.Prohibitions[i]
		for ai := range sc.Prohibitions[i].SourceExcerpts {
			if repairExcerpt(&sc.Prohibitions[i].SourceExcerpts[ai], sourceLines) {
				repaired++
			}
		}
		if repairExcerpt(&p.SourceExcerpt, sourceLines) {
			repaired++
		}
	}
	return repaired
}

// anchorMatch is one candidate location for a quote in the source.
type anchorMatch struct {
	start int // 1-indexed first line
	end   int // 1-indexed last line
}

// repairExcerpt applies the quote-primary, line-secondary strategy to a
// single SourceExcerpt. Returns true when LineStart/LineEnd were updated.
//
// Matching runs in two passes:
//  1. exact single-line containment (the original strategy);
//  2. whitespace-normalized scan across multi-line windows — extractor
//     quotes are frequently not byte-exact (wrapped lines joined into one
//     string, bullets/indentation stripped), and without this pass the
//     whole class fell through to LLM re-dispatch on every compile.
func repairExcerpt(se *SourceExcerpt, sourceLines []string) bool {
	quote := strings.TrimSpace(se.Quote)
	if quote == "" {
		// No data to repair from — quote-absent fallthrough.
		return false
	}
	// Pass 1: every line (1-indexed) that contains the quote verbatim.
	var matches []anchorMatch
	for idx, line := range sourceLines {
		if strings.Contains(line, quote) {
			matches = append(matches, anchorMatch{start: idx + 1, end: idx + 1})
		}
	}
	// Pass 2: whitespace-normalized multi-line scan.
	if len(matches) == 0 {
		matches = findNormalizedMatches(quote, sourceLines)
	}
	if len(matches) == 0 {
		// Quote absent from document — no-op, preserve existing anchor.
		return false
	}
	// Line-secondary tiebreaker: pick the match closest to the existing
	// LineStart. On ties (equidistant matches above/below current
	// LineStart), prefer the earlier line for determinism.
	pick := matches[0]
	bestDelta := absInt(matches[0].start - se.LineStart)
	for _, m := range matches[1:] {
		delta := absInt(m.start - se.LineStart)
		if delta < bestDelta {
			bestDelta = delta
			pick = m
		}
	}
	// Single-line exact matches preserve the recorded span shape (a
	// multi-line span that drifted as a block shifts wholesale);
	// normalized matches carry their own measured end line.
	newStart, newEnd := pick.start, pick.end
	if pick.start == pick.end && se.LineEnd > se.LineStart {
		newEnd = pick.start + (se.LineEnd - se.LineStart)
	}
	if newStart == se.LineStart && newEnd == se.LineEnd {
		return false
	}
	se.LineStart = newStart
	se.LineEnd = newEnd
	return true
}

// normalizeWS collapses every run of whitespace into a single space and
// trims the result.
func normalizeWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// findNormalizedMatches locates every occurrence of the whitespace-
// normalized quote inside the whitespace-normalized document and maps each
// occurrence back to its 1-indexed source line span. Deterministic, pure.
func findNormalizedMatches(quote string, sourceLines []string) []anchorMatch {
	nq := normalizeWS(quote)
	if nq == "" {
		return nil
	}
	// Build the normalized document with per-line offset spans.
	var b strings.Builder
	starts := make([]int, len(sourceLines))
	ends := make([]int, len(sourceLines))
	for i, line := range sourceLines {
		if i > 0 && b.Len() > 0 {
			b.WriteByte(' ')
		}
		starts[i] = b.Len()
		b.WriteString(normalizeWS(line))
		ends[i] = b.Len()
	}
	doc := b.String()

	lineAt := func(off int) int {
		for i := range sourceLines {
			if off >= starts[i] && off < ends[i] {
				return i + 1
			}
		}
		return 0
	}

	var matches []anchorMatch
	for from := 0; ; {
		idx := strings.Index(doc[from:], nq)
		if idx < 0 {
			break
		}
		off := from + idx
		s := lineAt(off)
		e := lineAt(off + len(nq) - 1)
		if s > 0 && e >= s {
			matches = append(matches, anchorMatch{start: s, end: e})
		}
		from = off + 1
	}
	return matches
}

// IsStale (free-function form) reports whether any directive or prohibition
// in sc has a SourceExcerpt whose Quote is non-empty but does not appear at
// LineStart..LineEnd in sourceLines. Used by Phase A to decide whether
// AutoRepairAnchors fully resolved the drift or LLM dispatch is still
// needed.
//
// This is intentionally a pure function over sourceLines so callers can
// reuse a single read of the parent .md across both the repair attempt and
// the post-repair re-check. The method form (*Sidecar).IsStale below reads
// the parent file from disk and remains the canonical drift gate.
func IsStale(sc *Sidecar, sourceLines []string) bool {
	if sc == nil {
		return false
	}
	if sc.MigrationPreserved != nil {
		return true
	}
	for _, d := range sc.Directives {
		if stale, _ := AnyAnchorStale(d, sourceLines); stale {
			return true
		}
	}
	for _, p := range sc.Prohibitions {
		for _, se := range p.Anchors() {
			if excerptStale(se, sourceLines) {
				return true
			}
		}
	}
	return false
}

// excerptStale reports whether the quote at LineStart..LineEnd is missing
// from sourceLines. Empty quote → never stale (no anchor to drift against).
// Exact containment is checked first; a whitespace-normalized comparison is
// the fallback, so a quote that differs from the prose only in collapsed
// whitespace (joined wrapped lines, stripped indentation) does not count
// as drift — semantic changes to the prose still do.
func excerptStale(se SourceExcerpt, sourceLines []string) bool {
	quote := strings.TrimSpace(se.Quote)
	if quote == "" {
		return false
	}
	if se.LineStart < 1 || se.LineEnd < se.LineStart {
		return true
	}
	if se.LineStart > len(sourceLines) || se.LineEnd > len(sourceLines) {
		return true
	}
	passage := strings.Join(sourceLines[se.LineStart-1:se.LineEnd], "\n")
	if strings.Contains(passage, quote) {
		return false
	}
	return !strings.Contains(normalizeWS(passage), normalizeWS(quote))
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// IsStale reports whether the sidecar's recorded directive quotes still
// match the live parent .md prose,'s drift contract:
// the sidecar is stale if any directive's source_excerpt.quote no longer
// appears at line_start..line_end in the parent body.
//
// projectRoot is used to resolve sidecar.path when it is relative.
// reason is empty when not stale; populated with the first violation found
// when stale (the dispatcher uses it for log lines, not control flow).
//
// Two-phase upgrade path: when MigrationPreserved is non-nil, the sidecar
// is a freshly-migrated skeleton awaiting its first canonical extraction.
// Always stale — compile's Phase A MUST dispatch the sidecar-extractor so
// the legacy directives in MigrationPreserved get re-anchored into the
// canonical directives field with real source_excerpts. Phase B then
// strips MigrationPreserved.
func (s *Sidecar) IsStale(projectRoot string) (stale bool, reason string, err error) {
	if s.MigrationPreserved != nil {
		return true, "migration-preserved baseline awaiting first canonical extraction", nil
	}

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
		// Report the OFFENDING anchor's index and range, not the directive's
		// first anchor: with 1..N anchors, naming the wrong span sends the
		// reader to prose that is perfectly fine.
		for ai, se := range d.Anchors() {
			if se.LineStart > len(lines) || se.LineEnd > len(lines) {
				return true, fmt.Sprintf("directive[%d].anchor[%d]: lines %d-%d outside body length %d",
					i, ai, se.LineStart, se.LineEnd, len(lines)), nil
			}
			if excerptStale(se, lines) {
				return true, fmt.Sprintf("directive[%d].anchor[%d]: quote not found at lines %d-%d",
					i, ai, se.LineStart, se.LineEnd), nil
			}
		}
	}
	return false, "", nil
}

// isDefaultFallbackExcerpt reports whether d's source_excerpt looks like the
// "no anchor available" default produced by migrate's findDirectiveSource
// when sentinel text didn't anchor to differently-phrased prose. Two shapes:
//
// 1. Full-fallback: line_start == line_end == 1 AND quote == directive text
// (when directive text ≤ 200 chars; migrate writes it verbatim).
// 2. Truncated-fallback: line_start == line_end == 1 AND len(quote) == 200
// AND directive text starts with quote (when directive text > 200 chars
// migrate truncates the quote to fit the schema's source_excerpt bounds).
//
// Both patterns have no real source anchor, so drift detection is undefined
// and we skip it. See applyArtifact in tools/edikt/cmd/migrate_sidecars.go.
func isDefaultFallbackExcerpt(d Directive) bool {
	// Ported to v2: a directive is a fallback only when EVERY anchor is one.
	// Judging by the first anchor alone would let a genuine second anchor be
	// discarded because the first happened to be a migrate fallback, which is
	// precisely the context-loss the multi-anchor shape exists to prevent.
	anchors := d.Anchors()
	if len(anchors) == 0 {
		return false
	}
	for _, se := range anchors {
		if !excerptIsFallback(se, d.Text) {
			return false
		}
	}
	return true
}

// excerptIsFallback is the single-anchor form of the fallback test. Both v1 and
// v2 route through it, so the two shapes cannot be judged differently.
func excerptIsFallback(se SourceExcerpt, text string) bool {
	if se.LineStart != 1 || se.LineEnd != 1 {
		return false
	}
	q := strings.TrimSpace(se.Quote)
	t := strings.TrimSpace(text)
	if q == t {
		return true
	}
	if len(se.Quote) == 200 && strings.HasPrefix(text, se.Quote) {
		return true
	}
	return false
}

// AnyAnchorStale reports whether ANY anchor of the directive no longer matches
// the parent prose, and which one.
//
// ANY, not all: a directive grounded in three spans is wrong the moment one of
// them stops matching, because the rule depended on that span. Requiring all
// anchors to break before reporting drift would silently downgrade a real
// defect in proportion to how well-anchored the directive was — the better the
// extraction, the quieter the alarm.
//
// Returns (false, -1) when the directive has no anchors: that is UNMEASURED,
// and callers must not read it as "not stale" (INV-013).  edikt-guard:allow
func AnyAnchorStale(d Directive, sourceLines []string) (bool, int) {
	if isDefaultFallbackExcerpt(d) {
		return false, -1
	}
	for i, se := range d.Anchors() {
		if excerptStale(se, sourceLines) {
			return true, i
		}
	}
	return false, -1
}
