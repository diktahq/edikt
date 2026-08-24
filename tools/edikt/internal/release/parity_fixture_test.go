package release

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRepoValidationParityFixture is one half of SAC-001's shared-fixture
// parity check (SPEC-015 FR-015). The bash counterpart,
// test/integration/install/test_repo_validation_parity.sh, runs the SAME
// fixture file against install.sh's validate_repo(). A fixture edit that
// only updates one side's expectation is not possible — there is one file,
// not two — which is the achievable meaning of "single source of truth"
// across an unavoidable shell/Go boundary.
func TestRepoValidationParityFixture(t *testing.T) {
	path := repoValidationFixturePath(t)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening fixture %s: %v", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	checked := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.LastIndex(line, "\t")
		if idx < 0 {
			t.Fatalf("fixture line %d is not tab-separated: %q", lineNo, line)
		}
		value, verdict := line[:idx], line[idx+1:]
		if verdict != "ACCEPT" && verdict != "REJECT" {
			t.Fatalf("fixture line %d has an unknown verdict %q (want ACCEPT or REJECT)", lineNo, verdict)
		}

		checked++
		got := ValidateRepo(value)
		want := verdict == "ACCEPT"
		if got != want {
			t.Errorf("line %d: ValidateRepo(%q) = %v, want %v (fixture says %s)", lineNo, value, got, want, verdict)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	// INV-013: zero input is unmeasured, never a pass.
	if checked == 0 {
		t.Fatal("fixture contained zero vectors — parity is unmeasured, not proven")
	}
}

// repoValidationFixturePath locates the shared fixture relative to this
// package's directory (tools/edikt/internal/release), matching the pattern
// internal/precondition/corpus_test.go already uses for repo-root-relative
// fixtures under `go test`, whose working directory is the package dir.
func repoValidationFixturePath(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// internal/release -> internal -> edikt -> tools -> repo root
	root := filepath.Clean(filepath.Join(wd, "..", "..", "..", ".."))
	path := filepath.Join(root, "test", "fixtures", "install-verification-parity", "vectors.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("shared parity fixture not present at %s: %v", path, err)
	}
	return path
}
