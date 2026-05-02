// Package sidecar loads, validates, and reasons about <artifact>.edikt.yaml
// sidecar files per ADR-027 (sidecar architecture, supersedes ADR-008) and
// templates/schemas/sidecar.schema.json (v1).
package sidecar

import (
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

	SourcePath string `yaml:"-"`
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
func Load(sidecarPath string) (*Sidecar, error) {
	f, err := os.Open(sidecarPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", sidecarPath, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var s Sidecar
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", sidecarPath, err)
	}
	s.SourcePath = sidecarPath
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", sidecarPath, err)
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
		if len(d.Text) > 200 {
			return fmt.Errorf("directives[%d].text: %d chars, max 200", i, len(d.Text))
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
