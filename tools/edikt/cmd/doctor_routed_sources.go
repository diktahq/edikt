package cmd

// doctor_routed_sources.go — "Routed source files" check (SPEC-005
// Phase 2 / AC-004) ported from the python heredoc previously embedded
// in commands/doctor.md (lines 121-161 pre-Phase 11.5).
//
// Walks the routing surface (.claude/rules/governance.md and
// .claude/rules/governance/*.md), extracts every cited ADR/INV ID via
// `(ref: ADR-NNN)` / `(ref: INV-NNN)` patterns, and verifies each one
// resolves to a source file under paths.decisions or paths.invariants.
// Missing source = ERROR — the model's routing table points at empty
// disk space.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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

	var routingFiles []string
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
	if len(routingFiles) == 0 {
		return 0, 0, false
	}

	cited := map[string]struct{}{}
	for _, f := range routingFiles {
		data, err := os.ReadFile(f)
		if err != nil {
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
