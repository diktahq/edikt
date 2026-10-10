package cmd

// lock_stagedsources_test.go — SPEC-024 phase 3, D396.
//
// Round 4 on this phase (aaef1450) added StagedSources — provenance keyed by
// version, unbounded — and then writeLock dropped a tag's entry the moment
// it activated, on the theory the top-level Source field and a fresh
// history entry both carry it forward from that point, "immune to
// eviction". Neither claim survives the version DEACTIVATING again: the
// top-level Source only ever holds the CURRENTLY active tag's provenance,
// and the fresh history entry sits in the same ring later writes trim to
// 49. So a version that activates, deactivates, and is later reactivated
// (rollback, dev unlink) had nothing left recording where its payload came
// from. This file is the regression coverage that scenario never had.

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteLock_StagedSourceSurvivesDeactivation is D396's exact scenario:
// install a payload locally, use it, use something else, let ~49 ordinary
// lock writes pass, roll back. Before the fix, lockSourceFor for the rolled-
// back-to version returns lockSourceUnknown at the point marked below, which
// effectiveLockSource silently reads as published.
func TestWriteLock_StagedSourceSurvivesDeactivation(t *testing.T) {
	root := t.TempDir()

	// install 0.7.0 locally (staged, not yet active).
	if err := recordInstalledSource(root, "0.7.0", lockSourceLocal); err != nil {
		t.Fatalf("recordInstalledSource: %v", err)
	}

	// use 0.7.0.
	lf, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	if err := writeLock(root, "0.7.0", "launcher", lockSourceFor(lf, "0.7.0")); err != nil {
		t.Fatalf("writeLock(0.7.0): %v", err)
	}

	// use 0.8.0 (a different, published version).
	if err := writeLock(root, "0.8.0", "launcher", lockSourcePublished); err != nil {
		t.Fatalf("writeLock(0.8.0): %v", err)
	}

	// ~49 ordinary lock writes pass, none touching 0.7.0.
	for i := 0; i < 49; i++ {
		if err := writeLock(root, "0.8.0", "launcher", lockSourcePublished); err != nil {
			t.Fatalf("writeLock filler #%d: %v", i, err)
		}
	}

	// The property under test: 0.7.0's provenance must still be findable
	// while it is NOT the active version and NOT freshly staged.
	lf2, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	if lf2.Active != "0.8.0" {
		t.Fatalf("test setup: active = %q, want 0.8.0", lf2.Active)
	}
	got := lockSourceFor(lf2, "0.7.0")
	if got != lockSourceLocal {
		t.Fatalf("lockSourceFor(root, %q) = %q, want %q after deactivation and 49 unrelated writes — "+
			"a version's provenance must survive it no longer being active, or a later rollback reads "+
			"it as unknown and effectiveLockSource silently resolves that to published (D396)",
			"0.7.0", got, lockSourceLocal)
	}

	// roll back to 0.7.0 — the consumer half: the value found above must
	// actually be what a reactivation carries forward.
	if err := writeLock(root, "0.7.0", "launcher", got); err != nil {
		t.Fatalf("writeLock (rollback): %v", err)
	}
	lf3, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	if lf3.Source != lockSourceLocal {
		t.Fatalf("after rolling back to the long-deactivated version, lock.yaml source = %q, want %q",
			lf3.Source, lockSourceLocal)
	}
}

// TestPruneStagedSource covers the fix's other half in isolation: an entry
// is removed only for the tag named, and only when asked — never as a side
// effect of the version merely being inactive.
func TestPruneStagedSource(t *testing.T) {
	lf := LockFile{StagedSources: []LockHistory{
		{Version: "0.7.0", Source: lockSourceLocal},
		{Version: "0.7.1", Source: lockSourcePublished},
	}}

	if changed := pruneStagedSource(&lf, "0.9.0"); changed {
		t.Fatalf("pruneStagedSource(0.9.0) reported a change over a list that does not contain it")
	}
	if len(lf.StagedSources) != 2 {
		t.Fatalf("pruning an absent tag mutated the list: %+v", lf.StagedSources)
	}

	if changed := pruneStagedSource(&lf, "0.7.0"); !changed {
		t.Fatalf("pruneStagedSource(0.7.0) reported no change, want true")
	}
	if len(lf.StagedSources) != 1 || lf.StagedSources[0].Version != "0.7.1" {
		t.Fatalf("StagedSources after pruning 0.7.0 = %+v, want only 0.7.1 remaining", lf.StagedSources)
	}
}

// TestPrune_RemovesStagedSourceForRemovedVersion drives the real `prune`
// command: once a version's payload directory is actually gone, its
// StagedSources entry must go with it — the ONLY point at which an entry
// should ever disappear (D396).
func TestPrune_RemovesStagedSourceForRemovedVersion(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)

	// Two locally-sourced, staged (never activated) versions on disk.
	for _, v := range []string{"0.5.0", "0.6.0"} {
		if err := os.MkdirAll(filepath.Join(root, "versions", v), 0o755); err != nil {
			t.Fatalf("mkdir versions/%s: %v", v, err)
		}
		if err := recordInstalledSource(root, v, lockSourceLocal); err != nil {
			t.Fatalf("recordInstalledSource(%s): %v", v, err)
		}
	}

	// An active version, kept by prune's own active/previous protection —
	// not the subject of this test, just enough for writeLock's normal
	// bookkeeping to have something to point `active` at.
	if err := writeLock(root, "0.9.0", "launcher", lockSourcePublished); err != nil {
		t.Fatalf("writeLock(0.9.0): %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "versions", "0.9.0"), 0o755); err != nil {
		t.Fatalf("mkdir versions/0.9.0: %v", err)
	}

	before, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock (before): %v", err)
	}
	if lockSourceFor(before, "0.5.0") != lockSourceLocal || lockSourceFor(before, "0.6.0") != lockSourceLocal {
		t.Fatalf("test setup: staged provenance not recorded before pruning: %+v", before.StagedSources)
	}

	// keep=1 (only the sort-newest tag survives on count) plus the active
	// protection: 0.9.0 sorts newest and is active, so 0.6.0 and 0.5.0 are
	// both eligible for removal.
	if _, err := runCmd(t, "prune", "--keep", "1"); err != nil {
		t.Fatalf("prune: %v", err)
	}

	for _, v := range []string{"0.5.0", "0.6.0"} {
		if _, err := os.Stat(filepath.Join(root, "versions", v)); !os.IsNotExist(err) {
			t.Fatalf("versions/%s still exists after prune (stat err: %v) — test setup did not exercise removal", v, err)
		}
	}

	after, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock (after): %v", err)
	}
	for _, v := range []string{"0.5.0", "0.6.0"} {
		for _, s := range after.StagedSources {
			if normalizeTag(s.Version) == v {
				t.Fatalf("StagedSources still carries %q after `prune` removed its payload directory — "+
					"the entry now describes a payload that no longer exists (D396)", v)
			}
		}
	}
	if lockSourceFor(after, "0.9.0") != lockSourcePublished {
		t.Fatalf("prune disturbed an unrelated (kept) version's provenance: lockSourceFor(0.9.0) = %q",
			lockSourceFor(after, "0.9.0"))
	}
}
