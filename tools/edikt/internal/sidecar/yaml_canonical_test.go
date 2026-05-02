package sidecar

// yaml_canonical_test.go — Phase 9 named test surface. Round-trips every
// valid fixture under test/fixtures/sidecars/valid/ through Marshal and
// confirms the canonical output decodes to a structurally equal Sidecar.
// Pins the "load → canonical write → load" identity that the diff-only
// rendering cache, the migration apply path, and the per-artifact :compile
// idempotency check all depend on.

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestYAMLCanonical_RoundTripFixtures(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "test", "fixtures", "sidecars", "valid")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read valid fixtures: %v", err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			path := filepath.Join(dir, e.Name())
			loaded, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			out, err := Marshal(loaded)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var reloaded Sidecar
			dec := yaml.NewDecoder(strings.NewReader(string(out)))
			dec.KnownFields(true)
			if err := dec.Decode(&reloaded); err != nil {
				t.Fatalf("decode canonical: %v\n--- OUTPUT ---\n%s", err, string(out))
			}
			if err := reloaded.Validate(); err != nil {
				t.Fatalf("validate canonical: %v", err)
			}

			// SourcePath is a runtime-only field; clear it before structural compare.
			loaded.SourcePath = ""
			normalizeSignals(loaded)
			normalizeSignals(&reloaded)

			if !reflect.DeepEqual(loaded.Directives, reloaded.Directives) {
				t.Errorf("directives differ after round-trip\nbefore: %+v\nafter:  %+v",
					loaded.Directives, reloaded.Directives)
			}
			if loaded.Topic != reloaded.Topic {
				t.Errorf("topic drift: %q vs %q", loaded.Topic, reloaded.Topic)
			}
			if loaded.Path != reloaded.Path {
				t.Errorf("path drift: %q vs %q", loaded.Path, reloaded.Path)
			}
			if !reflect.DeepEqual(loaded.Signals, reloaded.Signals) {
				t.Errorf("signals drift: %v vs %v", loaded.Signals, reloaded.Signals)
			}
		})
	}
}

// TestYAMLCanonical_DoubleMarshalIsIdempotent: writing the canonical bytes,
// reloading, and re-marshalling MUST produce the exact same bytes — this is
// the property that lets `:compile` short-circuit on byte-equal output.
func TestYAMLCanonical_DoubleMarshalIsIdempotent(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "test", "fixtures", "sidecars", "valid")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			loaded, err := Load(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			out1, err := Marshal(loaded)
			if err != nil {
				t.Fatalf("Marshal #1: %v", err)
			}

			var roundTripped Sidecar
			dec := yaml.NewDecoder(strings.NewReader(string(out1)))
			dec.KnownFields(true)
			if err := dec.Decode(&roundTripped); err != nil {
				t.Fatalf("decode #1: %v", err)
			}
			out2, err := Marshal(&roundTripped)
			if err != nil {
				t.Fatalf("Marshal #2: %v", err)
			}
			if string(out1) != string(out2) {
				t.Errorf("canonical Marshal not idempotent:\n--- first ---\n%s\n--- second ---\n%s",
					string(out1), string(out2))
			}
		})
	}
}

// TestYAMLCanonical_FormatGuards holds the line-level format guarantees
// the spec calls out (LF endings, 2-space indent, top-level key order).
func TestYAMLCanonical_FormatGuards(t *testing.T) {
	root := repoRoot(t)
	loaded, err := Load(filepath.Join(root, "test", "fixtures", "sidecars", "valid", "adr-001.edikt.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	out, err := Marshal(loaded)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	body := string(out)

	if strings.Contains(body, "\r\n") {
		t.Error("canonical output must use LF line endings, found CRLF")
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed != line {
			t.Errorf("trailing whitespace on canonical line: %q", line)
		}
	}
	expectedOrder := []string{"schema_version:", "topic:", "path:", "signals:", "directives:"}
	prev := -1
	for _, k := range expectedOrder {
		idx := strings.Index(body, k)
		if idx < 0 {
			t.Fatalf("canonical output missing key %q:\n%s", k, body)
		}
		if idx <= prev {
			t.Errorf("canonical output emits %q out of order:\n%s", k, body)
		}
		prev = idx
	}
}

// normalizeSignals collapses nil → empty slice and sorts the slice in place
// for structural comparison. Marshal emits a sorted `signals:` block, so a
// fixture with unsorted signals will round-trip into a sorted slice — the
// pre-load and post-load values must be canonicalized the same way before
// reflect.DeepEqual.
func normalizeSignals(s *Sidecar) {
	if s.Signals == nil {
		s.Signals = []string{}
	}
	sort.Strings(s.Signals)
}
