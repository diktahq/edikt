package cmd

// doctor_duplicate_ids.go — two artifacts holding the same id.
//
// edikt had no duplicate-id detection anywhere. `next-id` now refuses to MINT a
// taken id, but that only helps at allocation time: an id can collide from a
// merge, a manual copy, a bad rename, or a branch nobody ran next-id on. The
// allocator stops you creating one; this finds the ones already there. Without
// both, the class stays open.
//
// WHY IT MATTERS MORE THAN A FILENAME CLASH. An id is a relationship anchor.
// Governance refers to artifacts by id — supersessions, amendments, `(ref:
// ADR-NNN)` tails on every directive. A reported case had a draft minted onto an
// id whose live occupant was already the named specialization the draft claimed
// to be establishing. Committing would not have produced a rename conflict; it
// would have produced the structural drift the new record existed to prevent.
//
// BOTH PATHS ARE NAMED, never one. Which of two artifacts is the intruder is not
// something the tool can know — the newer file may be the correct one and the
// older the stale copy. Naming one would be a guess presented as a finding.
//
// Scope is the working tree. An id held in another checkout is parallel work,
// not corruption; that is next-id's problem, at allocation time, and it is
// handled there.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
)

// dupHolder is one artifact claiming an id.
type dupHolder struct {
	ID   string
	Path string
}

// collectArtifactIDs walks the configured artifact directories of one project
// root and returns id -> paths. Directories that do not exist are skipped;
// directories that cannot be read set readFailed, because an unreadable
// directory is not an empty one and a check that treated it as empty would
// report a clean result it did not measure.
func collectArtifactIDs(projectRoot string, dirs []string) (map[string][]string, bool) {
	out := map[string][]string{}
	readFailed := false

	for _, rel := range dirs {
		if strings.TrimSpace(rel) == "" {
			continue
		}
		abs := rel
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(projectRoot, rel)
		}
		entries, err := os.ReadDir(abs)
		if err != nil {
			if !os.IsNotExist(err) {
				readFailed = true
			}
			continue
		}
		for _, e := range entries {
			name := e.Name()
			// A SPEC is a directory; every other kind is a .md file.
			if !e.IsDir() && !strings.HasSuffix(name, ".md") {
				continue
			}
			// artifact_id.go is the ONE extractor; this file used to carry its
			// own `^([A-Z]+)-(\d+)` copy.
			id, ok := artifactIDFrom(name)
			if !ok {
				continue
			}
			p := filepath.Join(rel, name)
			out[id] = append(out[id], filepath.ToSlash(p))
		}
	}
	return out, readFailed
}

// runDuplicateIDCheck reports ids held by more than one artifact. Returns
// (warnings, ran). ran == false when there was nothing to inspect, so an
// unexamined corpus is never reported as a clean one.
func runDuplicateIDCheck(projectRoot string, w io.Writer) (warns int, ran bool) {
	cfg, err := govrun.LoadConfig(projectRoot)
	if err != nil {
		return 0, false
	}
	dirs := []string{
		cfg.Paths.Decisions,
		cfg.Paths.Invariants,
		cfg.Paths.Guidelines,
		cfg.Paths.Specs,
		cfg.Paths.Prds,
		cfg.Paths.Discovery,
		cfg.Paths.Brainstorms,
	}

	byID, readFailed := collectArtifactIDs(projectRoot, dirs)
	if len(byID) == 0 && !readFailed {
		return 0, false
	}

	var dupIDs []string
	for id, paths := range byID {
		if len(paths) > 1 {
			dupIDs = append(dupIDs, id)
		}
	}
	sort.Strings(dupIDs)

	fmt.Fprintln(w, "  ── Duplicate Artifact IDs ─────────────────────")

	if readFailed {
		// A directory that could not be read is not a directory with no
		// artifacts, and saying "no duplicates" over an unread corpus would be
		// the exact defect this check exists to catch: an absent measurement
		// reported as a clean one.
		fmt.Fprintln(w, "  WARN: one or more artifact directories could not be read — this scan is INCOMPLETE")
		warns++
	}

	if len(dupIDs) == 0 {
		fmt.Fprintf(w, "  OK: %d artifact id(s) inspected, none held twice\n", len(byID))
		return warns, true
	}

	fmt.Fprintf(w, "  WARN: %d artifact id(s) held by more than one artifact:\n", len(dupIDs))
	for _, id := range dupIDs {
		paths := byID[id]
		sort.Strings(paths)
		fmt.Fprintf(w, "    %s — %d holders:\n", id, len(paths))
		for _, p := range paths {
			fmt.Fprintf(w, "      %s\n", p)
		}
	}
	fmt.Fprintln(w, "    Both paths are named because which one is the intruder is not something")
	fmt.Fprintln(w, "    edikt can know. An id is a relationship anchor — governance refers to it in")
	fmt.Fprintln(w, "    supersessions, amendments and every (ref: ID) tail — so resolve it by")
	fmt.Fprintln(w, "    deciding which artifact owns the id, then renaming the other and updating")
	fmt.Fprintln(w, "    references to it.")
	return warns + len(dupIDs), true
}
