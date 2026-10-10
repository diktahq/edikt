# edikt

**The governance layer for agentic engineering.**

edikt compiles your engineering decisions into directives Claude follows automatically — every session, every engineer, every project.

## How you interact with edikt

You use edikt through **slash commands inside Claude Code** — `/edikt:init`, `/edikt:upgrade`, `/edikt:gov:compile`, `/edikt:doctor`, and so on. That is the primary surface and always will be.

The `edikt` CLI exists too and is fully discoverable (`edikt --help`). It's how the slash commands and lifecycle hooks talk to the deterministic helpers under the hood, and it's available for direct invocation when you need it (debugging, CI, scripting). But the recommended path for every user-facing operation is the slash command — they handle the surrounding context (config resolution, Claude state, recovery paths) that bare CLI invocations don't.

## Install

```bash
curl -fsSL https://github.com/diktahq/edikt/releases/download/v0.8.0/install.sh | bash
```

Installs the v0.8.0 Go launcher (`edikt`) and the versioned payload under `~/.edikt/`. Cosign-verified release assets, pinned to a specific release tag.

The installer asks whether to install globally or into the current project only. Pass `--global` or `--project` to skip the question.

### Or with Homebrew

```bash
brew install diktahq/tap/edikt
```

Homebrew installs **only the launcher binary**. The payload — commands, agents, hooks, rule packs — is fetched separately on first use, so a `brew install` alone does not set a project up.

```bash
edikt install v0.8.0    # fetch the payload
edikt use v0.8.0        # activate it and link the /edikt:* commands
```

**The two halves upgrade independently.** `brew upgrade edikt` replaces the launcher; `edikt upgrade` fetches a new payload and never touches the binary. Run both when moving between versions — a launcher newer than its payload, or the reverse, is a supported state but not usually the one you want.

Then open any project in Claude Code and say "initialize edikt" or run `/edikt:init`.

### Switch versions after installing

The launcher manages multiple installed versions side-by-side. Use `/edikt:upgrade` for the full upgrade flow; the binary commands below are also available for direct use:

```bash
edikt list                 # show all installed versions
edikt install v0.8.0       # download and install a specific v0.8.x release
edikt use v0.8.0           # activate it (switches the ~/.edikt/current symlink)
edikt rollback             # revert to the previous version
```

`/edikt:upgrade-pin` updates your project's `.edikt/config.yaml` to match whatever is active.

### Pin to a specific v0.8.x version at install time

```bash
curl -fsSL https://github.com/diktahq/edikt/releases/download/v0.8.0/install.sh | bash -s -- --ref v0.8.0
```

Works for any tag with release assets (v0.7.0 and forward). `--ref` is the only supported way to pin a version — there is no `EDIKT_REF` environment variable, and passing one has no effect.

### Stay on the v0.4 line (legacy)

If your project isn't ready for the sidecar architecture (introduced in v0.6.0 and current through v0.9.0), the v0.4.5 install path is still supported. v0.4.x lives in [diktahq/edikt-legacy](https://github.com/diktahq/edikt-legacy) (archived, read-only) — the current repo's history starts at v0.7.0:

```bash
curl -fsSL https://raw.githubusercontent.com/diktahq/edikt-legacy/v0.4.5/install.sh | bash
```

v0.4.5's installer uses a different fetch model (raw git-tag, not release assets), so its `--ref` flag only resolves to other v0.4.x tags — not v0.7.x:

```bash
curl -fsSL https://raw.githubusercontent.com/diktahq/edikt-legacy/v0.4.5/install.sh | bash -s -- --ref v0.4.3
```

v0.5.x is retracted. v0.7.x's installer cannot install v0.4.x (no release assets at those tags); use the v0.4.5 URL directly.

### Upgrading from an earlier line

Run `/edikt:upgrade` from inside Claude Code in your project. It installs the launcher and handles the cross-major layout migration (flat → versioned `~/.edikt/`).

Migrating your governance artifacts is state-based and per artifact, not version-based: an ADR, invariant or guideline needs migrating when its `.md` still carries an in-body directive block, or when the sidecar beside it is still `schema_version: 1`. When `/edikt:upgrade` finds any, it prints the list and points to `/edikt:migrate`, which lifts the blocks into `.edikt.yaml` sidecars, proves nothing was dropped, and recompiles. Upgrade itself applies no migration, and no other command does either — compile, doctor and init refuse and point to `/edikt:migrate` until the state is clean.

## The problem

Without governance, every Claude Code session starts from scratch. Architecture decisions live in your head. Conventions drift between sessions. Each engineer's Claude produces different code for the same standards.

## What edikt does

**Capture decisions.** Architecture Decision Records (ADRs), Invariant Records (hard constraints), and guidelines — all plain markdown.

Which kind a thing is has one answer, asked in one order. An invariant is a property that is never allowed to be false. An ADR is a choice you made when you could have chosen otherwise. A guideline is a default you follow unless you can name why not. Everything else is a commit message. The full decision, with the near-misses that catch the common mistakes, is in [docs/guides/governance-artifacts.md](docs/guides/governance-artifacts.md).

**Compile into the context Claude works in.** `/edikt:gov:compile` reads your decisions and produces directives Claude reads before it writes. The non-negotiable rules load on every edit. The rest load by topic, when Claude touches a file the topic covers or when the task calls for it. Directives use MUST/NEVER language with literal code tokens, and each topic carries a checklist Claude checks itself against before finishing.

**Check the work at the end of each phase.** When a plan phase finishes, a separate Claude process with no access to the working session checks the phase's acceptance criteria, and a post-flight review checks the diff against your directives. Nothing is blocked or rewritten while Claude is editing. The check happens once the phase is done.

**Govern the lifecycle.** PRD → spec → artifacts → plan → execute → drift detection. Status-gated transitions with specialist agent review at every step.

## What gets installed

- Compiled governance directives Claude reads every session
- 20 rule packs (Go, TypeScript, Python, Next.js, Django, and more)
- 26 specialist agents (architect, dba, security, api, qa, sre, and others)
- 21 lifecycle hooks (session start and end, compaction recovery, phase-end detection, subagent and worktree events)
- 50 commands, from init through drift detection

## Documentation

Full documentation, guides, and examples at **[edikt.dev](https://edikt.dev)**.

- [Getting Started](https://edikt.dev/getting-started) — install and init in 5 minutes
- [How Governance Compiles](https://edikt.dev/governance/compile) — from decisions to the surfaces Claude reads
- [Commands](https://edikt.dev/commands/) — all commands

## Plain markdown. No build step.

Every file is `.md` or `.yaml` you can read, edit, and version-control.

Commands, templates and rules have no dependencies. Each hook is a short shell
shim that runs the `edikt` binary, so hooks don't need `python3` or anything
else installed.

## Claude Code only

edikt uses Claude Code's platform primitives — path-conditional rules, lifecycle hooks, slash commands, specialist agents. The governance loop only works in Claude Code. The knowledge base (ADRs, specs, invariants) is plain markdown that works anywhere.

### Windows / WSL

On Windows, run inside WSL2. The payload installs to `~/.edikt/` inside the WSL filesystem. Claude Code for Windows accesses it through the WSL path.

### Claude Code parity

edikt tracks Claude Code feature adoption internally. Hook protocol, agent frontmatter fields (`effort`, `maxTurns`, `disallowedTools`, `initialPrompt`), conditional hook `if`, and the full PostCompact / SubagentStart / TaskCompleted / WorktreeCreate event set are all adopted. Plugin packaging is a future candidate.

---

[License](LICENSE) · [Changelog](CHANGELOG.md) · [edikt.dev](https://edikt.dev)
