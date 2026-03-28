# Commands

edikt has 24 commands. Each does exactly one thing. You rarely need to remember them — Claude responds to natural language after init.

## The commands

| Command | What it does |
|---------|-------------|
| [`/edikt:init`](/commands/init) | Detect project, infer architecture, install rules, agents, and context |
| [`/edikt:context`](/commands/context) | Load project context, plans, ADRs, and product docs into current session |
| [`/edikt:plan`](/commands/plan) | Interview + phased execution plan with pre-flight specialist review |
| [`/edikt:status`](/commands/status) | Dashboard — plan progress, rules, what's next |
| [`/edikt:intake`](/commands/intake) | Scan scattered docs and organize into edikt structure |
| [`/edikt:agents`](/commands/agents) | List, install, and manage specialist agent templates |
| [`/edikt:mcp`](/commands/mcp) | Connect to Linear, GitHub, or Jira via MCP |
| [`/edikt:team`](/commands/team) | Validate team member setup and show shared config |
| [`/edikt:adr`](/commands/adr) | Capture an architecture decision record |
| [`/edikt:invariant`](/commands/invariant) | Define a hard constraint that must never be violated |
| [`/edikt:brainstorm`](/commands/brainstorm) | Brainstorm features, explore design space, converge toward PRD or spec |
| [`/edikt:prd`](/commands/prd) | Write a product requirement document |
| [`/edikt:spec`](/commands/spec) | Technical specification from an accepted PRD |
| [`/edikt:spec-artifacts`](/commands/spec-artifacts) | Data model, contracts, migrations from an accepted spec |
| [`/edikt:compile`](/commands/compile) | Compile ADRs + invariants into governance directives |
| [`/edikt:drift`](/commands/drift) | Verify implementation matches spec, PRD, and ADRs |
| [`/edikt:review`](/commands/review) | Post-implementation specialist review — routes to domain agents |
| [`/edikt:review-governance`](/commands/review-governance) | Review governance doc language for enforceability and clarity |
| [`/edikt:audit`](/commands/audit) | Security audit — OWASP scan, secret detection, auth coverage |
| [`/edikt:session`](/commands/session) | End-of-session sweep — surface missed captures before context is lost |
| [`/edikt:docs`](/commands/docs) | Review documentation gaps for new routes, env vars, and services |
| [`/edikt:sync`](/commands/sync) | Translate linter configs into Claude rule packs |
| [`/edikt:rules-update`](/commands/rules-update) | Check for outdated rule packs and update them |
| [`/edikt:doctor`](/commands/doctor) | Validate governance setup and report actionable warnings |
| [`/edikt:upgrade`](/commands/upgrade) | Upgrade hooks, agents, and rules to the latest edikt version |

## You don't need to remember them

After `/edikt:init`, Claude responds to how you naturally talk. You don't need to think about which command to run — just say what you need.

> "what's our status?" → `/edikt:status`
> "what's next?" → `/edikt:status`
> "load context" → `/edikt:context`
> "let's plan this" → `/edikt:plan`
> "capture this decision" → `/edikt:adr`

See the full list on the [Natural Language](/natural-language) page.

## The one command you run once

`/edikt:init` is the setup command. Everything else is day-to-day. After init, most interactions happen through natural language — the slash commands are there when you want explicit control.
