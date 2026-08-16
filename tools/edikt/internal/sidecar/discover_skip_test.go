package sidecar

import (
	"os"
	"path/filepath"
	"testing"
)

// Field bug (bok-services 2026-08-07): ADRs retired with frontmatter
// `status: superseded` + `superseded_by:` — and no bolded body status line —
// were re-bootstrapped by Phase A and their directives compiled, duplicating
// rules that had been reclassified into guidelines. Discovery must honour
// the frontmatter status, not just the `**Status:**` body form.

func writeMD(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

func TestIsSkipListed_FrontmatterStatusSuperseded(t *testing.T) {
	dir := t.TempDir()
	p := writeMD(t, dir, "ADR-005-old.md",
		"---\nid: ADR-005\nstatus: superseded\nsuperseded_by: GL-004\n---\n\n# ADR-005\n\n## Decision\n\nOld rule.\n")
	skip, reason := IsSkipListed(p)
	if !skip {
		t.Fatal("frontmatter status: superseded must skip-list the artifact")
	}
	if reason == "" {
		t.Error("skip reason must be populated")
	}
}

func TestIsSkipListed_FrontmatterStatusSupersededByValue(t *testing.T) {
	dir := t.TempDir()
	// The status value itself sometimes carries the target inline.
	p := writeMD(t, dir, "ADR-006-old.md",
		"---\nstatus: Superseded by ADR-027\n---\n\n# ADR-006\n")
	if skip, _ := IsSkipListed(p); !skip {
		t.Fatal("frontmatter status: Superseded by ... must skip-list")
	}
}

func TestIsSkipListed_FrontmatterStatusDeprecated(t *testing.T) {
	dir := t.TempDir()
	p := writeMD(t, dir, "ADR-007-old.md",
		"---\nstatus: deprecated\n---\n\n# ADR-007\n")
	if skip, _ := IsSkipListed(p); !skip {
		t.Fatal("frontmatter status: deprecated must skip-list")
	}
}

func TestIsSkipListed_BodyStatusDeprecated(t *testing.T) {
	dir := t.TempDir()
	p := writeMD(t, dir, "ADR-008-old.md",
		"# ADR-008\n\n**Status:** Deprecated\n\n## Decision\n\nOld.\n")
	if skip, _ := IsSkipListed(p); !skip {
		t.Fatal("body **Status:** Deprecated must skip-list")
	}
}

func TestIsSkipListed_AcceptedStatusNotSkipped(t *testing.T) {
	dir := t.TempDir()
	p := writeMD(t, dir, "ADR-009-live.md",
		"---\nstatus: accepted\n---\n\n# ADR-009\n\n## Decision\n\nLive rule.\n")
	if skip, reason := IsSkipListed(p); skip {
		t.Fatalf("accepted artifact must not be skip-listed (reason=%q)", reason)
	}
}

func TestDiscover_MarksFrontmatterSupersededAsSkip(t *testing.T) {
	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMD(t, adrDir, "ADR-001-live.md",
		"---\nstatus: accepted\n---\n\n## Decision\n\nLive.\n")
	writeMD(t, adrDir, "ADR-002-retired.md",
		"---\nstatus: superseded\nsuperseded_by: GL-004\n---\n\n## Decision\n\nRetired.\n")

	pairs, err := Discover(root, []string{"docs/architecture/decisions", "", ""})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	skips := map[string]bool{}
	for _, p := range pairs {
		skips[p.ArtifactID] = p.Skip
	}
	if skips["ADR-001"] {
		t.Error("ADR-001 (accepted) must not be Skip")
	}
	if !skips["ADR-002"] {
		t.Error("ADR-002 (frontmatter superseded) must be Skip")
	}
}
