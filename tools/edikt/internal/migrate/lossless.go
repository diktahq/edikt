package migrate

// lossless.go — the lifter's proof that nothing was dropped (ADR-082 (c):
// "parse a dead format, write a v2 sidecar, prove nothing was dropped" is one
// frozen unit). Relocated from internal/lossless/check.go; the text
// normalisers it compares with stay in internal/lossless, where the
// contradiction detector and pin-preservation also use them.
//
// For each directive in the legacy v0.4.3 sentinel block, the check asserts
// that the corresponding (modality, ref_id, normalized noun-phrase) tuple is
// covered by ONE OF the v0.6.0 sidecar's directives, prohibitions, or
// manual_directives. Coverage means same modality semantic class AND same
// ref_id AND noun-phrase Levenshtein ratio ≤ 0.10. Pure Go, no LLM.

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/lossless"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// refIDExtractRe captures the ID itself for tuple comparison.
var refIDExtractRe = regexp.MustCompile(`\(ref:\s*([\w-]+)`)

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
	sent, err := ExtractSentinel(string(legacyMarkdown))
	if err != nil || !sent.Present {
		return nil
	}
	return CheckLosslessAgainstDirectives(sent.Directives, sc)
}

// CheckLosslessAgainstDirectives is the underlying tuple comparator.
// For each legacy directive string, asserts that an equivalent
// (modality, ref_id, normalized noun-phrase) tuple appears in ONE OF
// the v0.6.0 sidecar's directives, prohibitions, or manual_directives.
// Returns a slice of Loss entries describing each missed tuple.
//
// Used by:
// - CheckLossless (above) — parses the legacy block out of markdown
// - tools/edikt/internal/govrun/twophase.go Phase B verification —
// receives MigrationPreserved.Directives directly (no markdown
// re-parsing needed since migrate already preserved them verbatim)
//
// Pure Go, no LLM. Deterministic.
func CheckLosslessAgainstDirectives(legacyDirectives []string, sc *sidecar.Sidecar) []Loss {
	candidates := candidateSet(sc)

	var losses []Loss
	for _, legacy := range legacyDirectives {
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
	refID    string // e.g. "ADR-NNN", "INV-NNN"
	norm     string // lowercase + NFKC + articles stripped + collapsed
	source   string // "directives" | "prohibitions" | "manual_directives"
}

func tupleOf(text string) tuple {
	t := tuple{}
	t.modality = lossless.ModalityOf(text)
	if m := refIDExtractRe.FindStringSubmatch(text); len(m) > 1 {
		t.refID = strings.ToUpper(m[1])
	}
	t.norm = lossless.NormalizeNounPhrase(text)
	return t
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
		if lossless.LevenshteinRatio(legacy.norm, c.norm) <= 0.10 {
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
