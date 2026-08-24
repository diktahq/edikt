package cmd

// doctor_lost_migration.go — detects artifacts whose legacy sentinel block was
// destroyed by the migrator shipped in v0.7.0-rc1 … v0.7.3.
//
// Those builds stripped the in-body [edikt:directives:start] block from an
// artifact after writing its sidecar. For blocks they could parse, the contents
// were carried into migration_preserved first. For blocks they could NOT parse
// — hand-edited ones, and shapes predating the current format — nothing was
// carried: the sidecar recorded schema_detected and no content, the block was
// removed from the .md, and the command reported success.
//
// WHY THIS IS A PRECISE SIGNATURE, NOT A GUESS.
//
// Current builds preserve an unparseable block verbatim under raw_body. So
// "migration_preserved carrying schema_detected and nothing else" is not a
// general marker of an unparseable sentinel — a current migration of the very
// same block produces raw_body. It can now only be produced by a migration run
// on a pre-fix binary. That precision is the whole value of this check: it
// names artifacts that LOST content rather than artifacts that merely had an
// odd sentinel shape.
//
// WHY THIS DOES NOT USE THE LOSSLESS GATE.
//
// compile's Phase B lossless check would be the natural home, and it cannot
// run: a sidecar carrying migration_preserved is unconditionally stale, and
// Phase A refuses to reach Phase B while anything is stale. A detection path
// routed through it would report nothing and read as "you are fine" — the exact
// failure this check exists to surface. It reads sidecars directly instead.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// lostMigrationState is the per-artifact verdict. The three states are kept
// distinct on purpose: collapsing "no preservation block" into "healthy" would
// make a project that never migrated indistinguishable from one that migrated
// cleanly, and collapsing "lost" into either would hide the finding.
type lostMigrationState int

const (
	// lostMigrationNotApplicable — no migration_preserved block. The artifact
	// either never carried a sentinel or has already completed Phase B.
	lostMigrationNotApplicable lostMigrationState = iota
	// lostMigrationPreserved — carries directives (or other structured lists)
	// or raw_body. Its legacy content survived the migration.
	lostMigrationPreserved
	// lostMigrationLost — carries schema_detected and nothing else.
	lostMigrationLost
)

// classifyLostMigration decides which of the three states a sidecar is in.
// Split out from the walk so it can be tested without staging a project.
func classifyLostMigration(sc *sidecar.Sidecar) lostMigrationState {
	if sc == nil || sc.MigrationPreserved == nil {
		return lostMigrationNotApplicable
	}
	if sc.MigrationPreserved.HasContent() {
		return lostMigrationPreserved
	}
	return lostMigrationLost
}

// runLostMigrationCheck reports artifacts whose sentinel content was destroyed
// by a pre-fix migration. Returns (warnings, ran). ran == false when there is
// nothing to inspect, so a corpus that was never examined is not reported as a
// clean one — an absent measurement is not a passing measurement.
func runLostMigrationCheck(projectRoot string, w io.Writer) (warns int, ran bool) {
	cfg, err := govrun.LoadConfig(projectRoot)
	if err != nil {
		return 0, false
	}
	dirs := []string{cfg.Paths.Decisions, cfg.Paths.Invariants, cfg.Paths.Guidelines}
	pairs, derr := sidecar.Discover(projectRoot, dirs)
	if derr != nil {
		return 0, false
	}

	var lost, preserved []string
	inspected := 0
	for _, p := range pairs {
		if p.SidecarPath == "" {
			continue
		}
		if _, sErr := os.Stat(p.SidecarPath); sErr != nil {
			continue
		}
		sc, lErr := sidecar.Load(p.SidecarPath)
		if lErr != nil {
			// A sidecar that will not load is already reported by the
			// Sidecar Health group. Not this check's failure mode, and
			// counting it either way would be a guess.
			continue
		}
		inspected++
		name := p.ArtifactID
		if name == "" {
			name = filepath.Base(p.SidecarPath)
		}
		switch classifyLostMigration(sc) {
		case lostMigrationLost:
			lost = append(lost, name)
		case lostMigrationPreserved:
			preserved = append(preserved, name)
		}
	}

	if inspected == 0 {
		return 0, false
	}

	sort.Strings(lost)
	sort.Strings(preserved)

	fmt.Fprintln(w, "  ── Migration Content Loss ─────────────────────")

	if len(lost) == 0 {
		if len(preserved) > 0 {
			fmt.Fprintf(w, "  OK: %d artifact(s) awaiting extraction, all with their legacy content preserved\n", len(preserved))
		} else {
			fmt.Fprintf(w, "  OK: no artifact carries a migration baseline (%d sidecar(s) inspected)\n", inspected)
		}
		return 0, true
	}

	fmt.Fprintf(w, "  WARN: %d artifact(s) lost their legacy directive block during migration:\n", len(lost))
	for _, id := range lost {
		fmt.Fprintf(w, "    lost: %s\n", id)
	}
	fmt.Fprintln(w, "    Their migration_preserved block records what the sentinel looked like but")
	fmt.Fprintln(w, "    holds none of its content. The block was removed from the .md and preserved")
	fmt.Fprintln(w, "    nowhere. This is only produced by edikt v0.7.0-rc1 … v0.7.3.")
	fmt.Fprintln(w, "    Recover the pre-migration .md from version control, then re-run")
	fmt.Fprintln(w, "    `edikt migrate sidecars --dry-run` and `--apply` on this build.")
	if len(preserved) > 0 {
		fmt.Fprintf(w, "    (%d other artifact(s) carry a migration baseline and are intact.)\n", len(preserved))
	}
	return len(lost), true
}
