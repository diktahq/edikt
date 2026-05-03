---
type: adr
id: ADR-028
title: Two-phase compile — Phase A resync (LLM) + Phase B merge (deterministic)
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-05-02T00:00:00Z
references:
  adrs: [ADR-020, ADR-027, ADR-008]
  invariants: [INV-001]
  prds: []
  specs: []
amends: ADR-020
---

# ADR-028 — Two-phase compile: Phase A resync (LLM) + Phase B merge (deterministic)

## Status

**Accepted**

## Context

ADR-020 mandates a strict latency budget for the Go-binary `gov:compile` path:

- `<5s` full regenerate from cold cache
- `<500ms` no-op recompile (all hashes unchanged)
- `<2s` for `--check` mode

The budget exists because compile is a hot-path command — it runs on every artifact edit, on every CI build, and on every `:new` chain. Determinism plus speed is what made compile usable as a foreground tool rather than an offline build step.

Sidecar architecture (ADR-027) introduces a new operational reality: when a parent ADR / invariant / guideline body changes, the corresponding `<name>.edikt.yaml` sidecar's recorded `source_excerpt.quote` no longer matches the live prose. The sidecar is **stale**. Regenerating a stale sidecar requires re-running the directive extraction prompt against the parent body, which is an LLM call routed through a forked subagent. Per-subagent latency for extraction is empirically 30–60 s p50, capped at concurrency 8 (per ADR-018 / SPEC-006 evaluator-style isolation).

A naive implementation — "compile invokes subagents inline whenever it detects stale sidecars" — blows the ADR-020 budget by 60–100×. A 5-stale-sidecar compile would take ~30–90 s on the wall clock; a 50-stale mass-edit would take 5–7 minutes. Both are legitimate workloads. ADR-020's `<5s` budget cannot honestly bind those workloads, but the budget still bears on the deterministic projection step (sidecars → topic-grouped governance files) — that step has no LLM call and no excuse to be slow.

The right move is to amend ADR-020's contract so the latency budget binds the part of compile that doesn't make LLM calls, and a new conditional phase is introduced for the part that does. Determinism for `--check` mode is non-negotiable: CI gates depend on it.

## Decision

**Compile runs as two phases. Phase A is conditional and may invoke LLMs; Phase B is unconditional and is a pure deterministic merge with the ADR-020 latency budget.**

### Phase A — Resync (conditional)

- **Trigger.** Phase A runs if and only if at least one sidecar is stale. Staleness is determined by recomputing the parent body's content hash and comparing each sidecar's recorded `source_excerpt.quote` against the live prose at the recorded `line_start`/`line_end` range. Any mismatch flags the sidecar as stale.
- **Action.** For each stale sidecar, dispatch the directive extraction prompt to a forked subagent (`context: fork`) at concurrency 8, with continue-on-error semantics. Each subagent runs against a single artifact's parent body — never multiple artifacts in the same subagent context. Subagent failures are logged to `.edikt/state/compile-errors.log` and the run continues. At the end of Phase A, an aggregated failure report is produced; if any subagent failed, Phase B does NOT run and compile exits non-zero.
- **Latency.** No SLO. Phase A's runtime is dominated by subagent latency, which is bounded only by extraction-prompt cost.
- **Mandatory progress UI.** Phase A MUST emit a per-subagent progress line on stderr while it runs. Format: `Resyncing ADR-001... ADR-007... [▓▓▓░░░░] 2/7 (eta 45s)`. ETA is computed from the running p50 latency of completed subagents in the same run. The progress UI is the user's only feedback during a multi-minute resync; silent operation is forbidden.
- **`--check` mode FORBIDDEN.** When `gov:compile --check` is invoked and any sidecar is stale, compile exits 1 immediately with a single-line actionable error: `error: N sidecar(s) stale — run 'edikt gov compile' to resync`. `--check` MUST NOT dispatch any subagents. This preserves the CI gate's determinism: PRs that drift sidecars from prose fail CI and must be fixed by the author, not auto-fixed by the bot.

### Phase B — Merge (unconditional, deterministic)

- **Action.** Pure read-only deterministic merge. Walks the sidecar set, groups directives by `topic`, renders the routing table, writes `.claude/rules/governance.md` and `.claude/rules/governance/<topic>.md`. Output MUST be byte-equal across runs for byte-equal sidecar input. No LLM calls. No `Task` / `Agent` tool dispatch. No shell-out to anything that could introduce nondeterminism (date stamps, hash random salt, sort order without a tiebreaker).
- **Static analysis check.** A test in `test/integration/governance/` MUST verify, via Go AST inspection or symbol grep, that the merge code path has no transitive reference to `Agent`, `Task`, or any subprocess-spawning symbol. The check enforces "Phase B is pure" structurally, not by code review.
- **Latency SLO (preserved from ADR-020).** Phase B's budget binds the deterministic merge:
  - `<5s` for full regenerate from cold cache (50 sidecars baseline)
  - `<500ms` for no-op (all sidecars unchanged)
  - `<2s` for `--check`

  These numbers are the contract ADR-020 originally placed on the whole compile. ADR-028 reassigns them to Phase B specifically. The expectation is that Phase B is fast — it's parsing YAML and rendering markdown.

### Operational consequences

- **Steady-state compile (no stale sidecars):** Phase A is skipped. Total wall-clock = Phase B = sub-second. ADR-020's `<500ms` no-op budget continues to hold.
- **Active development (5–10 stale sidecars):** Total wall-clock = ~30–90 s. The user just edited the prose; the cost is expected and visible via the progress UI.
- **Mass-edit (50 stale sidecars):** Total wall-clock = ~5–7 minutes. No rate-limiting safeguard. The discipline is "don't let things get stale" — surfaced via `/edikt:doctor` warnings and CI gate. ADR-028 does not encode a soft cap.

### Failure semantics

- **Subagent failure in Phase A.** Logged to `.edikt/state/compile-errors.log` with the subagent's stderr, the parent artifact path, and the prompt template version. Run continues for the remaining sidecars. At the end of Phase A, if any subagent failed, compile exits 1 with an aggregated report listing each failed artifact and a one-line remediation hint per failure class. Topic files are NOT updated when any sidecar failed — the merge step is gated on Phase A success because partial topic files would silently drop directives.
- **Concurrent compile.** A file lock at `.edikt/state/compile.lock` serializes runs. The second compile blocks until the first releases the lock, or exits 1 with `--no-wait`. The lock is released even on Phase A failure (since topic files weren't touched).

## Consequences

- **Good — `--check` stays deterministic and CI-safe.** PRs that drift prose without resyncing sidecars fail CI. The fix is to run `edikt gov compile` locally and commit the regenerated sidecars. Authorship of sidecar regeneration stays with the human author (or a designated bot identity), not with whichever CI runner executes the check.
- **Good — Phase B's purity is structural.** A static-analysis test forbids `Agent`/`Task` symbols in the merge path. Future maintainers cannot accidentally introduce LLM calls into the deterministic path; the build will fail.
- **Good — ADR-020's contract remains meaningful.** The `<5s` / `<500ms` / `<2s` budgets continue to bind a real and measurable phase. They no longer claim to bind workloads they cannot honestly bound.
- **Bad — Mass-edit performance is unbounded.** A 50-stale resync takes 5–7 minutes. There is no soft cap, no "resync the most-recently-edited 10 sidecars first," no degraded mode. Mitigation: `/edikt:doctor` warnings on staleness count, and the CI `--check` gate makes accumulating stale sidecars structurally hard.
- **Bad — Two-phase splitting complicates exit codes.** Phase A failure prevents Phase B; Phase A success but Phase B failure (write error, lock contention) is a different failure class. Mitigation: exit codes are documented in `bin/edikt`'s `--help`; both phases log their own structured error events to `events.jsonl`.
- **Neutral — Progress UI is mandatory.** Adds a soft requirement on Phase A's implementation but it's a UX win for any non-trivial resync.

## Alternatives Considered

### One-phase compile that always re-runs subagents inline

- **Pros:** Simpler implementation. No phase-split bookkeeping. Always-fresh sidecars without a separate verb.
- **Cons:** Blows ADR-020's budget on every steady-state compile. `--check` becomes either nondeterministic (it triggers subagents) or a contradiction (it claims to be a check but mutates state).
- **Rejected because:** the budget is load-bearing. Compile is a hot-path tool; making every invocation potentially LLM-costly destroys the use case.

### Detect staleness but resync as a separate user-driven verb (`edikt resync`)

- **Pros:** Compile becomes purely deterministic, always. Resync is opt-in.
- **Cons:** Two-step UX for the common case (edit prose → run resync → run compile). New users will hit the "compile says my sidecars are stale, what now?" trap and experience compile as broken. Adds a second hot-path verb.
- **Rejected because:** the two-step is artificial. Phase A inside compile is conditional and only fires when needed; users don't need to learn a second verb.

### Resync inline but skip determinism for `--check`

- **Pros:** Simplest possible split — same code path, different behavior bit.
- **Cons:** CI cannot rely on `--check` being deterministic. Two CI runs of the same input produce different verdicts (subagents drift). Catastrophic for branch protection.
- **Rejected because:** `--check` is a contract surface for CI. Determinism is non-negotiable.

### Cache subagent outputs to disk and replay on `--check`

- **Pros:** `--check` becomes deterministic via cache hit.
- **Cons:** Cache invalidation is a separate problem with its own failure modes (stale cache, cross-machine cache differences, cache corruption). Complicates the model significantly for marginal gain. Phase B is already deterministic via the sidecars themselves.
- **Rejected because:** sidecars are the cache. Adding a cache on top of a cache is unnecessary.

## Confirmation

- `bin/edikt gov compile` (without `--check`) runs Phase A on stale sidecars with concurrency 8, emits progress lines, then runs Phase B unconditionally on Phase A success.
- `bin/edikt gov compile --check` exits 1 immediately when any sidecar is stale; emits the canonical actionable error.
- A static-analysis test in `test/integration/governance/test_phase_b_purity.go` (or equivalent) verifies the Phase B code path has no transitive reference to `Agent`/`Task` symbols.
- ADR-020's frontmatter has `amended_by: ADR-028` and the `**Status:**` line reads `Amended by ADR-028`.
- Phase B continues to satisfy ADR-020's `<5s` / `<500ms` / `<2s` SLOs on the 50-sidecar baseline corpus (covered by the existing benchmark in `test/perf/`).

## Directives


---

*Captured by edikt:adr — 2026-05-02*
