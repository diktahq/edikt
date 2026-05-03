// Package lossless implements the v0.4.3 → v0.6.0 lossless-set check
// (Phase 11 of PLAN-v060-governance-accuracy).
//
// For each directive in the legacy v0.4.3 sentinel block, the check
// asserts that the corresponding (modality, ref_id, normalized
// noun-phrase) tuple is covered by ONE OF the v0.6.0 sidecar's
// directives, prohibitions, or manual_directives. Coverage means same
// modality semantic class AND same ref_id AND noun-phrase Levenshtein
// ratio ≤ 0.10.
//
// Pure Go, no LLM. Honours ADR-030 — the lossless check is a tier-2
// deterministic primitive.
package lossless

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/parse"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"golang.org/x/text/unicode/norm"
)

// Loss describes one v0.4.3 tuple that the v0.6.0 sidecar fails to cover.
type Loss struct {
	Type       string `json:"type"`        // "missing-modality" | "missing-ref-id" | "missing-noun-phrase"
	LegacyText string `json:"legacy_text"` // verbatim v0.4.3 directive
	ExpectedIn string `json:"expected_in"` // "directives" | "prohibitions" | "manual_directives"
	Reason     string `json:"reason"`      // human-readable diagnostic
}

// CheckLossless walks every directive in the legacy v0.4.3 sentinel block
// and asserts that an equivalent tuple is present in the v0.6.0 sidecar.
// Returns a slice of Loss entries describing each missed tuple. An empty
// slice means the v0.6.0 sidecar is at least as faithful as the v0.4.3
// baseline.
//
// The legacy markdown MUST contain an `[edikt:directives:start/end]`
// sentinel block. If absent, returns nil (nothing to check against).
func CheckLossless(legacyMarkdown []byte, sc *sidecar.Sidecar) []Loss {
	sent, err := parse.ExtractSentinel(string(legacyMarkdown))
	if err != nil || !sent.Present {
		return nil
	}

	candidates := candidateSet(sc)

	var losses []Loss
	for _, legacy := range sent.Directives {
		t := tupleOf(legacy)
		if t.modality == "" && t.refID == "" && t.norm == "" {
			continue
		}

		matched, where := bestMatch(t, candidates)
		if matched {
			_ = where
			continue
		}

		// Diagnose the failure mode.
		switch {
		case !anyShareModality(t, candidates):
			losses = append(losses, Loss{
				Type:       "missing-modality",
				LegacyText: legacy,
				ExpectedIn: "directives",
				Reason:     fmt.Sprintf("no v0.6.0 entry uses modality %q", t.modality),
			})
		case !anyShareRefID(t, candidates):
			losses = append(losses, Loss{
				Type:       "missing-ref-id",
				LegacyText: legacy,
				ExpectedIn: "directives",
				Reason:     fmt.Sprintf("no v0.6.0 entry carries (ref: %s)", t.refID),
			})
		default:
			losses = append(losses, Loss{
				Type:       "missing-noun-phrase",
				LegacyText: legacy,
				ExpectedIn: "directives",
				Reason:     "no v0.6.0 entry within Levenshtein 0.10 of legacy noun-phrase",
			})
		}
	}
	return losses
}

// tuple captures (modality, ref_id, normalized noun-phrase) from a
// directive line. Used both for legacy parsing and for v0.6.0 candidate
// indexing.
type tuple struct {
	modality string // canonicalised semantic class — see modalityClass
	refID    string // e.g. "ADR-001", "INV-003"
	norm     string // lowercase + NFKC + articles stripped + collapsed
	source   string // "directives" | "prohibitions" | "manual_directives"
}

var (
	// refIDExtractRe captures the ID itself for tuple comparison.
	refIDExtractRe = regexp.MustCompile(`\(ref:\s*([\w-]+)`)
	// refIDStripRe matches the WHOLE `(ref: …)` parenthetical so the
	// noun-phrase normalisation drops it cleanly. Must absorb optional
	// `+ manual` suffix used by Phase 7 add-manual-directive auto-tags.
	refIDStripRe = regexp.MustCompile(`\(ref:[^)]*\)`)
	modalRe      = regexp.MustCompile(`(?i)\b(MUST NOT|MUST|SHOULD NOT|SHOULD|MAY|NEVER|ALWAYS|DO NOT)\b`)
	wsRe         = regexp.MustCompile(`\s+`)
	articleRe    = regexp.MustCompile(`(?i)\b(a|an|the)\b`)
)

func tupleOf(text string) tuple {
	t := tuple{}
	if m := modalRe.FindString(text); m != "" {
		t.modality = modalityClass(m)
	}
	if m := refIDExtractRe.FindStringSubmatch(text); len(m) > 1 {
		t.refID = strings.ToUpper(m[1])
	}
	t.norm = normalizeNounPhrase(text)
	return t
}

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

// candidateSet builds tuples for every searchable v0.6.0 entry across the
// three sources. ManualDirectives are stored as []string of full directive
// text including ref tail, mirroring the legacy shape.
func candidateSet(sc *sidecar.Sidecar) []tuple {
	if sc == nil {
		return nil
	}
	out := make([]tuple, 0, len(sc.Directives)+len(sc.Prohibitions)+len(sc.ManualDirectives))
	for _, d := range sc.Directives {
		t := tupleOf(d.Text)
		t.source = "directives"
		out = append(out, t)
	}
	for _, p := range sc.Prohibitions {
		t := tupleOf(p.Text)
		t.source = "prohibitions"
		out = append(out, t)
	}
	for _, m := range sc.ManualDirectives {
		t := tupleOf(m)
		t.source = "manual_directives"
		out = append(out, t)
	}
	return out
}

// bestMatch reports the best v0.6.0 candidate match for a legacy tuple,
// or false if no candidate satisfies all three axes (modality + ref_id +
// Levenshtein noun-phrase ratio ≤ 0.10).
func bestMatch(legacy tuple, candidates []tuple) (bool, string) {
	for _, c := range candidates {
		if c.modality != legacy.modality {
			continue
		}
		if legacy.refID != "" && c.refID != "" && c.refID != legacy.refID {
			continue
		}
		if levenshteinRatio(legacy.norm, c.norm) <= 0.10 {
			return true, c.source
		}
	}
	return false, ""
}

func anyShareModality(legacy tuple, candidates []tuple) bool {
	for _, c := range candidates {
		if c.modality == legacy.modality {
			return true
		}
	}
	return false
}

func anyShareRefID(legacy tuple, candidates []tuple) bool {
	for _, c := range candidates {
		if c.modality == legacy.modality && (legacy.refID == "" || c.refID == legacy.refID) {
			return true
		}
	}
	return false
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
