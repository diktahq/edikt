package gov

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// updateGolden regenerates the golden files in place when true.
// Set via: go test ./cmd/gov -run TestCompileGolden -update-golden
// or: EDIKT_UPDATE_GOLDEN=1 go test ./cmd/gov -run TestCompileGolden
var updateGolden = flag.Bool("update-golden", false, "regenerate golden files")

// compiledAtLine strips the compiled_at timestamp line before comparison so
// a re-run at a different time doesn't fail.
var compiledAtRE = regexp.MustCompile(`(?m)^<!-- compiled_at: [^\n]+ -->$\n?`)

// compiledByRE strips the compiled_by version line (binary version bumps
// should not break the test).
var compiledByRE = regexp.MustCompile(`(?m)^<!-- compiled_by: [^\n]+ -->$\n?`)

// normalize strips unstable lines from a governance file for comparison.
func normalize(b []byte) []byte {
	b = compiledAtRE.ReplaceAll(b, nil)
	b = compiledByRE.ReplaceAll(b, nil)
	return b
}

// TestCompileGolden verifies that `edikt gov compile` produces output
// byte-equal (modulo timestamps and version stamps) to the committed
// .claude/rules/ snapshot.
//
// This is the quality gate ensuring the Go binary never silently drops,
// reorders, or rewrites a governance directive compared to the known-good
// output. Run with -update-golden to regenerate after a deliberate
// governance change.
func TestCompileGolden(t *testing.T) {
	if os.Getenv("EDIKT_UPDATE_GOLDEN") == "1" {
		*updateGolden = true
	}

	repoRoot := goldenRoot(t)

	// Write compiled output to a scratch directory.
	outDir := t.TempDir()
	copyDir(t, filepath.Join(repoRoot, ".edikt"), filepath.Join(outDir, ".edikt"))
	copyDir(t, filepath.Join(repoRoot, "docs/architecture"), filepath.Join(outDir, "docs/architecture"))
	// Guidelines joined the corpus with GL-001 (2026-08-07); without this
	// copy the temp compile sees 0 guidelines and diverges from golden.
	copyDir(t, filepath.Join(repoRoot, "docs/guidelines"), filepath.Join(outDir, "docs/guidelines"))

	// Run compile against the scratch project. The dogfood project has been
	// migrated to sidecars (PLAN-v060-governance-accuracy phase 5 Half B), so
	// the two-phase sidecar-aware path is the right one. --legacy was retired
	// in favour of two-phase compile + supersession recognition + IsStale
	// default-fallback skip (drift.go).
	//
	// --skip-verify disables the post-compile verify gate. This test's
	// outDir only mirrors .edikt/ + docs/architecture/ — verify commands
	// inside the corpus reference repo files (templates/, bin/edikt,
	// .gitignore, test/security/ scripts) that don't exist in the
	// tempdir, so the gate would flag many ENOENT failures unrelated to
	// compile correctness. The verify gate's actual value — "do the
	// project's verifies pass against the real project?" — is exercised
	// elsewhere (`bin/edikt verify all` in CI). This test scopes to
	// "does compile output match golden?" only.
	buf, err := runGovCmd(t, "gov", "compile", "--skip-verify", outDir)
	if err != nil {
		if isExitCode(err, 1) {
			t.Fatalf("gov compile returned errors:\n%s", buf)
		}
		t.Fatalf("gov compile failed: %v\n%s", err, buf)
	}

	// The Go binary writes to <outDir>/.claude/rules/. Compare to repo golden.
	goldenDir := filepath.Join(repoRoot, ".claude/rules/governance")
	actualDir := filepath.Join(outDir, ".claude/rules/governance")
	goldenIdx := filepath.Join(repoRoot, ".claude/rules/governance.md")
	actualIdx := filepath.Join(outDir, ".claude/rules/governance.md")

	if *updateGolden {
		t.Log("--update-golden: copying actual output to repo golden")
		copyDir(t, actualDir, goldenDir)
		copyFile(t, actualIdx, goldenIdx)
		t.Log("golden files updated — commit the result")
		return
	}

	// Compare governance.md index.
	compareFiles(t, goldenIdx, actualIdx, "governance.md")

	// Compare every topic file.
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatalf("cannot read golden dir %s: %v", goldenDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		compareFiles(t,
			filepath.Join(goldenDir, e.Name()),
			filepath.Join(actualDir, e.Name()),
			"governance/"+e.Name(),
		)
	}

	// Also assert no extra files were created.
	//
	// Counted BY CLASS. This compared a .md-filtered loop against an unfiltered
	// directory count, so it only held while every generated file happened to
	// be a .md — the first non-.md surface (directive-index.yaml, then
	// manifest.yaml) broke it for a reason unrelated to the golden content.
	//
	// The generated non-.md surfaces are now named rather than counted, which
	// is strictly stronger: a count is satisfied by any file of the right
	// arity, whereas naming them fails if a surface is renamed, missing, or
	// joined by an unexpected sibling.
	var goldenMD, actualMD int
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			goldenMD++
		}
	}
	actEntries, _ := os.ReadDir(actualDir)
	expectedNonMD := map[string]bool{"directive-index.yaml": true, "manifest.yaml": true}
	seenNonMD := map[string]bool{}
	for _, e := range actEntries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".md") {
			actualMD++
			continue
		}
		if !expectedNonMD[e.Name()] {
			t.Errorf("unexpected generated file %q in the governance output dir", e.Name())
		}
		seenNonMD[e.Name()] = true
	}
	if actualMD != goldenMD {
		t.Errorf("expected %d topic .md file(s), got %d (extra or missing)", goldenMD, actualMD)
	}
	for name := range expectedNonMD {
		if !seenNonMD[name] {
			t.Errorf("generated surface %q missing from the governance output dir", name)
		}
	}
}

func compareFiles(t *testing.T, golden, actual, label string) {
	t.Helper()

	gBytes, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("[%s] cannot read golden file %s: %v", label, golden, err)
	}
	aBytes, err := os.ReadFile(actual)
	if err != nil {
		t.Fatalf("[%s] cannot read actual file %s: %v", label, actual, err)
	}

	gNorm := normalize(gBytes)
	aNorm := normalize(aBytes)
	if bytes.Equal(gNorm, aNorm) {
		return
	}

	// Report the first differing line.
	gLines := strings.Split(string(gNorm), "\n")
	aLines := strings.Split(string(aNorm), "\n")
	for i := 0; i < len(gLines) || i < len(aLines); i++ {
		var g, a string
		if i < len(gLines) {
			g = gLines[i]
		}
		if i < len(aLines) {
			a = aLines[i]
		}
		if g != a {
			t.Errorf("[%s] line %d differs\n  golden: %q\n  actual: %q", label, i+1, g, a)
			if i+3 < len(gLines) {
				t.Logf("  context (golden +3): %q", strings.Join(gLines[i:i+3], "\n"))
			}
			return
		}
	}
}

func goldenRoot(t *testing.T) string {
	t.Helper()
	// This file is at tools/edikt/cmd/gov/ — repo root is 4 levels up.
	abs, err := filepath.Abs("../../../../")
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdirall %s: %v", dst, err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("readdir %s: %v", src, err)
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			copyDir(t, s, d)
		} else {
			copyFile(t, s, d)
		}
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("readfile %s: %v", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("writefile %s: %v", dst, err)
	}
}
