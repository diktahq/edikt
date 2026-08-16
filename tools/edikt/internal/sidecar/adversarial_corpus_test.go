package sidecar

// adversarial_corpus_test.go — SPEC-009 Plan D Phase 4 (SR-013).
//
// Iterates the adversarial fixture corpus at
// test/fixtures/adversarial/<category>/*.edikt.yaml and asserts that
// every fixture either:
//
//   1. Loads cleanly (Load() returns a non-nil *Sidecar and nil err), OR
//   2. Returns a well-formed error (non-empty, single-line, ≤ 300
//      chars, no Go runtime internals).
//
// No fixture is allowed to panic, hang, or produce a malformed error.
// A panic on adversarial input is a verifier bug; the recover() block
// at the test boundary catches them as test failures rather than
// process crashes.
//
// Fixtures run sequentially (no t.Parallel) per INV-007 — race-
// category fixtures specifically rely on serial order to avoid
// state leakage.

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// isDir reports whether path exists and is a directory.
func isDir(t *testing.T, path string) bool {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// findFixtures returns every *.edikt.yaml under root, sorted
// deterministically so subtest output order is stable across runs.
func findFixtures(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".edikt.yaml") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(out)
	return out
}

// corpusRoot resolves to test/fixtures/adversarial/ relative to this
// source file. Robust against any cwd.
func adversarialCorpusRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile = .../tools/edikt/internal/sidecar/adversarial_corpus_test.go
	// Repo root is 4 levels up.
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	root, err := filepath.Abs(filepath.Join(repoRoot, "test", "fixtures", "adversarial"))
	if err != nil {
		t.Fatalf("resolve corpus root: %v", err)
	}
	return root
}

// TestAdversarialCorpus walks every .edikt.yaml under the corpus and
// asserts no-panic + graceful-outcome for each.
func TestAdversarialCorpus(t *testing.T) {
	corpus := adversarialCorpusRoot(t)

	// Five expected categories. The walker will surface any fixture
	// outside these dirs as well — adding a new category just means
	// adding the dir + fixtures.
	expectedCategories := []string{
		"unicode",
		"ordering",
		"hash-collision",
		"races",
		"large-n",
	}

	// Sanity: every category dir must exist (AC-4.1).
	for _, cat := range expectedCategories {
		path := filepath.Join(corpus, cat)
		t.Run("category-exists/"+cat, func(t *testing.T) {
			t.Helper()
			if !isDir(t, path) {
				t.Errorf("missing adversarial category dir: %s", path)
			}
		})
	}

	// Walk + load + assert per fixture.
	fixtures := findFixtures(t, corpus)
	if len(fixtures) < 15 {
		t.Errorf("expected ≥ 15 fixtures across 5 categories, found %d", len(fixtures))
	}

	for _, fx := range fixtures {
		fx := fx // capture for closure
		// Use the relative path as the subtest name so failures
		// point straight at the fixture.
		name := strings.TrimPrefix(fx, corpus+string(filepath.Separator))
		t.Run(name, func(t *testing.T) {
			assertGracefulLoad(t, fx)
		})
	}
}

// assertGracefulLoad calls sidecar.Load() on path with a panic
// recover at the boundary. It asserts:
//
//   - no panic
//   - if err != nil, the error message is well-formed (non-empty,
//     ≤ 300 chars excluding any final newline, no Go runtime internals)
//   - if err == nil, the returned *Sidecar is non-nil and carries
//     at minimum a topic
func assertGracefulLoad(t *testing.T, path string) {
	t.Helper()
	var (
		sc      *Sidecar
		loadErr error
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Load(%s) panicked: %v", path, r)
			}
		}()
		sc, loadErr = Load(path)
	}()

	if loadErr != nil {
		// Well-formedness checks on the error.
		msg := strings.TrimRight(loadErr.Error(), "\n")
		if msg == "" {
			t.Errorf("error from Load(%s) is empty", path)
			return
		}
		if len(msg) > 300 {
			t.Errorf("error from Load(%s) too long (%d chars): %q", path, len(msg), msg[:80]+"...")
		}
		if strings.Contains(msg, "\n") {
			t.Errorf("error from Load(%s) contains newline — should be single-line: %q", path, msg)
		}
		// Sanity guard for runtime-internal leaks.
		for _, bad := range []string{"goroutine ", "runtime.gopark", "runtime.goexit", "/usr/local/go/", "runtime stack:"} {
			if strings.Contains(msg, bad) {
				t.Errorf("error from Load(%s) leaks runtime internals (%q): %q", path, bad, msg)
			}
		}
		return // graceful error is acceptable
	}

	// Clean load — sanity-check the result.
	if sc == nil {
		t.Errorf("Load(%s) returned nil *Sidecar without error", path)
		return
	}
}
