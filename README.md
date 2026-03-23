# edikt

**The governance layer for agentic engineering.**

edikt governs your architecture and compiles your engineering decisions into automatic enforcement. It governs the Agentic SDLC from requirements to verification.

```bash
curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
```

Then open any project in Claude Code and run `/edikt:init`.

## What it does

Without edikt, every Claude Code session starts from scratch. Standards live in your head. Decisions get forgotten between sessions. Each engineer's Claude drifts differently.

edikt fixes this with two systems that reinforce each other:

**Architecture governance & compliance.** Capture architecture decisions (ADRs), constraints (invariants), and conventions (guidelines). `/edikt:compile` reads all three, checks for contradictions, and produces a governance file Claude reads automatically — every session, before writing code. Rule packs add correctness guardrails to the same enforcement surface.

**Agentic SDLC governance.** PRD → spec → artifacts → plan → execute → drift detection. Status-gated transitions. Specialist agents review at every critical step. Drift detection verifies what was built matches what was decided.

The lifecycle produces new engineering decisions. Compiled decisions govern the lifecycle. Decisions compound rather than decay.

## The full cycle

```
/edikt:prd             → requirements and acceptance criteria
/edikt:spec            → technical specification
/edikt:spec-artifacts  → data model, API contracts, test strategy
/edikt:plan            → phased execution with specialist review
  execute             → Claude builds with enforced standards
/edikt:drift           → verify implementation matches the spec
```

## What edikt installs

- **20 rule packs** — path-conditional standards (Go, TypeScript, Python, Next.js, Django, and more)
- **18 specialist agents** — architect, dba, security, api, qa, sre, and others
- **9 lifecycle hooks** — auto-format, plan injection, compaction recovery, quality gates
- **Compiled governance** — engineering decisions (ADRs, invariants, guidelines) compile into directives Claude follows automatically every session
- **24 commands** — from init through drift detection

## Research

123 eval runs across 2 experiments prove the enforcement mechanism works. Rules in `.claude/rules/` drive 100% compliance on conventions Claude has never seen in training data (15/15 with rules, 0/15 without). Fully reproducible — see [experiments/](experiments/).

## Documentation

Full documentation, guides, and examples at **[edikt.dev](https://edikt.dev)**.

- [Getting Started](https://edikt.dev/getting-started) — install and init in 5 minutes
- [How It Works](https://edikt.dev/governance/chain) — the governance chain
- [Commands](https://edikt.dev/commands/) — all 24 commands
- [Rule Packs](https://edikt.dev/rules/) — what gets enforced

## Claude Code only

edikt uses Claude Code's platform primitives — path-conditional rules, lifecycle hooks, slash commands, specialist agents, quality gates. Other tools don't have them. The knowledge base (project-context.md, ADRs, specs) is plain markdown that works anywhere. The governance loop only works in Claude Code.

## No build step. No runtime. No magic.

Every file is `.md` or `.yaml` you can read, edit, and version-control. Plain markdown, no dependencies.
