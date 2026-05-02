// Package redact scrubs credential-shaped substrings from text before it
// is written to disk or surfaced to logs. INV-007 requires benchmark JSONL
// to abort on credential-pattern detection; the analogous compile-errors
// log (Phase A failure capture) needs the same protection. This package
// is the shared scrubber both sites delegate to.
//
// The patterns are conservative: they target high-confidence credential
// shapes (AWS access keys, GitHub tokens, OpenAI keys, JWT-shaped base64,
// long base64 catch-all) so ordinary diagnostic text is not over-redacted.
// On match, the matched span is replaced with "<REDACTED:N bytes>" where
// N is the byte length of the original substring.
package redact

import (
	"fmt"
	"regexp"
)

// patterns is the ordered list of credential-shape regexes. Order matters
// because the catch-all (long base64) is a superset of more-specific
// shapes — applying it first would over-match. Each regex is anchored to
// require word boundaries so embedded text doesn't bleed into adjacent
// content.
var patterns = []*regexp.Regexp{
	// AWS access key ID: AKIA + 16 uppercase alphanumerics.
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),

	// GitHub fine-grained / classic / OAuth tokens.
	// ghp_, gho_, ghu_, ghs_, ghr_ followed by 36+ alphanumerics.
	regexp.MustCompile(`\bgh[posur]_[A-Za-z0-9]{36,255}\b`),

	// GitHub fine-grained personal access token (github_pat_).
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{82,}\b`),

	// OpenAI API keys: sk- prefix, 32+ alphanumerics. Anthropic keys
	// (sk-ant-) match the same prefix; covered by this pattern.
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{32,}\b`),

	// JWT: three base64url segments separated by dots. Header.Payload.Signature.
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),

	// Generic long base64-or-alphanumeric run (40+ chars, possibly with
	// trailing '='). This is the catch-all for tokens whose shape this
	// table does not enumerate explicitly. False-positive risk is real —
	// SHA-256 hex strings, signed git commit IDs, base64-encoded payloads
	// in legitimate diagnostics will match. The trade-off: when the
	// scrubber fires inside a Phase A failure log, the cost of an
	// over-redaction is a less-readable error message; the cost of an
	// under-redaction is a token leaking into a long-lived log file.
	// Per INV-007's "abort on credential-pattern detection" framing,
	// over-redaction is the safer side.
	regexp.MustCompile(`\b[A-Za-z0-9+/]{40,}={0,2}\b`),
}

// Scrub returns s with every credential-shaped substring replaced by a
// "<REDACTED:N bytes>" placeholder. Idempotent — running Scrub on already-
// scrubbed output is a no-op.
func Scrub(s string) string {
	for _, re := range patterns {
		s = re.ReplaceAllStringFunc(s, func(m string) string {
			return fmt.Sprintf("<REDACTED:%d bytes>", len(m))
		})
	}
	return s
}
