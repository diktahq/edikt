package phasea

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// D46 (b) / D47: after extraction, no entry may carry verify_kind behavioral
// unless both fixture files exist under the project root.
func TestVerifyNoBehavioralOverclaim(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sidecarPath := filepath.Join(dir, "ADR-800-x.edikt.yaml")
	body := `schema_version: 2
topic: "testing"
path: "docs/architecture/decisions/ADR-800-x.md"
signals:
  - "x"
directives:
  - text: "A rule MUST hold. (ref: ADR-800)"
    verify: "bash test/fixtures/behavioral/ADR-800/positive.sh"
    verify_kind: behavioral
    falsifying_observation: "the rule is violated and nothing notices"
    positive_fixture_path: test/fixtures/behavioral/ADR-800/positive.sh
    negative_fixture_path: test/fixtures/behavioral/ADR-800/negative.sh
    source_excerpts:
      - line_start: 1
        line_end: 1
        quote: "A rule holds."
prohibitions:
  - text: "MUST NOT break it. (ref: ADR-800)"
    verify: "bash test/x.sh"
    verify_kind: behavioral
    falsifying_observation: "it breaks"
    source_excerpts:
      - line_start: 1
        line_end: 1
        quote: "A rule holds."
`
	if err := os.WriteFile(sidecarPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	err := verifyNoBehavioralOverclaim(root, sidecarPath)
	if err == nil {
		t.Fatal("missing fixture files must be an over-claim")
	}
	msg := err.Error()
	for _, want := range []string{"directives[0]", "positive.sh does not exist", "negative.sh does not exist", "prohibitions[0]", "positive_fixture_path is empty"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("over-claim report lacks %q:\n%s", want, msg)
		}
	}

	// With both files present the directive is legitimate; the prohibition
	// still over-claims (no paths at all).
	fx := filepath.Join(root, "test", "fixtures", "behavioral", "ADR-800")
	if err := os.MkdirAll(fx, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"positive.sh", "negative.sh"} {
		if err := os.WriteFile(filepath.Join(fx, n), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	err = verifyNoBehavioralOverclaim(root, sidecarPath)
	if err == nil || strings.Contains(err.Error(), "directives[0]") || !strings.Contains(err.Error(), "prohibitions[0]") {
		t.Fatalf("only the pathless prohibition should remain an over-claim, got: %v", err)
	}
}

// D58 (b): an over-claim is a retryable content defect with its own axis,
// and its correction prompt carries the kind-by-shape rule and the entries.
func TestClassifyDefects_OverclaimAxisAndCorrection(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(dir, "ADR-801-x.md")
	if err := os.WriteFile(parent, []byte("# ADR-801\n\nA rule holds.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sidecarPath := filepath.Join(dir, "ADR-801-x.edikt.yaml")
	body := `schema_version: 2
topic: "testing"
path: "docs/architecture/decisions/ADR-801-x.md"
signals:
  - "x"
directives:
  - text: "A rule MUST hold. (ref: ADR-801)"
    verify: "go test ./internal/x/ -run TestRule"
    verify_kind: behavioral
    falsifying_observation: "the rule is violated"
    source_excerpts:
      - line_start: 3
        line_end: 3
        quote: "A rule holds."
`
	if err := os.WriteFile(sidecarPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := ClassifyDefectsIn(root, sidecarPath, parent)
	if rep.Axis != DefectAxisOverclaim || rep.Count() == 0 {
		t.Fatalf("expected the overclaim axis, got %+v", rep)
	}
	cp := rep.CorrectionPrompt()
	for _, want := range []string{"CORRECTION PASS", "directives[0]", "tooling", "behavioral ONLY when both", "A named test that proves the whole rule is still tooling"} {
		if !strings.Contains(cp, want) {
			t.Fatalf("correction prompt lacks %q:\n%s", want, cp)
		}
	}
	if r2 := ClassifyDefects(sidecarPath, parent); r2.Axis == DefectAxisOverclaim {
		t.Fatalf("no root → the over-claim axis cannot be judged; got %+v", r2)
	}
}
