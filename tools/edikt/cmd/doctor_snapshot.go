package cmd

// doctor_snapshot.go — SPEC-009 Plan D Phase 7 (SR-015).  // edikt-guard:allow
//
// Adopter-facing snapshot drift detection. Exposes the
// frozen-snapshot regression mechanism (Plan A Phase 11; the bash
// script at test/integration/compile-snapshot-regression.sh) as a
// doctor flag adopters can run against their own project's compiled
// governance surface.
//
// Snapshot format: sha256sum-style manifest at
// .edikt/snapshot/governance-checksums.txt with one `<hex>  <path>`
// line per .claude/rules/governance.md and
// .claude/rules/governance/*.md file. The format matches the
// edikt-internal baseline at test/fixtures/compile-snapshots/ so
// the same diff logic works for adopters and edikt itself.
//
// Flags wired in doctor.go:
//   --check-snapshot     enable this mode (suppresses the standard
//                        doctor health checks; the snapshot check
//                        becomes the sole subject)
//   --fail-on-drift      with --check-snapshot, exit 1 on drift
//                        (default: drift is reported but doctor
//                        exits 0)
//   --create-snapshot    with --check-snapshot, write a fresh
//                        baseline at .edikt/snapshot/governance-
//                        checksums.txt and exit 0
//
// All three flags are documented on `edikt doctor --help`.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// _ keeps io referenced when the body's io.Copy in sha256File (defined
// in upgrade.go) is the only use we don't directly import. We DO still
// use io.Writer in the runSnapshotCheck signature.
var _ io.Writer

// snapshotRelPath is the canonical project-relative path to the
// adopter's frozen baseline. The directory `.edikt/snapshot/` is NOT
// in `.edikt/.gitignore` — adopters check the baseline into their repo
// so it ships with the code and can be reviewed in PRs.
const snapshotRelPath = ".edikt/snapshot/governance-checksums.txt"

// governanceLiveRoot is the relative root of the compiled governance
// surface that edikt's gov compile writes. Snapshot scans this tree
// for governance.md (top-level) and governance/*.md (per-topic) files.
const governanceLiveRoot = ".claude/rules"

// snapshotOpts groups the per-invocation flags for the snapshot
// subcheck. Populated from cobra flags in doctor.go.
type snapshotOpts struct {
	FailOnDrift    bool
	CreateSnapshot bool
}

// runSnapshotCheck is the entry point invoked from doctor.go when
// --check-snapshot is set. Returns the exit code to pass back to
// the caller.
//
// Exit codes:
//
//	0 — snapshot matches live tree, OR drift exists but --fail-on-drift
//	    was NOT set, OR --create-snapshot ran and wrote the baseline
//	1 — drift exists AND --fail-on-drift was set
//	2 — operational error (live tree unreadable, snapshot dir
//	    unwritable when creating, etc.)
//
// Output goes to the provided writer for testability.
func runSnapshotCheck(projectRoot string, opts snapshotOpts, w io.Writer) int {
	live, err := scanLiveGovernance(projectRoot)
	if err != nil {
		fmt.Fprintf(w, "  ERROR: snapshot check: scan live tree: %v\n", err)
		return 2
	}
	if len(live) == 0 {
		fmt.Fprintf(w, "  Snapshot: no compiled governance found at %s/governance{,/*}.md — run `edikt gov compile` first.\n", governanceLiveRoot)
		return 0
	}

	snapshotPath := filepath.Join(projectRoot, snapshotRelPath)

	// --create-snapshot writes the manifest from the current live tree
	// and exits without comparing. Overwrites any existing baseline.
	if opts.CreateSnapshot {
		if err := writeSnapshotManifest(snapshotPath, live); err != nil {
			fmt.Fprintf(w, "  ERROR: snapshot create: %v\n", err)
			return 2
		}
		fmt.Fprintf(w, "  Snapshot: created at %s (%d files captured)\n", snapshotRelPath, len(live))
		return 0
	}

	baseline, err := readSnapshotManifest(snapshotPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(w, "  Snapshot: NOT FOUND at %s\n", snapshotRelPath)
			fmt.Fprintf(w, "    To capture the current compiled governance tree as the baseline:\n")
			fmt.Fprintf(w, "      bin/edikt doctor --check-snapshot --create-snapshot\n")
			return 0
		}
		fmt.Fprintf(w, "  ERROR: snapshot read: %v\n", err)
		return 2
	}

	added, removed, changed := diffSnapshots(baseline, live)
	if len(added) == 0 && len(removed) == 0 && len(changed) == 0 {
		fmt.Fprintf(w, "  Snapshot: OK — %d files match baseline at %s\n", len(live), snapshotRelPath)
		return 0
	}

	// Drift report.
	fmt.Fprintf(w, "  Snapshot: DRIFT — %d added, %d removed, %d changed\n",
		len(added), len(removed), len(changed))
	for _, f := range added {
		fmt.Fprintf(w, "    + %s\n", f)
	}
	for _, f := range removed {
		fmt.Fprintf(w, "    - %s\n", f)
	}
	for _, f := range changed {
		fmt.Fprintf(w, "    ~ %s\n", f)
	}
	fmt.Fprintf(w, "    To refresh the baseline after a deliberate change:\n")
	fmt.Fprintf(w, "      bin/edikt doctor --check-snapshot --create-snapshot\n")

	if opts.FailOnDrift {
		return 1
	}
	return 0
}

// scanLiveGovernance walks .claude/rules/governance.md (if present)
// and .claude/rules/governance/*.md and returns a sha256-keyed map
// of relative-path → hex digest. Relative paths are normalized to
// the form "governance.md" and "governance/<topic>.md" so the
// manifest is stable across projects with different repo roots.
func scanLiveGovernance(projectRoot string) (map[string]string, error) {
	out := make(map[string]string)
	root := filepath.Join(projectRoot, governanceLiveRoot)

	// Top-level governance.md.
	topPath := filepath.Join(root, "governance.md")
	if info, err := os.Stat(topPath); err == nil && !info.IsDir() {
		digest, err := sha256File(topPath)
		if err != nil {
			return nil, fmt.Errorf("hash %s: %w", topPath, err)
		}
		out["governance.md"] = digest
	}

	// Per-topic .md files under governance/.
	subDir := filepath.Join(root, "governance")
	entries, err := os.ReadDir(subDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			p := filepath.Join(subDir, name)
			digest, err := sha256File(p)
			if err != nil {
				return nil, fmt.Errorf("hash %s: %w", p, err)
			}
			out["governance/"+name] = digest
		}
	}
	// Absent subDir is OK (some projects have only the top-level file).

	return out, nil
}

// sha256File is defined in upgrade.go (same signature, same behavior).
// We reuse it here rather than declaring a sibling.

// readSnapshotManifest parses a sha256sum-style manifest:
//
//	<hex-digest>  <relative-path>\n
//
// Empty lines and lines starting with '#' are skipped. Returns
// os.IsNotExist-compatible error when the file is missing.
func readSnapshotManifest(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// sha256sum format: "<hex>  <path>". Two spaces; the second
		// is the separator (the first is part of the binary-mode
		// indicator that we don't honor — we always read text).
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("manifest %s line %d: malformed (expected 'hex<sp><sp>path')", path, i+1)
		}
		digest := strings.TrimSpace(parts[0])
		rel := strings.TrimSpace(parts[1])
		if len(digest) != 64 {
			return nil, fmt.Errorf("manifest %s line %d: digest %q is not 64 hex chars", path, i+1, digest)
		}
		out[rel] = strings.ToLower(digest)
	}
	return out, nil
}

// writeSnapshotManifest serializes the live map back to the manifest
// format, sorted by relative path so the output is deterministic
// across runs (git-friendly).
func writeSnapshotManifest(path string, live map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}

	rels := make([]string, 0, len(live))
	for r := range live {
		rels = append(rels, r)
	}
	sort.Strings(rels)

	var b strings.Builder
	for _, r := range rels {
		fmt.Fprintf(&b, "%s  %s\n", live[r], r)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// diffSnapshots returns three sorted lists of relative paths:
//
//	added   — present in live but not baseline
//	removed — present in baseline but not live
//	changed — present in both but with different digests
//
// All paths are returned sorted lexicographically so output is
// stable across runs.
func diffSnapshots(baseline, live map[string]string) (added, removed, changed []string) {
	for rel, liveDigest := range live {
		baseDigest, ok := baseline[rel]
		switch {
		case !ok:
			added = append(added, rel)
		case baseDigest != liveDigest:
			changed = append(changed, rel)
		}
	}
	for rel := range baseline {
		if _, ok := live[rel]; !ok {
			removed = append(removed, rel)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(changed)
	return
}
