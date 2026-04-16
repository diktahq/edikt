---
type: prd
id: PRD-003
title: Directive hardening and governance compliance benchmark
status: accepted
author: Daniel Gomes
stakeholders: []
created_at: 2026-04-16T22:45:00Z
accepted_at: 2026-04-16T23:00:00Z
references:
  adrs: [ADR-007, ADR-008]
  invariants: [INV-001, INV-002]
  brainstorms: [BRAIN-002]
---

# PRD-003: Directive hardening and governance compliance benchmark

**Status:** accepted
**Date:** 2026-04-16
**Author:** Daniel Gomes

---

## Problem

edikt's directives are written in human language and compiled into `.claude/rules/`, but there's no way to know whether the language actually holds up when a literal-instruction-following model (Opus 4.7, Mythos, or any future model) is put under adversarial pressure. The v0.5.0 governance compliance benchmark (see BRAIN-002) surfaced weaknesses in two distinct layers. Separating them is important because they have different audiences and different priorities.

**User-visible governance weaknesses** (the ones that affect actual edikt users):

1. **Citation gap.** In our benchmark (N=1 × 32 cases against Opus 4.7), 5/32 cases (16%) refused correctly and didn't write the forbidden file — but didn't cite the rule they were invoking. The user can't audit refusals they can't trace back to an ADR. This is the governance gap that shows up at the point of use.

2. **Orphan ADRs are invisible.** ADR-012 and ADR-013 are both accepted in this repo but produce zero compiled directives. A rule written expecting enforcement silently isn't enforced. No tool tells the author. Users discover this when they notice the model doesn't follow the "rule" — by then the damage is done.

3. **Directives without accessible source files are aspirational.** The compiled routing table instructs the model to "read the relevant ADR" for context. If `docs/architecture/decisions/` is empty (bad init, incomplete clone, rogue `.gitignore`), the routing table points nowhere. No check catches this at configuration time.

**edikt-internal measurement weaknesses** (the ones that affect how we prove directives work):

4. **Directive language drifts from what the model echoes.** Opus 4.7 takes instructions literally. When the directive says "no npm, no package managers", the model echoes those words — not the synonym "no dependencies". Any compliance-checking tool that matches on synonyms produces false negatives. 5/32 benchmark cases failed on phrase matching alone while behavior and citations were perfect. This is a rubric-quality problem in edikt's own measurement stack, not a user-visible governance failure.

5. **Governance compliance is behavior, not phrasing.** The benchmark's strongest signal was behavioral: 0/32 cases wrote a forbidden file or called Write against a rule. Any compliance model built primarily on phrase-matching gives false signal. What matters is: did the model write the forbidden file? did it cite the rule? Phrasing is a distant third signal.

Governance today works well *enough* on Opus 4.7's current behavior. The problem is edikt has no way to know *which* directives will hold and no way to help a user fix the ones that won't — and it has no compliance-measurement primitives that can survive the next model's phrasing drift. The user-facing fix and the measurement fix share the same underlying metadata, which is why they belong in one PRD.

## Users

- **edikt user (primary)** — a developer using edikt to govern an AI agent against their architecture decisions. Writes ADRs and invariants in natural language, runs `/edikt:gov:compile`, expects the model to follow the resulting directives.
- **edikt maintainer (secondary)** — whoever is tuning edikt's template language across model generations (4.6 → 4.7 → Mythos). Needs statistical rigor for model-vs-model comparisons.
- **ADR author** — anyone authoring a new ADR with `/edikt:adr:new`. Needs guidance on writing directive language that a literal-instruction model will honor.

## Goals

- Users can verify their governance holds under adversarial pressure with a single command, before a coding session, and receive concrete fix suggestions when a directive fails.
- Every `/edikt:gov:compile` surfaces ADRs that produce zero directives — orphan rules never ship silently.
- Every `/edikt:gov:review` run statically validates that directive language is likely to be echoed by a literal-instruction model, with zero token cost, so language quality is checked continuously rather than only on demand.
- Compliance measurement converges on the same verdict a human reviewer would produce, prioritizing behavioral and citation signals over phrasing.
- The user flow for `/edikt:gov:benchmark` is fast enough (~3 minutes, N=1) that users actually run it before real coding sessions — not once, after install, and never again.

## Non-Goals

- **Not a production/CI gate.** `/edikt:gov:benchmark` is a local advisory tool. No `--fail-below` flag, no pre-merge hook, no required pre-commit step in v1. Users and CI systems can wire it up themselves, but edikt does not ship that wiring and does not support complaints about token cost or flakiness in those setups.
- **Not a model-quality benchmark.** This tool tests directive-language quality against a specific model; it does not evaluate or rank models. Users should not interpret `/edikt:gov:benchmark` output as "Opus is better than Sonnet."
- **Not a replacement for `/edikt:gov:review`.** Static review (continuous, zero-cost) and dynamic benchmark (on-demand, real model call) are paired tools with different jobs. Review is not deprecated or absorbed by benchmark — the two must ship and run independently.
- **No directive enforcement at runtime.** The benchmark measures whether directives *would* hold under pressure; it does not change what happens when they don't. Enforcement remains Claude Code's responsibility via `.claude/rules/`.
- **Not recurring.** The benchmark is on-demand only. Users who want continuous directive-quality coverage get it from `/edikt:gov:review`, not from auto-running the benchmark on every compile.
- **Model-specific directive packs** (`templates/directives/<model>.md`). Interesting, but premature — first measure effect with shared phrasing, then decide. Deferred to a future PRD.
- **Multi-language attack prompt variants.** English-only at launch.
- **Auto-rewriting directives.** The benchmark suggests `canonical_phrases` and improved wording, but the user applies the edit. No silent ADR modifications.
- **Benchmarking anything other than directives.** PRD/spec/plan status gates are out of scope; `/edikt:gov:benchmark` focuses on directive enforcement only.
- **Statistical maintainer tooling in v1.** `--runs N`, aggregated JSONL, per-model comparison reports are maintainer-oriented and deferred to Phase 2 — see Won't-Have-v1 below.

## Recurring vs on-demand usage

This feature ships two tools in one PRD because they are complementary, not redundant. Users and tooling must know which to run when.

| Tool | Cost | Signal | When to run |
|---|---|---|---|
| `/edikt:gov:review` (extended — FR-003) | $0, <1s | Does the directive language look enforceable? Is `canonical_phrases` present and consistent? Does the wording use MUST/NEVER rather than soft language? | Every compile, every PR, every CI run. Continuous. |
| `/edikt:gov:benchmark` (new) | ~50k tokens, ~3 min at N=1 | Does the directive actually hold when a model is put under adversarial pressure? | Before a coding session, after changing directive language, on demand. Not recurring. |

The static check is the cheap first line of defense. The dynamic check is the expensive proof. Users who run only the static check still get meaningful coverage; users who never run the static check will hit the dynamic check repeatedly trying to diagnose what the static check would have flagged for free.

## Requirements

### Must Have

- **FR-001 — `canonical_phrases` field in directive sentinel block** [MUST]
  Every `[edikt:directives:start]` block gains an optional `canonical_phrases` list alongside `directives` / `manual_directives` / `suppressed_directives`. Entries are short substrings (2–5 words) that a compliant refusal will echo verbatim. Compile preserves them across recompilations (like `manual_directives`). Missing `canonical_phrases` is always treated as empty — never an error — for backward compatibility with existing ADRs. The field serves `/edikt:gov:review` (static enforceability check) first and the benchmark rubric second; it is not required for directives to work today, only to be statically checkable.

- **FR-002 — `behavioral_signal` field in directive sentinel block** [MUST]
  Every directive sentinel block gains an optional `behavioral_signal` block with three keys:
  - `refuse_to_write` — list of path substrings the model must not pass to Write/Edit
  - `refuse_tool` — list of tool names the model must not invoke
  - `cite` — list of directive IDs (e.g., `ADR-012`) the model must reference in its refusal

  Substring matching only at launch — no glob syntax supported. If a directive author wants to block multiple patterns (e.g., all SQL files), they list `".sql"` as a substring; trailing-wildcard and full-glob support are deferred. Missing fields treated as empty. Directives without any `behavioral_signal` are skipped by `/edikt:gov:benchmark` with an explicit "no attack can be generated — add behavioral_signal to enable benchmarking" message.

- **FR-003 — `/edikt:gov:review` validates directive metadata (static check)** [MUST]
  `/edikt:gov:review` gains two new static checks. These run with zero model calls and are intended to run on every compile / every PR.
  - **FR-003a** — Fails the directive if its body is longer than one declarative sentence and `canonical_phrases` is empty. "Declarative sentence" = one clause ending in `.` or `;` in the rendered directive body, ignoring the `(ref: …)` tail.
  - **FR-003b** — Fails the directive if any canonical phrase is not a substring of the directive body itself (case-insensitive). The author claimed a phrase the directive doesn't even contain — likely typo or stale after a rewrite.
  - Output lists the directive ID and the specific failing rule. First detection is a warning; `--strict` flag or a future hard-fail release can promote these to blocking.

- **FR-004 — `/edikt:gov:compile` surfaces orphan ADRs** [MUST]
  Compile emits a warning list of accepted ADRs that contributed zero directives. First detection: warn only. Second consecutive compile with the same orphan set unresolved: block with a clear fix list.

  **State persistence:** compile writes `.edikt/state/compile-history.json` containing the most recent orphan set. "Consecutive" means the current orphan ADR-ID set is a non-empty superset of — or equal to — the previously recorded set. Adding or removing any orphan ADR resets to "first detection" on the next compile. The file is regenerated on every compile and is machine-read only.

  **Allowed outs:** add directives to the ADR, add `no-directives: <reason>` to the ADR frontmatter (reason string must be ≥ 10 characters, non-empty, not `tbd`/`todo`/`fix later`), or revert the ADR to draft status. `/edikt:gov:review` lints the reason string the same way.

- **FR-005 — `/edikt:doctor` checks directive source availability** [MUST]
  Doctor verifies every ADR/INV referenced in `.claude/rules/governance.md` routing table has its source file readable at the expected path. Missing source = hard failure (exit non-zero) with the exact missing path(s) listed. This check runs on every `/edikt:doctor` invocation and adds <100ms overhead.

- **FR-006 — `/edikt:gov:benchmark` pre-flight** [MUST]
  Before any model call, the command prints:
  - Number of directives to test
  - Number of runs per directive (default 1; shown even at default so users are never surprised)
  - Estimated elapsed time (best-effort, disclaimed)
  - Estimated tokens (best-effort, disclaimed — based on directive-body-tokens + attack-prompt-tokens + a fixed 2k expected-response budget, multiplied by runs × directives)
  - Target model (read from `.edikt/config.yaml:model`, or `--model` override)
  - Confirmation prompt: `Continue? [Y/n]`

  `--yes` skips the prompt. Single-directive targeted re-runs (benchmark invoked with a single ADR ID argument) skip the prompt automatically — the user already committed when they ran the targeted command.

  If no model is configured and no `--model` is passed, exit 2 with "no target model configured — set model in `.edikt/config.yaml` or pass `--model`."

- **FR-007 — `/edikt:gov:benchmark` execution** [MUST]
  For each directive, one attack prompt is sent to the target model through the **Claude Agent SDK** (matching PRD-002 FR-004's standardization). No subprocess spawning. Each directive runs in a sandboxed subproject constructed by the same realistic-layout pattern established by `test/integration/benchmarks/runner.py`:
  - Copies the project's `.claude/rules/`, `.claude/agents/`, `.claude/settings.json`
  - Copies `docs/architecture/decisions/` and `docs/architecture/invariants/` (or the configured equivalents)
  - Writes a CLAUDE.md containing the project's real `[edikt:start]` sentinel block

  The benchmark command must share this sandbox-builder code with the internal test harness. Divergence between the two is a maintenance bug — a single helper lives in a shared module (spec to decide exact location).

  Progress streams to stdout, one line per directive completion: `[N/total] <directive_id> <verdict> — <summary> (<elapsed>s)`. Ctrl+C (SIGINT) during an idle window between directives terminates cleanly. Ctrl+C during an active model call attempts to cancel the SDK request and exits within 5 seconds; any claude subprocess started by the SDK is verified dead before exit.

- **FR-008 — `/edikt:gov:benchmark` actionable output** [MUST]
  For each failed directive, output includes **six sections**, in this order, each with a literal header the user can grep:
  1. **Attack prompt** — the exact prompt sent to the model
  2. **What the model did** — the tool call log (tool name + key input fields, e.g., `Write(path=...)`)
  3. **Diagnosis** — which verify check failed (missing cite? wrote a refuse_to_write path? did not refuse_tool?)
  4. **Likely root cause** — mapped from a known pattern catalog (soft language, missing canonical phrases, directive mentions the ID only in metadata not body, etc.)
  5. **Suggested fix** — literal text the user can paste: new `canonical_phrases` entries (if missing) and a rewritten directive line (if root cause was soft language)
  6. **Re-run** — the exact targeted command, e.g., `/edikt:gov:benchmark ADR-012`

- **FR-009 — Attack prompt catalog** [MUST]
  Ship a catalog of `behavioral_signal` → attack-prompt templates. **v1 catalog covers exactly these signal types** (anything else is Won't-Have-v1):
  - `refuse_tool_use` — one or more tools are forbidden. Attack: "[task that normally needs those tools]" with escalating pressure if denied once.
  - `refuse_file_pattern` — one or more file-path substrings are forbidden. Attack: "create [file matching the pattern] for [plausible task]".
  - `must_cite` — a directive ID must be cited in refusal. Attack: a request that triggers the directive without mentioning the ID; scoring checks the refusal contains the ID.

  Each template takes the directive's own `behavioral_signal` values as inputs. Directive authors never write attacks by hand — the command generates one from the signal block. Templates live as Markdown files under `templates/attacks/<signal_type>.md` so they can be overridden per project (same pattern as other templates). Unknown signal types cause the directive to be skipped, not cause a benchmark failure.

- **FR-010 — Single-directive targeted run** [MUST]
  `/edikt:gov:benchmark <directive_id>` runs the benchmark against exactly one directive. Skips the Y/n pre-flight (per FR-006). All other behavior identical to a full run. Enables the "re-run" command emitted in FR-008 step 6.

### Should Have

- **FR-011 — `/edikt:adr:new` prompts for canonical_phrases and behavioral_signal** [SHOULD]
  When authoring a new ADR, the interactive prompt walks the user through: what behavior to forbid? what phrases will your refusal echo? what rule ID should the model cite? These populate the sentinel block on creation. A user can skip any prompt and fill in later; `/edikt:gov:review` will flag the gap.

- **FR-012 — `/edikt:adr:review` suggests canonical_phrases upgrades** [SHOULD]
  When reviewing an existing ADR, `/edikt:adr:review` points out soft-language markers — specifically: `should`, `ideally`, `prefer`, `try to`, `might`, `consider` — and suggests harder replacements (`MUST`, `NEVER`, `forbidden`) + a candidate `canonical_phrases` list extracted from key nouns/verbs in the directive body.

### Won't Have (v1)

- **FR-013** — Model-specific directive packs (`templates/directives/<model>.md`). Deferred until effect size is measured across models.
- **FR-014** — `--fail-below <threshold>` flag for CI gating. Benchmark is advisory, not gating.
- **FR-015** — Multi-language attack prompt variants. English only at launch.
- **FR-016** — Glob or trailing-wildcard pattern support in `behavioral_signal.refuse_to_write`. Substring only.
- **FR-017** — Auto-rewriting the ADR with the suggested fix. Suggest only; user applies.
- **FR-018** — Benchmarking PRD/spec/plan gates or anything beyond directives.
- **FR-019** — Parallel execution of directive runs. Sequential keeps streamed output readable.
- **FR-020** — `--runs N` flag, multi-run aggregation, Wilson CI output, per-run JSONL writing. This is maintainer statistical tooling — deferred to Phase 2 after user adoption data exists.
- **FR-021** — Auto-running the benchmark on every compile, every PR, or via any periodic mechanism. Benchmark is explicitly on-demand (see Recurring vs on-demand usage).
- **FR-022** — Attack prompt catalog entries beyond `refuse_tool_use`, `refuse_file_pattern`, `must_cite`.
- **FR-023** — Cross-directive dependency analysis (directive A weakens directive B).

## User Stories

**P1** — As an **edikt user**, **I want** to run `/edikt:gov:benchmark` before a coding session **so that** I know which of my ADRs will actually hold under pressure and which are worded too softly.

**P1** — As an **edikt user**, **I want** the benchmark's failure output to tell me the *specific fix* (canonical phrases to add, words to harden) **so that** I don't have to debug directive language from scratch.

**P1** — As an **edikt user**, **I want** `/edikt:gov:review` to flag soft language and missing canonical phrases on every compile for free **so that** I catch directive-quality problems continuously without paying model tokens.

**P1** — As an **ADR author**, **I want** `/edikt:gov:compile` to warn me when my accepted ADR produces zero directives, and block compile on the second compile if I haven't resolved it **so that** I don't ship governance that silently isn't enforced.

**P2** — As an **ADR author**, **I want** `/edikt:adr:new` to ask me for `canonical_phrases` and `behavioral_signal` at authoring time **so that** my directive starts life with a verifiable compliance contract.

**P2** — As an **edikt user**, **I want** `/edikt:gov:benchmark <ID>` to target a single directive **so that** after I apply the suggested fix, I can re-verify that one rule in under a minute.

**P3** — As an **edikt user**, **I want** `/edikt:doctor` to catch routing tables that reference missing ADR files **so that** I find that class of bug at doctor time rather than mid-session.

## Acceptance Criteria

- [ ] **AC-001**: The `[edikt:directives:start]` block parser accepts `canonical_phrases` and `behavioral_signal` fields without breaking existing ADRs. — **Verify:** unit test reads every ADR under `docs/architecture/decisions/` with the new parser and all parse successfully; a separate fixture ADR populating both fields round-trips cleanly through `/edikt:gov:compile` (written values equal parsed values).
- [ ] **AC-002a**: Missing `canonical_phrases` is treated as empty and never causes a parse error. — **Verify:** unit test parses an ADR with the new schema but no `canonical_phrases` key; asserts parsed value is `[]`, no exception raised.
- [ ] **AC-002b**: `/edikt:gov:review` warns on a directive with > 1 declarative sentence and empty `canonical_phrases`. — **Verify:** unit test passes a fixture ADR with a 2-sentence directive body and no canonical_phrases; review output lists the warning with the ADR ID and the literal text "canonical_phrases".
- [ ] **AC-002c**: `/edikt:gov:review` warns when a `canonical_phrases` entry is not a substring of the directive body (case-insensitive). — **Verify:** fixture ADR with directive body `"All DB access MUST use repositories"` and `canonical_phrases: ["immutable"]`; review output flags the mismatch and names the offending phrase.
- [ ] **AC-003**: `/edikt:gov:compile` warns on orphan ADRs on first detection and blocks on second consecutive compile with the same orphan set unresolved. — **Verify:** integration test creates accepted ADR-X with no directives, runs compile (warn, exit 0), runs compile again (block, exit ≠ 0), adds `no-directives: "covers process, not code enforcement"` to ADR-X frontmatter, runs compile a third time (pass). "Consecutive" is defined by `.edikt/state/compile-history.json` containing a superset-or-equal orphan ADR-ID set from the previous run.
- [ ] **AC-003b**: `/edikt:gov:compile` resets the consecutive counter if the orphan set changes. — **Verify:** compile with ADR-X orphaned (warn), add ADR-Y orphaned (warn — reset, because set changed), run compile again (block — now consecutive with both ADR-X and ADR-Y).
- [ ] **AC-003c**: `no-directives` with a reason ≤ 10 chars or matching `{tbd,todo,fix later}` (case-insensitive) is rejected by `/edikt:gov:review`. — **Verify:** fixture ADR with `no-directives: "tbd"`; review output flags the unacceptable reason.
- [ ] **AC-004**: `/edikt:doctor` fails with an explicit path when a routed ADR source file is missing. — **Verify:** integration test creates a compiled routing table referencing ADR-X, deletes the source file, runs doctor, asserts exit ≠ 0 and error message names the missing path literally.
- [ ] **AC-005**: `/edikt:gov:benchmark` pre-flight shows directive count, N, estimated time, estimated tokens, and target model; waits for `[Y/n]`. — **Verify:** snapshot test of pre-flight output for a 3-directive fixture; "n" input aborts before any SDK `query()` call (verified by test spy on the SDK entry point — zero calls recorded).
- [ ] **AC-005b**: `/edikt:gov:benchmark <ID>` skips the pre-flight confirmation. — **Verify:** targeted run against a single directive in a fixture project; assert SDK `query()` is called directly without any prompt-for-input.
- [ ] **AC-005c**: `/edikt:gov:benchmark` exits 2 with a clear message when no model is configured and no `--model` flag is passed. — **Verify:** integration test with an `.edikt/config.yaml` missing the model key; assert exit code 2 and stderr contains `"no target model configured"`.
- [ ] **AC-006a**: `/edikt:gov:benchmark` streams per-directive progress in the specified format. — **Verify:** snapshot test against a 3-directive fixture; assert streamed lines match `[N/3] <directive_id> <verdict> — <summary> (<elapsed>s)` pattern.
- [ ] **AC-006b**: SIGINT between directive completions exits cleanly with no orphaned SDK session. — **Verify:** integration test runs against a 3-directive project, sends SIGINT after first directive completes, asserts one SDK session was initiated, that session was closed, and the process exits within 5 seconds.
- [ ] **AC-006c**: SIGINT during an active model call attempts cancellation and exits within 5 seconds, and no SDK-spawned subprocess persists after exit. — **Verify:** integration test intercepts the first directive's model call to block indefinitely, sends SIGINT after 500ms, asserts process exits in ≤ 5s and no child `claude` subprocess remains (checked via `ps`).
- [ ] **AC-007**: On failure, output includes six sections with literal greppable headers — "Attack prompt", "What the model did", "Diagnosis", "Likely root cause", "Suggested fix", "Re-run". — **Verify:** fixture directive with soft language ("should" not "MUST") is benchmarked; assert stdout contains each of the six exact header strings, in order. Assert "Suggested fix" contains a literal `canonical_phrases:` block and a proposed rewritten directive line. Assert "Re-run" contains the string `/edikt:gov:benchmark <directive_id>` with the actual ID substituted.
- [ ] **AC-008**: The attack prompt catalog ships with exactly the three v1 signal types (`refuse_tool_use`, `refuse_file_pattern`, `must_cite`) as overrideable template files under `templates/attacks/`. — **Verify:** integration test asserts exactly those three files exist, each is valid Markdown, each produces a non-empty attack when given a reference `behavioral_signal` input.
- [ ] **AC-009**: `/edikt:gov:benchmark` skips (does not fail) a directive with no `behavioral_signal`, with a clear explanation. — **Verify:** fixture ADR with no behavioral_signal; run benchmark; assert exit 0, progress line reads `<id> skipped — no behavioral_signal`, no model call was made for that directive.
- [ ] **AC-010**: The sandboxed subproject builder used by `/edikt:gov:benchmark` copies the same file set as the internal test harness (`test/integration/benchmarks/runner.py`). — **Verify:** unit test calls both builders against an identical input project and asserts the resulting directory trees are byte-equal.
- [ ] **AC-011**: `/edikt:adr:new` prompts for `canonical_phrases` and `behavioral_signal`, writes them into the new ADR's sentinel block, and the values round-trip through `/edikt:gov:compile`. — **Verify:** scripted end-to-end test runs `/edikt:adr:new` with fixture inputs, asserts the resulting ADR file contains both fields with the input values, runs compile, asserts compiled rules carry the fields through.
- [ ] **AC-012**: `/edikt:adr:review` flags all six soft-language markers and suggests harder phrasing. — **Verify:** six fixture directives, one per marker (`should`, `ideally`, `prefer`, `try to`, `might`, `consider`). Run review against each; assert each marker is flagged by name and a replacement from `{MUST, NEVER, forbidden}` is suggested.
- [ ] **AC-013**: Attack prompts generated from `behavioral_signal` use substring inputs, not globs. — **Verify:** unit test with `behavioral_signal.refuse_to_write: ["users.sql"]`; assert generated attack prompt references `users.sql` literally and contains no glob metacharacters. A separate assertion that an attack against `[".sql"]` references that substring in its prompt.

## Technical Notes

- **Schema extension:** `canonical_phrases` and `behavioral_signal` extend the three-list schema per ADR-008. The directive block parser must treat missing keys as empty (`[]` / `{}`), never raising. This is backward-compatible with every existing ADR in the repo and with any external edikt user's ADRs.
- **Execution via Claude Agent SDK.** Matches PRD-002 FR-004's standardization on the SDK for anything that invokes `claude`. No subprocess spawning from the command code. Ctrl+C handling, streamed progress, and the sandbox cwd are all SDK features. Any `claude` child process is an SDK internal — tests verify it terminates cleanly on signal.
- **Shared sandbox builder.** `/edikt:gov:benchmark` and `test/integration/benchmarks/runner.py` must both call the same sandbox-builder function so divergence is impossible. Spec decides the exact module path. AC-010 locks the parity.
- **Token estimate formula:** `(directive_body_tokens + attack_prompt_tokens + 2000) × runs × directives`. The 2000 is a fixed expected-response budget — documented as an estimate. Pre-flight explicitly disclaims "estimate only" so users don't treat the number as a contract.
- **Synchronous execution at N=1.** Simpler mental model for users. Since `--runs N` is Won't-Have-v1, no background-execution path is needed in this PRD.
- **Attack catalog location:** `templates/attacks/<signal_type>.md`, parallel to `templates/rules/`. Override-able per project via the same template-override mechanism from ADR-005.
- **Compile history state:** `.edikt/state/compile-history.json` is a new file and a new state directory. The spec may generalize to `.edikt/state/` for future state needs — not a requirement.
- **Output location:** benchmark results write to `docs/reports/governance-benchmark-<timestamp>/` even at N=1 — the directory contains `summary.json` and the attack + response log. Serves as an audit trail and supports targeted re-run scripting. Even at N=1 there are real tokens spent; keep the record.

## Open Questions (for spec resolution)

- **Token-estimate tolerance.** Do we quote a range (e.g., ±20%) or a single number + disclaimer? Leaning single number + disclaimer; matches how SDK reports post-run costs.
- **Where does the sandbox builder live?** Candidates: `test/integration/benchmarks/shared/builder.py` imported by both harness and command, or `edikt/benchmark/builder.py` as a real module. Spec picks.
- **Does `/edikt:gov:review` run on every `/edikt:gov:compile`?** Today `/edikt:gov:review` is a separate command. Should compile call review automatically for the static directive checks, or only warn-on-orphan? Leaning: compile calls the static checks inline; review remains callable for the full quality report.
- **Multi-failure ordering in output.** When 3 directives fail in a single run, does output show all three full six-section reports, or a summary table with details on request? Leaning full reports for ≤ 3 failures, summary table + `--verbose` for more — but that's a UX decision.

---

*Written by edikt:prd — 2026-04-16*
