// Package contradiction implements SPEC-010 phase 9's contradiction  edikt-guard:allow
// detector (AC-9.1, AC-9.2).
//
// Scope, deliberately narrow per docs/internal/audits/
// AUDIT-2026-08-09-semantica-external-comparison.md § B1's cost warning
// ("a general contradiction detector is a project, not an afternoon ...
// scope hard"): same co-loading set (topic — the unit that lands in one
// compiled .claude/rules/governance/<topic>.md, which is what a session
// actually loads together), same normalized noun-phrase (reusing ADR-034's  edikt-guard:allow
// lossless-gate normalizer, not a new one), opposing modality. Report only,
// never resolve — the same posture ADR-050 §4 and INV-011 already commit  edikt-guard:allow
// this project to for detection generally.
//
// Precedence rule (AC-9.2): supersession is the ONE precedence rule this
// corpus has, and it is already applied upstream of this package — a
// superseded artifact's directives never reach the `pairs` slice Detect
// receives (Phase A/B's exclusion accounting removes them before compile).
// Two directives that both survive that filter are, by construction, both
// currently-accepted governance with equal standing. There is no further
// rule for resolving a contradiction between two such directives — and per
// AC-9.2, a conflict with no rule MUST be surfaced to a human, never
// resolved by the system. Every Conflict this package returns is exactly
// that case.
//
// Pure Go, no LLM, deterministic — safe to run inside Phase B's report
// step (ADR-028's Phase B purity constraint governs the MERGE, not this  edikt-guard:allow
// read-only report, but the no-LLM property holds regardless).
package contradiction

import (
	"sort"

	"github.com/diktahq/edikt/tools/edikt/internal/lossless"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// nounPhraseMatchThreshold mirrors the lossless gate's own bound (ADR-034):  edikt-guard:allow
// two noun-phrases within this Levenshtein ratio are "the same claim" for
// matching purposes. Reusing the established threshold rather than
// inventing a second one for a structurally identical comparison.
const nounPhraseMatchThreshold = 0.10

// DirectiveRef identifies one side of a detected conflict.
type DirectiveRef struct {
	Text     string `json:"text"`
	Source   string `json:"source"` // artifact ID, e.g. "ADR-004"  edikt-guard:allow
	Modality string `json:"modality"`
}

// Conflict is one detected same-topic, same-noun-phrase, opposing-modality
// pair. Reported, never auto-resolved (see package doc).
type Conflict struct {
	Topic      string       `json:"topic"`
	NounPhrase string       `json:"noun_phrase"`
	A          DirectiveRef `json:"a"`
	B          DirectiveRef `json:"b"`
}

type entry struct {
	DirectiveRef
	norm string
}

// opposingModality reports whether two modality classes assert
// incompatible claims about the same subject. MANDATE/PROHIBITION
// (MUST vs MUST NOT) is the unambiguous pair; SHOULD/SHOULD-NOT is the
// same shape at a softer strength. MAY is permissive and never opposes
// anything — allowing something is never incompatible with a separate
// rule about it unless that rule is itself a MUST NOT, which the
// MANDATE/PROHIBITION pair already covers from the other side.
func opposingModality(a, b string) bool {
	pairs := map[string]string{
		"MANDATE":     "PROHIBITION",
		"PROHIBITION": "MANDATE",
		"SHOULD":      "SHOULD-NOT",
		"SHOULD-NOT":  "SHOULD",
	}
	return pairs[a] == b && a != "" && b != ""
}

// Detect scans every topic's contributing sidecars for same-noun-phrase,
// opposing-modality directive pairs. pairs MUST already have retired
// (superseded/deprecated/migration:skip) artifacts excluded — Detect does
// not re-derive that filter, matching Phase B's own contract that
// exclusion happens before either function sees the slice.
func Detect(pairs []sidecar.Pair) []Conflict {
	byTopic := make(map[string][]entry)
	for _, p := range pairs {
		if p.Sidecar == nil {
			continue
		}
		topic := p.Sidecar.Topic
		for _, d := range p.Sidecar.Directives {
			byTopic[topic] = append(byTopic[topic], entryFor(d.Text, p.ArtifactID))
		}
		for _, pr := range p.Sidecar.Prohibitions {
			byTopic[topic] = append(byTopic[topic], entryFor(pr.Text, p.ArtifactID))
		}
	}

	var conflicts []Conflict
	for topic, entries := range byTopic {
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				a, b := entries[i], entries[j]
				if !opposingModality(a.Modality, b.Modality) {
					continue
				}
				if a.norm == "" || b.norm == "" {
					continue // no extractable noun-phrase; nothing to compare
				}
				if lossless.LevenshteinRatio(a.norm, b.norm) > nounPhraseMatchThreshold {
					continue
				}
				conflicts = append(conflicts, Conflict{
					Topic:      topic,
					NounPhrase: a.norm,
					A:          a.DirectiveRef,
					B:          b.DirectiveRef,
				})
			}
		}
	}

	sort.Slice(conflicts, func(i, j int) bool {
		if conflicts[i].Topic != conflicts[j].Topic {
			return conflicts[i].Topic < conflicts[j].Topic
		}
		if conflicts[i].A.Source != conflicts[j].A.Source {
			return conflicts[i].A.Source < conflicts[j].A.Source
		}
		return conflicts[i].B.Source < conflicts[j].B.Source
	})
	return conflicts
}

func entryFor(text, source string) entry {
	return entry{
		DirectiveRef: DirectiveRef{
			Text:     text,
			Source:   source,
			Modality: lossless.ModalityOf(text),
		},
		norm: lossless.NormalizeNounPhrase(text),
	}
}
