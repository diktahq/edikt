package idvalidate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A CITATION IS AN EXTERNALLY-CONTROLLED VALUE, and it is about to be handed
// to `git cat-file` or to the filesystem.
//
// The auditor resolves what a governance record cites — artifact ids, commit
// hashes, repository paths — and every one of those comes out of prose that a
// model wrote. Validating each at the boundary, before any lookup, is what
// keeps "a citation" from becoming an argv element or a path out of the tree.
var commitHashPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// CommitHash validates s as a git object id safe to pass as an argv element.
//
// Seven is the floor because that is git's own abbreviation minimum; forty is
// the ceiling because that is a full sha1. The pattern is lowercase hex and
// the input is lowercased first, so a hash pasted as `ABC1234` is ACCEPTED and
// normalised: git renders ids in lowercase, a citation written in uppercase is
// a transcription of the same object, and refusing it would report a typo as
// an invalid value. What the pattern excludes is everything that is not hex —
// which is where a shell metacharacter or a flag would have to live.
// Shape-only — whether the object EXISTS is a lookup the caller makes
// with `git cat-file -e`, and a missing object is an unresolved citation to
// report, not a validation failure.
func CommitHash(s string) error {
	n := strings.ToLower(normalize(s))
	if !commitHashPattern.MatchString(n) {
		return fmt.Errorf("invalid commit hash %q: must match %s after NFKC + whitespace-strip "+
			"(7–40 lowercase hex characters; git's own abbreviation floor is 7)", s, commitHashPattern.String())
	}
	return nil
}

// RepoRelPath validates s as a repository-relative path and returns its
// absolute resolved form.
//
// Three refusals, and the third is the one a textual check misses:
//
//  1. an absolute path, or an empty one;
//  2. a path containing a `..` segment — `../../etc/passwd` is a refusal,
//     never a resolved citation;
//  3. a path whose RESOLVED form leaves the repository. A symlink inside the
//     tree pointing out of it is repo-relative and contains no `..`, so only
//     following it catches the escape.
//
// A path that simply does not exist is reported as missing rather than
// invalid: a typo in a citation and an attempt to read outside the tree are
// different findings, and collapsing them would make the first look like the
// second.
func RepoRelPath(repoRoot, s string) (string, error) {
	n := normalize(s)
	if n == "" {
		return "", fmt.Errorf("invalid path: empty")
	}
	if filepath.IsAbs(n) {
		return "", fmt.Errorf("invalid path %q: must be repository-relative, not absolute", s)
	}
	for _, seg := range strings.Split(filepath.ToSlash(n), "/") {
		if seg == ".." {
			return "", fmt.Errorf("invalid path %q: contains a `..` segment", s)
		}
	}

	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	if r, rerr := filepath.EvalSymlinks(rootAbs); rerr == nil {
		rootAbs = r
	}
	joined := filepath.Join(rootAbs, n)

	// Confinement is checked on the RESOLVED path. A missing file cannot be
	// resolved, so its containing directory is checked instead — that is what
	// distinguishes "cites a file that is not there" from "points out of the
	// repository".
	resolved, rerr := filepath.EvalSymlinks(joined)
	if rerr != nil {
		dir, derr := filepath.EvalSymlinks(filepath.Dir(joined))
		if derr != nil {
			return "", fmt.Errorf("path %q does not exist (neither the file nor its directory resolves)", s)
		}
		if !within(rootAbs, dir) {
			return "", fmt.Errorf("invalid path %q: resolves outside the repository (%s)", s, dir)
		}
		return "", fmt.Errorf("path %q does not exist", s)
	}
	if !within(rootAbs, resolved) {
		return "", fmt.Errorf("invalid path %q: resolves outside the repository (%s)", s, resolved)
	}
	return resolved, nil
}

func within(root, p string) bool {
	if p == root {
		return true
	}
	return strings.HasPrefix(p, root+string(os.PathSeparator))
}
