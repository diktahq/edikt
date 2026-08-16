// Package sidecar loads, validates, and reasons about <artifact>.edikt.yaml
// sidecar files (sidecar architecture) and
// templates/schemas/gov-sidecar.v1.schema.json (v1; the unversioned name was
// renamed in v0.6.0 per Phase 5 of PLAN-sidecar-review-fixes #31).
package sidecar

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is the on-disk shape this package understands.
const SchemaVersion = 1

// SchemaVersionV2 is the multi-anchor shape (source_excerpts[]). Both are
// accepted during the migration window: a corpus is converted by
// `edikt migrate to-v2` in one pass, but a project mid-upgrade — or a fixture
// tree deliberately pinning the v1 shape — must still load. Anchors() is what
// makes accepting both safe: every consumer reads one normalised list, so the
// two versions cannot be judged by different code paths.
const SchemaVersionV2 = 2

var (
	topicRe  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	signalRe = regexp.MustCompile(`^[a-z0-9][a-z0-9 _.-]*$`)
)

// validScopePhases is the closed enum for the scope field (v1.1).
// Any other value fails Validate.
var validScopePhases = map[string]bool{
	"planning":       true,
	"design":         true,
	"implementation": true,
	"review":         true,
}

// Sidecar mirrors the v1 schema one-to-one. Unknown fields anywhere in the
// document are rejected by the strict decoder, so the forbidden top-level
// keys (source_hash, agent_prompt_version, directives_hash) raise a parse
// error rather than being silently dropped.
type Sidecar struct {
	SchemaVersion int    `yaml:"schema_version"`
	Topic         string `yaml:"topic"`
	Path          string `yaml:"path"`

	// BodyDigest is the hex SHA-256 of the parent .md body after whitespace
	// normalization, recorded at extraction time (ADR-053). It powers the
	// BODY DRIFT signal — see bodydrift.go for the full contract.
	//
	// This is NOT the source_hash ADR-027 forbids, and the distinction is the
	// whole reason the field is permitted: ADR-027 banned persisted hashes
	// because divergence between recorded and computed values was the DEFECT
	// (they were fast-path substitutes for checking, and a stale record
	// silently suppressed real work). Here divergence is the PRODUCT — the
	// field exists precisely to be compared and to disagree. Different
	// contract, different name, so nobody reads it as the banned one
	// returning. source_hash, agent_prompt_version and directives_hash stay
	// forbidden by KnownFields(true).
	//
	// Optional by design: every sidecar written before ADR-053 has no
	// baseline, and each MUST be reported as UNMEASURED rather than folded
	// into the unchanged count (INV-013).
	BodyDigest string `yaml:"body_digest,omitempty"`

	Signals    []string    `yaml:"signals"`
	Directives []Directive `yaml:"directives"`

	// v1.1 targeting fields — optional, additive. KnownFields(true) means these fields
	// MUST be present in the struct before any sidecar that uses them can
	// round-trip through Load. Older readers (without these struct fields)
	// will hard-fail on decode — forward-only, not forward-compatible.
	// Paths is a list of glob patterns scoping where directives apply.
	Paths []string `yaml:"paths,omitempty"`
	// Scope is a list of lifecycle phases from the closed enum
	// {planning, design, implementation, review}.
	Scope []string `yaml:"scope,omitempty"`
	// Prohibitions are MUST NOT directives synthesised from rejected
	// ## Considered Options by the sidecar-extractor (Rule C).
	Prohibitions []Prohibition `yaml:"prohibitions,omitempty"`

	// User-authored overrides preserved across sidecar regenerations.
	// ManualDirectives are always included in the effective rule set.
	// SuppressedDirectives are subtracted from Directives at gov:compile time.
	// Populated by migrate_sidecars from the legacy sentinel block on upgrade.
	ManualDirectives     []string `yaml:"manual_directives,omitempty"`
	SuppressedDirectives []string `yaml:"suppressed_directives,omitempty"`

	// Aggregated at gov:compile time into governance.md's ## Reminders and
	// ## Verification Checklist sections. Populated by sidecar-extractor from
	// ## Confirmation (ADRs) and ## Enforcement (INVs) sections.
	Reminders    []string            `yaml:"reminders,omitempty"`
	Verification []VerificationEntry `yaml:"verification,omitempty"`

	// MigrationPreserved is a transient field populated by `edikt migrate
	// sidecars --apply` (two-phase upgrade: structural strip + LLM
	// extraction). It carries the verbatim content of the legacy
	// [edikt:directives:start] sentinel block so the sidecar-extractor
	// agent can use it as a canonical baseline on the first compile after
	// migration. Phase B of compile (the merge step) MUST strip this
	// field from the sidecar after the extractor's output supersedes it.
	// Steady-state sidecars never carry this field — its presence means
	// "just migrated, awaiting first canonical extraction" and IsStale
	// returns true unconditionally so Phase A dispatches the extractor.
	MigrationPreserved *MigrationPreserved `yaml:"migration_preserved,omitempty"`

	// ProposedPaths is a transient v2 field carrying extraction-time path
	// scope INFERENCES that no human has approved yet. It is the same shape
	// of mechanism as MigrationPreserved: written by one phase, consumed by
	// another, stripped before the sidecar reaches steady state.
	//
	// It exists as a separate field from Paths precisely so an inferred guess
	// can never silently become an enforced scope. Nothing reads
	// ProposedPaths for scoping — not merge, not verify-diff, not the render.
	// A proposal becomes authoritative only by moving into Paths, and the only
	// thing that moves it is `bin/edikt sidecar approve --kind paths`.
	//
	// The extractor emits it INTO THE SIDECAR rather than writing a second
	// file because the extractor is Write-only to its one target (INV-010);  edikt-guard:allow
	// tier-2 routes it out to .edikt/state/pending-paths/<id>.yaml.
	ProposedPaths []ProposedPath `yaml:"proposed_paths,omitempty"`

	// ProposedRemovals is a reviewed, human-initiated narrowing proposal —
	// see ProposedRemoval (F-033). Unlike ProposedPaths (extractor-
	// generated), nothing infers a glob is over-broad; a removal proposal is
	// always hand-authored by whoever is narrowing the scope. Same lifecycle
	// as ProposedPaths: transient, routed to a pending file, and it does not
	// remove anything from Paths until `bin/edikt sidecar approve --kind
	// paths` promotes it.
	ProposedRemovals []ProposedRemoval `yaml:"proposed_removals,omitempty"`

	// PathsApproval is the receipt for a paths approval (F-004).
	//
	// Without it, a sidecar whose paths[] went through the ceremony is
	// BYTE-IDENTICAL to one where an extractor wrote paths[] directly: the
	// approval appends the globs, clears ProposedPaths, writes, and deletes
	// the pending file, recording nothing about itself. No gate could tell an
	// approved scope from a declared one, and AC-4.4's "approved fields
	// survive regeneration byte-intact" was asserting a property the data
	// could not express.
	//
	// The hash is over the globs, not just a timestamp: a timestamp-only
	// receipt still validates after someone hand-edits paths[] afterwards,
	// which is the case it exists to catch.
	PathsApproval *PathsApproval `yaml:"paths_approval,omitempty"`

	// ProposedTopicDescription is the extractor's suggested one-line registry
	// description for this sidecar's topic — the "extracted" half of the
	// extracted-then-approved ceremony. Transient, same lifecycle as
	// ProposedPaths: routed out to a pending proposal and stripped.
	//
	// It is NEVER a description. Nothing reads it as one, no surface renders
	// it, and it cannot reach .edikt/topics.yaml except through a human
	// approval that stamps a hash over the exact bytes approved. A proposal
	// that could render is an invention with extra steps.
	ProposedTopicDescription string `yaml:"proposed_topic_description,omitempty"`

	SourcePath string `yaml:"-"`
}

// PathsApproval records that paths[] was human-approved through the ceremony.
type PathsApproval struct {
	ApprovedAt  string `yaml:"approved_at"`
	GlobsSHA256 string `yaml:"globs_sha256"`
}

// HashGlobs is the receipt's content address: SHA-256 over the sorted globs,
// newline-joined. Sorted so a reordering that changes nothing does not read as
// tampering, and newline-joined so two globs cannot be concatenated into one
// string colliding with a different pair.
func HashGlobs(globs []string) string {
	g := append([]string(nil), globs...)
	sort.Strings(g)
	sum := sha256.Sum256([]byte(strings.Join(g, "\n")))
	return hex.EncodeToString(sum[:])
}

// ProposedPath is one unapproved path-scope inference.
//
// MatchedExample is human-review context only. The mechanical requirement that
// a proposal match at least one real file is re-checked against the live tree
// by internal/pathsproposal at approval time — a proposal must not be allowed
// to certify itself (GL-002).
type ProposedPath struct {
	Glob           string `yaml:"glob"`
	Evidence       string `yaml:"evidence"`
	MatchedExample string `yaml:"matched_example,omitempty"`
}

// ProposedRemoval is one reviewed proposal to narrow paths[] by removing a
// glob (F-033). It carries no MatchedExample — the point of removal is
// that the glob is over-broad relative to the directive text, not that it
// matches nothing; the evidence a reviewer checks is the coverage preview
// computed live at approval time, not anything the proposal certifies about
// itself (GL-002).
type ProposedRemoval struct {
	Glob     string `yaml:"glob"`
	Evidence string `yaml:"evidence"`
}

// Directive is one rule extracted from the parent .md.
type Directive struct {
	Text          string        `yaml:"text"`
	SourceExcerpt SourceExcerpt `yaml:"source_excerpt,omitempty"`

	// SourceExcerpts is the v2 multi-anchor form: 1..N spans of parent prose
	// that together ground this directive. v1 could record only ONE span, so a
	// rule whose meaning is established across separate sentences had nowhere
	// to put the second — extraction had to drop context or split the rule.
	//
	// NEVER read this field or SourceExcerpt directly for logic. Both are
	// decode targets; Anchors() is the single read path. Two fields that must
	// agree are unified by one accessor, or they drift (GL-002).
	SourceExcerpts []SourceExcerpt `yaml:"source_excerpts,omitempty"`
	// NeedsReview marks a directive the extractor could not adjudicate from
	// inside a single artifact: an unattributed cross-artifact restatement,
	// or an aspirational obligation sitting in a Consequences section.
	//
	// It is a REVIEW FLAG, never a suppression. The directive compiles and
	// binds exactly as any other; the flag says a human still owes it a
	// decision. The two alternatives it exists to avoid are both silent —
	// extracting the entry unmarked, or dropping it unmentioned (INV-013).
	NeedsReview bool `yaml:"needs_review,omitempty"`

	// ActorScope marks a directive whose subject is an edikt-internal
	// automated operation (the compilation pipeline, gov:compile, migrate,
	// upgrade) rather than file content a human edit could violate (ADR-065).
	// hook match's write-time MUST-grade selection excludes ActorScope: true
	// entries unconditionally — that channel only observes Claude-Code-
	// mediated Edit/Write calls, which the Go-internal pipeline never makes,
	// so delivering these as PreToolUse bounces has no enforcement value
	// through that specific surface. Excluded entries still compile and
	// still render in ambient/topic-file surfaces. A reviewed, deliberate
	// per-directive opt-in — never inferred by the extractor.
	ActorScope bool `yaml:"actor_scope,omitempty"`

	// Verify is an optional shell command that returns exit 0 when the
	// directive holds. Run by `bin/edikt verify gov <ID>`. Schema:
	// templates/schemas/gov-sidecar.v1.schema.json (directives[].verify).
	Verify string `yaml:"verify,omitempty"`

	// v1.2 (additive) — SPEC-009 falsifiable-verification fields per ADR-036.  // edikt-guard:allow
	// All four are optional at JSON-schema level. Phase B compile enforces
	// SOME of ADR-036 §2's conditional-required constraints — verify_kind
	// when verify is set, and falsifying_observation when verify_kind is
	// behavioral — but NOT human_approved_at; see its field comment below
	// for the deferral.

	// VerifyKind classifies the enforcement medium of the verify command.
	// Enum: "behavioral" (runs code, asserts property) | "tooling" (stable
	// external tool with fixed rule ID) | "structural" (grep/file-presence —
	// only valid when the property is itself structural). The Mechanism
	// Quality axis per SPEC-009 Verification Layer Taxonomy.  // edikt-guard:allow
	VerifyKind string `yaml:"verify_kind,omitempty"`

	// Intent is a one-line semantic claim of the directive, generator-
	// neutral. Consumed by the L2 governance-verifier (ADR-037 intent-mode)  // edikt-guard:allow
	// instead of the raw Text. Max 300 chars (enforced by schema).
	Intent string `yaml:"intent,omitempty"`

	// FalsifyingObservation describes what a violation looks like. Used by
	// L2 to decide PASS/FAIL/NEEDS_REVIEW. Required (non-empty) when
	// VerifyKind == "behavioral"; Phase B compile enforces this. Max 300
	// chars.
	FalsifyingObservation string `yaml:"falsifying_observation,omitempty"`

	// HumanApprovedAt is the RFC 3339 UTC timestamp of human approval.
	// Set by `bin/edikt sidecar approve` (ADR-039).  // edikt-guard:allow
	//
	// Required by CONTRACT for VerifyKind == "behavioral" (ADR-036 §2), but
	// NOT enforced at compile: the check is deliberately deferred to
	// SPEC-009 Plan B (AC-4.3), and validatePhaseBConstraints implements
	// §2's other two constraints while skipping this one.
	// TestPhaseBConstraint_HumanApprovedAtDeferredToPlanB pins the
	// deferral, so an unapproved behavioral verify compiles today.
	//
	// This comment used to read "Required (non-empty) ... to compile",
	// which described a check that has never existed. A claim no check
	// backs is the defect; when the check is deliberately deferred, saying
	// so is the correction. When Plan B lands, enforce it in
	// validatePhaseBConstraints and retire both the deferral test and this
	// paragraph.
	HumanApprovedAt string `yaml:"human_approved_at,omitempty"`

	// PositiveFixturePath is the path to a script that exits 0 when the
	// directive's behavioral property holds. Required for verify_kind: behavioral.
	PositiveFixturePath string `yaml:"positive_fixture_path,omitempty"`

	// NegativeFixturePath is the path to a script that exits non-zero when the
	// directive's behavioral property is violated. Required for verify_kind: behavioral.
	NegativeFixturePath string `yaml:"negative_fixture_path,omitempty"`
}

// VerificationEntry is one row of the gov-sidecar verification[] array.
// The schema's oneOf accepts either a bare string (legacy form) or
// {text, verify?} (preferred for new entries). YAML round-trips through
// the polymorphic UnmarshalYAML/MarshalYAML pair below: bare-string
// entries decode with Verify="" and re-marshal as bare strings, so
// existing sidecars on disk stay byte-equal after a load+save cycle.
type VerificationEntry struct {
	Text   string `yaml:"text"`
	Verify string `yaml:"verify,omitempty"`
}

// UnmarshalYAML accepts either a bare string scalar OR a mapping with
// {text, verify?}. Any other shape is a parse error.
func (v *VerificationEntry) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		v.Text = node.Value
		return nil
	case yaml.MappingNode:
		// Avoid infinite recursion: decode into an anonymous shim type.
		shim := struct {
			Text   string `yaml:"text"`
			Verify string `yaml:"verify,omitempty"`
		}{}
		if err := node.Decode(&shim); err != nil {
			return err
		}
		v.Text = shim.Text
		v.Verify = shim.Verify
		return nil
	default:
		return fmt.Errorf("verification entry: want scalar or mapping, got kind %d", node.Kind)
	}
}

// MarshalYAML emits the legacy bare-string form when Verify is empty so
// existing sidecars on disk stay byte-stable; emits the object form when
// Verify is set.
func (v VerificationEntry) MarshalYAML() (interface{}, error) {
	if v.Verify == "" {
		return v.Text, nil
	}
	return struct {
		Text   string `yaml:"text"`
		Verify string `yaml:"verify,omitempty"`
	}{Text: v.Text, Verify: v.Verify}, nil
}

// MigrationPreserved carries the verbatim content of the legacy
// [edikt:directives:start] sentinel block from a pre-v0.6 artifact.
// Populated by `edikt migrate sidecars --apply` (Phase A: structural
// strip). Consumed by the sidecar-extractor agent on the first compile
// (Phase B), which uses the preserved lists as the canonical baseline
// and synthesises any missing canonical fields (topic, signals,
// source_excerpt) from prose. Phase B then strips this field from the
// written sidecar — steady-state sidecars never carry it.
type MigrationPreserved struct {
	// SchemaDetected is the migrator's classification of the legacy
	// block ("v0.5x-full" | "v0.5x-partial" | "v0.4.3-legacy" |
	// "unknown"). Audit-only — the extractor uses the preserved
	// content regardless of detection.
	SchemaDetected string `yaml:"schema_detected,omitempty"`

	// Verbatim lists from the legacy sentinel. The extractor MUST
	// output each entry in the corresponding canonical field of the
	// new sidecar; it MAY synthesize additional entries from prose
	// that aren't already covered, but it MUST NOT drop or rephrase
	// preserved entries.
	Directives           []string `yaml:"directives,omitempty"`
	ManualDirectives     []string `yaml:"manual_directives,omitempty"`
	SuppressedDirectives []string `yaml:"suppressed_directives,omitempty"`
	Reminders            []string `yaml:"reminders,omitempty"`
	Verification         []string `yaml:"verification,omitempty"`

	// Topic/Signals are hints if the legacy sentinel was a v0.5.x-full
	// shape. Extractor MAY use these when synthesising canonical
	// topic + signals; otherwise extracts from prose.
	Topic   string   `yaml:"topic,omitempty"`
	Signals []string `yaml:"signals,omitempty"`
}

// Anchors returns the effective anchor list for this directive, normalising v1
// and v2 to one shape.
//
// This is the ONLY place the two decode fields are reconciled. Every consumer —
// drift, grounding, doctor, render — iterates this, so a v1 sidecar and a v2
// sidecar cannot be judged by different code paths. An empty result means the
// directive is ungrounded, which callers must treat as unmeasured rather than
// as "no drift found" (INV-013).
func (d Directive) Anchors() []SourceExcerpt {
	if len(d.SourceExcerpts) > 0 {
		return d.SourceExcerpts
	}
	if d.SourceExcerpt != (SourceExcerpt{}) {
		return []SourceExcerpt{d.SourceExcerpt}
	}
	return nil
}

// Anchors returns the effective anchor list for this prohibition. See
// Directive.Anchors.
func (p Prohibition) Anchors() []SourceExcerpt {
	if len(p.SourceExcerpts) > 0 {
		return p.SourceExcerpts
	}
	if p.SourceExcerpt != (SourceExcerpt{}) {
		return []SourceExcerpt{p.SourceExcerpt}
	}
	return nil
}

// SourceExcerpt records the line range + verbatim quote in the parent.
type SourceExcerpt struct {
	LineStart int    `yaml:"line_start"`
	LineEnd   int    `yaml:"line_end"`
	Quote     string `yaml:"quote"`
	// Role is v2 advisory metadata describing what this anchor contributes
	// (statement | definition | scope | rationale | exception). Drift and
	// grounding treat every anchor alike regardless of role — role exists for
	// human review and render ordering, never to weight an anchor's authority.
	Role string `yaml:"role,omitempty"`
}

// Prohibition is a MUST NOT directive derived from a rejected ## Considered
// Option (Rule C in the sidecar-extractor prompt). The DerivedFrom field
// carries a stable label for the rejected option so doctor and migration
// tooling can identify the source (e.g. "rejected_option_a").
type Prohibition struct {
	Text          string        `yaml:"text"`
	SourceExcerpt SourceExcerpt `yaml:"source_excerpt,omitempty"`

	// SourceExcerpts is the v2 multi-anchor form. See Directive.SourceExcerpts:
	// read through Anchors(), never directly.
	SourceExcerpts []SourceExcerpt `yaml:"source_excerpts,omitempty"`

	// NeedsReview — see Directive.NeedsReview. Same contract: a review flag
	// on an entry that still compiles and still binds.
	NeedsReview bool `yaml:"needs_review,omitempty"`

	// ActorScope — see Directive.ActorScope (ADR-065). Same contract for
	// prohibitions.
	ActorScope bool `yaml:"actor_scope,omitempty"`

	DerivedFrom string `yaml:"derived_from,omitempty"`
	// Verify is an optional shell command that returns exit 0 when the
	// prohibition is honored, non-zero when it is violated. Run by
	// `bin/edikt verify gov <ID>`. Schema:
	// templates/schemas/gov-sidecar.v1.schema.json (prohibitions[].verify).
	Verify string `yaml:"verify,omitempty"`

	// v1.2 (additive) — SPEC-009 falsifiable-verification fields per ADR-036.  // edikt-guard:allow
	// Symmetric with Directive's fields above, including the enforcement
	// asymmetry: Phase B checks verify_kind and falsifying_observation on
	// prohibitions too, and likewise does NOT check human_approved_at —
	// deferred to SPEC-009 Plan B (AC-4.3). See Directive.HumanApprovedAt.
	VerifyKind            string `yaml:"verify_kind,omitempty"`
	Intent                string `yaml:"intent,omitempty"`
	FalsifyingObservation string `yaml:"falsifying_observation,omitempty"`
	HumanApprovedAt       string `yaml:"human_approved_at,omitempty"`
	PositiveFixturePath   string `yaml:"positive_fixture_path,omitempty"`
	NegativeFixturePath   string `yaml:"negative_fixture_path,omitempty"`
}

// Load reads sidecarPath, strictly decodes it, and runs the v1 validators.
//
// The reader is buffered (Phase 7 of PLAN-sidecar-review-fixes #40); this
// is observable only as a small reduction in syscall count when Discover
// loads dozens of sidecars in sequence — yaml.NewDecoder's read pattern is
// otherwise reasonable on a raw *os.File but still benefits from the
// 4 KiB bufio default on small files.
func Load(sidecarPath string) (*Sidecar, error) {
	// Read once. The previous form streamed through a bufio decoder to avoid
	// the 4 KiB default on small files; the schema check below needs the whole
	// document anyway, so one ReadFile serves both and reads the file once
	// rather than twice.
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", sidecarPath, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)

	var s Sidecar
	if err := dec.Decode(&s); err != nil {
		// A sidecar is written by an LLM subagent, and a subagent that is
		// resumed mid-write can flush its own tool-call framing into the
		// file — the observed shape is a trailing `</content>` /
		// `</invoke>` after the last key. yaml.v3 reports that as an
		// opaque scan error at some line, which sends the user hunting
		// through their prose. Name the actual failure instead.
		if marker, line := findToolCallMarkup(sidecarPath); marker != "" {
			return nil, fmt.Errorf(
				"parse %s: line %d contains tool-call markup (%q), not YAML — "+
					"the extractor subagent leaked its own framing into the file. "+
					"Delete the stray line(s) or regenerate with /edikt:<artifact>:compile",
				sidecarPath, line, marker)
		}
		return nil, fmt.Errorf("parse %s: %w", sidecarPath, err)
	}
	// D22/D45 — the authoritative schema is deliberately NOT called here.
	// `ValidateRawAgainstSchema` exists and works, but wiring it into Load
	// overturns a recorded decision: v12_test.go:128 asserts "expected nil
	// error (rejection is at schema or compile layer, not Go-loader)". The
	// loader is deliberately permissive; rejection belongs at a gate, and the
	// test pinning that stays.
	//
	// D45's ruled call sites are both gates, both calling the same
	// ValidateRawAgainstSchema against the same mirrored bytes:
	//   (a) phasea/runner.go — the generation boundary, extending D20's
	//       parse-and-rollback gate.
	//   (b) `edikt gov schema-check` — the corpus-wide check, wired into
	//       test/test-schema-copies.sh.
	//
	// Validate() below therefore REMAINS a second, hand-written statement of
	// validity. D22 is narrowed, not closed: the authoritative schema now runs
	// where output is generated, but the two definitions still coexist and can
	// still drift. Recorded rather than papered over — see AC-1.2.
	s.SourcePath = sidecarPath
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", sidecarPath, err)
	}
	return &s, nil
}

// toolCallTags are the XML-ish element names an LLM subagent can leak from
// its own tool-call framing into the file it is writing. Built into open and
// close forms at init rather than written literally, so this list stays
// readable and no stray tag sits in the source.
var toolCallTags = []string{
	"content", "invoke", "function_calls", "parameter",
	"antml:invoke", "antml:parameter", "antml:function_calls",
}

// toolCallMarkers holds the open and close forms of every toolCallTags
// entry. A line matches only when a marker is its entire trimmed content,
// so a directive that quotes one of these strings inline (e.g. a rule about
// the framing itself) is never flagged.
var toolCallMarkers = func() []string {
	m := make([]string, 0, len(toolCallTags)*2)
	for _, t := range toolCallTags {
		m = append(m, "<"+t+">", "</"+t+">")
	}
	return m
}()

// findToolCallMarkup re-reads the file and returns the first line whose
// entire content is a tool-call framing marker, with its 1-indexed line
// number. Returns ("", 0) when the file has no such line — including when
// it cannot be re-read, in which case the caller falls back to the raw
// YAML error.
//
// This runs only on the decode-failure path, so the happy path pays
// nothing for it.
func findToolCallMarkup(path string) (string, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0
	}
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		for _, marker := range toolCallMarkers {
			if trimmed == marker {
				return marker, i + 1
			}
		}
	}
	return "", 0
}

// Validate enforces schema constraints not captured by structural decode.
func (s *Sidecar) Validate() error {
	if s.SchemaVersion != SchemaVersion && s.SchemaVersion != SchemaVersionV2 {
		return fmt.Errorf("schema_version: got %d, want %d or %d", s.SchemaVersion, SchemaVersion, SchemaVersionV2)
	}
	// A v2 document must actually use the v2 shape. Accepting a v2 stamp on a
	// document whose directives still carry the singular key would let a
	// half-migrated sidecar pass as migrated — the failure mode the version
	// bump exists to make visible.
	if s.SchemaVersion == SchemaVersionV2 {
		for i, d := range s.Directives {
			if len(d.SourceExcerpts) == 0 && d.SourceExcerpt != (SourceExcerpt{}) {
				return fmt.Errorf("directives[%d]: schema_version 2 but still carries singular source_excerpt; run `edikt migrate to-v2`", i)
			}
		}
		for i, pr := range s.Prohibitions {
			if len(pr.SourceExcerpts) == 0 && pr.SourceExcerpt != (SourceExcerpt{}) {
				return fmt.Errorf("prohibitions[%d]: schema_version 2 but still carries singular source_excerpt; run `edikt migrate to-v2`", i)
			}
		}
	}
	if s.Topic == "" {
		return fmt.Errorf("topic: required")
	}
	if !topicRe.MatchString(s.Topic) {
		return fmt.Errorf("topic %q: must match %s", s.Topic, topicRe.String())
	}
	if s.Path == "" {
		return fmt.Errorf("path: required")
	}
	seen := make(map[string]bool, len(s.Signals))
	for _, sig := range s.Signals {
		if !signalRe.MatchString(sig) {
			return fmt.Errorf("signal %q: must match %s", sig, signalRe.String())
		}
		if seen[sig] {
			return fmt.Errorf("signals: duplicate %q (uniqueItems)", sig)
		}
		seen[sig] = true
	}
	for i, d := range s.Directives {
		if d.Text == "" {
			return fmt.Errorf("directives[%d].text: required", i)
		}
		if len(d.Text) > 500 {
			return fmt.Errorf("directives[%d].text: %d chars, max 500", i, len(d.Text))
		}
		// Validate EVERY anchor, not just the first. A v2 item whose second
		// anchor is malformed is as broken as one whose first is, and checking
		// only [0] would pass exactly the items multi-anchor exists to carry.
		anchors := d.Anchors()
		if len(anchors) == 0 {
			return fmt.Errorf("directives[%d]: no source anchor — an ungrounded item cannot be drift-checked", i)
		}
		for ai, se := range anchors {
			if se.LineStart < 1 {
				return fmt.Errorf("directives[%d].anchor[%d].line_start: %d, must be >= 1", i, ai, se.LineStart)
			}
			if se.LineEnd < se.LineStart {
				return fmt.Errorf("directives[%d].anchor[%d].line_end: %d < line_start %d", i, ai, se.LineEnd, se.LineStart)
			}
			if se.Quote == "" {
				return fmt.Errorf("directives[%d].anchor[%d].quote: required", i, ai)
			}
		}
	}
	// v1.1: scope must be from the closed enum when present.
	for i, phase := range s.Scope {
		if !validScopePhases[phase] {
			return fmt.Errorf("scope[%d]: %q is not a valid lifecycle phase (allowed: planning, design, implementation, review)", i, phase)
		}
	}
	// v1.1: paths entries must be non-empty strings.
	for i, p := range s.Paths {
		if p == "" {
			return fmt.Errorf("paths[%d]: empty string not allowed", i)
		}
	}
	// v1.1: prohibitions must satisfy the same source-excerpt invariants as
	// directives.
	for i, p := range s.Prohibitions {
		if p.Text == "" {
			return fmt.Errorf("prohibitions[%d].text: required", i)
		}
		if len(p.Text) > 500 {
			return fmt.Errorf("prohibitions[%d].text: %d chars, max 500", i, len(p.Text))
		}
		// Validate EVERY anchor, not just the first. A v2 item whose second
		// anchor is malformed is as broken as one whose first is, and checking
		// only [0] would pass exactly the items multi-anchor exists to carry.
		anchors := p.Anchors()
		if len(anchors) == 0 {
			return fmt.Errorf("prohibitions[%d]: no source anchor — an ungrounded item cannot be drift-checked", i)
		}
		for ai, se := range anchors {
			if se.LineStart < 1 {
				return fmt.Errorf("prohibitions[%d].anchor[%d].line_start: %d, must be >= 1", i, ai, se.LineStart)
			}
			if se.LineEnd < se.LineStart {
				return fmt.Errorf("prohibitions[%d].anchor[%d].line_end: %d < line_start %d", i, ai, se.LineEnd, se.LineStart)
			}
			if se.Quote == "" {
				return fmt.Errorf("prohibitions[%d].anchor[%d].quote: required", i, ai)
			}
		}
	}
	return nil
}
