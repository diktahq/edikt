# Keeping edikt Up to Date

edikt has two layers that need updating separately: the **global templates** (installed on your machine) and the **project configuration** (committed in each repo).

## How versioning works

Every edikt install writes its version to `~/.edikt/VERSION`. Every project records its edikt version in `.edikt/config.yaml`:

```yaml
edikt_version: "0.1.0"
```

`/edikt:doctor` compares the two and warns when they differ:
```
[!!] project on edikt 0.1.0, installed is 0.2.0 — run /edikt:upgrade
```

`/edikt:upgrade` reads both, shows a diff, applies changes, and bumps `edikt_version` in your config when done.

## Update flow

### Step 1 — Update global templates

```bash
curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
```

This takes ~10 seconds. Your commands, templates, and `~/.edikt/VERSION` are now current.

### Step 2 — Upgrade each project

Open the project in Claude Code and say:

> "Upgrade edikt"

edikt first checks if your global install is current. If a newer version exists on GitHub, it tells you to re-run the installer before proceeding — ensuring project upgrades always use the latest templates. Use `--offline` to skip this check in air-gapped environments.

Then it shows a diff of what will change:

```text
WHAT'S NEW
─────────────────────────────────────────────────────
v0.2.0 — New agents, rule pack updates, hook improvements
─────────────────────────────────────────────────────

Installed edikt: 0.2.0
Project edikt:   0.1.0

EDIKT UPGRADE
─────────────────────────────────────────────────────
Hooks (.claude/settings.json)
  ⬆  SessionStart   — updated
  ✓  PostToolUse    — up to date

Agents (.claude/agents/)
  ⬆  dba.md         — template updated
  ✓  architect.md   — up to date
  —  my-reviewer.md — custom, skipped

Rule packs (.claude/rules/)
  ⬆  go.md          1.0.0 → 1.1.0
  ✓  testing.md     — up to date
  —  my-custom.md   — custom, skipped

─────────────────────────────────────────────────────
Apply these upgrades? (y/n/select)
```

You can apply everything, cancel, or choose sections. After applying, `edikt_version` in `.edikt/config.yaml` is bumped to match.

### Step 3 — Share with your team

```bash
git add .claude/ .edikt/config.yaml && git commit -m "chore: upgrade edikt to 0.2.0"
git push
```

Your team gets the upgrade on next pull.

---

## Protecting customizations

**Agents** — Add `<!-- edikt:custom -->` to any agent file to skip it during upgrade. Or list custom agents in config:

```yaml
agents:
  custom:
    - dba              # team has customized
    - my-team-reviewer # not from edikt templates
```

**Rules** — Files without the `<!-- edikt:generated -->` marker are always skipped. Files with an `extend:` config keep the extension untouched while the base pack updates.

**Hooks** — edikt only updates its own hook entries. Hooks you added yourself are never removed.

**Config** — `edikt_version` is updated. New config blocks (like `artifacts:` in v0.1.1) are added if missing. Existing values are never overwritten.

**Commands** — `install.sh` checks for `<!-- edikt:custom -->` before overwriting commands. Customized commands survive reinstall.

---

## What gets upgraded

**Hooks** — New edikt versions add hook capabilities or fix bugs. Old inline bash hooks get migrated to `~/.edikt/hooks/*.sh` script references.

**Agent templates** — Specialist agents are periodically improved with better prompts and domain coverage.

**Rule packs** — Rule packs are versioned. Outdated packs are updated. Manually edited files are always skipped.

**CLAUDE.md sentinels** — v0.1.1 migrates CLAUDE.md section markers from HTML comments (`<!-- edikt:start -->`) to visible text markers (`[edikt:start]: #`). Claude Code v2.1.72+ hides HTML comments, so the old markers were invisible to Claude. Upgrade detects and migrates automatically.

---

## Upgrading to v0.2.0

v0.2.0 changes how governance is compiled. The flat `governance.md` is replaced by topic-grouped rule files. Here's what to expect:

**Compile migration** — running `/edikt:compile` automatically migrates from the old flat format to topic-grouped files:

```text
📦 Migrated from flat governance.md to topic-grouped rule files.
   Old format: 1 file, 13 directives
   New format: 3 topic files + index
```

**Directive sentinels** — for full-fidelity compilation, run `/edikt:review-governance` to generate directive sentinel blocks in your ADRs and invariants. Without sentinels, compile falls back to extraction (same quality as v0.1.x).

**Agent governance** — all agent templates now have `maxTurns`, `disallowedTools`, and `effort`. The new `evaluator` agent is installed automatically.

**New hooks** — 4 new hook events (StopFailure, TaskCreated, CwdChanged, FileChanged) and conditional `if` field on 2 existing hooks.

**Installer safety** — reinstalling now backs up existing files. Use `--dry-run` to preview changes.

**Recommended upgrade steps:**

```text
1. Update global:    curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
2. Upgrade project:  /edikt:upgrade
3. Generate sentinels: /edikt:review-governance
4. Recompile:        /edikt:compile
5. Commit:           git add .claude/ .edikt/ docs/ && git commit -m "chore: upgrade edikt to 0.2.0"
```

---

## Checking if a project needs upgrading

> "What's our status?"

Or directly:

> "Run doctor"

Doctor shows version status:
```text
[!!] project on edikt 0.1.0, installed is 0.2.0 — run /edikt:upgrade
[!!] go.md outdated (installed: 1.0.0, available: 0.1.0) — run /edikt:upgrade
```

---

## Managing multiple projects

For teams with many repos:

1. One person runs the installer
2. Upgrades each project: "upgrade edikt"
3. Commits and pushes

There's no central push mechanism — edikt stays offline-first and git-native.
