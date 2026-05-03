// Package phaseb implements the deterministic merge phase of two-phase
// gov:compile per ADR-028. It reads the validated sidecar set, groups
// directives by topic, and writes .claude/rules/governance/<topic>.md and
// .claude/rules/governance.md.
//
// PURITY CONTRACT (ADR-028 §"Phase B"). This package MUST NOT import any
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
}

// Options configures merge output.
type Options struct {
	OutDir          string // default .claude/rules/governance
	IndexPath       string // default .claude/rules/governance.md
	CompiledAt      string // ISO 8601; pinned for deterministic test runs
	CompilerVersion string
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
	Paths        []string
	Sources      []string
	Signals      []string
}

// manualEntry pairs a manual_directive's text with the artifact ID of the
// sidecar that authored it, so render can sort by that ref tag for
// determinism even when the entry's text does not embed `(ref: …)`.
type manualEntry struct {
	Text   string
	Source string
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

	// Collect global aggregates in artifact-ID order for determinism.
	// INV directives go ONLY to governance.md constraints — never to topic
	// files. Reminders and verification are aggregated across all artifacts.
	sortedPairs := append([]sidecar.Pair(nil), pairs...)
	sort.Slice(sortedPairs, func(i, j int) bool {
		return sortedPairs[i].ArtifactID < sortedPairs[j].ArtifactID
	})
	var invariantRules []compile.Rule
	var allReminders, allVerification []string
	for _, p := range sortedPairs {
		if p.Sidecar == nil {
			continue
		}
		if strings.HasPrefix(p.ArtifactID, "INV-") {
			for _, d := range p.Sidecar.Directives {
				invariantRules = append(invariantRules, compile.Rule{Text: d.Text, Source: p.ArtifactID})
			}
		}
		allReminders = append(allReminders, p.Sidecar.Reminders...)
		allVerification = append(allVerification, p.Sidecar.Verification...)
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
		g := groups[name]
		if len(g.Directives) > 0 || len(g.Manual) > 0 || len(g.Prohibitions) > 0 {
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
		fp := TopicFingerprint(g.Sidecars)
		dest := filepath.Join(opts.OutDir, name+".md")

		// Track directive count for the header comment (topic-file directives only;
		// invariant rules in governance.md are counted separately).
		res.TotalDirectives += len(g.Directives)

		// Diff-only short-circuit: if the existing topic file declares the
		// same fingerprint, every contributing sidecar is byte-equal to last
		// run; skip the render to keep mtime stable and the file untouched.
		// Bust the cache when the on-disk file lacks one of the three
		// Phase 8 managed regions (bootstrap-write semantics: a v0.6.0-rc4
		// shaped file gets the new prohibitions/manual anchors on first
		// post-upgrade compile).
		if existingFP, ok := readTopicFingerprint(dest); ok && existingFP == fp && hasAllRegions(dest) {
			res.TopicsUnchanged = append(res.TopicsUnchanged, name)
			continue
		}

		dirLines, prohLines, manLines := buildRegionLines(g)
		body, err := render.RenderTopic(render.TopicView{
			Name:             name,
			Paths:            g.Paths,
			Sources:          g.Sources,
			Rules:            g.Directives,
			CompiledAt:       opts.CompiledAt,
			CompilerVersion:  opts.CompilerVersion,
			Fingerprint:      fp,
			DirectiveLines:   dirLines,
			ProhibitionLines: prohLines,
			ManualLines:      manLines,
			DirectivesSHA:    regionSHA(dirLines, false),
			ProhibitionsSHA:  regionSHA(prohLines, true),
			ManualSHA:        regionSHA(manLines, false),
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
		ADRCount:          countByPrefix(pairs, "ADR-"),
		ADRAcceptedCount:  countByPrefix(pairs, "ADR-"),
		INVCount:          countByPrefix(pairs, "INV-"),
		INVActiveCount:    countByPrefix(pairs, "INV-"),
		GuidelineCount:    countGuidelines(pairs),
		DirectiveCount:    res.TotalDirectives + len(invariantRules),
		TopicCount:        len(topicNames),
		InvariantRules:    invariantRules,
		InvariantRestated: append([]compile.Rule(nil), invariantRules...),
		RoutingRows:       routingRows(groups, topicNames),
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
	return res, nil
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
		t.Paths = appendUnique(t.Paths, p.Sidecar.Path)
		t.Sources = appendUnique(t.Sources, p.ArtifactID)
		for _, s := range p.Sidecar.Signals {
			t.Signals = appendUnique(t.Signals, s)
		}
		// INV directives go only to governance.md constraints; topic files
		// contain ADR and guideline directives only.
		if !strings.HasPrefix(p.ArtifactID, "INV-") {
			for _, d := range p.Sidecar.Directives {
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
		sort.Strings(t.Sources)
	}
	return g
}

func routingRows(groups map[string]*topicGroup, topicNames []string) []render.RoutingRow {
	rows := make([]render.RoutingRow, 0, len(topicNames))
	for _, name := range topicNames {
		t := groups[name]
		signals := append([]string(nil), t.Signals...)
		sort.Strings(signals)
		rows = append(rows, render.RoutingRow{
			Signals: strings.Join(signals, ", "),
			Scope:   "implementation, review",
			File:    "governance/" + name + ".md",
		})
	}
	return rows
}

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

func countByPrefix(pairs []sidecar.Pair, prefix string) int {
	n := 0
	for _, p := range pairs {
		if strings.HasPrefix(p.ArtifactID, prefix) {
			n++
		}
	}
	return n
}

func countGuidelines(pairs []sidecar.Pair) int {
	n := 0
	for _, p := range pairs {
		if !strings.HasPrefix(p.ArtifactID, "ADR-") && !strings.HasPrefix(p.ArtifactID, "INV-") {
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
//   - Directives: extracted directives interleaved with manual_directives,
//     sorted by ref-tag (asc), then manual-flag (extracted before manual on
//     equal ref tag), then text. Manual entries get the
//     ` (ref: <ID> + manual) *(manual)*` annotation; an entry whose own
//     text already carries `(ref:` keeps the verbatim text and just gets the
//     `*(manual)*` marker appended.
//   - Prohibitions: text-only bullets sorted by text asc.
//   - Manual: text-only bullets sorted by text asc — a faithful copy that
//     downstream tooling can key on independently of the directives region.
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

// regionMarker matches the start/end sentinels of one Phase 8 managed
// region. Captures (kind, position): kind ∈ {directives, prohibitions,
// manual}, position ∈ {start, end}.
var regionMarker = regexp.MustCompile(`(?m)^\[edikt:(directives|prohibitions|manual):(start|end)\]: #$`)

// assertNoRegionOverlap enforces INV-005 byte-range integrity: the three
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
				return fmt.Errorf("INV-005 violation: duplicate %q start sentinel in %s", kind, topicName)
			}
			open[kind] = m[0]
		} else {
			startOff, ok := open[kind]
			if !ok {
				return fmt.Errorf("INV-005 violation: orphan %q end sentinel in %s", kind, topicName)
			}
			delete(open, kind)
			spans = append(spans, span{kind: kind, start: startOff, end: m[1]})
		}
	}
	for k := range open {
		return fmt.Errorf("INV-005 violation: unclosed %q region in %s", k, topicName)
	}
	for i := 0; i < len(spans); i++ {
		for j := i + 1; j < len(spans); j++ {
			if spans[i].start < spans[j].end && spans[j].start < spans[i].end {
				return fmt.Errorf("INV-005 violation: regions %s and %s overlap in %s", spans[i].kind, spans[j].kind, topicName)
			}
		}
	}
	return nil
}
