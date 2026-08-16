package normative

import (
	"os"
	"path/filepath"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// internal/normative -> tools/edikt -> repo root
	root := filepath.Clean(filepath.Join(wd, "..", "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "docs", "architecture", "decisions")); err != nil {
		t.Skipf("dogfood corpus not present at %s: %v", root, err)
	}
	return root
}

// TestNormativeCorpus reports all four Phase 2 measurements against this
// repo's own governance corpus, the way Phase 1 reported 711/712. These
// numbers are what Phase 4 ratchets. It is a measurement, not a gate.
// Run with -v to read the breakdown and the findings.
func TestNormativeCorpus(t *testing.T) {
	root := repoRoot(t)
	rep, err := Scan(root, []string{
		"docs/architecture/decisions",
		"docs/architecture/invariants",
		"docs/guidelines",
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	t.Logf("\n%s", rep.Summary())

	for _, c := range []*CheckReport{rep.Standalone, rep.MayLevel, rep.Force, rep.Duplicates} {
		if len(c.Findings) == 0 {
			continue
		}
		t.Logf("── %s ──", c.Name)
		for _, f := range c.Findings {
			text := f.Text
			if len(text) > 84 {
				text = text[:84] + "..."
			}
			partner := ""
			if f.Partner != "" {
				partner = " <> " + f.Partner
			}
			t.Logf("  %-20s %s%s %s", f.Verdict, f.Ref(), partner, text)
		}
	}

	// A scan that found nothing measured nothing. Without this the whole
	// test passes vacuously on a corpus that failed to load (INV-013).
	if rep.Corpus.TotalItems == 0 {
		t.Fatal("scanned zero items — the corpus was not found, so this reports nothing")
	}
	if rep.Corpus.SidecarsScanned == 0 {
		t.Fatal("scanned zero sidecars")
	}
}
