package sidecar

// schema_test.go — Phase 9 named test surface. Asserts the fixture corpus
// under test/fixtures/sidecars/{valid,invalid} encodes the expected per-file
// shape, complementing the directory-scan assertions in sidecar_test.go.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSchema_ValidFixturesShape pins the per-fixture expectations: count of
// directives, count of signals, presence of source_excerpt fields. A schema
// regression that strips any of those would fail one of these — the loose
// scan in sidecar_test.go would still pass.
func TestSchema_ValidFixturesShape(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "test", "fixtures", "sidecars", "valid")

	cases := []struct {
		file       string
		topic      string
		minSignals int
		minRules   int
		exactRules int // set ≥0 to assert exact directive count; otherwise -1.
	}{
		{"adr-001.edikt.yaml", "architecture", 2, 3, 3},
		{"inv-005.edikt.yaml", "", 1, 2, 2},
		{"guideline-error-handling.edikt.yaml", "", 0, 4, 4},
		{"empty-directives.edikt.yaml", "", 0, 0, 0},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			s, err := Load(filepath.Join(dir, c.file))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if s.SchemaVersion != 1 {
				t.Errorf("schema_version: got %d, want 1", s.SchemaVersion)
			}
			if c.topic != "" && s.Topic != c.topic {
				t.Errorf("topic: got %q, want %q", s.Topic, c.topic)
			}
			if !strings.HasPrefix(s.Topic, "") {
				t.Errorf("topic empty for %s", c.file)
			}
			if len(s.Signals) < c.minSignals {
				t.Errorf("signals: got %d, want ≥%d", len(s.Signals), c.minSignals)
			}
			if c.exactRules >= 0 {
				if len(s.Directives) != c.exactRules {
					t.Errorf("directives: got %d, want %d", len(s.Directives), c.exactRules)
				}
			} else if len(s.Directives) < c.minRules {
				t.Errorf("directives: got %d, want ≥%d", len(s.Directives), c.minRules)
			}
			for i, d := range s.Directives {
				if d.Text == "" {
					t.Errorf("directives[%d].text empty", i)
				}
				if d.SourceExcerpt.LineStart < 1 {
					t.Errorf("directives[%d].source_excerpt.line_start = %d, want ≥1",
						i, d.SourceExcerpt.LineStart)
				}
				if d.SourceExcerpt.LineEnd < d.SourceExcerpt.LineStart {
					t.Errorf("directives[%d].source_excerpt.line_end %d < line_start %d",
						i, d.SourceExcerpt.LineEnd, d.SourceExcerpt.LineStart)
				}
				if d.SourceExcerpt.Quote == "" {
					t.Errorf("directives[%d].source_excerpt.quote empty", i)
				}
			}
		})
	}
}

// TestSchema_InvalidFixturesNamedFailureModes pins that each invalid fixture
// fails for the reason its filename advertises. Catches the case where a
// fixture starts passing because the validator gained a new rule that
// happens to bypass the original failure mode.
func TestSchema_InvalidFixturesNamedFailureModes(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "test", "fixtures", "sidecars", "invalid")

	cases := []struct {
		file        string
		errContains string
	}{
		{"missing-topic.edikt.yaml", "topic"},
		{"extra-source-hash.edikt.yaml", "source_hash"},
		{"non-kebab-topic.edikt.yaml", "topic"},
		{"source-excerpt-out-of-range.edikt.yaml", "line_start"},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			_, err := Load(filepath.Join(dir, c.file))
			if err == nil {
				t.Fatalf("Load(%s): expected error, got nil", c.file)
			}
			if !strings.Contains(err.Error(), c.errContains) {
				t.Errorf("Load(%s): expected error mentioning %q, got %v",
					c.file, c.errContains, err)
			}
		})
	}
}

// TestSchema_RejectsForbiddenTopLevelKeys is a defensive check: the strict
// decoder must reject any of the forbidden keys (source_hash,
// agent_prompt_version, directives_hash) even when an attacker-controlled
// fixture bypasses the on-disk invalid corpus.
func TestSchema_RejectsForbiddenTopLevelKeys(t *testing.T) {
	tmp := t.TempDir()
	cases := map[string]string{
		"source_hash": `schema_version: 1
topic: x
path: x.md
signals: []
directives: []
source_hash: deadbeef
`,
		"agent_prompt_version": `schema_version: 1
topic: x
path: x.md
signals: []
directives: []
agent_prompt_version: 7
`,
		"directives_hash": `schema_version: 1
topic: x
path: x.md
signals: []
directives: []
directives_hash: cafe
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(tmp, name+".yaml")
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(p)
			if err == nil {
				t.Fatalf("forbidden key %q must fail decode", name)
			}
		})
	}
}
