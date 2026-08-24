package govrun

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/model"
)

// The lossless gate compares the extractor's output against
// MigrationPreserved.Directives. When that list is empty it compares against
// nothing, finds nothing missing, and the artifact passes a gate that never
// looked at it — a zero denominator reported as a clean result. INV-013:
// absence is never a passing measurement.
//
// These pin the classification the gate uses. They are unit-level on purpose:
// see TestPhaseB_LosslessGateIsCurrentlyUnreachable below for why an
// end-to-end assertion cannot run today.
func TestLosslessMeasurability_Classification(t *testing.T) {
	cases := []struct {
		name           string
		mp             *sidecar.MigrationPreserved
		wantMeasurable bool
		wantReasonHas  string
	}{
		{
			name:           "directives present — the only measurable shape",
			mp:             &sidecar.MigrationPreserved{Directives: []string{"Services MUST validate input. (ref: ADR-001)"}},
			wantMeasurable: true,
		},
		{
			// What the pre-raw_body migrator left behind for every sentinel it
			// could not parse: the classification survived, the content did not.
			name:           "empty block — baseline destroyed before the gate",
			mp:             &sidecar.MigrationPreserved{SchemaDetected: "unknown"},
			wantMeasurable: false,
			wantReasonHas:  "the baseline was lost",
		},
		{
			// What the fixed migrator writes: bytes survive, but as prose.
			name:           "raw_body only — preserved but not comparable",
			mp:             &sidecar.MigrationPreserved{SchemaDetected: "unknown", RawBody: "- Services MUST pin every dependency."},
			wantMeasurable: false,
			wantReasonHas:  "cannot be compared per-directive",
		},
		{
			// schema_detected is audit metadata about what the block LOOKED
			// like. Counting it as content is what let an artifact read as
			// "preserved" while its rule text was gone.
			name:           "schema_detected alone is not content",
			mp:             &sidecar.MigrationPreserved{SchemaDetected: "v0.4.3-legacy"},
			wantMeasurable: false,
			wantReasonHas:  "the baseline was lost",
		},
		{
			// Non-directive lists are preserved content, but the check still
			// has no directive tuples to compare — measurable must stay false
			// rather than silently passing.
			name:           "reminders only — content, but not a directive baseline",
			mp:             &sidecar.MigrationPreserved{Reminders: []string{"Remember the boundary."}},
			wantMeasurable: false,
			wantReasonHas:  "cannot be compared per-directive",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := losslessMeasurable(tc.mp)
			if got != tc.wantMeasurable {
				t.Fatalf("losslessMeasurable: want %v, got %v", tc.wantMeasurable, got)
			}
			if tc.wantMeasurable {
				return
			}
			reason := losslessUnmeasurableReason(tc.mp)
			if !strings.Contains(reason, tc.wantReasonHas) {
				t.Errorf("reason does not say why it could not be measured.\nwant substring %q\ngot %q", tc.wantReasonHas, reason)
			}
		})
	}
}

// HasContent must not count SchemaDetected. This is the specific confusion
// that made an empty preservation block read as a populated one.
func TestMigrationPreserved_HasContentIgnoresSchemaDetected(t *testing.T) {
	if (&sidecar.MigrationPreserved{SchemaDetected: "unknown"}).HasContent() {
		t.Error("schema_detected alone must not count as preserved content")
	}
	if !(&sidecar.MigrationPreserved{RawBody: "x"}).HasContent() {
		t.Error("raw_body must count as preserved content")
	}
	if (*sidecar.MigrationPreserved)(nil).HasContent() {
		t.Error("nil must not count as preserved content")
	}
}

func writePreservedSidecar(t *testing.T, adrDir, parentBase, artifactID, preserved string) string {
	t.Helper()
	body := fmt.Sprintf(`schema_version: 2
topic: "testing"
path: "docs/architecture/decisions/%s"
signals:
  - "bootstrap signal"
directives:
  - text: "Test rule MUST hold. (ref: %s)"
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: "Test rule."
migration_preserved:
%s`, parentBase, artifactID, preserved)
	p := filepath.Join(adrDir, strings.TrimSuffix(parentBase, ".md")+".edikt.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	return p
}

// TestPhaseB_LosslessGateIsCurrentlyUnreachable records a defect, not a
// desired property, and is written to FAIL the moment the defect is fixed.
//
// Two rules contradict each other:
//   - drift.go IsStale: a sidecar carrying migration_preserved is ALWAYS stale
//     ("migration-preserved baseline awaiting first canonical extraction").
//   - RunTwoPhase Phase A: if any sidecar is stale, emit a dispatch plan and
//     return "not running phase B".
//
// Phase B's lossless loop acts only on sidecars that carry
// migration_preserved. So the set it can act on is exactly the set that stops
// it running. The gate has never executed in this configuration, which is why
// no test covered it and why its zero-denominator defect survived unnoticed.
//
// The measurability fix above is still correct and still required — it is
// simply inert until this is resolved. Resolving it is a design decision
// (should Phase A distinguish "migrated, not yet extracted" from "migrated and
// extracted, awaiting verification"?) and is logged rather than taken here.
func TestPhaseB_LosslessGateIsCurrentlyUnreachable(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")
	root := stageNeverInitialised(t)
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")

	// A fully-extracted v2 sidecar that still carries its migration baseline —
	// precisely what the extractor is contracted to hand to Phase B.
	writePreservedSidecar(t, adrDir, "ADR-001-test.md", "ADR-001",
		"  schema_detected: v0.5x-full\n  directives:\n    - \"Test rule MUST hold. (ref: ADR-001)\"\n")

	agDir := filepath.Join(root, ".claude", "agents")
	if err := os.MkdirAll(agDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ag := "---\nname: sidecar-extractor\nmodel: sonnet\nprompt_version: 2\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(agDir, "sidecar-extractor.md"), []byte(ag), 0o644); err != nil {
		t.Fatal(err)
	}

	var errBuf, outBuf bytes.Buffer
	res, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root, Stderr: &errBuf, Stdout: &outBuf, OnLoss: "accept",
	}, model.RealClock{})

	if err == nil || !strings.Contains(err.Error(), "not running phase B") {
		t.Fatalf("DEFECT RESOLVED — Phase B is now reachable for a sidecar carrying "+
			"migration_preserved. Delete this test and assert the lossless gate's "+
			"behaviour end-to-end instead: the unit tests above only pin the "+
			"classification, not that the gate runs.\ngot err=%v\nstderr:\n%s", err, errBuf.String())
	}
	if res != nil && res.PhaseB != nil {
		t.Fatalf("Phase B ran unexpectedly; see above — this test must be replaced")
	}
	if res != nil && res.LosslessUnmeasured != nil {
		t.Errorf("lossless pass reported having run (%v) while Phase A refused to reach it", res.LosslessUnmeasured)
	}
}
