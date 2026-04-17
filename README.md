# edikt

**The governance layer for agentic engineering.**

edikt compiles your engineering decisions into directives Claude follows automatically — every session, every engineer, every project.

## What's new in v0.5.0

**Stability (SPEC-004)**
- Hook JSON protocol — all 20 lifecycle hooks now emit structured JSON output conforming to the Claude Code hook protocol
- Homebrew tap (`brew install diktahq/tap/edikt`) with two-tier update model
- Provenance frontmatter on generated files with upgrade-safe 3-way diff flow

**Directive hardening + governance benchmark (SPEC-005)**
- New optional sentinel fields `canonical_phrases` and `behavioral_signal` — backward-compatible, existing ADRs unchanged
- `/edikt:gov:benchmark` — tier-2 adversarial benchmark; install separately with `./bin/edikt install benchmark`; 2/2 PASS on INV-001 + INV-002 in the dogfood baseline run
- `/edikt:adr:review` now flags 6 soft-language markers (`should`, `ideally`, `prefer`, `try to`, `might`, `consider`) and adds `--backfill` to retrofit `canonical_phrases` onto existing ADRs
- `/edikt:gov:compile` orphan ADR detection with warn-then-block semantics; state persisted in `.edikt/state/compile-history.json`
- `/edikt:doctor` now verifies every ADR/INV cited in the routing table exists on disk

See [CHANGELOG](CHANGELOG.md) for the full release notes.

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
- 20 specialist agents (architect, dba, security, api, qa, sre, and others)
- 20 lifecycle hooks (plan injection, compaction recovery, quality gates)
- 35 commands from init through drift detection

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

### Claude Code parity

edikt tracks Claude Code feature adoption in [docs/internal/claude-code-parity.md](docs/internal/claude-code-parity.md). The v0.5.0 baseline is Claude Code v2.1.111 (April 2026). Hook protocol, agent frontmatter fields (`effort`, `maxTurns`, `disallowedTools`, `initialPrompt`), conditional hook `if`, and the full PostCompact / SubagentStart / TaskCompleted / WorktreeCreate event set are all adopted. Plugin packaging is a v0.6.0+ candidate.

---

[License](LICENSE) · [Changelog](CHANGELOG.md) · [edikt.dev](https://edikt.dev)
