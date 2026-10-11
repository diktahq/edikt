package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "PLAN-x-criteria.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func TestSchema_strictDecode(t *testing.T) {
	body := `plan: x
schema_version: 1
phases:
  - id: "1"
    name: foo
    classification: testable
    completion_promise: DONE
    criteria:
      - id: 1.1
        statement: works
        verify: "true"
    extra_field: nope
`
	_, err := LoadCriteria(writeYAML(t, body))
	if err == nil {
		t.Fatal("expected error on unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "extra_field") {
		t.Errorf("error should mention unknown field, got: %v", err)
	}
}

func TestSchema_missingRequired(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{
			"missing plan",
			"schema_version: 1\nphases:\n  - id: '1'\n    name: x\n    classification: testable\n    criteria:\n      - id: 1.1\n        statement: x\n        verify: 'true'\n",
			"plan: required",
		},
		{
			"empty phases",
			"plan: x\nschema_version: 1\nphases: []\n",
			"phases: at least one",
		},
		{
			"bad classification",
			"plan: x\nschema_version: 1\nphases:\n  - id: '1'\n    name: x\n    classification: weird\n    criteria:\n      - id: 1.1\n        statement: x\n        verify: 'true'\n",
			"classification",
		},
		{
			"duplicate criterion id",
			"plan: x\nschema_version: 1\nphases:\n  - id: '1'\n    name: x\n    classification: testable\n    criteria:\n      - id: 1.1\n        statement: a\n        verify: 'true'\n      - id: 1.1\n        statement: b\n        verify: 'true'\n",
			"duplicate id",
		},
		{
			"wrong schema version",
			"plan: x\nschema_version: 99\nphases:\n  - id: '1'\n    name: x\n    classification: testable\n    criteria:\n      - id: 1.1\n        statement: a\n        verify: 'true'\n",
			"schema_version",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadCriteria(writeYAML(t, tc.body))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestSchema_validRoundTrip(t *testing.T) {
	// The repo's own criteria sidecar must parse cleanly under strict mode.
	wd, _ := os.Getwd()
	for _, candidate := range []string{
		"../../../../docs/internal/plans/PLAN-sidecar-architecture-criteria.yaml",
		"../../../docs/internal/plans/PLAN-sidecar-architecture-criteria.yaml",
	} {
		p := filepath.Join(wd, candidate)
		if _, err := os.Stat(p); err == nil {
			cf, err := LoadCriteria(p)
			if err != nil {
				t.Fatalf("repo criteria sidecar should parse: %v", err)
			}
			if cf.Plan == "" {
				t.Fatal("plan should be non-empty")
			}
			if len(cf.Phases) == 0 {
				t.Fatal("phases should be non-empty")
			}
			return
		}
	}
	t.Skip("repo criteria sidecar not found from this CWD; covered elsewhere")
}

// TestSchema_UnrelatedSchemaConstraintDoesNotBlockLoad is D390's
// regression: a criteria file that violates a schema constraint having
// nothing to do with `requirements:` — here `timeout_s`'s `minimum: 1` —
// must still load. The Go decoder never enforced that minimum (TimeoutS is a
// plain int with no Validate() check), so a real pre-existing file carrying
// `timeout_s: 0` loaded and verified before AC-1.5 wired the third schema
// copy into LoadCriteria at all.
//
// Running the WHOLE-document schema validator at load (the shape AC-1.5
// first shipped as) would refuse this file on upgrade for a reason that has
// nothing to do with the field this phase adds — the exact breaking change
// SSP-004 exists to forbid, arriving through a route SSP-004's text does not
// name. LoadCriteria must validate only `requirements:` at load time.
func TestSchema_UnrelatedSchemaConstraintDoesNotBlockLoad(t *testing.T) {
	body := "plan: x\nschema_version: 1\nphases:\n  - id: '1'\n    classification: testable\n    " +
		"criteria:\n      - id: 1.1\n        statement: a\n        verify: 'true'\n        timeout_s: 0\n"
	cf, err := LoadCriteria(writeYAML(t, body))
	if err != nil {
		t.Fatalf("a document violating an UNRELATED schema constraint (timeout_s's minimum, "+
			"nothing to do with requirements:) was refused at load — this breaks upgrades for "+
			"downstream files SSP-004 promises not to break: %v", err)
	}
	if got := cf.Phases[0].Criteria[0].TimeoutS; got != 0 {
		t.Fatalf("timeout_s = %d, want 0 (decoded as written, the schema minimum is not enforced here)", got)
	}
}

func TestSchema_findPhase(t *testing.T) {
	cf := &CriteriaFile{
		Phases: []Phase{
			{ID: "1"}, {ID: "4b"}, {ID: "12"},
		},
	}
	if cf.FindPhase("4b") == nil {
		t.Error("FindPhase 4b: nil")
	}
	if cf.FindPhase("nope") != nil {
		t.Error("FindPhase nope: should be nil")
	}
}
