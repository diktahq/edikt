# Commands

edikt commands are organized by namespace. Each namespace groups related commands. You rarely need to remember them — Claude responds to natural language after init.

## Governance

Compile and maintain the rules Claude follows.

| Command | What it does |
|---------|-------------|
| [`/edikt:gov:compile`](/commands/gov/compile) | Compile ADRs, invariants, and guidelines into topic-grouped rule files |
| [`/edikt:gov:review`](/commands/gov/review) | Review governance doc language for enforceability and clarity |
| [`/edikt:gov:rules-update`](/commands/gov/rules-update) | Check for outdated rule packs and update them |
| [`/edikt:gov:sync`](/commands/gov/sync) | Translate linter configs into Claude rule packs |
| [`/edikt:gov:score`](/commands/gov/score) | Aggregate governance quality scoring |

## SDLC Chain

The full cycle from requirements through verification.

| Command | What it does |
|---------|-------------|
| [`/edikt:sdlc:prd`](/commands/sdlc/prd) | Write a product requirement document |
| [`/edikt:sdlc:spec`](/commands/sdlc/spec) | Technical specification from an accepted PRD |
| [`/edikt:sdlc:artifacts`](/commands/sdlc/artifacts) | Data model, contracts, migrations from an accepted spec |
| [`/edikt:sdlc:plan`](/commands/sdlc/plan) | Phased execution plan with pre-flight specialist review |
| [`/edikt:sdlc:review`](/commands/sdlc/review) | Post-implementation specialist review — routes to domain agents |
| [`/edikt:sdlc:drift`](/commands/sdlc/drift) | Verify implementation matches spec, PRD, and ADRs |
| [`/edikt:sdlc:audit`](/commands/sdlc/audit) | Security audit — OWASP scan, secret detection, auth coverage |

## Decisions

Capture and maintain architecture decisions and constraints.

| Command | What it does |
|---------|-------------|
| [`/edikt:adr:new`](/commands/adr/new) | Capture an architecture decision record |
| [`/edikt:adr:compile`](/commands/adr/compile) | Compile ADRs into governance directives |
| [`/edikt:adr:review`](/commands/adr/review) | Review ADR language quality |
| [`/edikt:invariant:new`](/commands/invariant/new) | Define a hard constraint that must never be violated |
| [`/edikt:invariant:compile`](/commands/invariant/compile) | Compile invariants into governance directives |
| [`/edikt:invariant:review`](/commands/invariant/review) | Review invariant language quality |
| [`/edikt:guideline:new`](/commands/guideline/new) | Capture a team coding standard or convention |
| [`/edikt:guideline:review`](/commands/guideline/review) | Review guideline language quality |

## Docs

Keep documentation current.

| Command | What it does |
|---------|-------------|
| [`/edikt:docs:review`](/commands/docs/review) | Review documentation gaps for new routes, env vars, and services |
| [`/edikt:docs:intake`](/commands/docs/intake) | Scan scattered docs and organize into edikt structure |

## Daily Use

Everything you'll run session to session.

| Command | What it does |
|---------|-------------|
| [`/edikt:capture`](/commands/capture) | Capture the current conversation into the right governance artifact |
| [`/edikt:context`](/commands/context) | Load project context, plans, ADRs, and product docs into current session |
| [`/edikt:status`](/commands/status) | Dashboard — plan progress, rules, what's next |
| [`/edikt:brainstorm`](/commands/brainstorm) | Brainstorm features, explore design space, converge toward PRD or spec |
| [`/edikt:session`](/commands/session) | End-of-session sweep — surface missed captures before context is lost |
| [`/edikt:doctor`](/commands/doctor) | Validate governance setup and report actionable warnings |
| [`/edikt:init`](/commands/init) | Detect project, infer architecture, install rules, agents, and context |
| [`/edikt:upgrade`](/commands/upgrade) | Upgrade hooks, agents, and rules to the latest edikt version |
| [`/edikt:agents`](/commands/agents) | List, install, and manage specialist agent templates |
| [`/edikt:mcp`](/commands/mcp) | Connect to Linear, GitHub, or Jira via MCP |
| [`/edikt:config`](/commands/config) | View and modify project configuration |
| `/edikt:team` (deprecated) | Merged into /edikt:init and /edikt:config |

## You don't need to remember them

After `/edikt:init`, Claude responds to how you naturally talk. You don't need to think about which command to run — just say what you need.

> "what's our status?" → `/edikt:status`
> "let's plan this" → `/edikt:sdlc:plan`
> "capture this decision" → `/edikt:adr:new`
> "any doc gaps?" → `/edikt:docs:review`
> "compile our governance" → `/edikt:gov:compile`

See the full list on the [Natural Language](/natural-language) page.

## The one command you run once

`/edikt:init` is the setup command. Everything else is day-to-day. After init, most interactions happen through natural language — the slash commands are there when you want explicit control.
