package cmd

// lock_source_unforgettable_test.go — SPEC-024 phase 3, AC-3.5 (D397).
//
// AC-3.1-AC-3.4 read 4/4 across five lane rounds while every one of D382,
// D392, D394 and D396 was still found — each by an adversarial lens, each
// closed exactly where it was found, because nothing bounded the overall
// claim the completion promise made: PROVENANCE RECORDED AND UNFORGETTABLE.
// "Unforgettable" was a word, not a criterion, until now.
//
// The property: a payload installed from a local source resolves to `local`
// for as long as its payload is installed — across every sequence of
// shipped commands — and stops resolving only once that payload is gone.
// Existing tests each pin one earlier defect in isolation
// (lock_stagedsources_test.go, install_provenance_test.go); this file is the
// one that drives the full sequence AC-3.5 names, through the shipped
// commands themselves rather than the internal functions underneath them,
// so a future regression in how those commands are WIRED together — not
// just in the functions individually — has somewhere to be caught.

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLockSource_SurvivesDeactivationAndEviction drives the sequence AC-3.5
// names: install a payload locally (`edikt install`), activate it (`edikt
// use`), deactivate it by activating a second, published version (`edikt
// use` again), then let enough further activations of that second version
// pass to overflow History's 49-entry cap. The first version's provenance
// must still resolve to local throughout — it is never active, never
// freshly staged, and long since evicted from History — and reactivating it
// (`edikt use` a third time) must carry that value back onto the active
// lock entry, exactly as if no deactivation or eviction pressure had ever
// happened.
func TestLockSource_SurvivesDeactivationAndEviction(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	// `edikt install` a local payload for 0.7.4 and `edikt use` it.
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "VERSION"), "0.7.4\n")
	t.Setenv("EDIKT_INSTALL_SOURCE", source)
	if _, err := runCmd(t, "install", "0.7.4"); err != nil {
		t.Fatalf("install 0.7.4: %v", err)
	}
	t.Setenv("EDIKT_INSTALL_SOURCE", "") // `use` does not read this; belt and braces.
	if _, err := runCmd(t, "use", "0.7.4"); err != nil {
		t.Fatalf("use 0.7.4: %v", err)
	}

	// A second, published version becomes available. Its provenance is
	// recorded the way install's own network path records one (see
	// TestInstall_NetworkSource_RecordsPublishedProvenance, which isolates
	// that recording from an actual fetch the same way) — this test's
	// subject is what happens to 0.7.4 AFTER this point, not how 0.8.0
	// arrived. Activating it is the real `edikt use`.
	if err := os.MkdirAll(filepath.Join(root, "versions", "0.8.0"), 0o755); err != nil {
		t.Fatalf("mkdir versions/0.8.0: %v", err)
	}
	if err := recordInstalledSource(root, "0.8.0", lockSourcePublished); err != nil {
		t.Fatalf("recordInstalledSource(0.8.0): %v", err)
	}
	if _, err := runCmd(t, "use", "0.8.0"); err != nil {
		t.Fatalf("use 0.8.0: %v", err)
	}

	lfDeactivated, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	if lfDeactivated.Active != "0.8.0" {
		t.Fatalf("test setup: active = %q, want 0.8.0", lfDeactivated.Active)
	}
	if got := lockSourceFor(lfDeactivated, "0.7.4"); got != lockSourceLocal {
		t.Fatalf("lockSourceFor(root, %q) = %q, want %q immediately after deactivation",
			"0.7.4", got, lockSourceLocal)
	}

	// 60 further shipped `edikt use 0.8.0` activations — more than
	// History's 49-entry cap — none touching 0.7.4. Driven through the real
	// command so the eviction pressure is the actual History cap being
	// exercised, not a simulated write.
	const fillerWrites = 60
	for i := 0; i < fillerWrites; i++ {
		if _, err := runCmd(t, "use", "0.8.0"); err != nil {
			t.Fatalf("use 0.8.0 filler #%d: %v", i, err)
		}
	}

	lfEvicted, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	if len(lfEvicted.History) < 49 {
		t.Fatalf("test setup: history has only %d entries, want at least 49 — the cap must actually "+
			"have been reached for this test to measure anything", len(lfEvicted.History))
	}
	for _, h := range lfEvicted.History {
		if normalizeTag(h.Version) == "0.7.4" {
			t.Fatalf("test setup: 0.7.4 still present in History after %d filler writes — the eviction "+
				"this test depends on did not happen: %+v", fillerWrites, lfEvicted.History)
		}
	}
	got := lockSourceFor(lfEvicted, "0.7.4")
	if got != lockSourceLocal {
		t.Fatalf("lockSourceFor(root, %q) = %q, want %q after deactivation and %d further shipped `use` "+
			"writes overflowing History's 49-entry cap — a locally-sourced payload's provenance must "+
			"survive for as long as it is installed, not merely until the ring buffer forgets it "+
			"(D397, AC-3.5)", "0.7.4", got, lockSourceLocal, fillerWrites)
	}

	// Reactivating it — a third shipped `edikt use` — must carry that value
	// back onto the active lock entry.
	if _, err := runCmd(t, "use", "0.7.4"); err != nil {
		t.Fatalf("use 0.7.4 (reactivate): %v", err)
	}
	lfReactivated, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	if lfReactivated.Source != lockSourceLocal {
		t.Fatalf("after reactivating the long-deactivated local version, lock.yaml source = %q, want %q — "+
			"the reactivation carried forward whatever lockSourceFor resolved, so this is the same "+
			"defect surfacing at the consumer that reads it", lfReactivated.Source, lockSourceLocal)
	}
}

// TestLockSource_GoneAfterPrune is AC-3.5's other half: once a locally
// sourced, staged-but-never-activated version's payload directory is
// actually removed by the shipped `edikt prune` command, its provenance
// record must go with it. StagedSources is deliberately unbounded (unlike
// History) precisely so it does NOT forget while the payload still exists;
// the property only holds end to end if removal is also the only way an
// entry disappears, which this test is what asserts that half.
func TestLockSource_GoneAfterPrune(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	// `edikt install` a local payload for 0.7.5, staged but never activated.
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "VERSION"), "0.7.5\n")
	t.Setenv("EDIKT_INSTALL_SOURCE", source)
	if _, err := runCmd(t, "install", "0.7.5"); err != nil {
		t.Fatalf("install 0.7.5: %v", err)
	}
	t.Setenv("EDIKT_INSTALL_SOURCE", "")

	// An active version, kept by prune's own active/previous protection and
	// by --keep — not this test's subject, just something for prune's
	// normal bookkeeping to point `active` at and for --keep=1 to retain.
	if err := os.MkdirAll(filepath.Join(root, "versions", "0.9.0"), 0o755); err != nil {
		t.Fatalf("mkdir versions/0.9.0: %v", err)
	}
	if err := writeLock(root, "0.9.0", "launcher", lockSourcePublished); err != nil {
		t.Fatalf("writeLock(0.9.0): %v", err)
	}

	before, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock (before): %v", err)
	}
	if lockSourceFor(before, "0.7.5") != lockSourceLocal {
		t.Fatalf("test setup: 0.7.5's provenance not recorded before pruning: %+v", before.StagedSources)
	}

	// keep=1: 0.9.0 sorts newest lexically and is active, so 0.7.5 is the
	// only version eligible for removal.
	if _, err := runCmd(t, "prune", "--keep", "1"); err != nil {
		t.Fatalf("prune: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "versions", "0.7.5")); !os.IsNotExist(err) {
		t.Fatalf("versions/0.7.5 still exists after prune (stat err: %v) — test setup did not exercise "+
			"the removal this test depends on", err)
	}

	after, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock (after): %v", err)
	}
	if got := lockSourceFor(after, "0.7.5"); got != lockSourceUnknown {
		t.Fatalf("lockSourceFor(root, %q) = %q after `prune` removed its payload directory, want %q "+
			"(lockSourceUnknown) — the provenance record for a payload that no longer exists must be "+
			"gone, not merely unreferenced (D397, AC-3.5)", "0.7.5", got, lockSourceUnknown)
	}
	for _, s := range after.StagedSources {
		if normalizeTag(s.Version) == "0.7.5" {
			t.Fatalf("StagedSources still carries %q after `prune` removed its payload directory: %+v",
				"0.7.5", after.StagedSources)
		}
	}
	if lockSourceFor(after, "0.9.0") != lockSourcePublished {
		t.Fatalf("prune disturbed an unrelated (kept) version's provenance: lockSourceFor(0.9.0) = %q",
			lockSourceFor(after, "0.9.0"))
	}
}
