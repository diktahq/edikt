package cmd

// migrate_to_v2_test.go — regression coverage for the "already v2" bucket
// only ever meaning "loaded and validated as v2", never "we didn't check."
//
// F1 / docs/internal/issues/migrate-to-v2-unvalidated-already-bucket.md:
// WouldConvertToV2/ConvertFileToV2 only detect the ABSENCE of v1 markers, so
// a hand-authored metadata card carrying none of v1's or v2's required keys
// used to fall into "already v2" alongside genuinely valid v2 sidecars,
// unvalidated. It must now land in a distinct "invalid" bucket instead.

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMigrateToV2_UnvalidatedMetadataCard_IsInvalidNotAlready reproduces the
// exact shape from the bok-services field report: a sidecar carrying
// id/title/decision/evidence/acceptance and none of schema_version/topic/
// path/signals/directives. It must be reported as invalid, not silently
// folded into "already v2".
func TestMigrateToV2_UnvalidatedMetadataCard_IsInvalidNotAlready(t *testing.T) {
	dir := t.TempDir()
	withCWD(t, dir)

	decisions := filepath.Join(dir, "docs", "architecture", "decisions")
	if err := os.MkdirAll(decisions, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(decisions, "ADR-900-metadata-card.md"),
		[]byte("# ADR-900: metadata card\n\n**Status:** Accepted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Not a sidecar in either shape — a hand-authored metadata card, matching
	// what the field report found on the real corpus.
	metadataCard := `id: ADR-900
title: metadata card
decision: some decision text
evidence: some evidence
acceptance: accepted by owner
`
	if err := os.WriteFile(filepath.Join(decisions, "ADR-900-metadata-card.edikt.yaml"),
		[]byte(metadataCard), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "migrate", "to-v2", "--dry-run")
	if err != nil {
		t.Fatalf("migrate to-v2 --dry-run failed: %v\noutput:\n%s", err, out)
	}
	if !contains(out, "1 invalid") {
		t.Errorf("expected the metadata card to be counted as invalid, got:\n%s", out)
	}
	// The metadata card must not also be silently credited as an
	// unvalidated pass under "already v2".
	if !contains(out, "0 already v2") {
		t.Errorf("expected 0 already v2 (the metadata card must not be credited as already v2), got:\n%s", out)
	}
}

// TestMigrateToV2_GenuinelyValidV2_StillCountsAsAlready is the control: a
// real, schema-valid v2 sidecar must still land in "already v2", not get
// swept into "invalid" by an overzealous fix.
func TestMigrateToV2_GenuinelyValidV2_StillCountsAsAlready(t *testing.T) {
	dir := t.TempDir()
	withCWD(t, dir)

	decisions := filepath.Join(dir, "docs", "architecture", "decisions")
	if err := os.MkdirAll(decisions, 0o755); err != nil {
		t.Fatal(err)
	}

	mdPath := filepath.Join(decisions, "ADR-901-valid-v2.md")
	if err := os.WriteFile(mdPath,
		[]byte("# ADR-901: valid v2\n\n**Status:** Accepted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	validV2 := `schema_version: 2
topic: hooks
path: docs/architecture/decisions/ADR-901-valid-v2.md
signals:
  - hook
directives:
  - text: "Hooks must emit JSON."
    source_excerpts:
      - line_start: 1
        line_end: 1
        quote: "Hooks must emit JSON."
`
	if err := os.WriteFile(filepath.Join(decisions, "ADR-901-valid-v2.edikt.yaml"),
		[]byte(validV2), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "migrate", "to-v2", "--dry-run")
	if err != nil {
		t.Fatalf("migrate to-v2 --dry-run failed: %v\noutput:\n%s", err, out)
	}
	if contains(out, "1 invalid") {
		t.Errorf("a genuinely valid v2 sidecar must not be flagged invalid, got:\n%s", out)
	}
	if !contains(out, "1 already v2") {
		t.Errorf("expected the valid v2 sidecar to be counted as already v2, got:\n%s", out)
	}
}
