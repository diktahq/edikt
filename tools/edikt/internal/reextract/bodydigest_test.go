package reextract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
)

// AC-4.4 — the ADR-053 body-digest baseline is stamped per completed artifact
// on the reextract path, and a full run leaves zero unmeasured.
//
// This is D4's requirement that the stamp survive the dispatcher's deletion.
// It used to run inside recordingRunner.onDone as each dispatch finished; it
// now runs inside phasea.Restore's per-item loop, reached here through
// Record().
//
// "Zero unmeasured" is the assertion that matters, and it is why this counts
// artifacts WITHOUT a digest rather than checking that some artifact has one.
// The stamp failed silently for the entire corpus during this phase — it was
// handed a relative artifact path while the sidecar path was already resolved
// under root, so every stamp errored and was reported as "reported as
// unmeasured", a phrase calm enough to read as normal output. A test that
// asserted "at least one is stamped" would not have caught it either, because
// the answer was zero for all of them. The count of unmeasured artifacts is
// the only form of this assertion that fails when the mechanism is dead.
func TestBodyDigestStampedPerArtifact(t *testing.T) {
	root := newCorpus(t, 3)

	res, err := Run(baseOpts(root))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if res.Dispatched != 3 {
		t.Fatalf("planned %d artifact(s); want 3 — with nothing planned there is nothing to stamp and this proves nothing", res.Dispatched)
	}

	// Stand in for tier-1's fan-out, then run the pass that stamps.
	extractInto(t, root, res.PlanID, -1)
	rec, rerr := Record(RecordOptions{ProjectRoot: root, PlanID: res.PlanID})
	if rerr != nil {
		t.Fatalf("record: %v", rerr)
	}
	if rec.Recorded != 3 {
		t.Fatalf("recorded %d; want 3", rec.Recorded)
	}

	var unmeasured []string
	for i := 1; i <= 3; i++ {
		id := adrID(i)
		p := filepath.Join(root, "docs/architecture/decisions", id+"-fixture.edikt.yaml")
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			t.Fatalf("sidecar %s unreadable after record: %v", id, readErr)
		}
		if !strings.Contains(string(b), "body_digest:") {
			unmeasured = append(unmeasured, id)
		}
	}
	if len(unmeasured) > 0 {
		t.Fatalf("%d of 3 artifact(s) left unmeasured for body drift after a full run: %v — "+
			"the ADR-053 stamp is not reaching them", len(unmeasured), unmeasured)
	}
}

// The stamp must be reached through the plan's own resolved paths, not a
// caller-relative one. Pins the specific defect above: Restore resolves the
// sidecar under root but was passing item.ArtifactPath through unresolved.
func TestBodyDigestUsesRootResolvedArtifactPath(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "phasea", "restore.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "stampBody(sidecarPath, item.ArtifactPath,") {
		t.Error("restore.go stamps with item.ArtifactPath, which is relative to the project root while " +
			"sidecarPath is already resolved — every stamp fails and reports itself as unmeasured")
	}
	_ = phasea.PlanIDUnresolved // keep the phasea dependency meaningful
}
