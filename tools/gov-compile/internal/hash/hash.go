// Package hash computes source_hash and directives_hash per ADR-008.
//
// ADR-008:
//   - source_hash     = SHA-256 of artifact body with the directives block
//                       excluded, CRLF→LF normalized, trailing whitespace
//                       stripped per line.
//   - directives_hash = SHA-256 of the auto `directives:` list items joined
//                       by \n. manual_directives and suppressed_directives
//                       MUST NOT be hashed.
package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// SourceHash computes the deterministic hash of a document's body excluding
// its sentinel block. Caller supplies the already-excluded body (use
// Document.BodyExcludingSentinel()).
func SourceHash(bodyExcludingSentinel string) string {
	norm := normalize(bodyExcludingSentinel)
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}

// DirectivesHash hashes ONLY the auto `directives:` list (not manual, not
// suppressed). Items are joined with \n.
func DirectivesHash(directives []string) string {
	joined := strings.Join(directives, "\n")
	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:])
}

// normalize: CRLF→LF, strip trailing whitespace per line. Matches ADR-008's
// "normalized (CRLF→LF, trailing whitespace stripped)".
func normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}
