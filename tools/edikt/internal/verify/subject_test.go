package verify

import (
	"os"
	"path/filepath"
	"testing"
)

// SPEC-018 SR-004. The three cases were proven to report PASS before the fix.
func TestUnreadableSubjects_CatchesTheNegatedVerifyHole(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "x.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const cmd = "! rg -l 'FORBIDDEN' src/*.go"

	// PIN THE SUBJECT FIRST: with src/ present the command must be allowed to
	// run, or this test would pass by refusing everything.
	if got := unreadableSubjects(root, cmd); got != "" {
		t.Fatalf("a readable subject must not be reported unmeasured, got %q", got)
	}

	if err := os.RemoveAll(filepath.Join(root, "src")); err != nil {
		t.Fatal(err)
	}
	if got := unreadableSubjects(root, cmd); got == "" {
		t.Fatal("rm -rf src: the subject is gone and the command would exit 0 — this must be unmeasured")
	}
}

// The conservative half. A false unmeasured is a gate that cries wolf, and this
// runs over every verify in the corpus.
func TestUnreadableSubjects_LeavesNonPathsAlone(t *testing.T) {
	root := t.TempDir()
	for _, cmd := range []string{
		"go test ./...",
		// NOTE: "go vet ./internal/verify/" is deliberately NOT here. That token
		// IS a repo path, and holding it unmeasured when the directory is
		// missing is the correct behaviour — the case below asserts it works
		// when the directory exists.
		"test -f go.mod",
		"rg -n 'pattern' --no-heading",
		"bash -c 'true'",
		"echo https://example.com/x.md",
		"grep -c foo $SOME_VAR/bar.txt",
	} {
		if got := unreadableSubjects(root, cmd); got != "" {
			t.Errorf("command %q was wrongly held unmeasured on token %q", cmd, got)
		}
	}
}

// A real relative directory that EXISTS must run, so the conservative half is
// not just refusing everything with a slash in it.
func TestUnreadableSubjects_ExistingRelativeDirRuns(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "verify"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := unreadableSubjects(root, "go vet ./internal/verify/"); got != "" {
		t.Fatalf("an existing directory must not be held unmeasured, got %q", got)
	}
}

// A glob names a set, so its DIRECTORY is the subject — src/*.go asserts
// nothing about which files exist, but does assume src/ does.
func TestUnreadableSubjects_GlobChecksItsDirectory(t *testing.T) {
	root := t.TempDir()
	if got := unreadableSubjects(root, "! rg -l 'X' nope/*.go"); got == "" {
		t.Fatal("a glob under a missing directory must be unmeasured")
	}
	if err := os.MkdirAll(filepath.Join(root, "nope"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := unreadableSubjects(root, "! rg -l 'X' nope/*.go"); got != "" {
		t.Fatalf("an empty but PRESENT directory is a measured zero, not unmeasured; got %q", got)
	}
}

// The gotest helper's package arguments are relative to tools/edikt, where it
// runs `go test`; a pre-flight that resolved them against the repo root called
// every gotest-backed criterion unmeasured (Phase 0 of PLAN-v090-release, all
// eight of them, on 2026-09-02).
func TestUnreadableSubjects_GotestPackagesResolveUnderToolsEdikt(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tools", "edikt", "internal", "hook"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "test", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "test", "lib", "gotest.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := unreadableSubjects(root, "bash test/lib/gotest.sh './internal/hook/' 'TestX'"); got != "" {
		t.Fatalf("gotest package under tools/edikt reported unreadable: %q", got)
	}
	// The contract cuts both ways: a package that does not exist there is
	// still unreadable, not silently accepted because the helper was named.
	if got := unreadableSubjects(root, "bash test/lib/gotest.sh './internal/nope/' 'TestX'"); got == "" {
		t.Fatal("a missing package under tools/edikt must still be reported")
	}
	// A later token is back at the repo root: a second helper chained with
	// && is read from where it actually lives, not under tools/edikt.
	if err := os.WriteFile(filepath.Join(root, "test", "lib", "absent.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := unreadableSubjects(root, "bash test/lib/gotest.sh './internal/hook/' 'TestX' && bash test/lib/absent.sh retired.go"); got != "" {
		t.Fatalf("a repo-root script after the gotest call reported unreadable: %q", got)
	}
}

// A criterion of the shape `cd tools/edikt && go test ./internal/foo/` names
// packages relative to the directory it cds into. Resolving them against the
// project root reports a subject that exists as unreadable, and ten of
// seventeen criteria in a sibling plan read that way on their first run —
// with one passing because its `...` suffix parsed differently, so the run
// looked like "one passed, ten skipped, probably fine".
func TestUnreadableSubjects_HonoursALeadingCd(t *testing.T) {
	root := t.TempDir()
	// tools/edikt/internal/foo exists; ./internal/foo does not exist at root.
	if err := os.MkdirAll(filepath.Join(root, "tools", "edikt", "internal", "foo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := unreadableSubjects(root, "cd tools/edikt && go test ./internal/foo/"); got != "" {
		t.Fatalf("a package under the cd'd directory must resolve, got unreadable: %q", got)
	}
	// And a genuinely absent package still reports.
	if got := unreadableSubjects(root, "cd tools/edikt && go test ./internal/nope/"); got == "" {
		t.Fatal("a package that does not exist under the cd'd directory must be reported")
	}
	// The cd governs EVERYTHING after it, the way the shell does: a script
	// named later resolves under the cd'd directory, not at the repo root.
	if err := os.WriteFile(filepath.Join(root, "tools", "edikt", "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := unreadableSubjects(root, "cd tools/edikt && go test ./internal/foo/ && bash ./run.sh"); got != "" {
		t.Fatalf("a path after the cd resolves under the cd'd directory, got %q", got)
	}
	// The gotest.sh wrapper is the OTHER case: it interprets its own `./pkg/`
	// arguments under tools/edikt while the shell stays at the project root,
	// so a script named after it is at the root.
	if err := os.WriteFile(filepath.Join(root, "at-root.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "test", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "test", "lib", "gotest.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := unreadableSubjects(root, "bash test/lib/gotest.sh './internal/foo/' 'TestX' && bash at-root.sh"); got != "" {
		t.Fatalf("the helper's args resolve under tools/edikt and a later script at the root, got %q", got)
	}
}
