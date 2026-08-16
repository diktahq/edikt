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
	r := &fakeRunner{}
	opts := baseOpts(root, r)
	opts.SkipFixtureProof = false

	if _, err := Run(opts); err != nil {
		t.Fatalf("Run: %v", err)
	}

	id := adrID(1)
	snap := SnapshotPath(root, id)
	b, err := os.ReadFile(snap)
	if err != nil {
		t.Fatalf("snapshot not written at %s: %v", snap, err)
	}

	// The snapshot must be the PRE-rewrite content, not the post-rewrite
	// content the dispatched sidecar now holds.
	wantPre := "schema_version: 2\nartifact: " + id + "\nregenerated: false\n"
	if string(b) != wantPre {
		t.Fatalf("snapshot content = %q, want pre-rewrite content %q", b, wantPre)
	}

	live := filepath.Join(root, "docs/architecture/decisions", id+"-fixture.edikt.yaml")
	liveB, err := os.ReadFile(live)
	if err != nil {
		t.Fatalf("live sidecar: %v", err)
	}
	wantPost := "schema_version: 2\nartifact: " + id + "\nregenerated: true\n"
	if string(liveB) != wantPost {
		t.Fatalf("live sidecar = %q, want post-rewrite content %q — the snapshot and live copy must differ after dispatch, or the review has nothing to show", liveB, wantPost)
	}
	if string(b) == string(liveB) {
		t.Fatal("snapshot equals live sidecar after dispatch — snapshotting must have run AFTER rewrite, not before")
	}
}

func TestRun_DoesNotSnapshotAlreadyDoneArtifacts(t *testing.T) {
	// Snapshotting is scoped to `tasks` (what THIS run dispatches), not all
	// eligible pairs. An artifact already done and not re-dispatched should
	// get no fresh snapshot — one would imply "this run touched it," which
	// would be false.
	root := newCorpus(t, 2)
	r := &fakeRunner{}
	opts := baseOpts(root, r)
	opts.SkipFixtureProof = false

	if _, err := Run(opts); err != nil {
		t.Fatalf("first run: %v", err)
	}

	id := adrID(1)
	snap := SnapshotPath(root, id)
	firstStat, err := os.Stat(snap)
	if err != nil {
		t.Fatalf("snapshot missing after first run: %v", err)
	}
	firstModTime := firstStat.ModTime()

	// Second run: everything is already done (same prompt version, unchanged
	// sidecars), so nothing should be re-dispatched or re-snapshotted.
	r2 := &fakeRunner{}
	opts2 := baseOpts(root, r2)
	opts2.SkipFixtureProof = false
	res, err := Run(opts2)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Dispatched != 0 {
		t.Fatalf("second run dispatched %d; want 0 (everything already done)", res.Dispatched)
	}

	secondStat, err := os.Stat(snap)
	if err != nil {
		t.Fatalf("snapshot vanished between runs: %v", err)
	}
	if !secondStat.ModTime().Equal(firstModTime) {
		t.Fatal("snapshot was rewritten on a run that dispatched nothing for this artifact")
	}
}
