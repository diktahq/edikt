package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scratchProject builds a project tree on disk with one spec and one criteria
// file, and returns the criteria file's path.
//
// It is a REAL tree rather than an in-memory fake because the thing under test
// is resolution: LoadCriteria has only the criteria file's path and has to
// find the spec from it. A fake that handed the loader a pre-resolved id set
// would exercise the map lookup and skip the part that can actually be wrong.
func scratchProject(t *testing.T, planID, criteriaBody string, specDirs map[string]string) string {
	t.Helper()
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatalf("mkdir .edikt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".edikt", "config.yaml"),
		[]byte("paths:\n  specs: docs/product/specs\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	for dirName, body := range specDirs {
		dir := filepath.Join(root, "docs", "product", "specs", dirName)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte(body), 0o644); err != nil {
			t.Fatalf("write spec.yaml: %v", err)
		}
	}

	plansDir := filepath.Join(root, "docs", "internal", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir plans: %v", err)
	}
	path := filepath.Join(plansDir, planID+"-criteria.yaml")
	if err := os.WriteFile(path, []byte(criteriaBody), 0o644); err != nil {
		t.Fatalf("write criteria: %v", err)
	}
	return path
}

const scratchSpec = `schema_version: '1.0'
type: spec
id: SPEC-042
title: A scratch spec
status: accepted
requirements:
- id: SR-001
  text: The first requirement.
- id: SR-002
  text: The second requirement.
protections:
- id: SSP-001
  text: A protection a criterion may legitimately be the thing that holds.
`

// criteriaWith builds a valid one-phase criteria file, optionally declaring
// requirements on its single criterion.
func criteriaWith(planID string, decls ...string) string {
	var b strings.Builder
	b.WriteString("plan: " + planID + "\n")
	b.WriteString("schema_version: 1\n")
	b.WriteString("phases:\n")
	b.WriteString("  - id: \"1\"\n")
	b.WriteString("    classification: testable\n")
	b.WriteString("    criteria:\n")
	b.WriteString("      - id: AC-1.1\n")
	b.WriteString("        statement: The widget compiles.\n")
	b.WriteString("        verify: go build ./...\n")
	if decls != nil {
		b.WriteString("        requirements:\n")
		for _, d := range decls {
			b.WriteString("          - " + d + "\n")
		}
	}
	return b.String()
}

// TestCriteria_RequirementsDecoded is the decoder half of the declaration.
//
// The schema accepting a key proves the file loads; it does not prove the Go
// side can SEE the value. A field the decoder drops would still load every
// file, still validate, and hand every consumer an empty list — a declaration
// that exists on disk and nowhere else.
func TestCriteria_RequirementsDecoded(t *testing.T) {
	t.Run("declared ids reach the struct, in file order", func(t *testing.T) {
		path := scratchProject(t, "PLAN-SPEC-042-scratch",
			criteriaWith("PLAN-SPEC-042-scratch", "SR-002", "SR-001", "SSP-001"),
			map[string]string{"SPEC-042-scratch": scratchSpec})

		c, err := LoadCriteria(path)
		if err != nil {
			t.Fatalf("LoadCriteria: %v", err)
		}
		got := c.Phases[0].Criteria[0].Requirements
		want := []string{"SR-002", "SR-001", "SSP-001"}
		if len(got) != len(want) {
			t.Fatalf("decoded %d requirement(s) %v, want %d %v", len(got), got, len(want), want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("requirements[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
			}
		}
	})

	t.Run("absent: a file with no requirements anywhere still loads", func(t *testing.T) {
		// The upgrade path. Under KnownFields(true) a mis-declared field is a
		// hard load failure, so this is what a REQUIRED field would have
		// broken in every downstream project that has not backfilled.
		path := scratchProject(t, "PLAN-SPEC-042-scratch",
			criteriaWith("PLAN-SPEC-042-scratch"),
			map[string]string{"SPEC-042-scratch": scratchSpec})

		c, err := LoadCriteria(path)
		if err != nil {
			t.Fatalf("a criteria file carrying no requirements: failed to load: %v", err)
		}
		if got := c.Phases[0].Criteria[0].Requirements; got != nil {
			t.Fatalf("expected nil requirements for an undeclared criterion, got %v", got)
		}
	})

	t.Run("absent: no spec is consulted when nothing is declared", func(t *testing.T) {
		// A plan that declares nothing must not acquire a dependency on a
		// spec existing. This tree has no docs/product/specs at all.
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		path := filepath.Join(root, "PLAN-SPEC-042-scratch-criteria.yaml")
		if err := os.WriteFile(path, []byte(criteriaWith("PLAN-SPEC-042-scratch")), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := LoadCriteria(path); err != nil {
			t.Fatalf("an undeclared criteria file must load with no spec present: %v", err)
		}
	})

	t.Run("an empty list is a declaration, not an omission", func(t *testing.T) {
		body := criteriaWith("PLAN-SPEC-042-scratch") + "        requirements: []\n"
		path := scratchProject(t, "PLAN-SPEC-042-scratch", body,
			map[string]string{"SPEC-042-scratch": scratchSpec})

		c, err := LoadCriteria(path)
		if err != nil {
			t.Fatalf("LoadCriteria: %v", err)
		}
		got := c.Phases[0].Criteria[0].Requirements
		if got == nil || len(got) != 0 {
			t.Fatalf("expected an empty, non-nil declaration, got %#v", got)
		}
	})
}

// TestCriteria_UnknownRequirementIdRejected is the falsification the criterion
// names: declare an id the spec does not define, and the file must fail to
// load NAMING that id.
//
// Naming it is not decoration. A rejection reporting "1 unknown id" leaves the
// author to diff two lists by hand, which is the same friction that makes a
// declaration go unmaintained in the first place.
func TestCriteria_UnknownRequirementIdRejected(t *testing.T) {
	specs := map[string]string{"SPEC-042-scratch": scratchSpec}

	t.Run("SR-999 against a spec with no SR-999", func(t *testing.T) {
		path := scratchProject(t, "PLAN-SPEC-042-scratch",
			criteriaWith("PLAN-SPEC-042-scratch", "SR-999"), specs)

		_, err := LoadCriteria(path)
		if err == nil {
			t.Fatal("a criteria file declaring SR-999 LOADED — the declaration is carried " +
				"with nothing validating it, which is the defect one level up")
		}
		for _, want := range []string{"SR-999", "AC-1.1", "SPEC-042"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("rejection does not name %q, so the author cannot act on it: %v", want, err)
			}
		}
	})

	t.Run("a real id beside an unknown one still fails, and names only the unknown", func(t *testing.T) {
		path := scratchProject(t, "PLAN-SPEC-042-scratch",
			criteriaWith("PLAN-SPEC-042-scratch", "SR-001", "SR-999"), specs)

		_, err := LoadCriteria(path)
		if err == nil {
			t.Fatal("one valid id masked an unknown one — a partially-correct declaration loaded")
		}
		if !strings.Contains(err.Error(), "SR-999") {
			t.Fatalf("rejection does not name SR-999: %v", err)
		}
		if strings.Contains(err.Error(), "declares SR-001") {
			t.Fatalf("rejection blames the id that IS defined: %v", err)
		}
	})

	t.Run("known requirement and protection ids both load", func(t *testing.T) {
		// The complement, and the reason it is here: a validator that
		// rejected everything would pass the two cases above while making the
		// field unusable.
		for _, id := range []string{"SR-001", "SR-002", "SSP-001"} {
			path := scratchProject(t, "PLAN-SPEC-042-scratch",
				criteriaWith("PLAN-SPEC-042-scratch", id), specs)
			if _, err := LoadCriteria(path); err != nil {
				t.Fatalf("%s is defined by the spec but was rejected: %v", id, err)
			}
		}
	})

	t.Run("a plan naming no spec cannot validate its declarations", func(t *testing.T) {
		path := scratchProject(t, "PLAN-loose-slug",
			criteriaWith("PLAN-loose-slug", "SR-001"), specs)

		_, err := LoadCriteria(path)
		if err == nil {
			t.Fatal("ids were accepted against a spec that was never identified — " +
				"the check passed by never running")
		}
		if !strings.Contains(err.Error(), "SR-001") || !strings.Contains(err.Error(), "PLAN-loose-slug") {
			t.Fatalf("rejection names neither the id nor the plan: %v", err)
		}
	})

	t.Run("a missing spec is a failure, never a silent pass", func(t *testing.T) {
		path := scratchProject(t, "PLAN-SPEC-042-scratch",
			criteriaWith("PLAN-SPEC-042-scratch", "SR-001"),
			map[string]string{}) // no spec on disk

		_, err := LoadCriteria(path)
		if err == nil {
			t.Fatal("declarations were accepted with no spec to check them against — " +
				"a control that observed nothing reported a pass (INV-013)")
		}
		if !strings.Contains(err.Error(), "SPEC-042") {
			t.Fatalf("rejection does not say which spec could not be found: %v", err)
		}
	})

	t.Run("SPEC-042 does not resolve to SPEC-0421", func(t *testing.T) {
		// The prefix match is anchored on a separator. Without that anchor a
		// declaration would validate against a neighbouring spec's ids.
		path := scratchProject(t, "PLAN-SPEC-042-scratch",
			criteriaWith("PLAN-SPEC-042-scratch", "SR-001"),
			map[string]string{"SPEC-0421-neighbour": scratchSpec})

		if _, err := LoadCriteria(path); err == nil {
			t.Fatal("SPEC-042 resolved to the SPEC-0421 directory — declarations would be " +
				"checked against a spec the author never named")
		}
	})
}

// TestCriteria_PlanFieldMustMatchFilename is AC-1.4's falsification.
//
// validateDeclaredRequirements resolves its oracle spec from c.Plan alone,
// and the file itself is located by filename — nothing compared the two.
// Reproduced here exactly as the criterion describes it: the SAME file, at
// the SAME path, validates a declaration against two different specs
// depending only on what its `plan:` field says, because nothing ties that
// field back to where the file actually lives.
func TestCriteria_PlanFieldMustMatchFilename(t *testing.T) {
	t.Run("same file, mismatched plan: field, is rejected rather than silently checked against the wrong spec", func(t *testing.T) {
		// The file lives under PLAN-SPEC-777-actual-location-criteria.yaml,
		// but its `plan:` field claims PLAN-SPEC-042-scratch, and SPEC-042
		// (unlike SPEC-777, which has no spec directory at all) genuinely
		// defines SR-001. Before the fix this LOADED: the declaration was
		// checked against SPEC-042 — the spec c.Plan happened to name — and
		// never against the plan the file's own location implies. A
		// coordinator reading "1 passed" would believe SPEC-777's SR-001 was
		// claimed; it was SPEC-042's.
		path := scratchProject(t, "PLAN-SPEC-777-actual-location",
			criteriaWith("PLAN-SPEC-042-scratch", "SR-001"),
			map[string]string{"SPEC-042-scratch": scratchSpec})

		_, err := LoadCriteria(path)
		if err == nil {
			t.Fatal("a criteria file located at PLAN-SPEC-777-actual-location-criteria.yaml but " +
				"declaring plan: PLAN-SPEC-042-scratch LOADED — its declaration was checked " +
				"against whichever spec the guarded document itself named, not the plan the " +
				"file's own filename asserts")
		}
		for _, want := range []string{"PLAN-SPEC-042-scratch", "PLAN-SPEC-777-actual-location"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("rejection does not name %q, so the author cannot tell what mismatched: %v", want, err)
			}
		}
	})

	t.Run("matching plan: field still loads", func(t *testing.T) {
		// The complement: this is not a rule against declaring anything at
		// all, only against a `plan:` that disagrees with its own filename.
		path := scratchProject(t, "PLAN-SPEC-042-scratch",
			criteriaWith("PLAN-SPEC-042-scratch", "SR-001"),
			map[string]string{"SPEC-042-scratch": scratchSpec})

		if _, err := LoadCriteria(path); err != nil {
			t.Fatalf("a plan: field that matches its own filename was rejected: %v", err)
		}
	})

	t.Run("a bare-slug plan: field under a PLAN--prefixed filename is a legitimate spelling, not a mismatch", func(t *testing.T) {
		// Several live criteria files in this corpus write the bare slug
		// (plan: v060-governance-accuracy) under a filename that carries the
		// PLAN- prefix (PLAN-v060-governance-accuracy-criteria.yaml). That
		// convention predates this check and must keep loading.
		root := t.TempDir()
		plansDir := filepath.Join(root, "docs", "internal", "plans")
		if err := os.MkdirAll(plansDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		path := filepath.Join(plansDir, "PLAN-loose-slug-criteria.yaml")
		if err := os.WriteFile(path, []byte(criteriaWith("loose-slug")), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := LoadCriteria(path); err != nil {
			t.Fatalf("a bare-slug plan: field under its PLAN--prefixed filename was rejected: %v", err)
		}
	})
}

// TestCriteria_DuplicateRequirementIdRejected is AC-1.5's falsification.
//
// A duplicate id within one criterion's own `requirements:` list is not two
// claims. The third mirrored schema copy
// (internal/sidecar/schema/plan-criteria.v1.schema.json) already declares
// `uniqueItems: true` on that list, held by its own byte-identity test and
// by test-schema-copies.sh — but before this fix, ValidateRawPlanCriteria was
// called from its own test file and NOWHERE ELSE, and LoadCriteria ran no
// JSON-schema validation at all. So `uniqueItems: true` asserted what the
// schema DOCUMENT said while no shipped code path ever applied it: this same
// probe — requirements: [SR-001, SR-001] — used to load and decode both.
// TestCriteria_FilenameCheckIsNoOpWhenNothingDeclared is D391's first finding
// (2026-09-09), reproduced and closed.
//
// validatePlanMatchesFilename used to run unconditionally from LoadCriteria,
// ahead of validateDeclaredRequirements's own no-op-when-nothing-declared
// guard. A criteria file that declares NO requirements at all — untouched by
// this phase — but whose `plan:` field has always disagreed with its own
// filename (a harmless, pre-existing spelling; nothing was ever resolved
// from that field before this phase) failed to load for the first time on
// this phase's account. That is the exact breaking-upgrade shape SSP-004
// forbids, one level up from D390's finding: the field itself stayed
// optional, and the harm arrived through a check next to it instead.
func TestCriteria_FilenameCheckIsNoOpWhenNothingDeclared(t *testing.T) {
	path := scratchProject(t, "PLAN-SPEC-777-actual-location",
		criteriaWith("PLAN-SPEC-042-scratch"), // no requirements: declared
		map[string]string{"SPEC-042-scratch": scratchSpec})

	if _, err := LoadCriteria(path); err != nil {
		t.Fatalf("a criteria file declaring NO requirements: failed to load solely because its "+
			"plan: field disagreed with its filename — this phase must not turn that pre-existing, "+
			"harmless spelling into a new load failure: %v", err)
	}
}

// TestCriteria_NonStringYAMLKeyElsewhereStillLoads is D391's second finding
// (2026-09-09), reproduced and closed.
//
// ValidateRawPlanCriteriaRequirements used to normalise the WHOLE decoded
// document via json.Marshal before navigating to each criterion's
// `requirements:`. gopkg.in/yaml.v3 decodes a mapping with a non-string key
// (an unquoted `true:` is ordinary YAML) into map[interface{}]interface{},
// which encoding/json cannot marshal — so ANY document carrying such a key
// ANYWHERE, including inside phase_metadata: (documented as freeform and
// runner-ignored, and a spelling this schema does not forbid), failed to
// load. Reproduced here with phase_metadata: carrying an unquoted boolean
// key, on a file that also declares a real requirement — proving the fix
// reaches the production LoadCriteria path, not only the validator in
// isolation.
func TestCriteria_NonStringYAMLKeyElsewhereStillLoads(t *testing.T) {
	body := `plan: PLAN-SPEC-042-scratch
schema_version: 1
phases:
  - id: "1"
    classification: testable
    phase_metadata:
      true: this unquoted YAML key decodes as a bool, not a string
    criteria:
      - id: AC-1.1
        statement: The widget compiles.
        verify: go build ./...
        requirements:
          - SR-001
`
	path := scratchProject(t, "PLAN-SPEC-042-scratch", body,
		map[string]string{"SPEC-042-scratch": scratchSpec})

	c, err := LoadCriteria(path)
	if err != nil {
		t.Fatalf("a document with a non-string YAML key inside phase_metadata: (which this "+
			"check never reads) failed to load while validating an unrelated requirements: "+
			"declaration: %v", err)
	}
	if got := c.Phases[0].Criteria[0].Requirements; len(got) != 1 || got[0] != "SR-001" {
		t.Fatalf("requirements: was not decoded correctly alongside the non-string key: %v", got)
	}
}

func TestCriteria_DuplicateRequirementIdRejected(t *testing.T) {
	path := scratchProject(t, "PLAN-SPEC-042-scratch",
		criteriaWith("PLAN-SPEC-042-scratch", "SR-001", "SR-001"),
		map[string]string{"SPEC-042-scratch": scratchSpec})

	_, err := LoadCriteria(path)
	if err == nil {
		t.Fatal("a criterion declaring requirements: [SR-001, SR-001] LOADED — a duplicate id " +
			"decoded as two claims instead of being rejected as one")
	}
	if !strings.Contains(err.Error(), "requirements") {
		t.Fatalf("rejection does not name the requirements list: %v", err)
	}
}
