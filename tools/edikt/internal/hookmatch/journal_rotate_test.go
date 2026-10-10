package hookmatch

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The defect this guards: rotation used to rename to a FIXED <name>.1, so every
// rotation overwrote the previous generation and two files were the entire
// retention policy. On 2026-08-29 that was 21 hours of history.

func writeRotationFixture(t *testing.T, p string, n int) {
	t.Helper()
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(`{"kind":"hook_run","ts":"2026-08-29T00:00:00Z"}` + "\n")
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func archivedGenerations(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "hook-journal-") {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestRotate_NeverOverwritesAPreviousGeneration(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "hook-journal.jsonl")
	arc := filepath.Join(root, ArchiveDirName)

	for i := 0; i < 3; i++ {
		writeRotationFixture(t, p, 10)
		rotate(p)
	}
	got := archivedGenerations(t, arc)
	if len(got) != 3 {
		t.Fatalf("three rotations must leave three generations, got %d: %v", len(got), got)
	}
	// The old behaviour would leave exactly one file named hook-journal.jsonl.1.
	for _, n := range got {
		if strings.HasSuffix(n, ".1") {
			t.Errorf("fixed-suffix name reappeared: %s", n)
		}
	}
}

func TestRotate_CompressesAndKeepsContent(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "hook-journal.jsonl")
	writeRotationFixture(t, p, 5)
	rotate(p)

	got := archivedGenerations(t, filepath.Join(root, ArchiveDirName))
	if len(got) != 1 || !strings.HasSuffix(got[0], ".gz") {
		t.Fatalf("expected one gzipped generation, got %v", got)
	}
	f, err := os.Open(filepath.Join(root, ArchiveDirName, got[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "\n"); n != 5 {
		t.Errorf("archivedGenerations generation lost records: want 5 lines, got %d", n)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("live journal should have been moved aside, not copied")
	}
}

func TestPruneArchive_CapsTotalButNeverEmptiesIt(t *testing.T) {
	dir := t.TempDir()
	big := make([]byte, MaxArchiveBytes+1)
	// Three generations, each over the whole cap on its own.
	for _, n := range []string{"hook-journal-20260101T000000Z.jsonl",
		"hook-journal-20260102T000000Z.jsonl",
		"hook-journal-20260103T000000Z.jsonl"} {
		if err := os.WriteFile(filepath.Join(dir, n), big, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pruneArchive(dir)
	got := archivedGenerations(t, dir)
	if len(got) != 1 {
		t.Fatalf("prune should have reduced to the newest survivor, got %v", got)
	}
	if got[0] != "hook-journal-20260103T000000Z.jsonl" {
		t.Errorf("prune deleted newest-first; want the newest kept, got %s", got[0])
	}
}
