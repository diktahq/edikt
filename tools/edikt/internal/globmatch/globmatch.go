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
// Semantics (unchanged from the vdGlobMatch behaviour this replaces):
//   - `*`  matches within a single path segment (filepath.Match semantics)
//   - `**` matches zero or more path segments
//
// Paths are compared as repo-relative slash-separated strings.
package globmatch

import (
	"path/filepath"
	"strings"
)

// Match reports whether path matches pattern.
//
// A pattern with no `**` delegates to filepath.Match. A pattern with `**` is
// split at the FIRST `**`: the literal prefix must prefix the path, and the
// remainder must match at some sub-path of what is left.
func Match(pattern, path string) bool {
	if pattern == "" {
		return false
	}
	if !strings.Contains(pattern, "**") {
		ok, _ := filepath.Match(pattern, path)
		return ok
	}

	idx := strings.Index(pattern, "**")
	prefix := pattern[:idx]
	rest := strings.TrimPrefix(pattern[idx+2:], "/")

	candidate := path
	if prefix != "" {
		if !strings.HasPrefix(candidate, prefix) {
			return false
		}
		candidate = candidate[len(prefix):]
	}

	if rest == "" {
		return true
	}

	// `**` absorbs zero or more segments, so try the suffix pattern at every
	// sub-path of the remaining candidate.
	for {
		if ok, _ := filepath.Match(rest, candidate); ok {
			return true
		}
		slash := strings.Index(candidate, "/")
		if slash < 0 {
			return false
		}
		candidate = candidate[slash+1:]
	}
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
