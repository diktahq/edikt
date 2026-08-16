package cmd

// migrate_sidecars.go — Phase 6 of the sidecar architecture plan.
//
// `edikt migrate sidecars` lifts existing in-body
// [edikt:directives:start]: # … [edikt:directives:end]: # sentinel blocks
// into co-located <artifact>.edikt.yaml sidecars conforming to
// templates/schemas/gov-sidecar.v1.schema.json (v1; renamed from
// sidecar.schema.json in v0.6.0 per PLAN-sidecar-review-fixes #31).
//
// Two lift paths:
// - v0.5.x / v0.6.0-rc1 (sentinel has source_hash + topic + signals):
// pure mechanical map.
// - v0.4.3 legacy (sentinel has content_hash, no topic/signals): mechanical
// directives lift; topic/signals re-extracted by dispatching the locked
// sidecar-extractor agent via `claude -p /edikt:<type>:compile <id>`.
//
// Always paired:
// 1. dry-run plan first (writes .edikt/state/migration-dry-run.json)
// 2. apply (refuses without prior dry-run within 24h, unless --force)
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

	"github.com/diktahq/edikt/tools/edikt/internal/parse"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/spf13/cobra"
)

var (
	migrateSidecarsDryRun    bool
	migrateSidecarsApply     bool
	migrateSidecarsForce     bool
	migrateSidecarsJSON      bool
	migrateSidecarsStrict    bool
	migrateSidecarsReport    string
	migrateSidecarsV12Output string
)

var migrateSidecarsCmd = &cobra.Command{
	Use:   "sidecars",
	Short: "Lift in-body sentinel blocks into co-located *.edikt.yaml sidecars",
	Long: `Lift existing in-body [edikt:directives:start]: # … [edikt:directives:end]: #
sentinel blocks from ADRs / invariants / guidelines into co-located
<artifact>.edikt.yaml sidecars (the v0.6.0 sidecar architecture + two-phase
compile model).

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
		// Validate report path is a non-empty string that doesn't escape to stdin/stdout aliases.
		if migrateSidecarsReport != "" {
			if migrateSidecarsReport == "-" {
				return fmt.Errorf("--report-json: use a file path, not \"-\"")
			}
		}
		return runMigrateSidecars(projectRoot, ediktRoot, migrateSidecarsDryRun, migrateSidecarsApply, migrateSidecarsForce, migrateSidecarsJSON, migrateSidecarsStrict, migrateSidecarsReport, migrateSidecarsV12Output)
	},
}

func init() {
	migrateSidecarsCmd.Flags().BoolVar(&migrateSidecarsDryRun, "dry-run", false, "preview the migration plan; writes a dry-run gate file")
	migrateSidecarsCmd.Flags().BoolVar(&migrateSidecarsApply, "apply", false, "apply the migration (requires prior --dry-run within 24h, or --force)")
	migrateSidecarsCmd.Flags().BoolVar(&migrateSidecarsForce, "force", false, "bypass the 24h dry-run gate (test/escape hatch)")
	migrateSidecarsCmd.Flags().BoolVar(&migrateSidecarsJSON, "json", false, "emit the dry-run plan / apply summary as JSON to stdout (suppresses prose UI)")
	migrateSidecarsCmd.Flags().BoolVar(&migrateSidecarsStrict, "strict", false, "exit 1 on LOST/FACTUAL regressions, exit 2 on DEGRADED, exit 0 if clean")
	migrateSidecarsCmd.Flags().StringVar(&migrateSidecarsReport, "report-json", "", "write regression manifest to this file path")
	migrateSidecarsCmd.Flags().StringVar(&migrateSidecarsV12Output, "dry-run-output", "", "write v1.2 migration summary JSON to this path (also bypasses dry-run gate; useful for idempotency verification)")
	migrateCmd.AddCommand(migrateSidecarsCmd)
}

// ─── Schema detection ────────────────────────────────────────────────────────

type schemaKind int

const (
	schemaUnknown     schemaKind = iota
	schemaV05x                   // source_hash + topic + signals (full v0.5.x)
	schemaV043                   // content_hash (v0.4.3 legacy)
	schemaV05xPartial            // source_hash present but topic/signals missing (Phase 8 of PLAN-sidecar-review-fixes #8)
)

// schemaKindLabel renders a schemaKind for warn-line diagnostics so users
// can map the migration verdict back to the detection branch.
func schemaKindLabel(k schemaKind) string {
	switch k {
	// These MUST match gov-sidecar.v1.schema.json's schema_detected enum
	// exactly. They did not: this returned "v0.5.x partial" against an enum of
	// "v0.5x-partial", so every migration wrote a schema-invalid value (D44).
	// Nothing caught it because no JSON-schema library was a dependency, so the
	// authoritative schema could not run — and the fixtures were written to
	// match this function rather than the schema.
	//
	// Direction ruled 2026-08-09: fix the EMITTER, do not widen the enum.
	// Widening would make the authoritative definition conform to a broken
	// producer and keep two spellings alive forever.
	case schemaV043:
		return "v0.4.3-legacy"
	case schemaV05x:
		return "v0.5x-full"
	case schemaV05xPartial:
		return "v0.5x-partial"
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
// • content_hash present → schemaV043 (legacy)
// • topic + directives present → schemaV05x (mechanical)
// • source_hash present (no topic/dirs) → schemaV05xPartial (LLM resync)
// • directives present (no topic/hashes) → schemaV05xPartial (LLM resync)
// • otherwise → schemaUnknown
//
// The dogfood corpus exposed three real shapes the original Phase 8
// detection missed:
// 1. ADRs that were hand-authored before /edikt:adr:compile shipped
// hash backfill (have topic + directives + paths + scope but no
// source_hash and no signals).
// 2. ADRs whose sentinel only carries a flat directives: list (no
// topic, no hashes) — these are the earliest sentinel shape.
// 3. The Phase-8-original case: source_hash present without topic.
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
	prds       string
	specs      string
}

func resolveArtifactDirs(projectRoot string) artifactDirs {
	d := artifactDirs{
		decisions:  filepath.Join(projectRoot, "docs/architecture/decisions"),
		invariants: filepath.Join(projectRoot, "docs/architecture/invariants"),
		guidelines: filepath.Join(projectRoot, "docs/guidelines"),
		prds:       filepath.Join(projectRoot, "docs/product/prds"),
		specs:      filepath.Join(projectRoot, "docs/product/specs"),
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
				if rest, ok := strings.CutPrefix(ts, "decisions:"); ok {
					d.decisions = filepath.Join(projectRoot, strings.TrimSpace(rest))
				} else if rest, ok := strings.CutPrefix(ts, "invariants:"); ok {
					d.invariants = filepath.Join(projectRoot, strings.TrimSpace(rest))
				} else if rest, ok := strings.CutPrefix(ts, "guidelines:"); ok {
					d.guidelines = filepath.Join(projectRoot, strings.TrimSpace(rest))
				} else if rest, ok := strings.CutPrefix(ts, "prds:"); ok {
					d.prds = filepath.Join(projectRoot, strings.TrimSpace(rest))
				} else if rest, ok := strings.CutPrefix(ts, "specs:"); ok {
					d.specs = filepath.Join(projectRoot, strings.TrimSpace(rest))
				}
			} else if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "#") {
				inPaths = false
			}
		}
	}
	return d
}

// isSkipListed inspects the .md at path and reports whether migration
// should leave it alone. Two opt-in mechanisms (Phase 6 of
// PLAN-sidecar-review-fixes #16) — the v0.5.x hardcoded prefix list
// has been removed:
//
// 1. YAML frontmatter declaring `migration: skip` (with optional
// `reason: "…"`) or `documents_legacy_format: true`.
// 2. Body marker `<!-- edikt:migration:skip reason="…" -->` near the
// top of the file (within the first 4 KiB to bound the scan).
//
// Any read error is treated as "not skipped" so a file that cannot be
// read still flows through the normal lift / failure path; isSkipListed
// is a fast-path filter, not the place to surface I/O problems.
func isSkipListed(path string) (bool, string) {
	// Single source of truth: internal/sidecar owns "which artifacts need a
	// sidecar", and every consumer must agree. This used to be a second,
	// hand-maintained copy of that rule, and it had drifted in two ways:
	// it never read frontmatter `status: superseded|deprecated`, and its
	// body regex required the `**Status:**` prefix so it missed both
	// `**Superseded by ADR-NNN**` and every `Deprecated` line.
	//
	// The consequence was a live disagreement between tools: `gov compile`
	// and `verify` skipped retired artifacts (shared predicate) while
	// `doctor` demanded sidecars for them (this copy) — five of them in
	// edikt's own repo, and four in a consumer repo where following
	// doctor's advice would have recreated sidecars an owner ruling had
	// deliberately deleted.
	return sidecar.IsSkipListed(path)
}

// candidate is one .md considered for migration.
type candidate struct {
	mdPath     string
	artifactID string // e.g. ADR-NNN (extracted from filename); "" if unparseable
	kind       string // "adr" | "invariant" | "guideline"
}

// planCache carries planArtifact's already-read body and parsed sentinel
// across to applyArtifact. Phase 7 of PLAN-sidecar-review-fixes #44 — the
// previous flow re-read the file and re-parsed the sentinel inside apply,
// which doubled the I/O and ExtractSentinel cost on every migrated
// artifact. Now apply reuses what plan computed.
type planCache struct {
	body      string
	sentinel  parse.Sentinel
	innerYAML string
	schema    schemaKind
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

// scanForResidualSentinels walks every candidate's parent .md AFTER
// migration and returns the paths of any file that still carries a
// column-0 `[edikt:directives:start]: #` marker outside a fenced code
// block. Documentation files that explicitly opt out of migration via
// the skip-list mechanism are excluded — they declare in their own
// frontmatter that they document the legacy format.
func scanForResidualSentinels(cands []candidate) []string {
	var stragglers []string
	for _, c := range cands {
		if skip, _ := isSkipListed(c.mdPath); skip {
			continue
		}
		body, err := os.ReadFile(c.mdPath)
		if err != nil {
			continue
		}
		// parse.ExtractSentinel already screens fenced regions per the
		// CommonMark-conformant fence-state tracker in
		// parse/sentinel.go. .Present == true means a real sentinel
		// survived the strip — a defect, not a documentation example.
		sent, perr := parse.ExtractSentinel(string(body))
		if perr != nil {
			continue
		}
		if sent.Present {
			stragglers = append(stragglers, c.mdPath)
		}
	}
	return stragglers
}

type liftResult struct {
	cand        candidate
	sidecarPath string
	action      string // "dry-preserve" | "wrote" | "skipped" | "failed" | "already-migrated"
	directives  int
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

	// Under the two-phase migration model, schema detection is audit
	// metadata only — it records what the sentinel LOOKED like for
	// diagnostics, but does NOT branch the lift path. Every shape
	// (v0.5x-full / v0.5x-partial / v0.4.3-legacy / unknown / hand-edited)
	// goes through the same applyArtifact: parse → preserve verbatim
	// into migration_preserved → write skeleton → strip sentinel.
	//
	// schemaUnknown previously skipped with a warning; that violated the
	// "no edikt sentinels in user files after migration" invariant
	// (caught by scanForResidualSentinels post-loop verification on
	// real user reports). The skip is gone — the extractor's preservation
	// rules + the post-extractor lossless gate handle whatever shape's
	// content ends up in migration_preserved.
	res.action = "dry-preserve"
	res.directives = len(sent.Directives)
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
// (per templates/schemas/gov-sidecar.v1.schema.json). When projectRoot is "" or
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

	// Two-phase migration (Phase A: structural cleanup).
	//
	// The migrator no longer tries to lift directives into the canonical
	// sidecar.Directives field — that was the source of the drift surface
	// where mechanical lift and LLM extraction produced different shapes.
	// Instead, the legacy sentinel block's content is preserved verbatim
	// into sidecar.MigrationPreserved, the canonical fields are left as a
	// skeleton (topic: needs-extraction, signals: [], directives: []),
	// and the sidecar-extractor agent runs on the next gov:compile
	// (Phase B) to produce the final canonical sidecar — using the
	// preserved baseline as the verbatim source of truth.
	//
	// Result: ALL sentinel shapes (v0.5.x full / v0.5.x partial / v0.4.3
	// legacy / unknown / hand-edited) follow the same path. No schema
	// branching, no findDirectiveSource fuzzy anchoring, no partial
	// sidecars at "needs-review". A single extraction pipeline produces
	// the canonical output.
	preserved := &sidecar.MigrationPreserved{
		SchemaDetected:       schemaKindLabel(res.cache.schema),
		Directives:           append([]string(nil), sent.Directives...),
		ManualDirectives:     append([]string(nil), sent.ManualDirectives...),
		SuppressedDirectives: append([]string(nil), sent.SuppressedDirectives...),
		Reminders:            append([]string(nil), sent.Reminders...),
		Verification:         append([]string(nil), sent.Verification...),
	}
	// Optional hints if the legacy sentinel was v0.5.x-full.
	if sent.Topic != "" {
		preserved.Topic = sent.Topic
	} else if len(sent.Topics) > 0 {
		preserved.Topic = sent.Topics[0]
	}
	if len(sent.Signals) > 0 {
		preserved.Signals = dedupAndSort(sent.Signals)
	}

	// Artifact id/type validation was historically enforced here because
	// the legacy migrate path passed those values into a `claude -p` argv
	// string (the in-Go LLM dispatch, retired in v0.6.0). Under the
	// two-phase model, migrate is purely structural — no argv
	// interpolation — and the dispatch happens later in /edikt:upgrade
	// (slash-side), which validates at its own boundary. Validating
	// here would reject otherwise-fine guideline files whose filenames
	// don't yield a parseable artifact ID (the artifactID is "" for
	// many guideline shapes by design). The slash flow handles the
	// post-extraction case.

	sc := sidecar.Sidecar{
		SchemaVersion:      1,
		Topic:              "needs-extraction",
		Path:               relPathOrBase(projectRoot, c.mdPath),
		Signals:            []string{},
		Directives:         []sidecar.Directive{},
		Paths:              append([]string(nil), sent.Paths...),
		Scope:              append([]string(nil), sent.Scope...),
		MigrationPreserved: preserved,
	}

	if err := sc.Validate(); err != nil {
		res.action = "failed"
		res.err = fmt.Errorf("sidecar validate: %w", err)
		return res
	}

	// No lossless check at the migrate boundary under the two-phase model
	// (Phase A is purely structural; the canonical directives field is
	// empty until Phase B / extractor runs). Lossless verification belongs
	// in Phase A of gov:compile, after the extractor's output is in hand.

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

	// Always "wrote" in the two-phase model — the schema-driven "partial"
	// vs "full" distinction is gone (all artifacts get the same skeleton +
	// MigrationPreserved shape; the extractor produces the canonical
	// content on next compile).
	res.action = "wrote"
	res.directives = len(sent.Directives)
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

func runMigrateSidecars(projectRoot, ediktRoot string, dryRun, apply, force, jsonOut, strict bool, reportJSON string, v12Output string) error {
	// --dry-run-output with --apply acts as --force for the dry-run gate so
	// the v1.2 migration verify command (AC-3.4) is self-contained without
	// needing a prior --dry-run in the same environment.
	if apply && v12Output != "" {
		force = true
	}
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
		toCreate   int
		hand       int
		wrote      int
		failed     int
		skipped    int
		alreadyMig int
		items      []migrateSidecarsItem
		// For --strict / --report-json: accumulate (sentinel, sidecarPath) pairs to diff.
		strictPairs []strictDiffPair
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
		case "dry-preserve":
			// Two-phase migration: every sentinel shape follows the same
			// path. Schema label is audit metadata only.
			label := schemaKindLabel(res.cache.schema)
			fmt.Fprintf(progressOut, "  %-40s → %-40s (%s → migration_preserved, %d directives)\n",
				short, sidecarShort, label, res.directives)
			toCreate++
		case "wrote":
			fmt.Fprintf(progressOut, "  %-40s → %-40s (wrote skeleton, %d preserved directives)\n",
				short, sidecarShort, res.directives)
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
		// Collect pairs for --strict / --report-json diff after apply.
		if (strict || reportJSON != "") && !dryRun && res.cache != nil &&
			(res.action == "wrote" || res.action == "wrote-partial") {
			strictPairs = append(strictPairs, strictDiffPair{
				mdPath:      c.mdPath,
				sidecarPath: res.sidecarPath,
				sentinel:    res.cache.sentinel,
			})
		}
		// Phase A of the two-phase migration is purely structural — no
		// directives are lifted into the canonical sidecar field, so
		// nothing can be lost here. The lossless gate now lives in
		// `edikt gov compile` (Phase B), where it compares the
		// extractor's output against MigrationPreserved.Directives.
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

	// Post-migration invariant: NO edikt sentinel markers remain in any
	// user-authored governance file (ADRs, invariants, guidelines). The
	// two-phase migration's whole point is structural cleanup — leaving
	// vestigial sentinels behind is a defect, not a soft warning. We
	// scan every candidate's parent .md and fail loudly if any column-0
	// [edikt:dir...] marker survived the strip (outside fenced code
	// blocks; in-fence examples in documentation files are not touched).
	if !dryRun && failed == 0 {
		stragglers := scanForResidualSentinels(cands)
		if len(stragglers) > 0 {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "error: residual edikt sentinels detected in user files after migration:")
			for _, s := range stragglers {
				fmt.Fprintf(os.Stderr, "  %s\n", s)
			}
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "The migration's structural-cleanup invariant has been violated.")
			fmt.Fprintln(os.Stderr, "Your project's pre-migration backup at .edikt/backups/ is intact.")
			return fmt.Errorf("post-migration verification: %d residual sentinel(s) in user files", len(stragglers))
		}
	}

	// Schema v1.1 → v1.2 backfill (ADR-036 §3): for every *.edikt.yaml  // edikt-guard:allow
	// directive or prohibition that carries a verify: field but no
	// verify_kind:, default to "structural". Runs only on --apply so
	// dry-run remains read-only. The pass is idempotent and confined to
	// *.edikt.yaml files — *.md bodies are never touched (ADR-027 §1).  // edikt-guard:allow
	var v12N int
	if !dryRun && failed == 0 {
		dirs := resolveArtifactDirs(projectRoot)
		v12Paths := collectYAMLSidecarsForV12(dirs)
		var v12Err error
		v12N, v12Err = runV12MigrationPass(v12Paths)
		if v12Err != nil {
			fmt.Fprintf(os.Stderr, "warn: v1.2 migration: %v\n", v12Err)
		}
		// Per SPEC-009 sre F6 mitigation: emit the exact summary line so  // edikt-guard:allow
		// adopters know to baseline the cheat-rate benchmark.
		fmt.Fprintf(os.Stdout, "%d directives migrated as structural; run `bin/edikt gov benchmark cheat-rate --all` to baseline\n", v12N)
		// --dry-run-output: write a JSON summary for idempotency verification.
		if v12Output != "" {
			summary := map[string]interface{}{
				"schema_migration":    "v1.1->v1.2",
				"directives_migrated": v12N,
				"timestamp":           time.Now().UTC().Format(time.RFC3339),
			}
			data, _ := json.MarshalIndent(summary, "", "  ")
			if err := os.MkdirAll(filepath.Dir(v12Output), 0o755); err == nil {
				_ = os.WriteFile(v12Output, data, 0o644)
			}
		}
	}

	emitEvent(ediktRoot, "sidecar_migration_complete", map[string]any{
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

	// --strict / --report-json: build regression manifest and enforce exit codes.
	if strict || reportJSON != "" {
		manifest, err := buildStrictManifest(strictPairs)
		if err != nil {
			os.Exit(3)
		}
		if reportJSON != "" {
			if err := writeStrictManifest(reportJSON, manifest); err != nil {
				fmt.Fprintf(os.Stderr, "warn: could not write report: %v\n", err)
			}
		}
		if strict {
			strictExit(manifest)
		}
	}

	return nil
}

func modeOf(dry bool) string {
	if dry {
		return "dry-run"
	}
	return "apply"
}

// ─── v1.1 → v1.2 migration pass ──────────────────────────────────────────────

// collectYAMLSidecarsForV12 returns paths of every *.edikt.yaml file under
// the governance artifact directories. These are candidates for the schema
// v1.2 backfill: setting verify_kind: structural on legacy verify-bearing
// directives and prohibitions (per ADR-036 §3 + SPEC-009 sre F6 mitigation).  // edikt-guard:allow
func collectYAMLSidecarsForV12(dirs artifactDirs) []string {
	var out []string
	for _, root := range []string{dirs.decisions, dirs.invariants, dirs.guidelines} {
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if strings.HasSuffix(p, ".edikt.yaml") {
				out = append(out, p)
			}
			return nil
		})
	}
	sort.Strings(out)
	return out
}

// runV12MigrationPass walks yamlPaths and sets verify_kind: structural on
// every directive (and prohibition) that carries a verify: field but no
// verify_kind:. Returns the total count of fields set and the first write
// error encountered (remaining files are still processed on error).
//
// The pass is read-only over *.md parent files — only the co-located
// *.edikt.yaml sidecar is mutated (ADR-036 §3, ADR-027 §1).  // edikt-guard:allow
func runV12MigrationPass(yamlPaths []string) (int, error) {
	var n int
	var firstErr error
	for _, p := range yamlPaths {
		sc, err := sidecar.Load(p)
		if err != nil {
			// Skeleton sidecars (topic: needs-extraction) or unreadable
			// files are skipped; they have no directives to migrate.
			continue
		}
		modified := false
		for i := range sc.Directives {
			if sc.Directives[i].Verify != "" && sc.Directives[i].VerifyKind == "" {
				sc.Directives[i].VerifyKind = "structural"
				n++
				modified = true
			}
		}
		for i := range sc.Prohibitions {
			if sc.Prohibitions[i].Verify != "" && sc.Prohibitions[i].VerifyKind == "" {
				sc.Prohibitions[i].VerifyKind = "structural"
				n++
				modified = true
			}
		}
		if !modified {
			continue
		}
		// Use Marshal to bypass the load-time cache; Marshal would
		// return pre-modification bytes for sidecars loaded via Load.
		out, marshalErr := sidecar.Marshal(sc)
		if marshalErr != nil {
			if firstErr == nil {
				firstErr = marshalErr
			}
			continue
		}
		if writeErr := atomicWriteNoFollow(p, out, 0o644); writeErr != nil {
			if firstErr == nil {
				firstErr = writeErr
			}
		}
	}
	return n, firstErr
}
