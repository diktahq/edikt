package phaseb

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

func mkPair(t *testing.T, projectRoot, basename, topic string, directives []sidecar.Directive) sidecar.Pair {
	t.Helper()
	dir := filepath.Join(projectRoot, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	parentPath := filepath.Join(dir, basename+".md")
	sidecarPath := filepath.Join(dir, basename+".edikt.yaml")
	if err := os.WriteFile(parentPath, []byte("# placeholder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sc := &sidecar.Sidecar{
		SchemaVersion: 1,
		Topic:         topic,
		Path:          "docs/architecture/decisions/" + basename + ".md",
		Signals:       []string{"x"},
		Directives:    directives,
		SourcePath:    sidecarPath,
	}
	if err := os.WriteFile(sidecarPath, []byte("placeholder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return sidecar.Pair{
		ParentPath:  parentPath,
		SidecarPath: sidecarPath,
		ArtifactID:  basename[:7], // "ADR-001"
		Sidecar:     sc,
	}
}

func TestMerge_WritesTopicAndIndex(t *testing.T) {
	root := t.TempDir()
	pairs := []sidecar.Pair{
		mkPair(t, root, "ADR-001-test", "architecture", []sidecar.Directive{
			{Text: "First directive. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		}),
	}
	res, err := Merge(root, pairs, Options{CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test"})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if !res.IndexWritten {
		t.Error("expected index to be written on first run")
	}
	if len(res.TopicsRendered) != 1 || res.TopicsRendered[0] != "architecture" {
		t.Errorf("expected ['architecture'] rendered, got %v", res.TopicsRendered)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "rules", "governance", "architecture.md")); err != nil {
		t.Errorf("topic file missing: %v", err)
	}
}

func TestMerge_IsIdempotent(t *testing.T) {
	root := t.TempDir()
	pairs := []sidecar.Pair{
		mkPair(t, root, "ADR-001-test", "architecture", []sidecar.Directive{
			{Text: "Same directive. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		}),
	}
	opts := Options{CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test"}

	if _, err := Merge(root, pairs, opts); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance", "architecture.md"))
	if err != nil {
		t.Fatal(err)
	}

	res2, err := Merge(root, pairs, opts)
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance", "architecture.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("byte-equal input must produce byte-equal output")
	}
	if len(res2.TopicsRendered) != 0 {
		t.Errorf("idempotent rerun should report 0 rendered, got %v", res2.TopicsRendered)
	}
	if len(res2.TopicsUnchanged) != 1 {
		t.Errorf("idempotent rerun should report 1 unchanged, got %v", res2.TopicsUnchanged)
	}
}

func TestMerge_OnlyAffectedTopicChanges(t *testing.T) {
	root := t.TempDir()
	pairs := []sidecar.Pair{
		mkPair(t, root, "ADR-001-test", "alpha", []sidecar.Directive{
			{Text: "Alpha directive. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		}),
		mkPair(t, root, "ADR-002-test", "beta", []sidecar.Directive{
			{Text: "Beta directive. (ref: ADR-002)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		}),
	}
	opts := Options{CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test"}

	if _, err := Merge(root, pairs, opts); err != nil {
		t.Fatal(err)
	}
	betaPath := filepath.Join(root, ".claude", "rules", "governance", "beta.md")
	betaBefore, _ := os.ReadFile(betaPath)

	pairs[0].Sidecar.Directives[0].Text = "Alpha modified. (ref: ADR-001)"
	res, err := Merge(root, pairs, opts)
	if err != nil {
		t.Fatal(err)
	}
	betaAfter, _ := os.ReadFile(betaPath)

	if string(betaBefore) != string(betaAfter) {
		t.Error("beta topic changed unexpectedly when only alpha sidecar mutated")
	}
	rendered := map[string]bool{}
	for _, n := range res.TopicsRendered {
		rendered[n] = true
	}
	if !rendered["alpha"] {
		t.Errorf("alpha must be rendered, got %v", res.TopicsRendered)
	}
	if rendered["beta"] {
		t.Errorf("beta must NOT be re-rendered, got %v", res.TopicsRendered)
	}
}
