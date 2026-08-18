# Keeping edikt Up to Date

::: tip Current entry point
**v0.7.1** is the current release and the entry point for upgrading. `diktahq/edikt`'s prior public releases stopped at **v0.4.5** — v0.5.x was retracted (ADR-042) and v0.6.0 was never published — but v0.7.0 restarted the release line and it, along with v0.7.1, is fully installable. If you have an installed project on v0.4.x, run `/edikt:upgrade` and it carries you straight to v0.7.1 in one pass — the migration and schema-upgrade steps handle the full jump uniformly.
:::

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
curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/v0.7.1/install.sh | bash
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

**CLAUDE.md sentinels** — v0.1.1 migrates CLAUDE.md section markers from HTML comments (`<!-- edikt:start -->`) to visible text markers (`[edikt:start]: #`). Claude Code v2.1.72+ hides HTML comments, so the old markers were invisible to the model. Upgrade detects and migrates automatically.

---

## Earlier versions

Nobody upgrading today lands on these, and `/edikt:upgrade` applies whatever migrations they introduced as part of the single v0.4.x → v0.7.1 pass. They're recorded for context, not as recipes to follow.

**v0.2.0** — replaced the flat `governance.md` with topic-grouped rule files, gave every agent template `maxTurns` / `disallowedTools` / `effort`, and added four hook events. `/edikt:gov:compile` performs the format migration itself; there was never a manual step.

**v0.3.0** — introduced the three-list directive schema (`directives:`, `manual_directives:`, `suppressed_directives:`), Invariant Records, generated `reminders:` and `verification:` lists, and `/edikt:gov:score`. v0.2.x blocks carrying only `directives:` stay readable — the missing lists are treated as empty.

**v0.5.0** — retracted (ADR-042), never released. Its two real changes, the hook JSON protocol and the versioned `~/.edikt/` layout, both survive inside v0.7.0. There is no v0.5.x install path.

---

## Sentinel-to-sidecar migration (mechanics reference — not a version to install)

This migration moved compiled governance metadata out of in-body sentinel blocks into co-located
**sidecars** (`<artifact>.edikt.yaml`) — a one-way structural change; the prose `.md` becomes immutable
to edikt, and the sidecar holds every compiled directive. It originally shipped as v0.6.0's headline
change, but v0.6.0 itself was never a published release — no public project ran this as a standalone
step. It's documented here because `/edikt:upgrade` still runs it internally, as one step of the real
v0.4.x → v0.7.0 path below.

The migration is two-phase: **Phase A** (`edikt migrate sidecars --apply`, pure Go, no LLM) strips
every in-body sentinel block and writes a skeleton sidecar with the legacy content preserved verbatim
in a transient `migration_preserved:` field; **Phase B** (`/edikt:gov:compile`) dispatches the
`sidecar-extractor` agent to turn that preserved content into canonical directives, with a
post-extraction lossless gate verifying nothing was dropped.

Full step-by-step detail, recovery paths, and rollback mechanics are in
[Sidecar Migration](/guides/sidecar-migration) — the per-artifact walkthrough with example output and
edge cases.

---

## Upgrading to v0.7 — the real path, v0.4.x → v0.7.1 directly

**This is the upgrade path that actually applies to you.** `diktahq/edikt`'s releases were retracted or
unpublished between v0.4.5 and v0.7.0 — v0.5.x and v0.6.0 were never published. If you're on a real,
installed project today and haven't upgraded since, you're on v0.4.x, and `/edikt:upgrade` carries you
straight to v0.7.1 in one run: the v0.6.0-era sentinel migration above, the v0.7.0 sidecar schema
upgrade, and a recompile under v0.7.0's corrected grading — all in one pass, no intermediate version to
install separately.

**Read this before you run it, not after — one change here isn't a migration step at all.** Grading now
reads the actual obligation strength of what you wrote (RFC-2119 modal force: MUST/SHALL/REQUIRED,
either polarity) instead of a three-word negative-marker list, and the deny channel that was supposed to
block `must`-grade writes now actually does. Measured directly against this project's own corpus before
shipping: **404 of 420 directives previously graded `advisory` move to `must`.** The moment you upgrade,
roughly 400 rules that were previously informational-only become enforced, blocking write-time rules —
against a corpus you didn't just change. This isn't a bug to route around; it's existing rules finally
enforcing what they say. If a refusal looks wrong, reword the directive rather than bypassing the gate.

**Recommended upgrade steps:**

```text
1. Update launcher:  curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/v0.7.1/install.sh | bash
2. Re-run upgrade:   /edikt:upgrade
                      (auto-runs sentinel migration, schema v1→v2 upgrade, and compile)
3. Review the result: /edikt:doctor
4. Spot-check a few directives your team wrote as bare "MUST" statements — expect them to
   now grade `must` and enforce at write time, correctly, if that's what you intended.
5. Commit:           git add .claude/ .edikt/ docs/ && git commit -m "chore: upgrade edikt to 0.7.1"
```

Full detail — the exact refusal messages, the sidecar schema v1→v2 change, and recovery paths — is in
[Upgrading to v0.7.0](/guides/v0.7.0-upgrade).

---

## Payload version management

edikt's payload (templates, commands, hooks, agents) is versioned independently of the launcher binary. You can upgrade, roll back, pin a version, or run multiple versions side by side.

### Upgrade

Fetch the latest payload and activate it:

```bash
edikt upgrade
```

If you're on Homebrew, `brew upgrade edikt` updates the launcher. `edikt upgrade` updates the payload. They're independent.

### Rollback

Revert to the previous payload version:

```bash
edikt rollback
```

**Rollback is payload-only.** `edikt rollback` flips `~/.edikt/current` back to the previous generation. It does not undo migrations (M1-M5). Migrations are permanent structural changes to your `~/.edikt/` layout and `~/.claude/` command files. If a migration caused a problem, contact support — don't expect rollback to fix it.

> **Note.** On a fresh install, `install.sh` makes a one-time backup of your `~/.claude/settings.json` at `~/.edikt/backup/pre-v0.6.0-<timestamp>/` before applying edikt's managed permissions block. There is no dedicated host-file rollback command — restore that file manually if you ever need to.

### Pinning a version

Stay on a specific version:

```bash
edikt use v0.7.1        # activate v0.7.1 immediately
edikt upgrade --pin v0.7.1  # fetch v0.7.1 and pin it
```

When pinned, `edikt upgrade` is a no-op until you clear the pin:

```bash
edikt upgrade --pin clear   # remove pin, next upgrade proceeds
```

### Listing installed versions

```bash
edikt list
```

Output:

```
  v0.4.3
  v0.7.0
* v0.7.1   (current)
```

### Pruning old versions

Keep only the N most recent versions (default: 3):

```bash
edikt prune          # remove all but 3 most recent + current
edikt prune --keep 5 # keep 5
edikt prune --dry-run  # preview what would be removed
```

`edikt prune` never removes the current version or any pinned version.

### What `edikt upgrade` does

1. Fetches the latest release tarball from GitHub
2. Verifies the SHA256 checksum against `SHA256SUMS`
3. Extracts to `~/.edikt/versions/<tag>/`
4. Runs any pending migrations (M1-M5) against your current install
5. Flips `~/.edikt/current` to the new version
6. Updates `~/.edikt/lock.yaml`

If verification fails, the new version is not activated. Your current version is untouched.

### Project-mode installs

If edikt is installed per-project (`.edikt/` inside the repo), `edikt upgrade` run from the project directory upgrades that project's payload independently of the global install.

```bash
cd my-project
edikt upgrade    # upgrades the project-local payload only
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
