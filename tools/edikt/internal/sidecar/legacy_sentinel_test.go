package sidecar

import (
	"os"
	"path/filepath"
	"testing"
)

// writeGov stages one .md under <root>/<dir>/ and returns the root.
func writeGov(t *testing.T, root, dir, name, body string) {
	t.Helper()
	abs := filepath.Join(root, dir)
	if err := os.MkdirAll(abs, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(abs, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

const decisionsDir = "docs/architecture/decisions"

// TestHasAnyLegacySentinel_DistinguishesAdoptionStates is the unit-level
// guard for the adoption deadlock. Both states present as "governance .md,
// no sidecars", but only one of them has anything for `migrate sidecars`
// to lift — and routing the other one there is a dead end.
func TestHasAnyLegacySentinel_DistinguishesAdoptionStates(t *testing.T) {
	dirs := []string{decisionsDir, "docs/architecture/invariants", "docs/guidelines"}

	t.Run("never-initialised: prose only, no sentinel", func(t *testing.T) {
		root := t.TempDir()
		writeGov(t, root, decisionsDir, "ADR-001-test.md",
			"# ADR-001\n\n## Decision\n\nHand-written, predates edikt.\n")
		if HasAnyLegacySentinel(root, dirs) {
			t.Fatal("an ADR that predates edikt has no legacy sentinel to migrate")
		}
	})

	t.Run("pre-migration: in-body sentinel present", func(t *testing.T) {
		root := t.TempDir()
		writeGov(t, root, decisionsDir, "ADR-001-test.md",
			"# ADR-001\n\n## Decision\n\nRule.\n\n"+
				"[edikt:directives:start]: #\ntopic: test\ndirectives:\n  - A rule.\n[edikt:directives:end]: #\n")
		if !HasAnyLegacySentinel(root, dirs) {
			t.Fatal("expected the in-body sentinel to be detected")
		}
	})

	t.Run("fenced example does not count", func(t *testing.T) {
		root := t.TempDir()
		// A doc that SHOWS the legacy format inside a code fence is not a
		// project carrying legacy state.
		writeGov(t, root, decisionsDir, "ADR-001-test.md",
			"# ADR-001\n\n## Decision\n\nThe legacy format looked like:\n\n"+
				"```\n[edikt:directives:start]: #\ndirectives:\n  - x\n[edikt:directives:end]: #\n```\n")
		if HasAnyLegacySentinel(root, dirs) {
			t.Fatal("a fenced documentation example must not pin the project to the pre-migration path")
		}
	})

	t.Run("skip-listed artifact does not count", func(t *testing.T) {
		root := t.TempDir()
		// A superseded ADR is never migrated, so its sentinel must not
		// force the whole project onto the migration path.
		writeGov(t, root, decisionsDir, "ADR-001-test.md",
			"# ADR-001\n\n**Status:** Superseded by ADR-002\n\n"+
				"[edikt:directives:start]: #\ndirectives:\n  - x\n[edikt:directives:end]: #\n")
		if HasAnyLegacySentinel(root, dirs) {
			t.Fatal("a skip-listed artifact's sentinel must not count")
		}
	})

	t.Run("empty project", func(t *testing.T) {
		if HasAnyLegacySentinel(t.TempDir(), dirs) {
			t.Fatal("no governance dirs at all means no sentinels")
		}
	})
}

// TestDiscover_KindComesFromDirectory pins that an artifact's class is
// derived from the directory it lives in, never from its filename. The
// filename-prefix heuristic it replaces counted every README and stray
// note in the decisions/invariants directories as a guideline, which is
// how governance.md came to report guidelines that did not exist.
func TestDiscover_KindComesFromDirectory(t *testing.T) {
	root := t.TempDir()
	invDir := "docs/architecture/invariants"
	glDir := "docs/guidelines"

	writeGov(t, root, decisionsDir, "ADR-001-test.md", "# ADR-001\n")
	// A stray note in the decisions dir — no ADR- prefix.
	writeGov(t, root, decisionsDir, "NOTES.md", "# Notes\n")
	writeGov(t, root, invDir, "INV-001-test.md", "# INV-001\n")
	writeGov(t, root, glDir, "error-handling.md", "# Error handling\n")

	pairs, err := Discover(root, []string{decisionsDir, invDir, glDir})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}

	got := map[string]string{}
	for _, p := range pairs {
		got[filepath.Base(p.ParentPath)] = p.Kind
	}
	want := map[string]string{
		"ADR-001-test.md":   KindADR,
		"NOTES.md":          KindADR, // lives in decisions/, so it is not a guideline
		"INV-001-test.md":   KindInvariant,
		"error-handling.md": KindGuideline,
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Errorf("%s: Kind = %q, want %q", name, got[name], kind)
		}
	}
}
