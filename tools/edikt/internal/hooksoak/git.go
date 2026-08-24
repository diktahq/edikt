package hooksoak

// git.go — resolves hook-soak.v1's `elapsed_days` from git, never from a
// gitignored state file.
//
// WHY THIS MATTERS (D2 owner ruling, 2026-08-20, quoted in the schema):
// "elapsed days is a reported fact, derived from git, not a gitignored state
// file". A `.edikt/state/` timestamp would make the soak clock local to
// whichever machine first ran the check — unverifiable by a second engineer,
// and silently reset by a fresh clone. `git log -1 --format=%cI <commit>` is
// the opposite: any clone that has SinceCommit in its history computes the
// identical commit date, so two engineers on the same commit always compute
// the same soak age.

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GitCommitDate runs the exact command hook-soak.v1's own field description
// names verbatim — `git log -1 --format=%cI <commit>` — against repoRoot, so
// elapsed_days is independently reproducible from git history alone by
// anyone reading the soak record, not merely by trusting the number it
// states.
//
// %cI is the commit date (not the author date) in strict ISO 8601, which
// time.RFC3339 parses without a format table of its own.
func GitCommitDate(repoRoot, commit string) (time.Time, error) {
	cmd := exec.Command("git", "log", "-1", "--format=%cI", commit)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return time.Time{}, fmt.Errorf("git log -1 --format=%%cI %s (in %s): %w", commit, repoRoot, err)
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		// `git log` on an unknown commit exits non-zero and is already
		// caught above by err != nil in every git version this repo has
		// ever run against, but an EMPTY, zero-exit-status output is treated
		// as the same failure rather than trusted: a resolver that turned
		// "found nothing" into a zero time.Time would report a soak window
		// that is centuries long instead of naming the real problem.
		return time.Time{}, fmt.Errorf("git log -1 --format=%%cI %s (in %s): commit not found", commit, repoRoot)
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse commit date %q for %s: %w", s, commit, err)
	}
	return t, nil
}

// ElapsedDays is the whole number of days between commitDate and now — the
// hook-soak.v1 `elapsed_days` fact.
//
// Never negative: a commit dated after now (clock skew, or a --since-commit
// that names the wrong ref) clamps to 0 rather than reporting a day count
// that would understate how little time has actually passed.
func ElapsedDays(commitDate, now time.Time) int {
	d := now.Sub(commitDate)
	days := int(d.Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}
