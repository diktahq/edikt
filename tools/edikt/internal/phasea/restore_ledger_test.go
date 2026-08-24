package phasea

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// AC-4.3 — the per-completion ledger hook must fire PER ARTIFACT, not once at
// the end of the batch.
//
// This is the behaviour D4 required to have a named new home before the
// dispatcher could be deleted. It lived on recordingRunner.onDone, which ran
// as each dispatch finished; its home now is RestoreOptions.OnItemRestored.
//
// The property matters because of resume. A run killed halfway must leave the
// items it already finished recorded, so the next run does not re-extract
// them — re-extraction is the expensive thing the ledger exists to avoid. A
// batch-end write loses precisely the work that cost the most to do.
//
// "Per item" is asserted by observing, from inside the FIRST item's callback,
// that the SECOND item's restore work has not happened yet.
//
// A weaker version of this test checked only that the first item's ledger
// line was on disk during the second callback. That passes against a batched
// implementation too — batched callbacks still run in sequence, so the second
// one still sees the first one's write. It proved ordering among callbacks
// and nothing about when the work happened. Verified by mutation: batching
// every notification to the end of the loop left it green.
//
// The discriminator is the sidecar of the item that has not been reached yet.
// Per-item, item[1] is still unstamped when item[0]'s hook fires. Batched,
// every item is fully restored before any hook fires, so item[1] already
// carries its body digest.
func TestLedgerPersistsPerArtifactOnMidBatchKill(t *testing.T) {
	f := newTwoItemFixture(t)

	secondSidecar := sidecarPathFor(f.secondMD)
	before, rerr := os.ReadFile(secondSidecar)
	if rerr != nil {
		t.Fatalf("fixture: second sidecar unreadable: %v", rerr)
	}

	var seen []string
	secondUntouchedAtFirstHook := false

	_, err := Restore(RestoreOptions{
		ProjectRoot: f.root,
		PlanID:      f.planID,
		OnItemRestored: func(item PlanItem, _ error) {
			if len(seen) == 0 {
				now, nerr := os.ReadFile(secondSidecar)
				// Unchanged means the loop has not reached item[1] yet, which
				// is only true if the hook fires as each item completes.
				secondUntouchedAtFirstHook = nerr == nil && string(now) == string(before)
			}
			seen = append(seen, item.ItemID)
		},
	})
	if err != nil {
		t.Fatalf("Restore(): %v", err)
	}

	// Guard the denominator. One item cannot demonstrate "per item, not per
	// batch" — with a single callback the two implementations are identical
	// and this would report a green having distinguished nothing.
	if len(seen) < 2 {
		t.Fatalf("hook fired %d time(s) (%v); at least 2 items are needed for this to test anything", len(seen), seen)
	}
	if !secondUntouchedAtFirstHook {
		t.Fatal("the second item was already restored when the first item's hook fired — " +
			"notifications are batched to the end of the loop, so a run killed midway " +
			"records nothing and a resume re-extracts work that was already done")
	}
}

func newTwoItemFixture(t *testing.T) *twoItemFixture {
	t.Helper()
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)

	var mds []string
	for _, id := range []string{"ADR-900", "ADR-901"} {
		md := writeADR(t, root, id, gateQuote)
		mds = append(mds, md)
		sc := filepath.Join(filepath.Dir(md), id+"-test.edikt.yaml")
		writeMarshalledSidecar(t, sc, &sidecar.Sidecar{
			SchemaVersion: 2,
			Topic:         "testing",
			Path:          "docs/architecture/decisions/" + id + "-test.md",
			Directives:    []sidecar.Directive{anchoredAtStaleQuote("Rule for " + id + " MUST hold. (ref: " + id + ")")},
		})
	}

	out, err := Plan(PlanOptions{ProjectRoot: root})
	if err != nil {
		t.Fatalf("Plan(): %v", err)
	}
	if out.ExitCode != ExitPlanEmitted || len(out.Plan.Items) != 2 {
		t.Fatalf("fixture did not produce a two-item plan: exit %d, %s", out.ExitCode, out.Message)
	}
	return &twoItemFixture{
		verifyFixture: verifyFixture{
			root:   root,
			planID: out.Plan.PlanID,
			plan:   out.Plan,
			mdPath: mds[0],
			scPath: sidecarPathFor(mds[0]),
			itemID: out.Plan.Items[0].ItemID,
		},
		secondMD: mds[1],
	}
}

// twoItemFixture adds the second artifact's parent path, which is what the
// per-item assertion observes.
type twoItemFixture struct {
	verifyFixture
	secondMD string
}
