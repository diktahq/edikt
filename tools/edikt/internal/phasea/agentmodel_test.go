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

// THE REGRESSION THIS FILE EXISTS FOR (D27).
//
// The CLI --model value must never become the reported extractor model. This
// asserts the two are independent: with EDIKT_EXTRACTOR_MODEL set to a value
// that resolves fine for the DISPATCHING session, the extractor model is still
// whatever the agent frontmatter says.
//
// Without this, setting the env var to claude-opus-5 while the agent pins
// sonnet reproduces the exact defect — a confident wrong label — and nothing
// would notice.
func TestResolveExtractorAgentModel_IgnoresCLIAndEnvPin(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, "---\nname: sidecar-extractor\nmodel: sonnet\n---\n")
	t.Setenv(ExtractorModelEnv, "claude-opus-5")

	// The dispatching-session pin resolves to the env value...
	session, err := ResolveExtractorModel("")
	if err != nil {
		t.Fatalf("session pin: %v", err)
	}
	if session != "claude-opus-5" {
		t.Fatalf("session pin: got %q, want claude-opus-5", session)
	}

	// ...and the extractor model is emphatically not that.
	got, err := ResolveExtractorAgentModel(root)
	if err != nil {
		t.Fatalf("agent pin: %v", err)
	}
	if got != "sonnet" {
		t.Fatalf("agent pin: got %q, want sonnet", got)
	}
	if got == session {
		t.Fatal("extractor model tracked the dispatching-session pin — D27 has regressed")
	}
}

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
			// Set both fallbacks to values the function must NOT return.
			t.Setenv(ExtractorModelEnv, "claude-opus-5")

			got, err := ResolveExtractorAgentModel(root)
			if err == nil {
				t.Fatalf("expected an error, got model %q", got)
			}
			if got != ExtractorModelUnknown {
				t.Fatalf("got %q, want %q", got, ExtractorModelUnknown)
			}
			if got == DefaultExtractorModel || got == "claude-opus-5" {
				t.Fatalf("fell back to a default (%q) instead of reporting UNKNOWN", got)
			}
		})
	}
}

// The shipped template must satisfy the contract this ADR pins, so the
// decision and the artifact cannot drift apart silently.
func TestShippedExtractorTemplateDeclaresAModel(t *testing.T) {
	root := t.TempDir()
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "..",
		"templates", "agents", "sidecar-extractor.md"))
	if err != nil {
		t.Skipf("shipped template not reachable from here: %v", err)
	}
	writeAgent(t, root, string(src))

	got, err := ResolveExtractorAgentModel(root)
	if err != nil {
		t.Fatalf("shipped sidecar-extractor.md pins no usable model: %v", err)
	}
	if got == "" || got == ExtractorModelUnknown {
		t.Fatalf("shipped template resolved to %q", got)
	}
	t.Logf("shipped extractor model: %s", got)
}
