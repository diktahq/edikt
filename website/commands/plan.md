# /edikt:plan

Turns a task or feature into a phased execution plan with dependencies, parallelism, and progress tracking.

## When to use it

Whenever a task is bigger than a single prompt. If it touches multiple files, has multiple steps, or spans more than one session — make a plan first.

## Usage

```
/edikt:plan
```

Or describe the task inline:

```
/edikt:plan add bulk order creation endpoint
/edikt:plan CON-42
/edikt:plan SPEC-005
/edikt:plan PLAN-007
/edikt:plan refactor the compile command
/edikt:plan add bulk order creation endpoint --no-review
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

```
How would you like to plan this?

1. edikt plan — phased execution plan with model assignment, cost estimate,
   codebase analysis, and specialist pre-flight review. Saved to docs/product/plans/.
2. Quick plan — help you think through the approach right here in conversation.
   No file, no ceremony.
```

Explicit `/edikt:plan` invocations with a SPEC, ticket, or PLAN reference skip this and go straight to the full flow.

## Plan mode

This command requires an interactive interview. If you (or Claude) are in plan mode (`/plan`), the interview will be silently skipped — Claude will describe what it would do instead of actually doing it, producing a low-quality plan. Exit plan mode first, then run `/edikt:plan`.

This applies to all edikt commands that interview the user: `init`, `plan`, `prd`, `spec`, `spec-artifacts`, `adr`, `invariant`, `intake`.

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

## Why it matters

Plans survive context compaction. The progress table in the plan file is the persistent state. When context gets compacted in a long session, Claude re-reads the plan and knows exactly where things stand — without losing progress.

## Natural language triggers

- "let's plan this"
- "create a plan for X"
- "break this into phases"
