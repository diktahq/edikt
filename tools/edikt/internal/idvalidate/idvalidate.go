// Package idvalidate enforces INV-006 at every dispatch boundary that
// passes externally-controlled values to a subprocess. The two values
// covered are artifact IDs (e.g. "ADR-001", "INV-005", a guideline slug)
// and artifact types ("adr", "invariant", "guideline"), which together
// form the prompt argv passed to the locked Phase A / migrate subagent
// dispatchers.
//
// Per INV-006: validators apply NFKC + casefold + whitespace-strip
// before allowlist comparison. Unicode lookalikes (Cyrillic 'А' vs
// ASCII 'A') and trailing whitespace cannot bypass the regex.
//
// The allowlists are deliberately narrow:
//
//   ArtifactID — `^[A-Za-z][A-Za-z0-9_-]{0,80}$` (ASCII letter,
//   then ≤80 alphanumeric/hyphen/underscore). Catches both formal IDs
//   ("ADR-001") and guideline slugs ("error-handling"). Rejects
//   newlines, backticks, shell metacharacters, instruction-injection
//   text, and Unicode lookalikes.
//
//   ArtifactType — exact match against {"adr", "invariant",
//   "guideline"} after lowercasing.
//
// Failure mode: returns an error with the rejected value quoted. The
// caller MUST refuse the dispatch.
package idvalidate

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

var (
	artifactIDPattern   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,80}$`)
	artifactTypeAllowed = map[string]struct{}{
		"adr":       {},
		"invariant": {},
		"guideline": {},
	}
)

// ArtifactID validates s as a safe artifact identifier per INV-006.
// Applies NFKC + whitespace-strip before the allowlist check.
func ArtifactID(s string) error {
	n := normalize(s)
	if !artifactIDPattern.MatchString(n) {
		return fmt.Errorf("invalid artifact ID %q: must match %s after NFKC + whitespace-strip", s, artifactIDPattern.String())
	}
	return nil
}

// ArtifactType validates s as one of "adr", "invariant", "guideline"
// after NFKC + whitespace-strip + lowercase. Rejects everything else.
func ArtifactType(s string) error {
	n := strings.ToLower(normalize(s))
	if _, ok := artifactTypeAllowed[n]; !ok {
		return fmt.Errorf("invalid artifact type %q: must be one of adr|invariant|guideline", s)
	}
	return nil
}

func normalize(s string) string {
	s = norm.NFKC.String(s)
	s = strings.TrimSpace(s)
	return s
}
