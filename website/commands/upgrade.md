# /edikt:upgrade

Upgrade edikt in this project — hooks, agents, and rule packs — using the provenance-first flow. Detects what actually changed vs what you customized, and presents a 3-way diff when both sides diverged.

## Usage

```bash
/edikt:upgrade
/edikt:upgrade --offline
```

| Argument | Description |
|----------|-------------|
| (none) | Checks for updates, then upgrades the project |
| `--offline` | Skip the remote version check (air-gapped environments) |

## Version check

Before upgrading the project, edikt checks if a newer version is available on GitHub. Three outcomes:

- **Newer version available** — shows the install command and stops. You update globally first, then re-run upgrade.
- **Up to date** — proceeds silently to the project upgrade.
- **No network** — warns and continues with the installed version.

```text
📦 edikt 0.2.0 is available (you have 0.1.3).

  Update now:
    curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash

  Then re-run /edikt:upgrade to apply changes to this project.
  To skip this check: /edikt:upgrade --offline
```

This ensures project upgrades always use the latest templates. Skip with `--offline` for air-gapped or CI environments.

## The problem it solves

When you install a new version of edikt (`curl ... | bash`), your global templates are updated but existing projects are not. Each project has its own `.claude/settings.json` (hooks) and `.claude/agents/` (agent templates) that were generated at init time and don't update automatically.

`/edikt:upgrade` bridges that gap. It compares what's installed in your project against the current templates and shows you exactly what would change — before touching anything.

## What it upgrades

**Hooks** (`.claude/settings.json`)
- SessionStart: add git-awareness — surfaces relevant agents based on what changed since last session
- PostToolUse: add auto-format hook (gofmt, prettier, black, rubocop, rustfmt)
- Stop: add doc gap and security signal detection
- PreCompact: add `/edikt:session` reminder

**Agent templates** (`.claude/agents/`)
- Updates edikt-managed agents to latest template versions
- Never touches agents marked with `<!-- edikt:custom -->` in the file
- Never touches agents listed under `agents.custom` in `.edikt/config.yaml`
- Never touches user-created agents (no matching edikt template)

**Rule packs** (`.claude/rules/`)
- Updates outdated rule packs (same logic as `/edikt:gov:rules-update`)
- Never touches manually edited files (no `<!-- edikt:generated -->` marker)
- Never touches custom rules not in the edikt registry

## Safe by design

- **Shows a diff summary before applying** — you see exactly what changes
- **Asks for confirmation** — apply all, cancel, or select sections individually
- **Never overwrites customizations** — manually edited agents and rules are skipped
- **Never removes user-added hooks** — only updates edikt-managed hook entries
- **Additive for missing hooks** — if PostToolUse is missing, it's added without touching the rest

## Protecting customized agents

Two mechanisms tell upgrade to skip an agent:

**File marker** — add `<!-- edikt:custom -->` anywhere in the agent file:

```yaml
---
name: dba
description: "..."
<!-- edikt:custom -->
tools:
  - Read
  - Grep
---
```

**Config** — list agents in `.edikt/config.yaml`:

```yaml
agents:
  custom:
    - dba              # skip on upgrade
    - my-team-reviewer # not from edikt templates
```

Config takes precedence over the file marker. Both protect the agent from upgrade.

## Output

```text
EDIKT UPGRADE
─────────────────────────────────────────────────────
Hooks (.claude/settings.json)
  ⬆  SessionStart   — add git-awareness
  ⬆  PostToolUse    — missing, will add auto-format hook
  ✓  PreToolUse     — up to date

Agents (.claude/agents/)
  ⬆  dba.md        — template updated
  ✓  architect.md  — up to date
  —  security.md   — custom, skipped

Rule packs (.claude/rules/)
  ⬆  go.md          1.0 → 1.2
  —  my-custom.md   — custom, skipped

─────────────────────────────────────────────────────
Apply these upgrades? (y/n/select)
```

## What's new

After every upgrade, edikt shows the release notes for the new version — the relevant section from the changelog — so you know what changed without having to look it up:

```text
WHAT'S NEW in 0.2.0
─────────────────────────────────────────────────────
{changelog content for this release}
─────────────────────────────────────────────────────
```

This appears whether or not actual changes were applied — if the project was already up to date, you still see the notes for the current version.

## Sharing upgrades with your team

After upgrading, commit the changes:

```bash
git add .claude/ && git commit -m "chore: upgrade edikt to latest"
```

Your team gets the upgrade on next pull — no manual steps needed.

## Provenance-first upgrade (v0.5.0)

v0.5.0 replaced the hash-diff classifier with a provenance-first upgrade flow. Every generated file now carries `edikt_template_hash` (MD5 of the source template before substitution). On upgrade:

| Situation | Action |
|---|---|
| Template unchanged (`stored_hash == current_hash`) | Silent skip — your file is fine |
| Template changed, you didn't edit | Auto-apply — you never touched it |
| Template changed AND you edited | 3-way diff prompt — you decide |
| File has `<!-- edikt:custom -->` | Always skip, regardless of template changes |
| File has no `edikt_template_hash` (pre-v0.5.0) | Legacy classifier (v0.4.3 diff heuristic) |

## Rollback

After upgrade, revert the payload to the previous version:

```bash
edikt rollback
```

This is a launcher-level operation, not a command. See [Upgrade and rollback](../guides/upgrade-and-rollback.md).

## Migration

Upgrading from v0.4.x? See [Migrating from v0.4](../guides/migrating-from-v0.4.md).

## Homebrew users

`brew upgrade edikt` updates the launcher binary. `edikt upgrade` updates the payload. They're independent. See [Homebrew install](../guides/homebrew.md) for the full two-tier model.

## Natural language triggers

- "upgrade edikt"
- "update edikt hooks"
- "my edikt hooks are outdated"
- "update to latest edikt"
