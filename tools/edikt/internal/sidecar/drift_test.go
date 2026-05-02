package sidecar

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFixture lays down a minimal parent .md + sidecar pair under tmp.
// The sidecar's source_excerpt points at line 3 of the parent.
func writeFixture(t *testing.T, tmp, quote string) (parentPath string, sc *Sidecar) {
	t.Helper()
	dir := filepath.Join(tmp, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	parentPath = filepath.Join(dir, "ADR-001-test.md")
	parent := "# Title\n\n" + quote + "\n"
	if err := os.WriteFile(parentPath, []byte(parent), 0o644); err != nil {
		t.Fatal(err)
	}
	sc = &Sidecar{
		SchemaVersion: 1,
		Topic:         "test",
		Path:          "docs/architecture/decisions/ADR-001-test.md",
		Directives: []Directive{
			{
				Text: "Test directive. (ref: ADR-001)",
				SourceExcerpt: SourceExcerpt{
					LineStart: 3,
					LineEnd:   3,
					Quote:     quote,
				},
			},
		},
	}
	return
}

func TestIsStale_InSyncReturnsFalse(t *testing.T) {
	tmp := t.TempDir()
	_, sc := writeFixture(t, tmp, "Use widgets when frobnicating.")
	stale, reason, err := sc.IsStale(tmp)
	if err != nil {
		t.Fatalf("IsStale: %v", err)
	}
	if stale {
		t.Fatalf("expected in-sync, got stale: %s", reason)
	}
}

func TestIsStale_ModifiedBodyReturnsTrue(t *testing.T) {
	tmp := t.TempDir()
	parentPath, sc := writeFixture(t, tmp, "Original quote.")
	if err := os.WriteFile(parentPath, []byte("# Title\n\nReplaced quote line.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale, reason, err := sc.IsStale(tmp)
	if err != nil {
		t.Fatalf("IsStale: %v", err)
	}
	if !stale {
		t.Fatal("expected stale after body modification")
	}
	if reason == "" {
		t.Error("expected reason populated when stale")
	}
}

func TestIsStale_LineRangeOutOfBoundsReturnsTrue(t *testing.T) {
	tmp := t.TempDir()
	_, sc := writeFixture(t, tmp, "Quote.")
	sc.Directives[0].SourceExcerpt.LineStart = 100
	sc.Directives[0].SourceExcerpt.LineEnd = 101
	stale, _, err := sc.IsStale(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !stale {
		t.Fatal("expected stale when line range exceeds body length")
	}
}
