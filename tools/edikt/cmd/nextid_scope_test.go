package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q", "."},
		{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func stageProject(t *testing.T, invNames ...string) string {
	t.Helper()
	root := t.TempDir()
	invDir := filepath.Join(root, "docs", "architecture", "invariants")
	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(invDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "edikt_version: \"0.8.0\"\npaths:\n  invariants: docs/architecture/invariants\n"
	if err := os.WriteFile(filepath.Join(root, ".edikt", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, n := range invNames {
		if err := os.WriteFile(filepath.Join(invDir, n+".md"), []byte("# "+n+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// DEGRADATION IS THE BUG. Outside a git repository the allocator cannot see
// other checkouts — it must SAY so, not print the same authoritative-looking
// line it prints after a complete scan.
func TestScanScope_DegradesLoudlyOutsideAGitRepo(t *testing.T) {
	root := stageProject(t, "INV-001-x")
	sc := worktreeRoots(root)
	if !sc.Degraded {
		t.Fatal("a non-repository must degrade")
	}
	d := sc.describe()
	if !strings.Contains(d, "this working tree only") {
		t.Errorf("scope line does not say the scan was partial: %q", d)
	}
	if !strings.Contains(d, "NOT covered") {
		t.Errorf("scope line does not say what it missed: %q", d)
	}
}

// A complete scan must still state its scope. "Existing invariants: ..." with
// no scope statement is what read as authoritative while being partial.
func TestScanScope_CompleteScanStillStatesScope(t *testing.T) {
	root := stageProject(t, "INV-001-x")
	gitInit(t, root)
	sc := worktreeRoots(root)
	if sc.Degraded {
		t.Fatalf("a real repo must not degrade: %s", sc.Reason)
	}
	if !strings.Contains(sc.describe(), "Scanned:") {
		t.Errorf("no scope statement: %q", sc.describe())
	}
}

// An id held in ANOTHER checkout is taken. Both placements must be covered:
// inside the repo directory and outside it. edikt's own worktrees live outside,
// which is why the defect never reproduced here.
func TestNextID_CountsIDsHeldInWorktrees(t *testing.T) {
	for _, tc := range []struct {
		name     string
		external bool
	}{
		{"worktree inside the repo directory", false},
		{"worktree outside the repo directory", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := stageProject(t, "INV-001-x", "INV-002-x")
			gitInit(t, root)

			wtPath := filepath.Join(root, ".claude", "worktrees", "wt")
			if tc.external {
				wtPath = filepath.Join(t.TempDir(), "outside")
			}
			cmd := exec.Command("git", "worktree", "add", "-q", "--detach", wtPath)
			cmd.Dir = root
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("worktree add: %v\n%s", err, out)
			}
			held := filepath.Join(wtPath, "docs", "architecture", "invariants")
			if err := os.MkdirAll(held, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(held, "INV-003-taken.md"), []byte("# INV-003\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			var buf bytes.Buffer
			if err := emitListing(&buf, root, "docs/architecture/invariants", listingINV); err != nil {
				t.Fatal(err)
			}
			got := buf.String()
			if strings.Contains(got, "Next INV number: INV-003") {
				t.Errorf("MINTED A TAKEN ID — INV-003 is held in a worktree:\n%s", got)
			}
			if !strings.Contains(got, "INV-004") {
				t.Errorf("want INV-004:\n%s", got)
			}
			if !strings.Contains(got, "INV-003-taken") {
				t.Errorf("the held id is missing from the Existing list:\n%s", got)
			}
		})
	}
}
