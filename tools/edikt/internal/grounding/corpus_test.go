package grounding

import (
	"os"
	"path/filepath"
	"testing"
)

// repoRoot walks up from this package to the repository root.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// internal/grounding -> tools/edikt -> repo root
	root := filepath.Clean(filepath.Join(wd, "..", "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "docs", "architecture", "decisions")); err != nil {
		t.Skipf("dogfood corpus not present at %s: %v", root, err)
	}
	return root
}

// TestGroundingCorpus reports the grounding measurement for this repo's own
// governance corpus. It is a measurement, not a gate — Phase 4 adds the
// ratchet that holds the number. Run with -v to read the breakdown.
func TestGroundingCorpus(t *testing.T) {
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
	for _, f := range rep.Findings {
		text := f.Text
		if len(text) > 90 {
			text = text[:90] + "..."
		}
		t.Logf("  %-28s %s[%d] %s", f.Verdict, f.ArtifactID, f.Index, text)
	}

	if rep.TotalItems == 0 {
		t.Fatal("scanned zero items — the corpus was not found, so this reports nothing")
	}
}
