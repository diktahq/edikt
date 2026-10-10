package phasea

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// A lifted sidecar: the pre-dispatch sidecar carries migration_preserved, so
// the plan reasons stale-migration-preserved and the snapshot holds the
// baseline the extractor was handed (through the migrate-time suffix).
func newLiftedFixture(t *testing.T, preserved []string) *verifyFixture {
	t.Helper()
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	md := writeADR(t, root, "ADR-900", gateQuote)
	sc := filepath.Join(filepath.Dir(md), "ADR-900-test.edikt.yaml")
	writeMarshalledSidecar(t, sc, &sidecar.Sidecar{
		SchemaVersion:      2,
		Topic:              "testing",
		Path:               "docs/architecture/decisions/ADR-900-test.md",
		MigrationPreserved: &sidecar.MigrationPreserved{Directives: preserved},
	})
	out, err := Plan(PlanOptions{ProjectRoot: root})
	if err != nil {
		t.Fatalf("Plan(): %v", err)
	}
	if out.ExitCode != ExitPlanEmitted || len(out.Plan.Items) != 1 {
		t.Fatalf("fixture did not produce a one-item plan: exit %d, %s", out.ExitCode, out.Message)
	}
	if out.Plan.Items[0].Reason != ReasonStaleMigrationPreserved {
		t.Fatalf("fixture item reason is %q, want %q", out.Plan.Items[0].Reason, ReasonStaleMigrationPreserved)
	}
	if _, err := os.Stat(PinSnapshotPath(root, out.Plan.PlanID, out.Plan.Items[0].ItemID)); err != nil {
		t.Fatalf("fixture did not capture a pin snapshot: %v", err)
	}
	// The lossless tests drive Verify too; the restore coupling has its own
	// tests, and the ones that assert the REFUSAL create their fixture
	// without calling this helper.
	if err := writeRestoreReceipt(root, out.Plan.PlanID, &RestoreOutcome{ExitCode: ExitRestoreOK}); err != nil {
		t.Fatalf("planting the restore receipt: %v", err)
	}
	return &verifyFixture{root: root, planID: out.Plan.PlanID, plan: out.Plan, mdPath: md, scPath: sc, itemID: out.Plan.Items[0].ItemID}
}

func verifyWith(t *testing.T, f *verifyFixture, onLoss string) ReceiptItem {
	t.Helper()
	out, _ := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: f.planID, OnLoss: onLoss})
	if out == nil || out.Receipt == nil || len(out.Receipt.Items) != 1 {
		t.Fatalf("verify produced no one-item receipt: %+v", out)
	}
	return out.Receipt.Items[0]
}

const preservedRule = "The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)"

func liftedOutput(t *testing.T, f *verifyFixture, text string) {
	t.Helper()
	writeMarshalledSidecar(t, f.scPath, &sidecar.Sidecar{
		SchemaVersion: 2, Topic: "testing", Path: "docs/architecture/decisions/ADR-900-test.md",
		Directives: []sidecar.Directive{anchoredAtGateQuote(text)},
	})
}

func TestVerify_LosslessLiftedItemDroppingAPreservedDirectiveIsLosslessLoss(t *testing.T) {
	f := newLiftedFixture(t, []string{preservedRule})
	liftedOutput(t, f, "Something else entirely. (ref: ADR-900)")
	it := verifyWith(t, f, "")
	if it.Status != StatusLosslessLoss {
		t.Fatalf("status %q, want %q: %s", it.Status, StatusLosslessLoss, it.Diagnostic)
	}
	if IsTerminalStatus(it.Status) {
		t.Fatal("lossless-loss must stay retryable — a re-dispatch with the suffix is the remedy")
	}
}

func TestVerify_LosslessLiftedItemKeepingThePreservedDirectiveIsOK(t *testing.T) {
	f := newLiftedFixture(t, []string{preservedRule})
	liftedOutput(t, f, preservedRule)
	if it := verifyWith(t, f, ""); it.Status != StatusOK {
		t.Fatalf("status %q, want ok: %s", it.Status, it.Diagnostic)
	}
}

func TestVerify_LosslessOnLossAcceptRecordsTheLossAndPasses(t *testing.T) {
	f := newLiftedFixture(t, []string{preservedRule})
	liftedOutput(t, f, "Something else entirely. (ref: ADR-900)")
	it := verifyWith(t, f, "accept")
	if it.Status != StatusOK || it.Diagnostic == "" {
		t.Fatalf("accept must pass the item WITH the loss on the record: %+v", it)
	}
}

func TestVerify_LosslessLiftedItemWithNoBaselineIsUnmeasured(t *testing.T) {
	f := newLiftedFixture(t, nil)
	liftedOutput(t, f, preservedRule)
	if it := verifyWith(t, f, ""); it.Status != StatusLosslessUnmeasured {
		t.Fatalf("status %q, want %q — nothing measured is not a pass", it.Status, StatusLosslessUnmeasured)
	}
}
