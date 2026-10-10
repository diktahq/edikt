package cmd

// verify_all_retention_test.go — AC-12.3's fourth claim.
//
// AC-12.3 carries four independent claims and named one test per claim. Three
// existed. `TestVerifyAll_Retention` did not, the criterion's -run alternation
// was satisfied by the other three, and phase 12 closed recorded "verified
// 4/4" with the retention property never asserted and — as it turned out —
// never implemented. On this repo .edikt/state/verify/ had reached 135,852
// files, 135,250 of them `-all-` reports.
//
// The claim is "keep the newest N reports per id", and every word of it is
// load-bearing, so each is asserted separately:
//
//   - NEWEST — the survivors are the newest, not an arbitrary N. A prune that
//     kept the oldest ten would satisfy a count-only assertion.
//   - N — the bound holds exactly, and an id under the bound is untouched.
//   - PER ID — the budget is per artifact, not a directory-wide cap. A global
//     cap would delete a rarely-run sidecar's only report to make room for a
//     busy one's.
//
// Two things retention must NOT do are asserted too, because both are
// silent-data-loss shapes:
//
//   - a plan's `-phase-` reports are untouched. They are the evidence a plan's
//     progress table cites; `verify all` did not write them.
//   - the .json and .txt halves of a report go together. A surviving .txt
//     whose .json is gone reads as present to one consumer and absent to the
//     other.
//
// FALSIFICATION, run before this test was trusted (each mutation applied to
// pruneVerifyState, test re-run, result recorded):
//   - remove the `out.Summary.ReportsPruned = pruneVerifyState(...)` call
//     entirely -> "id under retention keeps everything: got 16 want 10".
//   - `sort.Sort(sort.Reverse(...))` instead of `sort.Strings` (keep the
//     OLDEST) -> "the newest report for the run's own id was pruned".
//   - drop the `-all-` guard, grouping on the last `-` instead, so plan phase
//     reports fall into the prune -> "plan phase reports were pruned: 10 of 15
//     survive".
//   - remove the `.txt` from the extension loop -> "orphaned .txt left behind".
//
// The third mutant SURVIVED the first draft of this test, which seeded ONE
// plan phase report. Grouping on the last `-` put that lone report in a group
// of one, under the bound, so it was spared for the wrong reason and the test
// passed on a prune that would eat a plan's history. Fifteen are seeded now.
// Recorded because it is the same error the criterion itself made: a check
// that cannot distinguish the property holding from the property being
// unreachable.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// retentionProject builds a project with exactly one gov sidecar, so a
// runVerifyAll writes exactly one fresh report.
func retentionProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-live.md"),
		[]byte("---\nstatus: accepted\n---\n\n# ADR-001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sc := `schema_version: 1
topic: "testing"
path: "docs/architecture/decisions/ADR-001-live.md"
signals: []
directives:
  - text: "Rule MUST hold. (ref: ADR-001)"
    source_excerpt:
      line_start: 1
      line_end: 1
      quote: "x"
    verify: "true"
    verify_kind: structural
`
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-live.edikt.yaml"), []byte(sc), 0o644); err != nil {
		t.Fatal(err)
	}

	// A second LIVE artifact. The per-id assertion below seeds reports under
	// gov-INV-009 and requires them untouched; with the orphan pass in place
	// that only tests what it claims if INV-009 actually exists. Without this
	// the reports would be removed correctly, as orphans, and the test would
	// read as a retention bug.
	invDir := filepath.Join(root, "docs", "architecture", "invariants")
	if err := os.MkdirAll(invDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invDir, "INV-009-live.md"),
		[]byte("---\nstatus: active\n---\n\n# INV-009\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	invSC := strings.ReplaceAll(sc, "ADR-001-live.md", "INV-009-live.md")
	invSC = strings.ReplaceAll(invSC, "decisions", "invariants")
	invSC = strings.ReplaceAll(invSC, "(ref: ADR-001)", "(ref: INV-009)")
	if err := os.WriteFile(filepath.Join(invDir, "INV-009-live.edikt.yaml"), []byte(invSC), 0o644); err != nil {
		t.Fatal(err)
	}

	// prd and spec dirs must EXIST but may be empty: runVerifyAll only runs the
	// orphan pass when it could account for every class, and an unreadable
	// class is not the same as an empty one.
	for _, d := range []string{
		filepath.Join(root, "docs", "product", "prds"),
		filepath.Join(root, "docs", "product", "specs"),
		filepath.Join(root, "docs", "guidelines"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// seedReport writes a report pair under the stem the real writer would use.
func seedReport(t *testing.T, stateDir, stem string) {
	t.Helper()
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{".json", ".txt"} {
		if err := os.WriteFile(filepath.Join(stateDir, stem+ext), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// stemsFor returns the sorted report stems present for one id prefix.
func stemsFor(t *testing.T, stateDir, idPrefix string) []string {
	t.Helper()
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatalf("read state dir: %v", err)
	}
	var got []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, idPrefix) && strings.HasSuffix(n, ".json") {
			got = append(got, strings.TrimSuffix(n, ".json"))
		}
	}
	sort.Strings(got)
	return got
}

func TestVerifyAll_Retention(t *testing.T) {
	root := retentionProject(t)
	stateDir := filepath.Join(root, ".edikt", "state", "verify")

	orig := verifyAllRetainPerID
	verifyAllRetainPerID = 10
	defer func() { verifyAllRetainPerID = orig }()

	// 15 historical runs for the id this walk will also write, stamped in
	// ascending time order. The 2025 prefix keeps every seeded stamp older
	// than the one runVerifyAll is about to write.
	var seeded []string
	for i := 1; i <= 15; i++ {
		stem := "gov-ADR-001-all-" + "202501" + pad2(i) + "T000000Z"
		seedReport(t, stateDir, stem)
		seeded = append(seeded, stem)
	}
	// A second id with only 3 reports: under the bound, so untouched.
	for i := 1; i <= 3; i++ {
		seedReport(t, stateDir, "gov-INV-009-all-202501"+pad2(i)+"T000000Z")
	}
	// A plan's phase history — fifteen re-verifications of phase 1, well
	// over the bound. `verify all` did not write any of them and must not
	// remove them, however old they are. Fifteen and not one: a single
	// phase report survives any grouping rule that happens to isolate it,
	// so it cannot tell a correctly-scoped prune from an unscoped one.
	for i := 1; i <= 15; i++ {
		seedReport(t, stateDir, "PLAN-old-phase-1-202401"+pad2(i)+"T000000Z")
	}

	report, err := runVerifyAll(root, false)
	if err != nil {
		t.Fatalf("runVerifyAll: %v", err)
	}

	// ── N, exactly, for the busy id ──────────────────────────────────────
	kept := stemsFor(t, stateDir, "gov-ADR-001-all-")
	if len(kept) != 10 {
		t.Fatalf("id under retention keeps everything: got %d want 10\n%v", len(kept), kept)
	}

	// ── NEWEST, not any ten ──────────────────────────────────────────────
	// The report this run just wrote is the newest of all; it must survive.
	// The nine survivors beneath it must be the nine newest seeded stems,
	// and none of the six oldest may remain.
	fresh := 0
	for _, k := range kept {
		if !strings.HasPrefix(k, "gov-ADR-001-all-2025") {
			fresh++
		}
	}
	if fresh != 1 {
		t.Errorf("the newest report for the run's own id was pruned: %d fresh report(s) survive\n%v", fresh, kept)
	}
	for _, want := range seeded[6:] { // the 9 newest seeded
		if !hasStem(kept, want) {
			t.Errorf("a newer report was pruned while older ones survived: %s missing\n%v", want, kept)
		}
	}
	for _, gone := range seeded[:6] { // the 6 oldest seeded
		if hasStem(kept, gone) {
			t.Errorf("the oldest reports were kept: %s survived\n%v", gone, kept)
		}
	}

	// ── PER ID, not a directory-wide cap ─────────────────────────────────
	// INV-009 is live and has 3 seeded runs plus the one this walk wrote — all
	// under the bound, so none may go.
	invKept := stemsFor(t, stateDir, "gov-INV-009-all-")
	if len(invKept) != 4 {
		t.Errorf("an id below the bound must be untouched: got %d want 4 (3 seeded + 1 fresh)\n%v",
			len(invKept), invKept)
	}
	for i := 1; i <= 3; i++ {
		want := "gov-INV-009-all-202501" + pad2(i) + "T000000Z"
		if !hasStem(invKept, want) {
			t.Errorf("a seeded run of a live id below the bound was removed: %s\n%v", want, invKept)
		}
	}

	// ── nothing here is an orphan ────────────────────────────────────────
	// Both ids have a live sidecar, so the orphan pass must remove nothing.
	// Without this the retention counts above would still pass if the orphan
	// pass had eaten a live artifact's history and retention simply had less
	// to trim.
	if report.Summary.ReportsOrphaned != 0 {
		t.Errorf("orphan pass removed %d file(s) with every artifact live",
			report.Summary.ReportsOrphaned)
	}

	// ── a plan's phase evidence is not `verify all`'s to delete ──────────
	if got := stemsFor(t, stateDir, "PLAN-old-phase-1-"); len(got) != 15 {
		t.Errorf("plan phase reports were pruned: %d of 15 survive\n%v", len(got), got)
	}

	// ── both halves go together ──────────────────────────────────────────
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	jsons := map[string]bool{}
	var txts []string
	for _, e := range entries {
		n := e.Name()
		switch {
		case strings.HasSuffix(n, ".json"):
			jsons[strings.TrimSuffix(n, ".json")] = true
		case strings.HasSuffix(n, ".txt"):
			txts = append(txts, strings.TrimSuffix(n, ".txt"))
		}
	}
	for _, stem := range txts {
		if !jsons[stem] {
			t.Errorf("orphaned .txt left behind by retention: %s.txt", stem)
		}
	}

	// ── the deletion is reported, never silent ───────────────────────────
	// 6 stems x 2 files.
	if report.Summary.ReportsPruned != 12 {
		t.Errorf("run must report what retention removed: got %d want 12", report.Summary.ReportsPruned)
	}
}

// Retention is IDEMPOTENT: a directory already at the bound loses nothing on
// the next pass, and reports nothing. A prune that removed one more each time
// would drain the history to zero over a week of runs while every individual
// run looked reasonable.
//
// This drives pruneVerifyState directly rather than through runVerifyAll. Two
// runVerifyAll calls in the same second collide on WriteReports' one-second
// timestamp and write the SAME stem, so the second run displaces nothing —
// making the assertion depend on whether the test crossed a second boundary.
// The property under test is retention's, not the clock's.
func TestVerifyAll_RetentionIsIdempotent(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), ".edikt", "state", "verify")
	for i := 1; i <= 5; i++ {
		seedReport(t, stateDir, "gov-ADR-001-all-202501"+pad2(i)+"T000000Z")
	}

	// live == nil throughout: the orphan pass is off, so this exercises
	// retention alone, exactly as before the orphan pass existed.
	if got, orph := pruneVerifyState(stateDir, 3, nil); got != 4 || orph != 0 { // 2 stems x 2 files
		t.Fatalf("first pass: got %d pruned / %d orphaned, want 4 / 0", got, orph)
	}
	if got, _ := pruneVerifyState(stateDir, 3, nil); got != 0 {
		t.Errorf("a directory already at the bound must lose nothing: %d file(s) removed", got)
	}
	if got := stemsFor(t, stateDir, "gov-ADR-001-all-"); len(got) != 3 {
		t.Errorf("the bound did not hold across passes: got %d want 3\n%v", len(got), got)
	}
	// keep <= 0 is "retention off", never "delete everything" — the failure
	// mode of a mis-set knob has to be inaction, not data loss.
	if got, _ := pruneVerifyState(stateDir, 0, nil); got != 0 {
		t.Errorf("keep=0 must disable retention, not empty the directory: %d file(s) removed", got)
	}
	if got := stemsFor(t, stateDir, "gov-ADR-001-all-"); len(got) != 3 {
		t.Errorf("keep=0 removed reports: got %d want 3\n%v", len(got), got)
	}
}

func pad2(i int) string {
	if i < 10 {
		return "0" + string(rune('0'+i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}

func hasStem(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}
