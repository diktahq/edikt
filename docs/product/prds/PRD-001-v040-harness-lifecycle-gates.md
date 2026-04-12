---
type: prd
id: PRD-001
title: "v0.4.0 — Harness Improvements, Artifact Lifecycle, Quality Gate UX"
status: accepted
author: Daniel Gomes
stakeholders: []
created_at: 2026-04-11T01:30:00Z
references:
  adrs: [ADR-001, ADR-004]
  invariants: [INV-001]
  source_documents:
    - docs/plans/PLAN-long-running-harness.md (phases 2, 3, 5, 9)
    - docs/internal/product/prds/PRD-005-edikt-v4-control-plane.md (R7, R11)
  existing_artifacts:
    - docs/plans/artifacts/phase-context-handoff-example.md
    - docs/plans/artifacts/plan-criteria-schema.yaml
    - docs/plans/artifacts/experiment-evaluator-spec.md
---

# PRD-001: v0.4.0 — Harness Improvements, Artifact Lifecycle, Quality Gate UX

**Status:** accepted
**Date:** 2026-04-11
**Author:** Daniel Gomes

---

## Problem

edikt's SDLC chain (PRD → spec → artifacts → plan → execute → drift) governs the full build cycle, but the execution harness has four gaps that cost engineers time and trust:

1. **Plans don't learn from failures.** When a phase fails evaluation, the plan has no memory of what failed or how many times. The generator re-attempts blind — same mistake, same failure, wasted tokens and time. There's no escalation when a criterion is stuck.

2. **Context dies at phase boundaries.** Each phase starts cold. The generator doesn't know which files the previous phase produced, which ADRs are relevant, or what the artifact flow looks like. PostCompact recovery injects the plan phase but not the reading list. Engineers manually re-explain context every phase.

3. **Enforcement has gaps.** Quality gates fire but have no override UX — the hook asks Claude to handle it, with no logging or re-fire prevention. Artifact lifecycle states exist (`draft`, `accepted`) but aren't enforced uniformly — plan doesn't warn on draft artifacts, drift doesn't filter by status, doctor doesn't flag stale drafts.

4. **Experiment evaluation is brittle.** The experiment runner uses grep assertions that can't distinguish intent from coincidence. A semantic evaluator exists in design (experiment-evaluator-spec.md) but hasn't been built. Without it, experiments miss violations that pattern matching can't catch.

## Users

Engineers who use edikt's SDLC chain to build features with Claude Code — from PRD through implementation. Specifically:

- **Solo engineers** running multi-phase plans who hit repeated failures and lose context across compactions
- **Team leads** who configure quality gates and need override accountability
- **Anyone running experiments** to measure governance effectiveness

## Goals

- Plans that track failure history and escalate when stuck — no more blind re-attempts
- Phase boundaries that carry forward the exact files and decisions the next phase needs
- Machine-readable criteria alongside plan markdown — evaluators can read/write status without parsing prose
- Semantic experiment evaluation that catches violations grep can't see
- Quality gate overrides that are logged with git identity and don't re-fire after acknowledgment
- Uniform artifact lifecycle enforcement across plan, drift, and doctor commands
- Configurable evaluator with headless execution for CI and bias-free evaluation

## Non-Goals

- Multi-agent orchestration or parallel phase execution (per ADR-004: agents are advisors only)
- Automatic resolution of gate findings (gates require human judgment)
- Full status lifecycle UI/dashboard (command output is sufficient)
- Backporting LLM evaluator to all experiment fixtures (start with fixture 08, expand later)
- Dependency gating between plan phases (harness plan Phase 4 — deferred to a future release)

## Requirements

### Must Have

**Iteration tracking + backoff:**
- FR-001: Plan progress table includes `Attempt` column with `N/max` format [MUST]
- FR-002: Plan supports `stuck` status with human decision prompt after max iterations [MUST]
- FR-003: After each evaluation FAIL, forward fail reasons to the next generator prompt [MUST]
- FR-004: Same criterion ID failing 3 consecutive times (regardless of reason) triggers a warning with escalation [MUST]
- FR-023: Plan statuses: `pending`, `in-progress`, `evaluating`, `stuck`, `done`, `skipped` [MUST]

**Phase context handoff:**
- FR-005: Each plan phase has a `Context Needed:` field listing files the generator must read [MUST]
- FR-006: Plan includes an Artifact Flow Table mapping producing phases to consuming phases [MUST]
- FR-007: PostCompact hook injects context file list alongside plan phase and attempt count [MUST]

**Structured criteria sidecar:**
- FR-008: `/edikt:sdlc:plan` emits `PLAN-{slug}-criteria.yaml` alongside plan markdown. This is the runtime file; `docs/plans/artifacts/plan-criteria-schema.yaml` is the reference schema. [MUST]
- FR-009: Criteria sidecar tracks per-criterion status (pending/pass/fail), fail_reason, fail_count, verify command [MUST]
- FR-010: Evaluator reads and updates the criteria sidecar after each evaluation. Depends on FR-008 and FR-009. [MUST]

**LLM evaluator in experiments:**
- FR-011: Experiment runner supports `--llm-eval` flag for LLM-based evaluation [MUST]
- FR-012: LLM evaluator uses skeptical prompt — "assume violations until proven." Verify: evaluator prompt template contains skeptical stance language. [MUST]
- FR-013: When both grep and LLM evaluator run, the LLM verdict is the final verdict. Grep serves as a fast pre-check only. [MUST]
- FR-014: Severity tiers assigned by fixture author in `evaluator-criteria.yaml`. Three verdicts: PASS (all critical + important pass), WEAK PASS (all critical pass, 1+ important fail — surfaces warning, doesn't block), FAIL (any critical fails — blocks). Informational findings are logged but never affect the verdict. [MUST]

**Quality gate UX:**
- FR-015: Quality gate override writes to `~/.edikt/events.jsonl` with git identity (name + email from `git config`) [MUST]
- FR-016: Gate override check prevents re-firing on the same finding within a session. A session is a single Claude Code invocation, from start to exit. Override expires when the session ends. [MUST]

**Artifact lifecycle enforcement:**
- FR-017: `/edikt:sdlc:plan` warns if any spec artifact is still `draft` [MUST]
- FR-018: `/edikt:sdlc:drift` only validates against `accepted` or `implemented` artifacts, skips `draft` [MUST]
- FR-019: `/edikt:doctor` flags artifacts stuck in `draft` for more than 7 days (by mtime) [MUST]

**Evaluator configuration:**
- FR-029: Evaluator config section in `.edikt/config.yaml` with `evaluator.preflight` (default: true), `evaluator.phase-end` (default: true), `evaluator.mode` (headless | subagent, default: headless), `evaluator.max-attempts` (default: 5), `evaluator.model` (sonnet | opus | haiku, default: sonnet) [MUST]
- FR-030: Headless evaluator runs as a separate `claude -p` invocation with `--bare`, `--disallowedTools "Write,Edit"`, and `--output-format json`. Zero shared context with the generator session. [MUST]
- FR-031: When `evaluator.preflight: false`, plan step 11 (pre-flight criteria validation) is skipped [MUST]
- FR-032: When `evaluator.phase-end: false`, phase-end evaluation is skipped. Criteria sidecar is still emitted. [MUST]

### Should Have

- FR-020: Plan auto-promotes artifact status `accepted → in-progress` when a phase referencing it starts [SHOULD]
- FR-021: Drift auto-promotes artifact status `in-progress → implemented` when drift finds no violations [SHOULD]
- FR-022: LLM evaluator logs token usage in experiment metadata [SHOULD]
- FR-024: Phase startup governance directive: "Before implementing, read every file in Context Needed" [SHOULD]

### Won't Have (v1)

- FR-025: Multi-platform gate configuration (Claude Code only per ADR-001)
- FR-026: Automatic gate finding resolution
- FR-027: Backporting LLM evaluator to experiment fixtures 01-07
- FR-028: Visual status lifecycle dashboard
- FR-033: Dependency gating between plan phases (harness plan Phase 4)

## User Stories

**P1** — **As an** engineer running a multi-phase plan, **I want** the plan to track how many times each phase has been attempted and what failed **so that** I don't waste tokens on blind re-attempts and I know when to intervene.

**P2** — **As an** engineer starting a new phase, **I want** the plan to tell me exactly which files and decisions from previous phases I need to read **so that** I don't start cold and miss context that was established earlier.

**P3** — **As an** engineer whose plan produces acceptance criteria, **I want** those criteria in a machine-readable sidecar file **so that** the evaluator can track pass/fail status without parsing markdown prose.

**P4** — **As a** team lead who configured quality gates, **I want** gate overrides logged with the engineer's git identity **so that** I have accountability when critical findings are bypassed.

**P5** — **As an** engineer running governance experiments, **I want** a semantic evaluator that reads generated code and judges violations **so that** I catch issues that grep assertions miss (wrong intent, missing context, correct pattern in wrong place).

**P6** — **As an** engineer using the SDLC chain, **I want** the plan command to warn me when artifacts are still draft **so that** I don't start implementing against unreviewed designs.

**P7** — **As an** engineer running experiments, **I want** critical findings to block the verdict while informational ones are logged but don't affect the result **so that** I can distinguish noise from real violations.

**P8** — **As an** engineer running drift detection, **I want** drift to skip draft artifacts **so that** I'm not flagged for work that hasn't been finalized.

**P9** — **As an** engineer, **I want** doctor to flag artifacts stuck in draft for over a week **so that** I notice forgotten designs before they go stale.

**P10** — **As an** engineer, **I want** to choose whether the evaluator runs headless or as a subagent **so that** I get full context isolation in CI and can fall back to in-session evaluation when needed.

## Acceptance Criteria

- [ ] AC-001: Plan progress table shows `| Phase | Status | Attempt | Updated |` — Verify: generate a plan and inspect table format
- [ ] AC-002: After 3 consecutive failures on the same criterion ID, plan output includes escalation warning — Verify: simulate with test fixture
- [ ] AC-003: Plan phase includes `Context Needed:` field with file paths — Verify: generate plan from spec with artifacts and inspect
- [ ] AC-004: Plan includes Artifact Flow Table — Verify: inspect generated plan markdown
- [ ] AC-005: `PLAN-{slug}-criteria.yaml` emitted alongside plan — Verify: check file exists after plan generation
- [ ] AC-006: Criteria YAML has per-criterion status, verify command, fail_reason, fail_count fields — Verify: parse YAML and validate against reference schema
- [ ] AC-007: `--llm-eval` flag triggers LLM evaluation in experiment runner — Verify: run fixture 08 with flag, check eval output
- [ ] AC-008: LLM evaluator produces per-criterion PASS/FAIL with file:line evidence — Verify: inspect evaluator output format
- [ ] AC-009: Gate override writes entry to `~/.edikt/events.jsonl` with git name and email — Verify: trigger gate, override, check file
- [ ] AC-010: Second gate fire on same finding within same Claude Code session is skipped — Verify: trigger same gate twice, second is silent
- [ ] AC-011: `/edikt:sdlc:plan` warns when artifacts have `status: draft` — Verify: create draft artifact, run plan, check warning
- [ ] AC-012: `/edikt:sdlc:drift` skips artifacts with `status: draft` — Verify: run drift with draft artifact, confirm it's excluded
- [ ] AC-013: `/edikt:doctor` reports artifacts stuck in draft > 7 days — Verify: check doctor output with old draft artifact
- [ ] AC-014: PostCompact hook injects context file list and attempt count — Verify: inspect PostCompact output format
- [ ] AC-015: At max iterations (configurable, default 5), phase status transitions to `stuck` and a human decision prompt is shown — Verify: simulate max failures, inspect status and prompt
- [ ] AC-016: After evaluation FAIL, the next generator prompt includes the failing criteria and reasons — Verify: inspect generator prompt after a FAIL
- [ ] AC-017: Evaluator prompt template contains "assume violations until proven" or equivalent skeptical stance — Verify: `grep -q "assume.*violations" templates/agents/evaluator.md`
- [ ] AC-018: When grep says PASS and LLM says FAIL, final verdict is FAIL. When grep says FAIL and LLM says PASS, final verdict is PASS. — Verify: create fixture where they disagree, check final verdict
- [ ] AC-019: Fixture with `severity: informational` criterion — criterion failure does not affect verdict — Verify: run fixture where only informational criterion fails, verdict is PASS
- [ ] AC-020: WEAK PASS verdict when all critical pass but 1+ important fails — Verify: run fixture with important-only failure, check verdict string
- [ ] AC-021: `evaluator` section in config with preflight, phase-end, mode, max-attempts, model keys — Verify: `/edikt:config` shows evaluator section
- [ ] AC-022: Headless evaluator runs as `claude -p` with `--bare` and `--disallowedTools "Write,Edit"` — Verify: inspect invocation command in plan.md or hook
- [ ] AC-023: When `evaluator.preflight: false`, plan skips step 11 — Verify: set config, generate plan, confirm no pre-flight output
- [ ] AC-024: When `evaluator.phase-end: false`, phase-end evaluation skipped but criteria sidecar still emitted — Verify: set config, complete phase, confirm no evaluation but YAML exists

## Technical Notes

- All commands are `.md` files — no compiled code (INV-001)
- Agents are advisors only — they read and return findings, never write files (ADR-004)
- **Phase 1 (pre-flight criteria validation) shipped in v0.3.0.** The evaluator agent template (`templates/agents/evaluator.md`) has a Pre-flight Mode section, and `commands/sdlc/plan.md` step 11 invokes it. FR-008's `verify` field is populated by the existing pre-flight validation.
- **PRD-005 R7 context:** Gate configuration (`gates:` in config.yaml) and the "engineers cannot disable gates" constraint are already implemented. This PRD adds logging (FR-015) and re-fire prevention (FR-016) to the existing gate mechanism.
- **PRD-005 R11 context:** Spec requires PRD `accepted`, and artifacts requires spec `accepted` — both already shipped. This PRD adds the remaining guard rails: plan warns on draft (FR-017), drift filters by status (FR-018), doctor flags stale drafts (FR-019).
- Existing artifacts to preserve and reference (move to spec folder when spec is created):
  - `docs/plans/artifacts/phase-context-handoff-example.md`
  - `docs/plans/artifacts/plan-criteria-schema.yaml`
  - `docs/plans/artifacts/experiment-evaluator-spec.md`
- Quality gate hook is a shell script (`templates/hooks/subagent-stop.sh`) — override UX happens via Claude's systemMessage, not the hook itself
- Experiment runner is bash (`test/experiments/directive-effect/run.sh`) — LLM evaluator adds a second `claude -p` invocation per run
- Criteria sidecar naming: `PLAN-{slug}.md` → `PLAN-{slug}-criteria.yaml`. The existing `docs/plans/artifacts/plan-criteria-schema.yaml` is the reference schema, not the runtime file.
- **Dependency note:** FR-010 requires FR-008 and FR-009. FR-001 and FR-002 require the status values defined in FR-023.
- **Artifact lifecycle states:** `draft → accepted → in-progress → implemented → superseded`. Transitions: `draft → accepted` (user manual), `accepted → in-progress` (plan auto, FR-020), `in-progress → implemented` (drift auto, FR-021), `any → superseded` (user creates replacement).

## Open Questions

- RESOLVED: Max iteration count is configurable via `evaluator.max-attempts` (default 5) — FR-029
- NEEDS CLARIFICATION: Should events.jsonl be per-project or global (~/.edikt/events.jsonl)?

---

*Written by edikt:prd — 2026-04-11. Revised after PM review.*
