package cmd

// doctor_routed_sources.go — "Routed source files" check, ported from
// the python heredoc previously embedded in commands/doctor.md.
//
// Walks the routing surface (.claude/rules/governance.md and
// .claude/rules/governance/*.md), extracts every cited ADR/INV ID via
// `(ref: ADR-NNN)` / `(ref: INV-NNN)` patterns, and verifies each one
// resolves to a source file under paths.decisions or paths.invariants.
// Missing source = ERROR — a compiled surface cites an artifact that points at empty
// disk space.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/surfaces"
)

// citedRefRe matches `(ref: ADR-NNN)` or `(ref: INV-NNN)`. Tolerates
// whitespace around the ID, mirrors the python regex
// `\(ref:\s*(ADR-\d+|INV-\d+)\s*\)`.
var citedRefRe = regexp.MustCompile(`\(ref:\s*(ADR-\d+|INV-\d+)\s*\)`)

// runRoutedSourcesCheck validates that every cited ADR/INV ID in the
// routing surface resolves to an existing source file. Returns
// (errors, warnings, ran). ran is false when the routing surface is
// absent (project never ran gov:compile).
func runRoutedSourcesCheck(projectRoot string, w io.Writer) (errs, warns int, ran bool) {
	rulesDir := filepath.Join(projectRoot, ".claude", "rules")
	if _, err := os.Stat(rulesDir); err != nil {
		return 0, 0, false
	}

	// SURFACES COME FROM THE MANIFEST, not from a directory walk. A walk finds
	// whatever happens to be in the directory — including an orphan left by a
	// renamed topic — and misses any surface rendered elsewhere. The manifest
	// is the enumeration compile actually produced, which is the set this
	// check is supposed to be about.
	//
	// A project with no manifest falls back to the walk, because refusing
	// there would turn an ordinary pre-manifest project into a doctor failure.
	man, manErr := surfaces.Load(projectRoot)
	var routingFiles []string
	if manErr == nil && man.Present {
		if core, err := man.Resolve(surfaces.KindAmbientCore, ""); err == nil {
			routingFiles = append(routingFiles, core)
		}
		routingFiles = append(routingFiles, man.PathsOfKind(surfaces.KindTopicFile)...)
		routingFiles = append(routingFiles, man.PathsOfKind(surfaces.KindSkillPackage)...)
	} else {
		indexFile := filepath.Join(rulesDir, "governance.md")
		if _, err := os.Stat(indexFile); err == nil {
			routingFiles = append(routingFiles, indexFile)
		}
		govDir := filepath.Join(rulesDir, "governance")
		if entries, err := os.ReadDir(govDir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				routingFiles = append(routingFiles, filepath.Join(govDir, e.Name()))
			}
		}
	}
	if len(routingFiles) == 0 {
		return 0, 0, false
	}

	cited := map[string]struct{}{}
	var unreadable []string
	for _, f := range routingFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			// A surface the manifest lists but that cannot be read is a BROKEN
			// CONTRACT, not an absent one. Skipping it silently — which this
			// loop used to do — makes a manifest pointing at a moved file
			// indistinguishable from a corpus with fewer surfaces, and the
			// check then reports on whatever remains as though that were the
			// whole set.
			unreadable = append(unreadable, fmt.Sprintf("%s: %v", f, err))
			continue
		}
		for _, m := range citedRefRe.FindAllStringSubmatch(string(data), -1) {
			cited[m[1]] = struct{}{}
		}
	}

	dirs := resolveArtifactDirs(projectRoot)

	var ids []string
	for id := range cited {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	type missing struct {
		id       string
		expected string
	}
	var miss []missing
	resolved := 0
	for _, id := range ids {
		searchDir := dirs.decisions
		if strings.HasPrefix(id, "INV") {
			searchDir = dirs.invariants
		}
		if searchDir == "" {
			miss = append(miss, missing{id, fmt.Sprintf("%s-*.md", id)})
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(searchDir, id+"-*.md"))
		if len(matches) == 0 {
			miss = append(miss, missing{id, filepath.Join(searchDir, id+"-*.md")})
			continue
		}
		// Confirm at least one match is a regular file.
		hit := false
		for _, m := range matches {
			if info, err := os.Stat(m); err == nil && !info.IsDir() {
				hit = true
				break
			}
		}
		if hit {
			resolved++
		} else {
			miss = append(miss, missing{id, filepath.Join(searchDir, id+"-*.md")})
		}
	}

	if len(unreadable) > 0 {
		fmt.Fprintf(w, "  [error] Routed sources — %d listed surface(s) could not be read:\n", len(unreadable))
		for _, u := range unreadable {
			fmt.Fprintf(w, "          %s\n", u)
		}
		fmt.Fprintf(w, "          The render manifest names a surface that is not on disk. Re-run `edikt gov compile`.\n")
		errs += len(unreadable)
	}

	// NAME THE SURFACES, not just the count. A check that reports "12 of 12
	// resolve" is the same output whether it walked every surface or one, and
	// AC-4.6's whole point is that a consumer which silently skips surfaces it
	// can no longer find still exits 0.
	if len(routingFiles) > 0 {
		rels := make([]string, 0, len(routingFiles))
		for _, f := range routingFiles {
			if r, rerr := filepath.Rel(projectRoot, f); rerr == nil {
				rels = append(rels, r)
			} else {
				rels = append(rels, f)
			}
		}
		sort.Strings(rels)
		src := "the render manifest"
		if manErr != nil || !man.Present {
			src = "a directory walk (no render manifest present)"
		}
		fmt.Fprintf(w, "       surfaces (from %s): %s\n", src, strings.Join(rels, ", "))
	}

	if len(miss) == 0 {
		fmt.Fprintf(w, "  [ok] Routed sources — %d of %d resolve\n", resolved, len(ids))
		return 0, 0, true
	}
	for _, m := range miss {
		fmt.Fprintf(w, "  [FAIL] Missing source for routed directive: %s expected at %s\n", m.id, m.expected)
		errs++
	}
	return errs, warns, true
}
