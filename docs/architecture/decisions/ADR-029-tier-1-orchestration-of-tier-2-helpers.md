---
type: adr
id: ADR-029
title: Tier-1 markdown commands may orchestrate tier-2 helpers via exit code
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-05-02T00:00:00Z
references:
  adrs: [ADR-015, ADR-021, ADR-022]
  invariants: [INV-001]
  prds: []
  specs: []
amends: ADR-015
---

# ADR-029 — Tier-1 markdown commands may orchestrate tier-2 helpers via exit code

## Status

**Accepted**

## Context

ADR-015 establishes the tier-1 / tier-2 split. Tier-1 is the markdown command surface installed by `install.sh` (`commands/**/*.md`, templates, hooks); INV-001 holds verbatim — pure markdown, copy-files install, no runtime dependency on tier-2. Tier-2 is the optional helper surface installed via `edikt install <helper>` (the Go binary at `tools/edikt/`, plus future helpers); it may depend on packages, ship binaries, and pin versions.

ADR-015's prohibition is unambiguous: "Tier-1 commands MUST NOT read from, write to, or depend on any tier-2 file or helper at runtime." That prohibition was correct for the v0.4.x / v0.5.x feature surface, where tier-2 was a single benchmark helper and tier-1 commands had no legitimate need to call it. Two structural shifts since v0.5.0 changed the calculus:

1. **ADR-022 — single Go binary replaces bash launcher.** The user-facing `edikt` binary is now a Go binary at `tools/edikt/`. Subcommands like `gov compile`, `migrate sidecars`, `verify`, `doctor`, `install`, and `upgrade` are all tier-2 today. ADR-021 / ADR-022 expanded tier-2's footprint from "one optional benchmark" to "the entire orchestration substrate."
2. **PLAN-sidecar-architecture Phase 12 — `edikt verify` runner.** The plan-execution loop in `commands/sdlc/plan.md` needs to run the criteria-sidecar verification step before flipping a phase row from in-progress to done. The verification step is a tier-2 binary (`bin/edikt verify <plan-id> --phase N`) — there is no markdown-only path that can execute and pass-fail the per-phase verify shell commands with the determinism and isolation that contract requires.

The result is a forced choice. Either:

- **(A) Tier-1 markdown ABSOLUTELY MUST NOT invoke tier-2 binaries**, in which case `commands/sdlc/plan.md` cannot gate phase-row flips on verify results, and the entire verify-runner workflow has to be re-architected as a separate user verb the human runs by hand between phases. This destroys the plan-execution UX.
- **(B) Tier-1 markdown MAY invoke tier-2 binaries under tightly-scoped rules**, in which case ADR-015's blanket prohibition needs to be amended.

Option (A) is the conservative read of ADR-015 and was the operating assumption through v0.5.0. Option (B) is the pragmatic read that reflects how the system actually works post-ADR-022. The v0.6.0 review (`IMPLEMENTATION REVIEW — 2026-05-02`, finding #13) flagged the contradiction: `commands/sdlc/plan.md:447` already invokes `bin/edikt verify` without a documented carve-out, in violation of ADR-015 as written. The fix is either to remove that invocation (option A) or to amend ADR-015 with a tightly-bounded carve-out (option B).

This ADR adopts option B with explicit guard rails so the carve-out cannot quietly grow into a general-purpose dependency.

## Decision

**Tier-1 markdown commands MAY invoke tier-2 binaries via `bin/edikt <subcommand>` for orchestration purposes, provided the four rules below all hold. The tier boundary is preserved structurally — tier-1 still installs without tier-2 — but the tier boundary at runtime becomes a contract about exit codes, not a prohibition on invocation.**

### Rule 1 — Absence detection is mandatory

A tier-1 markdown command that invokes `bin/edikt <subcommand>` MUST detect absence of the binary on PATH before invocation and emit a clear actionable error directing the user to `edikt install <helper>` (or to the documented install path for the helper in question). The absence-detection check MUST cite the helper by name in the error so the user knows what to install.

### Rule 2 — Exit code is the only contract

The markdown command MUST NOT depend on the binary's output SHAPE. Output may be displayed verbatim to the user (passthrough is fine), but parsing — extracting fields, splitting on whitespace, regex-matching against the binary's output — is forbidden. The contract surface between tier-1 and tier-2 is the exit code. Output drift in tier-2 is allowed; behavior coupling on output shape is not.

This is the structural reason the carve-out is safe: tier-2 can change its output format, its progress UI, its JSON schema, or its prose wording without breaking any tier-1 command, because no tier-1 command reads tier-2 output programmatically. Exit codes are the stable contract, and exit codes are documented per-helper in tier-2 command frontmatter.

### Rule 3 — Acceptable orchestration verbs are enumerated

Tier-1 markdown MAY invoke the following tier-2 verbs:

- `bin/edikt verify <plan-id> [--phase N]`
- `bin/edikt doctor`
- `bin/edikt migrate <subcommand>`
- `bin/edikt gov compile`
- `bin/edikt install <helper>`
- `bin/edikt upgrade`
- `bin/edikt use`
- `bin/edikt rollback`

Other binary calls remain forbidden in tier-1. Adding a new orchestration verb to this list requires an ADR amending this one — the list is exhaustive, not illustrative.

### Rule 4 — Failure-mode parity is documented per command

Each tier-1 command that invokes a tier-2 binary MUST document its failure-mode in command frontmatter:

- `on_absent: skip-with-warning` — the binary is absent; the command emits a one-line warning, skips the orchestration step, and proceeds in degraded mode. Used when the orchestration is advisory (e.g., a verify gate that is desirable but not strictly required for correctness).
- `on_absent: refuse-and-direct-user` — the binary is absent; the command refuses to proceed, prints the install directive, and exits non-zero. Used when the orchestration is load-bearing (the command's correctness depends on the binary's verdict).

The choice is per-command and is a documented contract — users reading the command's frontmatter can predict its behavior under absence without running it. Absent the frontmatter field, the default is `refuse-and-direct-user` (fail-closed).

### Tier-installation contract — unchanged

Tier-1 still installs without tier-2. `install.sh` does not install the Go binary; users get tier-1 markdown and nothing else by default. Helper install (including the `edikt` binary itself) is a separate explicit step. This ADR does not change that — it changes the runtime relationship between tier-1 commands and tier-2 binaries, not the install relationship.

### ADR-015 amendment

This ADR amends ADR-015. Specifically, ADR-015's directive "Tier-1 commands MUST NOT … depend on any tier-2 file or helper at runtime" is replaced (for the runtime aspect) with the four-rule contract above. ADR-015's other directives (tier declared in frontmatter, tier frozen at install time, uninstall is byte-equal, exact-version pinning, isolated environments, fail-fast prerequisites) remain unchanged.

ADR-015's frontmatter is updated to record the amendment via `amended_by: ADR-029`, mirroring the shape ADR-020 uses for ADR-028. The `**Status:**` line in ADR-015 is updated from `Accepted` to `Accepted (Amended by ADR-029)`. Per INV-002, no other content of ADR-015 is modified.

## Consequences

- **Good — `commands/sdlc/plan.md` becomes contract-compliant.** The verify-gate invocation at `commands/sdlc/plan.md:447` is now an authorized orchestration call rather than an undocumented violation. The command's frontmatter declares its absence-handling, and the call goes through the exit-code-only contract.
- **Good — Future tier-1 commands have a clear pattern.** Doctor, migrate, install, upgrade — all of these have legitimate orchestration cases where a markdown command needs to drive a tier-2 binary. The four rules give every such command a single recipe to follow.
- **Good — Tier boundary stays meaningful.** The carve-out is narrow (eight enumerated verbs), has explicit absence-handling, and forbids output parsing. Tier-2 still owns its evolution: it can change output, change JSON shapes, change progress UIs without coordinating with tier-1.
- **Good — Tier-1 install stays pure.** Users who never install the Go binary still get every tier-1 command. Commands declared `on_absent: skip-with-warning` continue to work degraded; commands declared `on_absent: refuse-and-direct-user` print the install directive and exit. Either way, no surprise crashes.
- **Bad — The "tier-1 has no tier-2 dependency" mental model loses some precision.** New maintainers reading ADR-015 in isolation might miss the amendment. Mitigation: the ADR-015 frontmatter's `amended_by` field surfaces the amendment, and the command frontmatter convention (`tier_2_dependency`, `on_absent`) makes any orchestration call visible at the command-file level.
- **Bad — The orchestration-verb list will grow.** New tier-2 verbs that need tier-1 orchestration will require ADR amendments. Mitigation: the bar is low (one ADR per added verb), and the list is short enough that growth is not expected to be frequent.
- **Neutral — The exit-code-only contract requires tier-2 helpers to keep exit codes meaningful and stable.** Helpers like `edikt verify` already document their exit codes (0 / 1 / 2 / 3) explicitly. New tier-2 helpers that want to be reachable from tier-1 must document the same.

## Alternatives Considered

### A — Keep ADR-015 unchanged; remove `bin/edikt verify` call from `commands/sdlc/plan.md`

- **Pros:** Tier boundary stays absolute. No carve-out to manage.
- **Cons:** Plan-execution loop loses its verification gate. Users have to run `edikt verify` by hand between phases, then transcribe results back into the plan file. The whole point of Phase 12's runner — automated phase-row gating — is destroyed.
- **Rejected because:** the verification gate is the load-bearing piece that makes phase tracking trustworthy. Manual transcription of pass/fail back into a markdown table will drift; the tool exists exactly to prevent that drift.

### B — Allow tier-1 to call tier-2 freely with no contract

- **Pros:** Maximum flexibility. No bookkeeping.
- **Cons:** Tier boundary becomes meaningless. Tier-2 helpers must keep output formats stable forever to avoid breaking tier-1 callers. The "compile your decisions into automatic enforcement" vision becomes a tangle of cross-tier coupling.
- **Rejected because:** the tier boundary is load-bearing for edikt's install model. A meaningful tier boundary requires a contract.

### C — Move `verify` (and similar orchestration verbs) into tier-1 as markdown commands

- **Pros:** No carve-out needed. Pure tier-1 architecture preserved.
- **Cons:** Markdown cannot reliably execute shell commands with timeouts, capture per-criterion exit codes, and persist a structured report. The verify runner is genuinely Tier-2 work; trying to do it in markdown reproduces every problem `tools/edikt/` was created to solve.
- **Rejected because:** tier-2 exists for exactly this class of work. Pretending markdown can do it produces brittleness.

### D — Make tier-1's tier-2 calls go through a Bash wrapper that abstracts the binary call

- **Pros:** Tier-1 markdown only invokes shell, never binaries directly. Wrapper handles absence detection.
- **Cons:** Adds a layer of indirection that has to be installed, maintained, and kept in sync with the binary. The wrapper is itself a runtime dependency — the problem moves, it doesn't go away.
- **Rejected because:** the wrapper would either be tier-1 (in which case it has to declare `on_absent` behavior anyway, and we may as well declare it directly on the command) or tier-2 (in which case we still have a tier-1 → tier-2 invocation, just one step removed).

## Confirmation

To verify this ADR is in effect:

- `commands/sdlc/plan.md` declares `tier_2_dependency: edikt verify` and an `on_absent:` value in its frontmatter.
- ADR-015's frontmatter contains `amended_by: ADR-029` and the `**Status:**` line reads `Accepted (Amended by ADR-029)`.
- An integration test (`test/integration/plan-tier2-absent.sh` or equivalent) exercises the absence path: with `bin/edikt` removed from PATH, the plan-command's verify-gate code path emits the documented install directive and proceeds (or refuses, per the command's declared `on_absent` value).
- Tier-1 markdown commands that invoke tier-2 binaries restrict their calls to the eight enumerated verbs in Rule 3.

## Directives


---

*Captured by edikt:adr — 2026-05-02*
