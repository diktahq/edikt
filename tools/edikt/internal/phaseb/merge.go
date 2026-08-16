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

	// DirectiveIndexWritten reports whether surface (c) changed on this run.
	DirectiveIndexWritten bool `json:"directive_index_written"`

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
	// Stage 1 adds "directive-index" and "skill-package".
	Kind string `json:"kind"`
	// Path is repo-relative, slash-separated.
	Path string `json:"path"`
	// Topic is set for topic-scoped surfaces and empty for singletons.
	Topic string `json:"topic,omitempty"`
}

// Options configures merge output.
type Options struct {
	OutDir          string // default .claude/rules/governance
	IndexPath       string // default .claude/rules/governance.md
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
	// IndexedDirectives / IndexedProhibitions / IndexedSources record what
	// this topic's SCOPED sidecars routed to directive-index.yaml INSTEAD of
	// into the topic file's body (ADR-066).  edikt-guard:allow
	//
	// They exist so the rendered surfaces can say where the content went. An
	// empty Directives region is a legitimate, intended state under ADR-066,  edikt-guard:allow
	// and an intended state that renders identically to a failed compile is
	// the empty-result class INV-013 forbids: a reader cannot tell "routed"
	// from "lost" by looking. These counts are what turns the silence into a
	// statement.
	//
	// Counted through phaseb.IndexEntriesFor — the same function that decides
	// what the index actually contains — so the note can never claim a number
	// the index does not hold.
	IndexedDirectives   int
	IndexedProhibitions int
	IndexedSources      []string
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
const directiveIndexName = "directive-index.yaml"

// deliveryNoteFor explains an EMPTY Directives region that is empty on
// purpose.
//
// ADR-066 routes every path-scoped directive to directive-index.yaml and out  edikt-guard:allow
// of the topic file's body. On a corpus where a topic's contributing sidecars
// are ALL scoped — the common case once a project declares globs — the result
// is a topic file whose Directives region renders as two bare anchors and
// nothing between them.
//
// That is correct, and it is indistinguishable from a compile that dropped
// the topic's content on the floor. INV-013's rule is that a measured zero
// and a failed measurement must not look the same to a reader, and here they
// did: the file said nothing, so the reader supplied the worse of the two
// explanations. Measured on a foreign corpus, this was read as a broken
// compile.
//
// So the zero is said out loud, with the counts and the destination, and only
// when the claim is true: an empty region with nothing in the index behind it
// gets no note, because there would be nothing honest to say.
// deliveryNoteFormat is the note's wording, hoisted to a named constant so it
// can be HASHED into the topic fingerprint (see noteEmitterFingerprint).
//
// It cites NO edikt-internal ADR. This string renders into a USER's compiled
// governance files, where "(ADR-066)" would name an artifact in edikt's own  edikt-guard:allow
// corpus that the reader has no access to and whose number, in their repo,
// belongs to a different decision entirely. test/check-no-internal-refs.sh
// enforces this, and caught exactly that leak here. The rationale lives in the
// comments above, which do not ship.
//
// It is a constant for the same reason RendererFingerprint hashes the template
// sources rather than trusting the caller: a cache keyed on a topic's sidecars
// cannot see that the emitter above it changed, so an edited note would render
// for new topics and never propagate to the cached ones — "6 unchanged", a
// true statement about the corpus and a false one about the output. That is
// precisely the defect RendererFingerprint exists to prevent, and putting this
// wording in a Go string instead of a template moved it back out of reach.
// Measured: a one-word grammar fix here propagated to zero of six topic files
// until this constant was folded into the fingerprint.
const deliveryNoteFormat = "**No unscoped directives in this topic — the region below is empty by design, " +
	"not by a failed compile.** All %s and %s in this topic come from %s " +
	"declaring `paths:`, so they are delivered at write time from `%s` — matched per " +
	"directive against the file you are actually touching — instead of being " +
	"duplicated here. The reminders and verification checklist below are " +
	"this file's own content."

func deliveryNoteFor(g *topicGroup, directiveLines []string) string {
	if len(directiveLines) > 0 {
		return ""
	}
	total := g.IndexedDirectives + g.IndexedProhibitions
	if total == 0 {
		return ""
	}
	return blockquote(fmt.Sprintf(
		deliveryNoteFormat,
		plural(g.IndexedDirectives, "directive", "directives"),
		plural(g.IndexedProhibitions, "prohibition", "prohibitions"),
		plural(len(g.IndexedSources), "source", "sources"),
		directiveIndexName))
}

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
	h.Write([]byte("delivery-note-format\x00"))
	h.Write([]byte(deliveryNoteFormat))
	h.Write([]byte("\x00directive-index-name\x00"))
	h.Write([]byte(directiveIndexName))
	h.Write([]byte("\x00note-wrap-width\x00"))
	fmt.Fprintf(h, "%d", noteWrapWidth)
	return hex.EncodeToString(h.Sum(nil))
}

// noteWrapWidth is the column the generated prose notes wrap at, matching the
// hand-written wrapping already used in the skill-package bodies.
const noteWrapWidth = 78

// blockquote wraps text as a markdown blockquote.
func blockquote(text string) string { return wrapText(text, "> ") }

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

// plural renders "1 directive" / "3 directives" — a count and its noun,
// agreeing. Written out rather than left as "1 directive(s)" because these
// notes are read by a person deciding whether their governance is broken, and
// a rendered surface that reads like a debug print invites the same doubt the
// note exists to remove.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
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
				return fmt.Errorf("%s: directives[%d] %q: verify_kind is behavioral but positive_fixture_path is empty — add a positive fixture script per Plan C", p.ArtifactID, i, d.Text)
			}
			if d.VerifyKind == "behavioral" && d.NegativeFixturePath == "" {
				return fmt.Errorf("%s: directives[%d] %q: verify_kind is behavioral but negative_fixture_path is empty — add a negative fixture script per Plan C", p.ArtifactID, i, d.Text)
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
				return fmt.Errorf("%s: prohibitions[%d] %q: verify_kind is behavioral but positive_fixture_path is empty — add a positive fixture script per Plan C", p.ArtifactID, i, pr.Text)
			}
			if pr.VerifyKind == "behavioral" && pr.NegativeFixturePath == "" {
				return fmt.Errorf("%s: prohibitions[%d] %q: verify_kind is behavioral but negative_fixture_path is empty — add a negative fixture script per Plan C", p.ArtifactID, i, pr.Text)
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
		if strings.HasPrefix(p.ArtifactID, "INV-") && len(p.Sidecar.Paths) == 0 {
			suppressed := make(map[string]struct{}, len(p.Sidecar.SuppressedDirectives))
			for _, sd := range p.Sidecar.SuppressedDirectives {
				suppressed[sd] = struct{}{}
			}
			for _, d := range p.Sidecar.Directives {
				if _, isSuppressed := suppressed[d.Text]; isSuppressed {
					continue
				}
				invariantRules = append(invariantRules, compile.Rule{Text: d.Text, Source: p.ArtifactID})
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

		// Track directive count for the header comment (topic-file directives only;
		// invariant rules in governance.md are counted separately).
		res.TotalDirectives += len(g.Directives)

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
			// Says where the directives went when they did not come here.
			// Empty string for every topic that renders its own directives,
			// so the common case is byte-identical to before.
			DeliveryNote:    deliveryNoteFor(g, dirLines),
			DirectivesSHA:   regionSHA(dirLines, false),
			ProhibitionsSHA: regionSHA(prohLines, true),
			ManualSHA:       regionSHA(manLines, false),
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
		TopicIndex:        topicIndexRows(topicNames, opts.TopicDescriptions),
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
	dirIndexPath := filepath.Join(opts.OutDir, "directive-index.yaml")
	idxBody := RenderDirectiveIndex(BuildDirectiveIndex(pairs))
	dirIdxChanged, err := writeAtomicIfChanged(dirIndexPath, idxBody)
	if err != nil {
		return nil, fmt.Errorf("write directive index: %w", err)
	}
	res.DirectiveIndexWritten = dirIdxChanged

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
			Topic:        name,
			Description:  opts.TopicDescriptions[name],
			Directives:   g.Directives,
			Prohibitions: g.Prohibitions,
			Verification: dedupeStrings(topicVerification[name]),
			Reminders:    reminders,
			Sources:      g.Sources,
			// What the index took, so the stub can point honestly. Computed
			// for every topic, used only by the stub branch — a retired topic
			// renders its own bodies and has no pointer to qualify.
			IndexedDirectives:   g.IndexedDirectives,
			IndexedProhibitions: g.IndexedProhibitions,
			IndexedSources:      g.IndexedSources,
			DirectiveIndexPath:  relToRoot(projectRoot, dirIndexPath),
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
	res.Surfaces = append(res.Surfaces, Surface{
		Kind: "directive-index",
		Path: relToRoot(projectRoot, dirIndexPath),
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
	case "topic-file", "skill-package", "directive-index":
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
			idxDirs, idxProhs := IndexEntriesFor(p)
			if len(idxDirs)+len(idxProhs) > 0 {
				t.IndexedDirectives += len(idxDirs)
				t.IndexedProhibitions += len(idxProhs)
				t.IndexedSources = appendUnique(t.IndexedSources, p.ArtifactID)
			}
			for _, m := range p.Sidecar.ManualDirectives {
				t.Manual = append(t.Manual, manualEntry{Text: m, Source: p.ArtifactID})
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
		sort.Strings(t.IndexedSources)
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
		data, err := sidecar.Marshal(s)
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
func topicIndexRows(topicNames []string, descriptions map[string]string) []render.TopicIndexRow {
	rows := make([]render.TopicIndexRow, 0, len(topicNames))
	for _, name := range topicNames {
		rows = append(rows, render.TopicIndexRow{
			Topic:       name,
			Description: descriptions[name],
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

// regionSHA returns the sha256 of the rendered body of a managed region.
// The body is the concatenation of each bullet line (`- ` + text + `\n`).
// withProhibitionsHeading prepends `## Prohibitions\n` so the SHA covers
// the heading line embedded in the prohibitions region.
func regionSHA(lines []string, withProhibitionsHeading bool) string {
	var b strings.Builder
	if withProhibitionsHeading {
		b.WriteString("## Prohibitions\n")
	}
	for _, l := range lines {
		b.WriteString("- ")
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return render.RegionSHA(b.String())
}

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
