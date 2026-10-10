package reextract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
)

// AC-13.14. `gov reextract --force` re-extracted NOTHING for weeks and reported
// that it had: five snapshots written, "4 dispatched, 0 succeeded, 0 failed"
// printed, no plan emitted, tree clean (D425).
//
// Cause: reextract built its own candidate list and then called phasea.Plan
// with only ProjectRoot/Force/Only, so Plan ignored it and re-derived a
// STALE-ONLY work-list. After a CONTRACT change nothing is stale — every anchor
// still matches, which is the entire reason this verb exists separately — so the
// plan came back empty and the run exited 0.
//
// These assert the two halves separately, because they are different failures:
// a verb that does nothing, and a verb that MISREPORTS doing nothing. Fixing
// only the second would have left a silent no-op with an honest count.

func nonStaleCorpus(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("CLAUDE_HOME", t.TempDir())
	mk := func(p, body string) {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk(".edikt/config.yaml", "version: 0.9.0\n")
	mk(".claude/agents/sidecar-extractor.md",
		"---\nname: sidecar-extractor\nmodel: claude-opus-5\nprompt_version: 9\n---\nbody\n")
	for _, id := range []string{"ADR-001", "ADR-002"} {
		mk("docs/architecture/decisions/"+id+"-t.md",
			"# T\n\n**Status:** Accepted\n\nSome prose line here.\n")
		// Anchors match the parent exactly: NOTHING here is stale.
		mk("docs/architecture/decisions/"+id+"-t.edikt.yaml",
			"schema_version: 2\ntopic: testing\npath: docs/architecture/decisions/"+id+"-t.md\n"+
				"directives:\n  - text: \"A thing MUST hold. (ref: "+id+")\"\n"+
				"    source_excerpts:\n      - line_start: 5\n        line_end: 5\n"+
				"        quote: \"Some prose line here.\"\n")
	}
	return root
}

func TestReextract_PlansACorpusWhereNothingIsStale(t *testing.T) {
	root := nonStaleCorpus(t)

	out, err := phasea.Plan(phasea.PlanOptions{
		ProjectRoot: root,
		Force:       true,
		Artifacts: []string{
			filepath.Join(root, "docs/architecture/decisions/ADR-001-t.md"),
			filepath.Join(root, "docs/architecture/decisions/ADR-002-t.md"),
		},
	})
	if err != nil {
		t.Fatalf("Plan(): %v", err)
	}
	if out.ExitCode != phasea.ExitPlanEmitted || out.Plan == nil {
		t.Fatalf("a re-extraction of a NON-STALE corpus must still emit a plan — that is the whole\n"+
			"point of the verb, and it returned nothing for weeks while reporting dispatches.\n"+
			"exit %d: %s", out.ExitCode, out.Message)
	}
	if len(out.Plan.Items) != 2 {
		t.Fatalf("expected both artifacts planned, got %d", len(out.Plan.Items))
	}
	for _, it := range out.Plan.Items {
		if it.Reason != phasea.ReasonRequested {
			t.Errorf("item %s: reason %q, want %q — a contract replan is not a staleness finding",
				it.ItemID, it.Reason, phasea.ReasonRequested)
		}
	}
}

func TestReextract_StaleOnlyPlanningWouldFindNothingHere(t *testing.T) {
	// The control that makes the test above mean something. Without Artifacts,
	// the same corpus plans NOTHING — so the pass above is caused by passing the
	// candidate list, not by the corpus happening to be stale.
	root := nonStaleCorpus(t)

	out, err := phasea.Plan(phasea.PlanOptions{ProjectRoot: root, Force: true})
	if err != nil && out == nil {
		t.Fatalf("Plan(): %v", err)
	}
	if out.ExitCode == phasea.ExitPlanEmitted && out.Plan != nil && len(out.Plan.Items) > 0 {
		t.Fatalf("this corpus is NOT stale, so stale-only planning must find nothing; it planned %d.\n"+
			"If this ever fires, the fixture drifted and the test above proves less than it claims.",
			len(out.Plan.Items))
	}
}
