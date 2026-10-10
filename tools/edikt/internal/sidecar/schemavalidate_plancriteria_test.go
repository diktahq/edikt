package sidecar

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRootPlanCriteria(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, "..", "..", "..", "..")
}

// TestPlanCriteriaSchemaMirrorIsByteIdentical guards the embedded copy against
// drift from the authoritative file, exactly as the two gov-sidecar mirrors are
// guarded. Two copies of a schema that may silently disagree is worse than one.
func TestPlanCriteriaSchemaMirrorIsByteIdentical(t *testing.T) {
	authoritative := filepath.Join(repoRootPlanCriteria(t), "templates", "schemas", "plan-criteria.v1.schema.json")
	want, err := os.ReadFile(authoritative)
	if err != nil {
		t.Fatalf("read authoritative plan-criteria schema: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(planCriteriaSchema)) {
		t.Fatalf("embedded plan-criteria schema mirror has DRIFTED from %s.\n"+
			"The authoritative file is templates/schemas/plan-criteria.v1.schema.json.\n"+
			"Resync with:\n"+
			"  cp templates/schemas/plan-criteria.v1.schema.json tools/edikt/internal/sidecar/schema/",
			authoritative)
	}
}

// baseCriteria is a complete, valid v1 document with NO `requirements:` key
// anywhere. Every case below is this document plus or minus one thing, so a
// failure names the field under test rather than a stale fixture.
const baseCriteria = `plan: PLAN-optionality-fixture
schema_version: 1
phases:
  - id: "1"
    classification: testable
    criteria:
      - id: AC-1.1
        statement: The widget compiles.
        verify: go build ./...
`

// TestCriteria_RequirementsOptional is the schema half of the optionality
// claim, and OPTIONAL is the load-bearing word.
//
// The field is optional HERE and required by the coverage gate, never the
// reverse. This file is consumed under strict decoding, so shipping
// `requirements:` as a REQUIRED property would stop every criteria file in
// every downstream project that has not backfilled from loading at all on
// upgrade — a coverage improvement arriving as a breaking change.
//
// So the first subtest is the one that matters: a document with no
// `requirements:` anywhere must still validate. The rest establish that the
// field, when present, is actually constrained — an "optional" field the schema
// does not describe would accept a typo'd scalar forever and the optionality
// case would prove nothing about the field having been added at all.
func TestCriteria_RequirementsOptional(t *testing.T) {
	cases := []struct {
		name    string
		doc     string
		wantErr string // "" = must validate; otherwise a substring of the rejection
	}{
		{
			name: "absent — the upgrade path for every un-backfilled file",
			doc:  baseCriteria,
		},
		{
			name: "present with ids — the declaration this phase adds",
			doc: baseCriteria + `        requirements:
          - SR-001
          - SSP-004
`,
		},
		{
			name: "empty list — the explicit satisfies-nothing declaration",
			doc: baseCriteria + `        requirements: []
`,
		},
		{
			name: "a bare scalar is not a list",
			doc: baseCriteria + `        requirements: SR-001
`,
			wantErr: "requirements",
		},
		{
			name: "an empty id declares nothing",
			doc: baseCriteria + `        requirements:
          - ""
`,
			wantErr: "requirements",
		},
		{
			name: "the same id twice is a duplicate, not two claims",
			doc: baseCriteria + `        requirements:
          - SR-001
          - SR-001
`,
			wantErr: "requirements",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRawPlanCriteria([]byte(tc.doc))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("document was REJECTED but must validate: %v\n---\n%s", err, tc.doc)
				}
				return
			}
			if err == nil {
				t.Fatalf("document was ACCEPTED but must be rejected — the schema does not "+
					"constrain `requirements:` at all\n---\n%s", tc.doc)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("rejection does not name %q, so an author cannot tell what to fix: %v",
					tc.wantErr, err)
			}
		})
	}
}

// TestPlanCriteriaSchemaAcceptsThisRepoUnbackfilledPlan is the corpus half of
// the same claim, against a real file rather than a fixture.
//
// The plan that introduces `requirements:` carries none of its own at the time
// this lands. If the schema change were a breaking one, this file — written
// before the field existed — is exactly what would stop loading.
func TestPlanCriteriaSchemaAcceptsThisRepoUnbackfilledPlan(t *testing.T) {
	path := filepath.Join(repoRootPlanCriteria(t), "docs", "internal", "plans",
		"PLAN-SPEC-025-criteria-carry-an-adversary-criteria.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		// INV-016: the link breaks loudly rather than passing on an absent  // edikt-guard:allow
		// subject. A renamed plan must fail here, not be skipped.
		t.Fatalf("read %s: %v", path, err)
	}
	if err := ValidateRawPlanCriteria(raw); err != nil {
		t.Fatalf("a real criteria file carrying no `requirements:` was rejected by the "+
			"updated schema — the change is not additive: %v", err)
	}
}
