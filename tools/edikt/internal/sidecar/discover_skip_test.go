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

// N2 (docs/internal/audits/TRIAGE-2026-08-20-bok-services-governance-projection.md):
// a proposed ADR/invariant with no sidecar must be able to opt out of
// MISSING explicitly, without inferring the opt-out from status: proposed
// (this repo's own ADR-063 is proposed and legitimately HAS a sidecar, so
// a blanket status filter would suppress real signal). "sidecar: skip" is
// deliberately a different key from "no-directives:" — see N3.

func TestIsSkipListed_FrontmatterSidecarSkip(t *testing.T) {
	dir := t.TempDir()
	p := writeMD(t, dir, "ADR-100-pending.md",
		"---\nstatus: proposed\nsidecar: skip\nreason: \"awaiting acceptance gate\"\n---\n\n# ADR-100\n\nMUST do the thing.\n")
	skip, reason := IsSkipListed(p)
	if !skip {
		t.Fatal("frontmatter sidecar: skip must skip-list the artifact")
	}
	if reason != "awaiting acceptance gate" {
		t.Errorf("expected the declared reason to be surfaced, got %q", reason)
	}
}

func TestIsSkipListed_FrontmatterSidecarSkipNoReason(t *testing.T) {
	dir := t.TempDir()
	p := writeMD(t, dir, "ADR-101-pending.md",
		"---\nstatus: proposed\nsidecar: skip\n---\n\n# ADR-101\n\nMUST do the thing.\n")
	if skip, reason := IsSkipListed(p); !skip || reason == "" {
		t.Fatalf("sidecar: skip with no reason must still skip-list with a non-empty default reason; got skip=%v reason=%q", skip, reason)
	}
}

func TestIsSkipListed_BodyMarkerSidecarSkip(t *testing.T) {
	dir := t.TempDir()
	p := writeMD(t, dir, "ADR-102-pending.md",
		"<!-- edikt:sidecar:skip reason=\"split out by owner ruling, not yet accepted\" -->\n\n"+
			"# ADR-102\n\nMUST do the thing.\n")
	skip, reason := IsSkipListed(p)
	if !skip {
		t.Fatal("body marker edikt:sidecar:skip must skip-list the artifact")
	}
	if reason != "split out by owner ruling, not yet accepted" {
		t.Errorf("expected the declared reason to be surfaced, got %q", reason)
	}
}

// Control: status: proposed ALONE, with no explicit opt-out, must still
// error MISSING — this is the case the report's own suggested "scope
// MISSING to status: accepted" fix would have wrongly suppressed, and
// which this repo's own ADR-063 (proposed, with a real sidecar) proves
// matters: a proposed artifact CAN legitimately have a sidecar, so
// forgetting to compile one is still real signal.
func TestIsSkipListed_ProposedStatusAloneNotSkipped(t *testing.T) {
	dir := t.TempDir()
	p := writeMD(t, dir, "ADR-103-pending.md",
		"---\nstatus: proposed\n---\n\n# ADR-103\n\nMUST do the thing.\n")
	if skip, reason := IsSkipListed(p); skip {
		t.Fatalf("status: proposed alone (no explicit sidecar: skip) must NOT be skip-listed — got skip=%v reason=%q", skip, reason)
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
