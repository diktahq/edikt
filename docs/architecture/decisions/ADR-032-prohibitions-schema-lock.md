---
type: adr
id: ADR-032
title: Lock `prohibitions[]` as a separate top-level sidecar field through v1.x
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-05-03T00:00:00Z
references:
  adrs: [ADR-027, ADR-028, ADR-030]
  invariants: [INV-002, INV-005]
  prds: []
  specs: []
---

# ADR-032 — Lock `prohibitions[]` as a separate top-level sidecar field through v1.x

## Status

**Accepted**

## Context

Phase 1 of PLAN-v060-governance-accuracy added three v1.1 fields to the sidecar schema: `paths[]`, `scope[]`, and `prohibitions[]`. The `prohibitions[]` field carries `MUST NOT` directives synthesised from rejected `## Considered Options` (Phase 2 Rule C). Through Phases 6, 8, and 10 the field accumulated three separate dependencies on its top-level placement:

- The Phase 6 structural-equivalence comparator (`bin/edikt sidecar diff`) treats `len(prohibitions)` as a tier-1 strict-equality assertion. Test failures emit `len(prohibitions) mismatch: expected N, got M`.
- The Phase 8 compile pipeline renders prohibitions in their own sentinel-bracketed managed region `[edikt:prohibitions:start/end]` with a distinct sha256 anchor, guarded by an INV-005 byte-range overlap check against the directives and manual regions.
- The Phase 10 adversarial benchmark `--mode rejected-options` walks `sidecar.Prohibitions[]` directly to generate one attack per rejected option; the per-corpus pass rate threshold is computed against that walked count.

Numerous alternative shapes were considered during the v0.6.0 design phase: folding prohibitions into `directives[]` with a per-entry `kind: "prohibition"` discriminator, attaching prohibitions to a parent directive as a child `prohibits[]` array, or carrying them as a free-form annotation field. Each alternative was rejected for reasons documented in the considered-options section below. Now that the field has shipped and three subsystems depend on its position, this ADR locks the decision through the v1.x schema lifetime so the placement cannot be relitigated without explicit amendment evidence.

## Decision

`prohibitions[]` MUST remain a separate top-level field in the v1.x sidecar schema. The schema position MUST NOT change before a v2.0 schema version bump.

The v0.7.0 release MAY reconsider this decision in a new ADR amending ADR-032, BUT only with explicit evidence: either (a) compile UX feedback indicating the separation hurts authorial flow, or (b) adversarial benchmark drift data showing folded-in prohibitions perform measurably better at the harness layer.

## Consequences

### Good

- Three subsystems (Phase 6 comparator, Phase 8 render, Phase 10 benchmark) inherit a stable contract. Their tests don't need to chase schema reshapes.
- Mechanical regression detection stays trivial: a regression in prohibition extraction registers as a `len(prohibitions)` change observable by the Phase 6 hard-field comparator on the next compile.
- The INV-005 byte-range overlap guard — extended in Phase 8 to cover the new `[edikt:prohibitions:...]` managed region — is conceptually clean. Folding prohibitions into directives would require ad-hoc per-entry markers within the directives region, which conflicts with the existing managed-region semantics.

### Bad

- Schema authors who hand-edit sidecars (Phase 7's `add-manual-directive` notwithstanding) face a third top-level array to remember alongside `directives[]` and `manual_directives[]`. Mitigated by the validator and `bin/edikt doctor` orphan-ref check, but the cognitive overhead is real.
- A future ADR that wants to fold prohibitions back into directives faces a hard amendment cycle, not a soft revision.

### Accepted trade-off

The cognitive overhead is small (one extra top-level field) and the mechanical-regression-detection win is large (zero-to-build gates across three subsystems). The lock is the right call through v1.x.

## Considered Options

### A. Separate top-level `prohibitions[]` array (chosen)

The v0.6.0 implementation. Pros documented above.

### B. Fold into `directives[]` with a `kind` discriminator

Each entry would carry `kind: "directive" | "prohibition" | "manual"`. Compile would filter by kind during render.

- Pros: a single array for all directive-like entries; familiar shape for authors who think in terms of "rules".
- Cons: every consumer (comparator, render, benchmark, doctor) needs filter logic instead of direct field access; `len()` no longer answers "how many prohibitions"; INV-005 byte-range guard cannot cleanly carve the rendered output into three managed regions without ad-hoc per-entry markers; mechanical regression detection becomes a per-tuple analysis instead of a count comparison.

### C. Attach prohibitions to a parent directive as a child `prohibits[]` array

Each directive entry could carry `prohibits: [{text, source_excerpt}, ...]`.

- Pros: ties each prohibition to its originating decision directly; query "which prohibition came from which decision" is structural.
- Cons: prohibitions in v0.6.0 are synthesised from rejected `## Considered Options`, not from chosen directives — there's no parent directive to attach them to. The shape would require inventing a synthetic parent or duplicating provenance metadata.

### D. Free-form annotation field

A `notes: string` or `annotations: map[string]any` field where prohibitions live as YAML-encoded entries.

- Pros: maximum flexibility.
- Cons: defeats `additionalProperties: false` strictness; defeats `KnownFields(true)` decoder safety; defeats every mechanical check.

## Confirmation

- The schema declares `prohibitions[]` as an optional top-level array of objects. Verified by `templates/schemas/sidecar.v1.schema.json`.
- The Sidecar Go struct exposes `Prohibitions []Prohibition` as a top-level field. Verified by `tools/edikt/internal/sidecar/sidecar.go`.
- The Phase 6 comparator references `sc.Prohibitions` directly. Verified by `tools/edikt/internal/sidecardiff/sidecardiff.go`.
- The Phase 8 render emits a separate `[edikt:prohibitions:start/end]` managed region. Verified by `tools/edikt/internal/phaseb/merge.go`.
- The Phase 10 benchmark walks rejected options to generate attacks against `sc.Prohibitions`. Verified by `tools/edikt/internal/benchmark/benchmark.go` and `commands/gov/benchmark.md`.

## How to enforce

`prohibitions[]` MUST stay at the top level of the sidecar schema. NEVER fold it into `directives[]`, attach it as a child of a directive, or move it to a free-form annotation field through any v1.x release. Schema position changes require a new ADR amending ADR-032.

The schema lock is enforced mechanically by `templates/schemas/sidecar.v1.schema.json`'s `additionalProperties: false` plus the `KnownFields(true)` Go decoder — any v1.x-schema sidecar that places prohibitions elsewhere fails to parse.

The v0.7.0 reconsideration path requires evidence: compile UX feedback OR benchmark drift data. NEVER amend ADR-032 without one of those two evidence types.
