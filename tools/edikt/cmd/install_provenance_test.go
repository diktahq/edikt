package cmd

// install_provenance_test.go — SPEC-024 phase 3, D382.
//
// Phase 3's own criteria (AC-3.1-AC-3.4) verified 4/4 while the one command
// that actually knows a payload's provenance recorded none at all:
// `edikt install` (cmd/install.go) staged a payload and wrote only
// manifest.yaml, never touching lock.yaml. A later `edikt use <tag>` then
// resolved that version's source via lockSourceFor, found no entry, and
// lockSourceUnknown reads as published (SSP-004's deliberate default for a
// GENUINELY unknown provenance) — so a locally built payload `install`
// staged was auto-advanced over by `edikt upgrade` exactly as if it were a
// re-fetchable release. That is the precise loss SR-002 exists to prevent.
//
// This file is the coverage AC-3.1-AC-3.4 did not provide: every one of them
// asserts the lock manifest or the decision-table gate, and none of them
// looks at the verb that writes the payload in the first place.

import (
	"os"
	"path/filepath"
	"testing"
)

// TestInstall_LocalSource_RecordsLocalProvenance — the defect itself. Without
// D382's fix, this test's final assertion fails: lockSourceFor(root, "0.7.2")
// returns lockSourceUnknown, which effectiveLockSource resolves to
// lockSourcePublished, not lockSourceLocal.
func TestInstall_LocalSource_RecordsLocalProvenance(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "VERSION"), "0.7.2\n")
	t.Setenv("EDIKT_INSTALL_SOURCE", source)

	if _, err := runCmd(t, "install", "0.7.2"); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	lf, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	got := lockSourceFor(lf, "0.7.2")
	if got != lockSourceLocal {
		t.Fatalf("lockSourceFor(root, %q) = %q, want %q — a payload install staged from a local "+
			"source must be findable as local BEFORE it is ever activated, or `use` reads it as "+
			"unknown and effectiveLockSource silently resolves that to published (D382)",
			"0.7.2", got, lockSourceLocal)
	}
	if effectiveLockSource(got) != lockSourceLocal {
		t.Fatalf("effectiveLockSource(%q) = %q, want %q", got, effectiveLockSource(got), lockSourceLocal)
	}
}

// TestInstall_LocalSource_UseCarriesProvenanceForward — the consumer half:
// `use` must pick up what `install` recorded, so activating a staged local
// payload actually stamps the active lock entry as local rather than
// re-deriving unknown/published from a fresh writeLock call.
func TestInstall_LocalSource_UseCarriesProvenanceForward(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "VERSION"), "0.7.3\n")
	t.Setenv("EDIKT_INSTALL_SOURCE", source)

	if _, err := runCmd(t, "install", "0.7.3"); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	t.Setenv("EDIKT_INSTALL_SOURCE", "") // `use` does not read this; belt and braces.
	if _, err := runCmd(t, "use", "0.7.3"); err != nil {
		t.Fatalf("use failed: %v", err)
	}

	lf, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	if lf.Active != "0.7.3" {
		t.Fatalf("lock.yaml active = %q, want %q", lf.Active, "0.7.3")
	}
	if lf.Source != lockSourceLocal {
		t.Fatalf("active lock entry source = %q, want %q — `use` activated a payload `install` staged "+
			"from a local source, and the provenance install recorded did not survive activation (D382)",
			lf.Source, lockSourceLocal)
	}

	// The property this whole chain protects: a subsequent upgrade off this
	// version must now refuse, naming the local payload, not silently advance.
	reason, msg := upgradeProvenanceGate("0.7.3", "0.8.0", lf.Source)
	if reason != refusalLocalPayload {
		t.Fatalf("upgradeProvenanceGate(0.7.3, 0.8.0, %q) = %q, want %q — install+use produced a lock "+
			"entry that does not protect the payload it just staged and activated:\n%s",
			lf.Source, reason, refusalLocalPayload, msg)
	}
}

// TestInstall_NetworkSource_RecordsPublishedProvenance — the other half of
// the ruling: install's NETWORK path (no EDIKT_INSTALL_SOURCE) fetches a
// published release, and must record that explicitly rather than relying on
// an absent field to default to the same answer by accident (an unrelated
// future change to that default must not silently mis-attribute these
// payloads). downloadAndInstall itself is not exercised here — it is a real
// network fetch — so this only reaches the recording call by stubbing the
// version already on disk, the same shape TestReportInstallNotActivated_*
// uses to isolate the recording logic from the fetch.
func TestInstall_NetworkSource_RecordsPublishedProvenance(t *testing.T) {
	root := t.TempDir()

	if err := recordInstalledSource(root, "0.9.0", lockSourcePublished); err != nil {
		t.Fatalf("recordInstalledSource: %v", err)
	}

	lf, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	got := lockSourceFor(lf, "0.9.0")
	if got != lockSourcePublished {
		t.Fatalf("lockSourceFor(root, %q) = %q, want %q", "0.9.0", got, lockSourcePublished)
	}
}

// TestRecordInstalledSource_DoesNotActivate — install stages, it does not
// activate (reportInstallNotActivated already tells the operator so).
// recordInstalledSource must not make the version it records look current.
func TestRecordInstalledSource_DoesNotActivate(t *testing.T) {
	root := t.TempDir()
	if err := writeLock(root, "0.7.0", "launcher", lockSourcePublished); err != nil {
		t.Fatalf("writeLock: %v", err)
	}

	if err := recordInstalledSource(root, "0.8.0", lockSourceLocal); err != nil {
		t.Fatalf("recordInstalledSource: %v", err)
	}

	lf, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	if lf.Active != "0.7.0" {
		t.Fatalf("lock.yaml active = %q, want %q — recording a staged version's provenance must not "+
			"activate it", lf.Active, "0.7.0")
	}
}

// TestRecordInstalledSource_ReinstallingActiveTag_UpdatesTopLevelSource —
// D392's first standing defect. recordInstalledSource previously only ever
// appended a history entry; lockSourceFor reads the TOP-LEVEL Source field
// first whenever the tag it is asked about is the active one, so a history
// entry recorded for that same tag was never consulted. Reinstalling the
// currently active tag from a different source (e.g. `versions/<tag>` was
// removed and `install` recreates it locally) must therefore update the
// top-level field directly, or the stale value keeps winning forever.
func TestRecordInstalledSource_ReinstallingActiveTag_UpdatesTopLevelSource(t *testing.T) {
	root := t.TempDir()
	if err := writeLock(root, "0.9.0", "launcher", lockSourcePublished); err != nil {
		t.Fatalf("writeLock: %v", err)
	}

	// Re-install the SAME, already-active tag, this time from a local source
	// (its versions/ dir was removed and recreated locally, say).
	if err := recordInstalledSource(root, "0.9.0", lockSourceLocal); err != nil {
		t.Fatalf("recordInstalledSource: %v", err)
	}

	lf, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	if lf.Source != lockSourceLocal {
		t.Fatalf("lock.yaml top-level source = %q, want %q — recordInstalledSource recorded the active "+
			"tag's new provenance somewhere lockSourceFor never looks for the active tag (D392)",
			lf.Source, lockSourceLocal)
	}
	got := lockSourceFor(lf, "0.9.0")
	if got != lockSourceLocal {
		t.Fatalf("lockSourceFor(root, %q) = %q, want %q", "0.9.0", got, lockSourceLocal)
	}
	if lf.Active != "0.9.0" {
		t.Fatalf("lock.yaml active = %q, want %q — recording provenance for the active tag must not "+
			"disturb which tag is active", lf.Active, "0.9.0")
	}
}

// TestRecordInstalledSource_SurvivesHistoryEviction — D394's standing
// defect. `writeLock`'s History is capped at 49 entries so ordinary use does
// not grow lock.yaml without bound; before this fix, recordInstalledSource
// appended into that SAME capped list, so a payload `install` staged could
// have its provenance evicted by ~49 unrelated lock writes before an
// operator ever got around to `edikt use`-ing it. StagedSources is a
// separate, non-evicting field for exactly this reason.
func TestRecordInstalledSource_SurvivesHistoryEviction(t *testing.T) {
	root := t.TempDir()

	// Stage a local payload's provenance, not yet active.
	if err := recordInstalledSource(root, "0.9.5", lockSourceLocal); err != nil {
		t.Fatalf("recordInstalledSource: %v", err)
	}

	// 60 unrelated activations — more than History's 49-entry cap — none of
	// them touching 0.9.5.
	for i := 0; i < 60; i++ {
		if err := writeLock(root, "0.9.0", "launcher", lockSourcePublished); err != nil {
			t.Fatalf("writeLock #%d: %v", i, err)
		}
	}

	lf, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	if len(lf.History) != 50 {
		t.Fatalf("history has %d entries, want 50 (49 carried forward + 1 new each write) — the cap "+
			"itself should still be in force", len(lf.History))
	}
	got := lockSourceFor(lf, "0.9.5")
	if got != lockSourceLocal {
		t.Fatalf("lockSourceFor(root, %q) = %q, want %q after 60 unrelated lock writes — a staged "+
			"payload's provenance must survive until it is activated, however many other writes "+
			"happen first (D394)", "0.9.5", got, lockSourceLocal)
	}

	// Activating it now must carry the provenance forward, exactly as if no
	// eviction pressure had ever existed.
	if err := writeLock(root, "0.9.5", "launcher", lockSourceFor(lf, "0.9.5")); err != nil {
		t.Fatalf("writeLock: %v", err)
	}
	lf2, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	if lf2.Source != lockSourceLocal {
		t.Fatalf("after activating the long-staged version, lock.yaml source = %q, want %q",
			lf2.Source, lockSourceLocal)
	}
}

// writeTestFile writes data to path, creating parent directories as needed,
// and fails the test on any error.
func writeTestFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating parent dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
