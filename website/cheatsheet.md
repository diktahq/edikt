# Cheatsheet

One page. Every edikt command. Match by meaning, not exact words — Claude Code routes natural-language phrases to the right command.

## Daily

| Intent | Natural phrase | Command |
|---|---|---|
| Project status / what's next | "what's our status", "next steps" | `/edikt:status` |
| Load project context | "remind yourself", "load context" | `/edikt:context` |
| Capture a mid-session decision | "capture this", "what did we decide" | `/edikt:capture` |
| End-of-session sweep | "wrap up", "session summary" | `/edikt:session` |
| Validate setup | "check my setup", "health check" | `/edikt:doctor` |

## Discover & spec

| Intent | Natural phrase | Command |
|---|---|---|
| Reduce uncertainty before a PRD | "discovery doc", "what do we not know yet" | `/edikt:sdlc:discovery` |
| Write a PRD | "write a PRD", "requirements for X" | `/edikt:sdlc:prd` |
| Re-score a PRD | "review this PRD" | `/edikt:prd:review` |
| Write a technical spec | "write a spec", "design doc for X" | `/edikt:sdlc:spec` |
| Re-score a SPEC | "check FR coverage", "review this spec" | `/edikt:spec:review` |
| Generate spec artifacts | "generate the data model", "build the artifacts" | `/edikt:sdlc:artifacts` |
| Plan execution | "create a plan", "plan this ticket" | `/edikt:sdlc:plan` |
| Brainstorm an idea | "let's brainstorm", "explore options" | `/edikt:brainstorm` |

## Decide & constrain

| Intent | Natural phrase | Command |
|---|---|---|
| Capture an ADR | "save this decision", "write an ADR" | `/edikt:adr:new` |
| Add an invariant | "that's a hard rule", "never do X" | `/edikt:invariant:new` |
| Add a guideline | "add a guideline", "document this convention" | `/edikt:guideline:new` |
| Compile a single ADR | "generate sentinels for ADR-NNN" | `/edikt:adr:compile` |
| Compile a single invariant | "generate sentinels for INV-NNN" | `/edikt:invariant:compile` |
| Review ADR language | "review this ADR", "check ADR quality" | `/edikt:adr:review` |
| Review invariant language | "review this invariant" | `/edikt:invariant:review` |
| Review guideline language | "review our guidelines" | `/edikt:guideline:review` |

## Govern

| Intent | Natural phrase | Command |
|---|---|---|
| Compile all governance | "compile governance", "update the rules" | `/edikt:gov:compile` |
| Review governance quality | "are our ADRs well written" | `/edikt:gov:review` |
| Adversarial benchmark | "run the governance benchmark", "test directives under pressure" | `/edikt:gov:benchmark` |
| Update rule packs | "check for rule updates" | `/edikt:gov:rules-update` |
| Sync from linter config | "import eslint rules" | `/edikt:gov:sync` |

## Review & ship

| Intent | Natural phrase | Command |
|---|---|---|
| Check implementation drift | "check drift", "did we build what we decided" | `/edikt:sdlc:drift` |
| Post-implementation review | "review what we built" | `/edikt:sdlc:review` |
| Security audit | "run a security audit", "check for vulnerabilities" | `/edikt:sdlc:audit` |
| Documentation audit | "check for doc gaps", "audit documentation" | `/edikt:docs:review` |

## Setup & maintenance

| Intent | Natural phrase | Command |
|---|---|---|
| Initialize a project | "set up edikt", "onboard this repo" | `/edikt:init` |
| View or change config | "show config", "set database type" | `/edikt:config` |
| Import existing docs | "intake our documentation" | `/edikt:docs:intake` |
| Manage specialist agents | "list agents", "add the security agent" | `/edikt:agents` |
| Set up integrations | "connect Jira", "add MCP server" | `/edikt:mcp` |
| Upgrade edikt | "upgrade edikt", "check for edikt updates" | `/edikt:upgrade` |

## Common workflows

### Start a new feature (full SDLC chain)

```
/edikt:sdlc:discovery   →  reduce uncertainty (optional)
/edikt:sdlc:prd         →  write PRD with five forcing questions
/edikt:sdlc:spec        →  technical spec from the PRD
/edikt:sdlc:artifacts   →  data model, domain model, fixtures
/edikt:sdlc:plan        →  execution plan with phases
                        (build phase by phase)
/edikt:sdlc:drift       →  verify the build matches the decision
/edikt:sdlc:review      →  post-implementation review
```

### Capture a decision mid-session

```
/edikt:adr:new          →  write the ADR
                        (auto-chains to /edikt:adr:compile)
/edikt:gov:compile      →  refresh governance directives
```

### PRD lifecycle

```
/edikt:sdlc:prd PRD-001 ship FR-001    →  mark FR-001 as shipped
/edikt:sdlc:prd PRD-001 cancel         →  work stopped before shipping
/edikt:sdlc:prd PRD-001 deprecate      →  was shipped, now obsolete
/edikt:sdlc:prd PRD-001 supersede      →  ≥50% scope rewrite, new PRD
```

### Periodic governance health

```
/edikt:doctor                 →  is everything wired correctly?
/edikt:gov:review             →  are ADRs well-written?
/edikt:gov:benchmark          →  do directives hold under adversarial prompts?
/edikt:gov:rules-update       →  any new rule packs upstream?
```

## Where this comes from

Claude Code matches intent, not exact words. The trigger table in your project's `CLAUDE.md` (lines 53–82, the section between `[edikt:start]` and `[edikt:end]`) is the canonical source — Claude reads it every session. Add your own custom triggers there.

For more detail on any command, see [Commands](/commands/) or [Natural-language triggers](./natural-language).
