# Specialist Agents

edikt ships **18 domain specialist agents** — the ones that review your plans, audit your code, and implement features in their domain. Alongside them sit seven internal system agents that edikt's own commands dispatch (the evaluator, the sidecar extractor, and the rest); those are covered in [System agents](#system-agents) below, and are not part of the domain roster.

This guide explains what agents are, how routing works, how they interact with rule packs, how they are governed, and how model selection works per phase.

## What agents are

Specialist agents are subagents with a defined domain focus. Each agent has:

- A `description:` field that tells the model when to activate it
- Domain expertise scoped to one area (database, security, frontend, etc.)
- Constraints that keep it from straying outside its domain

Agents don't replace the model — they add specialist focus. The model is the engineer running the session. Agents are the specialists who review plans and code, and implement self-contained tasks in their domain.

## Two types of agents

**Advisory agents** (read-only) — review plans and code, return findings with severity levels. They never write code. This keeps invocations fast and non-destructive. (`pm` is the one advisor that writes anything: it authors PRDs, and nothing else.)

**Implementation agents** (read and write) — both review AND implement. The model delegates self-contained implementation tasks to them.

| Type | Agents |
|------|--------|
| Advisory | `architect`, `dba`, `security`, `api`, `sre`, `platform`, `docs`, `pm`, `ux`, `data`, `performance`, `compliance`, `seo`, `gtm` |
| Implementation | `backend`, `frontend`, `qa`, `mobile` |

## The 18 domain agents

### Always installed

| Agent | Domain |
|-------|--------|
| `architect` | System design, ADRs, component boundaries, architectural trade-offs |
| `docs` | Documentation accuracy, gap detection, runbooks |
| `qa` | Testing strategy, test writing, coverage |

### Common (most projects)

| Agent | Domain |
|-------|--------|
| `sre` | Reliability, observability, deployment, infrastructure |
| `security` | OWASP, auth patterns, secret management, threat modeling |
| `pm` | Product requirements, prioritization, user stories |
| `api` | API contracts, versioning, breaking changes, documentation |

### Stack-triggered

| Agent | Triggered by |
|-------|-------------|
| `backend` | Go, TypeScript, Python, PHP, Ruby, Java, Rust |
| `frontend` | TypeScript + React/Vue/Angular/Svelte/Next.js |
| `dba` | Go, Python, Java (database-heavy stacks) |
| `ux` | React, Next.js, Vue, Angular, Svelte, React Native, Flutter |
| `platform` | Docker, Kubernetes, Terraform |
| `mobile` | React Native, Flutter, Swift, Kotlin |
| `seo` | Next.js, web content projects |
| `gtm` | Web content projects |
| `data` | Data pipeline projects |

### Optional (add with `/edikt:agents add {slug}`)

| Agent | Domain |
|-------|--------|
| `performance` | Performance bottlenecks, profiling, optimization |
| `compliance` | HIPAA, PCI, SOC2, GDPR |

`compliance` also installs automatically when `project-context.md` mentions regulated data. The stack-triggered agents above (`data`, `mobile`, `seo`, `gtm`) are in the same opt-in set — they install on their own when their stack is detected, and can be added by hand when it isn't.

## System agents

Seven further templates ship in `templates/agents/`, but they are not domain specialists and you never route to them by hand — edikt's own commands dispatch them: `evaluator`, `evaluator-headless`, `sidecar-extractor`, `governance-verifier`, `post-flight-synthesizer`, `compile-quality-grader`, and `cheat-rate-adversary`. They install unconditionally, because the commands that dispatch them fail to resolve otherwise (ADR-043, ADR-044).

### Evaluator

The `evaluator` is the one you'll notice, because it runs at two points in the SDLC chain:

- **Pre-flight** — before a phase starts, validates that acceptance criteria are testable (TESTABLE / VAGUE / SUBJECTIVE / BLOCKED)
- **Phase-end** — after a phase completes, verifies each criterion with evidence (file:line citations)

It's skeptical by default, assuming work is incomplete until proven otherwise. Every PASS requires evidence; every FAIL requires a citation.

It runs in two modes: **headless** (the default — a separate `claude -p` invocation with zero shared context, which eliminates self-evaluation bias and works in CI) and **subagent** (a forked agent inside the session — faster, but only partial context isolation). Both are configurable under `evaluator.*` in `.edikt/config.yaml`.

The evaluator is core rather than optional: the plan harness and quality gates depend on it, so `/edikt:init` and `/edikt:upgrade` install it unconditionally, and `/edikt:doctor` probes its presence and configured mode before a phase can block on it.

See [Evaluator](/governance/evaluator) for the full mode comparison and configuration reference.

## How routing works

The model routes to agents using their `description:` field. Each description includes trigger conditions:

```yaml
# dba agent description
description: "Reviews and implements database schema, migrations,
  queries, and data modeling. Use proactively when migration or
  schema files are modified."
```

When the model sees you working on a migration file, it reads this description and delegates to the dba agent.

Three routing paths:

**Auto-routing** — the model reads file context and the agent descriptions, delegates when there's a match.

**Command routing** — edikt's commands route to the right specialists automatically:

| You say | What happens |
|---------|-------------|
| "review this" | `/edikt:sdlc:code-review` detects changed file domains, routes to matching agents |
| "audit the codebase" | `/edikt:sdlc:audit` routes to `security` and `sre` agents |
| "create a plan" | `/edikt:sdlc:plan` assigns reviewers to each phase based on domain |
| "generate spec artifacts" | `/edikt:sdlc:artifacts` routes each artifact to its domain specialist |

**Direct delegation** — ask by name: "have the dba review this migration"

### Plan pre-flight review

When you run `/edikt:sdlc:plan`, edikt scans the plan content for domain signals and invokes the relevant advisors before execution begins:

| Plan mentions... | Agent invoked |
|-----------------|--------------|
| SQL, migration, schema, index | `dba` |
| docker, terraform, helm, k8s | `platform`, `sre` |
| auth, JWT, payment, token, RBAC | `security` |
| API, endpoint, REST, webhook | `api` |
| bounded context, hexagonal, layer | `architect` |
| performance, cache, latency | `performance` |

Each advisor reviews only their domain and returns findings before you start building.

### Post-implementation review

`/edikt:sdlc:code-review` classifies changed files by domain and routes the diff to the relevant agents:

| Changed files | Agent invoked |
|---------------|--------------|
| `*.sql`, `migration*`, `schema*` | `dba` |
| `Dockerfile*`, `docker-compose*`, `*.tf`, `helm/*` | `sre` |
| `*auth*`, `*jwt*`, `*payment*`, `*token*` | `security` |
| `*route*`, `*handler*`, `*controller*`, `*api*` | `api` |
| `*cache*`, `*perf*`, `*optimize*` | `performance` |

## How agents work with rule packs

Agents and rule packs serve different purposes and work at different levels:

| | Rule packs | Agents |
|--|------------|--------|
| **When** | Every file, every session | When the task matches their domain |
| **How** | Loaded into context automatically | Spawned as subagents with isolated context |
| **What** | Static coding standards | Dynamic specialist review |
| **Language** | Language-specific (go.md, typescript.md) | Language-agnostic (dba reviews any SQL) |

They work together: the Go rule pack teaches the model Go patterns. The backend agent reviews whether those patterns were applied correctly. The rule pack prevents violations. The agent catches what the rules missed.

**Example:** Your Go rule pack says "always wrap errors with context." Your backend agent reviews a PR and catches a bare `return err` — even though the rule told it not to do this, the agent catches it in review.

## Severity model

All advisory agents use the same three-level model:

| Level | Meaning |
|-------|---------|
| Critical | Must address before shipping — data loss, security breach, broken contract |
| Warning | Should fix, not blocking |
| OK | Domain looks healthy |

Critical findings trigger quality gates that block progression. See [Quality Gates](/governance/gates).

## Agent governance

Every agent template carries frontmatter that constrains its autonomy and resource usage. These are enforced by Claude Code, not merely requested in the prompt.

### Turn limits

`maxTurns` caps how many agentic turns an agent takes before it stops. Read-only reviewers cap at 10 — enough to read, grep, and report. Agents that produce output over several passes (`backend`, `frontend`, `mobile`, `qa`, plus `pm` and `platform`) cap at 20, leaving room for write-test-fix cycles.

### Tool restrictions

The advisory/implementation boundary is enforced at the tool layer, not by instruction. Advisory agents declare `disallowedTools: [Write, Edit]`, so they can read and analyze but cannot modify code — `platform` reaches the same result through a `tools:` allowlist that simply never grants write access. The exception worth knowing: `pm` holds `Write` so it can author PRDs, but it has no `Edit` and never touches source.

### Effort level

`effort` controls how hard the model works per turn. It's independent of the model — a `high` effort agent still runs on whatever model the plan phase assigned.

| Effort | Agents |
|--------|--------|
| high | `architect`, `security`, `qa`, `performance`, `compliance` |
| medium | `api`, `backend`, `data`, `dba`, `docs`, `frontend`, `mobile`, `platform`, `pm`, `sre`, `ux` |
| low | `seo`, `gtm` |

### Auto-loaded context

Every agent template declares an `initialPrompt` — the context it reads before it responds when run as the main session agent via `claude --agent`. The `architect` reads all ADRs and invariants; `security` reads the architecture and identifies trust boundaries; `pm` reads all active PRDs and specs.

When the same agents are invoked as subagents by edikt commands, the parent command supplies the context instead.

### Resumption

When a specialist stops — review finished, or turn limit reached — it can be resumed with `SendMessage`. Claude Code auto-resumes stopped background agents rather than erroring, so edikt commands that spawn specialists can re-engage them later without tracking state. Useful when a review produces follow-up questions.

## Model selection — per phase, not per agent

Agents don't have a fixed model. No `model:` field exists in any agent template. The model is determined by what's being done and how complex it is — assigned at the plan phase level.

When you create a plan, each phase includes a complexity assessment and suggested model:

```
Phase 1: Multi-tenant schema design
  Complexity: High — architecture decision with security implications
  Suggested model: opus
  Reviewers: architect, dba, security

Phase 3: CRUD handler implementation
  Complexity: Medium
  Suggested model: sonnet
  Reviewers: backend, api
```

Complexity-to-model mapping:

| Task | Complexity | Suggested model |
|------|-----------|----------------|
| Architecture/design decisions | High | opus |
| Complex implementation (domain logic, state machines) | High | opus or sonnet |
| Standard implementation (CRUD, handlers, tests) | Medium | sonnet |
| Mechanical tasks (formatting, docs, simple tests) | Low | haiku or sonnet |
| Critical review (security, schema, API contracts) | High | opus |
| Routine review (formatting, naming, small fixes) | Low | sonnet or haiku |

When agents run outside a plan (ad-hoc review, direct delegation), they use the default model from the main conversation.

## Managing agents

**List installed agents:**
```
/edikt:agents
```

**Add an optional agent:**
```
/edikt:agents add performance
/edikt:agents add compliance
```

**Get recommendations for your stack:**
```
/edikt:agents suggest
```

**Command reference:** [/edikt:agents](/commands/agents)

## Customizing agents

Agents install to `.claude/agents/`, and you can edit any of them in place — change the domain expertise, tighten the constraints, adjust `maxTurns` or `effort`. Mark the file with `<!-- edikt:custom -->`, or list its slug under `agents.custom` in `.edikt/config.yaml`, so `/edikt:upgrade` leaves your version alone.

To add a new one, create a file in `.claude/agents/my-agent.md`:

```yaml
---
name: my-domain-expert
description: "Reviews X for Y. Use proactively when Z files are modified."
tools:
  - Read
  - Grep
  - Glob
---

You are a {domain} specialist with deep knowledge of...
```

The `description:` field is what the model reads to decide when to delegate, so make it specific and state the trigger condition explicitly.

## Agent memory

Two agents have persistent memory across sessions:

- `dba` — accumulates schema knowledge, migration history, query patterns
- `security` — accumulates threat model context, past findings, auth decisions

Memory stores at `.claude/agent-memory/{agent-name}/` and loads automatically. The dba agent remembers your schema decisions from last week.

To add memory to any agent, add `memory: project` to its frontmatter.
