---
type: adr
id: ADR-033
title: Add `gov <subcommand>` group to the ADR-029 tier-1 orchestration verb list
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-05-04T00:00:00Z
references:
  adrs: [ADR-029, ADR-031, ADR-020, ADR-030]
  invariants: [INV-001, INV-006]
  prds: []
  specs: []
amends: ADR-029
---

# ADR-033 — Add `gov <subcommand>` group to the ADR-029 tier-1 orchestration verb list

## Status

**Accepted**

## Context

ADR-029 permits tier-1 markdown commands to invoke `bin/edikt <subcommand>` for orchestration purposes, under four rules. Rule 3 enumerates the permitted verbs exhaustively. The current list (post-ADR-031) contains:

- `bin/edikt verify <plan-id> [--phase N]`
- `bin/edikt doctor`
- `bin/edikt migrate <subcommand>`
- `bin/edikt gov compile`
- `bin/edikt install <helper>`
- `bin/edikt upgrade`
- `bin/edikt use`
- `bin/edikt rollback`
- `bin/edikt sidecar <subcommand>` (added by ADR-031)

Adding a new verb requires an ADR amending ADR-029.

Phase 11.5 of PLAN-v060-governance-accuracy migrates four legacy Python heredocs from tier-1 markdown commands into proper tier-2 Go subcommands:

1. **`bin/edikt gov compile-history`** — implements the five-rule orphan-set state machine previously embedded in `commands/gov/compile.md` Pass 2 (~200 LOC python heredoc).
2. **`bin/edikt gov gitignore-bootstrap`** — implements the AC-019 .gitignore management previously embedded in `commands/gov/compile.md` (~30 LOC python heredoc).
3. **`bin/edikt gov directive-check`** — implements the three directive-quality checks previously embedded in `commands/gov/_shared-directive-checks.md` (~50 LOC python heredoc).
4. **`bin/edikt doctor`** — already enumerated; the routed-source check and statusLine-type check were ported into the existing doctor binary (no new top-level verb).

The first three are new sibling subcommands under the `gov` parent. ADR-029 Rule 3 currently lists `bin/edikt gov compile` as one specific verb — a literal reading rejects sibling verbs like `gov compile-history` because they're not the same string. The choice is the same as ADR-031 faced for `sidecar`:

- **(A)** Pin every new sibling verb individually via successive ADRs.
- **(B)** Broaden the entry to `bin/edikt gov <subcommand>` (group permit), matching the `migrate <subcommand>` and `sidecar <subcommand>` shapes already in the list.

## Decision

**Amend ADR-029 Rule 3 to replace the single-verb entry `bin/edikt gov compile` with the group-permit entry `bin/edikt gov <subcommand>`.**

The amended verb list (Phase 11.5):

- `bin/edikt verify <plan-id> [--phase N]`
- `bin/edikt doctor`
- `bin/edikt migrate <subcommand>`
- `bin/edikt gov <subcommand>` ← **broadened by this ADR (was: `gov compile`)**
- `bin/edikt install <helper>`
- `bin/edikt upgrade`
- `bin/edikt use`
- `bin/edikt rollback`
- `bin/edikt sidecar <subcommand>`

The broadening is intentional and mirrors the precedent set by ADR-031 (`sidecar` group permit). All future `gov` subcommands — `compile`, `compile-history`, `gitignore-bootstrap`, `directive-check`, `lossless-check` (Phase 11), and any future Phase additions — fall under this single permit. Pinning each one would require an ADR per addition; permitting the `gov` group once is cleaner and lower maintenance overhead.

All four ADR-029 rules still hold for every `gov` subcommand invocation:

1. **Absence detection** — every tier-1 caller checks `command -v bin/edikt` (or relies on the upstream gate in the markdown's frontmatter `tier_2_dependency:` declaration) before invocation.
2. **Exit code only** — Phase 11.5 callers pass output verbatim and never parse its shape. The orphan-set state machine returns `0` (continue) or `1` (BLOCK) plus `2` (INV-006 refusal) — the markdown branches on the code, not the prose.
3. **Enumerated verb** — `gov` is now a group-permit entry.
4. **Failure mode documented** — `commands/gov/compile.md` declares `tier_2_dependency: edikt` (refuse-and-direct-user is the default per ADR-029 Rule 4 fallback).

## Consequences

- **Good — Phase 11.5 ships contract-clean.** All four heredoc-replacement subcommands (`gov compile-history`, `gov gitignore-bootstrap`, `gov directive-check`, plus the routed-sources port into `bin/edikt doctor`) are authorized orchestration calls.
- **Good — Future Phase additions need no further amendment.** New `gov` subcommands (planned: `gov score`, `gov benchmark`) are covered without a new ADR.
- **Good — Symmetry across the verb list.** `migrate <subcommand>`, `sidecar <subcommand>`, `gov <subcommand>` all use the same group-permit shape. New maintainers reading Rule 3 see one consistent pattern.
- **Neutral — ADR-029 frontmatter updated.** ADR-029's `amended_by` field gains `ADR-033` (additive to the existing `ADR-031`).
- **Neutral — Tier-2 helper exit codes still meaningful and stable.** The existing `gov compile` exit codes (0/1) are joined by `gov compile-history` (0/1/2), `gov gitignore-bootstrap` (0/2), `gov directive-check` (0/2). All four documented in their `--help` text.
- **Neutral — INV-001 unchanged.** Tier-2 stays Go; tier-1 stays markdown. No new file types in either tier.

## Alternatives Considered

### A — Pin each new verb individually

- **Pros:** Maximally narrow.
- **Cons:** Phase 11.5 already adds three verbs; future phases will add more. Per-verb ADRs become a documentation tax with no security or correctness benefit.
- **Rejected because:** ADR-031 already established the group-permit precedent for `sidecar`; consistency wins.

### B — Read `gov compile` strictly and refuse Phase 11.5

- **Pros:** No amendment needed.
- **Cons:** The Phase 11.5 architectural debt (Python heredocs in tier-1) stays in the repo. Heredocs bypass every tier-2 invariant — not unit-tested, not benchmarked, not in the ADR-030 LLM-agnostic gate. Closing the debt is a v0.6.0 release-blocker.
- **Rejected because:** the architectural debt is real; refusing to migrate it leaves tier-1 carrying ~280 lines of Python that violates INV-001's "tier-2 belongs in Go" intent and ADR-030's "tier-2 LLM-agnostic" gate's coverage.

### C — Move the migrated logic into existing enumerated verbs (e.g. fold compile-history into `gov compile`)

- **Pros:** No amendment needed.
- **Cons:** Conflates pure deterministic state-machine work with the broader compile pipeline. Makes `gov compile` impossible to drive in isolation from a markdown caller. Loses the tight test surface (each subcommand's tests exercise just its own state machine).
- **Rejected because:** the subcommand-per-concern split is the same architectural pattern Phase 11 used for `lossless-check`; Phase 11.5 follows the same shape.

## ADR-029 Amendment

ADR-029 frontmatter gains `amended_by: [ADR-031, ADR-033]`. Per INV-002, no other content of ADR-029 is modified. Rule 3's verb list is updated to broaden `gov compile` into `gov <subcommand>`.

## Confirmation

- `bin/edikt gov compile-history --help` exits 0.
- `bin/edikt gov gitignore-bootstrap --help` exits 0.
- `bin/edikt gov directive-check --help` exits 0.
- `commands/gov/compile.md`, `commands/gov/_shared-directive-checks.md`, and `commands/doctor.md` no longer contain `python3 - <<` heredocs (verified via `grep -rn 'python3 - <<' commands/`).
- Each new subcommand has unit-test coverage under `tools/edikt/internal/<package>/` and integration-test coverage under `tools/edikt/cmd/gov/`.
- `tools/edikt/check/no-llm-in-tier-2.sh` covers the new Go source files (no new exemption entries required — Phase 11.5 is pure Go, no `claude` references).

## Directives


---

*Captured by edikt:adr — 2026-05-04*
