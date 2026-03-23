# Natural Language

edikt teaches Claude to respond to how you naturally talk — no need to remember slash commands.

After `/edikt:init`, your `CLAUDE.md` includes a trigger table. Claude reads it at session start and knows what to do when you ask everyday questions.

## How it works in practice

You're mid-session, deep in a feature. You want to know where things stand:

> **You:** what's our status?

> **Claude:** *(runs `/edikt:status`)*
> ```
> ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
>  EDIKT STATUS — Orders API
> ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
>
>  ACTIVE PLAN
>  ───────────
>  PLAN-bulk-orders  ·  2/4 phases (50%)
>
>  | Phase | Title         | Status      |
>  |-------|---------------|-------------|
>  | 1     | Schema        | done        |
>  | 2     | Domain model  | done        |
>  | 3     | HTTP handler  | in-progress |
>  | 4     | Tests         | -           |
>
>  WHAT'S NEXT
>  ───────────
>  Phase 3 — HTTP handler
>    - Wire up POST /orders/bulk in Chi router
>    - Validate with domain service
>    - Return 207 multi-status response
> ```

You didn't type `/edikt:status`. You just asked. Claude knew.

---

You come back after the weekend and want Claude to catch up:

> **You:** remind yourself about this project

> **Claude:** *(runs `/edikt:context`)*
> ```
> Loaded context for Orders API:
>
>   Project:   Go REST API, DDD, Chi, PostgreSQL
>   Plan:      PLAN-bulk-orders (Phase 3 in progress)
>   Rules:     code-quality, testing, security, error-handling, go, chi
>   Decisions: 3 ADRs (hexagonal arch, error strategy, auth pattern)
>   Product:   spec + 2 PRDs
>
> Ready. What are we working on?
> ```

---

You want to start planning a new feature:

> **You:** let's plan the export to CSV feature

> **Claude:** *(runs `/edikt:plan`)*
> Great, let me ask a few questions to scope this out...

---

## Full trigger list

| You say... | Claude runs |
|------------|-------------|
| "what's our status?" | `/edikt:status` |
| "where are we?" | `/edikt:status` |
| "what's next?" | `/edikt:status` |
| "what should we work on?" | `/edikt:status` |
| "remind yourself" | `/edikt:context` |
| "load context" | `/edikt:context` |
| "what's this project?" | `/edikt:context` |
| "let's plan X" | `/edikt:plan` |
| "create a plan for X" | `/edikt:plan` |
| "save this decision" | `/edikt:adr` |
| "record this choice" | `/edikt:adr` |
| "capture that" | `/edikt:adr` |
| "that's a hard rule" | `/edikt:invariant` |
| "never do X" | `/edikt:invariant` |
| "write a PRD for X" | `/edikt:prd` |
| "document this feature" | `/edikt:prd` |
| "what agents do we have?" | `/edikt:agents` |
| "add the security agent" | `/edikt:agents add security` |
| "setup Linear" | `/edikt:mcp add linear` |
| "validate my environment" | `/edikt:team setup` |

These are defined in `.claude/CLAUDE.md` — you can add your own triggers by editing that file.

## Adding your own triggers

Open `.claude/CLAUDE.md` and add rows to the trigger table:

```markdown
| "run the tests" | `make test` |
| "deploy to staging" | `make deploy-staging` |
| "what broke?" | `/edikt:status` then check recent git log |
```

Any instruction you find yourself repeating to Claude belongs here.
