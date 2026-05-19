package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withProject builds a project at a tmpdir, writes the config + scaffold,
// chdirs into it, and returns the project root. Cleanup is handled by t.Chdir
// (test framework restores the previous cwd on teardown) and t.TempDir
// (removes the temp dir).
func withProject(t *testing.T, configYAML string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatalf("mkdir .edikt: %v", err)
	}
	if configYAML != "" {
		if err := os.WriteFile(filepath.Join(root, ".edikt", "config.yaml"), []byte(configYAML), 0o644); err != nil {
			t.Fatalf("write config.yaml: %v", err)
		}
	}
	for relPath, content := range files {
		full := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir parent for %s: %v", relPath, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", relPath, err)
		}
	}
	t.Chdir(root)
	return root
}

func runAndCapture(t *testing.T, kind string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := runNextID(&buf, kind); err != nil {
		t.Fatalf("runNextID(%q): %v", kind, err)
	}
	return buf.String()
}

// ── No-config fallbacks ──────────────────────────────────────────────────────

func TestNextID_NoConfig_Spec(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	got := runAndCapture(t, "spec")
	want := "<!-- edikt:live -->\nNext SPEC number: SPEC-001\nExisting specs: (none yet)\n<!-- /edikt:live -->\n"
	if got != want {
		t.Errorf("no-config spec:\n got %q\nwant %q", got, want)
	}
}

func TestNextID_NoConfig_PRD(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	got := runAndCapture(t, "prd")
	if !strings.Contains(got, "PRD-001") || !strings.Contains(got, "(none yet)") {
		t.Errorf("no-config prd: missing PRD-001/(none yet) in %q", got)
	}
}

func TestNextID_NoConfig_ADR(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	got := runAndCapture(t, "adr")
	if !strings.Contains(got, "ADR-001") {
		t.Errorf("no-config adr: missing ADR-001 in %q", got)
	}
}

func TestNextID_NoConfig_INV(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	got := runAndCapture(t, "inv")
	if !strings.Contains(got, "INV-001") {
		t.Errorf("no-config inv: missing INV-001 in %q", got)
	}
}

func TestNextID_NoConfig_Plan_EmitsNothing(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	got := runAndCapture(t, "plan")
	if got != "" {
		t.Errorf("no-config plan should be silent, got %q", got)
	}
}

// ── With config + items ──────────────────────────────────────────────────────

func TestNextID_ADR_CountsAndLists(t *testing.T) {
	withProject(t, "base: docs\n", map[string]string{
		"docs/architecture/decisions/ADR-001-foo.md": "x",
		"docs/architecture/decisions/ADR-002-bar.md": "x",
		"docs/architecture/decisions/ADR-007-baz.md": "x",
	})
	got := runAndCapture(t, "adr")
	if !strings.Contains(got, "Next ADR number: ADR-004") {
		t.Errorf("expected next ADR-004 (3 existing → +1), got %q", got)
	}
	// Listing is sorted alphabetically by full basename.
	if !strings.Contains(got, "ADR-001-foo,ADR-002-bar,ADR-007-baz") {
		t.Errorf("listing not sorted/joined as expected, got %q", got)
	}
}

func TestNextID_SPEC_DirGlobRequiresSpecMD(t *testing.T) {
	// SPEC-001/spec.md present → counts. SPEC-002 dir without spec.md → ignored.
	withProject(t, "base: docs\n", map[string]string{
		"docs/product/specs/SPEC-001/spec.md":  "real spec",
		"docs/product/specs/SPEC-002/notes.md": "no spec.md here, must NOT count",
	})
	got := runAndCapture(t, "spec")
	if !strings.Contains(got, "Next SPEC number: SPEC-002") {
		t.Errorf("only SPEC-001/spec.md should count → next 002, got %q", got)
	}
	if strings.Contains(got, "SPEC-002") && strings.Contains(got, "Existing specs: SPEC-001,SPEC-002") {
		t.Errorf("SPEC-002 dir without spec.md should not appear in existing list, got %q", got)
	}
}

func TestNextID_INV_CustomPathsOverride(t *testing.T) {
	withProject(t, "paths:\n  invariants: memory-bank/constraints\n", map[string]string{
		"memory-bank/constraints/INV-001-tenant.md":  "x",
		"memory-bank/constraints/INV-002-money.md":   "x",
		"docs/architecture/invariants/INV-999-old.md": "should NOT count (wrong dir)",
	})
	got := runAndCapture(t, "inv")
	if !strings.Contains(got, "Next INV number: INV-003") {
		t.Errorf("custom paths.invariants ignored, got %q", got)
	}
	if strings.Contains(got, "INV-999") {
		t.Errorf("hardcoded default path leaked, got %q", got)
	}
}

func TestNextID_Discovery_BaseDerivedPath(t *testing.T) {
	withProject(t, "base: docs\n", map[string]string{
		"docs/product/discovery/DISCOVERY-001-foo.md": "x",
		"docs/product/discovery/DISCOVERY-002-bar.md": "x",
	})
	got := runAndCapture(t, "discovery")
	if !strings.Contains(got, "Next DISCOVERY number: DISCOVERY-003") {
		t.Errorf("discovery count off, got %q", got)
	}
	if !strings.Contains(got, "DISCOVERY-001-foo,DISCOVERY-002-bar") {
		t.Errorf("discovery listing not sorted/joined, got %q", got)
	}
}

func TestNextID_NoConfig_Discovery(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	got := runAndCapture(t, "discovery")
	if !strings.Contains(got, "DISCOVERY-001") || !strings.Contains(got, "(none yet)") {
		t.Errorf("no-config discovery: missing DISCOVERY-001/(none yet) in %q", got)
	}
}

func TestNextID_PRD_BaseDerivedPath(t *testing.T) {
	// Base override: prds defaults to {base}/product/prds.
	withProject(t, "base: notes\n", map[string]string{
		"notes/product/prds/PRD-001-feature.md": "x",
	})
	got := runAndCapture(t, "prd")
	if !strings.Contains(got, "Next PRD number: PRD-002") {
		t.Errorf("base-derived prds path not used, got %q", got)
	}
}

// ── Plan kind: most-recent + in-progress extraction ──────────────────────────

func TestNextID_Plan_FindsMostRecent(t *testing.T) {
	root := withProject(t, "base: docs\n", map[string]string{
		"docs/plans/PLAN-old.md": "| Phase 1 | complete |\n",
		"docs/plans/PLAN-new.md": "| Phase | Status |\n|---|---|\n| Phase A | in_progress |\n| Phase B | pending |\n",
	})
	// Force PLAN-new.md to be newer.
	now := mustStatMtime(t, filepath.Join(root, "docs/plans/PLAN-old.md"))
	if err := os.Chtimes(filepath.Join(root, "docs/plans/PLAN-new.md"), now.Add(1), now.Add(1)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	got := runAndCapture(t, "plan")
	if !strings.Contains(got, "Active plan: PLAN-new.md") {
		t.Errorf("most-recent plan not selected, got %q", got)
	}
	if !strings.Contains(got, "Current phase status: Phase A in_progress") {
		t.Errorf("first in-progress phase row not extracted, got %q", got)
	}
}

func TestNextID_Plan_NoInProgress(t *testing.T) {
	withProject(t, "base: docs\n", map[string]string{
		"docs/plans/PLAN-done.md": "| Phase 1 | complete |\n| Phase 2 | complete |\n",
	})
	got := runAndCapture(t, "plan")
	if !strings.Contains(got, "(none in progress)") {
		t.Errorf("expected (none in progress) for plan with no in_progress row, got %q", got)
	}
}

// ── Error path ───────────────────────────────────────────────────────────────

func TestNextID_UnknownKind_Errors(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	var buf bytes.Buffer
	err := runNextID(&buf, "bogus")
	if err == nil {
		t.Fatalf("expected error for unknown kind, got nil")
	}
	if !strings.Contains(err.Error(), "unknown kind") {
		t.Errorf("error message should name the unknown kind, got %q", err.Error())
	}
}

// ── Helper ───────────────────────────────────────────────────────────────────

func mustStatMtime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.ModTime()
}
