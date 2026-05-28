# edikt

**The governance layer for agentic engineering.**

edikt compiles your engineering decisions into directives Claude follows automatically — every session, every engineer, every project.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/v0.4.5/install.sh | bash
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

The installer is POSIX sh. On Windows, run it inside WSL2. Files install to `~/.claude/commands/edikt/` and `~/.edikt/` inside the WSL filesystem; Claude Code for Windows accesses them through the WSL path.

---

[License](LICENSE)
