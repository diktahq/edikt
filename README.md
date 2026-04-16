# edikt

**The governance layer for agentic engineering.**

edikt compiles your engineering decisions into directives Claude follows automatically — every session, every engineer, every project.

## Install

### macOS / Linux (via Homebrew)

```bash
brew install diktahq/tap/edikt
edikt install
```

### Any platform (via curl)

```bash
curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
```

Then open any project in Claude Code and say "initialize edikt" or run `/edikt:init`.

### Upgrading from v0.4.x?

Re-run the curl command, then run `edikt migrate --yes`. See [Migrating from v0.4](website/guides/migrating-from-v0.4.md).

### Upgrade and rollback

```bash
edikt upgrade          # fetch and activate the latest payload
edikt rollback         # revert to the previous payload version
edikt use v0.5.0       # pin to a specific version
edikt list             # show all installed versions
```

`brew upgrade edikt` updates the launcher binary. `edikt upgrade` updates the payload (templates, commands, hooks) independently. See [Upgrade and rollback](website/guides/upgrade-and-rollback.md).

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
- [Upgrade and rollback](website/guides/upgrade-and-rollback.md) — `edikt upgrade`, rollback, pinning
- [Migrating from v0.4](website/guides/migrating-from-v0.4.md) — step-by-step v0.4.x → v0.5.0
- [Homebrew install](website/guides/homebrew.md) — tap install and two-tier update model
- [How Governance Compiles](https://edikt.dev/governance/compile) — from decisions to enforcement
- [Commands](https://edikt.dev/commands/) — all commands

## Plain markdown. No build step. No dependencies.

Every file is `.md` or `.yaml` you can read, edit, and version-control.

## Claude Code only

edikt uses Claude Code's platform primitives — path-conditional rules, lifecycle hooks, slash commands, specialist agents. The governance loop only works in Claude Code. The knowledge base (ADRs, specs, invariants) is plain markdown that works anywhere.

### Windows / WSL

The launcher (`bin/edikt`) is POSIX sh. On Windows, run inside WSL2. The payload installs to `~/.edikt/` inside the WSL filesystem. Claude Code for Windows accesses it through the WSL path.

---

[License](LICENSE) · [Changelog](CHANGELOG.md) · [edikt.dev](https://edikt.dev)
