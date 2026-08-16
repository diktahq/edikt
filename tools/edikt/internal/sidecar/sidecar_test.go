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

// TestValidate_RejectsForbiddenSchemaVersion pins that an UNKNOWN schema version
// is rejected.
//
// It previously used 2 as the unknown version. SPEC-011 makes 2 the multi-anchor
// shape, so the probe moved to 3 — the assertion is unchanged in intent (an
// unrecognised version must not load) and the case was updated rather than
// deleted, because deleting it would have removed the only coverage of that
// boundary while appearing to be a passing test suite.
func TestValidate_RejectsForbiddenSchemaVersion(t *testing.T) {
	s := &Sidecar{SchemaVersion: 3, Topic: "x", Path: "x.md"}
	if err := s.Validate(); err == nil {
		t.Fatal("expected schema_version mismatch error for unknown version 3")
	}
}

// TestValidate_AcceptsV1AndV2 is the isolation control for the test above: it
// proves the version check accepts exactly the two known shapes, so the
// rejection of 3 is about being unknown and not about the check refusing
// everything.
func TestValidate_AcceptsV1AndV2(t *testing.T) {
	for _, v := range []int{SchemaVersion, SchemaVersionV2} {
		s := &Sidecar{SchemaVersion: v, Topic: "x", Path: "x.md"}
		if err := s.Validate(); err != nil {
			t.Errorf("schema_version %d should be accepted, got: %v", v, err)
		}
	}
}

// TestValidate_RejectsV2StampOnV1Shape pins that a version bump alone is not a
// migration: a document claiming v2 while still carrying the singular key is a
// half-migrated sidecar, and letting it pass would make the bump meaningless.
func TestValidate_RejectsV2StampOnV1Shape(t *testing.T) {
	s := &Sidecar{
		SchemaVersion: SchemaVersionV2, Topic: "x", Path: "x.md",
		Directives: []Directive{{
			Text:          "some rule",
			SourceExcerpt: SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "some rule"},
		}},
	}
	err := s.Validate()
	if err == nil {
		t.Fatal("v2 stamp on a singular-source_excerpt document was accepted")
	}
	if !strings.Contains(err.Error(), "migrate to-v2") {
		t.Errorf("error should tell the user how to fix it, got: %v", err)
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

// TestLoad_OptionalFields verifies that the four optional fields added in
// v0.6.0 (manual_directives, suppressed_directives, reminders, verification)
// are loaded and preserved correctly. These fields must survive a round-trip
// through Load so that gov:compile can compute the effective rule set and
// aggregate reminders/verification into governance.md.
func TestLoad_OptionalFields(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "test", "fixtures", "sidecars", "valid", "adr-with-overrides.edikt.yaml")
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(s.ManualDirectives) != 1 {
		t.Fatalf("manual_directives: want 1, got %d", len(s.ManualDirectives))
	}
	if s.ManualDirectives[0] != "Always verify the hook script is executable before running the test suite." {
		t.Errorf("manual_directives[0]: unexpected value %q", s.ManualDirectives[0])
	}

	if len(s.SuppressedDirectives) != 1 {
		t.Fatalf("suppressed_directives: want 1, got %d", len(s.SuppressedDirectives))
	}
	if s.SuppressedDirectives[0] != "Do not cache hook results across sessions." {
		t.Errorf("suppressed_directives[0]: unexpected value %q", s.SuppressedDirectives[0])
	}

	if len(s.Reminders) != 1 {
		t.Fatalf("reminders: want 1, got %d", len(s.Reminders))
	}
	if !strings.Contains(s.Reminders[0], "ref: ADR-003") {
		t.Errorf("reminders[0] missing ref tail: %q", s.Reminders[0])
	}

	if len(s.Verification) != 2 {
		t.Fatalf("verification: want 2, got %d", len(s.Verification))
	}
	for i, v := range s.Verification {
		if !strings.HasPrefix(v.Text, "[ ]") {
			t.Errorf("verification[%d].text does not start with '[ ]': %q", i, v.Text)
		}
	}
}

// TestLoad_OptionalFields_Absent verifies that a sidecar with none of the
// optional fields still loads cleanly and returns nil slices — not empty
// slices — so callers can distinguish "not present" from "present but empty".
func TestLoad_OptionalFields_Absent(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "test", "fixtures", "sidecars", "valid", "adr-001.edikt.yaml")
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.ManualDirectives != nil {
		t.Errorf("manual_directives: want nil when absent, got %v", s.ManualDirectives)
	}
	if s.SuppressedDirectives != nil {
		t.Errorf("suppressed_directives: want nil when absent, got %v", s.SuppressedDirectives)
	}
	if s.Reminders != nil {
		t.Errorf("reminders: want nil when absent, got %v", s.Reminders)
	}
	if s.Verification != nil {
		t.Errorf("verification: want nil when absent, got %v", s.Verification)
	}
}
