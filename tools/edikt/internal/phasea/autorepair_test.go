package phasea

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// #9 (c4-a): `gov compile --check` rewrote sidecars because TryAutoRepair
// wrote them itself — an os.WriteFile inside phasea, reachable from every
// caller including the non-writing flags, and a full re-marshal that dropped
// the `$schema` header. phasea never persists now: it repairs the in-memory
// sidecar and RETURNS the bytes; govrun decides whether to write, through its
// one guarded, header-preserving writer.
//
// The bytes must be the REPAIRED sidecar, not the load-time cache. That was
// the phantom-stale loop (bok-services, dogfood ADR-017/029): Marshal
// returned the cached original, so every "repair" persisted the drifted file.

const autorepairFixtureSidecar = `# yaml-language-server: $schema=../schemas/gov-sidecar.v1.schema.json
schema_version: 1
topic: "testing"
path: "ADR-001-x.md"
signals: []
directives:
  - text: "The rule MUST hold verbatim. (ref: ADR-001)"
    source_excerpt:
      line_start: 2
      line_end: 2
      quote: "The rule MUST hold verbatim."
`

func TestTryAutoRepair_NeverWrites(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "ADR-001-x.md")
	scPath := filepath.Join(dir, "ADR-001-x.edikt.yaml")

	// Quote lives at line 5; the sidecar anchors it at line 2 (drifted).
	if err := os.WriteFile(parent, []byte("# ADR-001\n\n## Decision\n\nThe rule MUST hold verbatim.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scPath, []byte(autorepairFixtureSidecar), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(scPath, old, old); err != nil {
		t.Fatal(err)
	}

	sc, err := sidecar.Load(scPath)
	if err != nil {
		t.Fatal(err)
	}
	out, err := TryAutoRepair(sc, dir, parent)
	if err != nil {
		t.Fatalf("TryAutoRepair: %v", err)
	}
	if out.AnchorsRepaired != 1 || out.StillStale {
		t.Fatalf("expected 1 repaired, not stale; got %+v", out)
	}

	// The contract under test: nothing on disk moved.
	after, _ := os.ReadFile(scPath)
	if !bytes.Equal(after, []byte(autorepairFixtureSidecar)) {
		t.Fatalf("TryAutoRepair wrote the sidecar; phasea must never persist:\n%s", after)
	}
	if st, _ := os.Stat(scPath); !st.ModTime().Equal(old) {
		t.Fatalf("TryAutoRepair touched the sidecar's mtime: %v -> %v", old, st.ModTime())
	}

	// The in-memory sidecar carries the repair (callers rely on the pointer).
	if se := sc.Directives[0].SourceExcerpt; se.LineStart != 5 || se.LineEnd != 5 {
		t.Fatalf("in-memory repair missing: line_start=%d line_end=%d", se.LineStart, se.LineEnd)
	}

	// And the returned bytes are the REPAIRED sidecar, not the load cache.
	if len(out.Repaired) == 0 {
		t.Fatal("a repair that moved an anchor must return the repaired bytes")
	}
	probe := filepath.Join(t.TempDir(), "probe.edikt.yaml")
	if err := os.WriteFile(probe, out.Repaired, 0o644); err != nil {
		t.Fatal(err)
	}
	reloaded, err := sidecar.Load(probe)
	if err != nil {
		t.Fatalf("returned bytes do not load: %v\n%s", err, out.Repaired)
	}
	if se := reloaded.Directives[0].SourceExcerpt; se.LineStart != 5 || se.LineEnd != 5 {
		t.Fatalf("returned bytes carry the pre-repair anchor (load-time cache): %d-%d\n%s",
			se.LineStart, se.LineEnd, out.Repaired)
	}
}

// Nothing repaired → nothing to persist: no bytes, so a caller cannot write
// a re-marshal of an untouched sidecar.
func TestTryAutoRepair_NothingRepairedReturnsNoBytes(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "ADR-001-x.md")
	scPath := filepath.Join(dir, "ADR-001-x.edikt.yaml")
	if err := os.WriteFile(parent, []byte("# ADR-001\n\n## Decision\n\nSomething else entirely.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scPath, []byte(autorepairFixtureSidecar), 0o644); err != nil {
		t.Fatal(err)
	}
	sc, err := sidecar.Load(scPath)
	if err != nil {
		t.Fatal(err)
	}
	out, err := TryAutoRepair(sc, dir, parent)
	if err != nil {
		t.Fatal(err)
	}
	if out.AnchorsRepaired != 0 || !out.StillStale || out.Repaired != nil {
		t.Fatalf("an absent quote repairs nothing and stays stale with no bytes; got %+v", out)
	}
}
