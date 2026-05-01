package parse

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// knownSentinelLists is the set of sentinel keys whose values are YAML block
// sequences but whose items may contain `: ` characters that break yaml.v3.
// We extract these with a custom line parser instead of YAML.
var knownSentinelLists = map[string]bool{
	"directives": true, "manual_directives": true, "suppressed_directives": true,
	"paths": true, "scope": true, "reminders": true, "verification": true,
	"canonical_phrases": true,
}

// Sentinel is the parsed body of an edikt directive sentinel block between
// `[edikt:directives:start]: #` and `[edikt:directives:end]: #`.
type Sentinel struct {
	Present bool

	// Hashes written by the per-artifact compile step (ADR-008).
	SourceHash     string `yaml:"source_hash,omitempty"`
	DirectivesHash string `yaml:"directives_hash,omitempty"`
	CompilerVersion string `yaml:"compiler_version,omitempty"`

	// Group assignment (optional per ADR-020; required in v0.7.0).
	// Topics holds all topic names (one or more). Topic is derived as the
	// first element and kept for callers that only need a single value.
	Topics []string `yaml:"topics,omitempty"`
	Topic  string   `yaml:"topic,omitempty"` // derived: Topics[0] after parse

	// Grouping metadata copied from the source sentinel into the compiled
	// topic file.
	Paths []string `yaml:"paths,omitempty"`
	Scope []string `yaml:"scope,omitempty"`

	// Three-list schema from ADR-008.
	Directives           []string `yaml:"directives"`
	ManualDirectives     []string `yaml:"manual_directives,omitempty"`
	SuppressedDirectives []string `yaml:"suppressed_directives,omitempty"`

	// Index-level aggregations, surfaced to the final governance.md.
	Reminders    []string `yaml:"reminders,omitempty"`
	Verification []string `yaml:"verification,omitempty"`

	// SPEC-005 extensions (optional).
	CanonicalPhrases []string               `yaml:"canonical_phrases,omitempty"`
	BehavioralSignal map[string]interface{} `yaml:"behavioral_signal,omitempty"`

	// StartByte / EndByte bound the sentinel block in the original body
	// so callers can read or rewrite it without re-searching.
	StartByte int
	EndByte   int
}

const (
	sentinelOpen  = "[edikt:directives:start]: #"
	sentinelClose = "[edikt:directives:end]: #"
)

// ExtractSentinel finds the first edikt directive sentinel block that starts
// at the beginning of a line and parses its YAML body. Inline occurrences
// (e.g. in code fences or backtick spans) are skipped — only a sentinel
// whose open marker begins at column 0 is treated as the live block.
//
// If the block is absent, returns Sentinel{Present: false} with no error —
// a missing block is a valid state and the caller decides how to handle it
// (legacy one-shot LLM generation path).
func ExtractSentinel(body string) (Sentinel, error) {
	var s Sentinel

	openIdx := findLineStart(body, sentinelOpen)
	if openIdx == -1 {
		return s, nil
	}
	closeIdx := findLineStart(body[openIdx+len(sentinelOpen):], sentinelClose)
	if closeIdx == -1 {
		return s, fmt.Errorf("sentinel block opened but not closed")
	}
	closeIdx += openIdx + len(sentinelOpen) // absolute position of the closing marker's first byte
	closeEnd := closeIdx + len(sentinelClose)

	// Inner YAML is between the end of the open marker's line and the start
	// of the close marker's line. Skip the newline after each marker.
	inner := body[openIdx+len(sentinelOpen) : closeIdx]
	inner = strings.TrimSpace(inner)

	if len(inner) > 0 {
		// Directive text contains `(ref: NNN)` which yaml.v3 refuses as a
		// flow-mapping value inside a block sequence. Use a custom two-pass
		// parser: line scanning for list fields, yaml.Unmarshal only for the
		// behavioral_signal sub-block (which is YAML-safe).
		parsed, err := parseSentinelBlock(inner)
		if err != nil {
			return Sentinel{}, fmt.Errorf("sentinel yaml: %w", err)
		}
		s = parsed
	}

	s.Present = true
	s.StartByte = openIdx
	s.EndByte = closeEnd
	return s, nil
}

// BodyExcludingSentinel returns the source body with the sentinel block
// replaced by a single newline. Used by source_hash (ADR-008: "SHA-256 of
// the artifact body with the directives block excluded, normalized").
func (d *Document) BodyExcludingSentinel() string {
	if !d.Sentinel.Present {
		return d.Body
	}
	return d.Body[:d.Sentinel.StartByte] + d.Body[d.Sentinel.EndByte:]
}

// findLineStart searches for the first occurrence of needle that begins at
// column 0 (i.e. is either at the start of body or immediately after a '\n')
// AND is not inside a fenced code block (``` or ~~~).
// Returns the byte offset of the match, or -1 if not found.
func findLineStart(body, needle string) int {
	// Build a set of code-fence regions so we can skip matches inside them.
	// A code fence is a line that starts with ``` or ~~~.
	type region struct{ start, end int }
	var fenced []region
	{
		lines := strings.Split(body, "\n")
		pos := 0
		inFence := false
		fenceStart := 0
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if !inFence && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")) {
				inFence = true
				fenceStart = pos
			} else if inFence && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")) {
				fenced = append(fenced, region{fenceStart, pos + len(line)})
				inFence = false
			}
			pos += len(line) + 1 // +1 for the '\n' removed by Split
		}
	}

	inFencedRegion := func(abs int) bool {
		for _, r := range fenced {
			if abs >= r.start && abs <= r.end {
				return true
			}
		}
		return false
	}

	offset := 0
	for {
		idx := strings.Index(body[offset:], needle)
		if idx == -1 {
			return -1
		}
		abs := offset + idx
		// Must be at column 0 and not inside a code fence.
		if (abs == 0 || body[abs-1] == '\n') && !inFencedRegion(abs) {
			return abs
		}
		// Skip past this false-positive and continue searching.
		offset = abs + 1
		if offset >= len(body) {
			return -1
		}
	}
}

// parseSentinelBlock parses the inner content of a sentinel block using a
// custom line scanner. Directive text contains `(ref: NNN)` which yaml.v3
// treats as a mapping-value in a flow context within a block sequence and
// refuses to parse. We therefore extract list fields manually.
func parseSentinelBlock(inner string) (Sentinel, error) {
	var s Sentinel
	lines := strings.Split(inner, "\n")
	currentList := ""
	var bsLines []string
	inBS := false

	appendItem := func(list, item string) {
		item = strings.Trim(item, `"`)
		switch list {
		case "directives":
			s.Directives = append(s.Directives, item)
		case "manual_directives":
			s.ManualDirectives = append(s.ManualDirectives, item)
		case "suppressed_directives":
			s.SuppressedDirectives = append(s.SuppressedDirectives, item)
		case "paths":
			s.Paths = append(s.Paths, item)
		case "scope":
			s.Scope = append(s.Scope, item)
		case "reminders":
			s.Reminders = append(s.Reminders, item)
		case "verification":
			s.Verification = append(s.Verification, strings.TrimLeft(item, "- []"))
		case "canonical_phrases":
			s.CanonicalPhrases = append(s.CanonicalPhrases, item)
		}
	}

	for _, line := range lines {
		// Collect behavioral_signal sub-block.
		if inBS {
			if strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t") {
				bsLines = append(bsLines, line)
				continue
			}
			// sub-block ended
			sub := strings.Join(bsLines, "\n")
			var m map[string]interface{}
			if err := yaml.Unmarshal([]byte(sub), &m); err == nil && len(m) > 0 {
				s.BehavioralSignal = m
			}
			inBS = false
			bsLines = nil
			currentList = ""
		}

		// Collect list items.
		if currentList != "" {
			if strings.HasPrefix(line, "  - ") {
				appendItem(currentList, strings.TrimPrefix(line, "  - "))
				continue
			}
			if strings.TrimSpace(line) == "" {
				continue
			}
			currentList = ""
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		ci := strings.Index(trimmed, ":")
		if ci <= 0 {
			continue
		}
		key := trimmed[:ci]
		rest := strings.TrimSpace(trimmed[ci+1:])

		if knownSentinelLists[key] {
			currentList = key
			if strings.HasPrefix(rest, "- ") {
				appendItem(key, strings.TrimPrefix(rest, "- "))
			}
			continue
		}

		if key == "behavioral_signal" {
			inBS = true
			continue
		}

		v := strings.Trim(rest, `"'`)
		switch key {
		case "source_hash":
			s.SourceHash = v
		case "directives_hash":
			s.DirectivesHash = v
		case "compiler_version":
			s.CompilerVersion = v
		case "topic":
			// Support both scalar ("topic: hooks") and inline YAML list
			// ("topic: [hooks, agent-rules]").
			if strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]") {
				inner := rest[1 : len(rest)-1]
				for _, part := range strings.Split(inner, ",") {
					t := strings.TrimSpace(strings.Trim(part, `"'`))
					if t != "" {
						s.Topics = append(s.Topics, t)
					}
				}
			} else {
				s.Topics = []string{v}
			}
			if len(s.Topics) > 0 {
				s.Topic = s.Topics[0]
			}
		}
	}

	// Flush trailing sub-block.
	if inBS && len(bsLines) > 0 {
		sub := strings.Join(bsLines, "\n")
		var m map[string]interface{}
		if err := yaml.Unmarshal([]byte(sub), &m); err == nil && len(m) > 0 {
			s.BehavioralSignal = m
		}
	}
	return s, nil
}
