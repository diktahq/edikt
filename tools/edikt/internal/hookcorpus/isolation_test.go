package hookcorpus

// isolation_test.go — SSP-002, asserted rather than asserted-in-a-comment.
//
// The rule has three parts and each has a different failure mode:
//
//	no import edge either way   — an edge would put this package in `hook run`'s
//	                              call graph, which is the thing the boundary
//	                              exists to keep it out of
//	no func init() here         — init runs on EVERY hook fire, because both
//	                              packages link into ONE binary
//	no package-level state here  — same reason: a package-level regexp compiles
//	                              at process start, and the process starts on
//	                              every PreToolUse in every session
//
// The source is parsed rather than grepped, so a comment mentioning init() or a
// string containing an import path cannot make this pass or fail by accident.
// The test itself imports neither package — reading files is not an edge.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageIsolation(t *testing.T) {
	hookDir, err := filepath.Abs("../hook")
	if err != nil {
		t.Fatal(err)
	}
	corpusDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}

	corpusFiles := goFiles(t, corpusDir, false)
	hookFiles := goFiles(t, hookDir, true)
	if len(corpusFiles) == 0 || len(hookFiles) == 0 {
		t.Fatalf("parsed %d hookcorpus and %d hook files — this assertion examined nothing "+
			"(INV-013)", len(corpusFiles), len(hookFiles))
	}
	t.Logf("examined %d hookcorpus file(s) and %d internal/hook file(s)", len(corpusFiles), len(hookFiles))

	t.Run("no import edge in either direction", func(t *testing.T) {
		for path, f := range corpusFiles {
			for _, imp := range f.Imports {
				if strings.Contains(imp.Path.Value, "internal/hook\"") ||
					strings.Contains(imp.Path.Value, "internal/hook/") {
					t.Errorf("%s imports %s — SSP-002 forbids it: an edge puts hookcorpus into "+
						"the call graph of the binary that runs on every hook fire",
						filepath.Base(path), imp.Path.Value)
				}
			}
		}
		for path, f := range hookFiles {
			for _, imp := range f.Imports {
				if strings.Contains(imp.Path.Value, "internal/hookcorpus") {
					t.Errorf("%s imports %s — SSP-002 forbids it", filepath.Base(path), imp.Path.Value)
				}
			}
		}
	})

	t.Run("hookcorpus declares no func init", func(t *testing.T) {
		for path, f := range corpusFiles {
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv != nil {
					continue
				}
				if fn.Name.Name == "init" {
					t.Errorf("%s declares func init() — it would run on every hook invocation, "+
						"for tooling that only ever runs offline", filepath.Base(path))
				}
			}
		}
	})

	t.Run("hookcorpus declares no package-level variable", func(t *testing.T) {
		// const is fine: an untyped constant costs nothing at process start.
		// var is not, and the specific one to watch for is a package-level
		// regexp.MustCompile — the ordinary Go idiom, and here a per-hook-fire
		// cost paid by code that never runs during a hook fire.
		for path, f := range corpusFiles {
			for _, decl := range f.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.VAR {
					continue
				}
				for _, spec := range gd.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, name := range vs.Names {
						t.Errorf("%s declares package-level var %s — SSP-002 allows no "+
							"non-trivial package state here; move it into a constructor",
							filepath.Base(path), name.Name)
					}
				}
			}
		}
	})
}

func goFiles(t *testing.T, dir string, recurse bool) map[string]*ast.File {
	t.Helper()
	out := map[string]*ast.File{}
	fset := token.NewFileSet()
	walk := func(path string) {
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly|parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		// Re-parse in full for the declaration checks; ImportsOnly stops at the
		// import block and would report every file as declaring nothing.
		full, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		full.Imports = f.Imports
		out[path] = full
	}
	if recurse {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			walk(path)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
		return out
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		walk(filepath.Join(dir, e.Name()))
	}
	return out
}
