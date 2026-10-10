package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Sentinel markers built at runtime so this source file does not contain a
// literal in-body managed region (which the pre-tool-use hook would block).
const (
	liftOpenMarker  = "[edikt:dir" + "ectives:start]: #"
	liftCloseMarker = "[edikt:dir" + "ectives:end]: #"
)

// The one lifter test that drives the built binary rather than RunLift: it
// stays in cmd because it exercises the cobra verb (`migrate lift`, and its
// `sidecars` alias) end to end. The rest moved to internal/migrate with the
// lifter (ADR-082 (c)).
// TestMigrateSidecars_JSONFlag pins ADR-029 / Phase 5 of
// PLAN-sidecar-review-fixes finding #29: migrate sidecars accepts --json
// and emits a single JSON document on stdout. The dry-run helper text is
// routed to stderr.
func TestMigrateSidecars_JSONFlag(t *testing.T) {
	bin := buildBinary(t)

	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "docs/architecture/decisions"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "# ADR-001 — example\n\n" +
		"## Decision\n\nA test directive. (ref: INV-003)\n\n" +
		liftOpenMarker + "\n" +
		"source_hash: abc\n" +
		"di" + "rectives_hash: def\n" +
		"topic: hooks\n" +
		"signals:\n  - hook\n" +
		"di" + "rectives:\n  - \"A test directive. (ref: INV-003)\"\n" +
		liftCloseMarker + "\n"
	if err := os.WriteFile(filepath.Join(work, "docs/architecture/decisions/ADR-001-test.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write adr: %v", err)
	}

	cmd := exec.Command(bin, "migrate", "lift", "--dry-run", "--json")
	cmd.Dir = work
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	// stdout MUST be a single parseable JSON document with the documented shape.
	var parsed struct {
		Status  string         `json:"status"`
		Mode    string         `json:"mode"`
		Summary map[string]int `json:"summary"`
		Items   []struct {
			Source  string `json:"source"`
			Sidecar string `json:"sidecar"`
			Action  string `json:"action"`
		} `json:"items"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("--json output not parseable: %v\nstdout:\n%s", err, stdout.String())
	}
	if parsed.Status != "ok" {
		t.Fatalf("status: want ok, got %q", parsed.Status)
	}
	if parsed.Mode != "dry-run" {
		t.Fatalf("mode: want dry-run, got %q", parsed.Mode)
	}
	if parsed.Summary["to_create"] != 1 {
		t.Fatalf("summary.to_create: want 1, got %d", parsed.Summary["to_create"])
	}
	if len(parsed.Items) != 1 || parsed.Items[0].Source != "ADR-001-test.md" {
		t.Fatalf("items[]: want one item for ADR-001-test.md; got %+v", parsed.Items)
	}

	// Prose progress lines MUST go to stderr in --json mode.
	if !strings.Contains(stderr.String(), "migrate lift (dry-run):") {
		t.Fatalf("expected prose progress on stderr; got:\n%s", stderr.String())
	}
}

// TestMigrateSidecars_CarriesOptionalFields verifies the upgrade regression fix:
// applyArtifact MUST carry manual_directives, suppressed_directives, reminders,
// and verification from the old sentinel block into the new .edikt.yaml sidecar.
// A v0.4.3 → v0.6.0 upgrade that drops these fields silently destroys user
// authored overrides, which is the primary upgrade regression risk.
