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
			"testable without verify",
			"plan: x\nschema_version: 1\nphases:\n  - id: '1'\n    name: x\n    classification: testable\n    criteria:\n      - id: 1.1\n        statement: x\n",
			"testable phases require verify",
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
