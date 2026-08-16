package cmd

// sidecar_approve_fixture_test.go — the bidirectional-fixture gate added
// after approving 17 real pending verifies broke `gov compile` the first
// time (Phase B's validatePhaseBConstraints refuses any behavioral
// directive with an empty positive_fixture_path/negative_fixture_path).
// Reproduces the exact shape of that break -- a sidecar + pending proposal
// with no fixture files on disk -- and asserts approve refuses BEFORE
// mutating anything, rather than accepting and leaving the corpus for
// `gov compile` to discover broken downstream.

import (
	"os"
	"path/filepath"
	"testing"
)

// seedApproveFixtureScenario writes a minimal sidecar with one directive
// and a matching pending-verify proposal for it, anchored at dir. Returns
// the pending-id.
func seedApproveFixtureScenario(t *testing.T, dir, artifactID string) string {
	t.Helper()
	_ = seedEdiktRoot(t, dir)

	sidecarPath := filepath.Join(dir, artifactID+"-example.edikt.yaml")
	sidecarBody := `schema_version: 2
topic: "testing"
path: "` + artifactID + `-example.md"
signals:
  - "example signal"
directives:
  - text: 'An example rule MUST hold. (ref: ` + artifactID + `)'
    source_excerpts:
      - line_start: 1
        line_end: 1
        quote: 'An example rule holds.'
`
	if err := os.WriteFile(sidecarPath, []byte(sidecarBody), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	pendingID := artifactID + "-d00"
	pendingPath := filepath.Join(dir, ".edikt", "state", "pending-verifies", pendingID+".yaml")
	pendingBody := `id: ` + pendingID + `
sidecar_path: ` + sidecarPath + `
directive_index: 0
proposed_verify: "true"
intent: "example intent"
falsifying_observation: "the example rule is violated"
proposed_at: "2026-08-14T00:00:00Z"
`
	if err := os.WriteFile(pendingPath, []byte(pendingBody), 0o644); err != nil {
		t.Fatalf("write pending verify: %v", err)
	}
	return pendingID
}

// TestSidecarApprove_RefusesWhenFixturesMissing is RED-BEFORE the fix:
// against the code as it stood when ADR-026-d03 broke gov compile tonight,
// this scenario would have exited 0 and written verify_kind: behavioral
// with empty fixture paths. It must now refuse (exit 1) and name what is
// missing, and must NOT touch the sidecar on disk.
func TestSidecarApprove_RefusesWhenFixturesMissing(t *testing.T) {
	dir := t.TempDir()
	pendingID := seedApproveFixtureScenario(t, dir, "ADR-999")
	withCWD(t, dir)

	sidecarPath := filepath.Join(dir, "ADR-999-example.edikt.yaml")
	before, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatalf("read sidecar before approve: %v", err)
	}

	out, runErr := runCmd(t, "sidecar", "approve", pendingID, "--decision=approve")
	if !isExitCode(runErr, 1) {
		t.Fatalf("want exit 1, got: %v\noutput: %s", runErr, out)
	}
	if !contains(out, "bidirectional fixture") {
		t.Errorf("expected the refusal to name the bidirectional fixture requirement, got: %s", out)
	}
	if !contains(out, "positive.sh") || !contains(out, "negative.sh") {
		t.Errorf("expected the refusal to name the missing fixture paths, got: %s", out)
	}

	after, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatalf("read sidecar after refused approve: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("sidecar was mutated despite the refusal:\nbefore: %s\nafter:  %s", before, after)
	}

	pendingPath := filepath.Join(dir, ".edikt", "state", "pending-verifies", pendingID+".yaml")
	if _, statErr := os.Stat(pendingPath); statErr != nil {
		t.Errorf("pending file should survive a refused approval, stat error: %v", statErr)
	}
}

// TestSidecarApprove_SucceedsWhenFixturesPresent is the control: the SAME
// scenario, but with both fixture files present, approves cleanly and
// records the resolved paths on the directive.
func TestSidecarApprove_SucceedsWhenFixturesPresent(t *testing.T) {
	dir := t.TempDir()
	pendingID := seedApproveFixtureScenario(t, dir, "ADR-998")
	withCWD(t, dir)

	fixtureDir := filepath.Join(dir, "test", "fixtures", "behavioral", "ADR-998")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "positive.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write positive fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "negative.sh"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write negative fixture: %v", err)
	}

	out, runErr := runCmd(t, "sidecar", "approve", pendingID, "--decision=approve")
	if runErr != nil {
		t.Fatalf("want exit 0, got: %v\noutput: %s", runErr, out)
	}
	if !contains(out, "ok: approved") {
		t.Errorf("expected success output, got: %s", out)
	}

	sidecarPath := filepath.Join(dir, "ADR-998-example.edikt.yaml")
	body, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatalf("read sidecar after approve: %v", err)
	}
	if !contains(string(body), "positive_fixture_path: test/fixtures/behavioral/ADR-998/positive.sh") {
		t.Errorf("expected positive_fixture_path recorded on the directive, got:\n%s", body)
	}
	if !contains(string(body), "negative_fixture_path: test/fixtures/behavioral/ADR-998/negative.sh") {
		t.Errorf("expected negative_fixture_path recorded on the directive, got:\n%s", body)
	}
}
