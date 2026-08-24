package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestRollbackMovesBinary is Phase 6a's headline regression (AC-6a.1, D10):
// $EDIKT_ROOT/bin/edikt must move with `current`. Before this fix, rollback
// flipped the payload version (templates/hooks/commands, via
// repairExternalSymlinks) but left whatever binary install.sh had most
// recently written in place at $EDIKT_ROOT/bin/edikt — a real file, not a
// symlink — so rolling back to an older payload paired it with a newer
// binary. This constructs that exact skew (the target version's own
// snapshot at versions/0.6.0/bin/edikt, a "newer" binary already sitting at
// $EDIKT_ROOT/bin/edikt) and asserts rollback repairs it to match the
// version it just activated.
func TestRollbackMovesBinary(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	// INV-007: sandbox CLAUDE_HOME so repairExternalSymlinks cannot escape
	// into the host's ~/.claude (same class of leak fixed for the other
	// rollback/use tests in this package).
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	oldBinary := "launcher-binary-content-for-0.6.0"
	newBinary := "launcher-binary-content-for-0.7.0"
	for v, content := range map[string]string{"0.6.0": oldBinary, "0.7.0": newBinary} {
		binDir := filepath.Join(root, "versions", v, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "edikt"), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Simulate install.sh having already swapped $EDIKT_ROOT/bin/edikt to
	// the NEW version's binary before `rollback` ever runs — that is the
	// real sequence: install.sh always updates the binary ahead of
	// activating a payload, and the defect is that nothing ever moved it
	// back on rollback.
	liveBinDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(liveBinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(liveBinDir, "edikt"), []byte(newBinary), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := writeLock(root, "0.6.0", "test"); err != nil {
		t.Fatal(err)
	}
	if err := writeLock(root, "0.7.0", "test"); err != nil {
		t.Fatal(err)
	}

	buf, err := runCmd(t, "rollback")
	if err != nil {
		t.Fatalf("rollback: %v\n%s", err, buf)
	}

	got, err := os.ReadFile(filepath.Join(root, "bin", "edikt"))
	if err != nil {
		t.Fatalf("reading bin/edikt after rollback: %v", err)
	}
	if string(got) != oldBinary {
		t.Errorf("bin/edikt after rollback = %q, want %q (the just-activated 0.6.0 payload's own binary)", got, oldBinary)
	}
}

// TestRollbackVersionedSurfacesOriginateFromSameVersion is AC-6a.2: after a
// rollback, the hook shim surface (templates/hooks/*, reached via the
// existing hooks -> current/templates/hooks symlink) and the launcher
// binary (reached via the new repairVersionedBinary copy) must all read
// back content stamped with the SAME activated version — not a mix of old
// payload and new binary. registry.yaml itself doesn't exist until Phase 6;
// this test stands in a fixture file at that same path to prove the
// carrying mechanism generalizes to whatever templates/hooks/ contains,
// which is the property Phase 6 depends on.
func TestRollbackVersionedSurfacesOriginateFromSameVersion(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	setupVersion := func(tag, marker string) {
		hooksDir := filepath.Join(root, "versions", tag, "templates", "hooks")
		if err := os.MkdirAll(hooksDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(hooksDir, "registry.yaml"), []byte("registry-"+marker), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(hooksDir, "some-shim.sh"), []byte("shim-"+marker), 0o644); err != nil {
			t.Fatal(err)
		}

		binDir := filepath.Join(root, "versions", tag, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "edikt"), []byte("binary-"+marker), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	setupVersion("0.6.0", "v6")
	setupVersion("0.7.0", "v7")

	if err := writeLock(root, "0.6.0", "test"); err != nil {
		t.Fatal(err)
	}
	if err := writeLock(root, "0.7.0", "test"); err != nil {
		t.Fatal(err)
	}

	buf, err := runCmd(t, "rollback")
	if err != nil {
		t.Fatalf("rollback: %v\n%s", err, buf)
	}

	read := func(rel string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		return string(data)
	}

	if got := read(filepath.Join("hooks", "registry.yaml")); got != "registry-v6" {
		t.Errorf("hooks/registry.yaml = %q, want %q (target version 0.6.0)", got, "registry-v6")
	}
	if got := read(filepath.Join("hooks", "some-shim.sh")); got != "shim-v6" {
		t.Errorf("hooks/some-shim.sh = %q, want %q (target version 0.6.0)", got, "shim-v6")
	}
	if got := read(filepath.Join("bin", "edikt")); got != "binary-v6" {
		t.Errorf("bin/edikt = %q, want %q (target version 0.6.0) — shims and the binary disagree on version", got, "binary-v6")
	}
}

// TestRollbackPreMigrationPayloadStillSucceeds is AC-6a.3: a payload
// installed before Phase 6a shipped has no versions/<tag>/bin/edikt
// snapshot at all. Rollback onto it must still succeed — the missing
// snapshot degrades to "binary not repaired" (the pre-Phase-6a status quo),
// never to a failed rollback. It must also leave the live binary
// byte-for-byte untouched, since there is nothing versioned to repair it
// from (INV-013: this is best-effort repair, not a check, so an absence
// path must be silent, not a false claim of having synced something).
func TestRollbackPreMigrationPayloadStillSucceeds(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	// No bin/ under either version — the pre-migration shape.
	for _, v := range []string{"0.4.3", "0.6.0"} {
		if err := os.MkdirAll(filepath.Join(root, "versions", v), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	liveBinDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(liveBinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := "pre-migration-binary-must-stay-untouched"
	if err := os.WriteFile(filepath.Join(liveBinDir, "edikt"), []byte(marker), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := writeLock(root, "0.4.3", "test"); err != nil {
		t.Fatal(err)
	}
	if err := writeLock(root, "0.6.0", "test"); err != nil {
		t.Fatal(err)
	}

	buf, err := runCmd(t, "rollback")
	if err != nil {
		t.Fatalf("rollback on a pre-migration payload should still succeed: %v\n%s", err, buf)
	}

	after, err := readLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if after.Active != "0.4.3" {
		t.Errorf("expected active=0.4.3 after rollback, got %s", after.Active)
	}

	got, err := os.ReadFile(filepath.Join(root, "bin", "edikt"))
	if err != nil {
		t.Fatalf("bin/edikt missing after rollback: %v", err)
	}
	if string(got) != marker {
		t.Errorf("bin/edikt was modified despite no versioned snapshot to repair from: got %q, want unchanged %q", got, marker)
	}
}

// TestUseSyncsBinaryToActivatedVersion proves the fix is not rollback-only.
// repairVersionedBinary lives inside repairExternalSymlinks, the function
// `use` and `rollback` already share — so `edikt use <tag>` must carry the
// binary exactly the same way, or the fix is only half applied.
func TestUseSyncsBinaryToActivatedVersion(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	// Both tags must be on the current line (>= versionLineFloor, 0.8.0 as of
	// SPEC-015 Phase 11) — use.go's payload-line floor (ADR-042) refuses to
	// activate anything below it.
	for tag, content := range map[string]string{
		"0.8.0": "binary-0.8.0",
		"0.8.1": "binary-0.8.1",
	} {
		binDir := filepath.Join(root, "versions", tag, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "edikt"), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	liveBinDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(liveBinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(liveBinDir, "edikt"), []byte("binary-0.8.1"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := writeLock(root, "0.8.1", "test"); err != nil {
		t.Fatal(err)
	}

	buf, err := runCmd(t, "use", "0.8.0")
	if err != nil {
		t.Fatalf("use 0.8.0: %v\n%s", err, buf)
	}

	got, err := os.ReadFile(filepath.Join(root, "bin", "edikt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "binary-0.8.0" {
		t.Errorf("bin/edikt after `use 0.8.0` = %q, want %q", got, "binary-0.8.0")
	}
}

// TestInstallSnapshotsRunningBinaryWhenVersionMatchesTag covers the write
// side of D10: `edikt install <tag>` must capture a per-version binary
// snapshot for repairVersionedBinary to later restore. The test binary
// built by TestMain carries the default Version ("dev"), so installing tag
// "dev" is the one case in this suite where snapshotLauncherBinary's
// version-match guard fires — letting this assert the snapshot is a
// byte-exact copy of the binary that ran the install, not a placeholder.
func TestInstallSnapshotsRunningBinaryWhenVersionMatchesTag(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "VERSION"), []byte("dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDIKT_INSTALL_SOURCE", src)

	buf, err := runCmd(t, "install", "dev")
	if err != nil {
		t.Fatalf("install dev: %v\n%s", err, buf)
	}

	snapshotPath := filepath.Join(root, "versions", "dev", "bin", "edikt")
	got, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("snapshot binary missing at %s: %v", snapshotPath, err)
	}
	want, err := os.ReadFile(_builtBinary)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("snapshotted binary does not match the running test binary (%d bytes vs %d bytes)", len(got), len(want))
	}

	info, err := os.Stat(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("snapshotted binary is not executable: mode=%v", info.Mode())
	}
}
