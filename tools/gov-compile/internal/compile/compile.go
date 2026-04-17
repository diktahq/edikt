// Package compile merges the three-list directive schema per ADR-008 and
// groups effective rules by topic.
package compile

import (
	"fmt"
	"sort"

	"github.com/diktahq/edikt/tools/gov-compile/internal/parse"
)

// Rule is a single directive destined for compiled output. Source is the
// artifact ID (e.g. "ADR-012" or "INV-003"). Text is the directive string
// verbatim from the sentinel block.
type Rule struct {
	Text   string
	Source string
}

// Topic groups rules and carries the aggregate paths/scope metadata.
type Topic struct {
	Name    string
	Rules   []Rule
	Paths   []string // union of contributing source paths, sorted, deduped
	Scope   []string // union of contributing source scopes, sorted, deduped
	Sources []string // artifact IDs, sorted, deduped
}

// EffectiveRules applies the ADR-008 three-list formula:
//
//	effective = (directives - suppressed_directives) ∪ manual_directives
//
// Set difference is by exact string match. Union preserves order: filtered
// auto-directives first, manuals appended. Duplicates across the two lists
// (same string in both filtered-directives and manual) are de-duped to one
// occurrence.
func EffectiveRules(s parse.Sentinel) []string {
	suppress := make(map[string]struct{}, len(s.SuppressedDirectives))
	for _, sd := range s.SuppressedDirectives {
		suppress[sd] = struct{}{}
	}
	seen := make(map[string]struct{}, len(s.Directives)+len(s.ManualDirectives))
	out := make([]string, 0, len(s.Directives)+len(s.ManualDirectives))
	for _, d := range s.Directives {
		if _, isSuppressed := suppress[d]; isSuppressed {
			continue
		}
		if _, dup := seen[d]; dup {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	for _, m := range s.ManualDirectives {
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out
}

// SourceID extracts the artifact ID from a document path.
// Handles "ADR-NNN-…", "INV-NNN-…", and guideline filenames.
func SourceID(path string) string {
	// Take basename, strip .md.
	base := basename(path)
	base = trimSuffix(base, ".md")
	// For ADR/INV forms, the ID is the "ADR-NNN" / "INV-NNN" prefix.
	if len(base) >= 7 && (base[:4] == "ADR-" || base[:4] == "INV-") {
		// Match "ADR-NNN" or "ADR-NNN-" (the trailing dash is optional).
		end := 4
		for end < len(base) && isDigit(base[end]) {
			end++
		}
		return base[:end]
	}
	return base
}

func basename(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

func trimSuffix(s, suf string) string {
	if len(s) >= len(suf) && s[len(s)-len(suf):] == suf {
		return s[:len(s)-len(suf)]
	}
	return s
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// Group buckets the effective_rules from every included document by
// `topic:` field in its sentinel block. Topic name "invariants" is reserved
// for the governance.md index (never a standalone topic file).
//
// Documents with an absent topic: field fall into the "_unassigned" bucket
// and trigger a warning — caller decides whether to error or delegate to
// the LLM fallback in commands/gov/compile.md.
func Group(docs []*parse.Document) (map[string]*Topic, []string, error) {
	topics := map[string]*Topic{}
	var unassigned []string

	for _, doc := range docs {
		if !doc.Sentinel.Present {
			// Caller should have filtered these; bail loudly.
			return nil, nil, fmt.Errorf("document %s has no sentinel block", doc.Path)
		}
		docTopics := doc.Sentinel.Topics
		if len(docTopics) == 0 && doc.Sentinel.Topic != "" {
			// Fallback for sentinels parsed before Topics was populated.
			docTopics = []string{doc.Sentinel.Topic}
		}
		if len(docTopics) == 0 {
			// Fall through to LLM in the markdown command; record the unmet.
			unassigned = append(unassigned, doc.Path)
			continue
		}
		src := SourceID(doc.Path)
		effectiveRules := EffectiveRules(doc.Sentinel)
		for _, topicName := range docTopics {
			if _, ok := topics[topicName]; !ok {
				topics[topicName] = &Topic{Name: topicName}
			}
			t := topics[topicName]
			for _, rule := range effectiveRules {
				t.Rules = append(t.Rules, Rule{Text: rule, Source: src})
			}
			t.Paths = mergeSortedUnique(t.Paths, doc.Sentinel.Paths)
			t.Scope = mergeSortedUnique(t.Scope, doc.Sentinel.Scope)
			if !contains(t.Sources, src) {
				t.Sources = append(t.Sources, src)
				sort.Strings(t.Sources)
			}
		}
	}

	// De-duplicate rules within each topic by exact string match. Keep
	// first-occurrence source ref per compile.md §13.
	for _, t := range topics {
		t.Rules = dedupRules(t.Rules)
	}
	return topics, unassigned, nil
}

func mergeSortedUnique(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	for _, s := range a {
		seen[s] = struct{}{}
	}
	out := append([]string{}, a...)
	for _, s := range b {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func dedupRules(in []Rule) []Rule {
	seen := make(map[string]struct{}, len(in))
	out := make([]Rule, 0, len(in))
	for _, r := range in {
		if _, ok := seen[r.Text]; ok {
			continue
		}
		seen[r.Text] = struct{}{}
		out = append(out, r)
	}
	return out
}
