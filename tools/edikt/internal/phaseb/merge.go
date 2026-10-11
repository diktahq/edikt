// Package phaseb implements the deterministic merge phase of two-phase
// gov:compile. It reads the validated sidecar set, groups
// directives by topic, and writes .claude/rules/governance/<topic>.md and
// .claude/rules/governance.md.
//
// PURITY CONTRACT. This package MUST NOT import any
// symbol that dispatches subagents, shells out, or makes a network call.
// Forbidden imports: os/exec, net/http, anything under tools/edikt/internal
// that wraps claude. Static check at tools/edikt/check/no-llm-in-merge.sh
// enforces this; the build will fail if a forbidden symbol creeps in.
package phaseb

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/diktahq/edikt/tools/edikt/internal/compile"
	"github.com/diktahq/edikt/tools/edikt/internal/render"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"gopkg.in/yaml.v3"
)

// Result describes what Phase B did.
type Result struct {
	TopicsRendered  []string `json:"topics_rendered"`
	TopicsUnchanged []string `json:"topics_unchanged"`
	IndexWritten    bool     `json:"index_written"`
	TotalDirectives int      `json:"total_directives"`

	// Scope accounting. TopicsScoped + TopicsUnscoped covers every topic
	// Phase B considered, cached or not, so the reported fraction tracks
	// the corpus rather than the cache. UndeclaredSources names the
	// sidecars holding the unscoped topics open — that list is the
	// clearing condition, not a diagnostic.
	TopicsScoped      []string `json:"topics_scoped"`
	TopicsUnscoped    []string `json:"topics_unscoped"`
	UndeclaredSources []string `json:"undeclared_sources"`

	// Surfaces enumerates every file this render wrote or left in place,
	// with its repo-relative path — the render's own answer to "what did you
	// produce", published as data.
	//
	// It exists because consumers otherwise have to ASSUME the directory
	// layout, and a consumer that assumes the layout silently stops covering
	// a surface the moment one moves. A guard must assert the resolver's
	// published outcome rather than reproduce the logic that produces it
	// (INV-014); this field is that outcome.  edikt-guard:allow
	//
	// It carries no hashes. The per-file hashes and the on-disk manifest are
	// SPEC-011 stage 1 work, and they cannot land before the `compiled_at`  edikt-guard:allow
	// determinism question is settled — recording a hash over bytes that
	// carry a live timestamp would make every recorded hash churn and turn
	// the manifest into a false tamper signal. Enumeration is useful on its
	// own and is not blocked on that.
	//
	// Every rendered topic appears, whether it was re-rendered this run or
	// served from the fingerprint cache. A surface missing because it was
	// unchanged would make "unchanged" and "not produced" indistinguishable.
	Surfaces []Surface `json:"surfaces"`

	// DeliveryMapWritten reports whether delivery-map.yaml changed this run.
	DeliveryMapWritten bool
	// JudgementRoutedDirectives counts directives reachable ONLY through a
	// skill package — no path match delivers them. Reported so the number is a
	// declared state rather than an accident nobody measured.
	JudgementRoutedDirectives int `json:"directive_index_written"`

	// SkillsRendered names the topics that got a surface (d) package.
	SkillsRendered []string `json:"skills_rendered"`

	// TopicsRetiredToSkill names topics whose tier-2 rules file was REMOVED
	// because the topic is unscoped and now lives at tier 3. Reported so the
	// retirement is visible rather than inferred from a file's absence.
	//
	// This is a SNAPSHOT, not a transition: it names every topic unscoped on
	// THIS run, whether or not it was scoped a moment ago — deliberately, so
	// a fresh clone (nothing on disk beforehand) still reports which topics
	// are skill-only. That is the wrong list to alarm on. A topic already
	// living at tier 3 losing nothing is routine; a topic that HAD a tier-2
	// file an instant ago and does not after this run is a reachability
	// regression this compile itself just caused — see
	// TopicsNewlyUnreachable, which answers that different question.
	TopicsRetiredToSkill []string `json:"topics_retired_to_skill"`

	// TopicsNewlyUnreachable names topics whose tier-2 rules file EXISTED ON
	// DISK immediately before this compile run and does not after it — a
	// true before/after regression, not a point-in-time snapshot. Distinct
	// from TopicsRetiredToSkill (every currently-unscoped topic, regardless
	// of prior state) and from doctor's own topic-scope check (a standing,
	// separately-invoked signal with no notion of "did THIS run cause it").
	//
	// F-115/A3: a topic reassignment (an artifact's contributing sidecar
	// moving from one topic to another) that leaves the DESTINATION topic
	// with no declared paths:, or leaves the SOURCE topic's remaining
	// contributors all undeclared, can silently drop write-time delivery
	// for a project's core domain — found live, with nothing warning at the
	// moment compile made the change. This field is what a caller checks to
	// report that loudly, attributed to the topic, at the moment it happens
	// — not on some later, separately-invoked doctor run.
	TopicsNewlyUnreachable []string `json:"topics_newly_unreachable,omitempty"`

	// OrphansRemoved names surfaces deleted because the topic that owned them
	// no longer exists. Reported, never silent: a file disappearing from a
	// governance tree with nothing naming it is indistinguishable from a
	// compile that lost it.
	OrphansRemoved []string `json:"orphans_removed"`

	// ManifestWritten is false on a no-op compile, which is the point — the
	// manifest carries no timestamp, so byte-equal input leaves it untouched.
	ManifestWritten bool `json:"manifest_written"`
}

// Surface is one file the render is responsible for.
type Surface struct {
	// Kind is the surface class: "ambient-core" or "topic-file" today.
	// Stage 1 adds "skill-package"; the "directive-index" surface it also added is retired (ADR-083).  edikt-guard:allow
	Kind string `json:"kind"`
	// Path is repo-relative, slash-separated.
	Path string `json:"path"`
	// Topic is set for topic-scoped surfaces and empty for singletons.
	Topic string `json:"topic,omitempty"`
}

// Options configures merge output.
type Options struct {
	OutDir    string // default .claude/rules/governance
	IndexPath string // default .claude/rules/governance.md
	// ContractBlock is a verbatim block the caller read from a SOURCE
	// artifact, passed in rather than resolved here.
	//
	// It has to be passed: the freshness check renders into a scratch root to
	// avoid writing over the tree it is comparing, and anything this function
	// resolves relative to its render root resolves to nothing there. The
	// shadow render then differs from the real one for a reason that has
	// nothing to do with the corpus — which is exactly the false drift signal
	// the freshness check exists to avoid producing.
	ContractBlock   string
	CompiledAt      string // ISO 8601; pinned for deterministic test runs
	CompilerVersion string

	// Excluded is the per-kind count of retired artifacts (superseded,
	// deprecated, or migration:skip) that the caller filtered out before
	// calling Merge.
	//
	// Phase B cannot derive this. By the time `pairs` arrives the retired
	// entries are already gone, and that is deliberate: they are withheld
	// so groupByTopic can never compile a retired ADR's directives as
	// duplicates of its replacement. Handing Merge the unfiltered set to
	// let it count for itself would put that regression one forgotten
	// filter away. A count carries the information without the hazard.
	//
	// nil means the caller did not measure; the index reports that rather
	// than printing a zero it cannot stand behind.
	Excluded map[string]int

	// TopicDescriptions maps a topic id to its APPROVED registry description
	// (.edikt/topics.yaml), to be rendered VERBATIM wherever the topic
	// appears. Phase B never rewrites, summarises, truncates, or synthesises
	// a description: a description the render produced is a description
	// nobody pinned, which is the exact failure the registry exists to
	// prevent (SSP-002).
	//
	// A topic absent from this map has no approved description YET. The
	// render omits the line and the caller reports the pending count with a
	// denominator — it does NOT invent a placeholder, and it does NOT refuse
	// to compile. Incompleteness that is visibly reported is not one of the
	// two failure modes the registry guards against (invention and silence).
	//
	// Phase B takes a plain map rather than the registry type so the merge
	// stays a pure function of its inputs: reading the registry file, and
	// deciding what to do about pending topics, belong to the caller.
	TopicDescriptions map[string]string
}

// topicGroup aggregates one topic's contributions across sidecars.
type topicGroup struct {
	Name       string
	Sidecars   []*sidecar.Sidecar
	Directives []compile.Rule
	// Phase 8: manual_directives and prohibitions are first-class regions.
	// Manual entries flow through with their owning ADR ID so the
	// interleaved sort by ref tag stays deterministic.
	Manual       []manualEntry
	Prohibitions []compile.Rule
	// Paths is the union of the code globs the contributing sidecars
	// declared in their `paths:` field. It is NOT the sidecars' own
	// document paths — building it from Sidecar.Path is the defect this
	// field's comment exists to prevent recurring.
	Paths []string
	// Scope is the union of the lifecycle phases the topic's contributing
	// sidecars declare in `scope:` (B3, SPEC-010 phase 8). Per the  edikt-guard:allow
	// sidecar-extractor's own contract, an empty per-sidecar `scope:` means
	// "no lifecycle filter applied" — NOT "unset" — so a sidecar that
	// declares nothing contributes no phases to this union rather than
	// contributing a default. An all-empty union means no lifecycle filter
	// renders.
	Scope []string
	// Undeclared holds the artifact IDs of contributing sidecars that
	// declared no globs at all. An absent `paths:` means NO RESTRICTION,
	// not NO PATHS, so a single undeclared member makes the whole topic's
	// union "everywhere" — see scopeFor.
	Undeclared []string
	Sources    []string
	Signals    []string
}

// unrestrictedGlob is how a topic file spells "applies everywhere".
//
// It is `**`, not the `**/*` the index template uses for governance.md,
// because the two are NOT equivalent under the matcher that actually reads
// this field. The tier-1 consumer (commands/gov/verify-diff.md step 4)
// matches with Python's fnmatch, where `**/*` compiles to a pattern
// requiring a slash — so `install.sh`, `README.md`, and every other
// root-level file fall out of scope. `**` matches everything under both
// fnmatch and the Go matcher in cmd/gov/verifydiff.go.
//
// Getting this wrong would silently reintroduce the under-scoping the
// fallback exists to prevent, just narrower: an unscoped topic would still
// miss every diff confined to the repo root.
const unrestrictedGlob = "**"

// scopeFor resolves a topic's compiled `paths:` list and the human-readable
// note explaining the result.
//
// The rule is set union, not a heuristic: a topic's scope is the union of
// its sidecars' scopes, and an undeclared sidecar contributes "everywhere".
// Union with everywhere is everywhere, so ONE undeclared member unscopes
// the topic.
//
// The declared globs are deliberately NOT emitted alongside **/* in that
// case. Listing them would make the union inert while the file still looked
// scoped — a reader sees a glob list and believes it restricts something.
//
// The asymmetry that settles the direction: under-scoping silently disables
// governance (rules stop loading, nothing reports it), while over-scoping
// costs tokens and noise. Only one of those is acceptable as a default for
// a tool whose product is enforcement.
func scopeFor(g *topicGroup) (paths []string, note string) {
	if len(g.Undeclared) == 0 && len(g.Paths) > 0 {
		return g.Paths, fmt.Sprintf("scope: %d glob(s) from all %d source(s)",
			len(g.Paths), len(g.Sources))
	}
	// Name WHICH sources failed to declare — that naming is the clearing
	// condition. Declare globs on them and the topic tightens on the next
	// compile with no further change here. The list is never truncated:
	// a hidden entry is an unfixable topic.
	return []string{unrestrictedGlob}, fmt.Sprintf(
		"scope: %s — UNSCOPED because %d of %d source(s) declare no paths: globs (%s). "+
			"Declaring globs on those tightens this file automatically.",
		unrestrictedGlob, len(g.Undeclared), len(g.Sources), strings.Join(g.Undeclared, ", "))
}

// directiveIndexName is the filename of surface (c) inside the governance
// output directory. Topic files sit in that same directory, so a bare
// filename is the correct relative reference from one to the other.

// noteEmitterFingerprint hashes everything about this package that decides the
// rendered bytes of a topic file WITHOUT being a template.
//
// render.RendererFingerprint covers the templates; this covers the prose the
// templates interpolate. Both are mixed into a topic's fingerprint, so the
// cache is keyed on the full set of things that can change the output: the
// sidecars, the approved description, the templates, and this.
//
// The wrap width is in the hash because changing it re-flows every note, which
// is a change to the bytes on disk even when not one word differs.
func noteEmitterFingerprint() string {
	h := sha256.New()
	h.Write([]byte("note-wrap-width\x00"))
	fmt.Fprintf(h, "%d", noteWrapWidth)

	// THE ROUTING TABLES, hashed over their whole input domain.
	//
	// Found by SR-003 (SPEC-018). Removing the ADR-prohibition promotion from
	// gradeFor changed which directives render into a topic body — and nothing
	// re-rendered. The index was rewritten (238 -> 126 entries) while all seven
	// topic files reported "unchanged", because their cache key hashes the
	// contributing SIDECARS and the sidecars had not moved. Only the renderer's
	// opinion of them had.
	//
	// That is exactly the defect this fingerprint exists to prevent, one level
	// up: it covered the note emitter and not the routing that decides
	// membership. `gov compile --check` caught the divergence honestly, which is
	// how it was found, but a compile that reports "0 rendered, 7 unchanged"
	// while its own --check calls those seven stale is a cache lying about a
	// decision it is entitled to skip.
	//
	// Both tables are pure functions over a small closed domain, so hashing
	// their outputs is exact rather than a proxy: any edit to either changes
	// this digest, and no edit elsewhere does.
	h.Write([]byte("\x00grade-table\x00"))
	for _, class := range []string{"invariant", "adr", "guideline", "unknown"} {
		for _, isProh := range []bool{false, true} {
			for _, enf := range []string{"", "deny", "advisory"} {
				fmt.Fprintf(h, "%s|%t|%s=%s;", class, isProh, enf, gradeFor(class, isProh, enf))
			}
		}
	}
	h.Write([]byte("\x00route-table\x00"))
	for _, grade := range []string{gradeMust, gradeAdvisory} {
		for _, actorScope := range []bool{false, true} {
			for _, tier3 := range []bool{false, true} {
				fmt.Fprintf(h, "%s|%t|%t=%s;", grade, actorScope, tier3, RouteOf(grade, actorScope, tier3))
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// noteWrapWidth is the column the generated prose notes wrap at, matching the
// hand-written wrapping already used in the skill-package bodies.
const noteWrapWidth = 78

// wrapText greedy-wraps text at noteWrapWidth, prefixing every line.
//
// Greedy and whitespace-only, so it is deterministic: the same input wraps to
// the same bytes on every host, which the render manifest's hash requires. A
// word longer than the width gets its own over-long line rather than being
// split — breaking a backticked path across lines would produce a note naming
// a file nobody can copy.
//
// Every generated note goes through here rather than being hand-wrapped at
// the fmt.Sprintf call site. Hand-wrapping only holds for the widths the
// author happened to test: a note interpolating "130 directives and 20
// prohibitions" is 11 columns wider than the same note reading "2 and 3", so
// a literal that looked wrapped in source rendered ragged on a real corpus.
func wrapText(text, prefix string) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	line := prefix + words[0]
	for _, w := range words[1:] {
		// Width is measured in RUNES, not bytes: these notes carry em dashes
		// and typographic quotes, and counting their UTF-8 bytes would wrap
		// the lines that contain them several columns early.
		if utf8.RuneCountInString(line)+1+utf8.RuneCountInString(w) > noteWrapWidth {
			b.WriteString(line)
			b.WriteByte('\n')
			line = prefix + w
			continue
		}
		line += " " + w
	}
	b.WriteString(line)
	return b.String()
}

// manualEntry pairs a manual_directive's text with the artifact ID of the
// sidecar that authored it, so render can sort by that ref tag for
// determinism even when the entry's text does not embed `(ref: …)`.
type manualEntry struct {
	Text   string
	Source string
}

// validatePhaseBConstraints enforces the compile-time v1.2 constraints from
// ADR-036 §2 over every non-nil sidecar's directives and prohibitions.  // edikt-guard:allow
// It runs before any topic rendering so a bad sidecar causes an early return.
func validatePhaseBConstraints(pairs []sidecar.Pair) error {
	for _, p := range pairs {
		if p.Sidecar == nil {
			continue
		}
		for i, d := range p.Sidecar.Directives {
			if d.Verify != "" && d.VerifyKind == "" {
				return fmt.Errorf("%s: directives[%d].verify set but verify_kind is empty — run `edikt migrate sidecars --apply` to default to structural", p.ArtifactID, i)
			}
			if d.VerifyKind == "behavioral" && d.FalsifyingObservation == "" {
				return fmt.Errorf("%s: directives[%d].verify_kind is behavioral but falsifying_observation is empty", p.ArtifactID, i)
			}
			// SPEC-009 Plan C Phase 4 — bidirectional fixture gate. Every  // edikt-guard:allow
			// behavioral directive MUST declare both a positive and a
			// negative fixture path. Files need not exist at compile time;
			// existence is a benchmark-time concern.
			if d.VerifyKind == "behavioral" && d.PositiveFixturePath == "" {
				return fmt.Errorf("%s: directives[%d] %q: verify_kind is behavioral but positive_fixture_path is empty — a behavioral verify requires a fixture PAIR (positive + negative); see https://edikt.dev/governance/behavioral-fixtures", p.ArtifactID, i, d.Text)
			}
			if d.VerifyKind == "behavioral" && d.NegativeFixturePath == "" {
				return fmt.Errorf("%s: directives[%d] %q: verify_kind is behavioral but negative_fixture_path is empty — a behavioral verify requires a fixture PAIR (positive + negative); the negative arm must exit 0 only when it DETECTS a violation; see https://edikt.dev/governance/behavioral-fixtures", p.ArtifactID, i, d.Text)
			}
		}
		for i, pr := range p.Sidecar.Prohibitions {
			if pr.Verify != "" && pr.VerifyKind == "" {
				return fmt.Errorf("%s: prohibitions[%d].verify set but verify_kind is empty — run `edikt migrate sidecars --apply` to default to structural", p.ArtifactID, i)
			}
			if pr.VerifyKind == "behavioral" && pr.FalsifyingObservation == "" {
				return fmt.Errorf("%s: prohibitions[%d].verify_kind is behavioral but falsifying_observation is empty", p.ArtifactID, i)
			}
			// SPEC-009 Plan C Phase 4 — bidirectional fixture gate (prohibition arm).  // edikt-guard:allow
			if pr.VerifyKind == "behavioral" && pr.PositiveFixturePath == "" {
				return fmt.Errorf("%s: prohibitions[%d] %q: verify_kind is behavioral but positive_fixture_path is empty — a behavioral verify requires a fixture PAIR (positive + negative); see https://edikt.dev/governance/behavioral-fixtures", p.ArtifactID, i, pr.Text)
			}
			if pr.VerifyKind == "behavioral" && pr.NegativeFixturePath == "" {
				return fmt.Errorf("%s: prohibitions[%d] %q: verify_kind is behavioral but negative_fixture_path is empty — a behavioral verify requires a fixture PAIR (positive + negative); the negative arm must exit 0 only when it DETECTS a violation; see https://edikt.dev/governance/behavioral-fixtures", p.ArtifactID, i, pr.Text)
			}
		}
	}
	return nil
}

// Merge runs the deterministic Phase B over a discovered sidecar set.
// Pairs without a Sidecar (missing on disk or LoadErr set) are skipped;
// the caller is expected to have already gated on Phase A success.
func Merge(projectRoot string, pairs []sidecar.Pair, opts Options) (*Result, error) {
	if opts.OutDir == "" {
		opts.OutDir = filepath.Join(projectRoot, ".claude", "rules", "governance")
	}
	if opts.IndexPath == "" {
		opts.IndexPath = filepath.Join(projectRoot, ".claude", "rules", "governance.md")
	}

	if err := validatePhaseBConstraints(pairs); err != nil {
		return nil, err
	}

	// Collect global aggregates in artifact-ID order for determinism.
	// INV directives go ONLY to governance.md constraints — never to topic
	// files. Reminders and verification are aggregated across all artifacts.
	sortedPairs := append([]sidecar.Pair(nil), pairs...)
	sort.Slice(sortedPairs, func(i, j int) bool {
		return sortedPairs[i].ArtifactID < sortedPairs[j].ArtifactID
	})
	var invariantRules []compile.Rule
	var allReminders, allVerification []string
	topicReminders := map[string][]string{}
	topicVerification := map[string][]string{}
	// Reminders belonging to artifacts that declared NO globs. The directive
	// index is keyed by glob and therefore has no key for them; without this
	// they reach no surface at all.
	pathlessReminders := map[string][]string{}
	for _, p := range sortedPairs {
		if p.Sidecar == nil {
			continue
		}
		// AMBIENT CONTENT RULE (SPEC-011 stage 1): the always-on core carries  edikt-guard:allow
		// ONE CANONICAL STATEMENT per PATHLESS invariant, and nothing else.
		//
		// Before this, every directive of every invariant went to the core —
		// 14 invariants expanded to ~150 lines, then were RESTATED verbatim
		// lower in the same file. An invariant that declares `paths:` is not
		// pathless: it is scoped like any other artifact and belongs in its
		// topic file, where the glob decides whether it loads. Sending it to
		// the core says "this applies to every edit" about a rule whose own
		// author said otherwise.
		//
		// The canonical statement is the FIRST surviving directive (spec
		// decision): sidecar order follows prose order, and an invariant's
		// Statement section precedes its Enforcement section, so the first
		// entry is the statement rather than one of its enforcement clauses.
		// SR-001 (SPEC-018): the `len(Paths) == 0` gate is REMOVED.
		//
		// It made the two tiers exclusive: adding a glob to reach the
		// write-time channel deleted the rule from the surface loaded on every
		// edit, in the same act. 18 of 21 invariants were absent from the
		// ambient core for exactly that reason, and the comment above reasons
		// only about WHICH directive to promote — the exclusion was a side
		// effect of a conditional written to answer a different question, not a
		// decision anyone recorded.
		//
		// A scoped invariant still reaches its topic file and the index through
		// the paths branch below, so this is additive: the tiers become layers
		// rather than alternatives.
		if strings.HasPrefix(p.ArtifactID, "INV-") {
			suppressed := make(map[string]struct{}, len(p.Sidecar.SuppressedDirectives))
			for _, sd := range p.Sidecar.SuppressedDirectives {
				suppressed[sd] = struct{}{}
			}
			for _, d := range p.Sidecar.Directives {
				if _, isSuppressed := suppressed[d.Text]; isSuppressed {
					continue
				}
				invariantRules = append(invariantRules, compile.Rule{Text: d.Text, Source: p.ArtifactID, Intent: d.Intent})
				break // canonical statement only — the rest live in the topic file
			}
		}
		// Reminders and verification are keyed BY TOPIC now, not flattened
		// into one always-on list. governance.md declares `paths: "**/*"`, so
		// the flat lists loaded on every edit regardless of subject — 335 of
		// governance.md's 543 lines (D37). Topic files carry a scoped
		// `paths:`, so the same content behind the right glob costs nothing
		// on an unrelated edit.
		//
		// INV-sourced entries keep going to the always-on core: an invariant's
		// reminder qualifies a non-negotiable, and scoping it would make the
		// constraint load while its qualifier did not.
		topic := p.Sidecar.Topic
		if len(p.Sidecar.Paths) == 0 && topic != "" {
			pathlessReminders[topic] = append(pathlessReminders[topic], p.Sidecar.Reminders...)
		}
		if (strings.HasPrefix(p.ArtifactID, "INV-") && len(p.Sidecar.Paths) == 0) || topic == "" {
			allReminders = append(allReminders, p.Sidecar.Reminders...)
			for _, v := range p.Sidecar.Verification {
				allVerification = append(allVerification, v.Text)
			}
			continue
		}
		topicReminders[topic] = append(topicReminders[topic], p.Sidecar.Reminders...)
		// Extract .Text from each VerificationEntry — the renderer expects
		// bare strings. The verify: command stays on the sidecar; it is
		// surfaced via `bin/edikt verify gov <ID>`, not the compiled checklist.
		for _, v := range p.Sidecar.Verification {
			topicVerification[topic] = append(topicVerification[topic], v.Text)
		}
	}

	groups := groupByTopic(pairs)
	topicNames := make([]string, 0, len(groups))
	for name := range groups {
		// Skip topics whose only directives came from INVs — groupByTopic
		// excludes INV directives, so a topic with only INV sidecars produces
		// an empty directive list. Don't write an empty topic file. A topic
		// with no directives but author-authored manual_directives or
		// prohibitions still warrants a file: the user explicitly added
		// content for it.
		//
		// ADR-066: a topic whose sidecars are entirely scoped (paths:) now  edikt-guard:allow
		// legitimately produces an empty Directives/Prohibitions list too —
		// that content moved to directive-index.yaml, not lost. Gating
		// solely on rendered content would make such a topic indistinguishable
		// from a genuinely empty one and drop it silently: not retired to
		// skill (it declared paths:, so the unscoped/tier-3 decision below
		// never sees it), not rendered, absent from every report. len(g.Paths)
		// keeps it visible: a declared scope always earns a file, even one
		// whose compiled-directives region is empty because its content lives
		// in the index instead.
		g := groups[name]
		if len(g.Directives) > 0 || len(g.Manual) > 0 || len(g.Prohibitions) > 0 || len(g.Paths) > 0 {
			topicNames = append(topicNames, name)
		}
	}
	sort.Strings(topicNames)

	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir governance: %w", err)
	}

	res := &Result{}

	for _, name := range topicNames {
		g := groups[name]
		topicDescription := opts.TopicDescriptions[name]
		// The description is now an INPUT to this topic's render, so it has to
		// be an input to the fingerprint that decides whether the render can
		// be skipped. Without this, editing and re-approving a description
		// leaves every already-rendered topic file frozen with the old text —
		// the short-circuit at line ~334 would find a matching fingerprint and
		// never look at the new bytes. A cache key that omits an input is not
		// a cache key.
		fp := TopicRenderFingerprint(g.Sidecars, topicDescription)
		dest := filepath.Join(opts.OutDir, name+".md")

		// Track directive count for the header comment (invariant rules in
		// governance.md are counted separately, below).
		//
		// g.Manual is included deliberately. ADR-008's formula for what a surface
		// carries is (directives - suppressed) ∪ manual_directives, and the
		// renderer emits both — but this counter read only g.Directives, so the
		// header undercounted by exactly the number of HAND-AUTHORED directives —
		// the ones a human wrote into a sidecar because the extractor did not
		// produce them. A count that drops them understates the surface by
		// precisely the population least likely to be noticed missing, since
		// nothing else in the pipeline regenerates them.
		res.TotalDirectives += len(g.Directives) + len(g.Manual)

		// Resolve scope BEFORE the short-circuit below. An unchanged topic
		// is still a topic whose scope was decided, and dropping it out of
		// the tally would make the coverage fraction depend on how much of
		// the corpus happened to be cached (INV-013: the fraction must move
		// with the corpus, not with the cache).
		topicPaths, scopeNote := scopeFor(g)
		unscoped := len(g.Undeclared) > 0
		if unscoped {
			res.TopicsUnscoped = append(res.TopicsUnscoped, name)
			for _, id := range g.Undeclared {
				res.UndeclaredSources = appendUnique(res.UndeclaredSources, id)
			}
		} else {
			res.TopicsScoped = append(res.TopicsScoped, name)
		}

		// The tier decision runs BEFORE the render-skip short-circuit, and that
		// ordering is load-bearing: the cache asks whether the CONTENT changed,
		// and retirement is a decision about WHERE the topic lives. Placed after
		// the short-circuit it never ran for a cached topic — measured as
		// "0 rendered, 10 unchanged" with all six unscoped files still on disk.
		// Same shape as the renderer-fingerprint defect: a decision behind a
		// cache check that the cache is entitled to skip.
		// TIER DECISION (SPEC-011 stage 1). An UNSCOPED topic — one whose  edikt-guard:allow
		// contributing sidecars declared no globs — does not belong in
		// .claude/rules/ at all. A rules file with `paths: **` loads on every
		// edit, which is the ambient cost this stage exists to remove; the
		// topic reaches the reader through its SKILL PACKAGE instead, loaded
		// when the task calls for it.
		//
		// Rendering BOTH is not a harmless intermediate state — it is the
		// double-loading condition AC-2.2 forbids, with the same directive
		// delivered twice by two tiers. So the rules file is REMOVED here
		// rather than left behind: an orphan tier-2 file would keep loading
		// forever while the skill quietly duplicated it.
		//
		// The topic is recorded as tier-3 whether or not a stale file was
		// there to delete. Keying the record on a successful os.Remove made
		// the field mean "a file was deleted this run", so a FRESH CLONE —
		// which has no stale files by definition — reported zero retirements
		// and its reader never learned which topics were skill-only. The
		// question a reader asks is "where does this topic live", and that
		// answer cannot depend on what happened to be on disk beforehand.
		if unscoped {
			res.TopicsRetiredToSkill = append(res.TopicsRetiredToSkill, name)
			// Existence checked BEFORE the remove, not inferred from the
			// remove's own success — os.IsNotExist(err) already tells us
			// "wasn't there", but reading that off the removal error ties
			// the regression signal to a side effect instead of to the fact
			// itself. A topic present here had a tier-2 file an instant ago;
			// this compile is what took it away. That is TopicsNewlyUnreachable.
			if _, statErr := os.Stat(dest); statErr == nil {
				res.TopicsNewlyUnreachable = append(res.TopicsNewlyUnreachable, name)
			}
			if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("retire topic file %s: %w", name, err)
			}
			continue
		}

		// Diff-only short-circuit: if the existing topic file declares the
		// same fingerprint, every contributing sidecar is byte-equal to last
		// run; skip the render to keep mtime stable and the file untouched.
		// Bust the cache when the on-disk file lacks one of the three
		// Phase 8 managed regions (bootstrap-write semantics: a v0.6.0-rc4
		// shaped file gets the new prohibitions/manual anchors on first
		// post-upgrade compile).
		//
		// hasScopeNote is the same mechanism for the paths: fix. The
		// fingerprint covers the SIDECARS, not the emitter, so a change to
		// how paths: is derived does not bust it — an upgraded binary would
		// leave every cached topic file scoped to the ADR documents
		// forever. Absence of the scope comment marks a pre-fix file.
		if existingFP, ok := readTopicFingerprint(dest); ok && existingFP == fp &&
			hasAllRegions(dest) && hasScopeNote(dest) {
			res.TopicsUnchanged = append(res.TopicsUnchanged, name)
			continue
		}

		dirLines, prohLines, manLines := buildRegionLines(g)
		body, err := render.RenderTopic(render.TopicView{
			Name:            name,
			Description:     topicDescription,
			Paths:           topicPaths,
			ScopeNote:       scopeNote,
			Sources:         g.Sources,
			Rules:           g.Directives,
			CompiledAt:      opts.CompiledAt,
			CompilerVersion: opts.CompilerVersion,
			Fingerprint:     fp,
			// PATHLESS reminders are included here, not only in the skill.
			// A pathless artifact has no glob, so the glob-keyed directive
			// index has no key for its reminders; the skill package used to
			// be their only home. Once a SCOPED topic's skill became a
			// pointer stub, that home vanished and they reached no surface at
			// all — the same silent-loss class this release exists to close,
			// reintroduced by the fix for a different one. The topic file is
			// the scoped topic's one home, so they belong here.
			//
			// dedupeStrings because a topic that is BOTH scoped and carries a
			// pathless artifact would otherwise list a shared reminder twice.
			Reminders:        dedupeStrings(append(append([]string(nil), topicReminders[name]...), pathlessReminders[name]...)),
			Verification:     topicVerification[name],
			DirectiveLines:   dirLines,
			ProhibitionLines: prohLines,
			ManualLines:      manLines,
			// No region anchors are passed: render computes each one from the
			// bytes IT emits between that region's markers (D357). This used
			// to hand render three hashes computed here from a
			// RECONSTRUCTION of the body render was about to produce,
			// carrying a withProhibitionsHeading flag that restated the
			// template's own structure. Predicting another component's bytes
			// is how one fact came to have three copies, one of which had
			// drifted, with nothing able to see it.
		})
		if err != nil {
			return nil, fmt.Errorf("render topic %s: %w", name, err)
		}
		if err := assertNoRegionOverlap(name, body); err != nil {
			return nil, err
		}
		changed, err := writeAtomicIfChanged(dest, body)
		if err != nil {
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
		if changed {
			res.TopicsRendered = append(res.TopicsRendered, name)
		} else {
			res.TopicsUnchanged = append(res.TopicsUnchanged, name)
		}
	}

	indexBody, err := render.RenderIndex(render.IndexView{
		CompiledAt:        opts.CompiledAt,
		CompilerVersion:   opts.CompilerVersion,
		ADRCompiled:       countSources(pairs, sidecar.KindADR),
		INVCompiled:       countSources(pairs, sidecar.KindInvariant),
		GuidelineCompiled: countSources(pairs, sidecar.KindGuideline),
		Excluded:          opts.Excluded,
		DirectiveCount:    res.TotalDirectives + len(invariantRules),
		TopicCount:        len(topicNames),
		InvariantRules:    invariantRules,
		InvariantRestated: append([]compile.Rule(nil), invariantRules...),
		TopicIndex:        topicIndexRows(topicNames, opts.TopicDescriptions, res.TopicsRetiredToSkill),
		Reminders:         allReminders,
		Verification:      allVerification,
	})
	if err != nil {
		return nil, fmt.Errorf("render index: %w", err)
	}
	idxChanged, err := writeAtomicIfChanged(opts.IndexPath, indexBody)
	if err != nil {
		return nil, fmt.Errorf("write index: %w", err)
	}
	res.IndexWritten = idxChanged

	// Surface (c): the directive index — the hook tier's input, and the new
	// home of reminder aggregation. The ambient core no longer carries a
	// Reminders section; without this write, every reminder on a scoped
	// artifact would have nowhere to go, which is the silent-loss class this
	// release exists to close.

	// Surface (d): skill packages — tier 3. Carries the topic's guidance and,
	// critically, the reminders of PATHLESS artifacts, which the glob-keyed
	// index has no key for and which would otherwise be dropped entirely.
	//
	// A SCOPED topic gets a POINTER STUB instead of a copy: its pinned
	// description plus a body naming the rules file that holds the content.
	//
	// The alternative shapes were both worse. Copying the directives into the
	// skill duplicated 255 of 346 directives across four topics (~9,683
	// est-tokens per surface) and fails AC-2.2 in its strong form — one
	// directive BODY, one surface, whether or not the two are loaded at the
	// same moment. Dropping the skill entirely would cost pre-file-touch
	// reach on the four highest-stakes domains: E3 showed task language
	// selecting `security` from "a filename with a semicolon reaching a
	// command invocation", where no file has been touched yet and the
	// write-time hook fires only after the approach is already chosen.
	//
	// The stub keeps both: content has exactly one home, and task language
	// still finds a signpost to it.
	retiredSet := map[string]bool{}
	for _, n := range res.TopicsRetiredToSkill {
		retiredSet[n] = true
	}

	// Surface (e): delivery-map.yaml — the ROUTING TABLE, not a delivery
	// surface. Written HERE and not beside the index above, because `surface`
	// depends on the tier decision and retiredSet is only complete at this
	// point. Building it earlier would have recorded every advisory directive
	// as topic-file delivered, including the ones that had just been retired to
	// a skill — a routing table that is wrong about precisely the case it was
	// added to make visible.
	deliveryMapPath := filepath.Join(opts.OutDir, deliveryMapName)
	deliveryMap := BuildDeliveryMap(pairs, retiredSet)

	// THE CONSEQUENCE OF PATHLESSNESS, counted rather than left to be inferred.
	//
	// A sidecar that declares no globs retires its whole TOPIC to tier 3, and
	// that decision reaches every OTHER sidecar in that topic — including
	// scoped ones whose own globs are perfectly good. Their advisory stops
	// being delivered on a path match and becomes reachable only by judgement,
	// through the skill package.
	//
	// That is a legitimate destination for a rule with no file scope: a process
	// rule or a naming convention has nothing to trigger on, and judgement
	// routing is correct for it. What was wrong is that a directive routed
	// there because inference came up EMPTY looked identical to one routed
	// there deliberately. Counting it is what turns today's number into a
	// recorded condition — and makes it move visibly when inference improves.
	seenSkill := map[string]bool{}
	for _, entries := range deliveryMap {
		for _, e := range entries {
			if e.Surface == SurfaceSkill {
				seenSkill[e.ID] = true
			}
		}
	}
	res.JudgementRoutedDirectives = len(seenSkill)

	dmBody := RenderDeliveryMap(deliveryMap)
	dmChanged, err := writeAtomicIfChanged(deliveryMapPath, dmBody)
	if err != nil {
		return nil, fmt.Errorf("write delivery map: %w", err)
	}
	res.DeliveryMapWritten = dmChanged

	skillsRoot := filepath.Join(projectRoot, ".claude", "skills")
	for _, name := range skillTopics(groups) {
		g := groups[name]
		// dedupeStrings, not a bare concat. A PATHLESS non-invariant artifact
		// lands in BOTH maps — pathlessReminders because it declared no
		// globs, topicReminders because it still belongs to a topic — so the
		// concatenation listed each of its reminders twice in the rendered
		// skill. Measured at 27 repeats across four skills. Harmless-looking,
		// but a rule stated twice reads as emphasis rather than as a render
		// bug, which is why it survived until a gate counted it.
		reminders := dedupeStrings(append(append([]string(nil), topicReminders[name]...), pathlessReminders[name]...))

		view := SkillView{
			Topic:       name,
			Description: opts.TopicDescriptions[name],
			// Passed in by the caller from the REAL project root: this
			// function may be rendering into a scratch tree.
			ContractBlock: contractBlockForTopic(opts.ContractBlock, name),
			// The UNION of extracted and manual directives, annotated exactly as
			// the topic file annotates them. A retired topic's skill is its
			// only home, so g.Directives alone loses every manual entry.
			Directives:   unionDirectiveRules(g),
			Prohibitions: g.Prohibitions,
			Verification: dedupeStrings(topicVerification[name]),
			Reminders:    reminders,
			Sources:      g.Sources,
			// What the index took, so the stub can point honestly. Computed
			// for every topic, used only by the stub branch — a retired topic
			// renders its own bodies and has no pointer to qualify.
			// Read from the SAME builder the topic file renders from, not
			// from len(g.Directives): the topic file's Directives region also
			// carries manual entries, so a topic with manual-only content has
			// a non-empty region while g.Directives is empty. Asking the
			// builder is asking what the file will actually contain.
			FileCarriesDirectives: func() bool {
				d, p, _ := buildRegionLines(g)
				return len(d) > 0 || len(p) > 0
			}(),
		}
		if !retiredSet[name] {
			// Tier 2 owns ALL of it — prohibitions and verification items too,
			// not just the directives. Clearing only Directives would have left
			// the stub carrying prohibitions that the rules file also carries,
			// which is the AC-2.2 duplication this stub exists to remove.
			view.Prohibitions = nil
			view.Verification = nil
			// Tier 2 owns the content. The stub carries the POINTER and
			// nothing else — not an abridged list, not the "important ones",
			// because any subset is still a second copy of those bodies.
			view.PointsTo = relToRoot(projectRoot, filepath.Join(opts.OutDir, name+".md"))
			view.Directives = nil
			view.Reminders = nil
		}
		body := RenderSkill(view)
		dir := filepath.Join(skillsRoot, "edikt-"+name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir skill %s: %w", name, err)
		}
		dest := filepath.Join(dir, "SKILL.md")
		if _, err := writeAtomicIfChanged(dest, body); err != nil {
			return nil, fmt.Errorf("write skill %s: %w", name, err)
		}
		res.Surfaces = append(res.Surfaces, Surface{
			Kind: "skill-package", Path: relToRoot(projectRoot, dest), Topic: name,
		})
		res.SkillsRendered = append(res.SkillsRendered, name)
	}

	// Publish the surface set. Built from topicNames (every topic considered
	// this run), not from TopicsRendered — a cache hit still produced that
	// surface, and an enumeration that dropped cached files would shrink
	// every time the cache worked.
	res.Surfaces = append(res.Surfaces, Surface{
		Kind: "ambient-core",
		Path: relToRoot(projectRoot, opts.IndexPath),
	})
	// The routing table is a GENERATED surface and is enumerated as one, even
	// though nothing loads it into a model's context. Surfaces feeds both the
	// manifest (INV-019: the reconciliation's subject set is the manifest, so
	// an unlisted file is unmanaged and never updated or retired) and the
	// render-freshness expectation. Omitting it made a fresh render not produce
	// it, and the freshness check correctly reported the live copy as an orphan
	// — the check working, on a surface that had been left out of the contract.
	res.Surfaces = append(res.Surfaces, Surface{
		Kind: "delivery-map",
		Path: relToRoot(projectRoot, deliveryMapPath),
	})
	retired := map[string]bool{}
	for _, n := range res.TopicsRetiredToSkill {
		retired[n] = true
	}
	for _, name := range topicNames {
		if retired[name] {
			continue // tier 3 now; there is no topic-file surface to enumerate
		}
		res.Surfaces = append(res.Surfaces, Surface{
			Kind:  "topic-file",
			Path:  relToRoot(projectRoot, filepath.Join(opts.OutDir, name+".md")),
			Topic: name,
		})
	}
	sort.Slice(res.Surfaces, func(i, j int) bool { return res.Surfaces[i].Path < res.Surfaces[j].Path })

	// ORPHAN CLEANUP (AC-2.5), before the manifest is overwritten.
	//
	// Ordering is load-bearing: the PREVIOUS manifest is the only record of
	// what the last compile wrote, and writing the new one first would destroy
	// it. A tree walk is not a substitute — it cannot tell a surface whose
	// topic disappeared from a file somebody added on purpose, and a cleanup
	// that guesses would delete the second kind.
	manifestPath := filepath.Join(opts.OutDir, ManifestName)
	current := map[string]bool{}
	for _, s := range res.Surfaces {
		current[s.Path] = true
	}
	if prevBody, rerr := os.ReadFile(manifestPath); rerr == nil {
		for _, e := range ParseManifest(string(prevBody)) {
			if current[e.Path] {
				continue
			}
			abs := filepath.Join(projectRoot, filepath.FromSlash(e.Path))
			// Only a surface THIS tool wrote and still recognises is removable.
			// The path came from our own manifest, so it is not arbitrary — but
			// a manifest can be hand-edited, and deleting whatever it names
			// would turn a text edit into a delete primitive.
			if !isRemovableSurface(e.Kind) {
				continue
			}
			if derr := os.Remove(abs); derr == nil {
				res.OrphansRemoved = append(res.OrphansRemoved, e.Path)
				// A skill package owns its directory; an empty edikt-<topic>/
				// left behind still registers as a skill with no content.
				if e.Kind == "skill-package" {
					_ = os.Remove(filepath.Dir(abs))
				}
			} else if !os.IsNotExist(derr) {
				return nil, fmt.Errorf("remove orphaned surface %s: %w", e.Path, derr)
			}
		}
		sort.Strings(res.OrphansRemoved)
	}

	// The manifest is written LAST: it records what exists, so it must be
	// written after everything it records — including the orphan removals.
	entries, merr := BuildManifest(projectRoot, res.Surfaces)
	if merr != nil {
		return nil, merr
	}
	manChanged, merr := writeAtomicIfChanged(manifestPath, RenderManifest(entries))
	if merr != nil {
		return nil, fmt.Errorf("write manifest: %w", merr)
	}
	res.ManifestWritten = manChanged
	res.Surfaces = append(res.Surfaces, Surface{
		Kind: "manifest",
		Path: relToRoot(projectRoot, manifestPath),
	})

	// DELIVERY ASSERTION — runs LAST, after every surface is written.
	//
	// Every effective rule (directives minus suppressed, union manual) must
	// appear on some rendered surface. Twice now a whole class of rule has
	// reached none: prohibitions and verification items for retired topics
	// (skillpackage.go:51-55), and manual directives for the same topics. Both
	// were found by someone counting, not by a gate. This is the gate.
	//
	// It fails the compile rather than warning: a rule that reaches no surface
	// is not delivered, and a compile that reports success while delivering
	// less than it declares is the failure mode this whole surface exists to
	// prevent (INV-011).
	undelivered, derr := AssertDelivery(projectRoot, pairs)
	if derr != nil {
		return nil, fmt.Errorf("delivery assertion: %w", derr)
	}
	if len(undelivered) > 0 {
		return nil, fmt.Errorf("delivery assertion failed — %s", FormatUndelivered(undelivered))
	}

	return res, nil
}

// isRemovableSurface names the surface kinds compile owns outright and may
// therefore delete when they orphan.
//
// An allowlist, not a denylist: a future surface kind is NOT deletable until
// someone adds it here deliberately. The failure mode of forgetting is a
// lingering orphan, which is visible and harmless; the failure mode of a
// denylist that forgot an entry is deleting a file edikt does not own.
func isRemovableSurface(kind string) bool {
	switch kind {
	case "topic-file", "skill-package":
		return true
	}
	return false
}

// relToRoot renders an absolute output path as a repo-relative slash path,
// falling back to the absolute path when it lies outside the root (which
// happens in tests that point OutDir somewhere else entirely).
// dedupeStrings preserves first-seen order and drops exact repeats.
func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func relToRoot(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

func groupByTopic(pairs []sidecar.Pair) map[string]*topicGroup {
	g := make(map[string]*topicGroup)
	for _, p := range pairs {
		if p.Sidecar == nil {
			continue
		}
		topic := p.Sidecar.Topic
		t, ok := g[topic]
		if !ok {
			t = &topicGroup{Name: topic}
			g[topic] = t
		}
		t.Sidecars = append(t.Sidecars, p.Sidecar)
		// The topic's code scope is the union of what its sidecars declare
		// in `paths:`. Sidecar.Path — the artifact's own .md location — is
		// NOT a code glob and must never land here: it scoped every
		// compiled topic file to the ADR documents themselves.
		//
		// The union covers exactly the artifacts whose content LANDS in the
		// topic file — the same set the directive loop below uses. A PATHLESS
		// INVARIANT contributes only its canonical statement to the ambient
		// core and nothing here, so counting it as undeclared held the whole
		// topic open on account of content that is not in it: a topic whose
		// ADRs were all properly scoped would still be reported unscoped, and
		// since SPEC-011 stage 1 would be RETIRED TO TIER 3 — moving scoped  edikt-guard:allow
		// tier-2 rules out of the reader's path because an invariant
		// elsewhere in the file's topic declared no globs. Declaring globs on
		// the invariant is not even the fix, because a scoped invariant
		// changes tier itself.
		contributesToTopicFile := !strings.HasPrefix(p.ArtifactID, "INV-") || len(p.Sidecar.Paths) > 0
		if len(p.Sidecar.Paths) > 0 {
			for _, glob := range p.Sidecar.Paths {
				t.Paths = appendUnique(t.Paths, glob)
			}
		} else if contributesToTopicFile {
			t.Undeclared = appendUnique(t.Undeclared, p.ArtifactID)
		}
		for _, phase := range p.Sidecar.Scope {
			t.Scope = appendUnique(t.Scope, phase)
		}
		t.Sources = appendUnique(t.Sources, p.ArtifactID)
		for _, s := range p.Sidecar.Signals {
			t.Signals = appendUnique(t.Signals, s)
		}
		// The three-list formula (ADR-008, implemented for the legacy path
		// by compile.EffectiveRules):
		//
		//   effective = (directives - suppressed_directives) ∪ manual_directives
		//
		// The live merge used to append Directives wholesale and never read
		// SuppressedDirectives, so an author who suppressed a directive got
		// it compiled anyway — silently, while the struct comment told them
		// subtraction happened at compile time. Only the legacy
		// --legacy-only path honoured the field.
		//
		// Set difference is by exact text match, matching EffectiveRules.
		// It applies to `directives` and nothing else: manual_directives
		// are unioned AFTER the subtraction (below), so re-stating a
		// suppressed directive as a manual one restores it in the author's
		// own wording — and prohibitions are outside the formula entirely,
		// being a later top-level array (ADR-032).
		suppressed := make(map[string]struct{}, len(p.Sidecar.SuppressedDirectives))
		for _, sd := range p.Sidecar.SuppressedDirectives {
			suppressed[sd] = struct{}{}
		}

		// A PATHLESS invariant contributes only its canonical statement to the
		// ambient core and nothing to a topic file. A SCOPED invariant (one
		// that declares `paths:`) is the opposite: it belongs in its topic
		// file like any other artifact, because its own author scoped it.
		//
		// Getting this wrong in the other direction is what the condition
		// guards: excluding all INV- artifacts here, while the core now takes
		// only the first directive of pathless ones, would silently DROP every
		// enforcement clause of every scoped invariant from the corpus.
		// ADR-066: a sidecar that declares paths: is delivered to the reader  edikt-guard:allow
		// through directive-index.yaml alone (hook match, synchronous and
		// per-directive-precise) — not also dumped into the topic file's
		// ambient body. Rendering it here too duplicated every scoped
		// directive across both channels on the same touch. Manual
		// directives have no index counterpart (they are hand-authored,
		// never extracted) and always render here regardless of paths.
		if len(p.Sidecar.Paths) > 0 {
			// Account for what the index took, so the topic file and the
			// skill package can say so instead of rendering an unexplained
			// empty region. Same function the index itself is built from.
			for _, m := range p.Sidecar.ManualDirectives {
				t.Manual = append(t.Manual, manualEntry{Text: m, Source: p.ArtifactID})
			}
			// ADR-079 (supersedes ADR-072's digest, amends ADR-066):  edikt-guard:allow
			// ADVISORY renders here in full; MUST stays in the index. The
			// split is by GRADE, not by whether an entry carries a paths
			// override, because grade already encodes delivery timing: MUST
			// is a guardrail that must interrupt a write, advisory is context
			// that must arrive BEFORE the model chooses. A topic file loads on
			// a paths: match before any write; PostToolUse arrived after it.
			//
			// ADR-072's trigger was the per-directive override, which one
			// directive of 831 carried — right intent, too narrow to move
			// anything. Its subset is a subset of advisory, so the general
			// rule REPLACES it rather than layering on it, and nothing is
			// dual-channel any more: each grade has exactly one home.
			//
			// Actor-scoped entries are NOT filtered here. ADR-065 excludes  edikt-guard:allow
			// them from the index "and only there" to remove PreToolUse noise,
			// explicitly leaving topic files untouched — so an advisory
			// actor-scoped directive renders here, as that ADR intends.
			//
			// Destination is RouteOf's verdict, not a local grade test, so this
			// renderer and delivery-map.yaml cannot disagree about where a
			// directive went. topicTier3 is false here because this branch only
			// runs for a topic that HAS a rules file; the tier-3 case renders
			// through the skill package instead.
			cls := classFor(p.ArtifactID)
			// A SCOPED invariant's first surviving directive is its canonical
			// statement and already lives in the ambient core (the INV- branch
			// above takes it regardless of paths). Rendering it here as well
			// put one body on two prose surfaces — AC-2.2's no-double-loading
			// gate caught INV-010:d01 in both governance.md and the compile
			// skill on 2026-09-02, the day actor-scoped MUST started routing
			// here. The index is a different matter (hook input, exempt from
			// that gate, and layered by design); prose surfaces are not.
			firstINV := strings.HasPrefix(p.ArtifactID, "INV-")
			for _, d := range p.Sidecar.Directives {
				if _, isSuppressed := suppressed[d.Text]; isSuppressed {
					continue
				}
				if firstINV {
					firstINV = false // the canonical statement; it is in the core
					continue
				}
				if RouteOf(gradeFor(cls, false, d.Enforcement), d.ActorScope, false) != SurfaceTopicFile {
					continue
				}
				t.Directives = append(t.Directives, compile.Rule{Text: d.Text, Source: p.ArtifactID})
			}
			for _, pr := range p.Sidecar.Prohibitions {
				if RouteOf(gradeFor(cls, true, pr.Enforcement), pr.ActorScope, false) != SurfaceTopicFile {
					continue
				}
				t.Prohibitions = append(t.Prohibitions, compile.Rule{Text: pr.Text, Source: p.ArtifactID})
			}
		} else if strings.HasPrefix(p.ArtifactID, "INV-") {
			// A PATHLESS INVARIANT contributes its FIRST surviving directive —
			// the canonical statement — to the ambient core, and it must not be
			// repeated here (AC-2.2: one body, one home).
			//
			// Its REMAINING directives used to go nowhere at all. The ambient
			// rule takes only the statement, and this branch excluded the whole
			// artifact from every topic group, so clauses 2..N reached no
			// surface: not ambient, not tier 2, not a skill. Measured at 10
			// MUST-grade directives on INV-002 — "no other changes to the  edikt-guard:allow
			// accepted ADR are permitted", "a new ADR MUST be written that
			// supersedes the old one" — the enforcement half of an invariant
			// whose statement alone does not tell a reader what to do.
			//
			// Every AC-3.x gate was green over this, and the AC-3.3 coverage
			// baseline is what found it, exactly as the 210 missing bodies were
			// found in phase 2.
			//
			// The remainder joins its topic group and follows the tier ladder
			// from there: a retired topic renders them in its skill body, a
			// scoped topic in its rules file. One home either way.
			first := true
			for _, d := range p.Sidecar.Directives {
				if _, isSuppressed := suppressed[d.Text]; isSuppressed {
					continue
				}
				if first {
					first = false // the canonical statement; it is in the core
					continue
				}
				t.Directives = append(t.Directives, compile.Rule{Text: d.Text, Source: p.ArtifactID})
			}
			// Prohibitions are NOT canonical statements and were dropped by the
			// same exclusion. All of them belong here.
			for _, pr := range p.Sidecar.Prohibitions {
				t.Prohibitions = append(t.Prohibitions, compile.Rule{Text: pr.Text, Source: p.ArtifactID})
			}
			for _, m := range p.Sidecar.ManualDirectives {
				t.Manual = append(t.Manual, manualEntry{Text: m, Source: p.ArtifactID})
			}
		} else {
			for _, d := range p.Sidecar.Directives {
				if _, isSuppressed := suppressed[d.Text]; isSuppressed {
					continue
				}
				t.Directives = append(t.Directives, compile.Rule{Text: d.Text, Source: p.ArtifactID})
			}
			for _, m := range p.Sidecar.ManualDirectives {
				t.Manual = append(t.Manual, manualEntry{Text: m, Source: p.ArtifactID})
			}
			for _, pr := range p.Sidecar.Prohibitions {
				t.Prohibitions = append(t.Prohibitions, compile.Rule{Text: pr.Text, Source: p.ArtifactID})
			}
		}
	}
	for _, t := range g {
		sort.Strings(t.Paths)
		sort.Strings(t.Scope)
		sort.Strings(t.Sources)
		sort.Strings(t.Undeclared)
	}
	return g
}

// The routing table's Scope column, and the scopeDisplay/allLifecyclePhases
// pair that computed it, were removed with the table itself (SPEC-011). The  edikt-guard:allow
// per-sidecar `scope:` data they read is untouched and still validated; what
// is gone is the one surface that displayed it. Recorded here rather than
// left as dead code with a passing unit test, which is the shape that hid
// scopeDisplay's own defect for months: a validated, populated field with no
// live consumer, and a test that proved the computation rather than the
// delivery.

// TopicFingerprint returns a stable hash over the (path, sidecar_content_hash)
// tuples contributing to a topic, per Phase 8 of PLAN-sidecar-architecture.
//
// The content hash is taken over the canonical-YAML serialization of the
// in-memory sidecar (sidecar.Marshal), not the raw file bytes — that way the
// fingerprint is invariant to harmless whitespace drift in pre-canonical
// sidecars and matches the CI canonical-write gate.
func TopicFingerprint(group []*sidecar.Sidecar) string {
	tuples := make([]string, 0, len(group))
	for _, s := range group {
		// Fingerprint over a copy with the transient extraction-time
		// proposal fields cleared. Per their own doc comments on Sidecar,
		// ProposedPaths/ProposedRemovals/ProposedTopicDescription are read
		// by nothing downstream — "not merge, not verify-diff, not the
		// render" — but sidecar.Marshal still serializes them since it is
		// also the general-purpose disk-persistence encoder (clearing them
		// there would silently drop a real pending proposal on write). Left
		// unstripped here, the fingerprint depended on whether the caller
		// happened to have already cleared them in memory (the compile
		// path's proposals routing does; a fresh disk load, as the
		// freshness-check's shadow render performs, does not) — identical
		// on-disk bytes, two different hashes, a false "stale" report with
		// no actual content change (found while closing PLAN-v090-release
		// Phase 6, 2026-09-04).
		fp := *s
		fp.ProposedPaths = nil
		fp.ProposedRemovals = nil
		fp.ProposedTopicDescription = ""
		data, err := sidecar.Marshal(&fp)
		if err != nil {
			// Fall back to on-disk bytes; preserves Phase 5's looser contract
			// when canonical marshal fails (should be unreachable in practice).
			data, err = os.ReadFile(s.SourcePath)
			if err != nil {
				continue
			}
		}
		sum := sha256.Sum256(data)
		tuples = append(tuples, s.Path+":"+hex.EncodeToString(sum[:]))
	}
	sort.Strings(tuples)
	h := sha256.New()
	for _, t := range tuples {
		h.Write([]byte(t))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// TopicRenderFingerprint is the value written to a topic file's
// `_fingerprint` frontmatter key, and the value the render-skip short-circuit
// compares against.
//
// It covers EVERY input to the topic render: the contributing sidecars, the
// topic's approved registry description, the RENDER TEMPLATES themselves, and
// the PROSE THIS PACKAGE INTERPOLATES INTO THEM (noteEmitterFingerprint).
// Both of the last two were missing at some point and both had to be measured
// to be believed: editing a template propagated to zero of ten topic files,
// and later editing the delivery-note wording propagated to zero of six —
// each time because the cache asked whether the INPUTS had changed and never
// whether the EMITTER had. TopicFingerprint alone covers only the sidecars,
// which is why it is not what gets written: a cache key that omits an input is
// not a cache key.
//
// This is the single exported entry point for that value on purpose. A caller
// that recomposed it from parts would be reproducing the logic it is meant to
// be asserting (INV-014).  edikt-guard:allow
func TopicRenderFingerprint(group []*sidecar.Sidecar, description string) string {
	fp := mixFingerprint(TopicFingerprint(group), description)
	fp = mixFingerprint(fp, render.RendererFingerprint())
	return mixFingerprint(fp, noteEmitterFingerprint())
}

// mixFingerprint folds a second input into an existing fingerprint.
//
// It is domain-separated (the sidecar hash and the description are joined by a
// NUL that neither can contain) so no pair of inputs can collide by
// concatenation. An empty description still mixes: "this topic has no approved
// description" and "this topic's description is the empty string" must not
// produce the same key as each other, and neither may collide with a
// pre-registry fingerprint — otherwise the first compile after the registry
// lands would reuse stale cached topic files.
func mixFingerprint(base, description string) string {
	h := sha256.New()
	h.Write([]byte(base))
	h.Write([]byte{0})
	h.Write([]byte("topic-description:v1"))
	h.Write([]byte{0})
	h.Write([]byte(description))
	return hex.EncodeToString(h.Sum(nil))
}

// topicIndexRows pairs each rendered topic with its approved description.
//
// A topic with no approved description gets an EMPTY Description. The template
// renders it as an explicit "(no approved description — pending)" marker
// rather than omitting the row: a topic silently missing from the index is
// indistinguishable from a topic that does not exist (INV-013), and the whole
// point of the migration window is that pending work is visible.
func topicIndexRows(topicNames []string, descriptions map[string]string,
	retiredToSkill []string) []render.TopicIndexRow {

	// A topic retired to a skill package has NO rules file, so the index must
	// not tell the reader to open one.
	skillOnly := make(map[string]bool, len(retiredToSkill))
	for _, n := range retiredToSkill {
		skillOnly[n] = true
	}

	rows := make([]render.TopicIndexRow, 0, len(topicNames))
	for _, name := range topicNames {
		rows = append(rows, render.TopicIndexRow{
			Topic:       name,
			Description: descriptions[name],
			SkillOnly:   skillOnly[name],
		})
	}
	return rows
}

// readTopicFingerprint extracts the `_fingerprint` field from an existing
// topic file's YAML frontmatter. Returns ("", false) when the file is absent,
// has no frontmatter, or the field is missing — every miss path forces a
// full render so the cache is fail-safe.
func readTopicFingerprint(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	front, ok := extractFrontmatter(data)
	if !ok {
		return "", false
	}
	var fm struct {
		Fingerprint string `yaml:"_fingerprint"`
	}
	// SPEC-009 Plan A AC-1.2: parses compiled-topic-file frontmatter for _fingerprint only.  // edikt-guard:allow
	// Not *.edikt.yaml. KnownFields off intentional — only one field needed; rest of frontmatter ignored.
	if err := yaml.Unmarshal(front, &fm); err != nil {
		return "", false
	}
	if fm.Fingerprint == "" {
		return "", false
	}
	return fm.Fingerprint, true
}

// extractFrontmatter returns the bytes between the first `---` and the next
// `---` line. Mirrors the convention enforced by render/templates/topic.md.tmpl.
func extractFrontmatter(data []byte) ([]byte, bool) {
	s := string(data)
	if !strings.HasPrefix(s, "---\n") {
		return nil, false
	}
	rest := s[4:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, false
	}
	return []byte(rest[:idx]), true
}

func writeAtomicIfChanged(path, content string) (bool, error) {
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) == content {
			return false, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	return true, nil
}

func appendUnique(slice []string, v string) []string {
	for _, s := range slice {
		if s == v {
			return slice
		}
	}
	return append(slice, v)
}

// countSources counts the artifacts of one class that actually contributed
// to this compile. Two filters, both load-bearing for the header's honesty:
//
//   - Kind — the artifact's class comes from which configured directory it
//     lives in, not from its filename. The old "no ADR-/INV- prefix ⇒
//     guideline" rule counted every README and stray note in the decisions
//     and invariants directories as a guideline, so `governance.md` claimed
//     guidelines that did not exist — including after the guidelines
//     directory had been deleted outright.
//   - Sidecar != nil — skip-listed artifacts (superseded ADRs, READMEs,
//     migration:skip markers) and any artifact without a sidecar contribute
//     no directives, so counting them overstates the compile's inputs.
func countSources(pairs []sidecar.Pair, kind string) int {
	n := 0
	for _, p := range pairs {
		if p.Kind == kind && p.Sidecar != nil {
			n++
		}
	}
	return n
}

// refTagRe extracts the ADR/INV/guideline ID from a directive's `(ref: …)`
// suffix. The first capture is the artifact ID. Falls back to the empty
// string when no tag is present so manual_directives without a ref tag
// still sort deterministically (alphabetical by text).
var refTagRe = regexp.MustCompile(`\(ref:\s*([A-Za-z0-9_-]+)`)

// extractRefTag returns the first artifact-ID found in a directive's
// `(ref: …)` clause, or "" when the directive has no tag.
func extractRefTag(text string) string {
	if m := refTagRe.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}

// buildRegionLines constructs the bullet bodies for the directives,
// prohibitions, and manual managed regions. Determinism contract:
//
// - Directives: extracted directives interleaved with manual_directives,
// sorted by ref-tag (asc), then manual-flag (extracted before manual on
// equal ref tag), then text. Manual entries get the
// ` (ref: <ID> + manual) *(manual)*` annotation; an entry whose own
// text already carries `(ref:` keeps the verbatim text and just gets the
// `*(manual)*` marker appended.
// - Prohibitions: text-only bullets sorted by text asc.
// - Manual: text-only bullets sorted by text asc — a faithful copy that
// downstream tooling can key on independently of the directives region.
// unionDirectiveRules returns g.Directives UNION g.Manual, carrying the same
// ref-tag and *(manual)* annotation and the same ordering buildRegionLines
// applies to the topic file's Directives region — as []compile.Rule, which is
// what SkillView.Directives takes.
//
// WHY THIS EXISTS. A topic RETIRED to a skill package has that skill as its
// ONLY home. Until now the skill received g.Directives raw and g.Manual was
// dropped, so every manual directive on a retired topic reached no compiled
// surface at all. Measured on a downstream corpus (bok-services, fresh
// compile): topics that render a topic file delivered 12 of 12 manual
// directives; topics retired to a skill delivered 2 and lost 61.
//
// THIS IS THE SAME DEFECT, ONE FIELD OVER, AS THE ONE ALREADY FIXED.
// skillpackage.go:51-55 records that rendering only Directives silently
// dropped Prohibitions and Verification for retired topics — "measured at 65
// prohibitions and 145 verification items lost across the six retired topics,
// found by the AC-2.7 coverage baseline rather than by any gate". Those two
// were fixed; Manual was not. And fourteen lines below the bug,
// FileCarriesDirectives already calls buildRegionLines(g) with a comment
// stating that the region "also carries manual entries, so a topic with
// manual-only content has a non-empty region while g.Directives is empty" —
// the author knew the union was the real content, used it for the PREDICATE,
// and passed the un-unioned field for the CONTENT. A defect that recurs one
// field over, in the same struct literal, after being fixed, is a design
// signal rather than an oversight: the struct takes a field named Directives
// and there is no type that means "everything this topic delivers".
//
// buildRegionLines is deliberately NOT refactored to share this code — the
// topic-file bytes must not shift. Drift between the two is prevented by a
// test asserting this function's Texts equal buildRegionLines' first return
// exactly, which asserts the resolved outcome rather than restating the logic
// (INV-014).
func unionDirectiveRules(g *topicGroup) []compile.Rule {
	type entry struct {
		text   string
		ref    string
		manual bool
		src    string
	}
	all := make([]entry, 0, len(g.Directives)+len(g.Manual))
	for _, d := range g.Directives {
		all = append(all, entry{text: d.Text, ref: extractRefTag(d.Text), manual: false, src: d.Source})
	}
	for _, m := range g.Manual {
		txt := m.Text
		if extractRefTag(txt) == "" {
			txt = strings.TrimRight(txt, " ") + " (ref: " + m.Source + " + manual)"
		}
		if !strings.Contains(txt, "*(manual)*") {
			txt = txt + " *(manual)*"
		}
		all = append(all, entry{text: txt, ref: m.Source, manual: true, src: m.Source})
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].ref != all[j].ref {
			return all[i].ref < all[j].ref
		}
		if all[i].manual != all[j].manual {
			return !all[i].manual && all[j].manual
		}
		return all[i].text < all[j].text
	})
	out := make([]compile.Rule, len(all))
	for i, e := range all {
		out[i] = compile.Rule{Text: e.text, Source: e.src}
	}
	return out
}

func buildRegionLines(g *topicGroup) (directives, prohibitions, manual []string) {
	type entry struct {
		text   string
		ref    string
		manual bool
	}
	all := make([]entry, 0, len(g.Directives)+len(g.Manual))
	for _, d := range g.Directives {
		all = append(all, entry{text: d.Text, ref: extractRefTag(d.Text), manual: false})
	}
	for _, m := range g.Manual {
		txt := m.Text
		if extractRefTag(txt) == "" {
			txt = strings.TrimRight(txt, " ") + " (ref: " + m.Source + " + manual)"
		}
		// Append the inline marker once; if a caller pre-annotated, leave
		// alone to keep the rendered line stable across regenerations.
		if !strings.Contains(txt, "*(manual)*") {
			txt = txt + " *(manual)*"
		}
		all = append(all, entry{text: txt, ref: m.Source, manual: true})
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].ref != all[j].ref {
			return all[i].ref < all[j].ref
		}
		if all[i].manual != all[j].manual {
			// extracted (false) before manual (true) on equal ref tag
			return !all[i].manual && all[j].manual
		}
		return all[i].text < all[j].text
	})
	directives = make([]string, len(all))
	for i, e := range all {
		directives[i] = e.text
	}

	prohibitions = make([]string, 0, len(g.Prohibitions))
	for _, p := range g.Prohibitions {
		prohibitions = append(prohibitions, p.Text)
	}
	sort.Strings(prohibitions)

	manual = make([]string, 0, len(g.Manual))
	for _, m := range g.Manual {
		manual = append(manual, m.Text)
	}
	sort.Strings(manual)
	return directives, prohibitions, manual
}

// regionSHA is GONE (D357). It hashed a reconstruction of the body the
// renderer was about to emit — each bullet as "- "+text+"\n", with a
// withProhibitionsHeading flag restating that topic.md.tmpl puts a heading
// inside one region's markers. render.RenderTopic now anchors each region
// from the bytes it actually emitted, so this file no longer models the
// renderer's private layout. Anything here that needs a region's hash must
// read the rendered output and call render.RegionSHA on it.

// hasAllRegions reports whether the file at path already declares all
// three Phase 8 managed regions. Missing-region paths force a fresh render
// even when the fingerprint cache would otherwise short-circuit, ensuring
// a v0.6.0-rc4-shaped governance file gets the new anchors on first
// post-upgrade compile (bootstrap-write semantics, AC #5).
func hasAllRegions(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	s := string(data)
	for _, kind := range []string{"directives", "prohibitions", "manual"} {
		if !strings.Contains(s, "[edikt:"+kind+":start]: #") {
			return false
		}
		if !strings.Contains(s, "[edikt:"+kind+":end]: #") {
			return false
		}
	}
	return true
}

// hasScopeNote reports whether a topic file on disk was written by an
// emitter that derives `paths:` from the sidecars' declared code globs.
//
// It is a shape detector, not a correctness check. Files written before
// that fix carry the ADR documents' own paths and no scope comment; the
// topic fingerprint hashes the SIDECARS, so it cannot tell that the
// emitter changed underneath an unchanged corpus. Without this, upgrading
// the binary would leave every cached topic file mis-scoped indefinitely.
func hasScopeNote(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "<!-- scope: ")
}

// regionMarker matches the start/end sentinels of one Phase 8 managed
// region. Captures (kind, position): kind ∈ {directives, prohibitions,
// manual}, position ∈ {start, end}.
var regionMarker = regexp.MustCompile(`(?m)^\[edikt:(directives|prohibitions|manual):(start|end)\]: #$`)

// assertNoRegionOverlap enforces byte-range integrity: the three
// managed regions in a topic file MUST NOT overlap, and each must close
// before the next opens. Returns a typed error citing the offending pair
// when an overlap or interleave is detected.
func assertNoRegionOverlap(topicName, body string) error {
	type span struct {
		kind  string
		start int
		end   int
	}
	matches := regionMarker.FindAllStringSubmatchIndex(body, -1)
	open := map[string]int{}
	var spans []span
	for _, m := range matches {
		// m[2:4]=kind, m[4:6]=position. Whole match offsets are m[0]:m[1].
		kind := body[m[2]:m[3]]
		pos := body[m[4]:m[5]]
		if pos == "start" {
			if _, dup := open[kind]; dup {
				return fmt.Errorf("sentinel-region violation: duplicate %q start sentinel in %s", kind, topicName)
			}
			open[kind] = m[0]
		} else {
			startOff, ok := open[kind]
			if !ok {
				return fmt.Errorf("sentinel-region violation: orphan %q end sentinel in %s", kind, topicName)
			}
			delete(open, kind)
			spans = append(spans, span{kind: kind, start: startOff, end: m[1]})
		}
	}
	for k := range open {
		return fmt.Errorf("sentinel-region violation: unclosed %q region in %s", k, topicName)
	}
	for i := 0; i < len(spans); i++ {
		for j := i + 1; j < len(spans); j++ {
			if spans[i].start < spans[j].end && spans[j].start < spans[i].end {
				return fmt.Errorf("sentinel-region violation: regions %s and %s overlap in %s", spans[i].kind, spans[j].kind, topicName)
			}
		}
	}
	return nil
}
