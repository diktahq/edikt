package gov

// benchmark_cheatrate_baseline_test.go — Plan F tests for baseline-
// pack inclusion in the cheat-rate corpus (SR-008 partial completion).
//
// Plan E's --all originally scanned only governance dirs (paths.
// decisions/invariants/guidelines). Plan F extends it to also discover
// orphan .edikt.yaml sidecars under templates/sidecars/baseline/ —
// the "baseline pack" corpus, which carries actual verify_kind:
// behavioral directives (15 across 8 sidecars).
//
// SDLC sidecars (paths.{prds,specs,plans}) are NOT yet covered — their
// schemas lack verify_kind. That's a separate plan's work.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// makeBaselineSidecarsTree writes N minimal-but-valid gov-schema
// sidecars at templates/sidecars/baseline/ inside the given root.
// Returns the project root for chaining.
func makeBaselineSidecarsTree(t *testing.T, root string, names ...string) {
	t.Helper()
	dir := filepath.Join(root, "templates", "sidecars", "baseline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	for _, name := range names {
		path := filepath.Join(dir, name+".edikt.yaml")
		body := `schema_version: 1
topic: ` + name + `
path: templates/sidecars/baseline/` + name + `.edikt.yaml
signals:
  - test
paths:
  - "**/*.go"
scope:
  - design
directives:
  - text: "Test directive for ` + name + `."
    source_excerpt:
      line_start: 1
      line_end: 1
      quote: "test"
    verify: "true"
    verify_kind: structural
`
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

// makeMinimalEdiktConfig drops a .edikt/config.yaml pointing at the
// standard governance paths. discoverSidecars reads this through
// govrun.GovernanceDirs.
func makeMinimalEdiktConfig(t *testing.T, root string) {
	t.Helper()
	cfgDir := filepath.Join(root, ".edikt")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir .edikt: %v", err)
	}
	cfg := "paths:\n  decisions: docs/architecture/decisions\n  invariants: docs/architecture/invariants\n  guidelines: docs/architecture/guidelines\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// TestDiscoverBaselinePackSidecars_Empty confirms the discovery
// function handles a missing baseline dir gracefully — adopter
// projects that never installed the pack should see no error.
func TestDiscoverBaselinePackSidecars_Empty(t *testing.T) {
	root := t.TempDir()
	// Note: NO templates/sidecars/baseline/ dir created.
	pairs, err := discoverBaselinePackSidecars(filepath.Join(root, baselinePackDir))
	if err != nil {
		t.Fatalf("unexpected error on missing dir: %v", err)
	}
	if len(pairs) != 0 {
		t.Errorf("expected 0 pairs on missing dir, got %d", len(pairs))
	}
}

// TestDiscoverBaselinePackSidecars_LoadsValidSidecars confirms the
// discovery function correctly walks a directory of orphan .edikt.yaml
// files and returns one Pair per loadable file.
func TestDiscoverBaselinePackSidecars_LoadsValidSidecars(t *testing.T) {
	root := t.TempDir()
	makeBaselineSidecarsTree(t, root, "backend-api", "db-queries", "frontend-component")

	pairs, err := discoverBaselinePackSidecars(filepath.Join(root, baselinePackDir))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(pairs) != 3 {
		t.Fatalf("expected 3 pairs, got %d", len(pairs))
	}

	// ArtifactIDs should come from filename stems and be predictable.
	var ids []string
	for _, p := range pairs {
		ids = append(ids, p.ArtifactID)
		if p.Sidecar == nil {
			t.Errorf("pair %s has nil Sidecar (load must have populated it)", p.ArtifactID)
		}
		if p.ParentPath != "" {
			t.Errorf("baseline pack pair has unexpected ParentPath %q (baseline pack is orphan-sidecar)", p.ParentPath)
		}
		if !strings.HasSuffix(p.SidecarPath, ".edikt.yaml") {
			t.Errorf("SidecarPath %q does not end in .edikt.yaml", p.SidecarPath)
		}
	}
	sort.Strings(ids)
	wantIDs := []string{"backend-api", "db-queries", "frontend-component"}
	for i, want := range wantIDs {
		if ids[i] != want {
			t.Errorf("pair %d: want id %q, got %q (sorted ids: %v)", i, want, ids[i], ids)
		}
	}
}

// TestDiscoverBaselinePackSidecars_SkipsMalformed confirms a corrupt
// .edikt.yaml is silently dropped rather than poisoning the whole
// --all run.
func TestDiscoverBaselinePackSidecars_SkipsMalformed(t *testing.T) {
	root := t.TempDir()
	makeBaselineSidecarsTree(t, root, "backend-api")
	// Write a malformed sidecar — invalid YAML.
	bad := filepath.Join(root, baselinePackDir, "broken.edikt.yaml")
	if err := os.WriteFile(bad, []byte("not: valid: yaml: [["), 0o644); err != nil {
		t.Fatalf("write bad: %v", err)
	}

	pairs, err := discoverBaselinePackSidecars(filepath.Join(root, baselinePackDir))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(pairs) != 1 {
		t.Errorf("expected 1 pair (malformed skipped), got %d", len(pairs))
	}
	if len(pairs) > 0 && pairs[0].ArtifactID != "backend-api" {
		t.Errorf("expected 'backend-api', got %q", pairs[0].ArtifactID)
	}
}

// TestSelectSidecarPairs_SingleIDResolvesBaseline confirms that a
// positional id matching a baseline-pack sidecar resolves correctly,
// not just gov sidecars.
func TestSelectSidecarPairs_SingleIDResolvesBaseline(t *testing.T) {
	root := t.TempDir()
	makeMinimalEdiktConfig(t, root)
	makeBaselineSidecarsTree(t, root, "backend-api", "db-queries")
	// No gov sidecars — only baseline.

	t.Chdir(root)

	pairs, err := selectSidecarPairs(root, []string{"backend-api"}, false)
	if err != nil {
		t.Fatalf("selectSidecarPairs: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(pairs))
	}
	if pairs[0].ArtifactID != "backend-api" {
		t.Errorf("got ArtifactID %q, want backend-api", pairs[0].ArtifactID)
	}
}

// TestSelectSidecarPairs_SingleIDNotFound confirms the error message
// names both corpus locations the lookup searched, so the operator
// knows where to look.
func TestSelectSidecarPairs_SingleIDNotFound(t *testing.T) {
	root := t.TempDir()
	makeMinimalEdiktConfig(t, root)
	makeBaselineSidecarsTree(t, root, "backend-api")
	t.Chdir(root)

	_, err := selectSidecarPairs(root, []string{"nonexistent-sidecar"}, false)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
	msg := err.Error()
	if !strings.Contains(msg, "baseline pack") {
		t.Errorf("error should mention baseline pack as a search location; got %q", msg)
	}
	if !strings.Contains(msg, "nonexistent-sidecar") {
		t.Errorf("error should name the missing id; got %q", msg)
	}
}

// TestSelectSidecarPairs_AllIncludesBaseline confirms --all returns
// BOTH gov sidecars (when present) AND baseline pack sidecars in a
// single union slice.
func TestSelectSidecarPairs_AllIncludesBaseline(t *testing.T) {
	root := t.TempDir()
	makeMinimalEdiktConfig(t, root)
	makeBaselineSidecarsTree(t, root, "backend-api", "db-queries", "frontend-component")
	// No gov sidecars in the fixture.
	t.Chdir(root)

	pairs, err := selectSidecarPairs(root, nil, true)
	if err != nil {
		t.Fatalf("selectSidecarPairs --all: %v", err)
	}
	if len(pairs) != 3 {
		t.Errorf("expected 3 baseline pairs in --all, got %d", len(pairs))
	}
	for _, p := range pairs {
		if p.Sidecar == nil {
			t.Errorf("pair %s has nil Sidecar", p.ArtifactID)
		}
	}
}
