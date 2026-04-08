# ADR-001 — edikt: Governance Layer for Agentic Engineering

**Status:** Accepted
**Date:** 2026-03-06
**Deciders:** Daniel Gomes

## Context

AI coding tools (Claude Code, Cursor, Copilot) produce inconsistent output because they start every session stateless. Architecture decisions get ignored, coding standards drift, and business rules are violated — not because the AI is incapable, but because the right context isn't loaded.

Existing solutions either try to orchestrate AI execution (too complex) or dump documentation into a single file (too blunt). Claude Code now has native features — path-conditional rules, custom agents, hooks, worktree isolation — that make a lighter approach possible.

## Decision

Build **edikt** as a governance layer for agentic engineering with two systems:

### Pillar 1 — Architecture & Design Governance

Install coding standards, security rules, and architecture patterns as `.claude/rules/` files where Claude automatically reads and enforces them. Three-tier rule system:

- **Base** (language-agnostic): code-quality, testing, security, error-handling
- **Language**: go, typescript, python, php (one .md per language)
- **Framework**: chi, nextjs, laravel, etc. (one .md per framework)

Rules are templates shipped with edikt, generated into `.claude/rules/` based on project config. Three levels of extensibility: toggle (config only), extend (append custom rules to a topic), create new topics.

### Pillar 2 — Product Management & Execution

Provide a framework to capture product context (project-context.md, product specs, PRDs) and create phased execution plans. Plans are the persistent state — progress tracked directly in the plan file, surviving context compaction.

### Core Commands (18 total)

| Command | Purpose |
|---------|---------|
| `/edikt:init` | Intelligent onboarding: detect project age, interview or audit, infer architecture, scaffold everything |
| `/edikt:context` | Load all context into session, write auto-memory snapshot |
| `/edikt:plan` | Interview + phased execution plan with parallelism analysis |
| `/edikt:status` | Dashboard: roadmap progress, active plans, governance health |
| `/edikt:intake` | Onboard scattered existing docs into edikt's standard structure |
| `/edikt:adr` | Capture an architectural decision — from scratch or from conversation |
| `/edikt:invariant` | Define a hard constraint that must never be violated |
| `/edikt:prd` | Write a product requirement document for a feature |
| `/edikt:agents` | List, inspect, and manage specialist agent templates |
| `/edikt:mcp` | Manage MCP server configuration (Linear, GitHub, Jira) |
| `/edikt:team` | Onboard team members and show shared team configuration |
| `/edikt:docs` | Audit documentation gaps for new routes, env vars, and services |
| `/edikt:sync` | Translate linter configs into Claude rule packs |
| `/edikt:doctor` | Validate governance setup and report actionable warnings |
| `/edikt:review` | Post-implementation specialist review — domain-routed agent findings |
| `/edikt:audit` | Security audit — OWASP scan, secret detection, auth coverage |
| `/edikt:session` | End-of-session sweep — surface missed captures before context is lost |
| `/edikt:upgrade` | Upgrade hooks, agents, and rules to the latest edikt version |
| `/edikt:rules-update` | Check for outdated rule packs and update them |

### Proactive Capture Loop

edikt's `CLAUDE.md` block instructs Claude to watch each response for signals worth capturing:

- **Architectural decisions** (technical choice with trade-offs) → suggest `/edikt:adr`
- **Hard constraints** (must-never-violate rules) → suggest `/edikt:invariant`
- **Product requirements** (clearly defined feature need) → suggest `/edikt:prd`

This is transparent — Claude tells you what it detected and you decide whether to capture it. No silent background writes. The suggestion fires only on strong signals; preferences and implementation details are ignored.

### Intelligent Init

Init detects whether a project is greenfield or established:

- **Greenfield**: User describes what they're building in natural language. edikt infers architecture complexity, bounded contexts, which rule packs to enable, and seeds project-context.md from the description. User confirms or toggles selections.
- **Established**: edikt runs a codebase audit (stack, directory structure, test patterns, CI/CD, git history) and recommends rule packs based on findings. Offers intake for existing docs.

### What edikt Does NOT Do

- **No execution orchestration** — Claude Code handles worktrees, agents, parallelism natively
- **No 25-command surface** — 18 commands, each with a single outcome
- **No proprietary formats** — Everything is plain markdown and YAML
- **No runtime dependencies** — Copy .md files, done
- **No backward compatibility with legacy tools** — Clean break

## Project Structure (after init)

```
docs/
├── project-context.md
├── product/
│   ├── spec.md
│   ├── prds/
│   └── plans/
└── reference/

.edikt/
├── config.yaml

.claude/
├── rules/              # generated from edikt templates
│   ├── code-quality.md
│   ├── testing.md
│   ├── security.md
│   ├── error-handling.md
│   └── {lang/framework}.md
├── agents/
├── settings.json       # hooks
└── CLAUDE.md
```

## Config

```yaml
# .edikt/config.yaml
base: docs
stack: [go, react]

rules:
  code-quality: { include: all }
  testing: { include: all }
  security: { include: all }
  error-handling: { include: all }
  go: { include: all }
  chi: { include: all }
  # architecture: { include: all }  # opt-in DDD/clean arch

sdlc:
  commit-convention: conventional
  pr-template: true
```

## Distribution

Single-line installer:
```bash
curl -fsSL https://raw.githubusercontent.com/you/edikt/main/install.sh | bash
```

Copies commands to `~/.claude/commands/` and templates to `~/.edikt/templates/`. No npm, no dependencies.

## Rationale

- **Rules in `.claude/rules/`** — Where Claude actually reads them, not a separate folder it might skip
- **Templates, not runtime** — edikt generates files and gets out of the way
- **Infer, don't interrogate** — Fewer questions, smarter defaults
- **5 commands** — Minimal surface area, maximum impact
- **Plain markdown** — Zero installation friction, survives API changes

## Claude Code Only (For Now)

edikt targets Claude Code exclusively for execution reliability. Other AI coding tools (Cursor, Copilot, Windsurf, Gemini CLI) lack the features edikt depends on:

- **Path-conditional rules** — Cursor/Copilot get one flat file; can't scope Go rules to `.go` files only
- **Hooks** — No way to enforce "load context before writing code" as a gate
- **Slash commands** — `/edikt:init`, `/edikt:plan` don't exist outside Claude Code
- **Custom agents** — Can't spawn reviewers or parallelize plan phases
- **Worktree isolation** — Phase-based execution with isolation is Claude Code native

The knowledge base (docs/, project-context.md, config) is plain markdown and works anywhere. If a team uses Cursor alongside Claude Code, init can optionally generate a `.cursorrules` file as a best-effort summary — but the full loop (init, rules, context, plan, execute with guardrails) only works in Claude Code.

Building for the lowest common denominator would mean losing everything that makes edikt effective. Better to be excellent on one platform than mediocre on five. If other tools add path-conditional rules and hooks later, support is trivial to add since the templates are already universal markdown.

## Consequences

- Established projects can use `/edikt:intake` to organize existing docs into edikt's structure
- Rule templates must be maintained and kept current with language/framework evolution
- Init intelligence requires good inference logic (can improve over time)
- ADRs, invariants, and PRDs are first-class artifacts captured via `/edikt:adr`, `/edikt:invariant`, `/edikt:prd`

## Directives

[edikt:directives:start]: #
paths:
  - "**/*"
scope:
  - planning
  - design
  - review
  - implementation
directives:
  - Claude Code is the only supported platform. Do not write code or configuration targeting Cursor, Copilot, or other AI coding tools. (ref: ADR-001)
  - Use a three-tier rule system: base (language-agnostic), lang, framework. One `.md` file per topic per tier. NEVER merge tiers or create subtiers. (ref: ADR-001)
  - Plans are the persistent execution state. Track progress in the plan file's progress table — it survives context compaction. NEVER track plan state elsewhere. (ref: ADR-001)
  - Installation is copy files only — no npm, no package managers, no build step. (ref: ADR-001)
[edikt:directives:end]: #
