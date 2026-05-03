---
type: adr
id: ADR-027
title: Sidecar architecture for governance metadata (supersedes ADR-008)
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-05-02T00:00:00Z
references:
  adrs: [ADR-008, ADR-018, ADR-020, ADR-022]
  invariants: [INV-001, INV-002, INV-005]
  prds: []
  specs: []
supersedes: ADR-008
---

# ADR-027 — Sidecar architecture for governance metadata

## Status

**Accepted**

## Context

Today's `gov:compile` (per ADR-008, ADR-020) writes generated directive metadata — `directives:`, `manual_directives:`, `suppressed_directives:`, `source_hash`, `directives_hash`, `compiler_version`, `topic`, `paths`, `scope` — into an in-body sentinel block at the bottom of every accepted ADR, invariant, and guideline. INV-002 declares accepted ADRs immutable. INV-005 + the `EDIKT_COMPILE_IN_PROGRESS=1` bypass guard the writes against accidental edits.

The boundary works but it is **definitional rather than structural**. Every reader of an ADR — every code path that loads, edits, parses, or audits the file — must know "the prose body above the `[edikt:directives:start]` marker is governed by INV-002 and is immutable; the YAML below it is generated and may be regenerated freely." That contract lives in convention and code comments, not in file boundaries. Two real failures resulted from this:

1. **Cross-artifact context contamination in v0.6.0-rc1.** When `gov:compile` ran across many ADRs in one Claude session, the parent context accumulated all of them. Directive extraction for ADR-022 dropped from 25 directives to 16 because the LLM, having already absorbed the verbose prose of ADR-020 and ADR-021 in the same session, deduplicated against them and silently elided directives it considered redundant. Each ADR's compile must run in a fresh context with a locked extraction prompt — but compile in v0.6.0-rc1 cannot do that without overhauling the whole orchestration.

2. **Compile cannot run out-of-band.** Because compile mutates accepted ADRs in place, `EDIKT_COMPILE_IN_PROGRESS=1` must be in scope at the moment of write. That signal is set by `bin/edikt gov compile` and is honored by the PreToolUse hook in the Claude session running the compile. External tooling — a CI workflow that wants to recompile, a developer running compile from a separate terminal while Claude is editing prose — has no clean way to participate. Compile is structurally coupled to the Claude session that initiated it.

The root cause of both is the same: **edikt writes to a file the user authors**. Every other resolution requires defending that boundary harder. The sidecar pattern dissolves the conflict by making the boundary *structural*: edikt writes to a file the user does not author, and the user writes to a file edikt does not touch.

## Decision

**Adopt a co-located sidecar pattern for governance metadata.** For every ADR, invariant, and guideline `<name>.md`, edikt maintains a co-located `<name>.edikt.yaml`. The sidecar contains the topic, signals, and directives extracted from the parent's prose body. edikt **never** writes to `<name>.md`. Compile becomes a pure deterministic merge over the existing sidecar set.

Operational rules:

1. **Compile is read-only over .md.** `gov:compile` reads parent prose only to recompute hashes for drift detection. It writes to `<name>.edikt.yaml` only when the sidecar's recorded source quote no longer matches the live body (resync — see ADR-028 / Phase 3). It writes to `.claude/rules/governance.md` and `.claude/rules/governance/<topic>.md` as it does today. It NEVER writes to any ADR, invariant, or guideline `.md`.

2. **Sidecar generation moves to artifact-edit verbs.**
   - `/edikt:adr:new`, `/edikt:invariant:new`, `/edikt:guideline:new` create the `(parent.md, parent.edikt.yaml)` pair atomically. Sidecar generation runs in a forked subagent (`context: fork`) with a locked extraction prompt — no parent-session contamination.
   - `/edikt:adr:compile` (and the per-artifact variants) regenerate the sidecar in a fresh subagent context per artifact. Replaces today's "compile mutates the parent ADR" path.
   - `/edikt:adr:review` cross-checks the sidecar against the live prose, surfaces drift, and regenerates the sidecar on user confirmation.

3. **Schema is sealed at v1.** Sidecars conform to `templates/schemas/sidecar.schema.json` (Phase 1). The on-disk shape contains: `schema_version` (const 1), `topic`, `path`, `signals`, `directives[]` (each with `text` + `source_excerpt{line_start, line_end, quote}`). `additionalProperties: false` is enforced at every object level. **`source_hash`, `agent_prompt_version`, and `directives_hash` are explicitly forbidden in the persisted shape.** All hashes are recomputed on read at compile time. Persisting them invites stale-hash bugs and drift between recorded and computed values.

4. **In-body sentinels are removed in v0.6.0.** The `[edikt:directives:start]` / `[edikt:directives:end]` block stops being authoritative. The `edikt migrate sidecars` tool (Phase 6) lifts existing sentinel content into sidecars and strips the in-body block. The migration runs once on upgrade; v0.6.0's compile reads sidecars only — there is no double-parser fallback window. Pre-migration projects produce a clear actionable error, not silent degradation.

5. **Subagent isolation eliminates cross-artifact contamination.** `:new` and per-artifact `:compile` always dispatch each artifact's extraction to a fresh subagent context. The parent never sees the other ADRs' bodies. This closes the v0.6.0-rc1 regression class.

6. **Out-of-band compile becomes possible.** Because compile no longer writes to user-authored files, `EDIKT_COMPILE_IN_PROGRESS=1` is no longer required to bypass the managed-region guard for `.md` artifacts. CI workflows, sandbox tooling, and concurrent Claude sessions can all run `bin/edikt gov compile` without coordinating with whoever holds the editing seat in the parent session. (The signal still exists for managed CLAUDE.md / settings.json regions per INV-005 narrowed scope.)

## Consequences

- **Good — INV-002 becomes structural.** "Accepted ADR content is immutable" is no longer a convention enforced by hooks; it is a fact: edikt has no code path that writes to `.md` files for governance artifacts. INV-005 narrows from "all managed markdown regions across the repo" to specifically "CLAUDE.md and settings.json regions" — the only places where edikt still does inline managed writes.

- **Good — topic-grouped governance becomes a pure projection.** `.claude/rules/governance/` is a deterministic function of the sidecar set. Diff-only topic rendering (Phase 8) becomes trivial via topic fingerprints (hash of the joined directive texts in topic-key order). When no topic's fingerprint changes, the corresponding governance file is not rewritten — saving git churn and watcher noise.

- **Good — compile latency improves.** The fast-path skip from ADR-008 (`source_hash` matches → no LLM call) is preserved structurally in the sidecar's recorded `quote` per directive: when every recorded quote still matches the live prose, no extraction is needed for that artifact. Per-artifact extraction stays bounded; cross-artifact context bloat is gone.

- **Good — extension authoring simplifies.** A user adding an ADR by hand now has a clear contract: write the prose body, run `/edikt:adr:compile` (or `/edikt:adr:new` from scratch), and the sidecar is generated. No "is this section managed or not" judgment call.

- **Bad — two files per artifact.** Every accepted ADR, invariant, and guideline ships with a `.edikt.yaml` companion. Git status shows twice as many entries during compile. CHANGELOG and review tooling must understand the pair as a unit. Mitigation: tooling (`/edikt:doctor`, `/edikt:adr:review`) reports the pair atomically, and editor workflows can collapse them via filename grouping. The cost is real but bounded.

- **Bad — migration is mandatory and breaking.** Projects pinned to v0.5.x cannot upgrade to v0.6.0 without running `edikt migrate sidecars` (Phase 6). The migration is dual-schema (handles v0.4.3 `content_hash:` legacy AND v0.5.x/v0.6.0-rc1 `source_hash:` schema) but it must run. Compile fails fast with a clear error if the migration hasn't run. Mitigation: `bin/edikt upgrade` chains the migration automatically; users get a one-line CHANGELOG note and a clear rollback path.

- **Neutral — INV-005 narrows but does not relax.** The byte-range overlap guard for managed regions still applies to `CLAUDE.md` and `settings.json`. The bypass signals (`EDIKT_COMPILE_IN_PROGRESS`, `EDIKT_MIGRATION_IN_PROGRESS`) keep their semantics. The change is in *scope*: governance artifacts are no longer managed-region territory because edikt doesn't write to them.

## Schema

The sidecar's on-disk shape is sealed by `templates/schemas/sidecar.schema.json` (Phase 1, JSON Schema 2020-12). Forbidden top-level properties: `source_hash`, `agent_prompt_version`, `directives_hash`. All hashes are recomputed on read at compile time. The schema is referenced — not duplicated — in this ADR; the `$id` URL `https://edikt.dev/schemas/sidecar/v1.json` and the schema's git-tracked path are the canonical source of truth.

## Migration

v0.6.0 ships `edikt migrate sidecars` (Phase 6, dual-schema lift). The migration:

- Walks every accepted ADR, active invariant, and guideline `.md` in the project.
- For each, parses the in-body `[edikt:directives:start]` / `[edikt:directives:end]` block.
- Detects schema variant: v0.4.3 legacy (`content_hash:`) vs v0.5.x / v0.6.0-rc1 (`source_hash:` + `topic:` + `signals:`).
- Lifts the contents into a co-located `<name>.edikt.yaml` matching the v1 schema.
- Strips the in-body sentinel block from `<name>.md`. INV-002 is not violated because the migration is a one-time structural lift; the prose body content is preserved byte-for-byte.
- Re-runs `gov:compile` to regenerate `.claude/rules/governance/`.

Migration is **mandatory on upgrade**. v0.6.0 reads sidecars only — there is no double-parser window. `bin/edikt upgrade` chains the migration automatically; running compile against a pre-migration project produces a single-line actionable error: `error: pre-migration project state — run 'edikt migrate sidecars' before 'edikt gov compile'`.

## Boundary Statement (resolves architect-domain question #1)

The sidecar is **generated metadata**. It is not part of the immutable ADR record. INV-002 governs the prose body of `<name>.md` — Context, Decision, Consequences, Status, and any other section the author writes. The sidecar `<name>.edikt.yaml` is regenerated whenever the body changes; compile may overwrite it freely.

This is the same boundary the project always wanted; it was previously expressed as "the prose above the sentinel block is immutable, the YAML below is generated." ADR-027 makes that boundary structural rather than definitional, which is the whole point.

## Alternatives Considered

### Stay with in-body sentinels, fix cross-artifact contamination via subagent dispatch only

- **Pros:** No file-shape change. No migration. Existing tooling continues to work.
- **Cons:** Doesn't solve out-of-band compile — `EDIKT_COMPILE_IN_PROGRESS` still required. Doesn't make INV-002 structural — every reader still has to know about the managed-region split inside ADR files. The contamination fix becomes a permanent ceremony tax on every compile path.
- **Rejected because:** the in-body sentinel is the root of both problems. Patching the symptoms keeps the structural confusion intact.

### One sidecar at the project level, not per-artifact

- **Pros:** Fewer files in git status. Single point of truth for all governance metadata.
- **Cons:** Loses co-location — readers can no longer browse to an ADR and see its associated metadata in the same directory. Loses git-blame locality — a directive change cannot be attributed to a specific artifact's edit. Loses isolation — a project-level file is one Claude session's contention point, defeating subagent dispatch.
- **Rejected because:** co-location is a substantial readability win, and the per-artifact contention story matters.

### Separate file extension or directory (e.g., `.edikt/sidecars/<name>.yaml`)

- **Pros:** Clear separation between user-authored and edikt-managed surface. No risk of editors auto-completing one when the user means the other.
- **Cons:** Loses co-location at the directory level — readers must navigate elsewhere. Loses ergonomics: `git mv` of an ADR doesn't move its sidecar. Increases the contract surface (which directory holds what).
- **Rejected because:** co-location ergonomics outweigh the cosmetic separation.

## Confirmation

- `templates/schemas/sidecar.schema.json` exists, validates the v1 shape, forbids `source_hash` / `agent_prompt_version` / `directives_hash` at root via `additionalProperties: false`. (Phase 1 — shipped.)
- `docs/architecture/decisions/ADR-008-deterministic-compile-and-three-list-schema.md` has `status: superseded`, `superseded_by: ADR-027`, and `**Status:** Superseded by ADR-027` as its top body line.
- `docs/architecture/decisions/ADR-027-*.md` exists with `status: accepted`, contains all required sections (Context, Decision, Consequences, Schema, Migration, Boundary Statement, Alternatives, Confirmation, Directives).
- `gov:compile` re-run picks up ADR-027's directives and drops ADR-008's. ADR-027's directives appear in the compiled topic file (likely `governance/compile.md`).
- The Phase 4 / 6 implementation work that follows can cite ADR-027 by ID.

## Directives


---

*Captured by edikt:adr — 2026-05-02*
