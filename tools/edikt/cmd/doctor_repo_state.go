package cmd

// doctor_repo_state.go — repository state, DERIVED, so that no document has to
// restate it.
//
// Four errors were found in the handoff records in a single afternoon, and they
// were one defect: hand-written prose restating machine-readable state. Two
// worktrees holding unpushed work on branches with no upstream were mentioned
// in neither handoff document, so a reader concluded all work sat on the one
// branch they could see. A commit count was written as "227 ahead"; it was 234
// when read and 236 an hour later.
//
// The lesson is NOT "never write a number down". A generated count never lies.
// The count comes back here, derived at run time, and the documents point at it.
//
// THE UPSTREAM-LESS BRANCH IS THE DANGEROUS CASE. `git rev-list --count
// origin/<branch>..HEAD` does not return zero for a branch that tracks nothing —
// it ERRORS, because origin/<branch> does not exist. A counter that reads the
// number and ignores the exit status silently skips exactly the branches whose
// work exists in no remote at all. That is how two of them stayed invisible.
// So they are enumerated separately, before the merely-ahead ones.
//
// READ-ONLY, AND NO NETWORK. This check never fetches. Every number it prints
// is measured against refs already on disk, and the header SAYS so: an unfetched
// origin ref is a claim about the last fetch, not about the remote, and printing
// "up to date" on that basis would be a control reporting a conclusion it never
// observed (INV-013).  // edikt-guard:allow
//
// ONE SCANNER. The checkout enumeration is `worktreeRoots` from
// nextid_scope.go — the same code that reports next-id's scope. A second
// walker here would be a second definition of "every checkout", and two
// definitions drift; that is the whole lesson this check exists to carry
// (INV-017).  // edikt-guard:allow

import (
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// repoCheckout is one registered worktree and the branch checked out in it.
type repoCheckout struct {
	Root string
	// Branch is empty when the checkout is on a detached HEAD.
	Branch string
}

// repoBranch is one local branch and how far it has run ahead of whatever it
// can be compared against.
type repoBranch struct {
	Name string
	// Upstream is the short tracking ref, empty when the branch tracks nothing.
	Upstream string
	// Unpushed is commits on the branch that are not on its upstream — or, for
	// a branch with no upstream, commits present in no remote ref at all.
	Unpushed int
	// Measured is false when the count could not be derived; the report then
	// says so rather than printing a zero it did not observe.
	Measured bool
	// UpstreamOnDisk records whether the configured tracking ref actually
	// resolves. Consulted only for an unmeasured branch, and only so the report
	// can name the reason it observed rather than guess at one.
	UpstreamOnDisk bool
}

// repoStateReport is everything this check found, all of it derived.
type repoStateReport struct {
	Top       string
	Checkouts []repoCheckout
	// NoUpstream and Ahead are kept apart deliberately — see the file header.
	NoUpstream []repoBranch
	Ahead      []repoBranch
	// Unmeasured is the third class this repository turned out to contain: a
	// branch whose upstream is CONFIGURED but whose tracking ref is not on
	// disk, usually because the remote branch was deleted. rev-list errors for
	// it exactly as it does for an upstream-less branch, and calling such a
	// branch "level" or "0 ahead" would be the same unobserved conclusion in a
	// second costume.
	Unmeasured []repoBranch
	// Level is the roll-up of branches that match their upstream exactly.
	Level int
	// Degraded is true when the worktree enumeration could not run; Reason
	// carries the scanner's own words for why.
	Degraded bool
	Reason   string
}

// gitRead runs a read-only git command in dir and returns its trimmed stdout.
// The second return is false on any failure, so a caller can never mistake a
// failed command for a zero.
func gitRead(dir string, args ...string) (string, bool) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimRight(string(out), "\n"), true
}

// collectRepoState gathers the report. The bool is false when there is nothing
// to measure — cwd is not inside a git repository — which is not a finding.
func collectRepoState(cwd string) (repoStateReport, bool) {
	var r repoStateReport

	top := gitTopLevel(cwd)
	if top == "" {
		return r, false
	}
	r.Top = top

	// The one scanner. Not a second walk of `git worktree list`.
	sc := worktreeRoots(top)
	r.Degraded, r.Reason = sc.Degraded, sc.Reason
	for _, root := range sc.Roots {
		r.Checkouts = append(r.Checkouts, repoCheckout{Root: root, Branch: sc.Branch[root]})
	}

	// Every local branch and what it tracks. Full refs are carried alongside
	// the short ones so the counts below name refs/heads/x rather than x — a
	// branch sharing a name with a path is ambiguous to rev-list otherwise.
	const fmtRefs = "%(refname:short)%09%(refname)%09%(upstream:short)%09%(upstream)"
	out, ok := gitRead(top, "for-each-ref", "--format="+fmtRefs, "refs/heads/")
	if !ok || strings.TrimSpace(out) == "" {
		return r, true
	}

	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) != 4 || f[0] == "" {
			continue
		}
		short, full, upShort, upFull := f[0], f[1], f[2], f[3]
		b := repoBranch{Name: short, Upstream: upShort}

		// DERIVED, both ways, and never read from a file or a constant.
		//   tracking   — commits on the branch that its upstream does not have
		//   no upstream — commits present in no remote ref at all, which is the
		//                 only honest comparison available for it
		var args []string
		if upFull == "" {
			args = []string{"rev-list", "--count", full, "--not", "--remotes"}
		} else {
			args = []string{"rev-list", "--count", upFull + ".." + full}
		}
		if s, ok := gitRead(top, args...); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
				b.Unpushed, b.Measured = n, true
			}
		}

		switch {
		case upFull == "":
			r.NoUpstream = append(r.NoUpstream, b)
		case !b.Measured:
			// Say which of the two it is, having looked.
			_, b.UpstreamOnDisk = gitRead(top, "rev-parse", "--verify", "--quiet", upFull)
			r.Unmeasured = append(r.Unmeasured, b)
		case b.Unpushed == 0:
			r.Level++
		default:
			r.Ahead = append(r.Ahead, b)
		}
	}

	for _, s := range []*[]repoBranch{&r.NoUpstream, &r.Ahead, &r.Unmeasured} {
		g := *s
		sort.Slice(g, func(i, j int) bool { return g[i].Name < g[j].Name })
	}
	return r, true
}

// nameWidth is the column the branch names line up in, so fifteen branches
// read as a table rather than fifteen sentences.
func nameWidth(groups ...[]repoBranch) int {
	w := 0
	for _, g := range groups {
		for _, b := range g {
			if len(b.Name) > w {
				w = len(b.Name)
			}
		}
	}
	if w > 44 {
		w = 44
	}
	return w
}

// commitsPhrase renders a count that may not have been measured. An
// unmeasured count prints as unmeasured, never as zero.
func commitsPhrase(b repoBranch) string {
	if !b.Measured {
		return "an UNMEASURED number of commits (the count command failed)"
	}
	return fmt.Sprintf("%d commit(s)", b.Unpushed)
}

// reportRepoState prints the group. Informational: it changes no exit code.
// The finding it exists to prevent is a document that is confidently wrong,
// not an installation that is broken.
func reportRepoState(r repoStateReport, out io.Writer) {
	fmt.Fprintf(out, "  repository state (read-only — doctor does not fetch; every count below is measured "+
		"against the last-fetched remote refs, not against the remote)\n")

	if r.Degraded {
		fmt.Fprintf(out, "    checkouts: this working tree only — %s, so other checkouts are NOT covered\n", r.Reason)
	} else {
		fmt.Fprintf(out, "    checkouts: %d\n", len(r.Checkouts))
	}
	for _, c := range r.Checkouts {
		branch := c.Branch
		if branch == "" {
			branch = "(detached HEAD)"
		}
		fmt.Fprintf(out, "      %s  %s\n", c.Root, branch)
	}

	fmt.Fprintf(out, "    branches: %d with NO UPSTREAM, %d ahead of their last-fetched upstream, %d UNMEASURED, %d level\n",
		len(r.NoUpstream), len(r.Ahead), len(r.Unmeasured), r.Level)

	// The reason belongs to the class, not to each member of it. Printed once.
	if len(r.NoUpstream) > 0 {
		fmt.Fprintf(out, "      why NO UPSTREAM is its own class: `git rev-list --count origin/<branch>..HEAD` ERRORS for "+
			"these rather than returning a number, so a counter that reads the number and ignores the exit status "+
			"skips every one of them in silence\n")
	}

	w := nameWidth(r.NoUpstream, r.Ahead, r.Unmeasured)
	for _, b := range r.NoUpstream {
		fmt.Fprintf(out, "      NO UPSTREAM  %-*s  %s present in no remote ref\n", w, b.Name, commitsPhrase(b))
	}
	for _, b := range r.Ahead {
		fmt.Fprintf(out, "      ahead        %-*s  %s not in %s as of the last fetch\n", w, b.Name, commitsPhrase(b), b.Upstream)
	}
	for _, b := range r.Unmeasured {
		why := b.Upstream + " is configured but not on disk — the remote branch was deleted, or never fetched"
		if b.UpstreamOnDisk {
			why = b.Upstream + " is on disk, but the count command failed anyway"
		}
		fmt.Fprintf(out, "      UNMEASURED   %-*s  count UNMEASURED, NOT zero: %s\n", w, b.Name, why)
	}
}

// runRepoStateCheck is doctor's entry point. Returns false when cwd is not a
// git repository, so the caller can stay silent rather than report an absence
// it cannot interpret.
func runRepoStateCheck(cwd string, out io.Writer) bool {
	r, ran := collectRepoState(cwd)
	if !ran {
		return false
	}
	reportRepoState(r, out)
	return true
}
