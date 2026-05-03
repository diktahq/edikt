package sidecar

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Pair couples a parent .md with its co-located .edikt.yaml.
// Sidecar is nil when the .yaml is missing on disk; LoadErr captures any
// validation/parse error so the caller can surface it without aborting the
// whole walk.
//
// Skip is true when the parent .md opted out of sidecar coverage via one of:
//   - frontmatter `migration: skip` / `documents_legacy_format: true`
//   - body comment marker `<!-- edikt:migration:skip reason="…" -->`
//   - `**Status:** Superseded by ADR-NNN` line (INV-002-compliant: no body
//     edit required, the status line was already there at acceptance time).
//
// SkipReason carries the human-readable reason for diagnostics. Callers
// that emit "sidecar missing" errors MUST suppress them when Skip is true.
type Pair struct {
	ParentPath  string
	SidecarPath string
	ArtifactID  string
	Sidecar     *Sidecar
	LoadErr     error
	Skip        bool
	SkipReason  string
}

// migrationSkipMarkerRe matches the inline body marker
// `<!-- edikt:migration:skip reason="…" -->`. Mirrors the regex in
// tools/edikt/cmd/migrate_sidecars.go — the two MUST stay in sync.
var migrationSkipMarkerRe = regexp.MustCompile(`<!--\s*edikt:migration:skip(?:\s+reason="([^"]*)")?\s*-->`)

// supersededStatusRe matches the canonical ADR status line
// `**Status:** Superseded by ADR-NNN` (case-insensitive, multiline).
// Superseded ADRs are historical references and never require a sidecar.
var supersededStatusRe = regexp.MustCompile(`(?mi)^\*\*Status:\*\*\s+Superseded\s+by\s+\S+`)

// isSkipListed inspects the .md at path and reports whether it opts out
// of sidecar coverage. Returns (true, reason) on hit. The first 4 KiB of
// the file is inspected to bound the scan. Read errors map to "not skipped"
// so a corrupted .md still flows through the normal load path.
func isSkipListed(path string) (bool, string) {
	base := strings.ToLower(filepath.Base(path))
	if base == "readme.md" {
		return true, "README files in artifact directories are documentation, not governance"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, ""
	}
	head := data
	if len(head) > 4096 {
		head = head[:4096]
	}
	text := string(head)

	if strings.HasPrefix(text, "---\n") {
		if end := strings.Index(text[4:], "\n---"); end >= 0 {
			if reason, ok := parseFrontmatterMigrationSkip(text[4 : 4+end]); ok {
				return true, reason
			}
		}
	}
	if m := migrationSkipMarkerRe.FindStringSubmatch(text); m != nil {
		reason := strings.TrimSpace(m[1])
		if reason == "" {
			reason = "marker comment present"
		}
		return true, reason
	}
	if supersededStatusRe.MatchString(text) {
		return true, "ADR superseded — directives no longer authoritative"
	}
	return false, ""
}

// parseFrontmatterMigrationSkip recognises the same two frontmatter scalars
// as the cmd package: `migration: skip` (with optional `reason: "…"`) and
// `documents_legacy_format: true`.
func parseFrontmatterMigrationSkip(front string) (string, bool) {
	var migrationVal, legacyVal, reasonVal string
	for _, line := range strings.Split(front, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "migration:"):
			migrationVal = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "migration:")), `"' `)
		case strings.HasPrefix(line, "documents_legacy_format:"):
			legacyVal = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "documents_legacy_format:")), `"' `)
		case strings.HasPrefix(line, "reason:"):
			reasonVal = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "reason:")), `"' `)
		}
	}
	if migrationVal == "skip" {
		if reasonVal != "" {
			return reasonVal, true
		}
		return "frontmatter migration: skip", true
	}
	if strings.EqualFold(legacyVal, "true") {
		if reasonVal != "" {
			return reasonVal, true
		}
		return "documents_legacy_format: true", true
	}
	return "", false
}

// Discover walks the artifact dirs (decisions, invariants, guidelines) under
// projectRoot and returns one Pair per parent .md. Sidecars without a
// matching parent are reported separately by the doctor (out of scope here).
func Discover(projectRoot string, dirs []string) ([]Pair, error) {
	var pairs []Pair
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		absDir := filepath.Join(projectRoot, dir)
		entries, err := os.ReadDir(absDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, ".edikt.yaml") {
				continue
			}
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			parentPath := filepath.Join(absDir, name)
			sidecarPath := strings.TrimSuffix(parentPath, ".md") + ".edikt.yaml"
			p := Pair{
				ParentPath:  parentPath,
				SidecarPath: sidecarPath,
				ArtifactID:  artifactIDFromName(name),
			}
			if skip, reason := isSkipListed(parentPath); skip {
				p.Skip = true
				p.SkipReason = reason
			}
			if _, err := os.Stat(sidecarPath); err == nil {
				sc, lerr := Load(sidecarPath)
				if lerr != nil {
					p.LoadErr = lerr
				} else {
					p.Sidecar = sc
				}
			}
			pairs = append(pairs, p)
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].ParentPath < pairs[j].ParentPath })
	return pairs, nil
}

// HasAnySidecar returns true iff at least one .edikt.yaml exists under any
// of the artifact dirs. Used by the cobra entry point to dispatch
// two-phase compile vs. surfacing the pre-migration error per ADR-027 §5.
func HasAnySidecar(projectRoot string, dirs []string) bool {
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		absDir := filepath.Join(projectRoot, dir)
		entries, err := os.ReadDir(absDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.HasSuffix(e.Name(), ".edikt.yaml") {
				return true
			}
		}
	}
	return false
}

// HasAnyGovernanceMarkdown returns true iff at least one governance .md
// (a non-sidecar .md whose basename does not start with "_") exists under
// any of the artifact dirs. Distinguishes a pre-migration project (has
// .md but no .edikt.yaml — must hard-fail per ADR-027 §5) from an empty
// project (no .md at all — compile is a no-op).
func HasAnyGovernanceMarkdown(projectRoot string, dirs []string) bool {
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		absDir := filepath.Join(projectRoot, dir)
		entries, err := os.ReadDir(absDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, ".edikt.yaml") {
				continue
			}
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			if strings.HasPrefix(name, "_") {
				continue
			}
			return true
		}
	}
	return false
}

func artifactIDFromName(name string) string {
	base := strings.TrimSuffix(name, ".md")
	if strings.HasPrefix(base, "ADR-") || strings.HasPrefix(base, "INV-") {
		end := 4
		for end < len(base) && base[end] >= '0' && base[end] <= '9' {
			end++
		}
		return base[:end]
	}
	return base
}
