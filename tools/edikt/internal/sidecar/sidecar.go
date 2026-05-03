// Package sidecar loads, validates, and reasons about <artifact>.edikt.yaml
// sidecar files per ADR-027 (sidecar architecture, supersedes ADR-008) and
// templates/schemas/sidecar.v1.schema.json (v1; the unversioned name was
// renamed in v0.6.0 per Phase 5 of PLAN-sidecar-review-fixes #31).
package sidecar

import (
	"bufio"
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is the on-disk shape this package understands.
const SchemaVersion = 1

var (
	topicRe  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	signalRe = regexp.MustCompile(`^[a-z0-9][a-z0-9 _.-]*$`)
)

// Sidecar mirrors the v1 schema one-to-one. Unknown fields anywhere in the
// document are rejected by the strict decoder, so the forbidden top-level
// keys (source_hash, agent_prompt_version, directives_hash) raise a parse
// error rather than being silently dropped.
type Sidecar struct {
	SchemaVersion int         `yaml:"schema_version"`
	Topic         string      `yaml:"topic"`
	Path          string      `yaml:"path"`
	Signals       []string    `yaml:"signals"`
	Directives    []Directive `yaml:"directives"`

	// User-authored overrides preserved across sidecar regenerations.
	// ManualDirectives are always included in the effective rule set.
	// SuppressedDirectives are subtracted from Directives at gov:compile time.
	// Populated by migrate_sidecars from the legacy sentinel block on upgrade.
	ManualDirectives     []string `yaml:"manual_directives,omitempty"`
	SuppressedDirectives []string `yaml:"suppressed_directives,omitempty"`

	// Aggregated at gov:compile time into governance.md's ## Reminders and
	// ## Verification Checklist sections. Populated by sidecar-extractor from
	// ## Confirmation (ADRs) and ## Enforcement (INVs) sections.
	Reminders    []string `yaml:"reminders,omitempty"`
	Verification []string `yaml:"verification,omitempty"`

	SourcePath string `yaml:"-"`

	// cachedMarshal stores the canonical-form bytes computed once at
	// Load time so TopicFingerprint can hash without re-marshaling. Phase 7
	// of PLAN-sidecar-review-fixes #39 — Phase B's fingerprint loop runs
	// Marshal on every contributing sidecar across every topic, and the
	// canonical form is invariant to in-memory mutation patterns Phase B
	// does not perform (Phase B is read-only). The cache is invalidated
	// implicitly by being struct-local: a fresh Sidecar built outside Load
	// (e.g. migrate's planArtifact) has an empty cache and Marshal
	// computes fresh.
	//
	// Determinism contract: cachedMarshal MUST always equal a fresh
	// Marshal(s) call for any caller that has not mutated s. The
	// Marshal-cache test in marshal_test.go pins this byte-equality
	// across the full valid-fixture corpus.
	cachedMarshal []byte `yaml:"-"`
}

// Directive is one rule extracted from the parent .md.
type Directive struct {
	Text          string        `yaml:"text"`
	SourceExcerpt SourceExcerpt `yaml:"source_excerpt"`
}

// SourceExcerpt records the line range + verbatim quote in the parent.
type SourceExcerpt struct {
	LineStart int    `yaml:"line_start"`
	LineEnd   int    `yaml:"line_end"`
	Quote     string `yaml:"quote"`
}

// Load reads sidecarPath, strictly decodes it, and runs the v1 validators.
//
// The reader is buffered (Phase 7 of PLAN-sidecar-review-fixes #40); this
// is observable only as a small reduction in syscall count when Discover
// loads dozens of sidecars in sequence — yaml.NewDecoder's read pattern is
// otherwise reasonable on a raw *os.File but still benefits from the
// 4 KiB bufio default on small files.
func Load(sidecarPath string) (*Sidecar, error) {
	f, err := os.Open(sidecarPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", sidecarPath, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(bufio.NewReader(f))
	dec.KnownFields(true)

	var s Sidecar
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", sidecarPath, err)
	}
	s.SourcePath = sidecarPath
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", sidecarPath, err)
	}
	// Pre-compute the canonical-form bytes once. Marshal is a pure
	// function of s, so capturing the result here lets TopicFingerprint
	// short-circuit re-marshaling on every Phase B run. Use the
	// uncached encoder path explicitly — Marshal would just return an
	// empty cache and recurse here otherwise.
	if data, err := marshalUncached(&s); err == nil {
		s.cachedMarshal = data
	}
	return &s, nil
}

// Validate enforces schema constraints not captured by structural decode.
func (s *Sidecar) Validate() error {
	if s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version: got %d, want %d", s.SchemaVersion, SchemaVersion)
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
		if d.SourceExcerpt.LineStart < 1 {
			return fmt.Errorf("directives[%d].source_excerpt.line_start: %d, must be >= 1", i, d.SourceExcerpt.LineStart)
		}
		if d.SourceExcerpt.LineEnd < d.SourceExcerpt.LineStart {
			return fmt.Errorf("directives[%d].source_excerpt.line_end: %d < line_start %d", i, d.SourceExcerpt.LineEnd, d.SourceExcerpt.LineStart)
		}
		if d.SourceExcerpt.Quote == "" {
			return fmt.Errorf("directives[%d].source_excerpt.quote: required", i)
		}
	}
	return nil
}
