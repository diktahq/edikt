package sidecar

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/parse"
)

// Pair couples a parent .md with its co-located .edikt.yaml.
// Sidecar is nil when the .yaml is missing on disk; LoadErr captures any
// validation/parse error so the caller can surface it without aborting the
// whole walk.
//
// Skip is true when the parent .md opted out of sidecar coverage via one of:
// - frontmatter `migration: skip` / `documents_legacy_format: true`
// - body comment marker `<!-- edikt:migration:skip reason="…" -->`
// - `**Status:** Superseded by ADR-NNN` line (: no body
// edit required, the status line was already there at acceptance time).
//
// SkipReason carries the human-readable reason for diagnostics. Callers
// that emit "sidecar missing" errors MUST suppress them when Skip is true.
type Pair struct {
	ParentPath  string
	SidecarPath string
	ArtifactID  string
	// Kind is the artifact class, derived from WHICH configured directory
	// the parent .md was found in — never from its filename. Inferring the
	// class from the ID prefix ("not ADR- and not INV- ⇒ guideline")
	// mis-classifies every README and stray note in the decisions and
	// invariants directories, which is how `governance.md` came to report
	// phantom guideline counts. One of KindADR, KindInvariant,
	// KindGuideline, or "" when the dir list carried no class for it.
	Kind       string
	Sidecar    *Sidecar
	LoadErr    error
	Skip       bool
	SkipReason string
}

// Artifact classes for Pair.Kind.
const (
	KindADR       = "adr"
	KindInvariant = "invariant"
	KindGuideline = "guideline"
)

// dirKinds is the positional class contract for the dirs slice passed to
// Discover: [decisions, invariants, guidelines]. That is exactly what
// govrun.GovernanceDirs returns, and it is the only shape any production
// caller passes. Positions beyond the third get no class.
var dirKinds = []string{KindADR, KindInvariant, KindGuideline}

// migrationSkipMarkerRe matches the inline body marker
// `<!-- edikt:migration:skip reason="…" -->`. Mirrors the regex in
// tools/edikt/cmd/migrate_sidecars.go — the two MUST stay in sync.
var migrationSkipMarkerRe = regexp.MustCompile(`<!--\s*edikt:migration:skip(?:\s+reason="([^"]*)")?\s*-->`)

// sidecarSkipMarkerRe is the body-comment form of an explicit, per-artifact
// "do not require a sidecar here" opt-out (N2,
// docs/internal/audits/TRIAGE-2026-08-20-bok-services-governance-projection.md).
// Distinct from migrationSkipMarkerRe: that one means "pre-migration, needs
// lifting into a sidecar"; this one means "deliberately not projected,
// possibly forever" — a proposed ADR/invariant a project isn't ready to
// compile, or one split out by an owner ruling that hasn't been accepted yet.
// `status: proposed` alone is NOT sufficient reason to skip MISSING — this
// repo's own ADR-063 is proposed and has a legitimate, real sidecar — so the  edikt-guard:allow
// opt-out must be explicit per artifact, not inferred from status.
var sidecarSkipMarkerRe = regexp.MustCompile(`<!--\s*edikt:sidecar:skip(?:\s+reason="([^"]*)")?\s*-->`)

// supersededStatusRe matches the canonical ADR status body lines
// `**Status:** Superseded by ADR-NNN` and `**Status:** Deprecated`
// (case-insensitive, multiline). Retired ADRs are historical references
// and never require a sidecar.
var supersededStatusRe = regexp.MustCompile(`(?mi)^\*\*Status:\*\*\s+(Superseded\s+by\s+\S+|Deprecated\b)`)

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
			front := text[4 : 4+end]
			if reason, ok := parseFrontmatterMigrationSkip(front); ok {
				return true, reason
			}
			if reason, ok := parseFrontmatterRetiredStatus(front); ok {
				return true, reason
			}
			if reason, ok := parseFrontmatterSidecarSkip(front); ok {
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
	if m := sidecarSkipMarkerRe.FindStringSubmatch(text); m != nil {
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

// parseFrontmatterRetiredStatus recognises frontmatter-only retirement:
// `status: superseded` (any "superseded…" value, e.g. "Superseded by
// ADR-027") and `status: deprecated`. Field regression: ADRs retired with
// frontmatter status but no bolded body `**Status:**` line were
// re-bootstrapped by Phase A and their directives compiled as duplicates.
func parseFrontmatterRetiredStatus(front string) (string, bool) {
	for _, line := range strings.Split(front, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "status:") {
			continue
		}
		val := strings.ToLower(strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "status:")), `"' `))
		switch {
		case strings.HasPrefix(val, "superseded"):
			return "frontmatter status: superseded — directives no longer authoritative", true
		case strings.HasPrefix(val, "deprecated"):
			return "frontmatter status: deprecated — directives no longer authoritative", true
		}
	}
	return "", false
}

// parseFrontmatterSidecarSkip recognises `sidecar: skip` (with optional
// `reason: "…"`) — an explicit, per-artifact opt-out from sidecar coverage
// that carries no implication about the artifact's status. Kept as its own
// key rather than folded into migration:skip or a status inference: this is
// "deliberately not projected" (a proposed record awaiting acceptance, or
// one an owner ruling split out and hasn't accepted yet), not "pre-migration"
// and not "retired." Named `sidecar:`, not `no-directives:` — that key
// already means something unrelated (a compile-time warning suppressor) and
// reusing it is exactly the naming collision N3 documents.
func parseFrontmatterSidecarSkip(front string) (string, bool) {
	var sidecarVal, reasonVal string
	for _, line := range strings.Split(front, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "sidecar:"):
			sidecarVal = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "sidecar:")), `"' `)
		case strings.HasPrefix(line, "reason:"):
			reasonVal = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "reason:")), `"' `)
		}
	}
	if sidecarVal != "skip" {
		return "", false
	}
	if reasonVal != "" {
		return reasonVal, true
	}
	return "frontmatter sidecar: skip", true
}

// IsSkipListed is the exported form of the skip-list check, for callers
// outside this package (verify all, doctor) that need to know whether a
// governance .md opted out of sidecar coverage.
func IsSkipListed(path string) (bool, string) {
	return isSkipListed(path)
}

// Discover walks the artifact dirs (decisions, invariants, guidelines) under
// projectRoot and returns one Pair per parent .md. Sidecars without a
// matching parent are reported separately by the doctor (out of scope here).
func Discover(projectRoot string, dirs []string) ([]Pair, error) {
	var pairs []Pair
	for i, dir := range dirs {
		if dir == "" {
			continue
		}
		kind := ""
		if i < len(dirKinds) {
			kind = dirKinds[i]
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
				Kind:        kind,
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
// two-phase compile vs. surfacing the pre-migration error.
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

// HasAnyLegacySentinel returns true iff at least one governance .md under
// the artifact dirs still carries a column-0 `[edikt:directives:start]: #`
// block outside a fenced region.
//
// This is what separates the two states that both present as
// "markdown but no sidecars":
//
//   - PRE-MIGRATION — the project used edikt <v0.6.0, so directives live in
//     in-body sentinel blocks. `edikt migrate sidecars --apply` lifts them.
//   - NEVER-INITIALISED — the artifacts predate edikt entirely (hand-written
//     ADRs, no edikt history). There is nothing to migrate; the sidecars must
//     be extracted from prose by the per-artifact `:compile` commands.
//
// Conflating the two is what deadlocked adoption on existing codebases:
// compile said "run migrate", migrate found nothing to lift and exited 0,
// and compile still refused. Skip-listed artifacts (superseded ADRs,
// migration:skip markers, READMEs) are excluded — they are never migrated,
// so their sentinels must not pin the whole project to the pre-migration
// path.
func HasAnyLegacySentinel(projectRoot string, dirs []string) bool {
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
			if strings.HasSuffix(name, ".edikt.yaml") || !strings.HasSuffix(name, ".md") {
				continue
			}
			p := filepath.Join(absDir, name)
			if skip, _ := isSkipListed(p); skip {
				continue
			}
			body, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			// parse.ExtractSentinel screens fenced regions via the
			// CommonMark-conformant fence tracker, so a documentation
			// example inside a ``` block does not count.
			sent, perr := parse.ExtractSentinel(string(body))
			if perr != nil {
				continue
			}
			if sent.Present {
				return true
			}
		}
	}
	return false
}

// HasAnyGovernanceMarkdown returns true iff at least one governance .md
// (a non-sidecar .md whose basename does not start with "_") exists under
// any of the artifact dirs. Distinguishes a pre-migration project (has
// .md but no .edikt.yaml — must hard-fail
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
