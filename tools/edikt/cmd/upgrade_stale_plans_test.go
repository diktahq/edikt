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

func writeStalePlanFixture(t *testing.T, root, planID string, schemaVersion int) {
	t.Helper()
	dir := filepath.Join(root, ".edikt", "state", "phasea")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"schema_version": schemaVersion, "plan_id": planID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, planID+".plan.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeStaleReceiptFixture(t *testing.T, root, planID string, consumed bool) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"consumed": consumed})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(phasea.ReceiptFilePath(root, planID), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// AC-7.9 / D57 (4) — an unconsumed plan written by an older schema is
// removed, named on one line; a consumed plan (finished business) and a
// current-schema plan (possibly mid-fan-out) are both left alone.
func TestUpgradeRemovesStalePlans(t *testing.T) {
	t.Run("removes an unconsumed stale plan and names it", func(t *testing.T) {
		root := t.TempDir()
		writeStalePlanFixture(t, root, "pap-aaaaaaaaaaaa", phasea.SchemaVersionPlan-1)

		var out bytes.Buffer
		n, err := removeStalePlans(root, &out)
		if err != nil {
			t.Fatalf("removeStalePlans: %v", err)
		}
		if n != 1 {
			t.Fatalf("removed = %d, want 1", n)
		}
		if !strings.Contains(out.String(), "pap-aaaaaaaaaaaa") {
			t.Fatalf("report must name the removed plan, got:\n%s", out.String())
		}
		if _, err := os.Stat(phasea.PlanFilePath(root, "pap-aaaaaaaaaaaa")); !os.IsNotExist(err) {
			t.Fatal("the stale plan file must be gone")
		}
	})

	t.Run("leaves a consumed stale plan alone", func(t *testing.T) {
		root := t.TempDir()
		writeStalePlanFixture(t, root, "pap-bbbbbbbbbbbb", phasea.SchemaVersionPlan-1)
		writeStaleReceiptFixture(t, root, "pap-bbbbbbbbbbbb", true)

		var out bytes.Buffer
		n, err := removeStalePlans(root, &out)
		if err != nil {
			t.Fatalf("removeStalePlans: %v", err)
		}
		if n != 0 {
			t.Fatalf("removed = %d, want 0 — a consumed plan is finished business, not stale", n)
		}
		if _, err := os.Stat(phasea.PlanFilePath(root, "pap-bbbbbbbbbbbb")); err != nil {
			t.Fatalf("a consumed plan's file must survive: %v", err)
		}
	})

	t.Run("leaves a current-schema plan alone", func(t *testing.T) {
		root := t.TempDir()
		writeStalePlanFixture(t, root, "pap-cccccccccccc", phasea.SchemaVersionPlan)

		var out bytes.Buffer
		n, err := removeStalePlans(root, &out)
		if err != nil {
			t.Fatalf("removeStalePlans: %v", err)
		}
		if n != 0 {
			t.Fatalf("removed = %d, want 0 — a current-schema plan may still be mid-fan-out", n)
		}
	})

	t.Run("cleans up the sibling receipt for a removed stale plan", func(t *testing.T) {
		root := t.TempDir()
		writeStalePlanFixture(t, root, "pap-dddddddddddd", phasea.SchemaVersionPlan-1)
		writeStaleReceiptFixture(t, root, "pap-dddddddddddd", false)

		var out bytes.Buffer
		if _, err := removeStalePlans(root, &out); err != nil {
			t.Fatalf("removeStalePlans: %v", err)
		}
		if _, err := os.Stat(phasea.ReceiptFilePath(root, "pap-dddddddddddd")); !os.IsNotExist(err) {
			t.Fatal("the orphaned receipt must be removed alongside its plan")
		}
	})

	t.Run("no phasea state directory is a no-op, not an error", func(t *testing.T) {
		root := t.TempDir()
		var out bytes.Buffer
		n, err := removeStalePlans(root, &out)
		if err != nil || n != 0 {
			t.Fatalf("n=%d err=%v, want 0, nil", n, err)
		}
	})
}
