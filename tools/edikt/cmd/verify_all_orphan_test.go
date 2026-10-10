package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// verify_all_orphan_test.go — the retention pruner had no orphan pass.
//
// THE DEFECT: pruneVerifyState trimmed per id and never asked whether that id
// should still exist. Measured on this repo, six SUPERSEDED ADRs still held
// their full retained history — ADR-054 (superseded-by-ADR-085), ADR-061,
// ADR-065, ADR-066, ADR-072 (all superseded-by-ADR-083) and ADR-075
// (superseded-by-ADR-081). `gov compile` skips superseded artifacts so it stops
// WRITING new reports; nothing removed the old ones.
//
// Dead disk is the mild half. Reports are keyed by BARE id
// (`gov-ADR-001-all-<stamp>`), and this repo has already carried two different
// ADR-084s on two branches (D330/D333). Under a reused id a stale report
// attaches to the NEW artifact, and doctor's latest-report lookup reports the
// old artifact's last run as though it belonged to the new one.
//
// The pass is bounded on BOTH sides and both are tested here: an id with no
// live sidecar loses its reports; a walk that could not establish what is live
// removes nothing. The second is the one that matters most — the failure mode
// of getting it wrong is deleting the whole directory.

// orphanSeed writes n report pairs for one id and returns the state dir.
func orphanSeed(t *testing.T, ids map[string]int) string {
	t.Helper()
	stateDir := filepath.Join(t.TempDir(), ".edikt", "state", "verify")
	for id, n := range ids {
		for i := 1; i <= n; i++ {
			seedReport(t, stateDir, id+"-all-202501"+pad2(i)+"T000000Z")
		}
	}
	return stateDir
}

// TestPrune_OrphanedIDIsRemoved — the defect. A report id with no live sidecar
// loses ALL its reports, not merely the ones over the retention bound.
func TestPrune_OrphanedIDIsRemoved(t *testing.T) {
	stateDir := orphanSeed(t, map[string]int{
		"gov-ADR-001": 3, // live
		"gov-ADR-054": 3, // superseded — nothing on disk names it any more
	})
	live := map[string]bool{"gov-ADR-001": true}

	pruned, orphaned := pruneVerifyState(stateDir, 10, live)
	if orphaned != 6 { // 3 stems x 2 files
		t.Errorf("orphaned: got %d, want 6 — a superseded artifact kept its history", orphaned)
	}
	// Retention removed nothing: both ids sit under the bound. Counting the
	// orphan removals as retention would hide "an artifact's entire history
	// went" inside "some old files went".
	if pruned != 0 {
		t.Errorf("pruned: got %d, want 0 — orphan removals must not be counted as retention", pruned)
	}
	if got := stemsFor(t, stateDir, "gov-ADR-054-all-"); len(got) != 0 {
		t.Errorf("the orphaned id still has %d report(s): %v", len(got), got)
	}
	// AND the live id is untouched — an orphan pass that removes everything is
	// not an orphan pass.
	if got := stemsFor(t, stateDir, "gov-ADR-001-all-"); len(got) != 3 {
		t.Errorf("the live id lost reports: got %d want 3\n%v", len(got), got)
	}
}

// TestPrune_OrphanBelowRetentionBoundIsStillRemoved. An orphan with one report
// is under any bound, so a keep-N test alone would spare it forever. The order
// of the two checks inside the pruner is the property.
func TestPrune_OrphanBelowRetentionBoundIsStillRemoved(t *testing.T) {
	stateDir := orphanSeed(t, map[string]int{"gov-ADR-072": 1})
	_, orphaned := pruneVerifyState(stateDir, 10, map[string]bool{"gov-ADR-001": true})
	if orphaned != 2 {
		t.Errorf("orphaned: got %d, want 2 — an orphan under the bound is still an orphan", orphaned)
	}
}

// TestPrune_UnknownLiveSetRemovesNothing — THE SAFETY DIRECTION, and the one a
// mistake here would be unrecoverable from. When the caller could not establish
// which artifacts are live it passes nil, and nothing is orphaned. An empty or
// failed scan reading as "nothing is live" would empty the directory (INV-013
// pointed at data; INV-019's provable-ownership rule from the other side).
func TestPrune_UnknownLiveSetRemovesNothing(t *testing.T) {
	stateDir := orphanSeed(t, map[string]int{"gov-ADR-001": 2, "gov-ADR-054": 2})

	pruned, orphaned := pruneVerifyState(stateDir, 10, nil)
	if orphaned != 0 || pruned != 0 {
		t.Fatalf("nil live set removed files: %d pruned, %d orphaned", pruned, orphaned)
	}
	for _, id := range []string{"gov-ADR-001-all-", "gov-ADR-054-all-"} {
		if got := stemsFor(t, stateDir, id); len(got) != 2 {
			t.Errorf("%s: got %d report(s), want 2 — an unmeasured live set must delete nothing", id, len(got))
		}
	}
}

// TestPrune_EmptyLiveSetIsNotNil pins the one boundary that reads ambiguously.
// An empty (non-nil) live set means "the walk ran and found no artifacts",
// which is a measurement, so its reports ARE orphans. nil means "the walk could
// not tell", which is not. The two must not collapse into each other.
func TestPrune_EmptyLiveSetIsNotNil(t *testing.T) {
	stateDir := orphanSeed(t, map[string]int{"gov-ADR-054": 2})
	_, orphaned := pruneVerifyState(stateDir, 10, map[string]bool{})
	if orphaned != 4 { // 2 stems x 2 files
		t.Errorf("an empty-but-measured live set must orphan: got %d, want 4", orphaned)
	}
	if got := stemsFor(t, stateDir, "gov-ADR-054-all-"); len(got) != 0 {
		t.Errorf("reports survived a measured-empty live set: %v", got)
	}
}

// TestPrune_PlanPhaseReportsAreNeverOrphaned. Plan phase reports
// (`<plan>-phase-N-<stamp>`) carry no `-all-` marker, so they fall outside this
// pruner entirely — and must, because they are the evidence a plan's progress
// table cites and `verify all` did not write them. Asserted here as well as in
// the retention test because the orphan pass is a NEW way to reach them: a plan
// id is never in the sidecar live set, so an unscoped orphan pass would take
// every one.
func TestPrune_PlanPhaseReportsAreNeverOrphaned(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), ".edikt", "state", "verify")
	for i := 1; i <= 4; i++ {
		seedReport(t, stateDir, "PLAN-v090-release-phase-1-202501"+pad2(i)+"T000000Z")
	}
	pruned, orphaned := pruneVerifyState(stateDir, 1, map[string]bool{"gov-ADR-001": true})
	if pruned != 0 || orphaned != 0 {
		t.Fatalf("a plan's phase evidence was touched: %d pruned, %d orphaned", pruned, orphaned)
	}
	if got := stemsFor(t, stateDir, "PLAN-v090-release-phase-1-"); len(got) != 4 {
		t.Errorf("plan phase reports removed: %d of 4 survive\n%v", len(got), got)
	}
}

// TestVerifyAll_GovOnlyNeverOrphans — the walk-scope guard. `--gov-only` never
// looks at prd or spec, so its live set cannot speak for them. Running the
// orphan pass from a gov-only walk would classify every prd and spec report as
// orphaned and delete the lot, which is a far worse defect than the one the
// pass fixes.
func TestVerifyAll_GovOnlyNeverOrphans(t *testing.T) {
	root := retentionProject(t)
	stateDir := filepath.Join(root, ".edikt", "state", "verify")
	seedReport(t, stateDir, "spec-SPEC-009-all-20250101T000000Z")
	seedReport(t, stateDir, "prd-PRD-007-all-20250101T000000Z")

	report, err := runVerifyAll(root, true) // govOnly
	if err != nil {
		t.Fatalf("runVerifyAll(gov-only): %v", err)
	}
	if report.Summary.ReportsOrphaned != 0 {
		t.Errorf("a gov-only walk orphaned %d file(s) — it never enumerated prd or spec",
			report.Summary.ReportsOrphaned)
	}
	for _, id := range []string{"spec-SPEC-009-all-", "prd-PRD-007-all-"} {
		if got := stemsFor(t, stateDir, id); len(got) != 1 {
			t.Errorf("%s removed by a gov-only walk: %v", id, got)
		}
	}
}

// TestVerifyAll_SupersededSidecarIsOrphanedEndToEnd — the real shape, through
// runVerifyAll rather than the pruner directly. A skip-listed (superseded)
// artifact is excluded from the walk, so its id is not live, so its reports go.
func TestVerifyAll_SupersededSidecarIsOrphanedEndToEnd(t *testing.T) {
	root := retentionProject(t)
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")

	// A superseded ADR: parent .md carries the status the skip-list matches.
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-054-old.md"),
		[]byte("---\nstatus: superseded-by-ADR-085\n---\n\n# ADR-054\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-054-old.edikt.yaml"),
		[]byte("schema_version: 1\ntopic: \"testing\"\npath: \"docs/architecture/decisions/ADR-054-old.md\"\nsignals: []\ndirectives: []\n"),
		0o644); err != nil {
		t.Fatal(err)
	}

	stateDir := filepath.Join(root, ".edikt", "state", "verify")
	for i := 1; i <= 3; i++ {
		seedReport(t, stateDir, "gov-ADR-054-all-202501"+pad2(i)+"T000000Z")
	}

	report, err := runVerifyAll(root, false)
	if err != nil {
		t.Fatalf("runVerifyAll: %v", err)
	}
	if report.Summary.ReportsOrphaned != 6 {
		t.Errorf("orphaned: got %d, want 6 — a superseded ADR kept its report history",
			report.Summary.ReportsOrphaned)
	}
	if got := stemsFor(t, stateDir, "gov-ADR-054-all-"); len(got) != 0 {
		t.Errorf("superseded ADR still holds %d report(s): %v", len(got), got)
	}
	// The live ADR's fresh report survives — the walk must not have eaten it.
	if got := stemsFor(t, stateDir, "gov-ADR-001-all-"); len(got) != 1 {
		t.Errorf("the live artifact's report went too: %v", got)
	}
}

// TestVerifyAll_UnreadableClassSuppressesTheOrphanPass — the end-to-end safety
// direction. A class that cannot be enumerated for any reason other than "the
// directory is absent" makes the live set untrustworthy, and an untrustworthy
// live set must orphan nothing.
func TestVerifyAll_UnreadableClassSuppressesTheOrphanPass(t *testing.T) {
	root := retentionProject(t)
	stateDir := filepath.Join(root, ".edikt", "state", "verify")
	seedReport(t, stateDir, "gov-ADR-054-all-20250101T000000Z")

	// Make the specs dir unreadable. A permission error is a failure to
	// measure, not a measurement that the class is empty.
	specs := filepath.Join(root, "docs", "product", "specs")
	if err := os.Chmod(specs, 0o000); err != nil {
		t.Skipf("cannot revoke read on %s: %v", specs, err)
	}
	defer os.Chmod(specs, 0o755)
	if _, err := os.ReadDir(specs); err == nil {
		t.Skip("running as a user that ignores directory permissions; the guard cannot be exercised here")
	}

	report, err := runVerifyAll(root, false)
	if err != nil {
		t.Fatalf("runVerifyAll: %v", err)
	}
	if report.Summary.ReportsOrphaned != 0 {
		t.Errorf("orphaned %d file(s) from a walk that could not read one class",
			report.Summary.ReportsOrphaned)
	}
	if got := stemsFor(t, stateDir, "gov-ADR-054-all-"); len(got) != 1 {
		t.Errorf("a failed scan authorised a deletion: %v", got)
	}
}
