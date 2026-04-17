---
type: adr
id: ADR-020
title: Migrate /edikt:gov:compile to a tier-2 helper for deterministic transformations
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-04-17T00:00:00Z
references:
  adrs: [ADR-008, ADR-015]
  invariants: [INV-001]
  prds: []
  specs: []
---

# ADR-020: Migrate `/edikt:gov:compile` to a tier-2 helper for deterministic transformations

**Status:** Accepted
**Date:** 2026-04-17
**Decision-makers:** Daniel Gomes

---

## Context and Problem Statement

A compile of edikt's governance today takes ~35 minutes for a clean run (40 source documents, 11 topic files, 153 tool calls). The command lives at `commands/gov/compile.md` as a 600-line LLM procedure. Every sentinel extraction, every topic grouping, every directive write is an LLM round-trip — including the steps that are purely mechanical: YAML frontmatter parsing, sentinel-block regex extraction, SHA-256 hashing per ADR-008, three-list set arithmetic, Jinja-style template rendering.

This produces four failure modes:

1. **Latency.** 35 minutes blocks authoring. `add directive → recompile → review → iterate` loops become unusable.
2. **Token cost.** 153 round-trips against a 600-line command with full governance context is not free.
3. **Non-determinism.** LLM sampling can produce slightly different directive wording or topic-assignment decisions across runs for the same source input. ADR-008's `source_hash`/`directives_hash` fast-path assumes byte-equal output; LLM drift silently invalidates that assumption.
4. **CI-hostile.** No one runs a 35-minute compile in CI. Teams work around it with `--skip-on-outage` hacks or drift between local and CI output.

The problem is not "the LLM needs better prompting." The problem is using a non-deterministic tool for deterministic work. ADR-015 already recognized this pattern and carved out tier-2 tooling for exactly this case — the governance benchmark migrated from markdown-walking to `tools/gov-benchmark/` for the same reasons. `/edikt:gov:compile` is the next obvious candidate.

## Decision Drivers

- Compile must finish in seconds on no-op and single-artifact changes, and under 10 seconds on a full regenerate.
- Output must be byte-equal across runs for byte-equal input (required for ADR-008's fast-path skip to work as designed).
- LLM should re-enter only for steps that genuinely need reasoning (one-shot sentinel generation for a new artifact, hand-edit conflict interview).
- The markdown command surface stays — users still run `/edikt:gov:compile` — but the heavy lifting moves to a tier-2 Python helper per ADR-015.
- Tier-1 / tier-2 boundary rules from ADR-015 hold: the markdown command may NOT depend on the tier-2 helper at runtime for anything core to edikt's first-install experience. Tier-2 install is opt-in via `edikt install gov-compile`. Users who don't install the tier-2 helper get the legacy LLM path (slow but functional).

## Considered Options

1. **Full tier-2 migration with LLM fallback.** Ship a Python helper that does the deterministic work; LLM re-enters for the narrow "reasoning" steps. Users opt in via `edikt install gov-compile`. Legacy LLM path remains for users who don't install.
2. **Better prompting of the current LLM path.** Batch reads, parallel tool calls, trim the 600-line command. Cuts time by ~2x at best — still minutes, still non-deterministic.
3. **Replace the entire LLM path with a native compile.** Remove LLM involvement entirely — including sentinel generation on first compile. Fast but loses the "write my ADR, let compile figure out the directives" experience that new users rely on.
4. **Add a cache layer without changing the architecture.** Cache the LLM's per-artifact output keyed on `source_hash`. Works for fast-path skip but doesn't fix first-compile latency or determinism.

## Decision

We will adopt **option 1** — a tier-2 helper that does 95% of the work deterministically, with the LLM retained for genuinely-reasoning steps.

### Split of responsibilities

**Moves to `tools/gov-compile/` (Python, deterministic):**

- Discover source documents under `docs/architecture/decisions`, `docs/architecture/invariants`, `docs/guidelines` (configurable via `.edikt/config.yaml` `paths:`).
- Parse YAML frontmatter, filter by `status:` (accepted ADRs, active INVs, all guidelines).
- Extract sentinel blocks verbatim between `[edikt:directives:start]: #` and `[edikt:directives:end]: #` markers.
- Compute `source_hash` (SHA-256 of body excluding the sentinel block, CRLF→LF normalized) and `directives_hash` (SHA-256 of sorted `directives:` list items) per ADR-008.
- Fast-path skip when both hashes match — byte-for-byte equal input means byte-for-byte equal output, no re-work.
- Compute effective rule set via the ADR-008 three-list formula: `(directives - suppressed_directives) ∪ manual_directives`.
- Group rules by topic using the `topic:` field in the sentinel block (new, optional initially, eventually required).
- Generate `paths:` globs per topic from a map in `.edikt/config.yaml`.
- Render topic files from a deterministic Jinja template; render the governance index the same way.
- Orphan detection + atomic state file write (already has a Python stub in `commands/gov/compile.md` §12d — lift-and-shift).
- Cross-reference validation (grep for `INV-NNN` / `ADR-NNN` in source; strip fabricated refs).

**Stays in `commands/gov/compile.md` (LLM, reasoning):**

1. **First-compile sentinel generation** for artifacts that have no `[edikt:directives:*]: #` block yet. Read `## Decision` or `## Rule`, emit MUST/NEVER directive text, write the block back into the source. Happens once per artifact, then the tier-2 helper reads the generated block on every subsequent compile.
2. **Hand-edit conflict interview** triggered when `source_hash` matches but `directives_hash` does not (user edited the compiled block directly). One LLM Q/A per conflict; strategy flag `--strategy=regenerate|preserve` short-circuits in headless mode.
3. **Contradiction warning wording.** The helper detects contradictions mechanically (same predicate, opposite sense across artifacts) and passes the list to the LLM only for human-readable framing of the warning.

### Determinism guarantees

| Property | How it holds |
|---|---|
| Byte-equal output for byte-equal input | Pure Python template rendering + sorted iteration |
| Reproducible across runs | No LLM sampling in the deterministic path |
| Reproducible across machines | Dependencies pinned with `==` per ADR-015 |
| CI-friendly | `gov-compile --check` runs in < 2 s on 40 sources |
| Fast iteration during authoring | Single-file changes complete in < 500 ms |

### Time targets

| Scenario | Before | After |
|---|---|---|
| No-op recompile (both hashes match) | ~5 min | < 500 ms |
| Single-file change | ~5 min | < 1 s |
| Full regenerate, all sentinels present | ~35 min | < 2 s |
| New ADR, sentinel generation needed | ~35 min | ~10 s (1 LLM call + deterministic rest) |

## Alternatives Considered

### Better prompting of the current LLM path

- **Pros:** No new tier-2 install to maintain; no Python dependency for compile.
- **Cons:** Cuts time ~2x at best; still minutes; does not solve determinism or token cost; fights the shape of the problem.
- **Rejected because:** the architectural mismatch (non-deterministic tool for deterministic work) is the root cause. Prompting tweaks treat a symptom.

### Full native compile, no LLM at all

- **Pros:** Maximum simplicity and speed.
- **Cons:** Users who write a new ADR have to author the `[edikt:directives:*]: #` block by hand. The "just write the decision and compile figures it out" experience is a headline product feature for edikt.
- **Rejected because:** authoring ergonomics matter. Retain the LLM for the one-shot generation step where it genuinely earns its keep.

### Cache-layer only

- **Pros:** Smallest change; preserves current architecture.
- **Cons:** First compile per artifact still pays the 35-min cost. Cache is non-trivial (invalidation on source edit, cross-platform cache location). Doesn't fix determinism (different LLM runs can still produce different cached values).
- **Rejected because:** solves latency for the narrow case only and leaves determinism unresolved.

## Consequences

- **Good:** Compile becomes usable on every save. CI can run `gov-compile --check` as a standard pre-merge gate. Authoring loop drops from "make coffee" to "blink."
- **Good:** `source_hash`/`directives_hash` fast-path in ADR-008 actually works as designed — same input, same bytes, same hash, instant skip.
- **Good:** Token cost per compile falls from ~100k+ tokens to ~5k (only the sentinel-generation + conflict-interview paths remain LLM).
- **Good:** The tier-2 precedent from ADR-015 widens — establishing a clear pattern for migrating any deterministic transformation out of LLM prompts. Future candidates: `/edikt:gov:rules-update`, `/edikt:adr:compile`, parts of `/edikt:sdlc:drift`.
- **Bad:** Users who don't run `edikt install gov-compile` stay on the legacy LLM path. Acceptable by ADR-015's tier-2 model — the slow path still works.
- **Bad:** Python dependency for the helper (`pyyaml`, `jinja2`, `jsonschema`). All pinned with `==` per ADR-015 tier-2 rules. Not bundled in `install.sh`.
- **Neutral:** Requires one data-model change — a `topic:` field in the sentinel block (optional initially, eventually required). Existing blocks without `topic:` fall through to LLM-driven grouping as a one-shot per artifact, producing the `topic:` value that subsequent compiles read deterministically.

## Confirmation

- `tools/gov-compile/compile.py` exists, installs via `edikt install gov-compile`.
- `commands/gov/compile.md` delegates to the tier-2 helper for the deterministic steps; drops its 500+ lines of inline procedure for those steps.
- A full regenerate of edikt's own governance (dogfood) completes in < 5 s wall-clock on the reference machine.
- Byte-equal output test: two consecutive compiles of the same source produce byte-equal `.claude/rules/` trees. Verified by `diff -r` in CI.
- No-op recompile (all hashes match) exits in < 500 ms and makes zero LLM calls.
- Legacy LLM path still functional for users without the tier-2 helper installed (ADR-015 parity contract).
- An added acceptance criterion in the successor SPEC: `/edikt:gov:compile --check` runs in under 2 s on 40 source documents.

## Directives

[edikt:directives:start]: #
source_hash: pending
directives_hash: pending
compiler_version: "0.4.3"
paths:
  - "commands/gov/compile.md"
  - "tools/gov-compile/**"
  - ".edikt/config.yaml"
scope:
  - implementation
  - design
  - review
directives:
  - Deterministic transformations in `/edikt:gov:compile` (YAML parse, sentinel extraction, hash computation, three-list merge, topic grouping, template rendering, orphan detection) MUST run in `tools/gov-compile/` (tier-2 Python helper per ADR-015). NEVER keep them as LLM round-trips in `commands/gov/compile.md`. (ref: ADR-020)
  - LLM invocations from `/edikt:gov:compile` are restricted to: (a) generating sentinel blocks for new artifacts that lack one, (b) hand-edit conflict interviews (when `source_hash` matches but `directives_hash` does not), (c) composing contradiction-warning wording from a mechanically-detected contradiction list. No other LLM calls are permitted during compile. (ref: ADR-020)
  - Compile output MUST be byte-equal across runs for byte-equal input. Non-determinism (LLM drift, unsorted iteration, timestamp embedding in hashed content) is forbidden. CI MUST include a diff-equality test. (ref: ADR-020)
  - `/edikt:gov:compile --check` on 40 source documents MUST complete in under 2 seconds. Full regenerate on the same corpus MUST complete in under 5 seconds. No-op recompile (both hashes match) MUST exit in under 500 ms with zero LLM calls. (ref: ADR-020)
  - Tier-2 `gov-compile` install is opt-in via `edikt install gov-compile`. Users without the helper installed MUST continue to get a functional (if slow) compile via the legacy LLM path. (ref: ADR-020, ADR-015)
  - Sentinel blocks SHOULD carry a `topic:` field (optional in v0.6.0, required in v0.7.0). Missing `topic:` falls back to one-shot LLM grouping, which writes the resolved topic back into the sentinel so subsequent compiles are deterministic. (ref: ADR-020)
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #

---

*Captured by edikt:adr — 2026-04-17*
