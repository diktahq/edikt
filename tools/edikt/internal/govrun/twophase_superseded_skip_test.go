package govrun

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/model"
)

// D46 (b): the status filter runs BEFORE staleness in every caller. A
// superseded ADR whose sidecar anchors are deliberately broken must be
// skipped by compile — not planned, not dispatched, not refused — while an
// accepted, fresh sibling compiles.
func TestRunTwoPhase_SupersededBrokenAnchorsAreSkippedNotReextracted(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")
	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(adrDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ADR-070-fresh.md", "---\nstatus: accepted\n---\n\n# ADR-070 — fresh\n\n## Decision\n\nFresh rule MUST hold.\n")
	write("ADR-070-fresh.edikt.yaml", `schema_version: 2
topic: "testing"
path: "docs/architecture/decisions/ADR-070-fresh.md"
signals:
  - "fresh"
directives:
  - text: "Fresh rule MUST hold. (ref: ADR-070)"
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: "Fresh rule MUST hold."
`)
	write("ADR-071-superseded.md", "---\nstatus: superseded\n---\n\n# ADR-071 — superseded\n\n**Status:** Superseded by ADR-070\n\n## Decision\n\nOld rule MUST hold.\n")
	write("ADR-071-superseded.edikt.yaml", `schema_version: 2
topic: "testing"
path: "docs/architecture/decisions/ADR-071-superseded.md"
signals:
  - "old"
directives:
  - text: "Old rule MUST hold. (ref: ADR-071)"
    source_excerpts:
      - line_start: 2
        line_end: 2
        quote: "this quote appears nowhere in the parent"
`)
	stageExtractorAgent(t, root, "claude-sonnet-5")

	var errBuf, outBuf bytes.Buffer
	_, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		Stderr:      &errBuf,
		Stdout:      &outBuf,
		OnLoss:      "accept",
	}, model.RealClock{})
	combined := outBuf.String() + errBuf.String()
	if err != nil {
		t.Fatalf("compile must succeed on the accepted sibling and skip the superseded one, got: %v\n%s", err, combined)
	}
	if strings.Contains(combined, "stale: ADR-071") || strings.Contains(combined, "bootstrap: ADR-071") {
		t.Fatalf("a superseded artifact must never be planned for re-extraction:\n%s", combined)
	}
	if _, serr := os.Stat(filepath.Join(root, ".edikt", "state", "phasea")); serr == nil {
		entries, _ := os.ReadDir(filepath.Join(root, ".edikt", "state", "phasea"))
		if len(entries) > 0 {
			t.Fatalf("no Phase A plan may exist for a corpus whose only broken anchors are on a superseded artifact: %d entr(y/ies)\n%s", len(entries), combined)
		}
	}
}
