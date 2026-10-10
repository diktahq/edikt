// Package audit is the deterministic half of the governance auditor: what a
// draft record can be checked for BEFORE a model reads it.
//
// Two things live here, and both exist because a model-authored draft passes a
// checklist that reads for presence rather than content:
//
//   - a prong verdict is not a verdict without the sentence that satisfied it.
//     A clean-context auditor passed a draft whose two "rejected alternatives"
//     were "keep relying on the tool" and "keep auditing by hand" — do-nothing
//     variants, which are the absence of a design rather than a competing one.
//     It read the section's presence. Requiring the quote is the general fix,
//     and it applies to every checklist gate, not only this one.
//
//   - restatement detection. A draft was an existing invariant's sentence with
//     one noun substituted, and nothing in the pipeline looked. Comparing the
//     draft's operative sentences against every directive in the corpus is
//     mechanical, so it runs before a model is asked anything.
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// ONE FUNCTION, TWO TREES, FOR NOW.
//
// Normalise is the content-derived identity function the brownfield lane
// authored for candidate identity. It is reproduced here rather than imported
// because that package lives on the lane's branch and reaching across lanes to
// import it is not a change this session may make. The two MUST collapse into
// one on merge — a rule stated twice is the drift this repo spends most of its
// gates preventing — and the parity cases in identity_test.go are the same
// cases the lane's own test pins, so a divergence shows up as a test failure
// rather than as two subtly different answers to "is this the same rule".
//
// Behaviour, unchanged from the lane's: lowercase, whitespace runs collapsed,
// punctuation stripped EXCEPT inside preserved spans (artifact ids and paths),
// so a rule naming one invariant and the same rule naming another stay two
// rules. Normalisation absorbs cosmetic edits; it deliberately does not try to
// absorb meaning-preserving rewrites, because it cannot tell those from
// meaning-changing ones.
var preservedSpan = regexp.MustCompile(`\b[A-Za-z]{2,6}-[0-9]{3,}\b|\b[\w.-]+(?:/[\w.-]+)+\b`)

var (
	nonWordRe    = regexp.MustCompile(`[^\p{L}\p{N}\s\x{0000}]`)
	whitespaceRe = regexp.MustCompile(`\s+`)
)

// Normalise reduces a rule's text to the form a digest is taken over.
func Normalise(s string) string {
	s = strings.ToLower(s)

	var held []string
	s = preservedSpan.ReplaceAllStringFunc(s, func(m string) string {
		held = append(held, m)
		return "\x00"
	})

	s = nonWordRe.ReplaceAllString(s, " ")

	for _, h := range held {
		s = strings.Replace(s, "\x00", h, 1)
	}

	s = whitespaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// RuleDigest is the identity of a rule: a digest over its kind and its
// normalised text. Evidence pointers, file paths outside the text and line
// numbers are deliberately absent, so the same rule found in a second place
// resolves to the same rule.
func RuleDigest(kind, text string) string {
	sum := sha256.Sum256([]byte(kind + "|" + Normalise(text)))
	return hex.EncodeToString(sum[:])
}
