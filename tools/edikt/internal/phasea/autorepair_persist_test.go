package phasea

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// Field bug (bok-services phantom-stale + dogfood ADR-017/029 repeat-repair):
// TryAutoRepair mutated anchors in memory, then persisted via
// sidecar.Marshal — which returns the LOAD-TIME CACHED bytes, i.e. the
// original file verbatim. Every compile re-repaired the same anchors,
// reported "fully resolved", and wrote back the stale file; the stop-hook
// then re-read the unchanged bytes and warned "stale" right after a
// compile that claimed everything fresh. The repair must round-trip: the
// ON-DISK sidecar carries the moved anchors after TryAutoRepair.
func TestTryAutoRepair_PersistsRepairedAnchorsToDisk(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "ADR-001-x.md")
	scPath := filepath.Join(dir, "ADR-001-x.edikt.yaml")

	// Quote lives at line 5; the sidecar anchors it at line 2 (drifted).
	body := "# ADR-001\n\n## Decision\n\nThe rule MUST hold verbatim.\n"
	if err := os.WriteFile(parent, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	scBody := `schema_version: 1
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
	if err := os.WriteFile(scPath, []byte(scBody), 0o644); err != nil {
		t.Fatal(err)
	}

	sc, err := sidecar.Load(scPath)
	if err != nil {
		t.Fatal(err)
	}
	out, err := TryAutoRepair(sc, dir, parent, scPath)
	if err != nil {
		t.Fatalf("TryAutoRepair: %v", err)
	}
	if out.AnchorsRepaired != 1 || out.StillStale {
		t.Fatalf("expected 1 repaired, not stale; got %+v", out)
	}

	// The contract under test: the repair survived the round-trip to disk.
	reloaded, err := sidecar.Load(scPath)
	if err != nil {
		t.Fatal(err)
	}
	se := reloaded.Directives[0].SourceExcerpt
	if se.LineStart != 5 || se.LineEnd != 5 {
		raw, _ := os.ReadFile(scPath)
		t.Fatalf("repaired anchor NOT persisted: on-disk line_start=%d line_end=%d\nfile:\n%s",
			se.LineStart, se.LineEnd, string(raw))
	}
	if stale, reason, _ := reloaded.IsStale(dir); stale {
		t.Fatalf("reloaded sidecar still stale: %s", reason)
	}
	_ = strings.TrimSpace("")
}
