package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// snapshotLauncherBinary copies the currently-running launcher binary into
// <targetDir>/bin/edikt when the binary's own compiled-in version matches
// tag, so a later `edikt use`/`edikt rollback` can restore the exact binary
// that shipped with this payload (D10, PLAN-v080-reliability-core Phase 6a).
//
// install.sh always replaces $EDIKT_ROOT/bin/edikt with the new tag's
// launcher BEFORE invoking `edikt install <tag>` / `edikt use <tag>` — so
// under the normal install.sh flow, the process running this command IS
// already the binary for tag. When that invariant does not hold (e.g. an
// operator runs `edikt install <old-tag>` directly from a newer binary),
// Version and tag disagree and this is a deliberate no-op: snapshotting the
// wrong binary under the wrong tag's directory would corrupt a *future*
// rollback rather than merely skip repairing this one.
//
// Best-effort: every failure path here returns silently rather than failing
// the install. A missing snapshot degrades to the pre-Phase-6a behavior
// (rollback/use repair everything except the binary) rather than blocking
// installation of a payload.
func snapshotLauncherBinary(targetDir, tag string) {
	if normalizeTag(Version) != normalizeTag(tag) {
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		return
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}

	src, err := os.Open(exePath)
	if err != nil {
		return
	}
	defer src.Close()

	destDir := filepath.Join(targetDir, "bin")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return
	}
	dest := filepath.Join(destDir, "edikt")
	tmp := dest + fmt.Sprintf(".tmp.%d", os.Getpid())

	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not stage launcher at %s: %v — "+
			"the launcher binary may not match the activated payload\n", tmp, err)
		return
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		os.Remove(tmp)
		fmt.Fprintf(os.Stderr, "warning: could not copy versioned launcher: %v — "+
			"the launcher binary may not match the activated payload\n", err)
		return
	}
	out.Close()
	_ = os.Chmod(tmp, 0o755)

	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		fmt.Fprintf(os.Stderr, "warning: could not activate versioned launcher: %v — "+
			"the launcher binary may not match the activated payload\n", err)
	}
}

// repairVersionedBinary keeps $EDIKT_ROOT/bin/edikt in lockstep with
// current/bin/edikt — the launcher snapshot captured for the active payload
// version by snapshotLauncherBinary. Called from repairExternalSymlinks, the
// function already shared by `edikt use` and `edikt rollback` (D10): both
// flip `current`, and both must now carry the binary the same way they
// already carry templates/, hooks/, and the Claude commands symlink. THE
// DEFECT this closes: rollback/use flipped the payload version but left
// $EDIKT_ROOT/bin/edikt untouched, a real file install.sh writes directly —
// so a rollback restored the old templates/hooks/commands while leaving a
// newer binary underneath them.
//
// current/bin/edikt is absent for any payload installed before this
// snapshot existed (pre-migration) or for a dev-linked `current` (no built
// binary in a working tree). That absence is not an error — INV-013 bars a
// *check* from reporting success having verified nothing, but this is not a
// check, it is best-effort repair. Its absence path is silence, not a false
// PASS: nothing here writes a report or emits a completion signal claiming
// the binary was verified or repaired when it was not.
// repairVersionedBinary copies the activated version's snapshotted launcher over
// $EDIKT_ROOT/bin/edikt, so a rollback or `use` cannot leave old shims running
// against a newer binary.
//
// INV-013: every outcome is REPORTED. A pre-migration payload legitimately has no
// snapshot and that is announced, not silent — the operator must be able to tell
// "there was nothing to carry" from "carrying failed", because the whole reason
// this function exists is that binary skew is invisible. A silent no-op here
// would reproduce the defect the phase was written to close.
func repairVersionedBinary(ediktRoot string) {
	versionedBinary := filepath.Join(ediktRoot, "current", "bin", "edikt")
	info, err := os.Stat(versionedBinary)
	if err != nil || !info.Mode().IsRegular() {
		fmt.Fprintf(os.Stderr,
			"note: activated payload carries no versioned launcher (%s) — "+
				"$EDIKT_ROOT/bin/edikt left as-is. Expected for a payload predating "+
				"versioned binaries; if this is a v0.8.0+ payload, the binary and the "+
				"hook shims are now from different versions.\n", versionedBinary)
		return
	}

	src, err := os.Open(versionedBinary)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not read versioned launcher %s: %v — "+
			"the launcher binary may not match the activated payload\n", versionedBinary, err)
		return
	}
	defer src.Close()

	binDir := filepath.Join(ediktRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not create %s: %v — "+
			"the launcher binary may not match the activated payload\n", binDir, err)
		return
	}
	dest := filepath.Join(binDir, "edikt")
	tmp := dest + fmt.Sprintf(".new.%d", os.Getpid())

	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		os.Remove(tmp)
		return
	}
	out.Close()
	_ = os.Chmod(tmp, 0o755)

	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
	}
}
