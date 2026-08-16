package reextract

// snapshot.go — a git-independent "before" state for the per-artifact
// review.
//
// DESIGN-QUESTIONS-2026-08-16.md Q3, option (a). The clean-tree
// precondition existed for exactly one reason, stated in
// commands/gov/reextract.md's own text: "the per-artifact review in this
// command reads `git diff` to show you what changed — that only works
// starting from a clean baseline." That is a review-baseline choice, not a
// correctness or safety property of re-extraction itself.
//
// The ledger already has the closest primitive to what a git-independent
// review needs: SidecarSHA256 makes "did this file change since dispatch"
// a checkable claim. What was missing was not the hash-comparison
// capability — it is a stored copy of the PRE-REWRITE bytes to diff
// against, since a hash tells you THAT something changed, not WHAT.
//
// Snapshotting happens in Run(), in the same loop that captures
// pinnedBefore — before ANY dispatch starts, so the sidecar's on-disk
// bytes at that point are still what extraction is about to overwrite.

import (
	"os"
	"path/filepath"
)

// SnapshotRelDir is where pre-rewrite sidecar copies live, relative to the
// project root. Gitignored alongside the rest of .edikt/state/ — these are
// working state for one review session, not committed evidence.
const SnapshotRelDir = ".edikt/state/reextract-snapshots"

// snapshotPath returns the path a given artifact's pre-rewrite sidecar copy
// is stored at.
func snapshotPath(root, artifactID string) string {
	return filepath.Join(root, SnapshotRelDir, artifactID+".edikt.yaml")
}

// writeSnapshot copies the sidecar's CURRENT on-disk bytes to the snapshot
// path, before dispatch overwrites them. Best-effort and non-fatal by
// design, matching this package's own fail-open-but-never-silent posture
// elsewhere (Run already tolerates a failed body-digest stamp the same
// way): a snapshot failure must not block re-extraction, since the
// extraction itself is the operation with real value, and a git-based
// review remains available as a fallback if the tree happens to be clean.
func writeSnapshot(root, artifactID, sidecarPath string) error {
	b, err := os.ReadFile(sidecarPath)
	if err != nil {
		return err
	}
	dst := snapshotPath(root, artifactID)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// SnapshotPath is the exported form, for the tier-1 command and `gov
// reextract` CLI surface to construct the same path independently (the
// review step needs to know where to diff against; restore needs to know
// where to restore from).
func SnapshotPath(root, artifactID string) string {
	return snapshotPath(root, artifactID)
}
