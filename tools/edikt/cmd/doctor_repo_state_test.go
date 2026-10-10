package cmd

// doctor_repo_state_test.go — evidence for PLAN-doctor-repository-state Phase 1.
//
// Every test here builds a real repository and measures what doctor says about
// it. Nothing is asserted against the repository this suite happens to run in:
// the criterion's "three checkouts" is an illustration of the defect, not a
// property of the machine, and pinning it would make the suite report the
// developer's worktree layout rather than the check.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitAt runs git in dir with a fixed identity, failing the test on error.
func gitAt(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// repoFixture builds a working tree whose `main` is pushed to a bare origin on
// disk, so `main` has a real upstream and starts level with it.
func repoFixture(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	origin := filepath.Join(base, "origin.git")
	gitAt(t, base, "init", "-q", "--bare", origin)

	work := filepath.Join(base, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	gitAt(t, work, "init", "-q", ".")
	// Pin the branch name rather than inherit init.defaultBranch from whoever
	// is running the suite.
	gitAt(t, work, "symbolic-ref", "HEAD", "refs/heads/main")
	gitAt(t, work, "commit", "-q", "--allow-empty", "-m", "root")
	gitAt(t, work, "remote", "add", "origin", origin)
	gitAt(t, work, "push", "-q", "-u", "origin", "main")
	return work
}

func commitsOn(t *testing.T, work, branch string, n int) {
	t.Helper()
	gitAt(t, work, "checkout", "-q", branch)
	for i := 0; i < n; i++ {
		gitAt(t, work, "commit", "-q", "--allow-empty", "-m", "c")
	}
}

func repoStateText(t *testing.T, work string) (repoStateReport, string) {
	t.Helper()
	r, ran := collectRepoState(work)
	if !ran {
		t.Fatalf("collectRepoState reported nothing to measure inside a git repository at %s", work)
	}
	var buf bytes.Buffer
	reportRepoState(r, &buf)
	return r, buf.String()
}

func branchOf(r repoStateReport, root string) (string, bool) {
	for _, c := range r.Checkouts {
		if c.Root == root {
			return c.Branch, true
		}
	}
	return "", false
}

// resolved is the path git will print for p — on macOS a temp dir is reached
// through /var, and git reports /private/var.
func resolved(t *testing.T, p string) string {
	t.Helper()
	rp, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(rp)
}

// AC-1.1 — EVERY registered worktree with its branch, not only the one doctor
// was invoked from. The falsification is the before/after: the same code that
// reports three must report one when there is one, or it is reporting a
// constant.
func TestDoctorRepoState_EnumeratesAllWorktrees(t *testing.T) {
	work := repoFixture(t)

	before, beforeText := repoStateText(t, work)
	if len(before.Checkouts) != 1 {
		t.Fatalf("a repository with no registered worktrees must report 1 checkout, got %d:\n%s",
			len(before.Checkouts), beforeText)
	}

	extra := t.TempDir()
	wtA := filepath.Join(extra, "a")
	wtB := filepath.Join(extra, "b")
	gitAt(t, work, "worktree", "add", "-q", "-b", "feature/a", wtA)
	gitAt(t, work, "worktree", "add", "-q", "-b", "feature/b", wtB)

	r, text := repoStateText(t, work)
	if len(r.Checkouts) != 3 {
		t.Fatalf("want 3 checkouts (this one and two registered worktrees), got %d:\n%s", len(r.Checkouts), text)
	}
	for _, want := range []struct{ root, branch string }{
		{resolved(t, work), "main"},
		{resolved(t, wtA), "feature/a"},
		{resolved(t, wtB), "feature/b"},
	} {
		got, ok := branchOf(r, want.root)
		if !ok {
			t.Errorf("checkout %s is missing from the enumeration:\n%s", want.root, text)
			continue
		}
		if got != want.branch {
			t.Errorf("checkout %s: branch = %q, want %q", want.root, got, want.branch)
		}
		if !strings.Contains(text, want.root) || !strings.Contains(text, want.branch) {
			t.Errorf("the printed report omits %s / %s:\n%s", want.root, want.branch, text)
		}
	}

	// ONE SCANNER, not two (the phase gate). The enumeration must BE
	// nextid_scope.go's, so the two callers cannot drift apart.
	sc := worktreeRoots(gitTopLevel(work))
	if len(sc.Roots) != len(r.Checkouts) {
		t.Fatalf("doctor saw %d checkouts, worktreeRoots saw %d — two scanners have already diverged",
			len(r.Checkouts), len(sc.Roots))
	}
	for i, root := range sc.Roots {
		if r.Checkouts[i].Root != root {
			t.Errorf("checkout %d: doctor has %s, worktreeRoots has %s", i, r.Checkouts[i].Root, root)
		}
	}
	src, err := os.ReadFile("doctor_repo_state.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "worktreeRoots(") {
		t.Error("doctor_repo_state.go does not call worktreeRoots — the enumeration must reuse nextid_scope.go")
	}
	if strings.Contains(string(src), `"worktree", "list"`) {
		t.Error("doctor_repo_state.go runs its own `git worktree list` — that is the second definition the gate forbids")
	}
}

// AC-1.2 — the upstream-less branch is the dangerous one, and it is reported
// as its own class. The test also proves the mechanism the criterion names:
// `git rev-list --count origin/<branch>..HEAD` ERRORS for such a branch, so a
// counter that reads the number and ignores the status skips it in silence.
func TestDoctorRepoState_UpstreamlessBranchReportedDistinctly(t *testing.T) {
	work := repoFixture(t)
	commitsOn(t, work, "main", 2) // ahead of origin/main, but tracking it
	gitAt(t, work, "checkout", "-q", "-b", "local-only")
	gitAt(t, work, "commit", "-q", "--allow-empty", "-m", "unpushed")

	r, text := repoStateText(t, work)

	if len(r.NoUpstream) != 1 || r.NoUpstream[0].Name != "local-only" {
		t.Fatalf("want exactly one upstream-less branch (local-only), got %+v:\n%s", r.NoUpstream, text)
	}
	if len(r.Ahead) != 1 || r.Ahead[0].Name != "main" {
		t.Fatalf("want exactly one merely-ahead branch (main), got %+v:\n%s", r.Ahead, text)
	}
	if r.Ahead[0].Upstream != "origin/main" {
		t.Errorf("main must be reported against its upstream, got %q", r.Ahead[0].Upstream)
	}

	var noUpstreamLine, aheadLine string
	for _, l := range strings.Split(text, "\n") {
		ln := strings.TrimSpace(l)
		if strings.HasPrefix(ln, "NO UPSTREAM") && strings.Contains(ln, "local-only") {
			noUpstreamLine = l
		}
		if strings.HasPrefix(ln, "ahead") && strings.Contains(ln, "main") {
			aheadLine = l
		}
	}
	if noUpstreamLine == "" {
		t.Errorf("local-only is not called out as having no upstream:\n%s", text)
	}
	if aheadLine == "" {
		t.Errorf("main is not reported as ahead of its upstream:\n%s", text)
	}
	if strings.Contains(aheadLine, "NO UPSTREAM") {
		t.Errorf("a merely-ahead branch is reported in the upstream-less class: %q", aheadLine)
	}

	// The mechanism, asserted rather than assumed.
	cmd := exec.Command("git", "rev-list", "--count", "origin/local-only..HEAD")
	cmd.Dir = work
	if out, err := cmd.Output(); err == nil {
		t.Fatalf("origin/local-only..HEAD returned %q instead of erroring — the premise of this check no longer holds",
			strings.TrimSpace(string(out)))
	}
}

// AC-1.3 — the count is DERIVED. A fixture with a known number reports exactly
// that number, and one more commit moves it with no other edit anywhere.
func TestDoctorRepoState_CountIsDerived(t *testing.T) {
	work := repoFixture(t)
	commitsOn(t, work, "main", 3)

	r, text := repoStateText(t, work)
	if len(r.Ahead) != 1 {
		t.Fatalf("want main reported as ahead, got %+v:\n%s", r.Ahead, text)
	}
	if !r.Ahead[0].Measured || r.Ahead[0].Unpushed != 3 {
		t.Fatalf("three unpushed commits must report 3, got %+v:\n%s", r.Ahead[0], text)
	}
	if !strings.Contains(text, "3 commit(s) not in origin/main") {
		t.Errorf("the printed count does not name the derived number:\n%s", text)
	}

	gitAt(t, work, "commit", "-q", "--allow-empty", "-m", "one more")

	r2, text2 := repoStateText(t, work)
	if !r2.Ahead[0].Measured || r2.Ahead[0].Unpushed != 4 {
		t.Fatalf("adding one commit must move the count to 4 with no other edit, got %+v:\n%s", r2.Ahead[0], text2)
	}

	// The upstream-less class derives its number too — commits present in no
	// remote ref at all, which is the only honest comparison available.
	gitAt(t, work, "checkout", "-q", "-b", "detached-work")
	gitAt(t, work, "commit", "-q", "--allow-empty", "-m", "a")
	gitAt(t, work, "commit", "-q", "--allow-empty", "-m", "b")
	r3, text3 := repoStateText(t, work)
	if len(r3.NoUpstream) != 1 || !r3.NoUpstream[0].Measured || r3.NoUpstream[0].Unpushed != 6 {
		t.Fatalf("detached-work carries 6 commits present in no remote ref, got %+v:\n%s", r3.NoUpstream, text3)
	}
}

// AC-1.4 (first half) — read-only and no network. A shim on PATH records every
// git subcommand the check runs, and origin points at an unreachable URL, so a
// fetch would both be recorded and fail.
func TestDoctorRepoState_NoNetwork(t *testing.T) {
	work := repoFixture(t)
	commitsOn(t, work, "main", 2)
	gitAt(t, work, "checkout", "-q", "-b", "local-only")
	gitAt(t, work, "commit", "-q", "--allow-empty", "-m", "unpushed")
	gitAt(t, work, "remote", "set-url", "origin", "https://127.0.0.1:9/unreachable.git")

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "invocations.log")
	shim := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + logPath + "\"\nexec \"" + realGit + "\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(shimDir, "git"), []byte(shim), 0o755); err != nil { //nolint:gosec // test shim must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	r, text := repoStateText(t, work)

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("the shim recorded nothing — the check did not run git through PATH, so this test measured nothing: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(logged)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("no git invocations were recorded — nothing was measured")
	}
	networkVerbs := []string{"fetch", "pull", "push", "clone", "ls-remote", "remote update", "submodule"}
	for _, l := range lines {
		for _, verb := range networkVerbs {
			if strings.Contains(l, verb) {
				t.Errorf("the check ran a network git command: %q (contains %q)", l, verb)
			}
		}
	}

	// It must still have measured something — a check that reached the network
	// zero times because it did nothing at all would pass the loop above.
	if len(r.NoUpstream) != 1 || len(r.Ahead) != 1 || !r.Ahead[0].Measured {
		t.Fatalf("the check produced no measurement with an unreachable origin:\n%s", text)
	}
}

// AC-1.4 (second half) — the output SAYS what the comparison was against. An
// unfetched origin ref is a claim about the last fetch, not about the remote,
// and reporting it as the latter would be a control asserting something it did
// not observe.
func TestDoctorRepoState_StatesComparisonBasis(t *testing.T) {
	work := repoFixture(t)
	commitsOn(t, work, "main", 1)
	_, text := repoStateText(t, work)

	for _, want := range []string{
		"does not fetch",
		"last-fetched",
		"not against the remote",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the report never states its comparison basis (missing %q):\n%s", want, text)
		}
	}
	if strings.Contains(text, "up to date with origin") {
		t.Errorf("the report claims currency with the remote it never contacted:\n%s", text)
	}

	// A branch whose upstream is CONFIGURED but whose tracking ref is not on
	// disk — the remote branch was deleted, which this repository has two live
	// examples of. rev-list errors for it just as it does for an upstream-less
	// branch. Reporting it as level, or as zero ahead, would be a conclusion
	// the check never observed; it must say UNMEASURED and name why.
	gitAt(t, work, "update-ref", "-d", "refs/remotes/origin/main")
	r2, text2 := repoStateText(t, work)
	if len(r2.Unmeasured) != 1 || r2.Unmeasured[0].Name != "main" {
		t.Fatalf("a configured upstream that is not on disk must be reported UNMEASURED, got %+v:\n%s",
			r2.Unmeasured, text2)
	}
	if r2.Unmeasured[0].UpstreamOnDisk {
		t.Errorf("the deleted tracking ref is reported as present")
	}
	if r2.Level != 0 || len(r2.Ahead) != 0 {
		t.Errorf("an unmeasured branch was counted as level or ahead: level=%d ahead=%+v", r2.Level, r2.Ahead)
	}
	if !strings.Contains(text2, "count UNMEASURED, NOT zero") {
		t.Errorf("the report does not say the count is unmeasured rather than zero:\n%s", text2)
	}
}

// AC-1.5 — falsifiable, not merely present. In a scratch repository the check
// reports no upstream-less branch; creating one makes it report that branch,
// by name. A reporter whose output does not change when the state changes is
// indistinguishable from one that never ran.
func TestDoctorRepoState_FalsifiedByScratchBranch(t *testing.T) {
	work := repoFixture(t)

	before, beforeText := repoStateText(t, work)
	if len(before.NoUpstream) != 0 {
		t.Fatalf("the scratch repository has no upstream-less branch; the check invented %+v:\n%s",
			before.NoUpstream, beforeText)
	}
	if !strings.Contains(beforeText, "0 with NO UPSTREAM") {
		t.Fatalf("the before state must positively report zero, not stay silent:\n%s", beforeText)
	}
	if strings.Contains(beforeText, "invisible-work") {
		t.Fatalf("the report names a branch that does not exist yet:\n%s", beforeText)
	}

	gitAt(t, work, "checkout", "-q", "-b", "invisible-work")
	gitAt(t, work, "commit", "-q", "--allow-empty", "-m", "the commit no document mentions")

	after, afterText := repoStateText(t, work)
	if len(after.NoUpstream) != 1 || after.NoUpstream[0].Name != "invisible-work" {
		t.Fatalf("creating an upstream-less branch changed nothing in the report: %+v\n%s",
			after.NoUpstream, afterText)
	}
	if !strings.Contains(afterText, "1 with NO UPSTREAM") || !strings.Contains(afterText, "invisible-work") {
		t.Fatalf("the branch is not named in the output:\n%s", afterText)
	}
	if afterText == beforeText {
		t.Fatal("the report is byte-identical before and after the state changed — it is not reading the repository")
	}
}
