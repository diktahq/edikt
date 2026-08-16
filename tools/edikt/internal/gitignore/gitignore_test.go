package gitignore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureEntry_FreshFile(t *testing.T) {
	root := t.TempDir()
	out, err := EnsureEntry(root, ".edikt/state/")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if out != Created {
		t.Fatalf("want Created, got %v", out)
	}
	body, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if string(body) != ".edikt/state/\n" {
		t.Errorf("unexpected body: %q", body)
	}
}

func TestEnsureEntry_AlreadyPresent(t *testing.T) {
	root := t.TempDir()
	gp := filepath.Join(root, ".gitignore")
	original := "# edikt\n.edikt/state/\nnode_modules\n"
	if err := os.WriteFile(gp, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := EnsureEntry(root, ".edikt/state/")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if out != AlreadyPresent {
		t.Fatalf("want AlreadyPresent, got %v", out)
	}
	body, _ := os.ReadFile(gp)
	if string(body) != original {
		t.Errorf("file mutated when entry already present: %q", body)
	}
}

func TestEnsureEntry_TrailingSlashVariant(t *testing.T) {
	root := t.TempDir()
	gp := filepath.Join(root, ".gitignore")
	// File contains the no-trailing-slash form; we ensure the
	// trailing-slash form. Should be detected as already present.
	if err := os.WriteFile(gp, []byte(".edikt/state\nfoo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := EnsureEntry(root, ".edikt/state/")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if out != AlreadyPresent {
		t.Fatalf("want AlreadyPresent (trailing-slash variant), got %v", out)
	}
	// Reverse direction: file has the trailing-slash form, request the
	// stripped form.
	root2 := t.TempDir()
	gp2 := filepath.Join(root2, ".gitignore")
	if err := os.WriteFile(gp2, []byte(".edikt/state/\nfoo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = EnsureEntry(root2, ".edikt/state")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if out != AlreadyPresent {
		t.Fatalf("want AlreadyPresent (reverse variant), got %v", out)
	}
}

func TestEnsureEntry_NoNewlinePrefix(t *testing.T) {
	root := t.TempDir()
	gp := filepath.Join(root, ".gitignore")
	// File ends without a trailing newline — the python script inserts
	// one before the appended entry.
	if err := os.WriteFile(gp, []byte("foo"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := EnsureEntry(root, ".edikt/state/")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if out != Appended {
		t.Fatalf("want Appended, got %v", out)
	}
	body, _ := os.ReadFile(gp)
	if string(body) != "foo\n.edikt/state/\n" {
		t.Errorf("unexpected body: %q", body)
	}
}

func TestEnsureEntry_AppendToExisting(t *testing.T) {
	root := t.TempDir()
	gp := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(gp, []byte("foo\nbar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := EnsureEntry(root, ".edikt/state/")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if out != Appended {
		t.Fatalf("want Appended, got %v", out)
	}
	body, _ := os.ReadFile(gp)
	if !strings.HasSuffix(string(body), ".edikt/state/\n") {
		t.Errorf("entry not appended: %q", body)
	}
}
