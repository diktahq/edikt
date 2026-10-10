package cmd

// doctor_schema_pin.go — the standing report for a config/corpus schema
// disagreement: edikt_version claims a schema line the corpus does not
// actually have. ADR-082 removed the upgrade-flow gate that used to withhold
// the edikt_version bump until migration completed (a project-level flag
// the decision rejects — state is per artifact); doctor is now the only
// place this disagreement is reported, for a manually edited edikt_version,
// an interrupted upgrade, or a corpus lifted after the version write.
//
// Once edikt_version is on the v0.8+ line (versionLineFloor), every
// governed sidecar is expected to be schema_version 2. A schema_version 1
// sidecar surviving alongside that pin means a future `gov compile`
// silently inherits a corpus the running binary cannot fully process
// (Phase A's dispatch gate refuses while any v1-shaped sidecar exists) —
// exactly the state a premature version bump produces.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// runSchemaPinCheck reports a WARN when edikt_version is on the v0.8+ line
// (which requires v2-shaped sidecars) but the corpus still contains a
// schema_version 1 sidecar. Returns 1 if it warned, 0 otherwise — 0 covers
// both "no disagreement" and "nothing to check" (no config, no pin, no
// governed sidecars), which the caller does not need to distinguish since
// neither is an actionable state for this check.
func runSchemaPinCheck(projectRoot string, out io.Writer) int {
	configPath := filepath.Join(projectRoot, ".edikt", "config.yaml")
	pinned := readPinnedVersion(configPath)
	if pinned == "" {
		return 0
	}
	if versionLineBelowFloor(pinned) {
		// Pinned below the line that requires v2 — schema_version 1 is
		// expected here, not a disagreement. ensureVersionLine (ADR-042)  // edikt-guard:allow
		// already refuses project-operating commands in this state.
		return 0
	}

	cfg, err := govrun.LoadConfig(projectRoot)
	if err != nil {
		return 0 // config unreadable — not this check's failure mode to report
	}
	dirs := []string{cfg.Paths.Decisions, cfg.Paths.Invariants, cfg.Paths.Guidelines}
	pairs, err := sidecar.Discover(projectRoot, dirs)
	if err != nil {
		return 0
	}

	var stale []string
	for _, p := range pairs {
		if p.SidecarPath == "" {
			continue
		}
		raw, rerr := os.ReadFile(p.SidecarPath)
		if rerr != nil {
			continue
		}
		would, cerr := sidecar.WouldConvertToV2(raw)
		if cerr != nil || !would {
			continue
		}
		stale = append(stale, p.ArtifactID)
	}
	if len(stale) == 0 {
		return 0
	}

	fmt.Fprintf(out, "  WARN: edikt_version is v%s (requires schema_version 2) but %d sidecar(s) are still schema_version 1:\n", pinned, len(stale))
	for _, id := range stale {
		fmt.Fprintf(out, "    stale: %s\n", id)
	}
	fmt.Fprintln(out, "  Run: edikt migrate lift")
	return 1
}
