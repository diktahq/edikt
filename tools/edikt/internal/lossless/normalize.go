// Package lossless holds the (modality, noun-phrase) normalisation
// primitives: modality classing, noun-phrase normalisation and a Levenshtein
// ratio. They were built for the v0.4.3 → v0.6.0 lossless-set check, which now
// lives with the lifter in internal/migrate (ADR-082 (c)); the contradiction
// detector and pin preservation use the same primitives through
// normalize_export.go. Pure Go, no LLM.
package lossless

import (
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

var (
	// refIDStripRe matches the WHOLE `(ref: …)` parenthetical so the
	// noun-phrase normalisation drops it cleanly. Must absorb optional
	// `+ manual` suffix used by Phase 7 add-manual-directive auto-tags.
	refIDStripRe = regexp.MustCompile(`\(ref:[^)]*\)`)
	modalRe      = regexp.MustCompile(`(?i)\b(MUST NOT|MUST|SHOULD NOT|SHOULD|MAY|NEVER|ALWAYS|DO NOT)\b`)
	wsRe         = regexp.MustCompile(`\s+`)
	articleRe    = regexp.MustCompile(`(?i)\b(a|an|the)\b`)
)

// modalityClass folds variant spellings into a canonical class. MUST NOT
// and NEVER and DO NOT are one class (prohibition). MUST and ALWAYS are
// another (mandate). SHOULD NOT and SHOULD and MAY remain distinct.
func modalityClass(raw string) string {
	u := strings.ToUpper(raw)
	switch u {
	case "MUST NOT", "NEVER", "DO NOT":
		return "PROHIBITION"
	case "MUST", "ALWAYS":
		return "MANDATE"
	case "SHOULD":
		return "SHOULD"
	case "SHOULD NOT":
		return "SHOULD-NOT"
	case "MAY":
		return "MAY"
	}
	return strings.ToUpper(raw)
}

// normalizeNounPhrase produces the comparable form used for fuzzy
// matching. Drops modality, ref tag, articles; lowercases; NFKC; collapses
// whitespace.
func normalizeNounPhrase(text string) string {
	s := refIDStripRe.ReplaceAllString(text, "")
	s = modalRe.ReplaceAllString(s, "")
	s = norm.NFKC.String(s)
	s = strings.ToLower(s)
	s = articleRe.ReplaceAllString(s, "")
	s = wsRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// levenshteinRatio = lev(a, b) / max(len(a), len(b)). Returns 0 when both
// inputs are empty.
func levenshteinRatio(a, b string) float64 {
	la, lb := len(a), len(b)
	if la == 0 && lb == 0 {
		return 0
	}
	d := levenshtein(a, b)
	mx := la
	if lb > mx {
		mx = lb
	}
	return float64(d) / float64(mx)
}

// levenshtein is a standard iterative two-row implementation. ~30 LOC.
// Same algorithm as sidecardiff.Levenshtein but the call sites have
// different normalisation pipelines, so we keep a local copy rather than
// importing across the package boundary.
func levenshtein(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			ins := curr[j-1] + 1
			del := prev[j] + 1
			sub := prev[j-1] + cost
			m := ins
			if del < m {
				m = del
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
