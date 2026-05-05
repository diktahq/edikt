// Package dircheck implements the three directive-quality checks
// previously embedded as a Python heredoc in
// commands/gov/_shared-directive-checks.md (lines 89-140 pre-Phase 11.5).
//
// Pure deterministic Go port — same input always produces the same
// warning text, byte-for-byte. Cross-caller consistency between
// /edikt:gov:compile and /edikt:gov:review is preserved by funnelling
// both invocations through this package (or its CLI wrapper at
// `bin/edikt gov directive-check`).
//
// Three checks (per the source markdown):
//
//	A — FR-003a: directive has >1 sentence but no canonical_phrases.
//	B — FR-003b: a canonical_phrase is not a substring of the body.
//	C — AC-003c: a no_directives_reason fails length / placeholder checks.
//
// All checks are warn-only (AC-021 grace period). The CLI MUST exit 0
// even when warnings fire — it never blocks a caller.
package dircheck

import (
	"fmt"
	"regexp"
	"strings"
)

// Input mirrors the JSON contract documented in
// _shared-directive-checks.md §"Input contract (stdin)".
type Input struct {
	ADRID              string   `json:"adr_id"`
	DirectiveBody      string   `json:"directive_body"`
	CanonicalPhrases   []string `json:"canonical_phrases"`
	NoDirectivesReason *string  `json:"no_directives_reason"`
}

// refTailRe strips the trailing `(ref: …)` clause before sentence counting
// (matches the python regex `\s*\(ref:[^)]+\)\s*$`).
var refTailRe = regexp.MustCompile(`\s*\(ref:[^)]+\)\s*$`)

// sentenceSplitRe splits on `[.;!?]` followed by whitespace OR at end of
// string (mirrors `(?<=[.;!?])\s+|(?<=[.;!?])$` from the python heredoc).
// Go's regexp does not support lookbehind, so we use FindAllStringIndex
// and reconstruct clauses manually.
var terminatorRe = regexp.MustCompile(`[.;!?]`)

// forbiddenPlaceholders mirrors the python `{"tbd", "todo", "fix later"}` set.
var forbiddenPlaceholders = map[string]struct{}{
	"tbd":       {},
	"todo":      {},
	"fix later": {},
}

// Check runs the three directive-quality checks and returns the warning
// list. The list is empty when the directive is clean. Output strings
// match the python heredoc byte-for-byte — downstream tests rely on
// exact substring matches.
func Check(in Input) []string {
	var warnings []string

	// ── Check A: length vs canonical_phrases ─────────────────────────
	stripped := refTailRe.ReplaceAllString(strings.TrimRight(in.DirectiveBody, " \t"), "")
	clauses := splitSentences(stripped)
	count := len(clauses)

	if count > 1 && len(in.CanonicalPhrases) == 0 {
		warnings = append(warnings,
			fmt.Sprintf("[WARN] %s: directive has %d sentences but no canonical_phrases — run /edikt:adr:review --backfill",
				in.ADRID, count),
		)
	}

	// ── Check B: phrase-not-in-body ──────────────────────────────────
	bodyLower := strings.ToLower(in.DirectiveBody)
	for _, phrase := range in.CanonicalPhrases {
		p := strings.TrimSpace(phrase)
		if p == "" {
			continue
		}
		if !strings.Contains(bodyLower, strings.ToLower(p)) {
			warnings = append(warnings,
				fmt.Sprintf(`[WARN] %s: canonical_phrase "%s" not found in directive body`,
					in.ADRID, p),
			)
		}
	}

	// ── Check C: no-directives reason ────────────────────────────────
	if in.NoDirectivesReason != nil {
		raw := *in.NoDirectivesReason
		reason := strings.TrimSpace(raw)
		_, isForbidden := forbiddenPlaceholders[strings.ToLower(reason)]
		if reason == "" || len(reason) < 10 || isForbidden {
			warnings = append(warnings,
				fmt.Sprintf(`[WARN] %s: no-directives reason "%s" is not acceptable — provide a meaningful explanation ≥ 10 characters`,
					in.ADRID, raw),
			)
		}
	}

	return warnings
}

// splitSentences ports the python regex split:
//
//	re.split(r'(?<=[.;!?])\s+|(?<=[.;!?])$', body)
//
// The semantics: split immediately after `.` `;` `!` `?` if followed by
// whitespace, or at the end of the string. We walk the string by hand
// because Go's regexp lacks lookbehind.
func splitSentences(s string) []string {
	var clauses []string
	start := 0
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '.' || c == ';' || c == '!' || c == '?' {
			// Lookahead: end of string or whitespace.
			if i+1 == len(s) {
				clauses = append(clauses, strings.TrimSpace(s[start:i+1]))
				start = i + 1
				i++
				continue
			}
			next := s[i+1]
			if next == ' ' || next == '\t' || next == '\n' || next == '\r' {
				clauses = append(clauses, strings.TrimSpace(s[start:i+1]))
				// Skip the contiguous whitespace block so we do not
				// emit empty clauses for "  " runs.
				i++
				for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
					i++
				}
				start = i
				continue
			}
		}
		i++
	}
	if start < len(s) {
		clauses = append(clauses, strings.TrimSpace(s[start:]))
	}

	// Filter empty clauses (matches the python `[c for c in clauses if c.strip()]`).
	out := clauses[:0]
	for _, c := range clauses {
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}
