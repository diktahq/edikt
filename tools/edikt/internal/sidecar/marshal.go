package sidecar

import (
	"bytes"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// Marshal serializes s with the canonical formatting required by Phase 8 of
// PLAN-sidecar-architecture: 2-space indent, LF line endings, sorted signals,
// stable struct-tag-defined key order at every level. Empty signals render as
// `signals: []` (nil and empty slices both normalize to `[]string{}` so the
// fingerprint computation is stable across producers).
//
// HISTORY, because the shape here is the fix for a real class of bug.
//
// Marshal used to return bytes memoized by Load, and Marshal existed as
// the escape hatch for callers that had mutated the struct. That default was
// backwards: the memoized path is correct only when nothing changed, and a
// caller who was not thinking about it got silent data loss — the mutation
// vanished and the write reported success.
//
// It bit three independent callers before it was removed:
//
//	cmd/sidecar.go        worked around locally with a loadForMutation helper
//	                      that declined to prime the cache — a call-site fix,
//	                      which is exactly why the next two repeated the bug
//	govrun/twophase.go    the migration_preserved strip was a NO-OP, so the
//	                      transient field survived every compile in violation
//	                      of ADR-034
//	sidecar/bodydrift.go  StampBodyDigest recorded no digest at all
//
// The memoized variant is gone rather than renamed, because no caller wanted
// it: the cache was marshalUncached's own output, so for an unmutated struct
// both paths returned byte-identical results. Its only distinguishing
// behaviour was being wrong. The one thing a cached variant could plausibly
// have offered — echoing a file's original bytes to preserve hand formatting
// — was never on offer, since Load stored canonical bytes, not file bytes.
//
// Cost of removal, measured rather than assumed: ~173µs per sidecar versus
// ~892ns memoized. TopicFingerprint is the only hot caller, at 57 sidecars
// per Phase B run — about 10ms against ADR-028's 500ms no-op SLO.
// Two percent of the budget to make the default correct.
func Marshal(s *Sidecar) ([]byte, error) {
	return marshalUncached(s)
}

// marshalUncached is the underlying encoder path; Marshal calls it on cache
// miss, and Load calls it once to seed the cache.
func marshalUncached(s *Sidecar) ([]byte, error) {
	clone := *s
	if clone.Signals == nil {
		clone.Signals = []string{}
	} else {
		sigs := append([]string(nil), clone.Signals...)
		sort.Strings(sigs)
		clone.Signals = sigs
	}
	clone.SourcePath = ""

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&clone); err != nil {
		return nil, fmt.Errorf("marshal sidecar: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close encoder: %w", err)
	}
	return buf.Bytes(), nil
}
