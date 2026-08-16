package phasea

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubClaude writes an executable shell stub standing in for the claude CLI.
func stubClaude(t *testing.T, script string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// Field bug (bok-services): stale agent definitions made the extractor exit 0
// with an empty response and zero files written; the dispatcher recorded
// success. The runner must verify the sidecar landed on disk.
func TestClaudeRunner_Resync_FailsWhenNoFileWritten(t *testing.T) {
	dir := t.TempDir()
	task := Task{
		ArtifactType: "adr",
		ArtifactID:   "ADR-001",
		ParentPath:   filepath.Join(dir, "ADR-001-x.md"),
		SidecarPath:  filepath.Join(dir, "ADR-001-x.edikt.yaml"),
	}
	r := &ClaudeRunner{Binary: stubClaude(t, "exit 0")}
	err := r.Resync(context.Background(), task)
	if err == nil {
		t.Fatal("exit-0-no-file dispatch must be reported as a failure")
	}
	if !strings.Contains(err.Error(), "zero-file dispatch") {
		t.Errorf("error must name the zero-file failure mode, got: %v", err)
	}
}

func TestClaudeRunner_Resync_FailsWhenExistingFileUntouched(t *testing.T) {
	dir := t.TempDir()
	sc := filepath.Join(dir, "ADR-001-x.edikt.yaml")
	if err := os.WriteFile(sc, []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	task := Task{ArtifactType: "adr", ArtifactID: "ADR-001", ParentPath: filepath.Join(dir, "ADR-001-x.md"), SidecarPath: sc}
	r := &ClaudeRunner{Binary: stubClaude(t, "exit 0")}
	if err := r.Resync(context.Background(), task); err == nil {
		t.Fatal("exit-0-untouched-file dispatch must be reported as a failure")
	}
}

func TestClaudeRunner_Resync_SucceedsWhenFileWritten(t *testing.T) {
	dir := t.TempDir()
	sc := filepath.Join(dir, "ADR-001-x.edikt.yaml")
	task := Task{ArtifactType: "adr", ArtifactID: "ADR-001", ParentPath: filepath.Join(dir, "ADR-001-x.md"), SidecarPath: sc}
	r := &ClaudeRunner{Binary: stubClaude(t, "printf 'schema_version: 1\\ntopic: testing\\npath: docs/x.md\\nsignals:\\n  - testing\\ndirectives: []\\n' > "+sc)}
	if err := r.Resync(context.Background(), task); err != nil {
		t.Fatalf("file-writing dispatch must succeed, got: %v", err)
	}
}

// D45 call site (a) SENSITIVITY. A gate that only ever sees valid output is
// untested no matter how green it is (GL-002). This writes a sidecar that
// PARSES as YAML but violates the schema — the exact gap between the old parse
// gate and this one — and asserts the dispatch is refused and rolled back.
//
// Without this, extending the gate could be a no-op and every test would pass.
func TestClaudeRunner_Resync_RejectsSchemaInvalidButParseableSidecar(t *testing.T) {
	dir := t.TempDir()
	sc := filepath.Join(dir, "ADR-001-x.edikt.yaml")
	// Valid YAML. Schema-invalid: `signals` is required.
	body := "schema_version: 1\\ntopic: testing\\npath: docs/x.md\\ndirectives: []\\n"
	r := &ClaudeRunner{Binary: stubClaude(t, "printf '"+body+"' > "+sc)}
	err := r.Resync(context.Background(), Task{
		ArtifactType: "adr", ArtifactID: "ADR-001", SidecarPath: sc,
	})
	if err == nil {
		t.Fatal("dispatch producing schema-invalid (but parseable) YAML was accepted — " +
			"the generation-boundary gate is not validating against the schema")
	}
	if _, statErr := os.Stat(sc); statErr == nil {
		t.Fatal("schema-invalid sidecar survived on disk — it was not rolled back")
	}
}
