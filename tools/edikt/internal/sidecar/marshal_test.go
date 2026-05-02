package sidecar

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMarshal_RoundTrip(t *testing.T) {
	in := &Sidecar{
		SchemaVersion: 1,
		Topic:         "architecture",
		Path:          "docs/architecture/decisions/ADR-001-test.md",
		Signals:       []string{"hooks", "platform", "agent"}, // unsorted on input
		Directives: []Directive{
			{
				Text: "First directive (ref: ADR-001)",
				SourceExcerpt: SourceExcerpt{
					LineStart: 10,
					LineEnd:   12,
					Quote:     "First directive",
				},
			},
		},
	}

	out, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got Sidecar
	dec := yaml.NewDecoder(strings.NewReader(string(out)))
	dec.KnownFields(true)
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("decode marshalled: %v\n%s", err, string(out))
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("decoded validate: %v", err)
	}
}

func TestMarshal_SortsSignals(t *testing.T) {
	in := &Sidecar{
		SchemaVersion: 1,
		Topic:         "architecture",
		Path:          "x.md",
		Signals:       []string{"zebra", "alpha", "mango"},
		Directives:    nil,
	}
	out, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	body := string(out)
	posA := strings.Index(body, "alpha")
	posM := strings.Index(body, "mango")
	posZ := strings.Index(body, "zebra")
	if !(posA < posM && posM < posZ) {
		t.Errorf("signals not sorted in output:\n%s", body)
	}
	// Original slice must not be mutated.
	if in.Signals[0] != "zebra" {
		t.Errorf("Marshal mutated input; first signal now %q", in.Signals[0])
	}
}

func TestMarshal_DeterministicByteEqual(t *testing.T) {
	in := &Sidecar{
		SchemaVersion: 1,
		Topic:         "architecture",
		Path:          "x.md",
		Signals:       []string{"b", "a"},
		Directives: []Directive{
			{Text: "T1", SourceExcerpt: SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "q"}},
			{Text: "T2", SourceExcerpt: SourceExcerpt{LineStart: 2, LineEnd: 3, Quote: "q2"}},
		},
	}
	a, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Errorf("Marshal not deterministic:\n%s\n---\n%s", string(a), string(b))
	}
}

func TestMarshal_EmptySignalsRendersBracket(t *testing.T) {
	in := &Sidecar{
		SchemaVersion: 1,
		Topic:         "architecture",
		Path:          "x.md",
		Signals:       nil,
		Directives:    nil,
	}
	out, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "signals: []") {
		t.Errorf("expected `signals: []` for nil signals, got:\n%s", string(out))
	}
}

func TestMarshal_KeyOrderTopLevel(t *testing.T) {
	in := &Sidecar{
		SchemaVersion: 1,
		Topic:         "x",
		Path:          "x.md",
		Signals:       []string{"a"},
		Directives: []Directive{
			{Text: "t", SourceExcerpt: SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "q"}},
		},
	}
	out, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	body := string(out)
	keys := []string{"schema_version:", "topic:", "path:", "signals:", "directives:"}
	prev := -1
	for _, k := range keys {
		idx := strings.Index(body, k)
		if idx < 0 {
			t.Fatalf("key %s missing in:\n%s", k, body)
		}
		if idx <= prev {
			t.Errorf("key %s out of canonical order in:\n%s", k, body)
		}
		prev = idx
	}
}

// TestMarshal_CacheMatchesFresh pins Phase 7 of PLAN-sidecar-review-fixes
// #39: every fixture loaded via Load MUST round-trip Marshal byte-equal to
// a fresh encoder pass. The cache is the determinism anchor for Phase B
// (TopicFingerprint hashes Marshal output), so if a future refactor breaks
// the cache invariant the topic fingerprint silently drifts — this test
// fails fast in that case.
func TestMarshal_CacheMatchesFresh(t *testing.T) {
	root, err := findFixtureRoot(t)
	if err != nil {
		t.Skipf("fixture root not found: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(root, "test/fixtures/sidecars/valid/*.edikt.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Skip("no valid fixtures found; skipping cache-equality assertion")
	}
	for _, p := range matches {
		s, err := Load(p)
		if err != nil {
			t.Fatalf("Load(%s): %v", p, err)
		}
		fresh, err := marshalUncached(s)
		if err != nil {
			t.Fatalf("marshalUncached(%s): %v", p, err)
		}
		via, err := Marshal(s)
		if err != nil {
			t.Fatalf("Marshal(%s): %v", p, err)
		}
		if string(via) != string(fresh) {
			t.Errorf("%s: cached Marshal != fresh\ncached:\n%s\nfresh:\n%s", p, via, fresh)
		}
		// Idempotence: two consecutive Marshal calls must return the same bytes.
		via2, _ := Marshal(s)
		if string(via) != string(via2) {
			t.Errorf("%s: Marshal not idempotent across calls", p)
		}
	}
}

// findFixtureRoot walks up from the test cwd looking for the repo root,
// identified by test/fixtures/sidecars/valid/.
func findFixtureRoot(t *testing.T) (string, error) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "test/fixtures/sidecars/valid")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no test/fixtures/sidecars/valid found above cwd")
}
