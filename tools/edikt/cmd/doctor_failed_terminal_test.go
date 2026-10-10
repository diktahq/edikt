package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
)

func writeReceiptFixture(t *testing.T, root string, r phasea.Receipt) {
	t.Helper()
	dir := filepath.Join(root, ".edikt", "state", "phasea")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, r.PlanID+".receipt.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// AC-7.5 — an unconsumed plan with a failed-terminal item (a host-rejected
// pin being one cause) is a doctor RED, distinct from the informational
// drift checks above.
func TestDoctorFailedTerminal_IsRed(t *testing.T) {
	root := t.TempDir()
	writeReceiptFixture(t, root, phasea.Receipt{
		SchemaVersion: 3,
		PlanID:        "pap-aaaaaaaaaaaa",
		Consumed:      false,
		VerifiedAt:    "2026-09-04T00:00:00Z",
		Items: []phasea.ReceiptItem{
			{ItemID: "ADR-900", Status: phasea.StatusFailedTerminal, Attempts: 1,
				Diagnostic: "the host rejected the explicit extractor.model pin: model not available"},
		},
	})

	notes := checkFailedTerminalReceipts(root)
	if len(notes) != 1 {
		t.Fatalf("want 1 failed-terminal note, got %d: %+v", len(notes), notes)
	}
	if notes[0].PlanID != "pap-aaaaaaaaaaaa" || notes[0].ItemID != "ADR-900" {
		t.Fatalf("note = %+v", notes[0])
	}

	var out bytes.Buffer
	n := runFailedTerminalCheck(root, &out)
	if n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
	if !strings.Contains(out.String(), "ERROR:") {
		t.Fatalf("a failed-terminal item must be reported as ERROR (doctor RED), got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "rejected the explicit extractor.model pin") {
		t.Fatalf("report must quote the diagnostic, got:\n%s", out.String())
	}
}

// A consumed plan (already resolved, or every item eventually reached ok)
// is not reported even if it happens to carry a failed-terminal entry from
// an earlier attempt — consumed:true is the receipt's own terminal state.
func TestDoctorFailedTerminal_SilentWhenConsumed(t *testing.T) {
	root := t.TempDir()
	writeReceiptFixture(t, root, phasea.Receipt{
		SchemaVersion: 3,
		PlanID:        "pap-bbbbbbbbbbbb",
		Consumed:      true,
		VerifiedAt:    "2026-09-04T00:00:00Z",
		Items: []phasea.ReceiptItem{
			{ItemID: "ADR-901", Status: phasea.StatusOK, Attempts: 1},
		},
	})

	if notes := checkFailedTerminalReceipts(root); len(notes) != 0 {
		t.Fatalf("want no notes for a consumed plan, got: %+v", notes)
	}
}

// Control: an ok-only, unconsumed receipt reports nothing.
func TestDoctorFailedTerminal_SilentWhenNoFailedTerminalItem(t *testing.T) {
	root := t.TempDir()
	writeReceiptFixture(t, root, phasea.Receipt{
		SchemaVersion: 3,
		PlanID:        "pap-cccccccccccc",
		Consumed:      false,
		VerifiedAt:    "2026-09-04T00:00:00Z",
		Items: []phasea.ReceiptItem{
			{ItemID: "ADR-902", Status: phasea.StatusOutputMissing, Attempts: 1, Diagnostic: "no sidecar on disk"},
		},
	})

	if notes := checkFailedTerminalReceipts(root); len(notes) != 0 {
		t.Fatalf("want no notes for a retryable (non-terminal) item, got: %+v", notes)
	}
}
