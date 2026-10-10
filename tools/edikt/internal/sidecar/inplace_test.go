package sidecar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const inplaceFixture = `schema_version: 2
topic: "hooks"   # a comment that must survive
path: "docs/architecture/decisions/ADR-900-x.md"
signals:
  - "hook"
proposed_paths:
  - glob: "tools/**"
    evidence: "because"
directives:
  - text: 'Hooks MUST emit JSON. (ref: ADR-900)'
    verify: "bash test/hooks/json.sh"
    verify_kind: behavioral
    falsifying_observation: >-
      a hook emits non-JSON
      and nothing notices
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: 'Hooks MUST emit JSON.'
  - text: 'Second rule MUST hold. (ref: ADR-900)'
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: 'Hooks MUST emit JSON.'
prohibitions:
  - text: 'MUST NOT shell out. (ref: ADR-900)'
    verify: "bash test/x.sh"
    verify_kind: behavioral
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: 'Hooks MUST emit JSON.'
`

func TestRemoveKeysInPlace_RemovesOnlyTheNamedLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.edikt.yaml")
	if err := os.WriteFile(p, []byte(inplaceFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := RemoveKeysInPlace(p, []KeyRemoval{
		{List: "directives", Index: 0, Keys: []string{"verify", "verify_kind", "positive_fixture_path"}},
		{List: "prohibitions", Index: 0, Keys: []string{"verify", "verify_kind"}},
		{List: "", Keys: []string{"proposed_paths"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("removed %d keys, want 5 (the absent positive_fixture_path is not counted)", n)
	}
	got, _ := os.ReadFile(p)
	before := strings.Split(inplaceFixture, "\n")
	after := strings.Split(string(got), "\n")
	// Every surviving line is a byte-identical line of the original, in order.
	j := 0
	for _, l := range after {
		for j < len(before) && before[j] != l {
			j++
		}
		if j == len(before) {
			t.Fatalf("line %q was not in the original (reserialised?)", l)
		}
		j++
	}
	removed := len(before) - len(after)
	if removed != 7 {
		t.Fatalf("removed %d lines, want 7: 2 verify keys ×2 entries + proposed_paths (3 lines)", removed)
	}
	s := string(got)
	for _, must := range []string{"# a comment that must survive", "falsifying_observation: >-", "      and nothing notices", "Second rule MUST hold"} {
		if !strings.Contains(s, must) {
			t.Fatalf("lost %q:\n%s", must, s)
		}
	}
	for _, gone := range []string{"verify:", "verify_kind:", "proposed_paths:", "glob:"} {
		if strings.Contains(s, gone) {
			t.Fatalf("still carries %q:\n%s", gone, s)
		}
	}
	if _, err := Load(p); err != nil {
		t.Fatalf("result must still load: %v", err)
	}
}

func TestRemoveKeysInPlace_NoMatchWritesNothing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.edikt.yaml")
	if err := os.WriteFile(p, []byte(inplaceFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	st1, _ := os.Stat(p)
	n, err := RemoveKeysInPlace(p, []KeyRemoval{{List: "directives", Index: 1, Keys: []string{"verify"}}})
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	st2, _ := os.Stat(p)
	if st1.ModTime() != st2.ModTime() || st1.Size() != st2.Size() {
		t.Fatal("a no-op removal must not touch the file")
	}
}

const inplaceFixtureWithDigest = `schema_version: 2
topic: "hooks"
path: "docs/architecture/decisions/ADR-900-x.md"
body_digest: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
signals:
  - "hook"
directives:
  - text: 'Hooks MUST emit JSON. (ref: ADR-900)'
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: 'Hooks MUST emit JSON.'
`

func TestInsertKeyInPlace_InsertsAfterBodyDigestWhenPresent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.edikt.yaml")
	if err := os.WriteFile(p, []byte(inplaceFixtureWithDigest), 0o644); err != nil {
		t.Fatal(err)
	}
	err := InsertKeyInPlace(p, []KeyValue{
		{Key: "extractor_model", Value: "claude-sonnet-5"},
		{Key: "extractor_contract", Value: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	want := "body_digest: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n" +
		"extractor_model: claude-sonnet-5\n" +
		"extractor_contract: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n" +
		"signals:"
	if !strings.Contains(s, want) {
		t.Fatalf("expected the new keys right after body_digest:\n%s", s)
	}
	if !strings.Contains(s, "Hooks MUST emit JSON") {
		t.Fatalf("lost unrelated content:\n%s", s)
	}
	if _, err := Load(p); err != nil {
		t.Fatalf("result must still load: %v", err)
	}
}

func TestInsertKeyInPlace_InsertsAfterPathWhenNoBodyDigest(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.edikt.yaml")
	if err := os.WriteFile(p, []byte(inplaceFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InsertKeyInPlace(p, []KeyValue{{Key: "extractor_model", Value: "unrecorded"}}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, _ := os.ReadFile(p)
	s := string(got)
	want := `path: "docs/architecture/decisions/ADR-900-x.md"` + "\nextractor_model: unrecorded\nsignals:"
	if !strings.Contains(s, want) {
		t.Fatalf("expected extractor_model right after path::\n%s", s)
	}
	if !strings.Contains(s, "# a comment that must survive") {
		t.Fatalf("lost the comment:\n%s", s)
	}
	if _, err := Load(p); err != nil {
		t.Fatalf("result must still load: %v", err)
	}
}

func TestInsertKeyInPlace_ReplacesExistingKeyInPlace(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.edikt.yaml")
	if err := os.WriteFile(p, []byte(inplaceFixtureWithDigest), 0o644); err != nil {
		t.Fatal(err)
	}
	// First stamp with a real model...
	if err := InsertKeyInPlace(p, []KeyValue{{Key: "extractor_model", Value: "claude-sonnet-5"}}); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	// ...then a second run (a re-extraction) stamps a different value. It
	// must be replaced, never duplicated.
	if err := InsertKeyInPlace(p, []KeyValue{{Key: "extractor_model", Value: "claude-opus-5"}}); err != nil {
		t.Fatalf("second insert: %v", err)
	}
	got, _ := os.ReadFile(p)
	s := string(got)
	if n := strings.Count(s, "extractor_model:"); n != 1 {
		t.Fatalf("expected exactly one extractor_model: line, got %d:\n%s", n, s)
	}
	if !strings.Contains(s, "extractor_model: claude-opus-5") {
		t.Fatalf("did not update to the new value:\n%s", s)
	}
	if strings.Contains(s, "claude-sonnet-5") {
		t.Fatalf("old value survived alongside the new one:\n%s", s)
	}
	if _, err := Load(p); err != nil {
		t.Fatalf("result must still load: %v", err)
	}
}

func TestInsertKeyInPlace_MixedInsertAndReplace(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.edikt.yaml")
	if err := os.WriteFile(p, []byte(inplaceFixtureWithDigest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InsertKeyInPlace(p, []KeyValue{{Key: "extractor_model", Value: "claude-sonnet-5"}}); err != nil {
		t.Fatalf("seed insert: %v", err)
	}
	// extractor_model now exists (replace path); extractor_contract does
	// not yet (insert path) — one call must handle both correctly.
	err := InsertKeyInPlace(p, []KeyValue{
		{Key: "extractor_model", Value: "claude-haiku-4-5"},
		{Key: "extractor_contract", Value: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
	})
	if err != nil {
		t.Fatalf("mixed insert: %v", err)
	}
	got, _ := os.ReadFile(p)
	s := string(got)
	if !strings.Contains(s, "extractor_model: claude-haiku-4-5") {
		t.Fatalf("replace half failed:\n%s", s)
	}
	if !strings.Contains(s, "extractor_contract: cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc") {
		t.Fatalf("insert half failed:\n%s", s)
	}
	if n := strings.Count(s, "extractor_model:"); n != 1 {
		t.Fatalf("expected exactly one extractor_model: line, got %d", n)
	}
	if _, err := Load(p); err != nil {
		t.Fatalf("result must still load: %v", err)
	}
}
