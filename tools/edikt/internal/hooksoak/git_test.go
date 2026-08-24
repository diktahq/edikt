package hooksoak

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestGitCommitDate_RealCommit_NotAStateFile is the one test in this
// package that deliberately does NOT stub git — it is the proof that
// elapsed_days genuinely comes from git history, satisfying PLAN-v080's
// criterion "elapsed days computed from a real git commit date, not a state
// file". It creates its own throwaway repository (never touching
// edikt-dev's own history) with two commits a known number of days apart,
// then asserts GitCommitDate reproduces the SAME date an independent `git
// show` call reports for the same commit — proving this function reads real
// git state rather than any bookkeeping of its own.
func TestGitCommitDate_RealCommit_NotAStateFile(t *testing.T) {
	repo := newFixtureRepo(t)
	commitDate := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	hash := repo.commitAt(t, "migrate group 1", commitDate)

	got, err := GitCommitDate(repo.dir, hash)
	if err != nil {
		t.Fatalf("GitCommitDate: %v", err)
	}
	if !got.Equal(commitDate) {
		t.Fatalf("commit date = %s, want %s", got, commitDate)
	}

	// Cross-check against an INDEPENDENT git invocation this test built
	// itself, not against GitCommitDate's own output — a test that only
	// compares a function to itself cannot prove it read anything real.
	indep := exec.Command("git", "show", "-s", "--format=%cI", hash)
	indep.Dir = repo.dir
	out, err := indep.Output()
	if err != nil {
		t.Fatalf("independent git show: %v", err)
	}
	indepDate, err := time.Parse(time.RFC3339, strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parse independent date: %v", err)
	}
	if !got.Equal(indepDate) {
		t.Fatalf("GitCommitDate disagrees with an independent git invocation: %s vs %s", got, indepDate)
	}
}

// Negative control: an unresolvable commit must error, not silently return
// the zero time (which would compute a multi-decade elapsed_days instead of
// naming the real problem).
func TestGitCommitDate_UnknownCommit_Errors(t *testing.T) {
	repo := newFixtureRepo(t)
	repo.commitAt(t, "seed", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if _, err := GitCommitDate(repo.dir, "0000000deadbeef"); err == nil {
		t.Fatal("an unknown commit hash must error")
	}
}

func TestGitCommitDate_NotARepo_Errors(t *testing.T) {
	if _, err := GitCommitDate(t.TempDir(), "abc1234"); err == nil {
		t.Fatal("running against a directory with no git history must error")
	}
}

func TestElapsedDays(t *testing.T) {
	commit := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		now  time.Time
		want int
	}{
		{"same instant", commit, 0},
		{"14 days later", commit.Add(14 * 24 * time.Hour), 14},
		{"partial day rounds down", commit.Add(14*24*time.Hour + 23*time.Hour), 14},
		{"before the commit clamps to zero", commit.Add(-24 * time.Hour), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ElapsedDays(commit, c.now); got != c.want {
				t.Fatalf("ElapsedDays = %d, want %d", got, c.want)
			}
		})
	}
}

// ── fixture repo helper ────────────────────────────────────────────────

type fixtureRepo struct{ dir string }

// newFixtureRepo creates a throwaway git repository under t.TempDir() —
// hermetic and self-contained, never touching this project's own history.
func newFixtureRepo(t *testing.T) fixtureRepo {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "hooksoak-test@example.invalid")
	run("config", "user.name", "hooksoak test")
	return fixtureRepo{dir: dir}
}

// commitAt creates an empty commit dated at `when` (both author and
// committer date), and returns its hash. Fixed GIT_*_DATE env vars are how
// a test controls a commit's date deterministically — without them, `git
// commit` stamps the real wall clock and this test could not assert a known
// elapsed-days figure.
func (r fixtureRepo) commitAt(t *testing.T, msg string, when time.Time) string {
	t.Helper()
	iso := when.Format(time.RFC3339)
	cmd := exec.Command("git", "commit", "-q", "--allow-empty", "-m", msg)
	cmd.Dir = r.dir
	cmd.Env = append(cmd.Environ(),
		"GIT_AUTHOR_DATE="+iso,
		"GIT_COMMITTER_DATE="+iso,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	hashCmd := exec.Command("git", "rev-parse", "HEAD")
	hashCmd.Dir = r.dir
	out, err := hashCmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	return strings.TrimSpace(string(out))
}
