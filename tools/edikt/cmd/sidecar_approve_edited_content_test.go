package cmd

// sidecar_approve_edited_content_test.go — ONE contract for --edited-content
// across all three kinds of `edikt sidecar approve`.
//
// The flag was accepted, ignored, and reported success for --kind=paths: the
// invocation validated, mutated the sidecar, removed the pending file, printed
// "ok: approved", and dropped the human's correction with no signal. Silence is
// the worst of the three possible outcomes here — worse than honouring it and
// worse than refusing it — because the operator's evidence for "my edit
// landed" is exit 0, and exit 0 is exactly what they got.
//
// The three tests below are the whole contract, stated once per kind:
//
//	paths             REFUSES  (exit 3, nothing touched)
//	verify            HONOURS  (the file's contents become the verify command)
//	topic-description HONOURS  (the file's contents become the description)
//
// Every one drives the real binary through runCmd rather than calling the
// handler, because "exits 3" is a claim about the process, not about a return
// value that some caller might still translate into 0.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/topicregistry"
)

// TestSidecarApprove_PathsRefusesEditedContent is the defect, inverted.
//
// It is written so that removing the guard turns it RED rather than leaving it
// green: the three assertions after the exit code are the three observable
// effects of the OLD behaviour — exit 0, a consumed pending file, and a mutated
// sidecar. A test that checked only for the presence of an error message would
// pass against a guard that fired after the write, which is not a refusal.
func TestSidecarApprove_PathsRefusesEditedContent(t *testing.T) {
	root := scratchProject(t)
	t.Chdir(root)

	scPath := filepath.Join(root, "docs", "architecture", "decisions", "ADR-900-scratch.edikt.yaml")
	before, err := os.ReadFile(scPath)
	if err != nil {
		t.Fatalf("read sidecar before: %v", err)
	}

	// A proposal that WOULD approve cleanly: the glob matches real files in the
	// scratch tree. So if the command exits 0 here it is because the guard is
	// missing, not because something else refused.
	pending := writePendingPaths(t, root, "ADR-900", `id: ADR-900
sidecar_path: docs/architecture/decisions/ADR-900-scratch.edikt.yaml
proposed_at: "2026-09-06T00:00:00Z"
proposed_paths:
  - glob: "src/engine/**/*.go"
    evidence: "ADR-900's decision names the engine package as the thing that records state."
`)

	// The correction a human would supply — a narrower scope than the one
	// proposed. This is the content that must not vanish.
	edited := filepath.Join(root, "corrected-scope.yaml")
	mustWrite(t, edited, "proposed_paths:\n  - glob: \"src/engine/state.go\"\n    evidence: \"only the state file, not the whole package\"\n")

	out, runErr := runCmd(t, "sidecar", "approve", "ADR-900",
		"--kind=paths", "--decision=approve", "--edited-content="+edited)

	if !isExitCode(runErr, 3) {
		t.Fatalf("want exit 3 (invalid args) for --kind=paths --edited-content, got %v\noutput: %s", runErr, out)
	}
	if !contains(out, "--edited-content") {
		t.Errorf("the refusal must name the flag it is refusing, got: %s", out)
	}
	if !contains(out, "paths") {
		t.Errorf("the refusal must name the kind that cannot honour it, got: %s", out)
	}

	if _, statErr := os.Stat(pending); statErr != nil {
		t.Errorf("the pending file was consumed by a refused approval: %v", statErr)
	}
	after, err := os.ReadFile(scPath)
	if err != nil {
		t.Fatalf("read sidecar after: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("the sidecar was mutated by a refused approval:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestSidecarApprove_VerifyHonoursEditedContent is the no-regression half for
// the ADR-039 original: the contents of --edited-content become the approved
// verify command, replacing proposed_verify. Exercised end to end, not merely
// compiled — the assertion reads the mutated sidecar off disk.
func TestSidecarApprove_VerifyHonoursEditedContent(t *testing.T) {
	dir := t.TempDir()
	pendingID := seedApproveFixtureScenario(t, dir, "ADR-996")
	withCWD(t, dir)

	fixtureDir := filepath.Join(dir, "test", "fixtures", "behavioral", "ADR-996")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "positive.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write positive fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "negative.sh"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write negative fixture: %v", err)
	}

	// Deliberately unlike proposed_verify ("true"), so the assertion cannot
	// pass on the proposal's own value.
	const corrected = "test -f docs/the-corrected-verify.md"
	edited := filepath.Join(dir, "corrected-verify.sh")
	if err := os.WriteFile(edited, []byte(corrected+"\n"), 0o644); err != nil {
		t.Fatalf("write edited content: %v", err)
	}

	out, runErr := runCmd(t, "sidecar", "approve", pendingID,
		"--kind=verify", "--decision=approve", "--edited-content="+edited)
	if runErr != nil {
		t.Fatalf("want exit 0, got %v\noutput: %s", runErr, out)
	}

	body, err := os.ReadFile(filepath.Join(dir, "ADR-996-example.edikt.yaml"))
	if err != nil {
		t.Fatalf("read sidecar after approve: %v", err)
	}
	if !contains(string(body), corrected) {
		t.Errorf("the edited verify did not reach the sidecar — --edited-content was dropped:\n%s", body)
	}
	if contains(string(body), "verify: \"true\"") || contains(string(body), "verify: true") {
		t.Errorf("proposed_verify survived instead of the edited content:\n%s", body)
	}
}

// TestSidecarApprove_TopicDescriptionHonoursEditedContent is the same
// no-regression assertion for the registry kind: the contents of
// --edited-content become the approved description AND the hash is stamped
// over those bytes, not over the proposed ones. A hash covering the proposal
// would certify a line nobody approved, which is the whole reason the field
// exists.
func TestSidecarApprove_TopicDescriptionHonoursEditedContent(t *testing.T) {
	root := t.TempDir()
	pendingDir := filepath.Join(root, ".edikt", "state", "pending-topic-descriptions")
	mustMkAll(t, pendingDir)
	mustWrite(t, filepath.Join(pendingDir, "compile.yaml"), `topic: compile
description: "the machine's own guess at when this topic is needed"
evidence: "derived from the corpus"
proposed_at: "2026-09-06T00:00:00Z"
`)
	withCWD(t, root)

	const corrected = "Deciding how a rule reaches the model, or changing it."
	edited := filepath.Join(root, "corrected-description.txt")
	mustWrite(t, edited, corrected+"\n")

	out, runErr := runCmd(t, "sidecar", "approve", "compile",
		"--kind=topic-description", "--decision=approve", "--edited-content="+edited)
	if runErr != nil {
		t.Fatalf("want exit 0, got %v\noutput: %s", runErr, out)
	}

	reg, err := topicregistry.Load(topicregistry.PathFor(root))
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	entry, ok := reg["compile"]
	if !ok {
		t.Fatalf("no registry entry for topic compile: %+v", reg)
	}
	if entry.Description != corrected {
		t.Errorf("description = %q, want the edited content %q — --edited-content was dropped",
			entry.Description, corrected)
	}
	if entry.DescriptionHash != topicregistry.HashDescription(corrected) {
		t.Errorf("description_hash covers bytes other than the ones approved: got %s, want %s",
			entry.DescriptionHash, topicregistry.HashDescription(corrected))
	}
}
