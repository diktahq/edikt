package idvalidate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A CITATION IS AN EXTERNALLY-CONTROLLED VALUE.
//
// The auditor resolves what a governance record cites: artifact ids, commit
// hashes, repository paths. Every one of those comes out of prose a model
// wrote, and each is about to be handed to `git cat-file` or a filesystem
// lookup. Validating at the boundary is what keeps a "citation" from being an
// argv element or a path out of the repository.

func TestCommitHash(t *testing.T) {
	valid := []string{
		"abc1234",
		"abc1234def5678",
		"0123456789abcdef0123456789abcdef01234567", // full 40
		"  abc1234  ", // whitespace stripped
		"ABC1234",     // the same object id, transcribed in uppercase
	}
	for _, s := range valid {
		if err := CommitHash(s); err != nil {
			t.Errorf("CommitHash(%q) = %v, want nil", s, err)
		}
	}

	invalid := []string{
		"",
		"abc123", // 6, below the abbreviation floor
		"0123456789abcdef0123456789abcdef012345678", // 41
		"abc1234; rm -rf /",                         // the reason this function exists
		"abc1234 && echo",
		"$(git rev-parse HEAD)",
		"--upload-pack=evil",
		"abc1234\nabc1234",
		"abcg234", // g is not hex
	}
	for _, s := range invalid {
		if err := CommitHash(s); err == nil {
			t.Errorf("CommitHash(%q) = nil, want an error", s)
		}
	}
}

func TestRepoRelPath(t *testing.T) {
	root := t.TempDir()
	// A real file, and a directory to hold it.
	if err := os.MkdirAll(filepath.Join(root, "docs", "architecture"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "docs", "architecture", "ADR-001.md")
	if err := os.WriteFile(target, []byte("# ADR-001\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The returned path is the SYMLINK-RESOLVED one — that is the point of
	// the confinement check — so the expectation resolves too (a temp dir is
	// itself behind a symlink on macOS).
	wantTarget, terr := filepath.EvalSymlinks(target)
	if terr != nil {
		t.Fatal(terr)
	}
	if got, err := RepoRelPath(root, "docs/architecture/ADR-001.md"); err != nil {
		t.Errorf("a repo-relative path to a real file must resolve: %v", err)
	} else if got != wantTarget {
		t.Errorf("resolved to %q, want %q", got, wantTarget)
	}

	// REFUSALS, not resolutions.
	for _, s := range []string{
		"../../etc/passwd",
		"docs/../../etc/passwd",
		"/etc/passwd",
		"",
		"docs/architecture/../../../etc/passwd",
	} {
		if _, err := RepoRelPath(root, s); err == nil {
			t.Errorf("RepoRelPath(%q) resolved; it must refuse", s)
		}
	}

	// A SYMLINK OUT OF THE REPOSITORY IS A REFUSAL. The path is
	// repo-relative and contains no "..", so a textual check passes it; only
	// resolving the link catches it.
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "docs", "escape.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := RepoRelPath(root, "docs/escape.md"); err == nil {
		t.Error("a symlink resolving outside the repository must be refused, not resolved")
	} else if !strings.Contains(err.Error(), "outside") {
		t.Errorf("the refusal must say the path leaves the repository: %v", err)
	}

	// A path that does not exist is NOT a validation failure: it is an
	// unresolved citation, which the caller reports as a finding. Confusing
	// the two would make a typo look like an attack.
	if _, err := RepoRelPath(root, "docs/architecture/ADR-999.md"); err != nil {
		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("a missing file must be reported as missing, not as invalid: %v", err)
		}
	}
}
