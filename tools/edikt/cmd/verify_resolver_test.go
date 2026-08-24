package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stagePlans(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "internal", "plans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("schema_version: 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// Every doc and every habit says `verify PLAN-NNN`, but the sidecar on disk is
// PLAN-NNN-<slug>-criteria.yaml. Exact-stem matching alone never resolved the
// id anyone actually types.
func TestLocateCriteriaSidecar_ResolvesByPrefix(t *testing.T) {
	root := stagePlans(t, "PLAN-027-review-boundary-and-guide-quality-criteria.yaml")
	got, err := locateCriteriaSidecar(root, "PLAN-027")
	if err != nil {
		t.Fatalf("prefix id did not resolve: %v", err)
	}
	if filepath.Base(got) != "PLAN-027-review-boundary-and-guide-quality-criteria.yaml" {
		t.Errorf("resolved to %s", filepath.Base(got))
	}
}

// The bare id (no PLAN- prefix) must resolve the same way.
func TestLocateCriteriaSidecar_ResolvesBareIDByPrefix(t *testing.T) {
	root := stagePlans(t, "PLAN-027-something-criteria.yaml")
	if _, err := locateCriteriaSidecar(root, "027"); err != nil {
		t.Fatalf("bare id did not resolve: %v", err)
	}
}

// An exact stem must still win, unchanged, and must not be treated as ambiguous
// against nothing.
func TestLocateCriteriaSidecar_ExactStemStillWins(t *testing.T) {
	root := stagePlans(t, "PLAN-027-criteria.yaml")
	got, err := locateCriteriaSidecar(root, "PLAN-027")
	if err != nil {
		t.Fatalf("exact stem broke: %v", err)
	}
	if filepath.Base(got) != "PLAN-027-criteria.yaml" {
		t.Errorf("resolved to %s", filepath.Base(got))
	}
}

// AMBIGUITY MUST REFUSE. Resolving an ambiguous id by picking one is worse than
// the exact-match miss it replaces: the user gets a confident answer about a
// plan they did not name.
func TestLocateCriteriaSidecar_AmbiguousRefusesAndNamesCandidates(t *testing.T) {
	root := stagePlans(t,
		"PLAN-027-alpha-criteria.yaml",
		"PLAN-027-beta-criteria.yaml",
	)
	_, err := locateCriteriaSidecar(root, "PLAN-027")
	if err == nil {
		t.Fatal("an ambiguous id resolved to a single plan — it must refuse")
	}
	for _, want := range []string{"PLAN-027-alpha-criteria.yaml", "PLAN-027-beta-criteria.yaml", "matches 2"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %q: %v", want, err)
		}
	}
}

// The hyphen anchors the prefix: PLAN-02 must not resolve PLAN-027.
func TestLocateCriteriaSidecar_PrefixIsHyphenAnchored(t *testing.T) {
	root := stagePlans(t, "PLAN-027-something-criteria.yaml")
	if _, err := locateCriteriaSidecar(root, "PLAN-02"); err == nil {
		t.Fatal("PLAN-02 resolved PLAN-027 — the prefix is not anchored on the hyphen")
	}
}

// A genuinely absent plan keeps failing loudly. That half already worked and is
// the part a consumer reported as silent — the silence was above the binary.
func TestLocateCriteriaSidecar_AbsentStaysLoud(t *testing.T) {
	root := stagePlans(t)
	_, err := locateCriteriaSidecar(root, "PLAN-nope")
	if err == nil {
		t.Fatal("an absent plan must error")
	}
	if !strings.Contains(err.Error(), "no criteria sidecar at") {
		t.Errorf("error text changed shape: %v", err)
	}
}
