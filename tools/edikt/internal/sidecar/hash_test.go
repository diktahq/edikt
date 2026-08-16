package sidecar

// hash_test.go — Phase 9 named test surface. Pins the canonical-content hash
// stability of an in-memory Sidecar (the input to phaseb's TopicFingerprint)
// so future canonical-marshal refactors cannot silently drift the
// diff-only-rendering cache key.

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// canonHash returns the SHA-256 of the canonical Marshal output. This is the
// shape phaseb.TopicFingerprint folds into its per-topic tuple hash.
func canonHash(t *testing.T, s *Sidecar) string {
	t.Helper()
	out, err := Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	sum := sha256.Sum256(out)
	return hex.EncodeToString(sum[:])
}

func newSidecar() *Sidecar {
	return &Sidecar{
		SchemaVersion: 1,
		Topic:         "architecture",
		Path:          "docs/architecture/decisions/ADR-001-test.md",
		Signals:       []string{"alpha", "beta"},
		Directives: []Directive{
			{Text: "Rule one. (ref: ADR-001)", SourceExcerpt: SourceExcerpt{LineStart: 10, LineEnd: 10, Quote: "Rule one."}},
			{Text: "Rule two. (ref: ADR-001)", SourceExcerpt: SourceExcerpt{LineStart: 20, LineEnd: 20, Quote: "Rule two."}},
		},
	}
}

func TestHash_StableAcrossRuns(t *testing.T) {
	a := canonHash(t, newSidecar())
	b := canonHash(t, newSidecar())
	if a != b {
		t.Fatalf("canonical hash drifted across calls: %s vs %s", a, b)
	}
}

func TestHash_StableUnderSignalReorder(t *testing.T) {
	s1 := newSidecar()
	s1.Signals = []string{"alpha", "beta"}
	s2 := newSidecar()
	s2.Signals = []string{"beta", "alpha"} // unsorted on input
	if got := canonHash(t, s1); got != canonHash(t, s2) {
		t.Fatalf("hash sensitive to input signal order; canonical writer must sort signals")
	}
}

func TestHash_DiffersOnDirectiveTextChange(t *testing.T) {
	base := newSidecar()
	mut := newSidecar()
	mut.Directives[0].Text = "Rule one (modified). (ref: ADR-001)"
	if canonHash(t, base) == canonHash(t, mut) {
		t.Fatal("hash unchanged after directive text mutation")
	}
}

func TestHash_DiffersOnSignalAddition(t *testing.T) {
	base := newSidecar()
	mut := newSidecar()
	mut.Signals = append(mut.Signals, "gamma")
	if canonHash(t, base) == canonHash(t, mut) {
		t.Fatal("hash unchanged after adding a new signal")
	}
}

func TestHash_DiffersOnSourceExcerptLineShift(t *testing.T) {
	base := newSidecar()
	mut := newSidecar()
	mut.Directives[0].SourceExcerpt.LineStart = 99
	mut.Directives[0].SourceExcerpt.LineEnd = 99
	if canonHash(t, base) == canonHash(t, mut) {
		t.Fatal("hash unchanged after source_excerpt line shift")
	}
}

// TestHash_InvariantToSourcePathField confirms SourcePath is excluded from
// the canonical body — it is a runtime-only convenience set by Load() and
// must not feed the cache key (otherwise sidecars loaded via different
// paths would never hit the diff-only cache).
func TestHash_InvariantToSourcePathField(t *testing.T) {
	a := newSidecar()
	a.SourcePath = "/tmp/a/foo.edikt.yaml"
	b := newSidecar()
	b.SourcePath = "/var/b/foo.edikt.yaml"
	if canonHash(t, a) != canonHash(t, b) {
		t.Fatal("canonical hash leaked SourcePath; cache key must be path-independent")
	}
}

// TestHash_NormalizesNilEmptySignals confirms nil and []string{} both
// canonicalize to `signals: []` so producers that omit the field hit the
// same cache slot as those that emit an explicit empty list.
func TestHash_NormalizesNilEmptySignals(t *testing.T) {
	a := newSidecar()
	a.Signals = nil
	b := newSidecar()
	b.Signals = []string{}
	if canonHash(t, a) != canonHash(t, b) {
		t.Fatal("nil and []string{} signals must produce equal canonical hashes")
	}
	out, _ := Marshal(a)
	if !strings.Contains(string(out), "signals: []") {
		t.Errorf("canonical output for nil signals missing `signals: []`:\n%s", out)
	}
}
