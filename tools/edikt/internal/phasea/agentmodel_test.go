package phasea

import (
	"os"
	"path/filepath"
	"testing"
)

func writeAgent(t *testing.T, root, body string) {
	t.Helper()
	p := filepath.Join(root, ExtractorAgentRelPath)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The pin is read from the agent frontmatter — the value that actually
// governs which model extracts (ADR-055 §1).
func TestResolveExtractorAgentModel_ReadsFrontmatter(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, "---\nname: sidecar-extractor\nmodel: sonnet\neffort: high\n---\n\nbody\n")

	got, err := ResolveExtractorAgentModel(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "sonnet" {
		t.Fatalf("got %q, want %q", got, "sonnet")
	}
}

// THE REGRESSION THIS FILE EXISTS FOR (D27), now guarded structurally rather
// than behaviorally.
//
// D27 was: the CLI --model value (a pin on the DISPATCHING session) leaked
// into the reported extractor model. ADR-085 removed the dispatching-session
// pin entirely — its model constant, its env-var override, and the resolver
// that combined them are gone, since no code dispatches the extractor via
// a CLI --model flag any more (the two-phase split moved dispatch to tier-1
// markdown via the host Agent-tool primitive). With no second pin left to
// leak from, the regression this file existed for has no mechanism to
// recur: ResolveExtractorAgentModel below has exactly one source, the
// installed agent frontmatter, and nothing else in this package can supply
// a competing value.

// INV-013 / ADR-055 §4: an unresolvable pin reports UNKNOWN and never falls
// back. Each case supplies a different reason the model cannot be determined,
// because "returns UNKNOWN" is only meaningful if it is reached for the right
// reason rather than by a single early return.
func TestResolveExtractorAgentModel_UnknownNeverFallsBack(t *testing.T) {
	cases := []struct {
		name string
		body string // "" means: do not write the file at all
		skip bool
	}{
		{name: "agent file absent", skip: true},
		{name: "no frontmatter fence", body: "no fence here\n"},
		{name: "unterminated fence", body: "---\nmodel: sonnet\n"},
		{name: "frontmatter without model", body: "---\nname: sidecar-extractor\n---\n"},
		{name: "empty model value", body: "---\nname: x\nmodel: \"\"\n---\n"},
		{name: "model fails shape validation", body: "---\nmodel: \"has spaces/and;semis\"\n---\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if !tc.skip {
				writeAgent(t, root, tc.body)
			}
			// F4/F5: ResolveExtractorAgentModel also falls back to the
			// active Claude profile's agents/ dir. Point CLAUDE_HOME at an
			// empty sandbox so this test's "nothing resolves" premise holds
			// regardless of what the machine running it actually has
			// installed under its real Claude config.
			t.Setenv("CLAUDE_HOME", t.TempDir())

			got, err := ResolveExtractorAgentModel(root)
			if err == nil {
				t.Fatalf("expected an error, got model %q", got)
			}
			if got != ExtractorModelUnknown {
				t.Fatalf("got %q, want %q", got, ExtractorModelUnknown)
			}
		})
	}
}

// F4/F5 — docs/internal/issues/agentmodel-resolver-no-global-fallback.md.
// A project with no project-local .claude/agents/sidecar-extractor.md must
// still resolve when the agent is installed under the active Claude
// profile's own agents/ dir — the same location doctor's agent-drift check
// already looks at. Before this fix, both resolvers were project-local
// only and returned a hard error here.
func TestResolveExtractorAgentModel_FallsBackToClaudeRoot(t *testing.T) {
	root := t.TempDir() // no .claude/agents/ under here at all
	claudeHome := t.TempDir()
	agentPath := filepath.Join(claudeHome, "agents", "sidecar-extractor.md")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentPath, []byte("---\nname: sidecar-extractor\nmodel: sonnet\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_HOME", claudeHome)

	got, err := ResolveExtractorAgentModel(root)
	if err != nil {
		t.Fatalf("expected fallback resolution to succeed, got error: %v", err)
	}
	if got != "sonnet" {
		t.Fatalf("got %q, want %q", got, "sonnet")
	}
}

func TestResolveExtractorPromptVersion_FallsBackToClaudeRoot(t *testing.T) {
	root := t.TempDir()
	claudeHome := t.TempDir()
	agentPath := filepath.Join(claudeHome, "agents", "sidecar-extractor.md")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentPath, []byte("---\nname: sidecar-extractor\nprompt_version: 3\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_HOME", claudeHome)

	got, err := ResolveExtractorPromptVersion(root)
	if err != nil {
		t.Fatalf("expected fallback resolution to succeed, got error: %v", err)
	}
	if got != "v3" {
		t.Fatalf("got %q, want %q", got, "v3")
	}
}

// Project-local must still win when BOTH locations resolve — the fallback
// is a last resort, not a competing source of truth.
func TestResolveExtractorAgentModel_ProjectLocalTakesPrecedenceOverFallback(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, "---\nname: sidecar-extractor\nmodel: opus\n---\n")

	claudeHome := t.TempDir()
	agentPath := filepath.Join(claudeHome, "agents", "sidecar-extractor.md")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentPath, []byte("---\nname: sidecar-extractor\nmodel: sonnet\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_HOME", claudeHome)

	got, err := ResolveExtractorAgentModel(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "opus" {
		t.Fatalf("got %q, want project-local %q (fallback must not shadow project-local)", got, "opus")
	}
}

// Inverted under ADR-085 Decision 2: the shipped template now carries no
// model: key at all, so extractor.model's default (inherit) is honored
// with no per-project edit. A model key reappearing here would mean the
// template regressed to a hardcoded pin, which is exactly what this ADR
// removed — the decision and the artifact must not drift apart silently
// in either direction.
func TestShippedExtractorTemplateDeclaresNoModel(t *testing.T) {
	root := t.TempDir()
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "..",
		"templates", "agents", "sidecar-extractor.md"))
	if err != nil {
		t.Skipf("shipped template not reachable from here: %v", err)
	}
	writeAgent(t, root, string(src))

	got, err := ResolveExtractorAgentModel(root)
	if err == nil {
		t.Fatalf("shipped template unexpectedly resolved to a pinned model %q — it must carry no model: key under inherit", got)
	}
	if got != ExtractorModelUnknown {
		t.Fatalf("got %q, want %q", got, ExtractorModelUnknown)
	}
}
