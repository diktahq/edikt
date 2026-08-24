package hook

// sessionstart_test.go — direct coverage for behaviour that
// test/fixtures/hook-corpus/session-start/matrix.yaml cannot reach: the
// git-log-driven changed-files scan and its migration/docker domain-signal
// classification (MEMORY.md's mtime is set by git checkout, not a fixture
// author — see that matrix's own header comment), and the session-summary
// disable flag's actually-distinguishable branch (the corpus's own
// session-summary-off scenario enters the config-gated code path but, in a
// sandbox with no MEMORY.md, produces byte-identical output to baseline — it
// proves the branch is TAKEN, not that it CHANGES anything).
//
// This file is the port target for four assertions retired from
// test/test-hooks.sh's content-grep era (PLAN-v080-reliability-core Phase
// 12): "SessionStart hook is git-aware (contains 'git log')", "... contains
// 'migration' domain signal", "... contains 'docker' domain signal", and
// "... respects session-summary disable flag" — all four grepped
// templates/hooks/session-start.sh, which is now a 10-line exec shim with
// none of that text in it. The behaviour moved to sessionstart.go; this file
// is where its coverage now lives.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestSessionStartClassifyDomainSignals is a pure table-driven test of
// sessionStartClassify — the four grep -ciE domain-signal counts and the
// SUMMARY/AGENTS strings built from them. No filesystem, no git: this is the
// classification logic in isolation.
func TestSessionStartClassifyDomainSignals(t *testing.T) {
	cases := []struct {
		name    string
		changed []string
		wantAgt string
		wantSub string // substring that must appear in the summary
	}{
		{
			name:    "migration file trips the dba signal",
			changed: []string{"db/migrations/0007_add_users.sql"},
			wantAgt: "dba",
			wantSub: "migration/schema file(s)",
		},
		{
			name:    "docker file trips the sre signal",
			changed: []string{"docker-compose.yml"},
			wantAgt: "sre",
			wantSub: "infra file(s)",
		},
		{
			name:    "Dockerfile (capitalized) still trips the sre signal",
			changed: []string{"Dockerfile"},
			wantAgt: "sre",
			wantSub: "infra file(s)",
		},
		{
			name:    "mixed migration and docker changes trip both, dba first",
			changed: []string{"db/migration_001.sql", "docker-compose.yml", "README.md"},
			wantAgt: "dba, sre",
			wantSub: "migration/schema file(s), 1 infra file(s)",
		},
		{
			name:    "no domain signal in the changed set yields no agents",
			changed: []string{"README.md", "notes.txt"},
			wantAgt: "",
			wantSub: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			summary, agents := sessionStartClassify(c.changed)
			if agents != c.wantAgt {
				t.Errorf("agents = %q, want %q (summary=%q)", agents, c.wantAgt, summary)
			}
			if c.wantSub != "" && !containsSubstring(summary, c.wantSub) {
				t.Errorf("summary = %q, want it to contain %q", summary, c.wantSub)
			}
		})
	}
}

func containsSubstring(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestSessionStartChangedFilesIsGitAware proves sessionStartChangedFiles
// actually shells out to `git log --since=<mtime>` and returns the files
// touched since — not just that the string "git log" appears somewhere. A
// real temp repo stands in for MEMORY.md's git-checkout-derived mtime: the
// memory file's mtime is pinned to a fixed past instant (not "now minus N",
// which a slow CI runner could race against the commit clock), then real
// commits are made afterward.
func TestSessionStartChangedFilesIsGitAware(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	dir := t.TempDir()
	runGit(t, dir, "", "init", "-q")
	runGit(t, dir, "", "config", "user.email", "test@edikt.local")
	runGit(t, dir, "", "config", "user.name", "edikt test")

	// Every commit gets an EXPLICIT date rather than relying on wall-clock
	// timing (fragile under -race or a loaded CI runner): t0 is the initial
	// commit, the memory boundary sits 5s after it, and the migration/docker
	// commits land 10s after it — comfortably on either side of the
	// boundary regardless of how long the test itself takes to run.
	t0 := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	writeTestFile(t, filepath.Join(dir, "README.md"), "# hi\n")
	runGit(t, dir, "", "add", "README.md")
	runGit(t, dir, t0.Format(time.RFC3339), "commit", "-q", "-m", "initial")

	// Memory file's mtime is the "--since" boundary: everything committed
	// AFTER it should be picked up, everything at or before it (the initial
	// commit) should not.
	memory := filepath.Join(dir, "MEMORY.md")
	writeTestFile(t, memory, "# memory\n")
	boundary := t0.Add(5 * time.Second)
	if err := os.Chtimes(memory, boundary, boundary); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	after := t0.Add(10 * time.Second)
	writeTestFile(t, filepath.Join(dir, "db", "migration_001.sql"), "ALTER TABLE x;\n")
	runGit(t, dir, "", "add", "db/migration_001.sql")
	runGit(t, dir, after.Format(time.RFC3339), "commit", "-q", "-m", "add migration")

	writeTestFile(t, filepath.Join(dir, "docker-compose.yml"), "services: {}\n")
	runGit(t, dir, "", "add", "docker-compose.yml")
	runGit(t, dir, after.Format(time.RFC3339), "commit", "-q", "-m", "add compose file")

	changed := sessionStartChangedFiles(dir, memory)
	want := map[string]bool{"db/migration_001.sql": true, "docker-compose.yml": true}
	if len(changed) != len(want) {
		t.Fatalf("sessionStartChangedFiles = %v, want exactly %v", changed, want)
	}
	for _, f := range changed {
		if !want[f] {
			t.Errorf("unexpected file in changed set: %q (full: %v)", f, changed)
		}
	}

	// Feed the git-derived list straight into the classifier — this is the
	// exact composition sessionStart() performs, proving the two functions
	// this migration ported (git log invocation, then domain classification)
	// still work together end to end on real git output, not just on a
	// hand-written []string.
	summary, agents := sessionStartClassify(changed)
	if agents != "dba, sre" {
		t.Errorf("agents = %q, want %q for a migration+docker change set", agents, "dba, sre")
	}
	if !containsSubstring(summary, "migration/schema file(s)") || !containsSubstring(summary, "infra file(s)") {
		t.Errorf("summary = %q, want both a migration and an infra count", summary)
	}
}

// TestSessionStartSessionSummaryFlagChangesOutput proves `session-summary:
// false` in .edikt/config.yaml is not merely READ (the corpus's
// session-summary-off scenario already proves that much) but changes what the
// hook actually says. Held constant across both runs: a fresh (age 0)
// MEMORY.md present, and a cwd that is not a git repo (so the changed-files
// scan comes back empty either way) — the ONLY variable is the flag, so any
// difference in output is attributable to it and nothing else.
func TestSessionStartSessionSummaryFlagChangesOutput(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH — session-start's preflight would short-circuit both cases identically")
	}

	run := func(t *testing.T, summaryOff bool) string {
		t.Helper()
		home := t.TempDir()
		cwd := t.TempDir()
		t.Setenv("HOME", home)
		t.Chdir(cwd)

		cfg := "base: docs\n"
		if summaryOff {
			cfg += "session-summary: false\n"
		}
		writeTestFile(t, filepath.Join(cwd, ".edikt", "config.yaml"), cfg)

		memory := testMemoryPath(home, cwd)
		writeTestFile(t, memory, "# memory\n")
		now := time.Now()
		if err := os.Chtimes(memory, now, now); err != nil {
			t.Fatalf("chtimes: %v", err)
		}

		inv := &Invocation{Stderr: io.Discard}
		res := sessionStart(inv)
		return res.Response.Context
	}

	off := run(t, true)
	on := run(t, false)

	wantOff := fmt.Sprintf(sessionStartMsgSummaryOffFmt, 0)
	wantOn := fmt.Sprintf(sessionStartMsgFallbackFmt, 0)

	if off != wantOff {
		t.Errorf("session-summary: false → %q, want %q", off, wantOff)
	}
	if on != wantOn {
		t.Errorf("session-summary unset → %q, want %q", on, wantOn)
	}
	if off == on {
		t.Fatalf("flag produced no observable difference — both runs said %q", off)
	}
}

// runGit runs a git command in dir. When date is non-empty, it pins both the
// author and committer date (RFC3339) so commit timestamps are deterministic
// rather than wall-clock — irrelevant for init/config/add, only meaningful
// for commit.
func runGit(t *testing.T, dir, date string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	env := append(os.Environ(), "GIT_AUTHOR_NAME=edikt test", "GIT_AUTHOR_EMAIL=test@edikt.local",
		"GIT_COMMITTER_NAME=edikt test", "GIT_COMMITTER_EMAIL=test@edikt.local")
	if date != "" {
		env = append(env, "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	}
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// testMemoryPath mirrors adapter.MemoryPath's convention ($HOME/.claude/
// projects/<cwd, '/' -> '-'>/memory/MEMORY.md) without importing the adapter
// package's encoding detail into this test file — same computation
// sessionStart() itself calls via adapter.MemoryPath.
func testMemoryPath(home, cwd string) string {
	encoded := make([]byte, len(cwd))
	for i := 0; i < len(cwd); i++ {
		if cwd[i] == '/' {
			encoded[i] = '-'
		} else {
			encoded[i] = cwd[i]
		}
	}
	return filepath.Join(home, ".claude", "projects", string(encoded), "memory", "MEMORY.md")
}
