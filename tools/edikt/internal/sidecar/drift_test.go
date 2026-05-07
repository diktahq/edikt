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

// TestSidecar_IsStale_DefaultFallbackSkipped pins Phase 8 strategy A:
// when migrate's findDirectiveSource defaults to line_start=line_end=1
// with quote=directive_text (no source anchor), IsStale must NOT flag it.
func TestSidecar_IsStale_DefaultFallbackSkipped(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	parentPath := filepath.Join(dir, "ADR-099-test.md")
	// Parent body has totally different prose — the default-fallback
	// quote would NOT match. Without the skip, this test would hit the
	// "quote not found" branch.
	if err := os.WriteFile(parentPath, []byte("# Title\n\nUnrelated parent prose.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sc := &Sidecar{
		SchemaVersion: 1,
		Topic:         "test",
		Path:          "docs/architecture/decisions/ADR-099-test.md",
		Directives: []Directive{
			{
				Text: "Some legacy sentinel directive text.",
				SourceExcerpt: SourceExcerpt{
					LineStart: 1,
					LineEnd:   1,
					Quote:     "Some legacy sentinel directive text.",
				},
			},
		},
	}
	stale, reason, err := sc.IsStale(tmp)
	if err != nil {
		t.Fatalf("IsStale: %v", err)
	}
	if stale {
		t.Fatalf("expected default-fallback excerpt to be skipped, got stale: %s", reason)
	}
}

// TestIsStale_FreshlyCompiledFileIsNotStale pins the regression for the
// v0.4.5 audit "born-stale sentinel" bug.
//
// In v0.2-v0.4, /edikt:gov:review and /edikt:adr:review used MD5 of
// "everything above [edikt:directives:start]: #" and compared against
// the stored content_hash:. The compiler hashed the body BEFORE
// appending the sentinel + blank-line separator (sed '$d' dropped the
// trailing line); the reader hashed AFTER, including the inserted blank
// line. Result: every freshly-compiled file was born-stale, forever, on
// any read-time check that didn't replicate the sed '$d' quirk.
//
// v0.6.0's sidecar architecture replaces content_hash with per-directive
// source_excerpt.quote lookup against the parent .md body line range
// (this IsStale implementation). No synthesised separator is involved,
// so write-time and read-time agree by construction.
//
// This test pins the contract: a sidecar whose source_excerpt.quote
// matches the parent .md prose at the recorded line range MUST report
// stale=false. If a future refactor reintroduces the v0.4.5 hash-based
// approach, this test catches the born-stale regression.
func TestIsStale_FreshlyCompiledFileIsNotStale(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	parentPath := filepath.Join(dir, "ADR-100-test.md")
	// Multi-line parent body — the v0.4.5 bug was triggered by the blank
	// line between body and sentinel. Mimic that shape: prose content
	// followed by a blank line before what would have been the sentinel
	// in the legacy schema. v0.6.0 doesn't write a sentinel, but the
	// blank line is preserved as a normal separator.
	parent := "---\ntype: adr\nid: ADR-100\nstatus: accepted\n---\n\n" +
		"# ADR-100 — Test\n\n" +
		"## Decision\n\n" +
		"All hooks MUST emit JSON output.\n" +
		"\n" + // ← the blank line that broke v0.4.5's content_hash
		"## Consequences\n\nNone.\n"
	if err := os.WriteFile(parentPath, []byte(parent), 0o644); err != nil {
		t.Fatal(err)
	}
	sc := &Sidecar{
		SchemaVersion: 1,
		Topic:         "hooks",
		Path:          "docs/architecture/decisions/ADR-100-test.md",
		Directives: []Directive{
			{
				Text: "All hooks MUST emit JSON output. (ref: ADR-100)",
				SourceExcerpt: SourceExcerpt{
					LineStart: 11,
					LineEnd:   11,
					Quote:     "All hooks MUST emit JSON output.",
				},
			},
		},
	}
	stale, reason, err := sc.IsStale(tmp)
	if err != nil {
		t.Fatalf("IsStale: %v", err)
	}
	if stale {
		t.Fatalf("regression: freshly-compiled sidecar reported stale (v0.4.5 bug pattern). reason=%s", reason)
	}
}
