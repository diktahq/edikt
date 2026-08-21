# edikt

**The governance layer for agentic engineering.**

edikt compiles your engineering decisions into directives Claude follows automatically — every session, every engineer, every project.

## How you interact with edikt

You use edikt through **slash commands inside Claude Code** — `/edikt:init`, `/edikt:upgrade`, `/edikt:gov:compile`, `/edikt:doctor`, and so on. That is the primary surface and always will be.

The `bin/edikt` CLI exists too and is fully discoverable (`edikt --help`). It's how the slash commands and lifecycle hooks talk to the deterministic helpers under the hood, and it's available for direct invocation when you need it (debugging, CI, scripting). But the recommended path for every user-facing operation is the slash command — they handle the surrounding context (config resolution, Claude state, recovery paths) that bare CLI invocations don't.

## Install

```bash
curl -fsSL https://github.com/diktahq/edikt/releases/download/v0.7.3/install.sh | bash
```

Installs the v0.7.3 Go launcher (`bin/edikt`) and the versioned payload under `~/.edikt/`. Cosign-verified release assets, pinned to a specific release tag.

Then open any project in Claude Code and say "initialize edikt" or run `/edikt:init`.

### Switch versions after installing

The launcher manages multiple installed versions side-by-side. Use `/edikt:upgrade` for the full upgrade flow; the binary commands below are also available for direct use:

```bash
edikt list                 # show all installed versions
edikt install v0.7.3       # download and install a specific v0.7.x release
edikt use v0.7.3           # activate it (switches the ~/.edikt/current symlink)
edikt rollback             # revert to the previous version
```

`/edikt:upgrade-pin` updates your project's `.edikt/config.yaml` to match whatever is active.

### Pin to a specific v0.7.x version at install time

```bash
curl -fsSL https://github.com/diktahq/edikt/releases/download/v0.7.3/install.sh | bash -s -- --ref v0.7.3
```

Works for any tag with release assets (v0.7.0 and forward). `--ref` is the only supported way to pin a version — there is no `EDIKT_REF` environment variable, and passing one has no effect.

### Stay on the v0.4 line (legacy)

If your project isn't ready for the sidecar architecture (introduced in v0.6.0 and current through v0.7.3), the v0.4.5 install path is still supported. v0.4.x lives in [diktahq/edikt-legacy](https://github.com/diktahq/edikt-legacy) (archived, read-only) — the current repo's history starts at v0.7.0:

```bash
curl -fsSL https://raw.githubusercontent.com/diktahq/edikt-legacy/v0.4.5/install.sh | bash
```

v0.4.5's installer uses a different fetch model (raw git-tag, not release assets), so its `--ref` flag only resolves to other v0.4.x tags — not v0.7.x:

```bash
curl -fsSL https://raw.githubusercontent.com/diktahq/edikt-legacy/v0.4.5/install.sh | bash -s -- --ref v0.4.3
```

v0.5.x is retracted. v0.7.x's installer cannot install v0.4.x (no release assets at those tags); use the v0.4.5 URL directly.

### Upgrading from v0.4.x to v0.7.3

Run `/edikt:upgrade` from inside Claude Code in your project. The flow handles the cross-major layout migration (flat → versioned `~/.edikt/`), launcher install, and the one-time `migrate sidecars --apply` step that lifts legacy in-body sentinels into `.edikt.yaml` sidecars.

## The problem

Without governance, every Claude Code session starts from scratch. Architecture decisions live in your head. Conventions drift between sessions. Each engineer's Claude produces different code for the same standards.

## What edikt does

**Capture decisions.** Architecture Decision Records (ADRs), Invariant Records (hard constraints), and guidelines — all plain markdown.

**Compile into enforcement.** `/edikt:gov:compile` reads your decisions and produces directives Claude reads automatically. MUST/NEVER language with literal code tokens, pre-action reminders, and a verification checklist Claude self-audits against before finishing.

**Govern the lifecycle.** PRD → spec → artifacts → plan → execute → drift detection. Status-gated transitions with specialist agent review at every step.

## What gets installed

- Compiled governance directives Claude reads every session
- 20 rule packs (Go, TypeScript, Python, Next.js, Django, and more)
- 25 specialist agents (architect, dba, security, api, qa, sre, and others)
- 22 lifecycle hooks (plan injection, compaction recovery, quality gates)
- 51 commands from init through drift detection

## Documentation

Full documentation, guides, and examples at **[edikt.dev](https://edikt.dev)**.

- [Getting Started](https://edikt.dev/getting-started) — install and init in 5 minutes
- [How Governance Compiles](https://edikt.dev/governance/compile) — from decisions to enforcement
- [Commands](https://edikt.dev/commands/) — all commands

## Plain markdown. No build step.

Every file is `.md` or `.yaml` you can read, edit, and version-control.

Tier-1 (commands, templates, rules) has no dependencies. The hook tier does:
21 of 25 shipped hooks invoke `python3` (F-061). It now fails loudly and
degrades to `warn`/advisory when `python3` is missing, rather than silently
disabling governance — but the dependency is real. `install.sh` checks for
it before it writes your `settings.json`.

## Claude Code only

edikt uses Claude Code's platform primitives — path-conditional rules, lifecycle hooks, slash commands, specialist agents. The governance loop only works in Claude Code. The knowledge base (ADRs, specs, invariants) is plain markdown that works anywhere.

### Windows / WSL

On Windows, run inside WSL2. The payload installs to `~/.edikt/` inside the WSL filesystem. Claude Code for Windows accesses it through the WSL path.

### Claude Code parity

edikt tracks Claude Code feature adoption internally. Hook protocol, agent frontmatter fields (`effort`, `maxTurns`, `disallowedTools`, `initialPrompt`), conditional hook `if`, and the full PostCompact / SubagentStart / TaskCompleted / WorktreeCreate event set are all adopted. Plugin packaging is a future candidate.

---

[License](LICENSE) · [Changelog](CHANGELOG.md) · [edikt.dev](https://edikt.dev)
