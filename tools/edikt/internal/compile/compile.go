// Package compile holds the Rule and Topic types the renderers share. The
// legacy sentinel-driven grouper that lived here went with govrun.Run under
// ADR-082 (c); the three-list formula is implemented by phaseb.
package compile

import ()

// Rule is a single directive destined for compiled output. Source is the
// artifact ID (e.g. "ADR-NNN" or "INV-NNN"). Text is the directive string
// verbatim from the sentinel block.
type Rule struct {
	Text   string
	Source string
	// Intent is the failure the rule prevents. Rendered on the ambient core
	// beneath the statement (SPEC-018 SR-001).
	//
	// Statement-only was measured at 62 est-tokens per rule against the
	// write-time payload's 76 — the always-loaded surface carrying LESS per
	// rule than the channel that interrupts a write. That inversion is the
	// reason this field exists: a bare constraint gets rationalised away under
	// pressure, and 15 of the 18 invariants promoted by SR-001 carry an intent
	// that statement-only would have dropped.
	Intent string
}

// Topic groups rules and carries the aggregate paths/scope metadata.
type Topic struct {
	Name    string
	Rules   []Rule
	Paths   []string // union of contributing source paths, sorted, deduped
	Scope   []string // union of contributing source scopes, sorted, deduped
	Signals []string // union of contributing source signals (routing keywords), insertion-order, deduped
	Sources []string // artifact IDs, sorted, deduped
}
