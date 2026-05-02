package sidecar

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot returns the keel repo root by walking up from this test file.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(here)
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "..", "..", "test", "fixtures", "sidecars")); err == nil {
				return filepath.Clean(filepath.Join(dir, "..", ".."))
			}
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not locate repo root from test file")
	return ""
}

func TestLoadValidFixtures(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "test", "fixtures", "sidecars", "valid")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read valid fixtures: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no valid fixtures present")
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			s, err := Load(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatalf("Load(%s): %v", e.Name(), err)
			}
			if s.SchemaVersion != 1 {
				t.Errorf("schema_version: got %d, want 1", s.SchemaVersion)
			}
			if s.Topic == "" {
				t.Errorf("topic is empty")
			}
		})
	}
}

func TestLoadInvalidFixturesReject(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "test", "fixtures", "sidecars", "invalid")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read invalid fixtures: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no invalid fixtures present")
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			_, err := Load(filepath.Join(dir, e.Name()))
			if err == nil {
				t.Fatalf("Load(%s): expected error, got nil", e.Name())
			}
		})
	}
}

func TestValidate_RejectsForbiddenSchemaVersion(t *testing.T) {
	s := &Sidecar{SchemaVersion: 2, Topic: "x", Path: "x.md"}
	if err := s.Validate(); err == nil {
		t.Fatal("expected schema_version mismatch error")
	}
}

func TestValidate_RejectsNonKebabTopic(t *testing.T) {
	s := &Sidecar{SchemaVersion: 1, Topic: "Bad_Topic", Path: "x.md"}
	if err := s.Validate(); err == nil {
		t.Fatal("expected topic regex error")
	}
}

func TestValidate_RejectsDirectiveOver500Chars(t *testing.T) {
	// The original 200-char ceiling proved too tight on real-world ADRs
	// (the ddd-workbench corpus had directives in the 209–236 range).
	// Bumped to 500 — natural-language rules with multiple clauses fit
	// comfortably; rules that would still exceed it should be split.
	s := &Sidecar{
		SchemaVersion: 1, Topic: "ok", Path: "x.md",
		Directives: []Directive{
			{Text: strings.Repeat("a", 501), SourceExcerpt: SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "q"}},
		},
	}
	if err := s.Validate(); err == nil {
		t.Fatal("expected length error for 501-char directive")
	}

	// And the inverse — a 400-char directive (above the old 200 limit
	// but below the new 500 ceiling) MUST validate cleanly.
	ok := &Sidecar{
		SchemaVersion: 1, Topic: "ok", Path: "x.md",
		Directives: []Directive{
			{Text: strings.Repeat("a", 400), SourceExcerpt: SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "q"}},
		},
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("400-char directive should validate; got %v", err)
	}
}

func TestValidate_RejectsDuplicateSignal(t *testing.T) {
	s := &Sidecar{
		SchemaVersion: 1, Topic: "ok", Path: "x.md",
		Signals: []string{"alpha", "alpha"},
	}
	if err := s.Validate(); err == nil {
		t.Fatal("expected uniqueItems error")
	}
}
