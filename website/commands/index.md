# Commands

edikt commands are organized by namespace. Each namespace groups related commands.

## Governance

Compile and maintain the rules Claude follows.

| Command | What it does |
|---------|-------------|
| [`/edikt:gov:compile`](/commands/gov/compile) | Compile ADRs, invariants, and guidelines into the four rendered governance surfaces |
| [`/edikt:gov:review`](/commands/gov/review) | Review governance doc language for enforceability and clarity |
| [`/edikt:gov:reextract`](/commands/gov/reextract) | Re-run the sidecar-extractor over the corpus, with per-artifact accept/reject review — manual, opt-in only |
| [`/edikt:gov:advisory-review`](/commands/gov/advisory-review) | Read-only judgment of whether compiled sidecars look stale — suggests, never writes |
| [`/edikt:gov:grade-compile`](/commands/gov/grade-compile) | Score the current compile output for quality across coherence, conciseness, and four other dimensions |
| [`/edikt:gov:verify-diff`](/commands/gov/verify-diff) | Judge whether a diff actually implements what a directive's `verify:` field claims |
| [`/edikt:gov:rules-update`](/commands/gov/rules-update) | Check for outdated rule packs and update them |
| [`/edikt:gov:sync`](/commands/gov/sync) | Translate linter configs into Claude rule packs |
| [`/edikt:gov:score`](/commands/gov/score) | Aggregate governance quality scoring |
| [`/edikt:gov:benchmark`](/commands/gov/benchmark) | Adversarial directive testing — opt-in install via `./bin/edikt install benchmark` |

## SDLC Chain

The full cycle from requirements through verification.

| Command | What it does |
|---------|-------------|
| [`/edikt:sdlc:discovery`](/commands/sdlc/discovery) | Structured uncertainty doc — Known, Unknown, Kill Criteria, Discovery Plan |
| [`/edikt:sdlc:prd`](/commands/sdlc/prd) | Write, continue, or transition a PRD (split markdown + YAML sidecar) |
| [`/edikt:sdlc:prd-review`](/commands/sdlc/prd-review) | Re-score a PRD against the rubric, report drift and broken refs |
| [`/edikt:sdlc:spec`](/commands/sdlc/spec) | Technical spec from a PRD, brainstorm, or free-text prompt |
| [`/edikt:sdlc:spec-review`](/commands/sdlc/spec-review) | Re-score a SPEC, verify FR coverage and AC pass-through |
| [`/edikt:sdlc:artifacts`](/commands/sdlc/artifacts) | Data model, contracts, migrations from an accepted spec |
| [`/edikt:sdlc:plan`](/commands/sdlc/plan) | Phased execution plan with pre-flight specialist review |
| [`/edikt:sdlc:code-review`](/commands/sdlc/code-review) | Post-implementation specialist review — routes to domain agents |
| [`/edikt:sdlc:drift`](/commands/sdlc/drift) | Verify implementation matches spec, PRD, and ADRs |
| [`/edikt:sdlc:audit`](/commands/sdlc/audit) | Security audit — OWASP scan, secret detection, auth coverage |
| [`/edikt:sdlc:post-flight`](/commands/sdlc/post-flight) | Post-phase review pipeline — composes criteria verify, governance verifier, and specialist routing into one deduplicated report |

## Decisions

Capture and maintain architecture decisions and constraints.

| Command | What it does |
|---------|-------------|
| [`/edikt:adr:new`](/commands/adr/new) | Capture an architecture decision record |
| [`/edikt:adr:compile`](/commands/adr/compile) | Compile ADRs into governance directives |
| [`/edikt:adr:enrich`](/commands/adr/enrich) | Add a manual directive to an ADR or invariant sidecar without editing the immutable prose |
| [`/edikt:adr:review`](/commands/adr/review) | Review ADR language quality |
| [`/edikt:invariant:new`](/commands/invariant/new) | Define a hard constraint that must never be violated |
| [`/edikt:invariant:compile`](/commands/invariant/compile) | Compile invariants into governance directives |
| [`/edikt:invariant:review`](/commands/invariant/review) | Review invariant language quality |
| [`/edikt:guideline:new`](/commands/guideline/new) | Capture a team coding standard or convention |
| [`/edikt:guideline:compile`](/commands/guideline/compile) | Compile guidelines into governance directives |
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
| [`/edikt:team`](/commands/team) (deprecated) | Merged into /edikt:init and /edikt:config |

## CLI reference

Surfaces that run as `bin/edikt` subcommands. Slash commands invoke most of these for you.

| Command | What it does |
|---------|-------------|
| [`edikt verify`](/commands/verify) | Runner for the `verify:` shell commands declared in plan, gov, PRD, and SPEC sidecars |
| [`edikt migrate sidecars`](/commands/migrate) | One-shot migration of in-body directive blocks into co-located `.edikt.yaml` sidecars |
| [`edikt migrate to-v2`](/commands/migrate#edikt-migrate-to-v2) | Rewrite v1 single-anchor sidecars into the v2 `source_excerpts[]` shape |
| [`/edikt:sidecar:approve`](/commands/sidecar/approve) | Review a pending behavioral verify proposal and promote, reject, or defer it |

## You don't need to remember them

After `/edikt:init`, Claude responds to how you naturally talk. You don't need to think about which command to run — just say what you need.

> "what's our status?" → `/edikt:status`
> "let's plan this" → `/edikt:sdlc:plan`
> "capture this decision" → `/edikt:adr:new`
> "any doc gaps?" → `/edikt:docs:review`
> "compile our governance" → `/edikt:gov:compile`

See the full list on the [Cheatsheet](/cheatsheet).

## The one command you run once

`/edikt:init` is the setup command. Everything else is day-to-day. After init, most interactions happen through natural language — the slash commands are there when you want explicit control.
