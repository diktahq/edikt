# edikt

**The governance layer for agentic engineering.**

edikt compiles your engineering decisions into directives Claude follows automatically — every session, every engineer, every project.

## Install

```bash
curl -fsSL https://github.com/diktahq/edikt/releases/download/v0.4.5/install.sh | bash
```

## The problem

Without governance, every Claude Code session starts from scratch. Architecture decisions live in your head. Conventions drift between sessions. Each engineer's Claude produces different code for the same standards.

## What edikt does

**Capture decisions.** Architecture Decision Records (ADRs), Invariant Records (hard constraints), and guidelines — all plain markdown.

**Compile into enforcement.** `/edikt:gov:compile` reads your decisions and produces directives Claude reads automatically. MUST/NEVER language with literal code tokens, pre-action reminders, and a verification checklist Claude self-audits against before finishing.

**Govern the lifecycle.**

## What gets installed

- Compiled governance directives Claude reads every session
- 20 rule packs (Go, TypeScript, Python, Next.js, Django, and more)
- 20 specialist agents (architect, dba, security, api, qa, sre, and others)
- 20 lifecycle hooks (plan injection, compaction recovery, quality gates)
- 35 commands from init through drift detection

## Plain markdown. No build step. No dependencies.

Every file is `.md` or `.yaml` you can read, edit, and version-control.

## Claude Code only

edikt uses Claude Code's platform primitives — path-conditional rules, lifecycle hooks, slash commands, specialist agents. The governance loop only works in Claude Code. The knowledge base (ADRs, specs, invariants) is plain markdown that works anywhere.

### Windows / WSL

The launcher (`bin/edikt`) is POSIX sh. On Windows, run inside WSL2. The payload installs to `~/.edikt/` inside the WSL filesystem. Claude Code for Windows accesses it through the WSL path.

### Claude Code parity

edikt tracks Claude Code feature adoption in [docs/internal/claude-code-parity.md](docs/internal/claude-code-parity.md). The v0.5.0 baseline is Claude Code v2.1.111 (April 2026). Hook protocol, agent frontmatter fields (`effort`, `maxTurns`, `disallowedTools`, `initialPrompt`), conditional hook `if`, and the full PostCompact / SubagentStart / TaskCompleted / WorktreeCreate event set are all adopted. Plugin packaging is a v0.6.0+ candidate.

---

[License](LICENSE) · [Changelog](CHANGELOG.md) · [edikt.dev](https://edikt.dev)
