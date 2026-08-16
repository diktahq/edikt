package lossless

// This file exports the (modality, noun-phrase) normalization primitives
// ADR-034's lossless gate already built, for reuse by the contradiction  edikt-guard:allow
// detector (SPEC-010 phase 9, AC-9.1/9.2). Per docs/internal/audits/  edikt-guard:allow
// AUDIT-2026-08-09-semantica-external-comparison.md § B1: "the noun-phrase
// primitive already exists in our tree ... a same-topic opposing-modality
// check can be built on the normalizer that gate already uses." Wrapping
// rather than renaming the unexported originals keeps this package's
// internal call sites unchanged and makes the reuse an explicit, reviewable
// addition instead of a silent rename.

// ModalityClass canonicalizes a raw modal word/phrase ("MUST", "MUST NOT",
// "NEVER", ...) into its semantic class ("MANDATE", "PROHIBITION", ...).
func ModalityClass(raw string) string { return modalityClass(raw) }

// ModalityOf extracts the first modal word/phrase from a directive's full
// text and returns its canonical class, or "" if the text contains none.
// Mirrors tupleOf's own modality-extraction step.
func ModalityOf(text string) string {
	if m := modalRe.FindString(text); m != "" {
		return modalityClass(m)
	}
	return ""
}

// NormalizeNounPhrase strips modality, ref tag, and articles from a
// directive's text, then lowercases, NFKC-normalizes, and collapses
// whitespace — the comparable form used for fuzzy noun-phrase matching.
func NormalizeNounPhrase(text string) string { return normalizeNounPhrase(text) }

// LevenshteinRatio returns the normalized edit distance between two
// strings, in [0, 1], where 0 means identical.
func LevenshteinRatio(a, b string) float64 { return levenshteinRatio(a, b) }
