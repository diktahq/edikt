package phasea

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// D40 — the resolver enumerates every candidate and says which one it
// selected; a refusal names every path it looked at, never firstErr alone.

func writeProfileAgent(t *testing.T, body string) string {
	t.Helper()
	claudeHome := t.TempDir()
	p := filepath.Join(claudeHome, "agents", "sidecar-extractor.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_HOME", claudeHome)
	return p
}

func TestResolvePromptVersion_NotFoundListsEveryCandidate(t *testing.T) {
	root := t.TempDir()
	claudeHome := t.TempDir()
	t.Setenv("CLAUDE_HOME", claudeHome)
	got, _, err := ResolveExtractorPromptVersionDetail(root)
	if err == nil || got != ExtractorPromptVersionUnknown {
		t.Fatalf("expected refusal, got %q err=%v", got, err)
	}
	msg := err.Error()
	for _, want := range []string{"extractor definition not found; looked at:", filepath.Join(root, ExtractorAgentRelPath), filepath.Join(claudeHome, "agents", "sidecar-extractor.md")} {
		if !strings.Contains(msg, want) {
			t.Fatalf("refusal %q does not name %q", msg, want)
		}
	}
}

func TestResolvePromptVersion_FoundButUnpinnedSaysSo(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, "---\nname: sidecar-extractor\nmodel: sonnet\n---\n")
	t.Setenv("CLAUDE_HOME", t.TempDir())
	got, _, err := ResolveExtractorPromptVersionDetail(root)
	if err == nil || got != ExtractorPromptVersionUnknown {
		t.Fatalf("expected refusal, got %q err=%v", got, err)
	}
	want := "definition found at " + filepath.Join(root, ExtractorAgentRelPath) + " but carries no prompt_version"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("refusal %q does not carry %q", err.Error(), want)
	}
	if strings.Contains(err.Error(), "not found; looked at") {
		t.Fatalf("a found-but-unpinned definition must not be reported as not found: %q", err.Error())
	}
}

func TestResolvePromptVersion_ProjectPinnedProfileUnpinnedSelectsProjectAndSaysSo(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, "---\nname: sidecar-extractor\nprompt_version: 4\n---\n")
	profile := writeProfileAgent(t, "---\nname: sidecar-extractor\nmodel: sonnet\n---\n")
	got, note, err := ResolveExtractorPromptVersionDetail(root)
	if err != nil || got != "v4" {
		t.Fatalf("got %q err=%v, want v4", got, err)
	}
	for _, want := range []string{"found 2 definitions", "selected " + filepath.Join(root, ExtractorAgentRelPath) + " (project overrides profile)", profile + " carries no prompt_version"} {
		if !strings.Contains(note, want) {
			t.Fatalf("note %q lacks %q", note, want)
		}
	}
}

func TestResolvePromptVersion_TwoPinnedCopiesDisagreeRefuses(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, "---\nname: sidecar-extractor\nprompt_version: 4\n---\n")
	profile := writeProfileAgent(t, "---\nname: sidecar-extractor\nprompt_version: 3\n---\n")
	got, _, err := ResolveExtractorPromptVersionDetail(root)
	if err == nil || got != ExtractorPromptVersionUnknown {
		t.Fatalf("two pinned copies at different versions must refuse; got %q err=%v", got, err)
	}
	for _, want := range []string{"conflict", "v4", "v3", profile} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal %q lacks %q", err.Error(), want)
		}
	}
}

func TestResolvePromptVersion_TwoPinnedCopiesAgreeSelectsProject(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, "---\nname: sidecar-extractor\nprompt_version: 4\n---\n")
	writeProfileAgent(t, "---\nname: sidecar-extractor\nprompt_version: 4\n---\n")
	got, note, err := ResolveExtractorPromptVersionDetail(root)
	if err != nil || got != "v4" || !strings.Contains(note, "project overrides profile") {
		t.Fatalf("got %q note=%q err=%v", got, note, err)
	}
}
