package hookmatch

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPathNormalize_CleansTraversalBeforeMatching pins the first half of
// AC-3.4. A literal path can be dressed up — `tools/../tools/edikt/x.go`,
// `./tools/edikt/x.go` — and an unCleaned matcher would miss a glob that
// plainly covers the file. That is a governance bypass available to anyone who
// types a path slightly differently.
func TestPathNormalize_CleansTraversalBeforeMatching(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tools", "edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeIndex(t, root, `
schema_version: 1
globs:
  "tools/**/*.go":
    - id: "INV-901:d01"
      grade: must
      text: "governed"
      reminders: []
`)
	for _, p := range []string{
		"tools/edikt/x.go",
		"./tools/edikt/x.go",
		"tools/../tools/edikt/x.go",
		"tools/edikt/../edikt/x.go",
	} {
		res := Match(root, p)
		if res.Outcome != OutcomeMatched {
			t.Errorf("%q did not match after normalisation (outcome=%s, norm=%q)", p, res.Outcome, res.NormPath)
		}
	}
}

// TestSymlinkResolve_LiteralMissesButTargetMatches is the disagreement case
// the criterion names, and the direction that actually leaks governance.
//
// `docs/link.go` matches no governed glob as a literal string. Its TARGET is
// `tools/edikt/real.go`, which `tools/**/*.go` covers. An agent that writes
// through the link edits governed code, and a matcher comparing literals
// injects nothing and reports nothing wrong.
func TestSymlinkResolve_LiteralMissesButTargetMatches(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tools", "edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(root, "tools", "edikt", "real.go")
	if err := os.WriteFile(real, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "docs", "link.go")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeIndex(t, root, `
schema_version: 1
globs:
  "tools/**/*.go":
    - id: "INV-901:d01"
      grade: must
      text: "governed"
      reminders: []
`)

	// PIN THE PREMISE: the literal path must genuinely NOT match, or the test
	// proves nothing about symlink resolution.
	if got, _ := NormalizePath(root, "docs/link.go"); got == "docs/link.go" {
		t.Fatalf("premise broken: path was not resolved at all (%q)", got)
	}

	res := Match(root, "docs/link.go")
	if res.Outcome != OutcomeMatched {
		t.Fatalf("a symlink whose TARGET is governed did not match: outcome=%s norm=%q — "+
			"an agent can write governed code through a link with nothing injected",
			res.Outcome, res.NormPath)
	}
}

// TestSymlinkResolve_LiteralMatchesButTargetDoesNot is the inverse, and it
// guards against "resolve everything" being mistaken for correctness.
//
// `tools/edikt/alias.go` matches `tools/**/*.go` as a literal. Its target is
// `docs/plain.txt`, which is not governed code. Injecting a MUST-grade Go
// directive for a write to a text file is a false positive, and false
// positives are how a gate gets disabled.
func TestSymlinkResolve_LiteralMatchesButTargetDoesNot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tools", "edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "docs", "plain.txt")
	if err := os.WriteFile(target, []byte("text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "tools", "edikt", "alias.go")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeIndex(t, root, `
schema_version: 1
globs:
  "tools/**/*.go":
    - id: "INV-901:d01"
      grade: must
      text: "governed"
      reminders: []
`)

	res := Match(root, "tools/edikt/alias.go")
	if res.Outcome == OutcomeMatched {
		t.Errorf("a link whose literal path matches but whose TARGET is a text file was "+
			"treated as governed Go code (norm=%q); false positives are how a gate gets disabled",
			res.NormPath)
	}
}

// TestPathNormalize_NonexistentPathIsNotAnError pins the most common
// PreToolUse case. Write CREATES files, so the path usually does not exist
// yet. Treating that as a normalisation failure would classify every new file
// as path_rejected — suppressing governance on exactly the writes that
// introduce new code.
func TestPathNormalize_NonexistentPathIsNotAnError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tools", "edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeIndex(t, root, `
schema_version: 1
globs:
  "tools/**/*.go":
    - id: "INV-901:d01"
      grade: must
      text: "governed"
      reminders: []
`)
	res := Match(root, "tools/edikt/brand-new.go")
	if res.Outcome != OutcomeMatched {
		t.Fatalf("a not-yet-created file was not matched: outcome=%s norm=%q", res.Outcome, res.NormPath)
	}
}

func writeIndex(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".claude", "rules", "governance")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "directive-index.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
