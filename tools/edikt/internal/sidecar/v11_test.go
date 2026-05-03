package sidecar

// v11_test.go — Phase 1 of PLAN-v060-governance-accuracy.
//
// Pins the schema v1.1 additive contract: Paths, Scope, Prohibitions are
// optional fields; existing v1.0 sidecars (fixtures shipped with rc4)
// continue to parse byte-equal under v1.1; Validate enforces the new
// invariants (scope enum, prohibition source_excerpt non-empty, paths
// non-empty strings).

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParse_v11_AdditiveFields covers AC-1.1 + AC-1.2 — a sidecar that
// declares paths, scope, and prohibitions parses and validates cleanly.
func TestParse_v11_AdditiveFields(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "with-v11-fields.edikt.yaml")
	body := []byte(`schema_version: 1
topic: ai
path: docs/architecture/decisions/ADR-099-test.md
signals:
  - voice pipeline
paths:
  - internal/stt/**/*.go
  - internal/voice/**/*.go
scope:
  - design
  - implementation
  - review
directives:
  - text: "Two-stage pipeline MUST be used for voice extraction. (ref: ADR-099)"
    source_excerpt:
      line_start: 12
      line_end: 12
      quote: "Two-stage pipeline must be used for voice extraction."
prohibitions:
  - text: "MUST NOT use a single-stage pipeline that lacks native speaker diarization. (ref: ADR-099)"
    source_excerpt:
      line_start: 24
      line_end: 26
      quote: "Cons: no reliable speaker diarization (prompt-based, not native)."
    derived_from: rejected_option_a
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := len(s.Paths), 2; got != want {
		t.Errorf("paths len: got %d, want %d", got, want)
	}
	if got, want := s.Paths[0], "internal/stt/**/*.go"; got != want {
		t.Errorf("paths[0]: got %q, want %q", got, want)
	}
	if got, want := len(s.Scope), 3; got != want {
		t.Errorf("scope len: got %d, want %d", got, want)
	}
	if got, want := s.Scope[1], "implementation"; got != want {
		t.Errorf("scope[1]: got %q, want %q", got, want)
	}
	if got, want := len(s.Prohibitions), 1; got != want {
		t.Fatalf("prohibitions len: got %d, want %d", got, want)
	}
	if got, want := s.Prohibitions[0].DerivedFrom, "rejected_option_a"; got != want {
		t.Errorf("prohibitions[0].derived_from: got %q, want %q", got, want)
	}
	if !strings.HasPrefix(s.Prohibitions[0].Text, "MUST NOT") {
		t.Errorf("prohibitions[0].text should start with MUST NOT, got: %q", s.Prohibitions[0].Text)
	}
}

// TestParse_v11_OmittedFields covers AC-1.4 (forward-compat surface) — a
// sidecar that omits the new fields parses cleanly and exposes nil slices,
// not empty slices, so callers can distinguish "not present" from "present
// but empty".
func TestParse_v11_OmittedFields(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "test", "fixtures", "sidecars", "valid", "adr-001.edikt.yaml")
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Paths != nil {
		t.Errorf("paths: want nil when absent, got %v", s.Paths)
	}
	if s.Scope != nil {
		t.Errorf("scope: want nil when absent, got %v", s.Scope)
	}
	if s.Prohibitions != nil {
		t.Errorf("prohibitions: want nil when absent, got %v", s.Prohibitions)
	}
}

// TestValidate_ScopeEnumRejected covers AC-1.3 (scope enum half) — any
// scope entry not in the closed enum {planning, design, implementation,
// review} fails Validate.
func TestValidate_ScopeEnumRejected(t *testing.T) {
	cases := []struct {
		name  string
		scope []string
	}{
		{"empty-string", []string{""}},
		{"unknown-phase", []string{"deployment"}},
		{"capitalized", []string{"Design"}},
		{"valid-then-invalid", []string{"design", "release"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Sidecar{
				SchemaVersion: 1, Topic: "ok", Path: "x.md",
				Scope: tc.scope,
			}
			err := s.Validate()
			if err == nil {
				t.Fatalf("expected scope enum rejection for %v", tc.scope)
			}
			if !strings.Contains(err.Error(), "scope[") {
				t.Errorf("error should cite scope index; got: %v", err)
			}
		})
	}

	// And a positive case — every valid phase passes.
	ok := &Sidecar{
		SchemaVersion: 1, Topic: "ok", Path: "x.md",
		Scope: []string{"planning", "design", "implementation", "review"},
	}
	if err := ok.Validate(); err != nil {
		t.Errorf("all-valid scope rejected: %v", err)
	}
}

// TestValidate_ProhibitionsRequireSourceExcerpt covers AC-1.3 (prohibitions
// half) — every prohibition entry needs the same source-excerpt
// non-emptiness invariants as a directive.
func TestValidate_ProhibitionsRequireSourceExcerpt(t *testing.T) {
	cases := []struct {
		name       string
		proh       Prohibition
		errFragmnt string
	}{
		{
			"missing-text",
			Prohibition{SourceExcerpt: SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "q"}},
			"text: required",
		},
		{
			"text-over-500",
			Prohibition{Text: strings.Repeat("a", 501), SourceExcerpt: SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "q"}},
			"max 500",
		},
		{
			"line-start-zero",
			Prohibition{Text: "MUST NOT do X", SourceExcerpt: SourceExcerpt{LineStart: 0, LineEnd: 1, Quote: "q"}},
			"line_start",
		},
		{
			"line-end-before-start",
			Prohibition{Text: "MUST NOT do X", SourceExcerpt: SourceExcerpt{LineStart: 5, LineEnd: 3, Quote: "q"}},
			"line_end",
		},
		{
			"quote-empty",
			Prohibition{Text: "MUST NOT do X", SourceExcerpt: SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: ""}},
			"quote: required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Sidecar{
				SchemaVersion: 1, Topic: "ok", Path: "x.md",
				Prohibitions: []Prohibition{tc.proh},
			}
			err := s.Validate()
			if err == nil {
				t.Fatalf("expected validation error containing %q", tc.errFragmnt)
			}
			if !strings.Contains(err.Error(), "prohibitions[0]") {
				t.Errorf("error should cite prohibitions[0]; got: %v", err)
			}
			if !strings.Contains(err.Error(), tc.errFragmnt) {
				t.Errorf("error fragment %q not in: %v", tc.errFragmnt, err)
			}
		})
	}
}

// TestValidate_PathsRejectsEmpty covers AC-1.3 (paths half) — empty path
// strings are rejected even though full glob-syntax validation is deferred.
func TestValidate_PathsRejectsEmpty(t *testing.T) {
	s := &Sidecar{
		SchemaVersion: 1, Topic: "ok", Path: "x.md",
		Paths: []string{"internal/stt/**", ""},
	}
	if err := s.Validate(); err == nil {
		t.Fatal("expected paths empty-string rejection")
	}
}

// TestForwardCompat_Rc4Parse covers AC-1.4 — every existing v1.0 fixture
// in test/fixtures/sidecars/valid/ continues to parse without error under
// v1.1. Regression guard for the additive contract: a Phase 1 change that
// requires fields the rc4 fixtures don't have would break this test.
func TestForwardCompat_Rc4Parse(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "test", "fixtures", "sidecars", "valid")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	if len(entries) < 5 {
		t.Fatalf("expected ≥5 valid fixtures (rc4 baseline); got %d", len(entries))
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			s, err := Load(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatalf("v1.1 Load of rc4-shaped fixture %s failed: %v", e.Name(), err)
			}
			// Confirm the new optional fields are nil for rc4-shaped fixtures
			// (forward-compat means the absence is preserved as nil, not
			// silently materialized as []).
			if s.Paths != nil {
				t.Errorf("rc4 fixture should have nil Paths; got %v", s.Paths)
			}
			if s.Scope != nil {
				t.Errorf("rc4 fixture should have nil Scope; got %v", s.Scope)
			}
			if s.Prohibitions != nil {
				t.Errorf("rc4 fixture should have nil Prohibitions; got %v", s.Prohibitions)
			}
		})
	}
}

// rc4HashBaseline pins the canonical Marshal sha256 for each rc4-shaped
// fixture as captured at v0.6.0 final ship. A divergence indicates that
// the v1.1 schema additions changed the marshal output for sidecars that
// don't use the new fields — which would force every rc1–rc4 user's
// existing sidecars to re-hash on first compile post-upgrade, triggering
// spurious hand-edit conflict interviews.
//
// **DO NOT EDIT THESE VALUES** without an explicit migration path. If a
// schema change must alter the marshal contract, bump SchemaVersion and
// add an explicit downgrade path; do not silently re-hash the corpus.
var rc4HashBaseline = map[string]string{
	"adr-001.edikt.yaml":                  "41b24720e7c5d67b37ca21bfbe49ad833f8d5261a759000e3b71bd7e5c0d7e01",
	"adr-with-overrides.edikt.yaml":       "d5bfff2ef2b5f21c57934909d9ccf1f7f528d4f71dfbd3fb88cb7a72126216c7",
	"empty-directives.edikt.yaml":         "e7ffabaeb14d237922a70b098c64c463977ce24d23281408e93f594152b38563",
	"guideline-error-handling.edikt.yaml": "6f4b1f1988b415306dea7a303a43d49fce23fa2d992bd71b61a09d2176707e31",
	"inv-005.edikt.yaml":                  "345b24fc06456775ea923a734e85b3b767a5a34510d17c9b160a3e808e41a4a8",
}

// TestHashStability_rc4 covers AC-1.5 — for each rc4-shaped fixture, the
// canonical Marshal output's sha256 MUST match the pinned baseline. This
// is the release-gate concern from Platform pre-flight finding #1.
//
// First-run protocol: the baseline above starts as PLACEHOLDER_*. Run the
// test once with `go test -run TestHashStability_rc4 -v` to capture the
// observed hashes from the test failure output, then paste them into
// rc4HashBaseline and re-run. Subsequent runs pin the values.
func TestHashStability_rc4(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "test", "fixtures", "sidecars", "valid")
	for fixture, want := range rc4HashBaseline {
		t.Run(fixture, func(t *testing.T) {
			s, err := Load(filepath.Join(dir, fixture))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			out, err := Marshal(s)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			sum := sha256.Sum256(out)
			got := hex.EncodeToString(sum[:])
			if strings.HasPrefix(want, "PLACEHOLDER_") {
				// First-run capture mode — print observed hash and fail
				// loudly so the developer pastes it into the baseline.
				t.Errorf("rc4HashBaseline[%q] is unset; observed sha256 = %q", fixture, got)
				return
			}
			if got != want {
				t.Errorf("hash drift for %s:\n  got  %s\n  want %s\nThis means the v1.1 schema additions changed the marshal output for an rc4-shaped sidecar. Either revert the change or document the migration.", fixture, got, want)
			}
		})
	}
}
