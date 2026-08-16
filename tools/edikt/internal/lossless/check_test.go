package lossless

import (
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

func legacyMD(directives ...string) []byte {
	var sb strings.Builder
	sb.WriteString("# ADR-099\n\n## Decision\n\nstub.\n\n")
	sb.WriteString("[edikt:directives:start]: #\ntopic: test\nsignals:\n  - test\ndirectives:\n")
	for _, d := range directives {
		sb.WriteString("  - \"")
		sb.WriteString(strings.ReplaceAll(d, `"`, `\"`))
		sb.WriteString("\"\n")
	}
	sb.WriteString("[edikt:directives:end]: #\n")
	return []byte(sb.String())
}

func dir(text string) sidecar.Directive {
	return sidecar.Directive{
		Text: text,
		SourceExcerpt: sidecar.SourceExcerpt{
			LineStart: 1, LineEnd: 1, Quote: text,
		},
	}
}

func proh(text string) sidecar.Prohibition {
	return sidecar.Prohibition{
		Text: text,
		SourceExcerpt: sidecar.SourceExcerpt{
			LineStart: 1, LineEnd: 1, Quote: text,
		},
	}
}

func TestCheckLossless_FullMatch(t *testing.T) {
	legacy := legacyMD(
		"All hooks MUST emit JSON. (ref: INV-003)",
		"Tier-2 binaries MUST NOT call the LLM. (ref: ADR-030)",
	)
	sc := &sidecar.Sidecar{
		Directives: []sidecar.Directive{
			dir("All hooks MUST emit JSON. (ref: INV-003)"),
			dir("Tier-2 binaries MUST NOT call the LLM. (ref: ADR-030)"),
		},
	}
	if got := CheckLossless(legacy, sc); len(got) != 0 {
		t.Fatalf("want 0 losses, got %d: %+v", len(got), got)
	}
}

func TestCheckLossless_DirectiveCoveredByProhibition(t *testing.T) {
	legacy := legacyMD("Authors MUST NOT use the legacy adapter. (ref: ADR-007)")
	sc := &sidecar.Sidecar{
		Prohibitions: []sidecar.Prohibition{
			proh("Authors MUST NOT use the legacy adapter. (ref: ADR-007)"),
		},
	}
	if got := CheckLossless(legacy, sc); len(got) != 0 {
		t.Fatalf("want 0 losses (covered by prohibitions), got %d: %+v", len(got), got)
	}
}

func TestCheckLossless_DirectiveCoveredByManual(t *testing.T) {
	legacy := legacyMD("Stage 2 MUST enable prompt caching. (ref: ADR-001)")
	sc := &sidecar.Sidecar{
		ManualDirectives: []string{
			"Stage 2 MUST enable prompt caching. (ref: ADR-001 + manual)",
		},
	}
	if got := CheckLossless(legacy, sc); len(got) != 0 {
		t.Fatalf("want 0 losses (covered by manual), got %d: %+v", len(got), got)
	}
}

func TestCheckLossless_MissingModality(t *testing.T) {
	legacy := legacyMD("Hooks MUST NOT echo unredacted user content. (ref: INV-004)")
	// Sidecar has only SHOULD-class entries — modality mismatch.
	sc := &sidecar.Sidecar{
		Directives: []sidecar.Directive{
			dir("Hooks SHOULD redact user content. (ref: INV-004)"),
		},
	}
	losses := CheckLossless(legacy, sc)
	if len(losses) != 1 || losses[0].Type != "missing-modality" {
		t.Fatalf("want 1 missing-modality loss, got %+v", losses)
	}
}

func TestCheckLossless_MissingRefID(t *testing.T) {
	legacy := legacyMD("All hooks MUST emit JSON. (ref: INV-003)")
	sc := &sidecar.Sidecar{
		Directives: []sidecar.Directive{
			dir("All hooks MUST emit JSON. (ref: ADR-027)"), // wrong ref
		},
	}
	losses := CheckLossless(legacy, sc)
	if len(losses) != 1 || losses[0].Type != "missing-ref-id" {
		t.Fatalf("want 1 missing-ref-id loss, got %+v", losses)
	}
}

func TestCheckLossless_MissingNounPhrase(t *testing.T) {
	legacy := legacyMD("Hooks MUST emit structured JSON output to stdout. (ref: INV-003)")
	sc := &sidecar.Sidecar{
		Directives: []sidecar.Directive{
			// Same modality + ref, but unrelated subject — Levenshtein over 0.10.
			dir("Settings MUST be re-loaded on every PostToolUse hook fire. (ref: INV-003)"),
		},
	}
	losses := CheckLossless(legacy, sc)
	if len(losses) != 1 || losses[0].Type != "missing-noun-phrase" {
		t.Fatalf("want 1 missing-noun-phrase loss, got %+v", losses)
	}
}

func TestCheckLossless_NormalizationStripsArticles(t *testing.T) {
	// Legacy uses "the" + "a"; sidecar drops both. Should still match.
	legacy := legacyMD("The compile pipeline MUST be a deterministic merge. (ref: ADR-028)")
	sc := &sidecar.Sidecar{
		Directives: []sidecar.Directive{
			dir("Compile pipeline MUST be deterministic merge. (ref: ADR-028)"),
		},
	}
	if got := CheckLossless(legacy, sc); len(got) != 0 {
		t.Fatalf("want 0 losses (article strip), got %+v", got)
	}
}

func TestCheckLossless_NormalizationNFKC(t *testing.T) {
	// Legacy contains NFK-decomposable form (ﬁ ligature U+FB01 = "fi").
	// Sidecar uses the canonical "fi". NFKC should fold them.
	legacy := legacyMD("Configuration MUST be canoniﬁed before compile. (ref: ADR-027)")
	sc := &sidecar.Sidecar{
		Directives: []sidecar.Directive{
			dir("Configuration MUST be canonified before compile. (ref: ADR-027)"),
		},
	}
	if got := CheckLossless(legacy, sc); len(got) != 0 {
		t.Fatalf("want 0 losses (NFKC fold), got %+v", got)
	}
}

func TestCheckLossless_AbsentSentinelReturnsNil(t *testing.T) {
	noSentinel := []byte("# ADR-100\n\n## Decision\n\nNo sentinel block here.\n")
	if got := CheckLossless(noSentinel, &sidecar.Sidecar{}); got != nil {
		t.Fatalf("want nil on absent sentinel, got %+v", got)
	}
}

func TestLevenshteinRatio_EdgeCases(t *testing.T) {
	if got := levenshteinRatio("", ""); got != 0 {
		t.Fatalf("empty/empty: got %v want 0", got)
	}
	if got := levenshteinRatio("abc", ""); got != 1.0 {
		t.Fatalf("abc/empty: got %v want 1.0", got)
	}
	if got := levenshteinRatio("abc", "abc"); got != 0 {
		t.Fatalf("identical: got %v want 0", got)
	}
}
