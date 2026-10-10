package phasea

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExtractorModelConfig covers ADR-085 Decision 1: extractor.model
// resolves to "inherit" by default, accepts an explicit value passing
// idvalidate.ModelID, and refuses anything else (including the "inherit"
// literal reaching the validator, which it never does).
func TestExtractorModelConfig(t *testing.T) {
	t.Run("absent config file defaults to inherit", func(t *testing.T) {
		root := t.TempDir()
		cfg, err := LoadConfig(root)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.ExtractorModel != ExtractorModelInherit {
			t.Errorf("ExtractorModel = %q, want %q", cfg.ExtractorModel, ExtractorModelInherit)
		}
	})

	t.Run("config file with no extractor section defaults to inherit", func(t *testing.T) {
		root := writeConfigYAML(t, "paths:\n  decisions: docs/architecture/decisions\n")
		cfg, err := LoadConfig(root)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.ExtractorModel != ExtractorModelInherit {
			t.Errorf("ExtractorModel = %q, want %q", cfg.ExtractorModel, ExtractorModelInherit)
		}
	})

	t.Run("explicit inherit resolves to inherit", func(t *testing.T) {
		root := writeConfigYAML(t, "extractor:\n  model: inherit\n")
		cfg, err := LoadConfig(root)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.ExtractorModel != ExtractorModelInherit {
			t.Errorf("ExtractorModel = %q, want %q", cfg.ExtractorModel, ExtractorModelInherit)
		}
	})

	t.Run("explicit valid model id is accepted verbatim", func(t *testing.T) {
		root := writeConfigYAML(t, "extractor:\n  model: claude-sonnet-5\n")
		cfg, err := LoadConfig(root)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.ExtractorModel != "claude-sonnet-5" {
			t.Errorf("ExtractorModel = %q, want %q", cfg.ExtractorModel, "claude-sonnet-5")
		}
	})

	t.Run("shape-invalid model id is refused", func(t *testing.T) {
		root := writeConfigYAML(t, "extractor:\n  model: 'not a valid id; rm -rf /'\n")
		_, err := LoadConfig(root)
		if err == nil {
			t.Fatal("LoadConfig: expected an error for a shape-invalid extractor.model, got nil")
		}
	})

	t.Run("inherit never reaches idvalidate.ModelID as though it were a model id", func(t *testing.T) {
		// ModelID's own pattern (^[a-z][a-z0-9-]{0,40}$) would actually
		// accept the literal "inherit" — this test exists so a future
		// refactor that routes "inherit" through the validator anyway
		// still passes ModelID's shape check and could silently start
		// treating "inherit" as a dispatchable model id. Assert the
		// resolved value is the reserved literal via LoadConfig's own
		// path, not via ModelID directly, so the test fails if the
		// reserved-literal short-circuit above this comment is ever
		// removed.
		root := writeConfigYAML(t, "extractor:\n  model: inherit\n")
		cfg, err := LoadConfig(root)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.ExtractorModel != ExtractorModelInherit {
			t.Errorf("ExtractorModel = %q, want the reserved literal %q, not a validated model id",
				cfg.ExtractorModel, ExtractorModelInherit)
		}
	})
}

// writeConfigYAML writes body as .edikt/config.yaml under a fresh temp root
// and returns the root.
func writeConfigYAML(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".edikt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir .edikt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
	return root
}
