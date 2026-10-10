package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stageManifestProject builds a templates root declaring three `always:` agents
// and one default rule pack, plus a project installing whichever of them the
// caller names.
func stageManifestProject(t *testing.T, installedAgents, installedRules []string) (projectRoot, templatesRoot string) {
	t.Helper()
	root := t.TempDir()
	templatesRoot = filepath.Join(root, "templates")
	projectRoot = filepath.Join(root, "project")

	mk := func(p string) {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mk(filepath.Join(templatesRoot, "agents"))
	mk(filepath.Join(templatesRoot, "rules", "base"))
	mk(filepath.Join(projectRoot, ".claude", "agents"))
	mk(filepath.Join(projectRoot, ".claude", "rules"))

	reg := "always:\n  - architect\n  - sidecar-extractor\n  - governance-verifier\n"
	if err := os.WriteFile(filepath.Join(templatesRoot, "agents", "_registry.yaml"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"architect", "sidecar-extractor", "governance-verifier"} {
		if err := os.WriteFile(filepath.Join(templatesRoot, "agents", a+".md"), []byte("# "+a+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rreg := "code-quality:\n  tier: base\n  template: base/code-quality.md\n  version: \"0.1.0\"\n  default: true\n" +
		"testing:\n  tier: base\n  template: base/testing.md\n  version: \"0.1.0\"\n  default: true\n"
	if err := os.WriteFile(filepath.Join(templatesRoot, "rules", "_registry.yaml"), []byte(rreg), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, r := range []string{"code-quality", "testing"} {
		if err := os.WriteFile(filepath.Join(templatesRoot, "rules", "base", r+".md"), []byte("# "+r+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, a := range installedAgents {
		if err := os.WriteFile(filepath.Join(projectRoot, ".claude", "agents", a+".md"), []byte("# "+a+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range installedRules {
		if err := os.WriteFile(filepath.Join(projectRoot, ".claude", "rules", r+".md"), []byte("# "+r+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return projectRoot, templatesRoot
}

// DIRECTION 2 — declared by the version, absent from the project. This is the
// direction upgrade cannot reach for agents and rule packs, because its loops
// walk installed files. Five agents went missing on a real upgrade this way.
func TestManifestDrift_ReportsDeclaredButAbsent(t *testing.T) {
	project, templates := stageManifestProject(t, []string{"architect"}, nil)

	var buf bytes.Buffer
	warns, ran := runManifestDriftCheck(project, templates, &buf)
	out := buf.String()

	if !ran {
		t.Fatalf("check did not run:\n%s", out)
	}
	for _, want := range []string{"missing: sidecar-extractor", "missing: governance-verifier"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not name %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "missing: architect") {
		t.Errorf("an installed agent was reported missing:\n%s", out)
	}
	// Rule packs are optional and share .claude/rules/ with compiled
	// governance. A project installing NONE of them has opted out, not
	// drifted, and must not generate a warning per declared pack.
	if strings.Contains(out, "missing: code-quality") {
		t.Errorf("an opted-out optional surface was reported as drift:\n%s", out)
	}
	if warns != 2 {
		t.Errorf("want 2 warnings (agents only), got %d\n%s", warns, out)
	}
}

// DIRECTION 3 — present in the project, no longer shipped by this version.
// Reported, never removed.
func TestManifestDrift_ReportsRetired(t *testing.T) {
	project, templates := stageManifestProject(t,
		[]string{"architect", "sidecar-extractor", "governance-verifier", "pre-compact-helper"},
		[]string{"code-quality"})

	var buf bytes.Buffer
	_, ran := runManifestDriftCheck(project, templates, &buf)
	if !ran {
		t.Fatal("check did not run")
	}
	if !strings.Contains(buf.String(), "retired: pre-compact-helper") {
		t.Errorf("an installed artifact this version no longer ships was not reported:\n%s", buf.String())
	}
}

// A user's own agent is NOT retired. INV-019's boundary requires ownership to
// be provable, and a template still shipping for a slug proves it is edikt's.
func TestManifestDrift_DoesNotCallAShippedTemplateRetired(t *testing.T) {
	// `architect` is shipped but not in `always:` here — it IS declared, so it
	// must not appear as retired either.
	project, templates := stageManifestProject(t,
		[]string{"architect", "sidecar-extractor", "governance-verifier"},
		[]string{"code-quality"})
	// Add a user agent with no template behind it.
	if err := os.WriteFile(filepath.Join(project, ".claude", "agents", "my-team-reviewer.md"), []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// And one that IS still shipped but undeclared — must not be called retired.
	if err := os.WriteFile(filepath.Join(templates, "agents", "docs.md"), []byte("# docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".claude", "agents", "docs.md"), []byte("# docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	runManifestDriftCheck(project, templates, &buf)
	out := buf.String()
	if strings.Contains(out, "retired: docs") {
		t.Errorf("an agent this version still ships was called retired:\n%s", out)
	}
	if !strings.Contains(out, "retired: my-team-reviewer") {
		t.Logf("note: user agent reported as retired — acceptable only if the report never removes; got:\n%s", out)
	}
}

// A clean project must report a MEASURED zero carrying the manifest's count.
// "OK" with no denominator is indistinguishable from a surface never examined.
func TestManifestDrift_CleanProjectReportsDenominator(t *testing.T) {
	project, templates := stageManifestProject(t,
		[]string{"architect", "sidecar-extractor", "governance-verifier"},
		[]string{"code-quality", "testing"})

	var buf bytes.Buffer
	warns, ran := runManifestDriftCheck(project, templates, &buf)
	out := buf.String()

	if !ran {
		t.Fatal("check did not run")
	}
	if warns != 0 {
		t.Errorf("clean project warned:\n%s", out)
	}
	if !strings.Contains(out, "agents — 3 of 3 declared present") {
		t.Errorf("agents line carries no denominator:\n%s", out)
	}
	if !strings.Contains(out, "rule packs — 2 of 2 declared present") {
		t.Errorf("rule packs line carries no denominator:\n%s", out)
	}
}

// An unreadable manifest is UNMEASURED, never a clean pass.
func TestManifestDrift_UnreadableManifestIsUnmeasured(t *testing.T) {
	project, templates := stageManifestProject(t, nil, nil)
	if err := os.WriteFile(filepath.Join(templates, "agents", "_registry.yaml"), []byte("{{{ not yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	warns, ran := runManifestDriftCheck(project, templates, &buf)
	if !ran {
		t.Fatal("check did not run")
	}
	if !strings.Contains(buf.String(), "UNMEASURED") {
		t.Errorf("an unreadable manifest was not reported as unmeasured:\n%s", buf.String())
	}
	if warns == 0 {
		t.Error("an unmeasured surface produced no warning")
	}
}

// No templates root — the check must decline to run rather than report a clean
// project it never examined.
func TestManifestDrift_NoTemplatesRootDoesNotRun(t *testing.T) {
	var buf bytes.Buffer
	_, ran := runManifestDriftCheck(t.TempDir(), filepath.Join(t.TempDir(), "absent"), &buf)
	if ran {
		t.Errorf("check claimed to run with no templates root:\n%s", buf.String())
	}
}

// PARTIAL installation IS drift. The opt-out path must not swallow the case it
// exists beside: a project that has rule packs and lost one is exactly the
// state this check is for.
func TestManifestDrift_PartialRulePackInstallIsDrift(t *testing.T) {
	project, templates := stageManifestProject(t,
		[]string{"architect", "sidecar-extractor", "governance-verifier"},
		[]string{"code-quality"}) // "testing" declared but absent

	var buf bytes.Buffer
	warns, _ := runManifestDriftCheck(project, templates, &buf)
	out := buf.String()

	if !strings.Contains(out, "missing: testing") {
		t.Errorf("a partially-installed surface did not report its missing pack:\n%s", out)
	}
	if strings.Contains(out, "does not use them") {
		t.Errorf("a project WITH rule packs was treated as opted out:\n%s", out)
	}
	if warns != 1 {
		t.Errorf("want 1 warning, got %d\n%s", warns, out)
	}
}

// A project that compiles governance keeps governance.md in .claude/rules/ and
// installs no packs. It must read as opted out, with the denominator stated.
func TestManifestDrift_CompiledGovernanceIsNotRulePackDrift(t *testing.T) {
	project, templates := stageManifestProject(t,
		[]string{"architect", "sidecar-extractor", "governance-verifier"}, nil)
	if err := os.WriteFile(filepath.Join(project, ".claude", "rules", "governance.md"), []byte("# compiled\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	runManifestDriftCheck(project, templates, &buf)
	out := buf.String()
	if !strings.Contains(out, "rule packs — none of the 2 declared are installed") {
		t.Errorf("compiled-governance project not reported as opted out with a denominator:\n%s", out)
	}
	if strings.Contains(out, "retired: governance") {
		t.Errorf("compiled governance output was reported as a retired rule pack:\n%s", out)
	}
}
