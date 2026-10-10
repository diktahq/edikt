package phasea

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// D57 (4) — a plan written by an older binary is the ordinary case an
// upgrade landing between plan and verify produces. The refusal names the
// actual gap and its remedy, rather than reading as a corrupt file.
func TestVerify_StaleSchemaVersionNamesTheGap(t *testing.T) {
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	writeADR(t, root, "ADR-900", gateQuote)

	planID := "pap-000000000000"
	stale := map[string]interface{}{
		"schema_version": SchemaVersionPlan - 1,
		"plan_id":        planID,
		"created_from":   map[string]string{"corpus_hash": strings.Repeat("a", 64), "config_hash": strings.Repeat("b", 64)},
		"budget":         map[string]interface{}{"items": 0, "ceiling": DefaultCeiling, "force_ceiling": DefaultForceCeiling, "applied_ceiling": "ceiling", "max_concurrency": DefaultMaxConcurrency},
		"items":          []interface{}{},
	}
	data, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	planPath := PlanFilePath(root, planID)
	if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := Verify(VerifyOptions{ProjectRoot: root, PlanID: planID})
	if err == nil {
		t.Fatal("want a refusal for a stale-schema plan")
	}
	if out.ExitCode != ExitVerifyMalformed {
		t.Fatalf("exit = %d, want %d (ExitVerifyMalformed)", out.ExitCode, ExitVerifyMalformed)
	}
	want := fmt.Sprintf("was written by schema v%d; this binary writes v%d — run /edikt:upgrade", SchemaVersionPlan-1, SchemaVersionPlan)
	if !strings.Contains(out.Message, want) {
		t.Fatalf("message must name the schema gap and its remedy;\nwant substring: %q\ngot: %s", want, out.Message)
	}
}
