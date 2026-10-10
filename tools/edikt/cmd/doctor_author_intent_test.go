package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
)

// writeIntentFixture lays down an artifact + sidecar and an index.
func writeIntentFixture(t *testing.T, root, id, status, sidecar, indexYAML string) govrun.Config {
	t.Helper()
	dir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, id+"-a-test-artifact")
	md := "# " + id + "\n\n**Status:** " + status + "\n\n## Decision\n\nSomething.\n"
	if err := os.WriteFile(base+".md", []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+".edikt.yaml", []byte(sidecar), 0o644); err != nil {
		t.Fatal(err)
	}
	idxDir := filepath.Join(root, ".claude", "rules", "governance")
	if err := os.MkdirAll(idxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(idxDir, "delivery-map.yaml"), []byte(indexYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	return mkcfg(t, root)
}

const (
	// A binding obligation, on approved globs. Exactly the case that surprises.
	intentSidecar = `schema_version: 2
topic: tooling
path: x.md
signals: [a]
paths:
  - tools/**/*
paths_approval:
  approved_at: 2026-08-13T09:51:38Z
  globs_sha256: deadbeef
directives:
  - text: "Tier-2 helpers MUST be Go modules."
    source_excerpts:
      - line_start: 1
        line_end: 1
        quote: "Something."
  - text: "The launcher MUST verify the signature."
    source_excerpts:
      - line_start: 1
        line_end: 1
        quote: "Something."
`
	// Someone else's artifact is routed, so `routed` is non-empty and the
	// empty-index early return does not mask the result.
	otherRoutedIndex = "schema_version: 1\nglobs:\n  other/**/*:\n    - id: INV-999:d01\n      text: \"X MUST Y.\"\n"
)

// TestAuthorIntentCheckFires — POSITIVE CONTROL, and the one that must fail if
// the count silently goes to zero. A check reporting nothing when there IS
// something is the defect class this corpus keeps producing; this test exists
// to make that failure loud.
func TestAuthorIntentCheckFires(t *testing.T) {
	root := t.TempDir()
	cfg := writeIntentFixture(t, root, "ADR-021", "Accepted", intentSidecar, otherRoutedIndex)

	gaps := advisoryDespiteAuthorIntent(root, cfg,
		filepath.Join(root, ".claude", "rules", "governance", "delivery-map.yaml"))
	if len(gaps) != 1 {
		t.Fatalf("gaps: got %d, want 1 — the check went silent on a corpus that has the defect", len(gaps))
	}
	g := gaps[0]
	// EXACT count, not >0: a predicate that half-works and reports 1 of 2 is
	// the same class of wrong as reporting 0.
	if !strings.Contains(g.Title, "2 directive(s)") {
		t.Errorf("must count BOTH binding directives; got %q", g.Title)
	}
	if !strings.Contains(g.Title, "1 ADR/guideline artifact(s)") {
		t.Errorf("must count the artifact; got %q", g.Title)
	}
	if !strings.Contains(strings.Join(g.Detail, " "), "enforcement: deny") {
		t.Errorf("the remedy must be NAMED, not implied; got %v", g.Detail)
	}
	if !strings.Contains(strings.Join(g.Detail, " "), "ADR-021") {
		t.Errorf("the artifact must be named; got %v", g.Detail)
	}
}

// TestAuthorIntentAmendedAcceptedIsCounted — REGRESSION for a wrong number I
// published. ADR-021's status reads "Accepted (Amended 2026-08-13 — launcher
// clause superseded in practice by ADR-022)". A scan that looks for
// "supersed" anywhere in the status line excludes it, which understated the
// finding as 551/38 when it is 563/39. An amendment note is not a status.
func TestAuthorIntentAmendedAcceptedIsCounted(t *testing.T) {
	root := t.TempDir()
	cfg := writeIntentFixture(t, root, "ADR-021",
		"Accepted (Amended 2026-08-13 — launcher clause superseded in practice by ADR-022)",
		intentSidecar, otherRoutedIndex)

	gaps := advisoryDespiteAuthorIntent(root, cfg,
		filepath.Join(root, ".claude", "rules", "governance", "delivery-map.yaml"))
	if len(gaps) != 1 {
		t.Fatalf("an Accepted-with-amendment ADR must still be counted; got %d gaps", len(gaps))
	}
}

// TestAuthorIntentNegativeControls. Each of these would inflate the count, and
// an inflated count is as useless as a zero one.
func TestAuthorIntentNegativeControls(t *testing.T) {
	idxPath := func(root string) string {
		return filepath.Join(root, ".claude", "rules", "governance", "delivery-map.yaml")
	}

	t.Run("no binding modal", func(t *testing.T) {
		root := t.TempDir()
		sc := strings.ReplaceAll(intentSidecar, "MUST", "should")
		cfg := writeIntentFixture(t, root, "ADR-021", "Accepted", sc, otherRoutedIndex)
		if g := advisoryDespiteAuthorIntent(root, cfg, idxPath(root)); len(g) != 0 {
			t.Errorf("a directive with no binding modal signals no enforcement intent; got %v", g)
		}
	})

	t.Run("no approved paths", func(t *testing.T) {
		root := t.TempDir()
		sc := strings.Split(intentSidecar, "paths:\n")[0] + "directives:\n" +
			strings.Split(intentSidecar, "directives:\n")[1]
		cfg := writeIntentFixture(t, root, "ADR-021", "Accepted", sc, otherRoutedIndex)
		if g := advisoryDespiteAuthorIntent(root, cfg, idxPath(root)); len(g) != 0 {
			t.Errorf("without the approval ceremony there is only one signal, not two; got %v", g)
		}
	})

	t.Run("artifact is routed", func(t *testing.T) {
		root := t.TempDir()
		idx := "schema_version: 1\nglobs:\n  tools/**/*:\n    - id: ADR-021:d01\n      text: \"X MUST Y.\"\n"
		cfg := writeIntentFixture(t, root, "ADR-021", "Accepted", intentSidecar, idx)
		if g := advisoryDespiteAuthorIntent(root, cfg, idxPath(root)); len(g) != 0 {
			t.Errorf("a routed artifact CAN deny and must not be reported; got %v", g)
		}
	})

	t.Run("not accepted", func(t *testing.T) {
		root := t.TempDir()
		cfg := writeIntentFixture(t, root, "ADR-021", "Superseded by ADR-022", intentSidecar, otherRoutedIndex)
		if g := advisoryDespiteAuthorIntent(root, cfg, idxPath(root)); len(g) != 0 {
			t.Errorf("a superseded artifact governs nothing; got %v", g)
		}
	})
}

// TestBindingModalRe pins what counts as authorial intent — the RFC-2119 set
// INV-020 requires of a hand-authored directive.
func TestBindingModalRe(t *testing.T) {
	for _, yes := range []string{
		"X MUST be Y.", "X MUST NOT be Y.", "X SHALL be Y.", "X SHALL NOT be Y.", "A REQUIRED field.",
	} {
		if !bindingModalRe.MatchString(yes) {
			t.Errorf("%q must read as binding", yes)
		}
	}
	for _, no := range []string{
		"X should be Y.", "X may be Y.", "We chose Go over Rust.", "X must be Y.", "MUSTARD is yellow.",
	} {
		if bindingModalRe.MatchString(no) {
			t.Errorf("%q must NOT read as binding", no)
		}
	}
}

// TestPathsApprovalNote — the approval-time half of afcce21e's proposal.
//
// The ceremony asks a human a question and records their answer; until now it
// said nothing about what the answer buys for an ADR. This pins WHO gets the
// note and that it names the remedy.
func TestPathsApprovalNote(t *testing.T) {
	for _, id := range []string{"ADR-021", "GL-002"} {
		lines := pathsApprovalNote(id)
		if len(lines) == 0 {
			t.Fatalf("%s must get the note: its directives cannot deny a write", id)
		}
		joined := strings.Join(lines, " ")
		for _, want := range []string{"TOPIC FILE", "advisory by class", "enforcement: deny"} {
			if !strings.Contains(joined, want) {
				t.Errorf("%s note must contain %q; got %q", id, want, joined)
			}
		}
	}
	// NEGATIVE CONTROL, and the one that matters: invariant directives DO
	// route, so the note would be false for them. A caveat that is wrong half
	// the time trains people to skip it.
	for _, id := range []string{"INV-010", "INV-001"} {
		if lines := pathsApprovalNote(id); len(lines) != 0 {
			t.Errorf("%s is path-routed already; the note is false for it: %v", id, lines)
		}
	}
}
