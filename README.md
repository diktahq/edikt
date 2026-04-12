# edikt

**The governance layer for agentic engineering.**

edikt compiles your engineering decisions into directives Claude follows automatically — every session, every engineer, every project.

```bash
curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
```

Then open any project in Claude Code and say "initialize edikt" or run `/edikt:init`.

## The problem

Without governance, every Claude Code session starts from scratch. Architecture decisions live in your head. Conventions drift between sessions. Each engineer's Claude produces different code for the same standards.

## What edikt does

**Capture decisions.** Architecture Decision Records (ADRs), Invariant Records (hard constraints), and guidelines — all plain markdown.

**Compile into enforcement.** `/edikt:gov:compile` reads your decisions and produces directives Claude reads automatically. MUST/NEVER language with literal code tokens, pre-action reminders, and a verification checklist Claude self-audits against before finishing.

**Govern the lifecycle.** PRD → spec → artifacts → plan → execute → drift detection. Status-gated transitions with specialist agent review at every step.

## What gets installed

- Compiled governance directives Claude reads every session
- 20 rule packs (Go, TypeScript, Python, Next.js, Django, and more)
- 18 specialist agents (architect, dba, security, api, qa, sre, and others)
- 15 lifecycle hooks (plan injection, compaction recovery, quality gates)
- 34 commands from init through drift detection

## Documentation

Full documentation, guides, and examples at **[edikt.dev](https://edikt.dev)**.

- [Getting Started](https://edikt.dev/getting-started) — install and init in 5 minutes
- [What is edikt](https://edikt.dev/what-is-edikt) — the full picture
- [How Governance Compiles](https://edikt.dev/governance/compile) — from decisions to enforcement
- [Invariant Records](https://edikt.dev/governance/invariant-records) — hard constraints that compile into non-negotiable directives
- [Writing Invariants](https://edikt.dev/governance/writing-invariants) — guide for writing effective constraints
- [Commands](https://edikt.dev/commands/) — all commands
- [Governance Quality](https://edikt.dev/commands/gov/score) — score your governance for LLM compliance

## Plain markdown. No build step. No dependencies.

Every file is `.md` or `.yaml` you can read, edit, and version-control.

## Claude Code only

edikt uses Claude Code's platform primitives — path-conditional rules, lifecycle hooks, slash commands, specialist agents. The governance loop only works in Claude Code. The knowledge base (ADRs, specs, invariants) is plain markdown that works anywhere.

---

[License](LICENSE) · [Changelog](CHANGELOG.md) · [edikt.dev](https://edikt.dev)
