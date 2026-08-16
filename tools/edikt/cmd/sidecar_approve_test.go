package cmd

// sidecar_approve_test.go — exit-code contract tests for the
// `edikt sidecar approve` subcommand. Per ADR-039:
//   0 — success (or intentional no-op for --decision=defer)
//   1 — validation or IO error
//   2 — pending-id not found in .edikt/state/pending-verifies/
//   3 — invalid args (missing --decision, bad enum value, malformed
//       --edited-content path)
//
// AC-3.4 of SPEC-009 Plan B Phase 3.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withCWD swaps CWD to dir for the duration of the test, restoring on cleanup.
func withCWD(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(orig)
	})
}

// seedEdiktRoot creates a .edikt/state/pending-verifies/ scaffolding inside
// dir so resolvePendingVerifiesDir() anchors here.
func seedEdiktRoot(t *testing.T, dir string) string {
	t.Helper()
	pendingDir := filepath.Join(dir, ".edikt", "state", "pending-verifies")
	if err := os.MkdirAll(pendingDir, 0o755); err != nil {
		t.Fatalf("mkdir pending-verifies: %v", err)
	}
	return pendingDir
}

// TestSidecarApprove_ExitCodes — the four exit-code branches of ADR-039.
func TestSidecarApprove_ExitCodes(t *testing.T) {
	t.Run("exit2_pending_id_not_found", func(t *testing.T) {
		dir := t.TempDir()
		_ = seedEdiktRoot(t, dir)
		withCWD(t, dir)

		out, err := runCmd(t,
			"sidecar", "approve", "nonexistent-id-xyz",
			"--decision=approve",
		)
		if !isExitCode(err, 2) {
			t.Fatalf("want exit 2, got: %v\noutput: %s", err, out)
		}
		if !contains(out, "pending-id not found") {
			t.Errorf("expected 'pending-id not found' in output, got: %s", out)
		}
	})

	t.Run("exit3_missing_decision_flag", func(t *testing.T) {
		dir := t.TempDir()
		_ = seedEdiktRoot(t, dir)
		withCWD(t, dir)

		out, err := runCmd(t,
			"sidecar", "approve", "some-id",
		)
		if !isExitCode(err, 3) {
			t.Fatalf("want exit 3, got: %v\noutput: %s", err, out)
		}
		if !contains(out, "--decision is required") {
			t.Errorf("expected '--decision is required' in output, got: %s", out)
		}
	})

	t.Run("exit3_invalid_decision_value", func(t *testing.T) {
		dir := t.TempDir()
		_ = seedEdiktRoot(t, dir)
		withCWD(t, dir)

		out, err := runCmd(t,
			"sidecar", "approve", "some-id",
			"--decision=maybe",
		)
		if !isExitCode(err, 3) {
			t.Fatalf("want exit 3, got: %v\noutput: %s", err, out)
		}
		if !contains(strings.ToLower(out), "invalid value") {
			t.Errorf("expected 'invalid value' in output, got: %s", out)
		}
	})

	t.Run("exit3_edited_content_only_with_approve", func(t *testing.T) {
		dir := t.TempDir()
		_ = seedEdiktRoot(t, dir)
		withCWD(t, dir)

		// Write a benign file the flag could point at.
		editFile := filepath.Join(dir, "verify.sh")
		if err := os.WriteFile(editFile, []byte("true\n"), 0o644); err != nil {
			t.Fatalf("write edit file: %v", err)
		}

		out, err := runCmd(t,
			"sidecar", "approve", "some-id",
			"--decision=reject",
			"--edited-content="+editFile,
		)
		if !isExitCode(err, 3) {
			t.Fatalf("want exit 3, got: %v\noutput: %s", err, out)
		}
		if !contains(out, "only valid with --decision=approve") {
			t.Errorf("expected restriction message, got: %s", out)
		}
	})
}
