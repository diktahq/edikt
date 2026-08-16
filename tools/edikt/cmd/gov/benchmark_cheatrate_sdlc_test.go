package gov

// benchmark_cheatrate_sdlc_test.go — Plan G tests for SDLC corpus
// discovery in the cheat-rate benchmark. Validates the
// SPEC/PRD/plan-criteria adapters surface only behavioral entries
// and that selectSidecarPairs' --all union now includes them.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSpecSidecar drops a minimal SPEC sidecar under specsDir/<dir>/
// spec.yaml. Each requirement spec is a 3-tuple (id, verify_kind, verify).
// Entries with empty verify_kind are emitted without the field.
func writeSpecSidecar(t *testing.T, specsDir, specDirName, specID string, reqs ...specReqSpec) {
	t.Helper()
	dir := filepath.Join(specsDir, specDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	body := "schema_version: \"1.0\"\ntype: spec\nid: " + specID + "\ntitle: Test\nstatus: draft\nauthor: test\ncreated_at: 2026-05-23T00:00:00Z\nsource_prompt: test\nrequirements:\n"
	for _, r := range reqs {
		body += "  - id: " + r.id + "\n    text: " + r.text + "\n    verify: \"" + r.verify + "\"\n"
		if r.verifyKind != "" {
			body += "    verify_kind: " + r.verifyKind + "\n"
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write spec.yaml: %v", err)
	}
}

type specReqSpec struct {
	id, text, verify, verifyKind string
}

// writePRDSidecar drops a minimal PRD sidecar at prdsDir/<name>.yaml.
func writePRDSidecar(t *testing.T, prdsDir, fileName, prdID string, reqs ...prdReqSpec) {
	t.Helper()
	if err := os.MkdirAll(prdsDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", prdsDir, err)
	}
	body := "schema_version: \"1.0\"\ntype: prd\nid: " + prdID + "\ntitle: Test\nstatus: draft\nrigor: standard\nauthor: test\ncreated_at: 2026-05-23T00:00:00Z\nrequirements:\n"
	for _, r := range reqs {
		body += "  - id: " + r.id + "\n    text: " + r.text + "\n    status: accepted\n    verify: \"" + r.verify + "\"\n"
		if r.verifyKind != "" {
			body += "    verify_kind: " + r.verifyKind + "\n"
		}
	}
	if err := os.WriteFile(filepath.Join(prdsDir, fileName), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", fileName, err)
	}
}

type prdReqSpec struct {
	id, text, verify, verifyKind string
}

// writePlanCriteriaSidecar drops a minimal plan-criteria sidecar at
// plansDir/<name>. Each criterion spec is (id, statement, verify_kind, verify).
func writePlanCriteriaSidecar(t *testing.T, plansDir, fileName, planID string, crits ...planCritSpec) {
	t.Helper()
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", plansDir, err)
	}
	body := "schema_version: 1\nplan: " + planID + "\ncreated: 2026-05-23T00:00:00Z\nphases:\n  - id: p1\n    name: Phase 1\n    classification: testable\n    criteria:\n"
	for _, c := range crits {
		body += "      - id: " + c.id + "\n        statement: " + c.statement + "\n        verify: \"" + c.verify + "\"\n"
		if c.verifyKind != "" {
			body += "        verify_kind: " + c.verifyKind + "\n"
		}
	}
	if err := os.WriteFile(filepath.Join(plansDir, fileName), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", fileName, err)
	}
}

type planCritSpec struct {
	id, statement, verify, verifyKind string
}

// makeSDLCConfig drops .edikt/config.yaml with paths.{specs,prds,plans}.
func makeSDLCConfig(t *testing.T, root string) {
	t.Helper()
	cfgDir := filepath.Join(root, ".edikt")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir .edikt: %v", err)
	}
	cfg := `paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  guidelines: docs/architecture/guidelines
  specs: docs/product/specs
  prds: docs/product/prds
  plans: docs/internal/plans
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// TestDiscoverSpecSidecars_FiltersToBehavioral confirms only entries
// with `verify_kind: behavioral` make it into the adapted Pair. The
// SPEC sidecar has a mix; only the behavioral one should surface.
func TestDiscoverSpecSidecars_FiltersToBehavioral(t *testing.T) {
	root := t.TempDir()
	makeSDLCConfig(t, root)
	specsDir := filepath.Join(root, "docs/product/specs")

	writeSpecSidecar(t, specsDir, "SPEC-001-foo", "SPEC-001",
		specReqSpec{id: "SR-001", text: "structural req", verify: "true", verifyKind: "structural"},
		specReqSpec{id: "SR-002", text: "behavioral req", verify: "true", verifyKind: "behavioral"},
		specReqSpec{id: "SR-003", text: "kindless req", verify: "true", verifyKind: ""},
	)

	pairs, err := discoverSpecSidecars(specsDir)
	if err != nil {
		t.Fatalf("discoverSpecSidecars: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected 1 SPEC pair (has 1 behavioral entry), got %d", len(pairs))
	}
	if pairs[0].ArtifactID != "SPEC-001" {
		t.Errorf("ArtifactID = %q, want SPEC-001", pairs[0].ArtifactID)
	}
	if pairs[0].Sidecar == nil {
		t.Fatal("Sidecar must not be nil")
	}
	if len(pairs[0].Sidecar.Directives) != 1 {
		t.Errorf("expected 1 adapted Directive (only the behavioral SR), got %d",
			len(pairs[0].Sidecar.Directives))
	}
	if len(pairs[0].Sidecar.Directives) > 0 {
		d := pairs[0].Sidecar.Directives[0]
		if d.VerifyKind != "behavioral" {
			t.Errorf("adapted Directive.VerifyKind = %q, want behavioral", d.VerifyKind)
		}
		if !strings.Contains(d.Text, "[SR-002]") {
			t.Errorf("adapted Directive.Text should embed original id; got %q", d.Text)
		}
	}
}

// TestDiscoverSpecSidecars_NoBehavioralSkipsSidecar confirms a SPEC
// with zero behavioral entries produces no Pair at all (rather than
// an empty-directives pair that would clutter --all output).
func TestDiscoverSpecSidecars_NoBehavioralSkipsSidecar(t *testing.T) {
	root := t.TempDir()
	specsDir := filepath.Join(root, "docs/product/specs")
	writeSpecSidecar(t, specsDir, "SPEC-002-bar", "SPEC-002",
		specReqSpec{id: "SR-001", text: "structural only", verify: "true", verifyKind: "structural"},
	)

	pairs, err := discoverSpecSidecars(specsDir)
	if err != nil {
		t.Fatalf("discoverSpecSidecars: %v", err)
	}
	if len(pairs) != 0 {
		t.Errorf("expected 0 pairs (no behavioral entries), got %d", len(pairs))
	}
}

// TestDiscoverPRDSidecars_FiltersToBehavioral mirrors the SPEC test
// for PRD sidecars (different on-disk shape — top-level .yaml not
// nested under a directory).
func TestDiscoverPRDSidecars_FiltersToBehavioral(t *testing.T) {
	root := t.TempDir()
	prdsDir := filepath.Join(root, "docs/product/prds")
	writePRDSidecar(t, prdsDir, "PRD-001.yaml", "PRD-001",
		prdReqSpec{id: "FR-001", text: "behavioral fr", verify: "true", verifyKind: "behavioral"},
		prdReqSpec{id: "FR-002", text: "tooling fr", verify: "true", verifyKind: "tooling"},
	)
	writePRDSidecar(t, prdsDir, "PRD-002.yaml", "PRD-002",
		prdReqSpec{id: "FR-001", text: "no behavioral", verify: "true", verifyKind: "structural"},
	)

	pairs, err := discoverPRDSidecars(prdsDir)
	if err != nil {
		t.Fatalf("discoverPRDSidecars: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected 1 PRD pair (PRD-001 has behavioral, PRD-002 doesn't), got %d", len(pairs))
	}
	if pairs[0].ArtifactID != "PRD-001" {
		t.Errorf("ArtifactID = %q, want PRD-001", pairs[0].ArtifactID)
	}
	if len(pairs[0].Sidecar.Directives) != 1 {
		t.Errorf("expected 1 adapted Directive, got %d", len(pairs[0].Sidecar.Directives))
	}
}

// TestDiscoverPlanCriteriaSidecars_FiltersToBehavioral covers the
// plan-criteria adapter path (different shape: phases[].criteria[]).
func TestDiscoverPlanCriteriaSidecars_FiltersToBehavioral(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "docs/internal/plans")
	writePlanCriteriaSidecar(t, plansDir, "PLAN-foo-criteria.yaml", "PLAN-foo",
		planCritSpec{id: "AC-1.1", statement: "structural ac", verify: "true", verifyKind: "structural"},
		planCritSpec{id: "AC-1.2", statement: "behavioral ac", verify: "true", verifyKind: "behavioral"},
	)

	pairs, err := discoverPlanCriteriaSidecars(plansDir)
	if err != nil {
		t.Fatalf("discoverPlanCriteriaSidecars: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected 1 plan-criteria pair, got %d", len(pairs))
	}
	if pairs[0].ArtifactID != "PLAN-foo" {
		t.Errorf("ArtifactID = %q, want PLAN-foo", pairs[0].ArtifactID)
	}
	if len(pairs[0].Sidecar.Directives) != 1 {
		t.Errorf("expected 1 adapted Directive (only behavioral AC), got %d",
			len(pairs[0].Sidecar.Directives))
	}
}

// TestDiscoverSDLCSidecars_AllCorporaTogether confirms the
// dispatcher walks all three SDLC corpora and returns the union.
func TestDiscoverSDLCSidecars_AllCorporaTogether(t *testing.T) {
	root := t.TempDir()
	makeSDLCConfig(t, root)

	writeSpecSidecar(t, filepath.Join(root, "docs/product/specs"),
		"SPEC-001-foo", "SPEC-001",
		specReqSpec{id: "SR-001", text: "spec", verify: "true", verifyKind: "behavioral"},
	)
	writePRDSidecar(t, filepath.Join(root, "docs/product/prds"),
		"PRD-001.yaml", "PRD-001",
		prdReqSpec{id: "FR-001", text: "prd", verify: "true", verifyKind: "behavioral"},
	)
	writePlanCriteriaSidecar(t, filepath.Join(root, "docs/internal/plans"),
		"PLAN-x-criteria.yaml", "PLAN-x",
		planCritSpec{id: "AC-1.1", statement: "plan", verify: "true", verifyKind: "behavioral"},
	)

	pairs, err := discoverSDLCSidecars(root)
	if err != nil {
		t.Fatalf("discoverSDLCSidecars: %v", err)
	}
	if len(pairs) != 3 {
		t.Fatalf("expected 3 SDLC pairs (one per corpus), got %d", len(pairs))
	}
	var ids []string
	for _, p := range pairs {
		ids = append(ids, p.ArtifactID)
	}
	wantIDs := map[string]bool{"SPEC-001": false, "PRD-001": false, "PLAN-x": false}
	for _, id := range ids {
		if _, ok := wantIDs[id]; ok {
			wantIDs[id] = true
		}
	}
	for id, seen := range wantIDs {
		if !seen {
			t.Errorf("expected pair with id %s, got ids %v", id, ids)
		}
	}
}

// TestDiscoverSDLCSidecars_MissingDirsTolerated confirms an empty
// config (no SDLC paths set) returns zero pairs without erroring.
func TestDiscoverSDLCSidecars_MissingDirsTolerated(t *testing.T) {
	root := t.TempDir()
	// No .edikt/config.yaml created. Should silently return zero.
	pairs, err := discoverSDLCSidecars(root)
	if err != nil {
		t.Errorf("unexpected error on missing config: %v", err)
	}
	if len(pairs) != 0 {
		t.Errorf("expected 0 pairs without config, got %d", len(pairs))
	}
}

// TestSelectSidecarPairs_UnionIncludesSDLC confirms --all returns
// pairs from all three corpora (gov + baseline + SDLC) in one slice.
func TestSelectSidecarPairs_UnionIncludesSDLC(t *testing.T) {
	root := t.TempDir()
	makeSDLCConfig(t, root)
	// One baseline pack sidecar.
	makeBaselineSidecarsTree(t, root, "backend-api")
	// One SDLC spec sidecar with a behavioral SR.
	writeSpecSidecar(t, filepath.Join(root, "docs/product/specs"),
		"SPEC-007-bar", "SPEC-007",
		specReqSpec{id: "SR-001", text: "x", verify: "true", verifyKind: "behavioral"},
	)
	t.Chdir(root)

	pairs, err := selectSidecarPairs(root, nil, true)
	if err != nil {
		t.Fatalf("selectSidecarPairs --all: %v", err)
	}
	if len(pairs) != 2 {
		t.Errorf("expected 2 pairs (1 baseline + 1 SDLC), got %d", len(pairs))
	}
	var found struct{ baseline, sdlc bool }
	for _, p := range pairs {
		switch p.ArtifactID {
		case "backend-api":
			found.baseline = true
		case "SPEC-007":
			found.sdlc = true
		}
	}
	if !found.baseline {
		t.Error("expected baseline pair (backend-api) in --all union")
	}
	if !found.sdlc {
		t.Error("expected SDLC pair (SPEC-007) in --all union")
	}
}

// TestSelectSidecarPairs_SingleIDResolvesSDLC confirms a positional
// id matching an SDLC sidecar resolves correctly.
func TestSelectSidecarPairs_SingleIDResolvesSDLC(t *testing.T) {
	root := t.TempDir()
	makeSDLCConfig(t, root)
	writeSpecSidecar(t, filepath.Join(root, "docs/product/specs"),
		"SPEC-009-bar", "SPEC-009",
		specReqSpec{id: "SR-001", text: "x", verify: "true", verifyKind: "behavioral"},
	)
	t.Chdir(root)

	pairs, err := selectSidecarPairs(root, []string{"SPEC-009"}, false)
	if err != nil {
		t.Fatalf("selectSidecarPairs: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(pairs))
	}
	if pairs[0].ArtifactID != "SPEC-009" {
		t.Errorf("got %q, want SPEC-009", pairs[0].ArtifactID)
	}
}
