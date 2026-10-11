package projectroot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFind_WalksParents(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".edikt", "config.yaml"), []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := Find(sub)
	if !ok || got != root {
		t.Fatalf("Find(%s) = %q,%v; want %q,true", sub, got, ok, root)
	}
	if _, ok := Find(t.TempDir()); ok {
		t.Fatal("found a project in a bare temp dir")
	}
}

func TestFind_BareEdiktDirIsNotAProject(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, ok := Find(root); ok {
		t.Fatalf("a .edikt/ with no config.yaml was accepted as a project: %q", got)
	}
	// A bare .edikt/ in a child does not shadow the real project above it.
	if err := os.WriteFile(filepath.Join(root, ".edikt", "config.yaml"), []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "child")
	if err := os.MkdirAll(filepath.Join(child, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, ok := Find(child); !ok || got != root {
		t.Fatalf("Find(%s) = %q,%v; want the parent project %q", child, got, ok, root)
	}
}

func TestFindFromCwd(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".edikt", "config.yaml"), []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	got, found, err := FindFromCwd()
	if err != nil || !found {
		t.Fatalf("FindFromCwd from %s: found=%v err=%v", sub, found, err)
	}
	// Compare resolved paths: macOS reports temp dirs through /private.
	want, _ := filepath.EvalSymlinks(root)
	gotR, _ := filepath.EvalSymlinks(got)
	if gotR != want {
		t.Fatalf("FindFromCwd = %q, want %q", gotR, want)
	}
	t.Chdir(t.TempDir())
	if _, found, err := FindFromCwd(); err != nil || found {
		t.Fatalf("FindFromCwd in a bare dir: found=%v err=%v", found, err)
	}
}
