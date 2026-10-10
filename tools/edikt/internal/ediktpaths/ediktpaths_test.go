package ediktpaths

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The defect this package closes (#11 comment item 2): `gov grade-compile`
// looked for its rubric at `<cwd>/.edikt/templates/…` and `<cwd>/templates/…`
// only. The second rung exists in exactly one place — a checkout of edikt
// itself — so on every adopter project the command refused with "none of the
// expected asset paths exist", and from a subdirectory of edikt-dev it
// refused too. The installed payload, which carries the file, was never a
// candidate. ADR-005 §1 names the payload as the fallback.

func realTemp(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(path+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// sandbox pins every rung of ResolveEdiktRoot so nothing reaches the
// developer's real ~/.edikt (INV-007), and returns a project dir with a
// .git boundary so the ancestor walk cannot climb out of it.
func sandbox(t *testing.T) (base, project string) {
	t.Helper()
	base = realTemp(t)
	project = filepath.Join(base, "proj")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(base, "home"))
	t.Setenv("EDIKT_ROOT", "")
	t.Setenv("EDIKT_HOME", "")
	t.Chdir(project)
	return base, project
}

const rel = "rubrics/compile-quality.md"

// Every rung, removed one at a time from the top: the winner is always the
// highest rung still present. This is the ladder ADR-005 §1 specifies
// (override → payload), with the edikt-root templates link and the dev
// checkout as the two later fallbacks.
func TestResolveTemplate_LadderOrder(t *testing.T) {
	base, project := sandbox(t)
	root := filepath.Join(base, "edikt-home")
	t.Setenv("EDIKT_HOME", root)

	override := filepath.Join(project, ".edikt", "templates", rel)
	payload := filepath.Join(root, "current", "templates", rel)
	rootTpl := filepath.Join(root, "templates", rel)
	repo := filepath.Join(project, "templates", rel)
	for _, p := range []string{override, payload, rootTpl, repo} {
		writeFile(t, p)
	}

	for _, want := range []string{override, payload, rootTpl, repo} {
		got, err := ResolveTemplate(project, rel)
		if err != nil {
			t.Fatalf("ResolveTemplate: %v", err)
		}
		if got != want {
			t.Fatalf("resolved %q, want %q", got, want)
		}
		if err := os.Remove(want); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ResolveTemplate(project, rel); err == nil {
		t.Fatal("every rung removed, yet ResolveTemplate reported success")
	}
}

// The adopter case the defect lived in: nothing under the project, the
// payload under $EDIKT_HOME.
func TestResolveTemplate_EdiktHomePayload(t *testing.T) {
	base, project := sandbox(t)
	root := filepath.Join(base, "eh")
	t.Setenv("EDIKT_HOME", root)
	want := filepath.Join(root, "current", "templates", rel)
	writeFile(t, want)

	got, err := ResolveTemplate(project, rel)
	if err != nil || got != want {
		t.Fatalf("got (%q, %v), want %q", got, err, want)
	}
}

// $EDIKT_ROOT is rung 1 of the root chain and beats $EDIKT_HOME.
func TestResolveTemplate_EdiktRootBeatsEdiktHome(t *testing.T) {
	base, project := sandbox(t)
	er := filepath.Join(base, "er")
	eh := filepath.Join(base, "eh")
	t.Setenv("EDIKT_ROOT", er)
	t.Setenv("EDIKT_HOME", eh)
	writeFile(t, filepath.Join(eh, "current", "templates", rel))
	want := filepath.Join(er, "current", "templates", rel)
	writeFile(t, want)

	got, err := ResolveTemplate(project, rel)
	if err != nil || got != want {
		t.Fatalf("got (%q, %v), want %q", got, err, want)
	}
}

// $HOME/.edikt is the global default when no env override is set.
func TestResolveTemplate_HomeDefault(t *testing.T) {
	base, project := sandbox(t)
	want := filepath.Join(base, "home", ".edikt", "current", "templates", rel)
	writeFile(t, want)

	got, err := ResolveTemplate(project, rel)
	if err != nil || got != want {
		t.Fatalf("got (%q, %v), want %q", got, err, want)
	}
}

// Project mode: a project-local install (.edikt/bin/edikt) found from a
// subdirectory is the edikt root, and its payload is used.
func TestResolveTemplate_ProjectModeFromSubdirectory(t *testing.T) {
	base, project := sandbox(t)
	launcher := filepath.Join(project, ".edikt", "bin", "edikt")
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A global install exists too; project mode must win over it.
	t.Setenv("EDIKT_HOME", filepath.Join(base, "global"))
	writeFile(t, filepath.Join(base, "global", "current", "templates", rel))
	want := filepath.Join(project, ".edikt", "current", "templates", rel)
	writeFile(t, want)

	sub := filepath.Join(project, "src", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	got, err := ResolveTemplate(project, rel)
	if err != nil || got != want {
		t.Fatalf("got (%q, %v), want %q", got, err, want)
	}
}

// A miss names every path it tried. "not found" alone cannot be acted on:
// the user cannot tell an uninstalled payload from a wrong $EDIKT_HOME.
func TestResolveTemplate_ErrorNamesEveryCandidate(t *testing.T) {
	base, project := sandbox(t)
	root := filepath.Join(base, "eh")
	t.Setenv("EDIKT_HOME", root)

	_, err := ResolveTemplate(project, rel)
	if err == nil {
		t.Fatal("nothing exists, yet ResolveTemplate reported success")
	}
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("error is %T, want *NotFoundError", err)
	}
	want := []string{
		filepath.Join(project, ".edikt", "templates", rel),
		filepath.Join(root, "current", "templates", rel),
		filepath.Join(root, "templates", rel),
		filepath.Join(project, "templates", rel),
	}
	if len(nf.Candidates) != len(want) {
		t.Fatalf("candidates %v, want %v", nf.Candidates, want)
	}
	for i, w := range want {
		if nf.Candidates[i] != w {
			t.Errorf("candidate %d = %q, want %q", i, nf.Candidates[i], w)
		}
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error text does not name %q:\n%s", w, err)
		}
	}
}

// A directory at a template's path is not the template: the file rung must
// skip it, and the dir variant must take it.
func TestResolveTemplate_RequiresRegularFile(t *testing.T) {
	base, project := sandbox(t)
	root := filepath.Join(base, "eh")
	t.Setenv("EDIKT_HOME", root)
	if err := os.MkdirAll(filepath.Join(project, ".edikt", "templates", rel), 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "current", "templates", rel)
	writeFile(t, want)

	got, err := ResolveTemplate(project, rel)
	if err != nil || got != want {
		t.Fatalf("got (%q, %v), want %q — a directory satisfied a file lookup", got, err, want)
	}

	dir, err := ResolveTemplateDir(project, "rubrics")
	if err != nil {
		t.Fatalf("ResolveTemplateDir: %v", err)
	}
	if wantDir := filepath.Join(project, ".edikt", "templates", "rubrics"); dir != wantDir {
		t.Fatalf("dir resolved %q, want the override %q", dir, wantDir)
	}
	if _, err := ResolveTemplateDir(project, rel+"-absent"); err == nil {
		t.Fatal("ResolveTemplateDir reported success for a path that exists nowhere")
	}
}

// rel is joined onto every rung; one that escapes the templates tree, or is
// absolute, would turn the lookup into an arbitrary read (INV-006).
func TestResolveTemplate_RejectsEscapingRel(t *testing.T) {
	_, project := sandbox(t)
	for _, bad := range []string{"", "/etc/passwd", "../secrets.md", "rubrics/../../x.md"} {
		if _, err := ResolveTemplate(project, bad); err == nil {
			t.Errorf("rel %q accepted", bad)
		}
	}
}

// Project mode makes <root>/templates and <project>/.edikt/templates the same
// directory. The candidate list must not name it twice.
func TestResolveTemplate_CandidatesAreDeduplicated(t *testing.T) {
	_, project := sandbox(t)
	t.Setenv("EDIKT_ROOT", filepath.Join(project, ".edikt"))
	c := Candidates(project, rel)
	seen := map[string]bool{}
	for _, p := range c {
		if seen[p] {
			t.Fatalf("candidate %q listed twice: %v", p, c)
		}
		seen[p] = true
	}
}
