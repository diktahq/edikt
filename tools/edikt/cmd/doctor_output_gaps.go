// doctor_output_gaps.go — the fifth and sixth delivery checks, read from the
// COMPILED OUTPUT rather than from the approval queue.
//
// WHY A SEPARATE FILE, AND NOT A FIFTH ENTRY IN doctor_delivery_gaps.go.
//
// The four checks there and the two here answer different questions, and the
// difference is the whole point:
//
//	queue-based   "is there approval work outstanding?"   reads .edikt/state/
//	output-based  "is a shipped surface degraded?"        reads .claude/
//
// Those coincide only while compile keeps the queue seeded. govrun/proposals.go
// does open an empty slot for every topic with no approved entry and no pending
// proposal, so in steady state a degraded skill implies a queue entry — but
// that is a COMPILE-TIME BEHAVIOUR, not an invariant. A queue emptied by hand,
// a slot deleted, or a surface left behind by an older binary all break the
// coupling, and then the surface is degraded with nothing watching.
//
// A check that reads the queue to answer a question about the output is
// therefore trusting the mechanism it is supposed to be able to contradict.
// These two read the rendered artefacts and reference no queue state at all.
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/diktahq/edikt/tools/edikt/internal/parse"

	"gopkg.in/yaml.v3"
)

// fallbackDescriptionMarker is the stable substring of the placeholder
// phaseb.RenderSkill emits when a topic has no approved registry description.
//
// Deliberately a SUBSTRING and not a copy of the format string: duplicating the
// full sentence would be a second definition to drift. A test asserts that
// phaseb's rendered output still contains this marker, so the coupling is
// checked rather than assumed (INV-014 — assert the resolved outcome, do not
// restate the logic that produces it).
const fallbackDescriptionMarker = "no approved registry description"

// runOutputGapsCheck reports degraded compiled surfaces.
//
// ran=false when there is no compiled output to judge. An unmeasured surface is
// not a passing one (INV-013).
func runOutputGapsCheck(projectRoot string, w io.Writer) (warns int, ran bool) {
	skillsDir := filepath.Join(projectRoot, ".claude", "skills")
	indexPath := filepath.Join(projectRoot, ".claude", "rules", "governance", "delivery-map.yaml")
	_, skillsErr := os.Stat(skillsDir)
	_, indexErr := os.Stat(indexPath)
	if skillsErr != nil && indexErr != nil {
		return 0, false
	}
	cfg, err := govrun.LoadConfig(projectRoot)
	if err != nil {
		return 0, false
	}

	var gaps []deliveryGap
	gaps = append(gaps, skillsShippingFallbackDescription(skillsDir)...)
	gaps = append(gaps, artifactsReachingNoGlob(projectRoot, cfg, indexPath)...)
	gaps = append(gaps, advisoryDespiteAuthorIntent(projectRoot, cfg, indexPath)...)

	fmt.Fprintln(w, "  ── Compiled Surface ───────────────────────────")
	if len(gaps) == 0 {
		fmt.Fprintln(w, "    ok   every skill carries an approved description; "+
			"every routed artifact reaches a glob")
		return 0, true
	}
	for _, g := range gaps {
		fmt.Fprintf(w, "    warn %s\n", g.Title)
		for _, d := range g.Detail {
			fmt.Fprintf(w, "           %s\n", d)
		}
		fmt.Fprintf(w, "           → %s\n", g.Consequence)
	}
	return len(gaps), true
}

// skillsShippingFallbackDescription — the FIFTH check.
//
// `description:` is what the host matches a task against. When a topic has no
// approved registry entry the skill still ships, with the routing field spent
// on an instruction to run an admin command. The surface is not missing; it is
// present and pointing at edikt's own backlog, which is worse than absent
// because it looks like a description.
func skillsShippingFallbackDescription(skillsDir string) []deliveryGap {
	ents, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}
	var degraded []string
	total := 0
	for _, e := range ents {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "edikt-") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(skillsDir, e.Name(), "SKILL.md"))
		if err != nil {
			continue
		}
		total++
		if strings.Contains(string(body), fallbackDescriptionMarker) {
			degraded = append(degraded, strings.TrimPrefix(e.Name(), "edikt-"))
		}
	}
	if len(degraded) == 0 {
		return nil
	}
	sort.Strings(degraded)
	return []deliveryGap{{
		Title: fmt.Sprintf("%d of %d skill(s) ship a placeholder description", len(degraded), total),
		Detail: []string{
			strings.Join(degraded, ", "),
			"run: edikt sidecar approve --list",
		},
		// The consequence the queue-based check states INACCURATELY: it says the
		// topic renders with no description. It renders WITH one — an admin
		// instruction, in the field the host routes on.
		Consequence: "the skill still ships; its `description:` — what the host matches a task " +
			"against — carries an approval instruction instead of what the topic covers, " +
			"so the topic is discoverable only by accident",
	}}
}

// advisoryDespiteAuthorIntent — the SILENCE afcce21e identified.
//
// WHAT IT REPORTS. A directive on an ADR or guideline that (a) states a binding
// obligation, (b) sits on path globs a human explicitly approved, and (c) is
// advisory by class and so reaches no glob in the write-time index. On
// edikt-dev's own corpus that is 551 directives across 38 ADRs — 55.7% of the
// corpus and 71.8% of all accepted-ADR directives.
//
// WHY IT IS A CHECK AND NOT A FIX. d03e5ad7 established the routing: ADR-071
// makes grade class-weighted and ADR-079 makes grade the routing key, so
// INV -> must -> write-time index and ADR directive -> advisory -> topic file.
// afcce21e argued both sides and concluded the design is RIGHT — widening the
// deny channel by class would recreate the ratio SR-003 removed, when it found
// 112 of 238 write-time MUSTs were rejected options. What is wrong is that
// nothing tells the author. The sidecar looks identical, the compile succeeds,
// and the topic file renders the rule with its MUST intact. The only evidence
// is an absence in a file nobody reads, which is how this was found by accident
// on the fourth attempt.
//
// So this reports and names the remedy. It does not route anything.
//
// NOT A DEFECT PER DIRECTIVE. Most of the 551 are correctly advisory — an ADR
// records a decision, and "we chose Go over Rust" should not block a write. The
// finding is the SIZE of the silent set, and that an author has no way to learn
// their rule is in it.
func advisoryDespiteAuthorIntent(root string, cfg govrun.Config, indexPath string) []deliveryGap {
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		return nil
	}
	var idx struct {
		Globs map[string][]struct {
			ID string `yaml:"id"`
		} `yaml:"globs"`
	}
	if yaml.Unmarshal(raw, &idx) != nil {
		return nil
	}
	routed := map[string]bool{}
	for _, ents := range idx.Globs {
		for _, e := range ents {
			if id, ok := artifactIDFrom(e.ID); ok {
				routed[id] = true
			}
		}
	}

	count := 0
	arts := map[string]int{}
	for _, d := range govDirs(cfg) {
		ents, err := os.ReadDir(filepath.Join(root, d))
		if err != nil {
			continue
		}
		for _, e := range ents {
			n := e.Name()
			if e.IsDir() || !strings.HasSuffix(n, ".md") || !governedArtifactRe.MatchString(n) {
				continue
			}
			id, ok := artifactIDFrom(n)
			if !ok || routed[id] {
				continue
			}
			// Invariants route by class; only ADR/GL directives can be in this
			// set at all. Naming the prefixes rather than inferring from
			// routedness keeps the check honest if routing ever changes.
			if !strings.HasPrefix(id, "ADR-") && !strings.HasPrefix(id, "GL-") {
				continue
			}
			doc, derr := parse.LoadDocument(filepath.Join(root, d, n))
			if derr != nil || !doc.IsIncluded(kindForDir(d)) {
				continue
			}
			sraw, serr := os.ReadFile(filepath.Join(root, d, strings.TrimSuffix(n, ".md")+".edikt.yaml"))
			if serr != nil {
				continue
			}
			var sc struct {
				Paths         []string `yaml:"paths"`
				PathsApproval *struct {
					ApprovedAt string `yaml:"approved_at"`
				} `yaml:"paths_approval"`
				Directives []struct {
					Text string `yaml:"text"`
				} `yaml:"directives"`
				Prohibitions []struct {
					Text string `yaml:"text"`
				} `yaml:"prohibitions"`
			}
			if yaml.Unmarshal(sraw, &sc) != nil {
				continue
			}
			// APPROVED GLOBS ARE THE SECOND SIGNAL. A binding modal alone is
			// one claim; completing the path-approval ceremony is a human
			// answering a question. Both together are what makes the silence
			// surprising, so both are required here.
			if len(sc.Paths) == 0 || sc.PathsApproval == nil {
				continue
			}
			for _, t := range sc.Directives {
				if bindingModalRe.MatchString(t.Text) {
					count++
					arts[id]++
				}
			}
			for _, t := range sc.Prohibitions {
				if bindingModalRe.MatchString(t.Text) {
					count++
					arts[id]++
				}
			}
		}
	}
	if count == 0 {
		return nil
	}
	names := make([]string, 0, len(arts))
	for a := range arts {
		names = append(names, a)
	}
	sort.Strings(names)
	shown := names
	if len(shown) > 8 {
		shown = append(append([]string{}, shown[:8]...),
			fmt.Sprintf("... and %d more", len(names)-8))
	}
	return []deliveryGap{{
		Title: fmt.Sprintf("%d directive(s) on %d ADR/guideline artifact(s) state a binding obligation "+
			"on approved paths and cannot deny a write", count, len(arts)),
		Detail: append(shown,
			"advisory by class (ADR-071); grade is the routing key (ADR-079)",
			"remedy, per directive: set `enforcement: deny` on the ones that must block"),
		Consequence: "they are delivered by the topic file and skill package, so the rule is READ " +
			"but never enforced — most are correctly advisory, and nothing currently tells an " +
			"author which of theirs is in this set",
	}}
}

// bindingModalRe matches RFC-2119 binding force, the same set INV-020 requires
// of a hand-authored directive. Deliberately NOT the routing signal — ADR-071
// replaced modal-derived grade for a measured reason — it is only how this
// check infers what the AUTHOR intended.
var bindingModalRe = regexp.MustCompile(`\b(MUST NOT|SHALL NOT|MUST|SHALL|REQUIRED)\b`)

// artifactsReachingNoGlob — the SIXTH check.
//
// An artifact whose directives reach no glob in the delivery map is
// delivered by the ambient core and its skill package and by nothing else. The
// rule is not lost, but it cannot deny a write, so its enforcement grade
// differs from an identically-graded rule that routes — and nothing says so.
func artifactsReachingNoGlob(root string, cfg govrun.Config, indexPath string) []deliveryGap {
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		return nil
	}
	var idx struct {
		Globs map[string][]struct {
			ID string `yaml:"id"`
		} `yaml:"globs"`
	}
	if yaml.Unmarshal(raw, &idx) != nil {
		return nil
	}
	routed := map[string]bool{}
	for _, ents := range idx.Globs {
		for _, e := range ents {
			if id, ok := artifactIDFrom(e.ID); ok {
				routed[id] = true
			}
		}
	}
	// A completely empty index is the EMPTY-SURFACE case, already reported by
	// the queue-based emptyDeliverySurface check. Reporting every artifact here
	// as well would bury that one finding under a list of all of them.
	if len(routed) == 0 {
		return nil
	}

	var unrouted []string
	total := 0
	for _, d := range govDirs(cfg) {
		ents, err := os.ReadDir(filepath.Join(root, d))
		if err != nil {
			continue
		}
		for _, e := range ents {
			n := e.Name()
			if e.IsDir() || !strings.HasSuffix(n, ".md") || !governedArtifactRe.MatchString(n) {
				continue
			}
			doc, err := parse.LoadDocument(filepath.Join(root, d, n))
			if err != nil || !doc.IsIncluded(kindForDir(d)) {
				continue
			}
			id, ok := artifactIDFrom(n)
			if !ok {
				continue
			}
			total++
			if !routed[id] {
				unrouted = append(unrouted, id)
			}
		}
	}
	if len(unrouted) == 0 {
		return nil
	}
	sort.Strings(unrouted)
	shown := unrouted
	if len(shown) > 10 {
		shown = append(append([]string{}, shown[:10]...),
			fmt.Sprintf("... and %d more", len(unrouted)-10))
	}
	return []deliveryGap{{
		Title: fmt.Sprintf("%d of %d accepted artifact(s) reach no glob in the delivery map",
			len(unrouted), total),
		Detail: append(shown, "read: .claude/rules/governance/delivery-map.yaml"),
		Consequence: "their directives are delivered by the ambient core and skill packages only — " +
			"they cannot deny a write, so any measurement of the write-time channel " +
			"measures the routed subset, not the corpus",
	}}
}
