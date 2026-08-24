package cmd

// nextid_scope.go — where `next-id` looks, and what it says it looked at.
//
// The allocator read ONE directory (a single os.ReadDir) and printed the result
// as though it described the corpus. A consumer holding a checkout under
// .claude/worktrees/ was handed an id that already existed there, sidecar and
// all — and the "Existing …" line listed only the main tree's, reading as
// authoritative while being partial.
//
// Minting a taken id corrupts silently and surfaces at merge. Worse than a
// number clash: an id is a relationship anchor. In the reported case the
// occupant was active, dated, and already held the exact role the new record
// claimed to be establishing, so committing would have produced structural
// drift rather than a rename conflict.
//
// SOURCE: `git worktree list`, not a scan of local refs.
//
// Worktrees are checkouts someone is holding right now, and the list includes
// ones OUTSIDE the repository directory — this repo's own live in /private/tmp,
// which is why the defect does not reproduce here and why a fix that merely
// walked the repo would still collide. Scanning refs would also cover branchless
// work, but it proposes ids around branches that may never merge: this repo
// carries twelve local branches including several stale backups and audits, and
// every one of them would inflate the counter permanently against abandoned
// work.
//
// DEGRADATION IS THE POINT. When git is absent, the directory is not a
// repository, or the command fails, the allocator SAYS it scanned only the
// working tree. Falling back silently while printing the same
// authoritative-looking line is the bug, not the mitigation.

import (
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// scanScope records where ids were collected from, so the printed listing can
// state what it actually covered rather than implying the corpus.
type scanScope struct {
	// Roots are the checkout roots scanned, main tree first.
	Roots []string
	// Degraded is true when the worktree enumeration could not run.
	Degraded bool
	// Reason explains a degraded scan in one clause, for the user-facing line.
	Reason string
}

// worktreeRoots enumerates every checkout on disk via `git worktree list`.
//
// Returns a degraded scope carrying the reason rather than an error: an
// allocator that refused to run without git would be worse than one that runs
// and says what it could see. What it must never do is stay quiet.
func worktreeRoots(projectRoot string) scanScope {
	sc := scanScope{Roots: []string{projectRoot}}

	if _, err := exec.LookPath("git"); err != nil {
		sc.Degraded = true
		sc.Reason = "git is not on PATH"
		return sc
	}

	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		sc.Degraded = true
		sc.Reason = "`git worktree list` failed here (not a git repository, or git refused)"
		return sc
	}

	seen := map[string]bool{filepath.Clean(projectRoot): true}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		p := filepath.Clean(strings.TrimSpace(strings.TrimPrefix(line, "worktree ")))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		sc.Roots = append(sc.Roots, p)
	}
	// Deterministic order after the main tree, so two runs read the same.
	if len(sc.Roots) > 1 {
		rest := sc.Roots[1:]
		sort.Strings(rest)
	}
	return sc
}

// describe renders the one-line scope statement appended to the listing.
//
// It always states a count, and it always distinguishes "one checkout" from
// "the corpus". The reporter's words for why: theirs "read as authoritative
// while being partial".
func (s scanScope) describe() string {
	if s.Degraded {
		return "Scanned: this working tree only — " + s.Reason +
			", so ids held in other checkouts are NOT covered"
	}
	if len(s.Roots) <= 1 {
		return "Scanned: 1 checkout (this working tree; no other worktrees registered)"
	}
	return "Scanned: " + itoa(len(s.Roots)) + " checkouts (this working tree and " +
		itoa(len(s.Roots)-1) + " registered worktree(s))"
}
