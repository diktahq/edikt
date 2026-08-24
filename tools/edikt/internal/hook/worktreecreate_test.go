package hook

// worktreecreate_test.go — direct coverage for two worktree-create.sh
// branches test/fixtures/hook-corpus/worktree-create/matrix.yaml cannot
// reach: the corpus sandbox has no per-scenario mechanism to seed an
// arbitrary symlink into the project tree (only payload/config/env are
// scenario-scoped), and the shared _base tree carries none — see that
// matrix's own header comment.

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

// TestWorktreeCreateRejectsSymlinkedPath proves the `[ -L "$WORKTREE_PATH" ]`
// guard: a REAL symlink at the resolved path must produce exit 1 and zero
// stdout, never the path echo a real directory or a nonexistent path would
// get.
func TestWorktreeCreateRejectsSymlinkedPath(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	target := filepath.Join(cwd, "real-dir")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	link := filepath.Join(cwd, "wt-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	inv := &Invocation{Stderr: io.Discard, Payload: adapter.Payload{WorktreePath: link}}
	res := worktreeCreate(inv)

	if res.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", res.ExitCode)
	}
	if !res.Response.IsZero() {
		t.Errorf("response = %+v, want zero (no stdout) on the symlink-reject path", res.Response)
	}

	// Negative control: the SAME target, referenced by its real (non-symlink)
	// path, must echo normally — proving the rejection is about the symlink,
	// not about the target directory itself.
	inv2 := &Invocation{Stderr: io.Discard, Payload: adapter.Payload{WorktreePath: target}}
	res2 := worktreeCreate(inv2)
	if res2.ExitCode != 0 {
		t.Errorf("real-dir exit code = %d, want 0", res2.ExitCode)
	}
	if res2.Response.Text != target {
		t.Errorf("real-dir response.Text = %q, want %q", res2.Response.Text, target)
	}
}

// TestWorktreeCreateRejectsMetacharacterPath proves
// `if path and all(c not in path for c in "\n\r\t"): print(path)` — a
// worktree path containing a newline, carriage return, or tab is silently
// dropped rather than echoed, matching the security-motivated rejection in
// worktree-create.sh's own python extraction.
func TestWorktreeCreateRejectsMetacharacterPath(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	cases := []string{
		"worktrees/feat\nure",
		"worktrees/feat\rure",
		"worktrees/feat\ture",
	}
	for _, path := range cases {
		inv := &Invocation{Stderr: io.Discard, Payload: adapter.Payload{WorktreePath: path}}
		res := worktreeCreate(inv)
		if !res.Response.IsZero() {
			t.Errorf("path %q: response = %+v, want zero (rejected)", path, res.Response)
		}
	}

	// Negative control: the SAME family of path, without the embedded
	// metacharacter, must echo normally.
	inv := &Invocation{Stderr: io.Discard, Payload: adapter.Payload{WorktreePath: "worktrees/feature"}}
	res := worktreeCreate(inv)
	if res.Response.Text != "worktrees/feature" {
		t.Errorf("clean path: response.Text = %q, want \"worktrees/feature\"", res.Response.Text)
	}
}
