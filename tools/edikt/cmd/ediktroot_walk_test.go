package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkExec writes an executable stub at path, creating parents.
func mkExec(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// tmpRoot returns t.TempDir() with symlinks resolved. On macOS /var is a
// symlink to /private/var, and os.Getwd() returns the resolved path — so an
// expectation built from the unresolved path fails while the code is correct.
// (That is exactly how this test first failed: the oracle was wrong, not the
// bound.)
func tmpRoot(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// The escape, as a permanent regression test. The walk used to stop only at
// $HOME, which does nothing when cwd lives outside a sandboxed HOME: it
// climbed the real filesystem and returned whatever .edikt/bin/edikt it found
// — in the incident, the developer's own install. No environment pinning
// could close it, because the input is the working directory.
func TestWalk_StopsAtProjectBoundary_DoesNotEscapeToAncestorInstall(t *testing.T) {
	root := tmpRoot(t)

	// An "outside" install standing in for ~/.edikt — the thing the walk must
	// no longer reach.
	outside := filepath.Join(root, "outside")
	mkExec(t, filepath.Join(outside, ".edikt", "bin", "edikt"))

	// A project beneath it, with a git marker and NO install of its own.
	project := filepath.Join(outside, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(project, "src", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	sandboxHome := filepath.Join(root, "sandbox-home")
	if err := os.MkdirAll(sandboxHome, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", sandboxHome)
	t.Setenv("EDIKT_ROOT", "")
	t.Setenv("EDIKT_HOME", "")
	chdir(t, sub)

	got, err := resolveEdiktRoot()
	if err != nil {
		t.Fatalf("resolveEdiktRoot: %v", err)
	}
	if strings.HasPrefix(got, outside+string(os.PathSeparator)) && !strings.HasPrefix(got, project) {
		t.Fatalf("ESCAPED: resolved %q — the walk climbed past the project boundary to an ancestor install", got)
	}
	// With no project-local install, resolution must fall through to rung 4.
	want := filepath.Join(sandboxHome, ".edikt")
	if got != want {
		t.Fatalf("resolved %q, want the rung-4 global fallback %q", got, want)
	}
}

// The walk's actual job, preserved: find THIS project's install from a
// subdirectory of it.
func TestWalk_FindsProjectLocalInstallFromSubdirectory(t *testing.T) {
	root := tmpRoot(t)
	project := filepath.Join(root, "proj")
	mkExec(t, filepath.Join(project, ".edikt", "bin", "edikt"))
	sub := filepath.Join(project, "src", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("EDIKT_ROOT", "")
	t.Setenv("EDIKT_HOME", "")
	chdir(t, sub)

	got, err := resolveEdiktRoot()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(project, ".edikt"); got != want {
		t.Fatalf("resolved %q, want the project-local install %q", got, want)
	}
}

// The workspace shape, pinning BOTH halves of the ruling: the walk must not
// find a workspace-level install above a child repo, and EDIKT_ROOT must.
//
// This exists so a future "helpful" relaxation of the bound goes red. Serving
// the workspace case through the walk requires exactly the motion that
// produced the escape — climbing past a git root.
func TestWalk_WorkspaceInstallNotFoundImplicitly_ButEdiktRootServesIt(t *testing.T) {
	root := tmpRoot(t)
	workspace := filepath.Join(root, "workspace")
	mkExec(t, filepath.Join(workspace, ".edikt", "bin", "edikt"))

	child := filepath.Join(workspace, "child-repo")
	if err := os.MkdirAll(filepath.Join(child, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sandboxHome := filepath.Join(root, "home")
	if err := os.MkdirAll(sandboxHome, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, child)

	t.Run("walk does NOT climb past the child repo's boundary", func(t *testing.T) {
		t.Setenv("HOME", sandboxHome)
		t.Setenv("EDIKT_ROOT", "")
		t.Setenv("EDIKT_HOME", "")
		got, err := resolveEdiktRoot()
		if err != nil {
			t.Fatal(err)
		}
		if got == filepath.Join(workspace, ".edikt") {
			t.Fatalf("walk found the workspace install %q implicitly — the bound was relaxed", got)
		}
	})

	t.Run("EDIKT_ROOT serves the workspace configuration explicitly", func(t *testing.T) {
		want := filepath.Join(workspace, ".edikt")
		t.Setenv("HOME", sandboxHome)
		t.Setenv("EDIKT_ROOT", want)
		got, err := resolveEdiktRoot()
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("EDIKT_ROOT=%q resolved to %q — the documented replacement does not work", want, got)
		}
	})
}

// A project whose own root carries the install is still found AT that root —
// the boundary check must run after the marker test, not before.
func TestWalk_BoundaryDoesNotHideAnInstallAtTheBoundaryItself(t *testing.T) {
	root := tmpRoot(t)
	project := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	mkExec(t, filepath.Join(project, ".edikt", "bin", "edikt"))
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("EDIKT_ROOT", "")
	t.Setenv("EDIKT_HOME", "")
	chdir(t, project)

	got, err := resolveEdiktRoot()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(project, ".edikt"); got != want {
		t.Fatalf("resolved %q, want %q — the boundary check hid an install at the boundary", got, want)
	}
}
