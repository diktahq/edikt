# /edikt:sdlc:plan

Turns a task or feature into a phased execution plan with dependencies, parallelism, and progress tracking.

## When to use it

Whenever a task is bigger than a single prompt. If it touches multiple files, has multiple steps, or spans more than one session — make a plan first.

## Usage

```bash
/edikt:sdlc:plan
```

Or describe the task inline:

```bash
/edikt:sdlc:plan add bulk order creation endpoint
/edikt:sdlc:plan CON-42
/edikt:sdlc:plan SPEC-005
/edikt:sdlc:plan PLAN-007
/edikt:sdlc:plan refactor the compile command
/edikt:sdlc:plan add bulk order creation endpoint --no-review
```

## Arguments

| Argument | Description |
|----------|-------------|
| (none) | Infers from conversation context, or asks interactively |
| A task description | Uses it as the plan task |
| A ticket ID (e.g. `CON-42`) | References the ticket, fetches details via MCP if configured |
| `SPEC-NNN` | Uses the spec and its accepted artifacts as primary planning context |
| `PLAN-NNN` | Continue, re-plan, or create a sub-plan for an existing plan |
| `--no-review` | Skip the pre-flight specialist review after the plan is written |

## Full plan vs quick plan

When the input is a natural language description (not a SPEC, ticket, or PLAN reference), edikt offers a choice:

```text
How would you like to plan this?

1. edikt plan — phased execution plan with model assignment, cost estimate,
   codebase analysis, and specialist pre-flight review. Saved to docs/product/plans/.
2. Quick plan — help you think through the approach right here in conversation.
   No file, no ceremony.
```

Explicit `/edikt:sdlc:plan` invocations with a SPEC, ticket, or PLAN reference skip this and go straight to the full flow.

## Plan mode

This command requires an interactive interview. If you (or Claude) are in plan mode (`/plan`), the interview will be silently skipped — Claude will describe what it would do instead of actually doing it, producing a low-quality plan. Exit plan mode first, then run `/edikt:sdlc:plan`.

This applies to all edikt commands that interview the user: `init`, `sdlc:plan`, `sdlc:prd`, `sdlc:spec`, `sdlc:artifacts`, `adr:new`, `invariant:new`, `docs:intake`.

## What happens

edikt asks 3-6 targeted questions, reads your codebase for relevant context, then writes a phased plan to `docs/product/plans/`.

**Example conversation:**

> **Claude:** What's the scope — just the API endpoint, or does this include the domain model and tests too?
>
> **You:** Everything. Domain model, repo, handler, tests.
>
> **Claude:** Any existing order code to build on, or greenfield?
>
> **You:** We have the Order entity, no bulk operations yet.
>
> **Claude:** Got it. Writing plan...

## What a plan looks like

```markdown
# Plan: Bulk Order Creation

| Phase | Task            | Depends on | Status |
|-------|-----------------|------------|--------|
| 1     | Domain model    | —          | -      |
| 2     | Repository      | 1          | -      |
| 3     | HTTP handler    | 2          | -      |
| 4     | Tests           | 1, 2, 3    | -      |

### Phase 1 — Domain model
Add BulkOrder aggregate and CreateBulkOrder command...

### Phase 2 — Repository
Implement BulkOrderRepository with transaction support...
```

## Artifact coverage (v0.2.0)

When a plan is generated from a SPEC that has artifacts (from `/edikt:sdlc:artifacts`), edikt verifies every artifact has plan coverage:

| Artifact type | What the plan must include |
|---|---|
| `fixtures*.yaml` | A phase that creates seed data |
| `test-strategy.md` | Each test category mapped to at least one phase |
| `contracts/api*.yaml` | Every endpoint in at least one phase |
| `contracts/events*.yaml` | Producer + consumer phases per event |
| `migrations/*.sql` | A phase per migration |
| `data-model*.mmd` | Reference only — no phase needed |

If any artifact has no coverage, edikt blocks the plan and asks you to add phases, defer the artifact explicitly, or cancel. No artifact gets silently skipped.

```text
Artifact coverage:
  ✓ fixtures.yaml → Phase 8 (database seeding)
  ✓ test-strategy.md → Phases 2, 4, 6, 8 (tests embedded)
  ✓ contracts/api.yaml → Phases 3, 4, 5 (all 12 endpoints covered)
  ⚠ contracts/api-ai.yaml → Phase 7 added (POST /api/v1/ai/ask was uncovered)
```

## Phase-end evaluation (v0.2.0)

Each phase can include acceptance criteria — binary PASS/FAIL assertions. When a phase completes, a fresh evaluator agent checks the work against these criteria before recommending a context reset for the next phase.

Evaluation is conditional: high-complexity phases evaluate by default, simple phases skip. You can override with `evaluate: true/false` per phase.

## Context resets

At phase boundaries, edikt recommends starting a fresh session. Context resets outperform compaction for multi-phase work — later phases get a clean context window instead of accumulated noise.

## Why it matters

Plans survive context compaction. The progress table in the plan file is the persistent state. When context gets compacted in a long session, Claude re-reads the plan and knows exactly where things stand — without losing progress.

## Iteration tracking

Plans track how many times each phase has been attempted. The progress table includes an Attempt column:

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | done   | 1/5     | 2026-04-11 |
| 2     | in-progress | 2/5 | 2026-04-11 |
| 3     | pending | 0/5   | — |

Six statuses: `pending`, `in-progress`, `evaluating`, `done`, `stuck`, `skipped`.

After each evaluation failure, the attempt count increments and the failing criteria are forwarded to the next attempt. If the same criterion fails 3 consecutive times, an escalation warning surfaces. At max attempts (configurable via `evaluator.max-attempts`, default 5), the phase goes `stuck` with four options: continue trying, skip, rewrite criteria, or stop.

## Context handoff

Each phase declares a **Context Needed** field — the files the generator must read before starting:

```text
**Context Needed:**
- docs/product/specs/SPEC-005/contracts/api-orders.yaml — API contract
- internal/repository/orders.go — repository from Phase 2
- docs/architecture/decisions/ADR-012.md — error handling decision
```

An Artifact Flow Table shows which phases produce files consumed by other phases:

```text
┌─────────────────┬───────────────────────────────┬────────────────────┐
│ Producing Phase │           Artifact            │ Consuming Phase(s) │
├─────────────────┼───────────────────────────────┼────────────────────┤
│ 1               │ internal/domain/order.go      │ 2, 3               │
├─────────────────┼───────────────────────────────┼────────────────────┤
│ 2               │ internal/repository/orders.go │ 3                  │
└─────────────────┴───────────────────────────────┴────────────────────┘
```

After context compaction, the PostCompact hook re-injects the active phase, attempt count, last failing criteria, and context file list — so you never lose track of where you are or what to read.

## Criteria sidecar

Plans emit a `PLAN-{slug}-criteria.yaml` file alongside the plan markdown. This is a machine-readable companion that tracks per-criterion status:

```yaml
phases:
  - phase: 1
    title: "Domain model"
    status: pending
    attempt: "0/5"
    criteria:
      - id: AC-1.1
        status: pending
        verify: "grep -c 'type Order struct' internal/domain/order.go"
        fail_reason: null
        fail_count: 0
```

The evaluator reads and updates this file after each evaluation — no markdown parsing needed. Pre-flight validation populates the `verify` field with proposed shell commands.

## Plan-level and per-phase model assignment

As of v0.6.0, plan files carry a `model:` field at the top level and per-phase overrides:

```yaml
---
type: plan
id: PLAN-bulk-orders
model: claude-sonnet-4-6        # plan-level default
phases:
  - id: 1
    model: claude-haiku-4         # cheap CRUD work
  - id: 2
    model: claude-opus-4          # security-sensitive logic
  - id: 3
    # no model — inherits the plan-level default
---
```

The inheritance chain is per-phase override → plan-level default → `defaults.plan_model` in `.edikt/config.yaml` → `claude-sonnet-4-6`. Set the project-wide default once and let plans override per phase only when the cost/capability tradeoff actually matters.

```bash
/edikt:config set defaults.plan_model claude-sonnet-4-6
```

## Evaluator configuration

The evaluator's behavior is configurable in `.edikt/config.yaml`:

```yaml
evaluator:
  preflight: true          # pre-flight criteria validation (default: true)
  phase-end: true          # phase-end evaluation (default: true)
  mode: headless           # headless | subagent (default: headless)
  max-attempts: 5          # max retries before stuck (default: 5)
  model: sonnet            # sonnet | opus | haiku (default: sonnet)
```

See [Evaluator](/governance/evaluator) for details on headless vs subagent mode.

## Natural language triggers

- "let's plan this"
- "create a plan for X"
- "plan to fix these issues"
- "break this into phases"
