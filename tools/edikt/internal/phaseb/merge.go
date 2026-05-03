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
	Paths      []string
	Sources    []string
	Signals    []string
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
		// an empty directive list. Don't write an empty topic file.
		if len(groups[name].Directives) > 0 {
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
		if existingFP, ok := readTopicFingerprint(dest); ok && existingFP == fp {
			res.TopicsUnchanged = append(res.TopicsUnchanged, name)
			continue
		}

		body, err := render.RenderTopic(render.TopicView{
			Name:            name,
			Paths:           g.Paths,
			Sources:         g.Sources,
			Rules:           g.Directives,
			CompiledAt:      opts.CompiledAt,
			CompilerVersion: opts.CompilerVersion,
			Fingerprint:     fp,
		})
		if err != nil {
			return nil, fmt.Errorf("render topic %s: %w", name, err)
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
