// Package idvalidate validates externally-controlled values at every
// dispatch boundary that passes them to a subprocess. The two values
// covered are artifact IDs (e.g. "ADR-NNN", "INV-NNN", a guideline slug)
// and artifact types ("adr", "invariant", "guideline"), which together
// form the prompt argv passed to the locked Phase A / migrate subagent
// dispatchers.
//
// Validators apply NFKC + casefold + whitespace-strip before allowlist
// comparison. Unicode lookalikes (Cyrillic 'А' vs ASCII 'A') and trailing
// whitespace cannot bypass the regex.
//
// The allowlists are deliberately narrow:
//
// ArtifactID — `^[A-Za-z][A-Za-z0-9_-]{0,80}$` (ASCII letter,
// then ≤80 alphanumeric/hyphen/underscore). Catches both formal IDs
// ("ADR-NNN") and guideline slugs ("error-handling"). Rejects
// newlines, backticks, shell metacharacters, instruction-injection
// text, and Unicode lookalikes.
//
// ArtifactType — exact match against {"adr", "invariant",
// "guideline"} after lowercasing.
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

	// ModelID — `^[a-z][a-z0-9-]{0,40}$`. An LLM model identifier
	// reaching an argv element, e.g. the extractor dispatcher's
	// `--model` flag. Operator-supplied via env or config, so it is
	// externally-controlled by INV-006's definition and validated at  // edikt-guard:allow
	// the boundary like every other such value. Mirrors the pattern
	// internal/benchmark/cheatrate already applies to its own
	// adversary model; the two are independent by design (cheatrate
	// keeps its validators local to avoid coupling the benchmark path
	// to the dispatch path).
	modelIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,40}$`)
)

// ModelID validates s as a safe LLM model identifier for argv.
// Applies NFKC + whitespace-strip + lowercase before the allowlist check.
//
// Deliberately shape-only: it does not check the id against a list of
// known models. A closed list would have to be edited every time a model
// ships, and a stale list refusing a valid model is a worse failure than
// a typo reaching the CLI, which reports the bad id itself. The job here
// is keeping shell metacharacters and injection text out of argv.
func ModelID(s string) error {
	n := strings.ToLower(normalize(s))
	if !modelIDPattern.MatchString(n) {
		return fmt.Errorf("invalid model id %q: must match %s after NFKC + whitespace-strip", s, modelIDPattern.String())
	}
	return nil
}

// ArtifactID validates s as a safe artifact identifier.
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
