package cmd

// doctor_orphan_surfaces.go — backstop for gov compile's manifest-diff
// orphan cleanup (phaseb/merge.go's ORPHAN CLEANUP, AC-2.5), which can only
// remove a surface it finds listed in the PREVIOUS manifest it reads. A
// surface that fell outside manifest tracking for any reason — it existed
// before manifest tracking started, or a manifest was reset/lost between
// compiles — becomes permanently invisible to that cleanup: there is no
// history for the next compile to diff against, so it is never named, never
// removed, and never reported. Confirmed live: a topic file with a stale
// compile_schema_version and real directive content, absent from the
// current manifest, sat undetected in a real project's governance/ dir.
//
// This matters beyond disk hygiene: `.claude/rules/governance/*.md` is
// Claude Code's own ambient rule-load path. A file sitting there loads
// regardless of whether `gov compile` still considers it current — an
// orphaned topic file is not inert, it is stale governance still being
// delivered to every session.
//
// This check does the tree walk the manifest-diff cleanup deliberately
// does NOT do (manifest.go's own comment: "a cleanup that guesses would
// delete" a file someone added on purpose) — but reporting is not deleting,
// so the caution that correctly stops automatic deletion does not apply to
// surfacing the fact for a human to look at.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/phaseb"
)

// runOrphanSurfacesCheck compares every .md file actually present in
// .claude/rules/governance/ against the current manifest's topic-file
// entries. Returns the warning count (0 or 1) and whether the check ran
// (false when the governance dir or manifest don't exist yet — nothing to
// check, not a failure).
func runOrphanSurfacesCheck(projectRoot string, out io.Writer) (warnCount int, ran bool) {
	govDir := filepath.Join(projectRoot, ".claude", "rules", "governance")
	manifestPath := filepath.Join(govDir, phaseb.ManifestName)

	manifestBody, err := os.ReadFile(manifestPath)
	if err != nil {
		return 0, false // no manifest yet — nothing to compare against
	}
	tracked := map[string]bool{}
	for _, e := range phaseb.ParseManifest(string(manifestBody)) {
		tracked[filepath.Base(e.Path)] = true
	}

	entries, err := os.ReadDir(govDir)
	if err != nil {
		return 0, false
	}

	var orphans []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if !tracked[e.Name()] {
			orphans = append(orphans, e.Name())
		}
	}
	if len(orphans) == 0 {
		return 0, true
	}

	fmt.Fprintf(out, "  WARN: %d file(s) in .claude/rules/governance/ are not in the current manifest — not produced by the last compile, but still load as ambient rules:\n", len(orphans))
	for _, name := range orphans {
		fmt.Fprintf(out, "    orphan: %s\n", name)
	}
	fmt.Fprintln(out, "  These predate manifest tracking or survived a lost/reset manifest — gov compile's own cleanup only removes what it finds in the PREVIOUS manifest, so it cannot see these. Review and delete manually if stale.")
	return 1, true
}
