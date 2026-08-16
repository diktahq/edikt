// Package postflight implements the effective-specialist-set computation
// for the SPEC-008 post-flight review pipeline.  // edikt-guard:allow
//
// The orchestrator (`commands/sdlc/post-flight.md`) needs to decide which
// L3 specialists to dispatch on a given diff. The decision is the union
// of file-pattern auto-detection and the user's `required:` list, minus
// the user's `never:` list — all configured under `post-flight:` in
// `.edikt/config.yaml`.
//
// EffectiveSet is the pure-function set algebra. The caller (the
// orchestrator or the `bin/edikt gov post-flight-scope` subcommand)
// supplies the auto-detected list separately; EffectiveSet itself only
// does union/subtract/dedup/sort.
//
// Determinism: the result is sorted lexicographically so callers can rely
// on stable order across runs (important for shell-script consumers).
package postflight

import "sort"

// EffectiveSet returns (autoDetected ∪ required) − never, deduplicated and
// sorted lexicographically. The result is the L3 specialist set the
// orchestrator should dispatch.
//
// Semantics:
//   - Membership is by exact string match. Empty strings are dropped.
//   - never overrides required AND autoDetected. If a name appears in
//     never, it is excluded from the result no matter where else it
//     appears.
//   - The result is deterministic (sorted) and free of duplicates.
//
// Callers that want to honor a configured `auto:` filter (e.g.
// `auto: ['security', 'dba']` — restrict auto-detection to a subset)
// must apply that filter to autoDetected BEFORE calling EffectiveSet.
// When `auto: ['*']` is configured (the default), the caller passes
// autoDetected as-is.
func EffectiveSet(autoDetected, required, never []string) []string {
	neverSet := toSet(never)
	out := make(map[string]struct{}, len(autoDetected)+len(required))

	for _, name := range autoDetected {
		if name == "" {
			continue
		}
		if _, blocked := neverSet[name]; blocked {
			continue
		}
		out[name] = struct{}{}
	}
	for _, name := range required {
		if name == "" {
			continue
		}
		if _, blocked := neverSet[name]; blocked {
			continue
		}
		out[name] = struct{}{}
	}

	result := make([]string, 0, len(out))
	for name := range out {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// toSet builds a set from a slice for O(1) membership. Empty strings are
// skipped so a callers's stray entry does not blanket-block everything.
func toSet(s []string) map[string]struct{} {
	m := make(map[string]struct{}, len(s))
	for _, v := range s {
		if v == "" {
			continue
		}
		m[v] = struct{}{}
	}
	return m
}
