package reextract

import (
	"os"
	"path/filepath"
	"testing"
)

// DESIGN-QUESTIONS-2026-08-16.md Q3, option (a) — the git-independent review
// baseline. Proven on the actual dispatch path (Run), not the snapshot
// helper in isolation: a correct writeSnapshot wired up at the wrong point
// in Run would still leave the review with nothing pre-rewrite to diff
// against.

func TestRun_WritesPreRewriteSnapshot(t *testing.T) {
	root := newCorpus(t, 2)
	opts := baseOpts(root)
	opts.SkipFixtureProof = false

	firstRes, err := Run(opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	id := adrID(1)
	snap := SnapshotPath(root, id)
	b, err := os.ReadFile(snap)
	if err != nil {
		t.Fatalf("snapshot not written at %s: %v", snap, err)
	}

	// The snapshot must be the PRE-rewrite content — the bytes on disk when
	// the run started, which is the whole point of a review baseline. It is
	// compared against the live file rather than a hardcoded string: the
	// fixture's exact bytes are not the subject, "the snapshot is not the
	// post-rewrite content" is.
	live := filepath.Join(root, "docs/architecture/decisions", id+"-fixture.edikt.yaml")
	pre, perr := os.ReadFile(live)
	if perr != nil {
		t.Fatalf("live sidecar: %v", perr)
	}
	if string(b) != string(pre) {
		t.Fatalf("snapshot content = %q, want the pre-rewrite bytes %q", b, pre)
	}

	// Stand in for tier-1's fan-out, then confirm the snapshot did NOT follow
	// the live file. A snapshot that tracked the rewrite leaves the review
	// with nothing to diff against.
	extractInto(t, root, firstRes.PlanID, -1)
	if _, rerr := Record(RecordOptions{ProjectRoot: root, PlanID: firstRes.PlanID}); rerr != nil {
		t.Fatalf("record: %v", rerr)
	}
	liveB, lerr := os.ReadFile(live)
	if lerr != nil {
		t.Fatalf("live sidecar after extraction: %v", lerr)
	}
	if string(b) == string(liveB) {
		t.Fatal("snapshot equals live sidecar after extraction — snapshotting must have run BEFORE the rewrite, not after")
	}
}

func TestRun_DoesNotSnapshotAlreadyDoneArtifacts(t *testing.T) {
	// Snapshotting is scoped to `tasks` (what THIS run dispatches), not all
	// eligible pairs. An artifact already done and not re-dispatched should
	// get no fresh snapshot — one would imply "this run touched it," which
	// would be false.
	root := newCorpus(t, 2)
	opts := baseOpts(root)
	opts.SkipFixtureProof = false

	firstRes, err := Run(opts)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	id := adrID(1)
	snap := SnapshotPath(root, id)
	firstStat, serr := os.Stat(snap)
	if serr != nil {
		t.Fatalf("snapshot missing after first run: %v", serr)
	}
	firstModTime := firstStat.ModTime()

	// Second run: everything is already done (same prompt version, unchanged
	// sidecars), so nothing should be re-planned or re-snapshotted.
	//
	// Run() only plans now, so the corpus has to actually be regenerated
	// between the two runs for "already done" to be true — otherwise the
	// second run would legitimately replan and this would fail for a reason
	// that has nothing to do with snapshots.
	extractInto(t, root, firstRes.PlanID, -1)
	if _, rerr := Record(RecordOptions{ProjectRoot: root, PlanID: firstRes.PlanID}); rerr != nil {
		t.Fatalf("record: %v", rerr)
	}
	opts2 := baseOpts(root)
	opts2.SkipFixtureProof = false
	res, rerr2 := Run(opts2)
	if rerr2 != nil {
		t.Fatalf("second run: %v", rerr2)
	}
	if res.Dispatched != 0 {
		t.Fatalf("second run planned %d; want 0 (everything already done)", res.Dispatched)
	}

	secondStat, err := os.Stat(snap)
	if err != nil {
		t.Fatalf("snapshot vanished between runs: %v", err)
	}
	if !secondStat.ModTime().Equal(firstModTime) {
		t.Fatal("snapshot was rewritten on a run that dispatched nothing for this artifact")
	}
}
