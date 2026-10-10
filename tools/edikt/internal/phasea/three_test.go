package phasea

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// SPEC-013 SR-011 — `phasea plan --artifact` plans that one artifact whether or
// not the stale detector would have picked it, with reason "requested".
func TestPlan_ArtifactRequested_PlansAFreshArtifact(t *testing.T) {
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	md := writeADR(t, root, "ADR-900", gateQuote)
	// A fresh, valid sidecar: the stale detector has nothing to plan here.
	writeMarshalledSidecar(t, sidecarPathFor(md), &sidecar.Sidecar{
		SchemaVersion: 2, Topic: "testing", Path: "docs/architecture/decisions/ADR-900-test.md",
		Directives: []sidecar.Directive{anchoredAtGateQuote("The gate MUST hold.")},
	})
	if out, _ := Plan(PlanOptions{ProjectRoot: root}); out.ExitCode != ExitNothingToPlan {
		t.Fatalf("control: a fresh corpus should have nothing to plan, got exit %d: %s", out.ExitCode, out.Message)
	}
	out, err := Plan(PlanOptions{ProjectRoot: root, Artifact: "docs/architecture/decisions/ADR-900-test.md"})
	if err != nil || out.ExitCode != ExitPlanEmitted {
		t.Fatalf("--artifact: exit %d, err %v: %s", out.ExitCode, err, out.Message)
	}
	if len(out.Plan.Items) != 1 || out.Plan.Items[0].Reason != ReasonRequested {
		t.Fatalf("want one item with reason %q, got %+v", ReasonRequested, out.Plan.Items)
	}
	if _, err := os.Stat(PinSnapshotPath(root, out.Plan.PlanID, out.Plan.Items[0].ItemID)); err != nil {
		t.Fatalf("a requested item with a prior sidecar must capture its pin snapshot: %v", err)
	}
}

func TestPlan_ArtifactRequested_UnknownPathRefuses(t *testing.T) {
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	writeADR(t, root, "ADR-900", gateQuote)
	out, _ := Plan(PlanOptions{ProjectRoot: root, Artifact: "docs/architecture/decisions/ADR-999-nope.md"})
	if out.ExitCode != ExitRefused || !strings.Contains(out.Message, "ADR-999-nope.md") {
		t.Fatalf("an unknown --artifact must refuse and name the path; got exit %d: %s", out.ExitCode, out.Message)
	}
}

// SPEC-013 SR-012 — the cap is a plan field, from phasea.max_concurrency.
func TestPlan_BudgetCarriesMaxConcurrency(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		want         int
	}{
		{"default", "", DefaultMaxConcurrency},
		{"configured", "phasea:\n  max_concurrency: 3\n", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			isolate(t)
			writeExtractorAgent(t, root, 3)
			if tc.config != "" {
				writeConfig(t, root, tc.config)
			}
			writeADR(t, root, "ADR-900", gateQuote)
			out, err := Plan(PlanOptions{ProjectRoot: root})
			if err != nil || out.ExitCode != ExitPlanEmitted {
				t.Fatalf("Plan: exit %d, %v", out.ExitCode, err)
			}
			if out.Plan.Budget.MaxConcurrency != tc.want {
				t.Fatalf("budget.max_concurrency = %d, want %d", out.Plan.Budget.MaxConcurrency, tc.want)
			}
			if out.Plan.SchemaVersion != SchemaVersionPlan {
				t.Fatalf("plan schema_version = %d, want %d", out.Plan.SchemaVersion, SchemaVersionPlan)
			}
		})
	}
}

func TestLoadConfig_MaxConcurrency_RefusesNonPositive(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, "phasea:\n  max_concurrency: 0\n")
	if _, err := LoadConfig(root); err == nil || !strings.Contains(err.Error(), "phasea.max_concurrency") {
		t.Fatalf("a non-positive cap must be refused by name; got %v", err)
	}
}

// SPEC-013 SR-013 — a dispatch the dispatcher reports as dead is `stalled`,
// retryable, attempts incremented; without the report the same item is
// `output-missing` (the default; stalled is the classified case).
func TestVerify_StalledFromDispatcherReport(t *testing.T) {
	f := newFixture(t, "")
	// No sidecar written: the extractor never produced output.
	out, _ := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: f.planID})
	if it := onlyItem(t, out); it.Status != StatusOutputMissing {
		t.Fatalf("control: without a dispatcher report the item is output-missing, got %q", it.Status)
	}
	f2 := newFixture(t, "")
	out2, _ := Verify(VerifyOptions{ProjectRoot: f2.root, PlanID: f2.planID, Stalled: []string{f2.itemID}})
	it := onlyItem(t, out2)
	if it.Status != StatusStalled {
		t.Fatalf("with the dispatcher's report the item is stalled, got %q", it.Status)
	}
	if it.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", it.Attempts)
	}
	if out2.ExitCode != ExitVerifyIncomplete {
		t.Fatalf("stalled is retryable: exit %d, want %d", out2.ExitCode, ExitVerifyIncomplete)
	}
	if IsTerminalStatus(StatusStalled) {
		t.Fatal("stalled must not be terminal")
	}
	if out2.Receipt.SchemaVersion != SchemaVersionReceipt {
		t.Fatalf("receipt schema_version = %d, want %d", out2.Receipt.SchemaVersion, SchemaVersionReceipt)
	}
}

// A stalled report on an item that DID produce output is not honoured: the
// output is the evidence, the report is not (SR-013's falsifying observation).
func TestVerify_StalledReportDoesNotOverrideOutput(t *testing.T) {
	f := newFixture(t, "")
	writeMarshalledSidecar(t, f.scPath, &sidecar.Sidecar{
		SchemaVersion: 2, Topic: "testing", Path: "docs/architecture/decisions/ADR-900-test.md",
		Directives: []sidecar.Directive{anchoredAtGateQuote("The gate MUST hold.")},
	})
	out, _ := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: f.planID, Stalled: []string{f.itemID}})
	if it := onlyItem(t, out); it.Status == StatusStalled {
		t.Fatal("an item with output on disk was marked stalled on the dispatcher's word alone")
	}
}

// SPEC-013 SR-014 — the overlap report from extractor write times, labelled
// approximate, never changing the exit code. The times reach verify through a
// REAL restore: the clustered writes fire the line after restore has rewritten
// both sidecars.
func TestVerify_OverlapReport_Approximate(t *testing.T) {
	root, plan, mds := overlapProject(t)
	now := time.Now()
	for _, md := range mds {
		writeExtractionAt(t, root, md, now)
	}
	if r, err := Restore(RestoreOptions{ProjectRoot: root, PlanID: plan.PlanID}); err != nil || r.ExitCode != ExitRestoreOK {
		t.Fatalf("restore: %v %+v", err, r)
	}
	v, _ := Verify(VerifyOptions{ProjectRoot: root, PlanID: plan.PlanID})
	if v.ExitCode != ExitVerifyOK {
		t.Fatalf("the overlap report must not change the exit code: %d — %s", v.ExitCode, v.Message)
	}
	if !strings.Contains(v.Message, "approximate") || !strings.Contains(v.Message, "cap of 1") ||
		!strings.Contains(v.Message, "recorded by restore") {
		t.Fatalf("expected an approximate overlap line from restore-recorded write times naming the cap; got:\n%s", v.Message)
	}
}

// Control for the overlap line: write times spread beyond the window produce
// no line, so the report cannot fire on every multi-item plan. The exit code
// is asserted because this control used to stop at verify's restore-receipt
// refusal and never reach the report it claimed to control.
func TestVerify_OverlapReport_SilentWhenSpread(t *testing.T) {
	root, plan, mds := overlapProject(t)
	now := time.Now()
	times := map[string]string{}
	for i, md := range mds {
		at := now.Add(-time.Duration(i) * 10 * time.Minute)
		writeExtractionAt(t, root, md, now)
		times[plan.Items[i].ItemID] = at.UTC().Format(time.RFC3339Nano)
	}
	if err := writeRestoreReceipt(root, plan.PlanID, &RestoreOutcome{ExitCode: ExitRestoreOK, OutputsWrittenAt: times}); err != nil {
		t.Fatalf("planting the restore receipt: %v", err)
	}
	v, _ := Verify(VerifyOptions{ProjectRoot: root, PlanID: plan.PlanID})
	if v.ExitCode != ExitVerifyOK {
		t.Fatalf("the control must reach the overlap report: verify exit %d\n%s", v.ExitCode, v.Message)
	}
	if strings.Contains(v.Message, "concurrency") {
		t.Fatalf("writes ten minutes apart must not produce an overlap line:\n%s", v.Message)
	}
}
