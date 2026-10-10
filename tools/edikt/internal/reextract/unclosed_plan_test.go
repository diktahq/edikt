package reextract

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
)

// TestReextract_ResumeAfterPartialDispatchRefusesUnclosedPlan — c1-d through
// `gov reextract`. The fan-out rewrote some sidecars and the session died
// before `phasea restore`. A resume re-plans the same artifacts; at HEAD it
// derived the SAME plan_id (the corpus line ignored the sidecar bytes) and
// rewrote that plan's pins with the extractor's unrestored output, so every
// pinned field and manual directive the restore would have reinstated was
// gone. The resume must refuse, name the open plan, and leave its pins alone.
func TestReextract_ResumeAfterPartialDispatchRefusesUnclosedPlan(t *testing.T) {
	root := newCorpus(t, 3)
	first, err := Run(baseOpts(root))
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.PlanID == "" || first.Dispatched != 3 {
		t.Fatalf("setup: first run planned %d item(s) under %q", first.Dispatched, first.PlanID)
	}
	pins := map[string][]byte{}
	for i := 1; i <= 3; i++ {
		id := adrID(i)
		b, rerr := os.ReadFile(phasea.PinSnapshotPath(root, first.PlanID, id))
		if rerr != nil {
			t.Fatalf("setup: no pin for %s: %v", id, rerr)
		}
		pins[id] = b
	}
	// Two extractors finished, then the session died: no restore, no verify.
	extractInto(t, root, first.PlanID, 2)

	res, rerr := Run(baseOpts(root))
	if rerr == nil {
		t.Fatalf("resume over unrestored output of %s succeeded (planned %d under %q) — the open plan's "+
			"pins are the only copy of what restore must reinstate", first.PlanID, res.Dispatched, res.PlanID)
	}
	for _, want := range []string{first.PlanID, "edikt phasea restore --plan " + first.PlanID} {
		if !strings.Contains(rerr.Error(), want) {
			t.Errorf("the refusal must name %q:\n%v", want, rerr)
		}
	}
	for id, want := range pins {
		got, gerr := os.ReadFile(phasea.PinSnapshotPath(root, first.PlanID, id))
		if gerr != nil || !bytes.Equal(got, want) {
			t.Errorf("%s's pin under %s was rewritten by the refused resume", id, first.PlanID)
		}
	}
}
