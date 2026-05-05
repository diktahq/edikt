package gov

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitignoreBootstrap_FreshFile(t *testing.T) {
	work := t.TempDir()
	out, err := runGovIn(t, work, "gov", "gitignore-bootstrap")
	if err != nil {
		t.Fatalf("expected exit 0, got err=%v\n%s", err, out)
	}
	body, _ := os.ReadFile(filepath.Join(work, ".gitignore"))
	if string(body) != ".edikt/state/\n" {
		t.Errorf("unexpected .gitignore body: %q", body)
	}
}

func TestGitignoreBootstrap_AlreadyPresent(t *testing.T) {
	work := t.TempDir()
	gp := filepath.Join(work, ".gitignore")
	if err := os.WriteFile(gp, []byte(".edikt/state/\nfoo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runGovIn(t, work, "gov", "gitignore-bootstrap")
	if err != nil {
		t.Fatalf("expected exit 0, got err=%v\n%s", err, out)
	}
	body, _ := os.ReadFile(gp)
	if string(body) != ".edikt/state/\nfoo\n" {
		t.Errorf("file mutated when already present: %q", body)
	}
}

func TestGitignoreBootstrap_INV006_Traversal(t *testing.T) {
	work := t.TempDir()
	out, err := runGovIn(t, work, "gov", "gitignore-bootstrap", "--entry", "../foo")
	if !isExitCode(err, 2) {
		t.Fatalf("expected exit 2 on traversal entry, got err=%v\n%s", err, out)
	}
	if !strings.Contains(out, "must not contain") {
		t.Errorf("missing INV-006 refusal message: %s", out)
	}
}
