# Natural Language

edikt teaches Claude to respond to how you naturally talk — no need to remember slash commands.

After `/edikt:init`, your `CLAUDE.md` includes a trigger table. Claude reads it at session start and knows what to do when you ask everyday questions. The table matches intent, not exact phrases — if the meaning is close, Claude runs the right command.

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

> **You:** help me plan the export to CSV feature

> **Claude:** *(runs `/edikt:sdlc:plan`)*
> Great, let me ask a few questions to scope this out...

---

## Full trigger list

Claude matches intent, not exact words. These are representative examples — any phrase with the same meaning works.

| Intent | Examples | Command |
|--------|----------|---------|
| Project status / what's next | "what's our status", "where are we", "what's next", "project status", "next steps" | `/edikt:status` |
| Load project context | "load context", "remind yourself", "what's this project", "give me context" | `/edikt:context` |
| Create an execution plan | "create a plan", "make a plan", "plan for X", "plan this ticket", "help me plan", "how should we approach X", "continue the plan", "re-plan phase 3", "plan to fix these issues", "plan these changes" | `/edikt:sdlc:plan` |
| Reduce uncertainty before a PRD | "what do we not know yet", "structured discovery", "discovery doc", "uncertainty doc" | `/edikt:sdlc:discovery` |
| Capture an architecture decision | "save this decision", "record this", "capture that", "write an ADR", "document this decision" | `/edikt:adr:new` |
| Add a hard constraint | "add an invariant", "that's a hard rule", "never do X", "this must always be true" | `/edikt:invariant:new` |
| Write a PRD | "write a PRD", "document this feature", "requirements for X", "product requirements" | `/edikt:sdlc:prd` |
| Re-score an existing PRD | "review this PRD", "re-score PRD-NNN", "check PRD quality", "PRD drift" | `/edikt:prd:review` |
| Write a technical spec | "write a spec", "technical spec for X", "spec this out", "design doc for X" | `/edikt:sdlc:spec` |
| Re-score an existing SPEC | "review this spec", "check FR coverage", "AC pass-through check" | `/edikt:spec:review` |
| Generate spec artifacts | "generate artifacts", "create the data model", "generate the contracts", "build the artifacts" | `/edikt:sdlc:artifacts` |
| Check implementation drift | "check drift", "did we build what we decided", "verify the implementation", "are we on track with the spec" | `/edikt:sdlc:drift` |
| Compile governance | "compile governance", "update directives", "update the rules" | `/edikt:gov:compile` |
| Review governance quality | "review governance", "are our ADRs well written", "check governance quality" | `/edikt:gov:review` |
| Run governance benchmark | "run the governance benchmark", "test our directives against the model", "adversarial directive test", "check if our directives hold under pressure" | `/edikt:gov:benchmark` |
| Review implementation | "review what we built", "post-implementation review", "review this code" | `/edikt:sdlc:review` |
| Security audit | "run a security audit", "check for vulnerabilities", "security check" | `/edikt:sdlc:audit` |
| Check documentation gaps | "check for doc gaps", "what docs are outdated", "audit documentation" | `/edikt:docs:review` |
| Validate setup | "check my setup", "is everything configured right", "health check", "run doctor" | `/edikt:doctor` |
| Initialize project or onboard | "set up edikt", "initialize this project", "onboard this repo", "validate my environment", "onboard me", "team setup" | `/edikt:init` |
| View or change config | "show config", "change config", "disable quality gates", "set database type", "what can I configure" | `/edikt:config` |
| Import existing docs | "import existing docs", "onboard these docs", "intake our documentation" | `/edikt:docs:intake` |
| Update rule packs | "check for rule updates", "are my rules outdated", "update rules" | `/edikt:gov:rules-update` |
| Sync linter rules | "sync rules from linter", "import linter config", "sync eslint rules" | `/edikt:gov:sync` |
| Capture mid-session decisions | "capture this", "save this decision", "what did we decide", "mid-session sweep" | `/edikt:capture` |
| Create a guideline | "add a guideline", "create a team guideline", "document this convention" | `/edikt:guideline:new` |
| Review guideline quality | "review our guidelines", "check guideline language" | `/edikt:guideline:review` |
| Generate ADR sentinels | "compile this adr", "generate sentinels for ADR-NNN" | `/edikt:adr:compile` |
| Review ADR language | "review this adr", "check ADR-NNN quality" | `/edikt:adr:review` |
| Generate invariant sentinels | "compile this invariant", "generate sentinels for INV-NNN" | `/edikt:invariant:compile` |
| Review invariant language | "review this invariant", "check INV-NNN quality" | `/edikt:invariant:review` |
| End-of-session sweep | "wrap up this session", "end of session", "session summary" | `/edikt:session` |
| Upgrade edikt | "upgrade edikt", "update edikt", "check for edikt updates" | `/edikt:upgrade` |
| List or manage agents | "what agents do we have", "list agents", "add the security agent" | `/edikt:agents` |
| Set up integrations | "setup Linear", "connect Jira", "add MCP server" | `/edikt:mcp` |
| Brainstorm / explore ideas | "let's brainstorm", "brainstorm this", "explore options for X", "I have an idea", "let's think through X" | `/edikt:brainstorm` |
| Team onboarding *(deprecated)* | "team onboard" | `/edikt:team` *(redirects to init)* |

These are defined in `CLAUDE.md` — you can add your own triggers by editing that file.

## Adding your own triggers

Open `CLAUDE.md` and add rows to the trigger table:

```markdown
| Run tests | "run the tests", "check if tests pass" | `make test` |
| Deploy | "deploy to staging", "ship it to staging" | `make deploy-staging` |
```

Any instruction you find yourself repeating to Claude belongs here.
