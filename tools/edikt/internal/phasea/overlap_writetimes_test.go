package phasea

// overlap_writetimes_test.go — PLAN-v090-issue-fixes Phase 3 (#11 comment
// item 5, c5-b). SR-014's concurrency line read each ok sidecar's mtime at
// verify time. Restore stamps every sidecar it restores and verify stamps
// every ok one, so by then each mtime was the moment one of those two ran and
// every multi-item plan read as one burst. Restore now records the
// extractor's write times before it writes anything, and verify reads those.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// ── AC-3.4 ───────────────────────────────────────────────────────────────

// overlapProject is a two-item plan with a concurrency cap of 1 and no
// sidecars yet.
func overlapProject(t *testing.T) (root string, plan *DispatchPlan, mds []string) {
	t.Helper()
	root = t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	writeConfig(t, root, "phasea:\n  max_concurrency: 1\n")
	mds = []string{writeADR(t, root, "ADR-901", gateQuote), writeADR(t, root, "ADR-902", gateQuote)}
	out, err := Plan(PlanOptions{ProjectRoot: root})
	if err != nil || out.ExitCode != ExitPlanEmitted || len(out.Plan.Items) != 2 {
		t.Fatalf("plan: %v %+v", err, out)
	}
	return root, out.Plan, mds
}

func writeExtractionAt(t *testing.T, root, md string, at time.Time) {
	t.Helper()
	sc := sidecarPathFor(md)
	writeMarshalledSidecar(t, sc, &sidecar.Sidecar{
		SchemaVersion: 2, Topic: "testing", Path: relPath(root, md),
		Directives: []sidecar.Directive{anchoredAtGateQuote("The gate MUST hold.")},
	})
	if err := os.Chtimes(sc, at, at); err != nil {
		t.Fatal(err)
	}
}

// TestVerify_OverlapReport_SilentAfterRealRestore — c5-b. The extractor's
// writes were ten minutes apart; restore then rewrote both within the same
// second. Read from mtimes, every multi-item plan looked like a burst.
func TestVerify_OverlapReport_SilentAfterRealRestore(t *testing.T) {
	root, plan, mds := overlapProject(t)
	now := time.Now()
	for i, md := range mds {
		writeExtractionAt(t, root, md, now.Add(-time.Duration(i+1)*10*time.Minute))
	}
	if r, err := Restore(RestoreOptions{ProjectRoot: root, PlanID: plan.PlanID}); err != nil || r.ExitCode != ExitRestoreOK {
		t.Fatalf("restore: %v %+v", err, r)
	}
	v, _ := Verify(VerifyOptions{ProjectRoot: root, PlanID: plan.PlanID})
	if v.ExitCode != ExitVerifyOK {
		t.Fatalf("setup: verify exit %d\n%s", v.ExitCode, v.Message)
	}
	if strings.Contains(v.Message, "concurrency") {
		t.Fatalf("writes ten minutes apart were reported as overlapping after restore rewrote them:\n%s", v.Message)
	}
}

// TestVerify_OverlapReport_UnmeasuredOnLegacyReceipt — a receipt written
// before write times were recorded has none to compare. That is "not
// measured", never silence and never a count read off restore's own writes.
func TestVerify_OverlapReport_UnmeasuredOnLegacyReceipt(t *testing.T) {
	root, plan, mds := overlapProject(t)
	now := time.Now()
	for _, md := range mds {
		writeExtractionAt(t, root, md, now)
	}
	legacy, _ := json.Marshal(map[string]any{
		"schema_version": 1, "plan_id": plan.PlanID,
		"restored_at": now.UTC().Format(time.RFC3339), "pins_restored": 0,
	})
	if err := os.MkdirAll(stateDirFor(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(RestoreReceiptPath(root, plan.PlanID), legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	v, _ := Verify(VerifyOptions{ProjectRoot: root, PlanID: plan.PlanID})
	if v.ExitCode != ExitVerifyOK {
		t.Fatalf("setup: verify exit %d\n%s", v.ExitCode, v.Message)
	}
	if !strings.Contains(v.Message, "concurrency: not measured") {
		t.Fatalf("a legacy receipt carries no write times; the report must say not measured:\n%s", v.Message)
	}
	if strings.Contains(v.Message, "approximate") {
		t.Fatalf("an overlap was reported from times nobody recorded:\n%s", v.Message)
	}
}

// TestRestore_RecordsOutputWriteTimesBeforeMutating — the time recorded is
// the extractor's, captured before restore's stamp moves the mtime.
func TestRestore_RecordsOutputWriteTimesBeforeMutating(t *testing.T) {
	root, plan, mds := overlapProject(t)
	at := time.Now().Add(-5 * time.Minute)
	writeExtractionAt(t, root, mds[0], at)
	writeExtractionAt(t, root, mds[1], at.Add(time.Minute))

	if r, err := Restore(RestoreOptions{ProjectRoot: root, PlanID: plan.PlanID}); err != nil || r.ExitCode != ExitRestoreOK {
		t.Fatalf("restore: %v %+v", err, r)
	}
	st, err := os.Stat(sidecarPathFor(mds[0]))
	if err != nil {
		t.Fatal(err)
	}
	if !st.ModTime().After(at.Add(time.Minute)) {
		t.Fatal("setup: restore did not rewrite the sidecar, so this measures nothing about ordering")
	}
	rr := mustRestoreReceipt(t, root, plan.PlanID)
	for i, want := range []time.Time{at, at.Add(time.Minute)} {
		id := plan.Items[i].ItemID
		got, ok := rr.OutputsWrittenAt[id]
		if !ok {
			t.Errorf("%s: no extractor write time recorded", id)
			continue
		}
		if got != want.UTC().Format(time.RFC3339Nano) {
			t.Errorf("%s: recorded %s, want the extractor's %s (restore's own write is %s)",
				id, got, want.UTC().Format(time.RFC3339Nano), st.ModTime().UTC().Format(time.RFC3339Nano))
		}
	}
	if _, perr := time.Parse(time.RFC3339Nano, rr.RestoredAt); perr != nil || !strings.Contains(rr.RestoredAt, ".") {
		t.Errorf("restored_at %q is not RFC3339Nano", rr.RestoredAt)
	}
}

// TestRestore_RerunAndRetryKeepPriorWriteTimes — a restore re-run inside the
// same second, and a retry round that re-dispatches only the failing item,
// both keep the write time of an item nothing rewrote.
func TestRestore_RerunAndRetryKeepPriorWriteTimes(t *testing.T) {
	root, plan, mds := overlapProject(t)
	idA, idB := plan.Items[0].ItemID, plan.Items[1].ItemID
	tA := time.Now().Add(-30 * time.Minute)
	writeExtractionAt(t, root, mds[0], tA) // B's dispatch produced nothing
	wantA := tA.UTC().Format(time.RFC3339Nano)

	if _, err := Restore(RestoreOptions{ProjectRoot: root, PlanID: plan.PlanID}); err != nil {
		t.Fatalf("restore 1: %v", err)
	}
	r2, err := Restore(RestoreOptions{ProjectRoot: root, PlanID: plan.PlanID})
	if err != nil {
		t.Fatalf("restore re-run: %v", err)
	}
	if containsID(r2.Unwritten, idA) {
		t.Fatalf("a restore re-run reported %s unwritten — the first run already restored it:\n%s", idA, r2.Message)
	}
	if got := mustRestoreReceipt(t, root, plan.PlanID).OutputsWrittenAt[idA]; got != wantA {
		t.Fatalf("after a re-run %s's write time is %q, want the extractor's %q", idA, got, wantA)
	}

	v, _ := Verify(VerifyOptions{ProjectRoot: root, PlanID: plan.PlanID})
	if v.ExitCode != ExitVerifyIncomplete {
		t.Fatalf("setup: want %s ok and %s output-missing, got exit %d\n%s", idA, idB, v.ExitCode, v.Message)
	}

	// The retry round re-dispatches B only.
	tB := time.Now().Add(5 * time.Second)
	writeExtractionAt(t, root, mds[1], tB)
	r3, err := Restore(RestoreOptions{ProjectRoot: root, PlanID: plan.PlanID})
	if err != nil {
		t.Fatalf("restore on the retry round: %v", err)
	}
	if containsID(r3.Unwritten, idB) {
		t.Fatalf("the retried item was reported unwritten:\n%s", r3.Message)
	}
	rr := mustRestoreReceipt(t, root, plan.PlanID)
	if rr.OutputsWrittenAt[idA] != wantA {
		t.Errorf("the retry round moved %s's write time to %q, want %q (it was not re-dispatched)", idA, rr.OutputsWrittenAt[idA], wantA)
	}
	if want := tB.UTC().Format(time.RFC3339Nano); rr.OutputsWrittenAt[idB] != want {
		t.Errorf("the retried %s has write time %q, want %q", idB, rr.OutputsWrittenAt[idB], want)
	}
	if _, err := os.Stat(filepath.Join(root, plan.Items[1].ExpectedOutput.Path)); err != nil {
		t.Fatal(err)
	}
}
