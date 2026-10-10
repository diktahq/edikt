package phaseb

// deliverymap.go — the ROUTING TABLE, and the single place that decides which
// surface a directive is delivered by.
//
// WHY THIS EXISTS
//
// ADR-079 made delivery two-surfaced: MUST-grade is injected at write time from
// directive-index.yaml, advisory renders into its topic file. Both are correct,
// and together they broke every instrument that had been answering "does this
// directive reach whoever edits this file?" by asking the INDEX — because the
// index stopped being the whole answer.
//
// Two gates caught it immediately and were right to: check-review-coverage-
// equivalence (790 -> 238) and check-directive-scope-coverage (28 -> 37 gaps).
// Neither baseline was lowered. The instrument was wrong, not the corpus, and
// this file is the instrument.
//
// TWO QUESTIONS, NOT ONE. The conflation that broke them is worth naming,
// because it will recur otherwise:
//
//	"what does the write-time hook INJECT for this path?"  -> hookmatch.Match
//	"what GOVERNS this path, on any surface?"              -> hookmatch.MatchGoverning
//
// They had one answer while there was one surface. They now have two, and the
// names say which is which.
//
// IT CARRIES NO DIRECTIVE TEXT, deliberately. A routing table that duplicated
// the text would be a second copy able to drift from the surface that actually
// delivers it — and would re-create, in a file nobody reads, exactly the
// double-delivery ADR-079 removed. Its consumers need identity, grade and
// destination; both gates were verified to consume no text before this was
// written (gov scope's only consumer reads `directives_in_scope`, a count).
//
// So this is NOT a delivery surface. Nothing loads it into a model's context.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// The surfaces a directive can be delivered by.
//
// SurfaceUnrouted is not a filler value — it is the finding. A directive that
// reaches no surface is compiled, counted in every coverage number, and read by
// nobody, which is the shape INV-013 exists to make impossible to mistake for a
// clean result. It is recorded here so it can be counted rather than inferred
// from an absence.
//
// No row of RouteOf produces it any more (see actorScope above). It stays in
// the vocabulary because delivery-map.yaml files rendered before 2026-09-02
// carry it, and because the day a future edit reintroduces a row that returns
// it, the word "unrouted" in a routing table should already mean something.
const (
	SurfaceTopicFile = "topic-file" // .claude/rules/governance/<topic>.md, loads on a paths: match
	SurfaceSkill     = "skill"      // .claude/skills/edikt-<topic>/SKILL.md, judgement-routed
	SurfaceUnrouted  = "unrouted"   // reaches nothing — reported, never silently dropped
)

// RouteOf is THE routing decision. Every renderer that includes or excludes a
// directive consults this function, so a surface cannot quietly disagree with
// the table that claims to describe it.
//
// The three inputs are the three things that actually decide destination:
//
//	grade       — ADR-079: must is a guardrail that must interrupt a write;
//	              advisory is context that must arrive before the model chooses.
//	actorScope  — ADR-065: an actor-scoped directive constrains an edikt-internal
//	              operation, not file content a human edit could violate, so it is
//	              excluded from write-time injection — and ONLY from there.
//	              ADR-065's decision text: "Excluded directives are not deleted or
//	              demoted. They still compile, still render in the ambient/topic-
//	              file surfaces … and still bind in every sense except one: they
//	              stop firing as write-time PreToolUse bounces." So an actor-scoped
//	              MUST takes the same home as advisory — its topic file, or its
//	              skill when the topic is tier 3 — because the one channel that
//	              cannot carry it is the index, and a directive read by judgement
//	              is still read.
//
//	              This table used to send actor-scoped MUST to SurfaceUnrouted and
//	              pin that as "ADR-065's asymmetry". It was not ADR-065's; it was
//	              ADR-079's implementation reading "MUST stays in the index" as
//	              "MUST renders nowhere else", which left every actor-scoped MUST
//	              with no surface at all. Measured at 7 of INV-010's 9 directives
//	              — the compile pipeline's own read-only obligations — reaching
//	              nothing, on a corpus whose delivery gate was green (the
//	              2026-09-01 delivery assertion is what found them).
//	topicTier3  — SPEC-011 stage 1: a topic whose contributing sidecars declare
//	              no globs has no rules file, so its advisory lives in the skill
//	              package and is reached by judgement rather than by a path match.
//
// RouteOf names the surface that delivers a directive. Since ADR-083 there
// are two: the topic file (loaded when a path matches) and the skill package
// (judgement-routed, for a topic with no scoped rules). The write-time
// surface — directive-index.yaml, injected by PreToolUse for must-grade  edikt-guard:allow
// path-scoped directives under ADR-066 — is retired; its 482 entries on this  edikt-guard:allow
// corpus are re-homed into their topic files by this rule returning the topic
// surface for them (PLAN-v090-release Phase 0, AC-0.3). Grade keeps deciding
// which surface holds a rule among the surfaces that remain (ADR-083 h): the  edikt-guard:allow
// grade travels on the delivery-map entry, and the phase-end verifier reads
// it there.
func RouteOf(grade string, actorScope bool, topicTier3 bool) string {
	_ = grade
	_ = actorScope
	if topicTier3 {
		return SurfaceSkill
	}
	return SurfaceTopicFile
}

// DeliveryEntry is one directive's routing record. No Text field — see header.
type DeliveryEntry struct {
	ID      string
	Grade   string
	Class   string
	Topic   string
	Surface string
}

// BuildDeliveryMap keys every SCOPED directive by each glob it resolves to,
// exactly as BuildDirectiveIndex does, so the two are matched with the same
// globs by the same matcher and a path cannot resolve differently between them.
//
// tier3 reports whether a topic was retired to its skill package. It is passed
// in rather than recomputed because merge.go owns that decision; recomputing it
// here would be a second implementation of a rule that already has one, which
// is the defect this file was written to remove.
func BuildDeliveryMap(pairs []sidecar.Pair, tier3 map[string]bool) map[string][]DeliveryEntry {
	out := map[string][]DeliveryEntry{}

	sorted := append([]sidecar.Pair(nil), pairs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ArtifactID < sorted[j].ArtifactID })

	for _, p := range sorted {
		sc := p.Sidecar
		if sc == nil || p.Skip || len(sc.Paths) == 0 {
			continue
		}
		suppressed := make(map[string]struct{}, len(sc.SuppressedDirectives))
		for _, sd := range sc.SuppressedDirectives {
			suppressed[sd] = struct{}{}
		}
		class := classFor(p.ArtifactID)
		isT3 := tier3[sc.Topic]

		add := func(id string, grade string, actorScope bool, paths []string) {
			e := DeliveryEntry{
				ID:      id,
				Grade:   grade,
				Class:   class,
				Topic:   sc.Topic,
				Surface: RouteOf(grade, actorScope, isT3),
			}
			globs := append([]string(nil), resolvePaths(paths, sc.Paths)...)
			sort.Strings(globs)
			for _, g := range globs {
				out[g] = append(out[g], e)
			}
		}

		for i, d := range sc.Directives {
			if _, skip := suppressed[d.Text]; skip {
				continue
			}
			add(entryID(p.ArtifactID, 'd', i+1), gradeFor(class, false, d.Enforcement), d.ActorScope, d.Paths)
		}
		for i, ph := range sc.Prohibitions {
			add(entryID(p.ArtifactID, 'p', i+1), gradeFor(class, true, ph.Enforcement), ph.ActorScope, ph.Paths)
		}
	}
	return out
}

// deliveryMapName is the routing table's filename inside the governance dir.
const deliveryMapName = "delivery-map.yaml"

// RenderDeliveryMap emits the map as deterministic YAML — hand-written for the
// same reason RenderDirectiveIndex is: marshalling a Go map randomises key
// order, and a surface whose bytes move on identical input cannot be hashed
// into a manifest.
func RenderDeliveryMap(m map[string][]DeliveryEntry) string {
	var b strings.Builder
	b.WriteString("# edikt delivery map — generated by gov-compile, do not edit manually.\n")
	b.WriteString("# The ROUTING TABLE: which directives govern a path, and which surface\n")
	b.WriteString("# delivers each one. NOT a delivery surface — nothing loads this into a\n")
	b.WriteString("# model's context, and it deliberately carries no directive text.\n")
	b.WriteString("#\n")
	b.WriteString("# Read by `edikt hook match`, `edikt gov scope`, the session tally\n")
	b.WriteString("# and the prune counter. It is the ONE source of path->directive scope: the\n")
	b.WriteString("# write-time index it once sat beside is retired, and nothing injects.\n")
	b.WriteString("schema_version: 1\n")
	b.WriteString("globs:\n")

	globs := make([]string, 0, len(m))
	for g := range m {
		globs = append(globs, g)
	}
	sort.Strings(globs)
	for _, g := range globs {
		entries := append([]DeliveryEntry(nil), m[g]...)
		sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
		fmt.Fprintf(&b, "  %q:\n", g)
		for _, e := range entries {
			fmt.Fprintf(&b, "    - id: %s\n", e.ID)
			fmt.Fprintf(&b, "      grade: %s\n", e.Grade)
			fmt.Fprintf(&b, "      class: %s\n", e.Class)
			fmt.Fprintf(&b, "      topic: %s\n", e.Topic)
			fmt.Fprintf(&b, "      surface: %s\n", e.Surface)
		}
	}
	return b.String()
}
