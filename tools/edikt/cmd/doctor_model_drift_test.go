package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// stageModelDrift writes an accepted ADR, an extractor agent definition
// pinning agentModel, and a sidecar whose extractor_model is pinned to
// recordedModel — independent of the agent's current pin, the way a
// sidecar extracted under a since-changed model would be.
func stageModelDrift(t *testing.T, agentModel, recordedModel string) string {
	t.Helper()
	root := t.TempDir()

	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	quote := "The extractor MUST quote the artifact byte-exactly."
	body := "# Title\n\n**Status:** Accepted\n\n" + quote + "\n" // quote sits at line 5
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-900-test.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	agentPath := filepath.Join(root, ".claude", "agents", "sidecar-extractor.md")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	agentBody := "---\nname: sidecar-extractor\nmodel: " + agentModel + "\nprompt_version: 4\n---\n\nbody\n"
	if err := os.WriteFile(agentPath, []byte(agentBody), 0o644); err != nil {
		t.Fatal(err)
	}

	sc := &sidecar.Sidecar{
		SchemaVersion:  2,
		Topic:          "testing",
		Path:           "docs/architecture/decisions/ADR-900-test.md",
		ExtractorModel: recordedModel,
		Directives: []sidecar.Directive{{
			Text:           "The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)",
			SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 5, LineEnd: 5, Quote: quote}},
		}},
	}
	scBytes, merr := sidecar.Marshal(sc)
	if merr != nil {
		t.Fatal(merr)
	}
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-900-test.edikt.yaml"), scBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// AC-7.5 — a sidecar's extractor_model no longer matching the currently-
// resolved extractor model is reported as INFORMATION, naming both models,
// and never counted against doctor's warn/error tally.
func TestModelDriftReported(t *testing.T) {
	root := stageModelDrift(t, "claude-sonnet-5", "claude-opus-5")

	notes := checkExtractorModelDrift(root)
	if len(notes) != 1 {
		t.Fatalf("want 1 drift note, got %d: %+v", len(notes), notes)
	}
	if notes[0].Extracted != "claude-opus-5" || notes[0].Current != "claude-sonnet-5" {
		t.Fatalf("note = %+v, want extracted=claude-opus-5 current=claude-sonnet-5", notes[0])
	}

	var out bytes.Buffer
	runExtractorModelDriftCheck(root, &out)
	got := out.String()
	if !strings.Contains(got, "INFO:") {
		t.Fatalf("drift must be reported as INFO, got:\n%s", got)
	}
	if !strings.Contains(got, "claude-opus-5") || !strings.Contains(got, "claude-sonnet-5") {
		t.Fatalf("report must name both models, got:\n%s", got)
	}
}

// Control: no drift when the sidecar's recorded model still matches the
// resolved agent's current pin.
func TestModelDriftReported_SilentWhenUnchanged(t *testing.T) {
	root := stageModelDrift(t, "claude-sonnet-5", "claude-sonnet-5")

	if notes := checkExtractorModelDrift(root); len(notes) != 0 {
		t.Fatalf("want no drift when the model is unchanged, got: %+v", notes)
	}

	var out bytes.Buffer
	runExtractorModelDriftCheck(root, &out)
	if out.Len() != 0 {
		t.Fatalf("want no output when the model is unchanged, got:\n%s", out.String())
	}
}
