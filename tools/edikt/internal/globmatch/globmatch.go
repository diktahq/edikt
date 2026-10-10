// Package globmatch is the ONE place edikt decides whether a repo-relative
// path matches a sidecar `paths:` glob.
//
// Why it exists as its own package: two consumers must AGREE on what a glob
// means, and until now they each carried their own copy. `vdGlobMatch` in
// cmd/gov/verifydiff.go decided diff-time scope; the paths-proposal validator
// (internal/pathsproposal) decides whether a proposed glob matches anything at
// APPROVAL time. If those two disagree, a human approves a glob that matched a
// file during the ceremony and silently scopes nothing at diff time — an
// approval that grants no coverage while reading as coverage. GL-002: things
// that must agree are unified; things that must check each other are kept
// apart. These must agree.
//
// Semantics:
//   - `*`  matches within a single path segment (filepath.Match semantics)
//   - `**` matches zero or more path segments
//
// Paths are compared as repo-relative slash-separated strings.
//
// # Why this is segment-based and not prefix-based
//
// The first implementation split the pattern at the FIRST `**` and required
// the text before it to be a LITERAL prefix of the path (strings.HasPrefix).
// That comparison cannot be satisfied by a pattern whose prefix itself holds a
// wildcard, so EVERY glob with a `*` before a `**` matched nothing, in every
// repository — `internal/*/migrations/**/*.sql` never matched
// `internal/billing/migrations/001_init.sql`. The package doc above already
// claimed `*` and `**` compose; the code only honoured the composition when
// the `**` came first. Nothing crashed and nothing was reported: at diff time
// such a glob silently scopes nothing, which is verbatim the failure this
// package exists to prevent — an approval that reads as coverage and grants
// none.
//
// Matching therefore walks pattern segments against path segments, so a
// wildcard is evaluated wherever it appears rather than only after the first
// `**`. `*` is still handed to filepath.Match one segment at a time, which is
// what keeps it from crossing a separator.
package globmatch

import (
	"path/filepath"
	"strings"
)

// Match reports whether path matches pattern.
//
// A pattern with no `**` delegates to filepath.Match, so a single `*` never
// crosses a path separator. A pattern containing `**` is matched segment by
// segment: a segment that is exactly `**` absorbs zero or more whole path
// segments; every other segment must match its path segment under
// filepath.Match. A malformed pattern (filepath.ErrBadPattern) matches
// nothing, as before.
func Match(pattern, path string) bool {
	if pattern == "" {
		return false
	}
	if !strings.Contains(pattern, "**") {
		ok, _ := filepath.Match(pattern, path)
		return ok
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

// matchSegments matches slash-split pattern segments against slash-split path
// segments. `**` is the only segment that may consume more (or fewer) than one
// path segment.
func matchSegments(pat, path []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			// Zero or more segments: try every split point, shortest first.
			for i := 0; i <= len(path); i++ {
				if matchSegments(pat[1:], path[i:]) {
					return true
				}
			}
			return false
		}
		if len(path) == 0 {
			return false
		}
		ok, err := filepath.Match(pat[0], path[0])
		if err != nil || !ok {
			return false
		}
		pat, path = pat[1:], path[1:]
	}
	return len(path) == 0
}

// LiteralPrefix returns the leading portion of pattern before the first
// wildcard metacharacter (`*`, `?`, `[`, `{`).
//
// It is the anchor test a catch-all check needs: a glob whose literal prefix is
// empty is anchored nowhere and can match the whole repository.
func LiteralPrefix(pattern string) string {
	i := strings.IndexAny(pattern, "*?[{")
	if i < 0 {
		return pattern
	}
	return pattern[:i]
}
