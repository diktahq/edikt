package hookmatch

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGoverningFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "rules", "governance")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The map holds every directive with the surface that delivers it. Since
	// ADR-083 there are two surfaces; grade travels on the entry.
	if err := os.WriteFile(filepath.Join(dir, "delivery-map.yaml"), []byte(`schema_version: 1
globs:
  "src/**/*.go":
    - id: INV-001:d01
      grade: must
      topic: architecture
      surface: topic-file
    - id: ADR-001:d01
      grade: advisory
      topic: architecture
      surface: topic-file
    - id: ADR-002:d01
      grade: advisory
      topic: compile
      surface: skill
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "x.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// MatchGoverning reports what governs the path on every surface that
// delivers, with the grade pinned on each entry. There used to be a second
// matcher here (Match) answering the narrower "what does PreToolUse inject";
// ADR-083 retired it with the write-time surface, so this is the one answer.
func TestMatchGoverning_SeesEverySurface(t *testing.T) {
	root := writeGoverningFixture(t)
	target := filepath.Join(root, "src", "x.go")

	gov := MatchGoverning(root, target)
	if len(gov.Entries) != 3 {
		t.Fatalf("MatchGoverning must report every governing entry, got %d: %+v", len(gov.Entries), gov.Entries)
	}
	bySurface := map[string]int{}
	byGrade := map[string]int{}
	for _, e := range gov.Entries {
		bySurface[e.Surface]++
		byGrade[e.Grade]++
	}
	if bySurface["topic-file"] != 2 || bySurface["skill"] != 1 {
		t.Errorf("want 2 topic-file + 1 skill entries, got %v", bySurface)
	}
	if bySurface["write-time"] != 0 {
		t.Errorf("the retired write-time surface reappeared: %v", bySurface)
	}
	if byGrade["must"] != 1 || byGrade["advisory"] != 2 {
		t.Errorf("grade must travel on the entry: got %v", byGrade)
	}
}

// Fail LOUDLY, not open. MatchGoverning sits on the measurement path, where a
// silent zero would let a coverage gate certify a corpus it never read.
func TestMatchGoverning_MissingMapIsAnOutcomeNotAnEmptyPass(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := MatchGoverning(root, filepath.Join(root, "x.go"))
	if res.Outcome != OutcomeIndexMissing {
		t.Fatalf("a missing delivery map must be reported as an outcome, got %q", res.Outcome)
	}
	if len(res.Entries) != 0 {
		t.Fatalf("no entries can be resolved without a map, got %+v", res.Entries)
	}
}

// One directive reachable through two globs is reported once, so a coverage
// COUNT is not inflated by a directive whose sidecar happens to declare
// overlapping globs.
func TestMatchGoverning_DedupsAcrossOverlappingGlobs(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "rules", "governance")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "delivery-map.yaml"), []byte(`schema_version: 1
globs:
  "src/**/*":
    - id: ADR-001:d01
      grade: advisory
      surface: topic-file
  "src/**/*.go":
    - id: ADR-001:d01
      grade: advisory
      surface: topic-file
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "x.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := MatchGoverning(root, filepath.Join(root, "src", "x.go"))
	if len(res.Entries) != 1 {
		t.Fatalf("a directive reachable through two globs must be counted once, got %d: %+v", len(res.Entries), res.Entries)
	}
}
