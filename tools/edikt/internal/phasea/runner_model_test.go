package phasea

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubCLI writes an executable that records its own argv to argvFile and
// then creates the sidecar the runner stats for, so Resync reaches its
// success path. Using a real executable keeps this a genuine exec — the
// argv asserted below is the argv the operating system received, not a
// value read back out of a struct field.
func stubCLI(t *testing.T, argvFile, sidecarPath string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-claude")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > " + argvFile + "\n" +
		// A MINIMAL VALID sidecar, not just parseable bytes. Resync now
		// rejects output that will not load (D20), and a topic-less file is
		// not something the extractor ever produces — pinning a fixture
		// shape production cannot emit is what this repo keeps getting
		// caught by.
		"printf 'schema_version: 1\\ntopic: testing\\npath: docs/x.md\\nsignals:\\n  - testing\\ndirectives: []\\n' > " + sidecarPath + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return bin
}

func runResync(t *testing.T, r *ClaudeRunner) []string {
	t.Helper()
	work := t.TempDir()
	argvFile := filepath.Join(work, "argv.txt")
	sidecar := filepath.Join(work, "ADR-001-test.edikt.yaml")
	parent := filepath.Join(work, "ADR-001-test.md")
	if err := os.WriteFile(parent, []byte("# ADR-001\n"), 0o644); err != nil {
		t.Fatalf("write parent: %v", err)
	}
	r.Binary = stubCLI(t, argvFile, sidecar)

	if err := r.Resync(context.Background(), Task{
		ArtifactType: "adr",
		ArtifactID:   "ADR-001",
		ParentPath:   parent,
		SidecarPath:  sidecar,
	}); err != nil {
		t.Fatalf("resync: %v", err)
	}
	raw, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("stub recorded no argv: %v", err)
	}
	return strings.Fields(strings.TrimSpace(string(raw)))
}

// TestResync_PinsTheModel is the core defect.
//
// The dispatch was `exec.CommandContext(ctx, bin, "-p", prompt)` — no
// --model at all. Every extraction the project has ever run, including the
// banked greenfield baseline, was produced by whatever model the CLI
// happened to default to at that moment. That makes every measurement
// taken so far unattributable: a change in extraction quality between two
// runs cannot be assigned to the prompt, the corpus, or a silent model
// change underneath both.
//
// Pinning does not retroactively attribute the baseline. It makes every
// run from here on attributable, which is the precondition for measuring
// the extractor fix at all.
func TestResync_PinsTheModel(t *testing.T) {
	t.Setenv("EDIKT_EXTRACTOR_MODEL", "")
	argv := runResync(t, &ClaudeRunner{})

	idx := -1
	for i, a := range argv {
		if a == "--model" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("dispatch carries no --model flag; the model is whatever the CLI defaults to.\nargv: %v", argv)
	}
	if idx+1 >= len(argv) {
		t.Fatalf("--model passed with no value:\nargv: %v", argv)
	}
	if got := argv[idx+1]; got != DefaultExtractorModel {
		t.Errorf("--model = %q, want the pinned default %q", got, DefaultExtractorModel)
	}
}

// TestResync_ModelIsConfigurable pins the override path. A pin nobody can
// change is a pin that gets edited in source and diverges per checkout.
func TestResync_ModelIsConfigurable(t *testing.T) {
	t.Setenv("EDIKT_EXTRACTOR_MODEL", "claude-sonnet-5")
	argv := runResync(t, &ClaudeRunner{})

	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "--model claude-sonnet-5") {
		t.Errorf("EDIKT_EXTRACTOR_MODEL did not reach argv:\nargv: %v", argv)
	}
}

// TestResync_RejectsUnsafeModel keeps the override inside INV-006. The
// value comes from the environment, so it is externally-controlled and
// flows into an argv element — exactly the shape the invariant covers.
func TestResync_RejectsUnsafeModel(t *testing.T) {
	t.Setenv("EDIKT_EXTRACTOR_MODEL", "claude-opus-5; rm -rf /")

	work := t.TempDir()
	sidecar := filepath.Join(work, "ADR-001-test.edikt.yaml")
	parent := filepath.Join(work, "ADR-001-test.md")
	if err := os.WriteFile(parent, []byte("# ADR-001\n"), 0o644); err != nil {
		t.Fatalf("write parent: %v", err)
	}
	r := &ClaudeRunner{Binary: stubCLI(t, filepath.Join(work, "argv.txt"), sidecar)}

	err := r.Resync(context.Background(), Task{
		ArtifactType: "adr", ArtifactID: "ADR-001",
		ParentPath: parent, SidecarPath: sidecar,
	})
	if err == nil {
		t.Fatal("a shell-metacharacter model id was accepted")
	}
	if !strings.Contains(err.Error(), "invalid model id") {
		t.Errorf("error does not name the rejected value: %v", err)
	}
	if _, serr := os.Stat(filepath.Join(work, "argv.txt")); serr == nil {
		t.Error("the CLI was executed despite an invalid model id — validation must precede dispatch")
	}
}

// TestResolveExtractorModel_Precedence pins the resolution order so a
// caller can predict which value wins without reading the implementation.
func TestResolveExtractorModel_Precedence(t *testing.T) {
	t.Setenv("EDIKT_EXTRACTOR_MODEL", "claude-sonnet-5")

	got, err := ResolveExtractorModel("claude-haiku-4-5")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "claude-haiku-4-5" {
		t.Errorf("explicit argument must outrank the environment; got %q", got)
	}

	if got, err = ResolveExtractorModel(""); err != nil || got != "claude-sonnet-5" {
		t.Errorf("environment must outrank the default; got %q err %v", got, err)
	}

	t.Setenv("EDIKT_EXTRACTOR_MODEL", "")
	if got, err = ResolveExtractorModel(""); err != nil || got != DefaultExtractorModel {
		t.Errorf("default must apply when nothing is set; got %q err %v", got, err)
	}
}
