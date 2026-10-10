package reextract

import (
	"path/filepath"
	"testing"
)

// TestReextract_UnwrittenItemIsLedgeredFailedNotDone — PLAN-v090-issue-fixes
// Phase 3 (#8, c2-b). An item whose extractor wrote nothing over a
// gate-passing prior was stamped by restore and recorded `done`, so the batch
// reported a regeneration that never happened and a resume skipped it.
func TestReextract_UnwrittenItemIsLedgeredFailedNotDone(t *testing.T) {
	root := newCorpus(t, 2)
	unwritten := adrID(2)
	// An earlier good extraction: this prior clears the content gate, which
	// is the case restore could not tell from a fresh write.
	mustWrite(t, filepath.Join(root, "docs/architecture/decisions", unwritten+"-fixture.edikt.yaml"),
		regeneratedSidecar("docs/architecture/decisions/"+unwritten+"-fixture.md", unwritten))

	plan, err := Run(baseOpts(root))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if plan.Dispatched != 2 {
		t.Fatalf("setup: planned %d, want both artifacts", plan.Dispatched)
	}
	extractAllExcept(t, root, plan.PlanID, unwritten)

	rec, rerr := Record(RecordOptions{ProjectRoot: root, PlanID: plan.PlanID})
	if rerr == nil {
		t.Error("a batch with an item nothing wrote must report failure")
	}
	if rec == nil || rec.Recorded != 1 {
		t.Fatalf("recorded %+v, want exactly the one artifact the fan-out wrote", rec)
	}
	var l Ledger
	readJSON(t, filepath.Join(root, LedgerRelPath), &l)
	if e := l.Artifacts[unwritten]; e.Status != StatusFailed || e.Err == "" {
		t.Fatalf("%s ledgered %q (err %q), want %q: the extractor wrote nothing for it", unwritten, e.Status, e.Err, StatusFailed)
	}
}
