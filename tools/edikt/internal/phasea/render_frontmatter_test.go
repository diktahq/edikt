package phasea

import (
	"strings"
	"testing"
)

const fixtureAgentBody = "---\n" +
	"name: sidecar-extractor\n" +
	"prompt_version: 8\n" +
	"effort: high\n" +
	"---\n\n" +
	"body text\n"

func TestRenderExtractorFrontmatter(t *testing.T) {
	t.Run("inherit produces no model key", func(t *testing.T) {
		got, err := RenderExtractorFrontmatter([]byte(fixtureAgentBody), ExtractorModelInherit)
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		if strings.Contains(string(got), "model:") {
			t.Errorf("inherit rendered a model: key:\n%s", got)
		}
		if !strings.Contains(string(got), "body text") {
			t.Error("body was dropped by rendering")
		}
	})

	t.Run("explicit value writes model: right after prompt_version:", func(t *testing.T) {
		got, err := RenderExtractorFrontmatter([]byte(fixtureAgentBody), "claude-opus-5")
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		want := "prompt_version: 8\nmodel: claude-opus-5\neffort: high"
		if !strings.Contains(string(got), want) {
			t.Errorf("rendered bytes missing expected sequence %q:\n%s", want, got)
		}
	})

	t.Run("explicit value replaces an existing model: line, not duplicates it", func(t *testing.T) {
		src := "---\nname: sidecar-extractor\nprompt_version: 8\nmodel: sonnet\neffort: high\n---\n\nbody\n"
		got, err := RenderExtractorFrontmatter([]byte(src), "claude-opus-5")
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		if n := strings.Count(string(got), "model:"); n != 1 {
			t.Errorf("expected exactly one model: line, got %d:\n%s", n, got)
		}
		if !strings.Contains(string(got), "model: claude-opus-5") {
			t.Errorf("did not resolve to the requested model:\n%s", got)
		}
	})

	t.Run("inherit strips a pre-existing model: line", func(t *testing.T) {
		src := "---\nname: sidecar-extractor\nprompt_version: 8\nmodel: sonnet\neffort: high\n---\n\nbody\n"
		got, err := RenderExtractorFrontmatter([]byte(src), ExtractorModelInherit)
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		if strings.Contains(string(got), "model:") {
			t.Errorf("inherit did not strip the existing model: line:\n%s", got)
		}
	})

	t.Run("round trip through explicit and back to inherit leaves no residue", func(t *testing.T) {
		explicit, err := RenderExtractorFrontmatter([]byte(fixtureAgentBody), "claude-haiku-4-5")
		if err != nil {
			t.Fatalf("render explicit: %v", err)
		}
		backToInherit, err := RenderExtractorFrontmatter(explicit, ExtractorModelInherit)
		if err != nil {
			t.Fatalf("render inherit: %v", err)
		}
		if strings.Contains(string(backToInherit), "model:") {
			t.Errorf("residue after round trip:\n%s", backToInherit)
		}
	})

	t.Run("malformed frontmatter is refused, not guessed at", func(t *testing.T) {
		if _, err := RenderExtractorFrontmatter([]byte("no fence here\n"), ExtractorModelInherit); err == nil {
			t.Error("expected an error for a missing frontmatter fence")
		}
		if _, err := RenderExtractorFrontmatter([]byte("---\nname: x\n"), ExtractorModelInherit); err == nil {
			t.Error("expected an error for an unterminated frontmatter fence")
		}
	})
}
