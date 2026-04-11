---
type: spec
id: SPEC-001
title: "Plan Harness — Iteration Tracking, Context Handoff, Criteria Sidecar"
status: accepted
author: Daniel Gomes
implements: PRD-001
source_prd: docs/product/prds/PRD-001-v040-harness-lifecycle-gates.md
created_at: 2026-04-11T02:30:00Z
references:
  adrs: [ADR-001, ADR-004]
  invariants: [INV-001]
  source_artifacts:
    - docs/plans/artifacts/phase-context-handoff-example.md
    - docs/plans/artifacts/plan-criteria-schema.yaml
---

# SPEC-001: Plan Harness — Iteration Tracking, Context Handoff, Criteria Sidecar

**Implements:** PRD-001 (FR-001 through FR-010, FR-023, FR-024)
**Date:** 2026-04-11
**Author:** Daniel Gomes

---

## Summary

This spec adds three capabilities to `/edikt:sdlc:plan`: iteration tracking with backoff (plans remember failures and escalate), phase context handoff (phases carry forward required reading lists), and a structured criteria sidecar (machine-readable YAML alongside plan markdown). Together, these eliminate blind re-attempts, cold-start phases, and prose-dependent evaluation.

## Context

The plan command (`commands/sdlc/plan.md`) generates phased execution plans with acceptance criteria, model assignment, and evaluator integration. It supports pre-flight validation (v0.3.0) and phase-end evaluation via the evaluator agent.

Three gaps remain:

1. **No failure memory.** The progress table tracks `| Phase | Status | Updated |` but has no attempt counter. When a phase fails, the generator retries blind — no record of what failed or how many times. The evaluator writes findings to conversation context, but compaction erases them.

2. **No context handoff.** Each phase starts cold. The PostCompact hook (`templates/hooks/post-compact.sh`) injects the active plan phase and invariants, but not the files the phase needs to read. Engineers manually re-explain what previous phases produced.

3. **No structured criteria.** Acceptance criteria live in plan markdown. The evaluator parses prose to find them. Machine-readable criteria would let the evaluator read/write status, track fail counts, and propose verification commands — all without parsing markdown.

The Long-Running Harness Plan (phases 2, 3, 5) and two existing artifacts define the target design. This spec translates them into changes to specific files.

## Existing Architecture

- **Plan command:** `commands/sdlc/plan.md` (~360 lines). Steps 1-11 handle interview, codebase scan, phase generation, pre-flight review, and criteria validation. Progress table template is in the Reference section (~line 315).
- **PostCompact hook:** `templates/hooks/post-compact.sh` (~60 lines). Parses the progress table via regex to find the active phase, injects plan name + phase number + invariants as a `systemMessage`.
- **Evaluator agent:** `templates/agents/evaluator.md`. Pre-flight mode (criteria classification) and phase-end mode (PASS/FAIL with evidence). Currently runs as a subagent via Agent tool.
- **Reference artifacts:** `docs/plans/artifacts/plan-criteria-schema.yaml` (sidecar schema), `docs/plans/artifacts/phase-context-handoff-example.md` (format examples).

## Proposed Design

### 1. Iteration Tracking

The progress table gains an `Attempt` column. New statuses formalize the phase lifecycle. Backoff logic detects stuck criteria and escalates.

**Progress table format change:**

```markdown
| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | done   | 1/5     | 2026-04-11 |
| 2     | in-progress | 2/5 | 2026-04-11 |
| 3     | pending | 0/5    | — |
```

**Status values:**

| Status | Meaning |
|--------|---------|
| `pending` | Not started |
| `in-progress` | Generator is working on this phase |
| `evaluating` | Phase-end evaluator is running |
| `done` | All acceptance criteria PASS |
| `stuck` | Max attempts reached, human decision needed |
| `skipped` | Explicitly skipped by user |

**Backoff logic (in plan execution flow):**

1. After each evaluation FAIL: increment `Attempt` in progress table. Update `fail_count` and `fail_reason` per criterion in the criteria sidecar.
2. Before retrying: read the criteria sidecar. Include failing criteria and reasons in the generator prompt: "Previous attempt failed. Fix these: AC-2.1 (no tenant_id in log calls), AC-2.3 (missing rollback in migration)."
3. If the same criterion ID has `fail_count >= 3`: escalate with warning:
   ```
   ⚠️ AC-2.1 has failed 3 consecutive times.
      Last reason: no tenant_id in log calls
      Consider: rewrite the criterion, adjust the approach, or ask for help.
   ```
4. At max attempts (configurable via `evaluator.max-attempts`, default 5): set status to `stuck`, prompt the user:
   ```
   Phase 2 is stuck after 5 attempts.
   Options:
     1. Continue trying (increase max)
     2. Skip this phase
     3. Rewrite failing criteria
     4. Stop and review
   ```

**Max attempts source:** Read `evaluator.max-attempts` from `.edikt/config.yaml`. Default to 5 if not set.

### 2. Phase Context Handoff

Each phase declares what it needs to read. An Artifact Flow Table shows the full handoff graph. The PostCompact hook injects the reading list.

**Context Needed field (per phase):**

```markdown
## Phase 3: API Handlers

**Objective:** Implement HTTP handlers for order endpoints
**Dependencies:** Phase 1, Phase 2
**Context Needed:**
- `docs/product/specs/SPEC-005/contracts/api-orders.yaml` — API contract from spec artifacts
- `internal/repository/orders.go` — repository created in Phase 2
- `docs/architecture/decisions/ADR-012.md` — error handling decision
```

The plan command MUST populate this field by analyzing:
- Spec artifacts referenced by the phase
- Files produced by dependency phases (from the Artifact Flow Table)
- ADRs referenced in the plan

**Artifact Flow Table (new section in plan template):**

```markdown
## Artifact Flow

| Producing Phase | Artifact | Consuming Phase(s) |
|-----------------|----------|---------------------|
| 1 | `internal/domain/order.go` | 2, 3 |
| 2 | `internal/repository/orders.go` | 3 |
| 2 | `migrations/001_create_orders.sql` | — |
| 3 | `internal/handler/orders.go` | 4 (tests) |
```

Placed between the dependency graph and Phase 1 details.

**Phase startup directive (FR-024):**

Add to plan execution rules:

```
Before implementing any plan phase:
1. Read every file listed in that phase's Context Needed section.
2. If a listed file does not exist, check the progress table — the producing phase may not be complete.
3. Do not proceed until all context files have been read.
```

### 3. Structured Criteria Sidecar

The plan command emits a `PLAN-{slug}-criteria.yaml` file alongside the plan markdown. The evaluator reads and updates it.

**File location:** Always a sibling of the plan file.
- `docs/plans/PLAN-foo.md` → `docs/plans/PLAN-foo-criteria.yaml`
- `docs/product/plans/PLAN-bar.md` → `docs/product/plans/PLAN-bar-criteria.yaml`

**Schema:** Per `docs/plans/artifacts/plan-criteria-schema.yaml` (reference schema). Key fields:

```yaml
plan: "PLAN-{slug}"
generated: "YYYY-MM-DD"
last_evaluated: null

phases:
  - phase: 1
    title: "Domain model"
    status: "pending"
    attempt: "0/5"
    criteria:
      - id: "AC-1.1"
        description: "order.go has Order struct with ID, CustomerID, Items, Total"
        status: "pending"
        verify: "grep -c 'type Order struct' internal/domain/order.go"
        last_evaluated: null
        fail_reason: null
        fail_count: 0
```

**Generation (plan step 8):**

After writing the plan markdown, emit the criteria sidecar:
1. For each phase, extract acceptance criteria from the plan text
2. Assign IDs: `AC-{phase}.{seq}` (e.g., AC-1.1, AC-1.2, AC-2.1)
3. If pre-flight ran (step 11), populate `verify` with proposed commands
4. Set all `status: pending`, `fail_count: 0`, `fail_reason: null`
5. Write to `{plan_dir}/PLAN-{slug}-criteria.yaml`

**Evaluator updates (phase-end flow):**

After each evaluation, the evaluator (or the plan command orchestrating it) updates the sidecar:
1. Read the sidecar
2. For each criterion the evaluator judged: update `status` (pass/fail), `last_evaluated`, `fail_reason` (if fail)
3. Increment `fail_count` for each fail (reset to 0 on pass)
4. Update the phase-level `status` and `attempt`
5. Write back

### 4. PostCompact Hook Updates

**File:** `templates/hooks/post-compact.sh`

Changes to the hook:

1. **Extract attempt count** from the progress table. Current regex finds the phase number and status. New regex also extracts the Attempt column value (e.g., `2/5`).

2. **Inject context file list.** After finding the active plan, read the plan markdown, find the active phase's `Context Needed:` section, extract file paths. Include in the systemMessage.

3. **Inject last failing criteria.** Read the criteria sidecar (sibling of the plan file). Find criteria with `status: fail` for the active phase. Include criterion ID and `fail_reason` in the systemMessage.

**Updated output format:**

```
Context recovered after compaction. Active plan: PLAN-foo.
Phase 3 — API handlers (attempt 2/5).
Last failing criteria: AC-3.2 (no tenant_id in log calls).
Before continuing, read:
  - docs/product/specs/SPEC-005/contracts/api-orders.yaml
  - internal/repository/orders.go
  - docs/architecture/decisions/ADR-012.md
Invariants (2): INV-001, INV-012.
```

## Components

### `commands/sdlc/plan.md`
- **Progress table template:** Change from `| Phase | Status | Updated |` to `| Phase | Status | Attempt | Updated |`
- **Status values:** Add `pending`, `in-progress`, `evaluating`, `stuck`, `done`, `skipped` to the reference section
- **Backoff logic:** Add to phase-end flow (step after evaluation). Read `evaluator.max-attempts` from config.
- **Fail forwarding:** Add instruction to read criteria sidecar before retrying a failed phase
- **Context Needed field:** Add to phase structure requirements in Reference section
- **Artifact Flow Table:** Add as new section requirement between dependency graph and phases
- **Sidecar emission:** Add step 8b after step 8 — generate `PLAN-{slug}-criteria.yaml`
- **Sidecar update:** Add to phase-end flow — update sidecar after evaluation
- **Phase startup directive:** Add to Reference section

### `templates/hooks/post-compact.sh`
- **Attempt extraction:** Extend progress table regex to capture Attempt column
- **Context injection:** Parse `Context Needed:` from active phase, inject file list
- **Fail criteria injection:** Read criteria sidecar, inject failing criteria for active phase

### `docs/plans/artifacts/plan-criteria-schema.yaml`
- Already exists and matches the design. No changes needed. This file is the reference schema.

### `docs/plans/artifacts/phase-context-handoff-example.md`
- Already exists with format examples. No changes needed. This file is the reference example.

## Non-Goals

- Evaluator execution mode changes (covered in SPEC-002)
- LLM evaluator in experiments (covered in SPEC-002)
- Quality gate UX (covered in SPEC-003)
- Artifact lifecycle enforcement (covered in SPEC-003)
- Dependency gating between phases (deferred per PRD-001)
- Similarity detection for fail reason matching (v0.5.0)

## Alternatives Considered

### Sidecar as JSON instead of YAML

- **Pros:** Easier to parse programmatically, standard `jq` tooling
- **Cons:** Less readable for engineers reviewing criteria status, inconsistent with edikt's YAML-first convention (config.yaml, registry.yaml)
- **Rejected because:** YAML is the project standard and shell scripts can parse it with `grep`/`awk`. Engineers will read this file directly.

### Context handoff via a separate index file

- **Pros:** Single source of truth for all phase handoffs
- **Cons:** Another file to maintain, diverges from plan markdown as the primary artifact
- **Rejected because:** Embedding Context Needed in the plan keeps everything in one place. The Artifact Flow Table provides the cross-phase view without a separate file.

### Attempt tracking in the sidecar only (not the progress table)

- **Pros:** Single source of truth for attempt data
- **Cons:** PostCompact hook would need to read the YAML sidecar (more complex), progress table would lose information visible at a glance
- **Rejected because:** The progress table is the primary human-readable state. It must show attempt count for at-a-glance status. The sidecar has the detailed per-criterion data.

## Risks & Mitigations

| Risk | Impact | Likelihood | Mitigation | Rollback |
|---|---|---|---|---|
| PostCompact regex breaks on new table format | Lost plan context after compaction | Medium | Test regex against multiple table formats. Include fallback to current format if new columns not found. | Revert hook to current format |
| Criteria sidecar gets out of sync with plan markdown | Evaluator judges stale criteria | Low | Sidecar is always regenerated from plan markdown. Manual edits to plan criteria should trigger sidecar regeneration. | Delete sidecar, regenerate from plan |
| Context Needed lists become stale as phases modify files | Generator reads wrong files | Low | Context Needed is generated at plan creation, not maintained dynamically. Engineers can update it. | Generator reads the plan as a whole |

## Security Considerations

None identified. All changes are to command templates and a shell hook. No credentials, no external services, no user data.

## Performance Approach

Standard patterns sufficient. The sidecar is a small YAML file (<100 lines for most plans). The PostCompact hook adds a few more `grep` calls — negligible overhead.

## Acceptance Criteria

- AC-001: Plan progress table shows `| Phase | Status | Attempt | Updated |` — Verify: generate a plan and inspect table format
- AC-002: After 3 consecutive failures on the same criterion ID, plan output includes escalation warning — Verify: simulate with test fixture, check for "failed 3 consecutive times" text
- AC-003: Plan phase includes `Context Needed:` field with file paths — Verify: generate plan from spec with artifacts and inspect
- AC-004: Plan includes Artifact Flow Table between dependency graph and phase details — Verify: inspect generated plan markdown
- AC-005: `PLAN-{slug}-criteria.yaml` emitted alongside plan file in the same directory — Verify: check file exists after plan generation
- AC-006: Criteria YAML has per-criterion `status`, `verify`, `fail_reason`, `fail_count` fields — Verify: parse YAML and validate against reference schema
- AC-014: PostCompact hook injects context file list and attempt count — Verify: inspect PostCompact output format with mock progress table
- AC-015: At max iterations (configurable, default 5), phase status transitions to `stuck` and human decision prompt shown — Verify: simulate max failures, inspect status and prompt
- AC-016: After evaluation FAIL, the next generator prompt includes failing criteria and reasons from the sidecar — Verify: inspect generator prompt after a FAIL
- AC-023: When `evaluator.preflight: false`, plan skips step 11 — Verify: set config, generate plan, confirm no pre-flight output
- AC-024: When `evaluator.phase-end: false`, phase-end evaluation skipped but criteria sidecar still emitted — Verify: set config, complete phase, confirm no evaluation but YAML exists

## Testing Strategy

- **Structure tests:** Verify plan.md template has Attempt column, new statuses, Context Needed field, Artifact Flow Table reference, sidecar emission step
- **PostCompact hook tests:** Test regex extraction of phase + attempt from multiple table formats (with/without attempt column for backward compat)
- **Schema validation:** Parse reference schema, generate a sample sidecar, validate against schema
- **Backward compatibility:** Plans without Attempt column should still work in PostCompact hook (graceful fallback)

## Dependencies

- `evaluator.max-attempts` config key — needs `/edikt:config` command to support it (already added in v0.3.1)
- Pre-flight validation (plan step 11) — already shipped in v0.3.0
- Evaluator agent template — already exists at `templates/agents/evaluator.md`

## Open Questions

None — all questions resolved during PRD review and spec interview.

---

*Generated by edikt:spec — 2026-04-11*
