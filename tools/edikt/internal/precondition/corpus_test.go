package precondition

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
	root := filepath.Clean(filepath.Join(wd, "..", "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "docs", "architecture", "decisions")); err != nil {
		t.Skipf("dogfood corpus not present at %s: %v", root, err)
	}
	return root
}

// TestPreconditionCorpus reports the Phase 3 measurement against this repo's
// own governance corpus. A measurement, not a gate — Phase 4 ratchets it.
// Run with -v to read the breakdown and every finding.
func TestPreconditionCorpus(t *testing.T) {
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
		t.Logf("  %-20s %s[%s:%d]", f.Verdict, f.ArtifactID, f.Kind, f.Index)
		if len(f.Missing) > 0 {
			t.Logf("      MISSING:    %v", f.Missing)
		}
		if len(f.Unresolved) > 0 {
			t.Logf("      declined:   %v", f.Unresolved)
		}
		t.Logf("      verify:     %s", f.Verify)
	}

	// A scan that examined no verify command measured nothing, and every
	// count above would be a zero with no subject behind it (INV-013).
	if rep.SidecarsScanned == 0 {
		t.Fatal("scanned zero sidecars — the corpus was not found")
	}
	if rep.VerifyCommands == 0 {
		t.Fatal("found zero verify commands — this reports nothing about preconditions")
	}
}
