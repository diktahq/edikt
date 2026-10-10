package gov

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gradecompile_payload_test.go — #11 comment item 2.
//
// THE DEFECT, measured on HEAD 95517875: `gov grade-compile --print-inputs`
// looked for its rubric, schema and grader template at
// `<cwd>/.edikt/templates/…` and `<cwd>/templates/…` only. The second rung is
// a checkout of edikt itself, so on an adopter project — where the files live
// in the installed payload — the command exited 1 with "none of the expected
// asset paths exist", and /edikt:gov:grade-compile and /edikt:migrate Step 4
// stopped there. From a subdirectory it also resolved the governance dir
// against cwd and refused. ADR-005 §1 names the payload as the fallback.

// gradeAssets are the three files --print-inputs resolves, relative to a
// templates tree.
var gradeAssets = map[string]string{
	"rubric:":         "rubrics/compile-quality.md",
	"schema:":         "schemas/compile-quality-report.v1.schema.json",
	"agent_template:": "agents/compile-quality-grader.md",
}

type gradeSandbox struct {
	base, project, ediktHome string
}

// newGradeSandbox builds an adopter project (no templates/ of its own, a
// compiled governance dir) and an installed payload under a sandboxed
// $EDIKT_HOME. Nothing resolves against the developer's real HOME (INV-007).
func newGradeSandbox(t *testing.T) gradeSandbox {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := gradeSandbox{
		base:      base,
		project:   filepath.Join(base, "proj"),
		ediktHome: filepath.Join(base, "edikt-home"),
	}
	for _, d := range []string{
		filepath.Join(s.project, ".git"),
		filepath.Join(s.project, ".edikt"),
		filepath.Join(s.project, ".claude", "rules", "governance"),
		filepath.Join(base, "home"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(s.project, ".edikt", "config.yaml"), []byte("edikt_version: \"0.9.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, rel := range gradeAssets {
		plant(t, filepath.Join(s.ediktHome, "current", "templates", rel))
	}
	return s
}

func plant(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# "+filepath.Base(path)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// printInputs runs `gov grade-compile --print-inputs` from dir and returns the
// parsed key → path map, the exit code and the raw output.
func (s gradeSandbox) printInputs(t *testing.T, dir string) (map[string]string, int, string) {
	t.Helper()
	cmd := exec.Command(buildBinary(t), "gov", "grade-compile", "--print-inputs")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"HOME="+filepath.Join(s.base, "home"),
		"EDIKT_HOME="+s.ediktHome,
		"EDIKT_ROOT=",
		"CLAUDE_HOME="+filepath.Join(s.base, "home", ".claude"),
		"CLAUDE_CONFIG_DIR="+filepath.Join(s.base, "home", ".claude"),
		"EDIKT_INVOKER=ci",
		"EDIKT_SKIP_VERSION_GATE=1",
	)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("exec: %v\n%s", err, out)
	}
	got := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		k, v, ok := strings.Cut(line, " ")
		if ok && strings.HasSuffix(k, ":") {
			got[k] = strings.TrimSpace(v)
		}
	}
	return got, code, string(out)
}

func TestGradeCompilePrintInputs_ResolvesInstalledPayloadOutsideRepo(t *testing.T) {
	s := newGradeSandbox(t)
	got, code, out := s.printInputs(t, s.project)
	if code != 0 {
		t.Fatalf("--print-inputs on an adopter project exited %d — the installed payload was not a candidate:\n%s", code, out)
	}
	for key, rel := range gradeAssets {
		want := filepath.Join(s.ediktHome, "current", "templates", rel)
		if got[key] != want {
			t.Errorf("%s resolved %q, want the installed payload %q", key, got[key], want)
		}
	}
}

func TestGradeCompilePrintInputs_ProjectOverrideBeatsPayload(t *testing.T) {
	s := newGradeSandbox(t)
	override := filepath.Join(s.project, ".edikt", "templates", gradeAssets["rubric:"])
	plant(t, override)
	got, code, out := s.printInputs(t, s.project)
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if got["rubric:"] != override {
		t.Errorf("rubric resolved %q, want the project override %q (ADR-005 §1)", got["rubric:"], override)
	}
	// Overriding one asset does not move the others off the payload.
	if want := filepath.Join(s.ediktHome, "current", "templates", gradeAssets["schema:"]); got["schema:"] != want {
		t.Errorf("schema resolved %q, want the payload %q", got["schema:"], want)
	}
}

func TestGradeCompilePrintInputs_FromSubdirectory(t *testing.T) {
	s := newGradeSandbox(t)
	sub := filepath.Join(s.project, "src", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, code, out := s.printInputs(t, sub)
	if code != 0 {
		t.Fatalf("--print-inputs from a project subdirectory exited %d:\n%s", code, out)
	}
	if want := filepath.Join(s.project, ".claude", "rules", "governance"); got["governance_dir:"] != want {
		t.Errorf("governance_dir resolved %q, want the project's %q", got["governance_dir:"], want)
	}
	if want := filepath.Join(s.ediktHome, "current", "templates", gradeAssets["rubric:"]); got["rubric:"] != want {
		t.Errorf("rubric resolved %q, want %q", got["rubric:"], want)
	}
}

// The latent sibling: cheat-rate's adversary template had the same two-rung
// list, so `gov benchmark cheat-rate` could not run outside edikt-dev either.
func TestResolveAdversaryTemplate_FindsPayload(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(base, "proj")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	ediktHome := filepath.Join(base, "edikt-home")
	t.Setenv("HOME", filepath.Join(base, "home"))
	t.Setenv("EDIKT_ROOT", "")
	t.Setenv("EDIKT_HOME", ediktHome)
	t.Chdir(project)
	want := filepath.Join(ediktHome, "current", "templates", "agents", "cheat-rate-adversary.md")
	plant(t, want)

	got, err := resolveAdversaryTemplate(project)
	if err != nil {
		t.Fatalf("resolveAdversaryTemplate: %v", err)
	}
	if got != want {
		t.Fatalf("resolved %q, want the installed payload %q", got, want)
	}
}
