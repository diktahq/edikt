package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}

// The positive-control pair ADR-082 (a) requires: a fenced marker is not a
// block; an unfenced block is. A scanner that cannot tell them apart reports
// a stale corpus for a document that merely discusses the format.
func TestScan_PositiveControl(t *testing.T) {
	root := repoRoot(t)
	fx := filepath.Join(root, "test", "fixtures", "migration-scan")

	fenced, err := ScanFile(filepath.Join(fx, "fenced.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fenced) != 0 {
		t.Fatalf("fenced.md must report nothing (marker only inside fences); got lines %v", fenced)
	}
	unfenced, err := ScanFile(filepath.Join(fx, "unfenced.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(unfenced) != 1 || unfenced[0] != 7 {
		t.Fatalf("unfenced.md must report exactly its block at line 7; got %v", unfenced)
	}

	findings, err := Scan(root, []string{"test/fixtures/migration-scan"})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !strings.HasSuffix(findings[0].Path, "unfenced.md") {
		t.Fatalf("Scan over the pair must find only unfenced.md; got %+v", findings)
	}
	if got := Files(findings); len(got) != 1 {
		t.Fatalf("Files must dedupe to one path, got %v", got)
	}
}

// On this corpus, as of 2026-09-02: ADR-009 (accepted, no sidecar, held out of
// the compile by an explicit edikt:migration:skip marker) carries a real
// unfenced block at line 128; ADR-008 (superseded) mentions the marker
// inside a yaml fence at line 27 AND carries a real unfenced block at line 259;
// INV-005 mentions the marker only inside fences. The scanner reports the two
// real blocks and neither fenced mention. Lifecycle (superseded or not) is not
// the scanner's business — it reports files; the caller decides what a block
// on a retired artifact means. When either block is lifted, this assertion is
// updated in the same commit, not weakened.
func TestScan_ThisCorpus(t *testing.T) {
	root := repoRoot(t)
	findings, err := Scan(root, []string{"docs/architecture/decisions", "docs/architecture/invariants", "docs/guidelines"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]int{}
	for _, f := range findings {
		key := ""
		for _, id := range []string{"ADR-008-", "ADR-009-", "INV-005-"} {
			if strings.Contains(f.Path, id) {
				key = id
			}
		}
		if key == "" {
			t.Errorf("unexpected block on this corpus: %+v — if a new residual block appeared, name it here", f)
			continue
		}
		got[key] = append(got[key], f.Line)
	}
	if lines := got["ADR-009-"]; len(lines) != 1 || lines[0] != 128 {
		t.Errorf("ADR-009 must report exactly its block at line 128, got %v", lines)
	}
	if lines := got["ADR-008-"]; len(lines) != 1 || lines[0] != 259 {
		t.Errorf("ADR-008 must report its real block at 259 and NOT the fenced mention at 27, got %v", lines)
	}
	if lines := got["INV-005-"]; len(lines) != 0 {
		t.Errorf("INV-005 mentions the marker only in fences; got %v", lines)
	}
}

// A directory that does not exist is skipped, not an error and not a finding.
func TestScan_MissingDirIsSkipped(t *testing.T) {
	findings, err := Scan(t.TempDir(), []string{"docs/nope"})
	if err != nil || len(findings) != 0 {
		t.Fatalf("missing dir: err=%v findings=%v", err, findings)
	}
}
