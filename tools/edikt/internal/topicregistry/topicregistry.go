// Package topicregistry reads and writes .edikt/topics.yaml — the one
// pinned-human-judgment artifact SPEC-011 introduces.  edikt-guard:allow
//
// The registry maps a topic id to a single task-language line stating when a
// task needs that topic. Every render consumes those lines VERBATIM: no
// rewriting, no summarising, no re-deriving (SSP-002). That verbatim contract
// is the whole point — a description regenerated per compile is a description
// nobody pinned, which is the failure mode the registry exists to prevent.
//
// Two mechanisms guard it:
//
//   - approved_at proves a human approved something once.
//   - description_hash proves THESE BYTES are the thing they approved.
//
// The second is not redundant with the first. approved_at alone survives an
// edit to the description it was stamped for, so a changed line reads as
// approved when nobody approved it (pre-flight SEC #7). The hash is what makes
// that mechanically detectable. Detection posture on mismatch is FLAG, not
// refuse: an unapproved-edit registry still renders — it reports the drift with
// a count and a denominator — because bricking compile over a human's own
// in-progress edit to their own file trades a real failure for a worse one.
//
// The pipeline NEVER writes this file. Its only sanctioned writers are
// `bin/edikt sidecar approve --kind topic-description` and the human's own
// editor; compile treats it as read-only input, the same posture INV-010 gives  edikt-guard:allow
// parent .md prose.
package topicregistry

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// RelPath is the registry's location relative to a project root.
const RelPath = ".edikt/topics.yaml"

// MaxDescriptionChars is the schema ceiling. ~160 chars is roughly the 25-token
// budget the ambient topic index is priced at, per topic.
const MaxDescriptionChars = 160

var topicIDRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// Entry is one registry row.
type Entry struct {
	Description     string `yaml:"description"`
	DescriptionHash string `yaml:"description_hash"`
	ApprovedAt      string `yaml:"approved_at"`
}

// Registry is the whole file: topic id -> entry.
type Registry map[string]Entry

// PathFor returns the registry path for a project root.
func PathFor(root string) string { return filepath.Join(root, ".edikt", "topics.yaml") }

// HashDescription returns the hex SHA-256 of a description's exact bytes.
//
// No normalisation. Whitespace normalisation would make "these are the bytes
// approved" false for any change the normaliser folds away, and the field's
// entire job is to notice a changed description.
func HashDescription(d string) string {
	sum := sha256.Sum256([]byte(d))
	return hex.EncodeToString(sum[:])
}

// Load reads and strict-decodes a registry file.
//
// A missing file is an error here, not an empty registry — see LoadOrEmpty for
// the caller that legitimately tolerates absence. Distinguishing them at the
// API boundary keeps a consumer from reading "file not found" as "zero topics
// approved" (INV-013).  edikt-guard:allow
func Load(path string) (Registry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := yaml.NewDecoder(bufio.NewReader(f))
	dec.KnownFields(true)
	reg := Registry{}
	if err := dec.Decode(&reg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return reg, nil
}

// LoadOrEmpty returns an empty registry when the file does not exist, and
// propagates every other error.
//
// Absence is legitimate exactly once: a project that has never run the
// approval ceremony has no registry, and that is a no-subject case rather than
// a failure. Every other read error still fails.
func LoadOrEmpty(path string) (Registry, error) {
	reg, err := Load(path)
	if os.IsNotExist(err) {
		return Registry{}, nil
	}
	return reg, err
}

// Marshal renders a registry deterministically: keys sorted, two-space indent.
//
// Go map iteration order is randomised, so marshalling the map directly would
// produce a different byte sequence per run. Sorted emission is what makes
// "two consecutive compiles produce byte-equal output" true of this file.
func Marshal(reg Registry) ([]byte, error) {
	topics := make([]string, 0, len(reg))
	for t := range reg {
		topics = append(topics, t)
	}
	sort.Strings(topics)

	root := &yaml.Node{Kind: yaml.MappingNode}
	for _, t := range topics {
		e := reg[t]
		val := &yaml.Node{Kind: yaml.MappingNode}
		appendScalar(val, "description", e.Description)
		appendScalar(val, "description_hash", e.DescriptionHash)
		appendScalar(val, "approved_at", e.ApprovedAt)
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: t},
			val)
	}

	var sb strings.Builder
	sb.WriteString("# edikt topic registry — human-owned pinned judgment.\n")
	sb.WriteString("# Written by `bin/edikt sidecar approve --kind topic-description`.\n")
	sb.WriteString("# Consumed VERBATIM by every rendered surface. The compile pipeline never writes this file.\n")

	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return []byte(sb.String()), nil
}

func appendScalar(m *yaml.Node, key, value string) {
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value, Style: yaml.DoubleQuotedStyle})
}

// ValidateEntry checks one row against the topic-registry.v1 contract.
func ValidateEntry(topic string, e Entry) error {
	if !topicIDRe.MatchString(topic) {
		return fmt.Errorf("topic %q: must match ^[a-z][a-z0-9-]{0,39}$", topic)
	}
	if e.Description == "" {
		return fmt.Errorf("topic %q: description is empty", topic)
	}
	if len(e.Description) > MaxDescriptionChars {
		return fmt.Errorf("topic %q: description is %d chars (max %d)", topic, len(e.Description), MaxDescriptionChars)
	}
	if strings.ContainsAny(e.Description, "\r\n") {
		return fmt.Errorf("topic %q: description must be a single line", topic)
	}
	if e.DescriptionHash == "" {
		return fmt.Errorf("topic %q: description_hash is empty", topic)
	}
	if e.ApprovedAt == "" {
		return fmt.Errorf("topic %q: approved_at is empty", topic)
	}
	return nil
}

// Drift is one registry row whose recorded hash no longer covers its bytes.
type Drift struct {
	Topic    string
	Recorded string
	Actual   string
}

// Coverage is the measured state of the registry against a corpus of topics.
//
// Every field is a count with a stated denominator, because a report of
// "registry ok" that cannot say how many topics it covered is indistinguishable
// from a report that covered none (INV-013).  edikt-guard:allow
type Coverage struct {
	CorpusTopics []string // sorted, distinct topics found in the sidecar corpus
	Approved     []string // sorted corpus topics with an approved registry entry
	Pending      []string // sorted corpus topics with NO registry entry
	Orphans      []string // sorted registry entries whose topic appears in no sidecar
	HashMismatch []Drift  // approved entries whose description bytes changed since approval
}

// Total is the denominator: how many distinct topics the corpus actually has.
func (c Coverage) Total() int { return len(c.CorpusTopics) }

// Measure compares a registry against the topics a corpus uses.
//
// It never invents an entry and never fails: incompleteness that is visibly
// reported is not one of the two failure modes the registry exists to prevent
// (invention and silence). The caller decides what to do with Pending — during
// the migration window compile renders what is approved and REPORTS what is
// pending; once a topic has an approved entry, a regression back to unapproved
// is a hard error.
func Measure(reg Registry, corpusTopics []string) Coverage {
	seen := map[string]bool{}
	cov := Coverage{}
	for _, t := range corpusTopics {
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		cov.CorpusTopics = append(cov.CorpusTopics, t)
	}
	sort.Strings(cov.CorpusTopics)

	for _, t := range cov.CorpusTopics {
		e, ok := reg[t]
		if !ok {
			cov.Pending = append(cov.Pending, t)
			continue
		}
		cov.Approved = append(cov.Approved, t)
		if actual := HashDescription(e.Description); actual != e.DescriptionHash {
			cov.HashMismatch = append(cov.HashMismatch, Drift{Topic: t, Recorded: e.DescriptionHash, Actual: actual})
		}
	}

	for t := range reg {
		if !seen[t] {
			cov.Orphans = append(cov.Orphans, t)
		}
	}
	sort.Strings(cov.Orphans)
	sort.Slice(cov.HashMismatch, func(i, j int) bool { return cov.HashMismatch[i].Topic < cov.HashMismatch[j].Topic })

	return cov
}

// Report renders the coverage as the single line compile prints.
//
// The line always carries the denominator and always names the pending and
// unapproved-edit counts, so "0 pending" is a measured zero rather than a
// silence that could equally mean the check never ran.
func (c Coverage) Report() string {
	if c.Total() == 0 {
		return "topic registry: no subject — the corpus declares zero topics"
	}
	s := fmt.Sprintf("topic registry: %d/%d topics approved, %d pending approval",
		len(c.Approved), c.Total(), len(c.Pending))
	if len(c.Pending) > 0 {
		s += fmt.Sprintf(" (%s)", strings.Join(c.Pending, ", "))
	}
	if len(c.HashMismatch) > 0 {
		names := make([]string, 0, len(c.HashMismatch))
		for _, d := range c.HashMismatch {
			names = append(names, d.Topic)
		}
		s += fmt.Sprintf("; %d description(s) edited since approval (%s) — re-approve to re-pin",
			len(c.HashMismatch), strings.Join(names, ", "))
	}
	if len(c.Orphans) > 0 {
		s += fmt.Sprintf("; %d orphan entr(y/ies) (%s)", len(c.Orphans), strings.Join(c.Orphans, ", "))
	}
	return s
}
