package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A diagnostic asserts the OUTCOME a hooks path produces, not the presence of
// a setting (FR-033, INV-014). A `core.hooksPath` pointing at a directory
// that does not exist disables every git hook silently — bok had never run a
// git hook in its history, and this repo sets the same key.

func gitRepo(t *testing.T, hooksPath string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git absent: %v", err)
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = root
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	if hooksPath != "" {
		run("config", "core.hooksPath", hooksPath)
	}
	return root
}

func TestDoctorHooksPath_MissingDirectoryIsRed(t *testing.T) {
	root := gitRepo(t, ".githooks")
	var out bytes.Buffer
	errN, _ := runGitHooksPathCheck(root, &out)
	if errN == 0 {
		t.Fatalf("a hooks path that does not exist must be RED:\n%s", out.String())
	}
	s := out.String()
	for _, want := range []string{".githooks", "ZERO", "🔴"} {
		if !strings.Contains(s, want) {
			t.Errorf("the finding must name %q:\n%s", want, s)
		}
	}
}

// `edikt init` installs pre-push into .git/hooks/. Configuring core.hooksPath
// makes git ignore that directory entirely, so edikt's own hook stops running
// and nothing says so — bok's repository, exactly.
func TestDoctorHooksPath_EdiktsOwnHookShadowedByTheConfiguredPathIsRed(t *testing.T) {
	root := gitRepo(t, ".githooks")
	if err := os.MkdirAll(filepath.Join(root, ".githooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "hooks", "pre-push"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	errN, _ := runGitHooksPathCheck(root, &out)
	if errN == 0 {
		t.Fatalf("edikt's hook shadowed by core.hooksPath must be RED:\n%s", out.String())
	}
	s := out.String()
	for _, want := range []string{"pre-push", ".githooks", "never runs"} {
		if !strings.Contains(s, want) {
			t.Errorf("the finding must name %q:\n%s", want, s)
		}
	}
}

func TestDoctorHooksPath_NonExecutableHookIsReported(t *testing.T) {
	root := gitRepo(t, ".githooks")
	d := filepath.Join(root, ".githooks")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "pre-commit"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	errN, warnN := runGitHooksPathCheck(root, &out)
	if errN+warnN == 0 {
		t.Fatalf("a hook that is present but not executable never runs; it must be reported:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "executable") || !strings.Contains(out.String(), "pre-commit") {
		t.Errorf("the finding must say the hook is not executable:\n%s", out.String())
	}
}

// A message check placed in `pre-commit` validates the PREVIOUS commit's
// message: it cannot fail on the commit being made. Only a hook that
// RECEIVES the message path as $1 — commit-msg — can (INV-014).
func TestDoctorHooksPath_MessageCheckInPreCommitIsACantFailControl(t *testing.T) {
	root := gitRepo(t, ".githooks")
	d := filepath.Join(root, ".githooks")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	// The shape bok shipped: the message validation lives in pre-commit.
	if err := os.WriteFile(filepath.Join(d, "pre-commit"),
		[]byte("#!/bin/sh\ngit log -1 --format=%B | grep -qE '^(feat|fix)' || exit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "commit-msg"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	errN, warnN := runGitHooksPathCheck(root, &out)
	if errN+warnN == 0 {
		t.Fatalf("a message check in pre-commit is a can't-fail control and must be reported:\n%s", out.String())
	}
	s := out.String()
	if !strings.Contains(s, "pre-commit") || !strings.Contains(s, "$1") {
		t.Errorf("the finding must name the hook and the argument that decides it:\n%s", s)
	}
}

// The remedy is printed only after its precondition is established (FR-032):
// a healthy repo gets no remedy, and a repo with no hooks path configured is
// not a finding at all.
func TestDoctorHooksPath_HealthyAndUnconfiguredAreSilent(t *testing.T) {
	for _, c := range []struct{ name, hooksPath string }{
		{"no core.hooksPath set", ""},
		{"hooks path with both hooks executable", ".githooks"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := gitRepo(t, c.hooksPath)
			if c.hooksPath != "" {
				d := filepath.Join(root, c.hooksPath)
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(d, "pre-commit"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer
			errN, warnN := runGitHooksPathCheck(root, &out)
			if errN != 0 || warnN != 0 {
				t.Fatalf("must be silent, got %d/%d:\n%s", errN, warnN, out.String())
			}
			if strings.Contains(out.String(), "Run:") {
				t.Errorf("a remedy was printed with no finding to remedy:\n%s", out.String())
			}
		})
	}
}

// Not a git repository at all: nothing to measure, and saying nothing is
// correct — but it must not be counted as a pass either.
func TestDoctorHooksPath_NonRepoIsSilent(t *testing.T) {
	var out bytes.Buffer
	errN, warnN := runGitHooksPathCheck(t.TempDir(), &out)
	if errN != 0 || warnN != 0 {
		t.Fatalf("a non-repo must produce no finding, got %d/%d:\n%s", errN, warnN, out.String())
	}
}
