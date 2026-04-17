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

	// Run compile against the scratch project.
	buf, err := runGovCmd(t, "gov", "compile", outDir)
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
	actEntries, _ := os.ReadDir(actualDir)
	if len(actEntries) != len(entries) {
		t.Errorf("expected %d topic files, got %d (extra or missing files)",
			len(entries), len(actEntries))
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
	// This file is at tools/gov-compile/cmd/gov/ — repo root is 4 levels up.
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
