package cmd

// migrate_sidecars.go — Phase 6 of the sidecar architecture plan.
//
// `edikt migrate sidecars` lifts existing in-body
// [edikt:directives:start]: # … [edikt:directives:end]: # sentinel blocks
// into co-located <artifact>.edikt.yaml sidecars conforming to
// templates/schemas/sidecar.v1.schema.json (v1; renamed from
// sidecar.schema.json in v0.6.0 per PLAN-sidecar-review-fixes #31).
//
// Two lift paths:
//   - v0.5.x / v0.6.0-rc1 (sentinel has source_hash + topic + signals):
//     pure mechanical map.
//   - v0.4.3 legacy (sentinel has content_hash, no topic/signals): mechanical
//     directives lift; topic/signals re-extracted by dispatching the locked
//     sidecar-extractor agent via `claude -p /edikt:<type>:compile <id>`.
//
// Always paired:
//   1. dry-run plan first  (writes .edikt/state/migration-dry-run.json)
//   2. apply               (refuses without prior dry-run within 24h, unless --force)
//
// Idempotent: re-running --apply on an already-migrated repo is a no-op.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/idvalidate"
	"github.com/diktahq/edikt/tools/edikt/internal/parse"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/spf13/cobra"
)

var (
	migrateSidecarsDryRun bool
	migrateSidecarsApply  bool
	migrateSidecarsForce  bool
	migrateSidecarsJSON   bool
)

var migrateSidecarsCmd = &cobra.Command{
	Use:   "sidecars",
	Short: "Lift in-body sentinel blocks into co-located *.edikt.yaml sidecars",
	Long: `Lift existing in-body [edikt:directives:start]: # … [edikt:directives:end]: #
sentinel blocks from ADRs / invariants / guidelines into co-located
<artifact>.edikt.yaml sidecars (Phase 6, ADR-027 (sidecar architecture)
and ADR-028 (two-phase compile)).

Detects per-artifact schema (v0.4.3 legacy vs v0.5.x/v0.6.0-rc1) and applies
the correct lift path. Mandatory --dry-run before --apply.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !migrateSidecarsDryRun && !migrateSidecarsApply {
			return fmt.Errorf("must pass --dry-run or --apply")
		}
		if migrateSidecarsDryRun && migrateSidecarsApply {
			return fmt.Errorf("--dry-run and --apply are mutually exclusive")
		}
		ediktRoot, _ := resolveEdiktRoot()
		projectRoot, err := os.Getwd()
		if err != nil {
			return err
		}
		return runMigrateSidecars(projectRoot, ediktRoot, migrateSidecarsDryRun, migrateSidecarsApply, migrateSidecarsForce, migrateSidecarsJSON)
	},
}

func init() {
	migrateSidecarsCmd.Flags().BoolVar(&migrateSidecarsDryRun, "dry-run", false, "preview the migration plan; writes a dry-run gate file")
	migrateSidecarsCmd.Flags().BoolVar(&migrateSidecarsApply, "apply", false, "apply the migration (requires prior --dry-run within 24h, or --force)")
	migrateSidecarsCmd.Flags().BoolVar(&migrateSidecarsForce, "force", false, "bypass the 24h dry-run gate (test/escape hatch)")
	migrateSidecarsCmd.Flags().BoolVar(&migrateSidecarsJSON, "json", false, "emit the dry-run plan / apply summary as JSON to stdout (suppresses prose UI)")
	migrateCmd.AddCommand(migrateSidecarsCmd)
}

// ─── Schema detection ────────────────────────────────────────────────────────

type schemaKind int

const (
	schemaUnknown schemaKind = iota
	schemaV05x         // source_hash + topic + signals (full v0.5.x)
	schemaV043         // content_hash (v0.4.3 legacy)
	schemaV05xPartial  // source_hash present but topic/signals missing (Phase 8 of PLAN-sidecar-review-fixes #8)
)

// schemaKindLabel renders a schemaKind for warn-line diagnostics so users
// can map the migration verdict back to the detection branch.
func schemaKindLabel(k schemaKind) string {
	switch k {
	case schemaV043:
		return "v0.4.3 legacy"
	case schemaV05x:
		return "v0.5.x full"
	case schemaV05xPartial:
		return "v0.5.x partial"
	default:
		return "unknown"
	}
}

// detectSchema inspects the raw inner YAML body of a sentinel block (the
// bytes between the open and close markers, untrimmed). Returns the
// detected schema kind. Truly unrecognizable blocks return schemaUnknown.
//
// Detection covers four shipped sentinel shapes (Phase 8 of
// PLAN-sidecar-review-fixes #8):
//
//   • content_hash present                  → schemaV043 (legacy)
//   • topic + directives present            → schemaV05x (mechanical)
//   • source_hash present (no topic/dirs)   → schemaV05xPartial (LLM resync)
//   • directives present (no topic/hashes)  → schemaV05xPartial (LLM resync)
//   • otherwise                             → schemaUnknown
//
// The dogfood corpus exposed three real shapes the original Phase 8
// detection missed:
//   1. ADRs that were hand-authored before /edikt:adr:compile shipped
//      hash backfill (have topic + directives + paths + scope but no
//      source_hash and no signals).
//   2. ADRs whose sentinel only carries a flat directives: list (no
//      topic, no hashes) — these are the earliest sentinel shape.
//   3. The Phase-8-original case: source_hash present without topic.
//
// All three must lift cleanly; the mechanical path handles (1) and the
// LLM-resync path handles (2) and (3). governance/tooling.md line 6
// documents the broader principle: topic is optional and falls back to
// LLM grouping, so migrate must accept any sentinel with content worth
// lifting and route the gaps through the locked extractor.
func detectSchema(inner string) schemaKind {
	hasContentHash := hasTopLevelKey(inner, "content_hash")
	hasSourceHash := hasTopLevelKey(inner, "source_hash")
	hasTopic := hasTopLevelKey(inner, "topic") || hasTopLevelKey(inner, "topics")
	hasDirectives := hasTopLevelKey(inner, "directives")

	if hasContentHash {
		return schemaV043
	}
	if hasTopic && hasDirectives {
		return schemaV05x
	}
	if hasSourceHash || hasDirectives {
		return schemaV05xPartial
	}
	return schemaUnknown
}

// ─── Fence detection (defensive) ─────────────────────────────────────────────

// sentinelInsideFence reports whether the open-marker offset falls inside a
// fenced code block. parse.ExtractSentinel already screens column-0 + fenced
// regions, but we re-check defensively as the spec requires.
//
// Fence-state tracking is CommonMark-conformant per Phase 3 §3.2: the
// closing fence MUST use the same marker character as the opener AND
// its run length MUST be ≥ the opener's. Mixed-marker close lines are
// treated as ordinary content rather than toggling `inFence` — without
// this, a `~~~` line inside a ``` block (or vice versa) silently flips
// the state and a fenced sentinel example escapes the skip.
func sentinelInsideFence(body string, openOffset int) bool {
	pos := 0
	inFence := false
	var openerChar byte
	var openerLen int
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		lineEnd := pos + len(line)
		trimmed := strings.TrimSpace(line)
		fenceChar, fenceLen := fencePrefix(trimmed)
		if openOffset >= pos && openOffset <= lineEnd {
			return inFence
		}
		if !inFence && fenceLen >= 3 {
			inFence = true
			openerChar = fenceChar
			openerLen = fenceLen
		} else if inFence && fenceLen >= openerLen && fenceChar == openerChar {
			inFence = false
		}
		pos = lineEnd + 1 // +1 for the newline
	}
	return false
}

// fencePrefix mirrors parse.fencePrefix. Duplicated here because the
// parse package's helper is unexported (intentional — internal/parse is
// the canonical home for fence parsing). Keeping the migrate copy small
// and side-by-side with sentinelInsideFence localizes the logic.
func fencePrefix(trimmed string) (byte, int) {
	if len(trimmed) == 0 {
		return 0, 0
	}
	c := trimmed[0]
	if c != '`' && c != '~' {
		return 0, 0
	}
	n := 1
	for n < len(trimmed) && trimmed[n] == c {
		n++
	}
	if n < 3 {
		return 0, 0
	}
	return c, n
}

// ─── Candidate discovery ─────────────────────────────────────────────────────

type artifactDirs struct {
	decisions  string
	invariants string
	guidelines string
}

func resolveArtifactDirs(projectRoot string) artifactDirs {
	d := artifactDirs{
		decisions:  filepath.Join(projectRoot, "docs/architecture/decisions"),
		invariants: filepath.Join(projectRoot, "docs/architecture/invariants"),
		guidelines: filepath.Join(projectRoot, "docs/guidelines"),
	}
	cfg := filepath.Join(projectRoot, ".edikt", "config.yaml")
	data, err := os.ReadFile(cfg)
	if err != nil {
		return d
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	inPaths := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "paths:") {
			inPaths = true
			continue
		}
		if inPaths {
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
				ts := strings.TrimSpace(line)
				if strings.HasPrefix(ts, "decisions:") {
					d.decisions = filepath.Join(projectRoot, strings.TrimSpace(strings.TrimPrefix(ts, "decisions:")))
				} else if strings.HasPrefix(ts, "invariants:") {
					d.invariants = filepath.Join(projectRoot, strings.TrimSpace(strings.TrimPrefix(ts, "invariants:")))
				} else if strings.HasPrefix(ts, "guidelines:") {
					d.guidelines = filepath.Join(projectRoot, strings.TrimSpace(strings.TrimPrefix(ts, "guidelines:")))
				}
			} else if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "#") {
				inPaths = false
			}
		}
	}
	return d
}

// migrationSkipMarkerRe matches the inline content marker
// `<!-- edikt:migration:skip reason="…" -->`. The reason= clause is
// optional but recommended; when absent, the skip reason is recorded
// as "marker comment present".
var migrationSkipMarkerRe = regexp.MustCompile(`<!--\s*edikt:migration:skip(?:\s+reason="([^"]*)")?\s*-->`)

// isSkipListed inspects the .md at path and reports whether migration
// should leave it alone. Two opt-in mechanisms (Phase 6 of
// PLAN-sidecar-review-fixes #16) — the v0.5.x hardcoded prefix list
// has been removed:
//
//  1. YAML frontmatter declaring `migration: skip` (with optional
//     `reason: "…"`) or `documents_legacy_format: true`.
//  2. Body marker `<!-- edikt:migration:skip reason="…" -->` near the
//     top of the file (within the first 4 KiB to bound the scan).
//
// Any read error is treated as "not skipped" so a file that cannot be
// read still flows through the normal lift / failure path; isSkipListed
// is a fast-path filter, not the place to surface I/O problems.
func isSkipListed(path string) (bool, string) {
	// README files in artifact directories are directory-level docs,
	// not governance artifacts — they describe the directory's
	// purpose to humans and have no directives to migrate. The
	// v0.6.0-rc3 dogfood compile surfaced docs/guidelines/README.md
	// as a spurious migration target that the user then had to write
	// an empty stub sidecar for. Skip them by name (any case).
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
		reason := ""
		if len(m) > 1 {
			reason = strings.TrimSpace(m[1])
		}
		if reason == "" {
			reason = "marker comment present"
		}
		return true, reason
	}
	return false, ""
}

// parseFrontmatterMigrationSkip extracts a skip declaration from the YAML
// frontmatter body. Recognizes only top-level scalar keys; nested or
// sequence forms are ignored. Returns the reason on hit; defaults the
// reason to a stable description of the trigger key when none is given.
func parseFrontmatterMigrationSkip(front string) (string, bool) {
	var migrationVal, legacyVal, reasonVal string
	sc := bufio.NewScanner(strings.NewReader(front))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
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

// candidate is one .md considered for migration.
type candidate struct {
	mdPath     string
	artifactID string // e.g. ADR-001 (extracted from filename); "" if unparseable
	kind       string // "adr" | "invariant" | "guideline"
}

// planCache carries planArtifact's already-read body and parsed sentinel
// across to applyArtifact. Phase 7 of PLAN-sidecar-review-fixes #44 — the
// previous flow re-read the file and re-parsed the sentinel inside apply,
// which doubled the I/O and ExtractSentinel cost on every migrated
// artifact. Now apply reuses what plan computed.
type planCache struct {
	body     string
	sentinel parse.Sentinel
	innerYAML string
	schema   schemaKind
}

func collectCandidates(projectRoot string) []candidate {
	dirs := resolveArtifactDirs(projectRoot)
	var out []candidate
	walk := func(root, kind string) {
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(p, ".md") {
				return nil
			}
			if skip, _ := isSkipListed(p); skip {
				return nil
			}
			out = append(out, candidate{mdPath: p, artifactID: extractArtifactID(p), kind: kind})
			return nil
		})
	}
	walk(dirs.decisions, "adr")
	walk(dirs.invariants, "invariant")
	walk(dirs.guidelines, "guideline")
	sort.Slice(out, func(i, j int) bool { return out[i].mdPath < out[j].mdPath })
	return out
}

var artifactIDFromFile = regexp.MustCompile(`^([A-Z]+-[0-9]+)-`)

func extractArtifactID(p string) string {
	base := filepath.Base(p)
	m := artifactIDFromFile.FindStringSubmatch(base)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// ─── Lift logic ──────────────────────────────────────────────────────────────

type liftResult struct {
	cand        candidate
	sidecarPath string
	action      string // "dry-mechanical" | "dry-legacy-llm" | "wrote" | "wrote-partial" | "skipped" | "failed" | "already-migrated"
	directives  int
	needsLLM    bool
	handReviews int
	err         error
	warnLines   []string

	// cache populated by planArtifact and reused by applyArtifact so the
	// .md is read once and the sentinel parsed once per artifact. Empty
	// when action ∈ {"skipped", "already-migrated"} or when the read /
	// parse failed (in which case action carries the corresponding
	// failure status).
	cache *planCache
}

func planArtifact(c candidate) liftResult {
	res := liftResult{cand: c, sidecarPath: sidecarPathFor(c.mdPath)}

	body, err := os.ReadFile(c.mdPath)
	if err != nil {
		res.action = "skipped"
		res.warnLines = append(res.warnLines, fmt.Sprintf("read failed: %v", err))
		return res
	}
	bodyStr := string(body)

	sent, err := parse.ExtractSentinel(bodyStr)
	if err != nil {
		res.action = "skipped"
		res.warnLines = append(res.warnLines, fmt.Sprintf("sentinel parse: %v", err))
		return res
	}
	if !sent.Present {
		// Already-migrated case: sidecar exists and no in-body sentinel.
		if _, statErr := os.Stat(res.sidecarPath); statErr == nil {
			res.action = "already-migrated"
		} else {
			res.action = "skipped"
		}
		return res
	}

	// Defensive fence check (parse already screens, but spec requires it).
	if sentinelInsideFence(bodyStr, sent.StartByte) {
		res.action = "skipped"
		return res
	}

	// Read the inner YAML directly so detection isn't gated on the parser.
	inner := extractInnerYAML(bodyStr, sent)
	kind := detectSchema(inner)
	res.cache = &planCache{body: bodyStr, sentinel: sent, innerYAML: inner, schema: kind}
	switch kind {
	case schemaUnknown:
		res.action = "skipped"
		res.warnLines = append(res.warnLines,
			fmt.Sprintf("migrate sidecars: skipping %s — unrecognized schema", c.mdPath))
		return res
	case schemaV05x:
		res.action = "dry-mechanical"
		res.directives = len(sent.Directives)
	case schemaV05xPartial:
		// Source_hash present but topic/signals missing. Mechanical lift
		// can't synthesize them, so apply will dispatch the locked
		// extractor (same path as v0.4.3); plan just records the
		// resync requirement.
		res.action = "dry-llm-resync"
		res.directives = len(sent.Directives)
		res.needsLLM = true
	case schemaV043:
		res.action = "dry-legacy-llm"
		res.directives = len(sent.Directives)
		res.needsLLM = true
	}
	return res
}

func extractInnerYAML(body string, sent parse.Sentinel) string {
	const open = "[edikt:directives:start]: #"
	const close = "[edikt:directives:end]: #"
	startInner := sent.StartByte + len(open)
	endInner := sent.EndByte - len(close)
	if startInner < 0 || endInner < startInner || endInner > len(body) {
		return ""
	}
	return strings.TrimSpace(body[startInner:endInner])
}

func sidecarPathFor(mdPath string) string {
	dir := filepath.Dir(mdPath)
	base := strings.TrimSuffix(filepath.Base(mdPath), ".md")
	return filepath.Join(dir, base+".edikt.yaml")
}

// relPathOrBase returns target relative to projectRoot when filepath.Rel
// succeeds and the result does not escape (no leading "..") — that is the
// shape the schema documents and the IsStale resolver expects. When
// projectRoot is empty or Rel fails (e.g. cross-volume on Windows, or a
// caller that legitimately has no project context like a tmp-dir unit
// test), fall back to the basename so the sidecar still validates.
func relPathOrBase(projectRoot, target string) string {
	if projectRoot == "" {
		return filepath.Base(target)
	}
	r, err := filepath.Rel(projectRoot, target)
	if err != nil {
		return filepath.Base(target)
	}
	if strings.HasPrefix(r, "..") {
		return filepath.Base(target)
	}
	return filepath.ToSlash(r)
}

// applyArtifact performs the actual write for a planned candidate.
//
// projectRoot is the directory the sidecar's `path:` field is relative to
// (per templates/schemas/sidecar.v1.schema.json). When projectRoot is "" or
// filepath.Rel fails, applyArtifact falls back to the .md basename so the
// schema's minLength: 1 still holds — but doctor's PATH MISMATCH check
// (Phase 7) will flag that fallback at the next health check.
func applyArtifact(c candidate, ediktRoot, projectRoot string) liftResult {
	res := planArtifact(c)
	if res.action == "skipped" || res.action == "already-migrated" {
		return res
	}
	if res.cache == nil {
		// Defensive: planArtifact populates the cache on every non-skip /
		// non-already-migrated path. A nil cache here would indicate a
		// regression in plan; refuse to proceed rather than silently
		// re-reading the file and masking the bug.
		res.action = "failed"
		res.err = fmt.Errorf("internal: planArtifact returned action=%q with no cached body/sentinel", res.action)
		return res
	}
	bodyStr := res.cache.body
	sent := res.cache.sentinel

	// Build the prose body (everything outside the sentinel) for line-anchoring.
	prose := bodyStr[:sent.StartByte] + bodyStr[sent.EndByte:]

	sc := sidecar.Sidecar{
		SchemaVersion:        1,
		Path:                 relPathOrBase(projectRoot, c.mdPath),
		Signals:              dedupAndSort(sent.Signals),
		ManualDirectives:     sent.ManualDirectives,
		SuppressedDirectives: sent.SuppressedDirectives,
		Reminders:            sent.Reminders,
		Verification:         sent.Verification,
	}
	if sent.Topic != "" {
		sc.Topic = sent.Topic
	} else if len(sent.Topics) > 0 {
		sc.Topic = sent.Topics[0]
	}

	for _, dtext := range sent.Directives {
		dir := sidecar.Directive{Text: dtext}
		ls, le, q, ok := findDirectiveSource(prose, dtext)
		if ok {
			dir.SourceExcerpt = sidecar.SourceExcerpt{LineStart: ls, LineEnd: le, Quote: q}
		} else {
			truncated := dtext
			if len(truncated) > 200 {
				truncated = truncated[:200]
			}
			dir.SourceExcerpt = sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: truncated}
			res.handReviews++
			res.warnLines = append(res.warnLines, fmt.Sprintf(
				"migrate sidecars: %s: directive %q has no source match — flagged for hand-review",
				c.mdPath, truncate(dtext, 60)))
		}
		sc.Directives = append(sc.Directives, dir)
	}

	// Schema-detection branch: v0.4.3 (content_hash) and partial v0.5.x
	// (source_hash without topic/signals) cannot lift mechanically because
	// topic/signals are missing. Per ADR-030, the tier-2 binary is
	// LLM-agnostic — we write a partial sidecar with topic: needs-review
	// and let the host-agent-driven tier-1 markdown
	// (commands/upgrade.md → /edikt:<kind>:compile) dispatch the locked
	// sidecar-extractor agent for resync. The previous in-Go
	// `exec.Command(claude, -p, slash)` hard-coded a Claude CLI dependency
	// that broke under Codex / Cursor / any non-Claude host agent.
	kind := res.cache.schema
	var legacyPartial bool
	if kind == schemaV043 || kind == schemaV05xPartial {
		// INV-006: the artifact id + type are still validated even
		// though we no longer dispatch — they end up in the sidecar's
		// path: field, and a malformed id should never land on disk.
		if err := idvalidate.ArtifactID(c.artifactID); err != nil {
			res.warnLines = append(res.warnLines, fmt.Sprintf(
				"migrate sidecars: %s: artifact id rejected by INV-006 gate (%v) — writing partial mechanical sidecar",
				c.mdPath, err))
		} else if err := idvalidate.ArtifactType(c.kind); err != nil {
			res.warnLines = append(res.warnLines, fmt.Sprintf(
				"migrate sidecars: %s: artifact type rejected by INV-006 gate (%v) — writing partial mechanical sidecar",
				c.mdPath, err))
		} else {
			res.warnLines = append(res.warnLines, fmt.Sprintf(
				"migrate sidecars: %s: %s schema needs LLM resync — writing partial mechanical sidecar with topic: needs-review. Run /edikt:upgrade or /edikt:%s:compile %s under your host agent (Claude / Codex / Cursor) to fill in topic + signals.",
				c.mdPath, schemaKindLabel(kind), c.kind, c.artifactID))
		}
		legacyPartial = true
		sc.Topic = "needs-review"
		sc.Signals = nil
	}

	if sc.Topic == "" {
		sc.Topic = "needs-review"
	}
	if sc.Signals == nil {
		sc.Signals = []string{}
	}

	if err := sc.Validate(); err != nil {
		res.action = "failed"
		res.err = fmt.Errorf("sidecar validate: %w", err)
		return res
	}

	// Marshal canonically.
	out, err := marshalSidecar(&sc)
	if err != nil {
		res.action = "failed"
		res.err = err
		return res
	}

	// Set migration env so any managed-region guards skip ADR bodies.
	_ = os.Setenv("EDIKT_MIGRATION_IN_PROGRESS", "1")
	defer os.Unsetenv("EDIKT_MIGRATION_IN_PROGRESS")

	if err := atomicWriteNoFollow(res.sidecarPath, out, 0o644); err != nil {
		res.action = "failed"
		res.err = err
		return res
	}

	if err := removeSentinelFromMd(c.mdPath, bodyStr, sent); err != nil {
		res.action = "failed"
		res.err = err
		return res
	}

	if legacyPartial {
		res.action = "wrote-partial"
	} else {
		res.action = "wrote"
	}
	return res
}

func removeSentinelFromMd(path, body string, sent parse.Sentinel) error {
	newBody := body[:sent.StartByte] + body[sent.EndByte:]
	// Trim a single trailing blank line that often follows the sentinel.
	newBody = trimDoubleBlank(newBody, sent.StartByte)
	return atomicWriteNoFollow(path, []byte(newBody), 0o644)
}

// trimDoubleBlank removes one redundant blank line at the splice point.
func trimDoubleBlank(s string, at int) string {
	if at <= 0 || at >= len(s) {
		return s
	}
	// Look for "\n\n\n" centered at the splice and collapse to "\n\n".
	for i := at - 2; i <= at && i+3 <= len(s); i++ {
		if i < 0 {
			continue
		}
		if s[i] == '\n' && s[i+1] == '\n' && s[i+2] == '\n' {
			return s[:i+1] + s[i+2:]
		}
	}
	return s
}

// findDirectiveSource searches prose for the directive text and returns the
// 1-indexed line range and the verbatim quote (the matching line(s)).
func findDirectiveSource(prose, directive string) (int, int, string, bool) {
	// Strip a trailing "(ref: …)" tail to broaden matching, but search both forms.
	candidates := []string{directive}
	if idx := strings.LastIndex(directive, "(ref:"); idx > 0 {
		stripped := strings.TrimSpace(directive[:idx])
		stripped = strings.TrimRight(stripped, ".")
		if stripped != "" {
			candidates = append(candidates, stripped)
		}
	}
	for _, needle := range candidates {
		if needle == "" {
			continue
		}
		idx := strings.Index(prose, needle)
		if idx < 0 {
			continue
		}
		// Compute line range for [idx, idx+len(needle)).
		ls := strings.Count(prose[:idx], "\n") + 1
		end := idx + len(needle)
		le := strings.Count(prose[:end], "\n") + 1
		// Quote: the full line(s) containing the match.
		lineStart := strings.LastIndex(prose[:idx], "\n") + 1
		afterEnd := strings.Index(prose[end:], "\n")
		var lineEnd int
		if afterEnd < 0 {
			lineEnd = len(prose)
		} else {
			lineEnd = end + afterEnd
		}
		quote := prose[lineStart:lineEnd]
		quote = strings.TrimSpace(quote)
		if quote == "" {
			quote = needle
		}
		return ls, le, quote, true
	}
	return 0, 0, "", false
}

func dedupAndSort(xs []string) []string {
	if len(xs) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(xs))
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// marshalSidecar serializes a Sidecar with canonical formatting.
// Delegates to sidecar.Marshal so all writers share the same canonical form
// (Phase 8 contract).
func marshalSidecar(s *sidecar.Sidecar) ([]byte, error) {
	return sidecar.Marshal(s)
}

// ─── Dry-run gate ────────────────────────────────────────────────────────────

type dryRunState struct {
	RanAt string `json:"ran_at"`
	Scope string `json:"scope"`
	Cwd   string `json:"cwd"`
}

func dryRunStatePath(ediktRoot string) string {
	return filepath.Join(ediktRoot, "state", "migration-dry-run.json")
}

func writeDryRunState(ediktRoot, cwd string) error {
	if err := os.MkdirAll(filepath.Join(ediktRoot, "state"), 0o755); err != nil {
		return err
	}
	st := dryRunState{
		RanAt: time.Now().UTC().Format(time.RFC3339),
		Scope: "sidecars",
		Cwd:   cwd,
	}
	data, _ := json.MarshalIndent(st, "", "  ")
	return os.WriteFile(dryRunStatePath(ediktRoot), data, 0o644)
}

func checkDryRunGate(ediktRoot, cwd string) error {
	p := dryRunStatePath(ediktRoot)
	data, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("migrate sidecars: --dry-run required first (or pass --force). Run: edikt migrate sidecars --dry-run")
	}
	var st dryRunState
	if err := json.Unmarshal(data, &st); err != nil {
		return fmt.Errorf("migrate sidecars: --dry-run required first (or pass --force). Run: edikt migrate sidecars --dry-run")
	}
	t, err := time.Parse(time.RFC3339, st.RanAt)
	if err != nil || time.Since(t) > 24*time.Hour {
		return fmt.Errorf("migrate sidecars: --dry-run required first (or pass --force). Run: edikt migrate sidecars --dry-run")
	}
	if filepath.Clean(st.Cwd) != filepath.Clean(cwd) {
		return fmt.Errorf("migrate sidecars: --dry-run required first (or pass --force). Run: edikt migrate sidecars --dry-run")
	}
	return nil
}

// ─── Top-level driver ────────────────────────────────────────────────────────

// migrateSidecarsItem is one row in the JSON output's items[] array.
type migrateSidecarsItem struct {
	Source     string `json:"source"`
	Sidecar    string `json:"sidecar"`
	Action     string `json:"action"`
	Directives int    `json:"directives,omitempty"`
	Error      string `json:"error,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// migrateSidecarsJSONOut is the contract surface for `--json` (mirrors the
// shape `verify --json` uses: status / summary / items[]).
type migrateSidecarsJSONOut struct {
	Status  string                `json:"status"`
	Mode    string                `json:"mode"`
	Summary map[string]int        `json:"summary"`
	Items   []migrateSidecarsItem `json:"items"`
	Error   string                `json:"error,omitempty"`
}

func runMigrateSidecars(projectRoot, ediktRoot string, dryRun, apply, force, jsonOut bool) error {
	if apply && !force {
		if err := checkDryRunGate(ediktRoot, projectRoot); err != nil {
			return err
		}
	}

	cands := collectCandidates(projectRoot)

	// In JSON mode the prose header lines and per-row prints go to stderr
	// at low verbosity; stdout is reserved for the single JSON document.
	progressOut := os.Stdout
	if jsonOut {
		progressOut = os.Stderr
	}

	if dryRun {
		fmt.Fprintln(progressOut, "migrate sidecars (dry-run):")
	} else {
		fmt.Fprintln(progressOut, "migrate sidecars (apply):")
	}

	var (
		toCreate    int
		hand        int
		wrote       int
		failed      int
		skipped     int
		alreadyMig  int
		items       []migrateSidecarsItem
	)

	for _, c := range cands {
		var res liftResult
		if dryRun {
			res = planArtifact(c)
		} else {
			res = applyArtifact(c, ediktRoot, projectRoot)
		}

		short := filepath.Base(c.mdPath)
		sidecarShort := filepath.Base(res.sidecarPath)
		item := migrateSidecarsItem{
			Source:     short,
			Sidecar:    sidecarShort,
			Action:     res.action,
			Directives: res.directives,
		}
		switch res.action {
		case "dry-mechanical":
			fmt.Fprintf(progressOut, "  %-40s → %-40s (v0.5.x mechanical, %d directives)\n",
				short, sidecarShort, res.directives)
			toCreate++
		case "dry-legacy-llm":
			fmt.Fprintf(progressOut, "  %-40s → %-40s (v0.4.3 legacy, needs LLM step on apply)\n",
				short, sidecarShort)
			toCreate++
		case "dry-llm-resync":
			fmt.Fprintf(progressOut, "  %-40s → %-40s (v0.5.x partial, needs LLM topic-resync on apply)\n",
				short, sidecarShort)
			toCreate++
		case "wrote":
			fmt.Fprintf(progressOut, "  %-40s → %-40s (wrote, %d directives)\n",
				short, sidecarShort, res.directives)
			wrote++
		case "wrote-partial":
			fmt.Fprintf(progressOut, "  %-40s → %-40s (wrote-partial, topic: needs-review)\n",
				short, sidecarShort)
			wrote++
		case "failed":
			fmt.Fprintf(progressOut, "  %-40s → FAILED: %v\n", short, res.err)
			if res.err != nil {
				item.Error = res.err.Error()
			}
			failed++
		case "skipped":
			if len(res.warnLines) > 0 {
				fmt.Fprintf(progressOut, "  %-40s → SKIPPED (%s)\n", short, res.warnLines[0])
				item.Reason = res.warnLines[0]
			} else {
				fmt.Fprintf(progressOut, "  %-40s → SKIPPED (no sentinel block)\n", short)
				item.Reason = "no sentinel block"
			}
			skipped++
		case "already-migrated":
			alreadyMig++
		}
		items = append(items, item)
		hand += res.handReviews
		for _, w := range res.warnLines {
			fmt.Fprintln(os.Stderr, w)
		}
	}

	fmt.Fprintln(progressOut)
	if dryRun {
		fmt.Fprintf(progressOut, "%d sidecars to create, 0 conflicts, %d hand-reviews.\n", toCreate, hand)
		if err := writeDryRunState(ediktRoot, projectRoot); err != nil {
			fmt.Fprintf(os.Stderr, "warn: could not write dry-run gate: %v\n", err)
		}
	} else {
		fmt.Fprintf(progressOut, "%d sidecars wrote, %d failed, %d skipped, %d already-migrated.\n",
			wrote, failed, skipped, alreadyMig)
	}

	emitEvent(ediktRoot, "sidecar_migration_complete", map[string]interface{}{
		"mode":             modeOf(dryRun),
		"to_create":        toCreate,
		"wrote":            wrote,
		"failed":           failed,
		"skipped":          skipped,
		"already_migrated": alreadyMig,
		"hand_reviews":     hand,
	})

	if jsonOut {
		out := migrateSidecarsJSONOut{
			Status: "ok",
			Mode:   modeOf(dryRun),
			Summary: map[string]int{
				"to_create":        toCreate,
				"wrote":            wrote,
				"failed":           failed,
				"skipped":          skipped,
				"already_migrated": alreadyMig,
				"hand_reviews":     hand,
			},
			Items: items,
		}
		if items == nil {
			out.Items = []migrateSidecarsItem{}
		}
		if failed > 0 {
			out.Status = "error"
			out.Error = fmt.Sprintf("%d artifacts failed to migrate", failed)
		}
		body, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(os.Stdout, string(body))
	}

	if failed > 0 {
		return fmt.Errorf("%d artifacts failed to migrate", failed)
	}
	return nil
}

func modeOf(dry bool) string {
	if dry {
		return "dry-run"
	}
	return "apply"
}
