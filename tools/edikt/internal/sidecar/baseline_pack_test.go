package sidecar

// baseline_pack_test.go — SPEC-009 Plan C Phase 5.
//
// Pins the contract that every sidecar shipped under
// templates/sidecars/baseline/ loads cleanly through the KnownFields(true)
// loader. The baseline pack is the canonical starter kit for adopter
// projects; any drift between the v1 schema and a baseline sidecar would
// break first-run onboarding, so we lock it here.
//
// The 8 canonical baseline names are listed explicitly rather than
// discovered so a renamed or missing file fails the test loudly with a
// targeted message — a glob match would surface that as "test/dir empty"
// without naming which sidecar was lost.

import (
	"os"
	"path/filepath"
	"testing"
)

// baselineSidecarNames enumerates the 8 canonical baseline sidecar
// basenames (without the .edikt.yaml suffix). Keep this list in sync
// with templates/sidecars/baseline/ — the test treats this slice as the
// source of truth and the AC-5.1 integration check enforces the same
// list at the shell layer.
var baselineSidecarNames = []string{
	"backend-api",
	"frontend-component",
	"frontend-page",
	"db-queries",
	"db-transactions",
	"events-bus",
	"test-coverage",
	"owasp-baseline",
}

// TestBaselinePack_Loads asserts that every baseline sidecar parses
// without error under the strict KnownFields(true) loader. Any
// additional or renamed field on the v1.2 schema that the loader does
// not know about would surface here as a parse error — the test is
// intentionally a regression net on the schema/struct boundary, not a
// content review.
func TestBaselinePack_Loads(t *testing.T) {
	root := repoRoot(t)
	baselineDir := filepath.Join(root, "templates", "sidecars", "baseline")

	// Sanity check: the directory must exist before we iterate. A
	// missing directory means the baseline pack was never installed,
	// which is itself a Phase 5 regression worth flagging.
	if info, err := os.Stat(baselineDir); err != nil || !info.IsDir() {
		t.Fatalf("baseline sidecar directory missing: %s (err=%v)", baselineDir, err)
	}

	for _, name := range baselineSidecarNames {
		name := name // capture for parallel subtests
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(baselineDir, name+".edikt.yaml")
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("baseline sidecar %s missing at %s: %v", name, path, err)
			}
			s, err := Load(path)
			if err != nil {
				t.Fatalf("Load(%s) failed: %v", name, err)
			}
			if s == nil {
				t.Fatalf("Load(%s) returned nil sidecar without error", name)
			}
			if s.SchemaVersion != SchemaVersion {
				t.Errorf("Load(%s): schema_version = %d, want %d", name, s.SchemaVersion, SchemaVersion)
			}
			if s.Topic != name {
				t.Errorf("Load(%s): topic = %q, want %q (canonical baseline names must match the file basename)", name, s.Topic, name)
			}
			if len(s.Directives) == 0 {
				t.Errorf("Load(%s): no directives — every baseline sidecar must declare at least one directive", name)
			}
		})
	}
}
