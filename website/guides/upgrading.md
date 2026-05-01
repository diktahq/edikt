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
curl -fsSL https://github.com/diktahq/edikt/releases/download/v0.5.0/install.sh | bash
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

**Compile migration** — running `/edikt:gov:compile` automatically migrates from the old flat format to topic-grouped files:

```text
📦 Migrated from flat governance.md to topic-grouped rule files.
   Old format: 1 file, 13 directives
   New format: 3 topic files + index
```

**Directive sentinels** — for full-fidelity compilation, run `/edikt:gov:review` to generate directive sentinel blocks in your ADRs and invariants. Without sentinels, compile falls back to extraction (same quality as v0.1.x).

**Agent governance** — all agent templates now have `maxTurns`, `disallowedTools`, and `effort`. The new `evaluator` agent is installed automatically.

**New hooks** — 4 new hook events (StopFailure, TaskCreated, CwdChanged, FileChanged) and conditional `if` field on 2 existing hooks.

**Installer safety** — reinstalling now backs up existing files. Use `--dry-run` to preview changes.

**Recommended upgrade steps:**

```text
1. Update global:    curl -fsSL https://github.com/diktahq/edikt/releases/download/v0.5.0/install.sh | bash
2. Upgrade project:  /edikt:upgrade
3. Generate sentinels: /edikt:gov:review
4. Recompile:        /edikt:gov:compile
5. Commit:           git add .claude/ .edikt/ docs/ && git commit -m "chore: upgrade edikt to 0.2.0"
```

---

## Upgrading to v0.3.0

v0.3.0 introduces the three-list directive schema, Invariant Records, and compile improvements. Here's what to expect:

**Three-list schema (ADR-008)** — sentinel blocks now carry `directives:`, `manual_directives:`, and `suppressed_directives:`. Existing v0.2.x blocks with only `directives:` continue to work — the missing lists are treated as empty. Run `/edikt:invariant:compile` and `/edikt:adr:compile` to upgrade blocks to the new schema.

**Reminders and verification checklist** — compile now generates `reminders:` and `verification:` lists inside sentinel blocks. These aggregate into `## Reminders` and `## Verification Checklist` sections in governance.md. Recompile to generate them.

**"No exceptions." reinforcement** — invariant directives derived from absolute-language Statements get "No exceptions." appended. This is automatic on recompile.

**New command: `/edikt:gov:score`** — scores your compiled governance for LLM compliance. Run after recompiling to check directive quality.

**Project templates** — v0.3.0 projects can override ADR/invariant/guideline templates in `.edikt/templates/`. If your project doesn't have them, edikt uses built-in defaults. No action needed.

**Recommended upgrade steps:**

```text
1. Update global:       curl -fsSL https://github.com/diktahq/edikt/releases/download/v0.5.0/install.sh | bash
2. Upgrade project:     /edikt:upgrade
3. Compile invariants:  /edikt:invariant:compile
4. Compile ADRs:        /edikt:adr:compile
5. Compile governance:  /edikt:gov:compile
6. Score quality:       /edikt:gov:score
7. Commit:              git add .claude/ .edikt/ docs/ && git commit -m "chore: upgrade edikt to 0.3.0"
```

---

## Upgrading to v0.5.0

v0.5.0 ships two spec bundles. Here's what to expect.

**Stability (SPEC-004) — hook JSON protocol**

Hook output migrated from plaintext to JSON. The migration is transparent to most users — user-visible message content is preserved byte-for-byte inside `{"systemMessage": ...}` wrappers. If you consume raw hook stdout in custom tooling, you'll need to unwrap the JSON. See [ADR-014](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-014-hook-json-wrapping-in-stability-scope.md).

**Stability (SPEC-004) — versioned layout migration**

v0.5.0 changes `~/.edikt/` from a flat layout to a versioned one. This is a one-time migration. See [Migrating from v0.4](migrating-from-v0.4.md) for the step-by-step.

**Directive hardening (SPEC-005) — backward-compatible sentinel fields**

Existing ADRs parse without changes. Two new optional fields (`canonical_phrases`, `behavioral_signal`) default to `[]`/`{}` when absent. FR-003a (compile warns on multi-sentence directives without `canonical_phrases`) is warn-only in v0.5.0 — non-blocking. Run `/edikt:adr:review --backfill` to retrofit existing ADRs at your own pace.

**Tier-2 benchmark — opt-in install**

`/edikt:gov:benchmark` is not bundled in `install.sh`. To add it:

```bash
./bin/edikt install benchmark
```

This installs the Python helper into `~/.edikt/venv/gov-benchmark/`. No action required if you don't want the benchmark.

**Recommended upgrade steps for v0.5.0:**

```text
1. Update launcher:  brew upgrade edikt  OR  curl -fsSL ... | bash
2. Migrate layout:   edikt migrate --yes
3. Upgrade project:  /edikt:upgrade  (in Claude Code)
4. Recompile:        /edikt:gov:compile
5. Optional:         ./bin/edikt install benchmark
6. Optional:         /edikt:adr:review --backfill   # populate canonical_phrases
7. Commit:           git add .claude/ .edikt/ docs/ && git commit -m "chore: upgrade edikt to 0.5.0"
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
