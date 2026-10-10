package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stageAgentDrift(t *testing.T) (ediktRoot, claudeRoot, projectRoot string) {
	t.Helper()
	ediktRoot, claudeRoot, projectRoot = t.TempDir(), t.TempDir(), t.TempDir()
	tmplDir := filepath.Join(ediktRoot, "templates", "agents")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tmpl := "---\nname: sidecar-extractor\nmaxTurns: 8\n---\ncurrent prompt\n"
	if err := os.WriteFile(filepath.Join(tmplDir, "sidecar-extractor.md"), []byte(tmpl), 0o644); err != nil {
		t.Fatal(err)
	}
	return
}

func TestAgentDrift_StaleUserLevelCopyWarns(t *testing.T) {
	ediktRoot, claudeRoot, projectRoot := stageAgentDrift(t)
	// Stale user-level copy: old maxTurns, old prompt — the exact field bug.
	userAgents := filepath.Join(claudeRoot, "agents")
	if err := os.MkdirAll(userAgents, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := "---\nname: sidecar-extractor\nmaxTurns: 3\n---\nold prompt\n"
	if err := os.WriteFile(filepath.Join(userAgents, "sidecar-extractor.md"), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	n := runAgentDriftCheck(ediktRoot, claudeRoot, projectRoot, &out)
	if n != 1 {
		t.Fatalf("expected 1 drift warning, got %d:\n%s", n, out.String())
	}
	for _, want := range []string{"sidecar-extractor", "caches agent definitions", "restart"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("warning must mention %q, got:\n%s", want, out.String())
		}
	}
}

func TestAgentDrift_MatchingAndCustomCopiesSilent(t *testing.T) {
	ediktRoot, claudeRoot, projectRoot := stageAgentDrift(t)
	projAgents := filepath.Join(projectRoot, ".claude", "agents")
	if err := os.MkdirAll(projAgents, 0o755); err != nil {
		t.Fatal(err)
	}
	// Byte-identical project copy → silent.
	tmplBody, _ := os.ReadFile(filepath.Join(ediktRoot, "templates", "agents", "sidecar-extractor.md"))
	if err := os.WriteFile(filepath.Join(projAgents, "sidecar-extractor.md"), tmplBody, 0o644); err != nil {
		t.Fatal(err)
	}
	// Diverged but explicitly custom user copy → silent.
	userAgents := filepath.Join(claudeRoot, "agents")
	if err := os.MkdirAll(userAgents, 0o755); err != nil {
		t.Fatal(err)
	}
	custom := "---\nname: sidecar-extractor\n---\n<!-- edikt:custom -->\nmy own prompt\n"
	if err := os.WriteFile(filepath.Join(userAgents, "sidecar-extractor.md"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if n := runAgentDriftCheck(ediktRoot, claudeRoot, projectRoot, &out); n != 0 {
		t.Fatalf("expected no warnings, got %d:\n%s", n, out.String())
	}
}

func TestAgentDrift_NoInstalledCopiesSilent(t *testing.T) {
	ediktRoot, claudeRoot, projectRoot := stageAgentDrift(t)
	var out bytes.Buffer
	if n := runAgentDriftCheck(ediktRoot, claudeRoot, projectRoot, &out); n != 0 {
		t.Fatalf("missing copies are not drift; got %d warnings", n)
	}
}

// ADR-085: sidecar-extractor.md's model: line is config-derived. An
// installed copy carrying model: <value> that matches the project's
// extractor.model config must not be reported as drift, even though it
// differs byte-for-byte from the (model-less) shipped template.
func TestAgentDrift_ModelKeyIsConfigDerived(t *testing.T) {
	ediktRoot, claudeRoot, projectRoot := stageAgentDrift(t)

	if err := os.MkdirAll(filepath.Join(projectRoot, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "extractor:\n  model: sonnet\n"
	if err := os.WriteFile(filepath.Join(projectRoot, ".edikt", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	projAgents := filepath.Join(projectRoot, ".claude", "agents")
	if err := os.MkdirAll(projAgents, 0o755); err != nil {
		t.Fatal(err)
	}
	// Exactly what RenderExtractorFrontmatter(template, "sonnet") produces:
	// the template's own bytes with model: sonnet appended to the
	// frontmatter (this fixture template has no prompt_version: line to
	// anchor after).
	rendered := "---\nname: sidecar-extractor\nmaxTurns: 8\nmodel: sonnet\n---\ncurrent prompt\n"
	if err := os.WriteFile(filepath.Join(projAgents, "sidecar-extractor.md"), []byte(rendered), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if n := runAgentDriftCheck(ediktRoot, claudeRoot, projectRoot, &out); n != 0 {
		t.Fatalf("config-consistent model: line reported as drift, got %d:\n%s", n, out.String())
	}
}

// The other half of the same guarantee: a model: line that does NOT match
// the resolved extractor.model config (here, the config default —
// inherit, which renders no model: key at all) is still real drift and
// must still be reported. Fixing the false positive must not create a
// false negative.
func TestAgentDrift_MismatchedModelKeyStillReportsDrift(t *testing.T) {
	ediktRoot, claudeRoot, projectRoot := stageAgentDrift(t)
	// No .edikt/config.yaml — extractor.model defaults to inherit, so the
	// rendered expectation carries no model: key at all.

	projAgents := filepath.Join(projectRoot, ".claude", "agents")
	if err := os.MkdirAll(projAgents, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := "---\nname: sidecar-extractor\nmaxTurns: 8\nmodel: sonnet\n---\ncurrent prompt\n"
	if err := os.WriteFile(filepath.Join(projAgents, "sidecar-extractor.md"), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if n := runAgentDriftCheck(ediktRoot, claudeRoot, projectRoot, &out); n != 1 {
		t.Fatalf("expected 1 drift warning for a model: pin the config no longer asks for, got %d:\n%s", n, out.String())
	}
}
