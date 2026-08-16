package phaseb

// skillpackage.go — surface (d) of the four-surface render.
//
// A skill package is `.claude/skills/edikt-<topic>/SKILL.md`: the registry
// description as frontmatter, and a body of topical guidance the host loads
// ON DEMAND rather than on every edit.
//
// WHERE IT SITS IN THE LADDER
//
//	tier 1  ambient      canonical statements of PATHLESS INVARIANTS + the topic index
//	tier 2  scoped rules topic files WITH real globs — load when you touch a covered file
//	tier 3  skills       everything else — loaded when the task calls for the topic
//
// A topic whose contributing sidecars declare no globs does NOT fall through
// to ambient. It falls HERE. That is the correction that makes the ambient
// budget reachable without first buying path coverage for the whole corpus.
//
// WHAT GOES IN THE BODY, AND WHAT MUST NOT
//
// Guidance only. Invariant-sourced MUST-grade entries stay OUT: they are
// already in the ambient core as canonical statements, and a rule that appears
// in two tiers is the double-loading AC-2.2 exists to forbid.
//
// Reminders DO belong here, and this is the second half of closing the
// reminder gap. The glob-keyed directive index can only carry reminders for
// artifacts that declared globs; a PATHLESS artifact's reminders have no key
// there. Without this surface those reminders have nowhere to go at all —
// measured at 32 across 11 artifacts on this corpus.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/compile"
)

// SkillView is one rendered skill package.
//
// PointsTo, when set, makes this a POINTER STUB: the topic is scoped to tier 2
// and that rules file owns the directive bodies. The stub carries the pinned
// description and the pointer, never a copy — see AC-2.2, which forbids a
// directive body appearing in more than one surface at all, not merely in two
// surfaces loaded at the same moment.
type SkillView struct {
	Topic       string
	Description string
	PointsTo    string
	Directives  []compile.Rule
	// Prohibitions and Verification are carried for the SAME reason the
	// directives are. A retired topic's skill is the ONLY home of its content,
	// and rendering only Directives dropped both silently: measured at 65
	// prohibitions and 145 verification items lost across the six retired
	// topics, found by the AC-2.7 coverage baseline rather than by any gate.
	//
	// A prohibition is not a lesser directive. "MUST NOT add a host-agent-CLI
	// abstraction inside tier-2" is the kind of rule someone breaks precisely
	// because they never saw it.
	Prohibitions []compile.Rule
	Verification []string
	Reminders    []string
	// Sources is every artifact contributing to this topic. It is NOT a claim
	// about where those artifacts' text ended up — see IndexedSources, which
	// is the half of this list whose bodies went to directive-index.yaml
	// instead of into the file PointsTo names. Rendering Sources alone under
	// a pointer that said "it carries every compiled directive" was a
	// measurable overclaim: on a fully scoped topic, NONE of the listed
	// artifacts had a body in the file being pointed at.
	Sources []string

	// IndexedDirectives / IndexedProhibitions / IndexedSources describe the
	// content this topic delivers through directive-index.yaml at write time
	// rather than through the topic file (ADR-066).  edikt-guard:allow
	//
	// A pointer stub has to know this. Its whole job is to tell a reader who
	// arrived by task language where the rules are, and "read that file, it
	// carries everything" is false for a topic whose directives are all
	// scoped — the file's Directives region is empty, and a reader who opens
	// it and finds nothing concludes governance is broken rather than that it
	// is waiting for the write.
	IndexedDirectives   int
	IndexedProhibitions int
	IndexedSources      []string
	// FileCarriesDirectives is whether the file PointsTo names actually has
	// directive or prohibition bodies of its own. Kept as a separate fact
	// rather than inferred from the counts above: a topic can be partly
	// scoped, and "some directives are in the file AND some are in the index"
	// is a third state that neither count expresses on its own.
	FileCarriesDirectives bool
	// DirectiveIndexPath is the repo-relative path of the index, so the stub
	// names a path the reader can actually open.
	DirectiveIndexPath string
}

// RenderSkill emits `.claude/skills/edikt-<topic>/SKILL.md`.
//
// No timestamp: this is a hashed surface like the others, and a diagnostic
// that changes on every no-op compile defeats the determinism the manifest
// depends on.
func RenderSkill(v SkillView) string {
	var b strings.Builder

	// Frontmatter `description` is what the host matches a task against, and
	// it is the registry's approved line VERBATIM — never re-derived here
	// (SSP-002). An absent description is stated as absent rather than
	// silently omitted, so a skill that cannot be routed to says why.
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: edikt-%s\n", v.Topic)
	if v.Description != "" {
		fmt.Fprintf(&b, "description: %s\n", yamlQuote(v.Description))
	} else {
		fmt.Fprintf(&b, "description: %s\n",
			yamlQuote(fmt.Sprintf("edikt governance for the %s topic (no approved registry description — run `bin/edikt sidecar approve --kind topic-description %s`)", v.Topic, v.Topic)))
	}
	b.WriteString("---\n")
	b.WriteString("<!-- edikt:compiled — generated by gov-compile, do not edit manually -->\n")
	fmt.Fprintf(&b, "<!-- sources: %s -->\n", strings.Join(v.Sources, ", "))
	// The `sources:` line above names every artifact that CONTRIBUTES to the
	// topic. On its own it was read as "these artifacts' text is in the file
	// this package points at" — which is false for any artifact ADR-066  edikt-guard:allow
	// routed to the index. Naming the routed subset on its own line removes
	// the implication without shortening `sources:`, which readers and
	// tooling use to answer the different question of what the topic is
	// built from.
	if len(v.IndexedSources) > 0 {
		// A retired (tier-3) topic has no topic file to disclaim — its rules
		// file was removed and this package IS the surface. Saying "not
		// carried in the topic file" there would name a file that does not
		// exist, which is its own species of the misdirection this line was
		// added to end.
		carrier := "in this package"
		if v.PointsTo != "" {
			carrier = "in the topic file"
		}
		fmt.Fprintf(&b, "<!-- delivered via %s, not %s: %s -->\n",
			v.indexPath(), carrier, strings.Join(v.IndexedSources, ", "))
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "# %s\n\n", titleForTopic(v.Topic))
	if v.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", v.Description)
	}

	// POINTER STUB. Deliberately short and deliberately imperative: the whole
	// value of this surface is that a reader who arrived by task language
	// rather than by touching a file follows the pointer. A stub that merely
	// mentions a path is a bibliography; this one states the action.
	if v.PointsTo != "" {
		fmt.Fprintf(&b, "The rules for this topic live in `%s`.\n\n", v.PointsTo)
		b.WriteString("**Read that file now, before you continue.** It loads automatically once you\n")
		b.WriteString("touch a file it covers — but if you are still deciding an approach, nothing has\n")
		b.WriteString("been touched yet and it has not loaded.\n\n")

		// WHAT THE FILE ACTUALLY CARRIES, in the three states it can be in.
		//
		// The single unconditional sentence this replaced — "It carries every
		// compiled directive and prohibition for <topic>" — was true only
		// before ADR-066 moved scoped bodies to the index. After it, a fully  edikt-guard:allow
		// scoped topic's rules file has an EMPTY Directives region, and the
		// stub was sending readers to it with a promise the file could not
		// keep. Pointing somebody at an empty file and telling them it holds
		// everything is worse than not pointing at all: they conclude the
		// compile is broken, or that the topic has no rules.
		indexed := v.IndexedDirectives + v.IndexedProhibitions
		var claim string
		switch {
		case indexed == 0:
			claim = fmt.Sprintf(
				"It carries every compiled directive and prohibition for **%s**. They are "+
					"not repeated here: one directive has exactly one home, so there is no "+
					"second copy that could drift out of step with it.", v.Topic)
		case v.FileCarriesDirectives:
			claim = fmt.Sprintf(
				"It carries this topic's unscoped directives and prohibitions, plus every "+
					"reminder and verification item for **%s**. A further %s and %s are "+
					"path-scoped: those are delivered at write time from `%s`, matched per "+
					"directive against the file you are actually touching, rather than "+
					"written into either file. Nothing is repeated in this "+
					"package: one directive has exactly one home, so there is no second "+
					"copy that could drift out of step with it.",
				v.Topic,
				plural(v.IndexedDirectives, "directive", "directives"),
				plural(v.IndexedProhibitions, "prohibition", "prohibitions"),
				v.indexPath())
		default:
			claim = fmt.Sprintf(
				"What that file carries is this topic's reminders and verification "+
					"checklist. Its compiled-directive regions are EMPTY BY DESIGN: all %s "+
					"and %s for **%s** are path-scoped, so they are delivered at write time "+
					"from `%s`, matched per directive against the file you are actually "+
					"touching. An empty region there means routed, not missing — "+
					"and nothing is repeated in this package, because one directive has "+
					"exactly one home.",
				plural(v.IndexedDirectives, "directive", "directives"),
				plural(v.IndexedProhibitions, "prohibition", "prohibitions"),
				v.Topic,
				v.indexPath())
		}
		b.WriteString(wrapText(claim, ""))
		b.WriteString("\n\n")

		b.WriteString("Non-negotiable constraints are not repeated here either — they are always loaded.\n")
		return b.String()
	}

	if len(v.Directives) == 0 {
		// A measured zero, said out loud. An empty skill body and a skill that
		// failed to render must not look the same to a reader (INV-013).  edikt-guard:allow
		b.WriteString("_No topical guidance compiled for this topic._\n")
	} else {
		b.WriteString("## Guidance\n\n")
		for _, r := range v.Directives {
			fmt.Fprintf(&b, "- %s\n", r.Text)
		}
		b.WriteString("\n")
	}

	if len(v.Prohibitions) > 0 {
		b.WriteString("## Prohibitions\n\n")
		for _, r := range v.Prohibitions {
			fmt.Fprintf(&b, "- %s\n", r.Text)
		}
		b.WriteString("\n")
	}

	if len(v.Reminders) > 0 {
		b.WriteString("## Before you act\n\n")
		for _, r := range v.Reminders {
			fmt.Fprintf(&b, "- %s\n", r)
		}
		b.WriteString("\n")
	}

	if len(v.Verification) > 0 {
		b.WriteString("## Verification Checklist\n\n")
		for _, x := range v.Verification {
			fmt.Fprintf(&b, "- [ ] %s\n", x)
		}
		b.WriteString("\n")
	}

	b.WriteString("Non-negotiable constraints are not repeated here — they are always loaded.\n")
	return b.String()
}

// indexPath is the path the stub names for surface (c), falling back to the
// bare filename when the caller did not supply one. The fallback is never
// silently wrong: the index is always written beside the topic files, so the
// bare name resolves from the topic file's own directory either way.
func (v SkillView) indexPath() string {
	if v.DirectiveIndexPath != "" {
		return v.DirectiveIndexPath
	}
	return directiveIndexName
}

func titleForTopic(t string) string {
	parts := strings.Split(t, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// skillTopics returns the topics that should render a skill package, sorted.
//
// Every rendered topic gets one. A topic whose rules are already scoped still
// benefits: the skill is how someone reaches the guidance when they are
// THINKING about the topic rather than editing a file that happens to match.
func skillTopics(groups map[string]*topicGroup) []string {
	out := make([]string, 0, len(groups))
	for name := range groups {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
