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
// When s was loaded via Load, the canonical bytes were computed once during
// load and cached on the struct (Phase 7 of PLAN-sidecar-review-fixes #39).
// Marshal returns a copy of the cached bytes in that case; on cache miss
// it falls through to the encoder.
func Marshal(s *Sidecar) ([]byte, error) {
	if len(s.cachedMarshal) > 0 {
		// Defensive copy: callers may mutate the returned slice. The cache
		// is the determinism anchor; a stray append elsewhere must not
		// poison the next Marshal call.
		out := make([]byte, len(s.cachedMarshal))
		copy(out, s.cachedMarshal)
		return out, nil
	}
	return marshalUncached(s)
}

// marshalUncached is the underlying encoder path; Marshal calls it on cache
// miss, and Load calls it once to seed the cache.
func marshalUncached(s *Sidecar) ([]byte, error) {
	clone := *s
	clone.cachedMarshal = nil
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
