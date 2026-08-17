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
📦 edikt v0.7.1 is available (you have v0.7.0).

  Update now:
    curl -fsSL https://github.com/diktahq/edikt/releases/download/v0.7.1/install.sh | bash

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

## Provenance-first upgrade

v0.6.0 replaced the hash-diff classifier with a provenance-first upgrade flow. Every generated file now carries `edikt_template_hash` (MD5 of the source template before substitution). On upgrade:

| Situation | Action |
|---|---|
| Template unchanged (`stored_hash == current_hash`) | Silent skip — your file is fine |
| Template changed, you didn't edit | Auto-apply — you never touched it |
| Template changed AND you edited | 3-way diff prompt — you decide |
| File has `<!-- edikt:custom -->` | Always skip, regardless of template changes |
| File has no `edikt_template_hash` (pre-v0.6.0) | Legacy classifier (v0.4.3 diff heuristic) |

## Pinning only

The last step of an upgrade is bumping this project's pin. `/edikt:upgrade` does it for you as part of the full flow — but you can run just that step on its own.

**`/edikt:upgrade-pin` updates this project's `.edikt/config.yaml` `edikt_version:` field to match the currently active edikt install.** All other config content is preserved byte-for-byte.

Use it when you see the warning:

```
warn: this project pins edikt X.Y.Z but the active version is A.B.C
      Run `edikt upgrade-pin` inside the project to update the pin.
```

### Synopsis

```bash
/edikt:upgrade-pin
```

Per the principle that slash commands are the primary user surface, invoke `/edikt:upgrade-pin` rather than calling `edikt upgrade-pin` directly. The binary remains discoverable via `edikt --help`.

### How it works

1. **Config guard.** If no `.edikt/config.yaml` exists in the current directory or any ancestor, the command stops and points you at `/edikt:init`.
2. **Confirm current state.** Compares `edikt version` (the active install) against the `edikt_version:` the project pins. If they already match, it reports "already aligned" and stops.
3. **Run the pin update.** Invokes `edikt upgrade-pin`, which walks up from the current directory to find `.edikt/config.yaml`, updates or appends the `edikt_version` field, and exits 0. Every other line in the file is left untouched.
4. **Verify and stage.** Surfaces `git diff .edikt/config.yaml` so you can confirm only `edikt_version` changed, and suggests a focused commit such as `chore(edikt): pin to vX.Y.Z to match installed launcher`.

### When to run the pin on its own

- After installing a new edikt version that changes the launcher (e.g. v0.4.x → v0.6.x), once you've decided to adopt it in this project.
- After running the full upgrade flow above, to align the project pin with what's actually loaded.
- When a teammate bumped the active version but the committed `.edikt/config.yaml` is stale.

### When not to run it

- If you intentionally pin this project to an older version (e.g. you need v0.4.x behavior while the active install is v0.6.x). In that case run `edikt use <pinned-version>` to switch the active install down to match instead.

### Notes

- This does only the last step of an upgrade — bumping the pin. The full upgrade flow (fetch new version → migrate sidecars if needed → update hooks/agents/CLAUDE.md → bump pin) is what the rest of this page describes.
- If `bin/edikt` is missing, bootstrap it by running `/edikt:upgrade` in Claude Code first.

Related: [`/edikt:config`](/commands/config) views and changes `.edikt/config.yaml`; [`/edikt:doctor`](/commands/doctor) surfaces the version-pin drift warning.

## Rollback

After upgrade, revert the payload to the previous version:

```bash
edikt rollback
```

This is a launcher-level operation, not a command. See [Payload version management](../guides/upgrading.md#payload-version-management).

## v0.6.0 sidecar migration

If your project still has legacy in-body `[edikt:directives:start]` sentinel blocks, the upgrade detects them and prompts before applying:

```text
EDIKT UPGRADE — v0.6.0 sidecar migration (Phase A: structural strip)
─────────────────────────────────────────────────────
Detected 14 artifacts with legacy [edikt:directives:start] blocks.

All shapes (v0.5.x-full / v0.5.x-partial / v0.4.3-legacy / unknown)
take the same code path: strip the in-body block, write a skeleton
sidecar with topic: needs-extraction, preserve the legacy contents
verbatim in a transient migration_preserved: field.

Phase B (canonical extraction) runs next via /edikt:gov:compile.

Apply now? [y/N]
```

The detection scan respects the skip-list (files that document the sentinel format, SPEC-* files) and excludes sentinel blocks inside fenced code regions. The migration tool runs `--dry-run` first, prints the plan, and on `y` runs `--apply`. After Phase A completes (post-migration sentinel verification passes), `/edikt:upgrade` runs `/edikt:gov:compile` to dispatch the `sidecar-extractor` agent per artifact (Phase B) and runs the post-extractor lossless gate. On `N`, the upgrade prints:

```text
Migration deferred. Run /edikt:gov:compile to apply when ready.
Compile will refuse until migration is applied.
```

There is no double-parser window. `/edikt:gov:compile` exits 1 with an actionable error when in-body sentinels are still present in non-skip-list, non-fenced files.

The full two-phase model is in [`/edikt:migrate sidecars`](/commands/migrate). The walkthrough is in [Sidecar Migration](/guides/sidecar-migration).

## Natural language triggers

- "upgrade edikt"
- "update edikt hooks"
- "my edikt hooks are outdated"
- "update to latest edikt"
