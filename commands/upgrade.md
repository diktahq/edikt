---
name: edikt:upgrade
description: "Upgrade edikt in this project — hooks, agents, and rules to the latest installed version"
effort: normal
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Bash
  - AskUserQuestion
---

# edikt:upgrade

Upgrade edikt in this project to match the currently installed edikt version. Updates hooks, agent templates, and rule packs — never overwrites customizations without asking.

## Arguments

- `$ARGUMENTS` — Optional. `--offline` skips the remote version check. `--no-review` is not applicable to this command.

## Instructions

### 0. Check for Updates

If `--offline` is in `$ARGUMENTS`, skip this step entirely and proceed to Step 1.

Otherwise, check if a newer edikt version is available:

```bash
LATEST_VERSION=$(curl -fsSL --max-time 5 "https://raw.githubusercontent.com/diktahq/edikt/main/VERSION" 2>/dev/null | tr -d '[:space:]')
INSTALLED_VERSION=$(cat .edikt/VERSION 2>/dev/null || cat ~/.edikt/VERSION 2>/dev/null | tr -d '[:space:]')
```

Three outcomes:

**Fetch failed** (no network, timeout, empty response):
```
⚠ Could not check for updates (network unavailable). Proceeding with installed version.
  To skip this check: /edikt:upgrade --offline
```
Proceed to Step 1 normally.

**Latest version matches installed** — proceed to Step 1 silently.

**Latest version is newer than installed:**
```
📦 edikt {LATEST_VERSION} is available (you have {INSTALLED_VERSION}).

  Update now:
    curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash

  Then re-run /edikt:upgrade to apply changes to this project.
  To skip this check: /edikt:upgrade --offline
```
Stop here — do not proceed to Step 1. The user needs to update global templates first, otherwise the project upgrade would use stale templates.

### 1. Check Prerequisites

Read `.edikt/config.yaml`. If not found:
```
No edikt config found. Run /edikt:init to set up this project.
```

Check that edikt templates exist at `~/.edikt/templates/`. If not:
```
edikt templates not found. Re-install edikt:
  curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
```

Use the Bash tool to read both versions — do NOT infer or guess them:
```bash
cat ~/.edikt/VERSION 2>/dev/null | tr -d '[:space:]'
grep '^edikt_version:' .edikt/config.yaml | awk '{print $2}' | tr -d '"'
```

Use the actual output of these commands as INSTALLED_VERSION and PROJECT_VERSION.

Show at the top of the output:
```
Installed edikt: {INSTALLED_VERSION}
Project edikt:   {PROJECT_VERSION}
```

If INSTALLED_VERSION == PROJECT_VERSION AND there are no changes detected in step 2 AND `edikt_version` is already set in `.edikt/config.yaml`, show:
```
✅ Already up to date (edikt {INSTALLED_VERSION}) — nothing to upgrade.
```
and stop.

If INSTALLED_VERSION != PROJECT_VERSION, always proceed with the upgrade — the version difference alone is reason enough.

If `edikt_version` is missing from `.edikt/config.yaml` (project predates versioning), always proceed — adding the version is itself an upgrade.

### 2. Detect What Needs Upgrading

Run all checks in parallel and collect findings.

#### 2a. Hooks check

Read `.claude/settings.json`. Read `~/.edikt/templates/settings.json.tmpl`.

For each hook type, check two things: (1) is the content correct, and (2) is it using the modern `.sh` script reference format?

**Migration check — inline bash vs. script references:**
If any `type: command` hook has its logic inline (a long bash string) rather than referencing `$HOME/.edikt/hooks/*.sh`, it is outdated regardless of content. Note: "using inline bash — migrate to script reference".

**Content checks:**
- `SessionStart`: command should reference `$HOME/.edikt/hooks/session-start.sh` — if not → outdated
- `PreToolUse`: must be present with `Write|Edit` matcher — if missing → missing
- `PostToolUse`: must be present with `Write|Edit` matcher — if missing → missing
- `Stop`: must be type:command referencing `$HOME/.edikt/hooks/stop-hook.sh` — if type:prompt or inline → outdated
- `PreCompact`: command should reference `$HOME/.edikt/hooks/pre-compact.sh` — if not → outdated
- `UserPromptSubmit`: must be present — if missing → missing (v4: injects active plan phase)
- `PostCompact`: must be present — if missing → missing (v4: re-injects context after compaction)
- `SubagentStop`: must be present — if missing → missing (v4: logs agent activity + quality gates)
- `InstructionsLoaded`: must be present — if missing → missing (v4: logs rule pack loading)

For each outdated or missing hook, note what changed in plain English:
- "SessionStart: inline bash → migrate to `$HOME/.edikt/hooks/session-start.sh`"
- "PostToolUse: missing (auto-formats files after edits)"
- "UserPromptSubmit: missing (v4 — injects active plan phase into every prompt)"
- "PostCompact: missing (v4 — re-injects plan + invariants after compaction)"
- "SubagentStop: missing (v4 — logs agent activity, quality gates)"
- "InstructionsLoaded: missing (v4 — logs which rule packs load)"
- "Stop: outdated format (may cause JSON validation error) → migrate to `$HOME/.edikt/hooks/stop-hook.sh`"
- "PreCompact: inline bash → migrate to `$HOME/.edikt/hooks/pre-compact.sh`"

#### 2b. CLAUDE.md sentinel check

Read `CLAUDE.md`. Check which sentinel format is in use:

```bash
grep -qF '[edikt:start]' CLAUDE.md 2>/dev/null && echo "new"
grep -qF '<!-- edikt:start' CLAUDE.md 2>/dev/null && echo "old"
```

- Old HTML comment sentinels found (`<!-- edikt:start`) → outdated
- New visible sentinels found (`[edikt:start]`) → up to date
- No edikt block found → skip (nothing to migrate)

Note when outdated: "CLAUDE.md using old HTML sentinels — Claude Code v2.1.72+ hides these, preventing Claude from seeing section boundaries"

#### 2c. Agent check


List files in `.claude/agents/`. For each, check if a matching template exists in `~/.edikt/templates/agents/`.

**Skip customized agents.** An agent is customized if:
1. It contains `<!-- edikt:custom -->` anywhere in the file, OR
2. It is listed in `.edikt/config.yaml` under `agents.custom`

```yaml
# .edikt/config.yaml
agents:
  custom:
    - dba       # skip on upgrade — team has customized this agent
    - my-team-reviewer    # not from edikt templates
```

For each agent that is NOT customized and has a edikt template, compare content hashes — NOT modification times:
```bash
template_hash=$(md5 -q ~/.edikt/templates/agents/{slug}.md 2>/dev/null || md5sum ~/.edikt/templates/agents/{slug}.md 2>/dev/null | awk '{print $1}')
installed_hash=$(md5 -q .claude/agents/{slug}.md 2>/dev/null || md5sum .claude/agents/{slug}.md 2>/dev/null | awk '{print $1}')
```

- If customized → skip (note as "custom — skipped")
- If hashes differ → outdated
- If hashes match → up to date

Do NOT touch agents that have no matching template (user-created agents) or that are marked as custom.

**Detect new agents.** List files in `~/.edikt/templates/agents/`. For each template, check if a matching file exists in `.claude/agents/`. If a template has no installed counterpart, it's a new agent added in this version.

New agents are classified as **core** or **optional**:

- **Core agents** are installed automatically — they're required for edikt's governance mechanisms to work. The evaluator agents (`evaluator.md`, `evaluator-headless.md`) are core: the plan harness and quality gates depend on them.
- **Optional agents** (all other specialist agents) are offered to the user with a description of what they do. The user chooses which to install.

```
New agents in v{version}:

  Installed automatically (core):
  ✓ evaluator-headless.md — headless phase-end evaluator (required by plan harness)

  Available (choose which to add):
  [1] gtm.md — go-to-market strategy review
  [2] mobile.md — mobile platform specialist
  [a] Install all    [s] Skip all    [1,2] Install selected
```

Core agents that the user doesn't want can be disabled after install:
- Delete `.claude/agents/{slug}.md` to remove
- Add to `agents.custom` in `.edikt/config.yaml` to skip on future upgrades

If the user declines an optional agent, add it to `agents.custom` in config so future upgrades don't ask again.

#### 2d. Config check

Read `.edikt/config.yaml`. Check for missing keys that were added in newer versions:

- `artifacts:` block missing → outdated (added in v0.1.1)
- `artifacts.database.default_type` missing → outdated
- `artifacts.fixtures.format` missing → outdated

Note each missing key with a description:
- "`artifacts:` block missing — enables database-type-aware spec-artifacts"

Do NOT flag keys that exist but have unexpected values — those may be intentional user customizations.

#### 2e-bis. Project templates check (v0.3.0+)

Check whether the project has the three per-artifact project templates that v0.3.0 requires for new artifact creation via the lookup chain (see ADR-005, ADR-009, and commands/<artifact>/new.md Section 1a).

```bash
HAS_ADR_TEMPLATE=$([ -f .edikt/templates/adr.md ] && echo "yes" || echo "no")
HAS_INVARIANT_TEMPLATE=$([ -f .edikt/templates/invariant.md ] && echo "yes" || echo "no")
HAS_GUIDELINE_TEMPLATE=$([ -f .edikt/templates/guideline.md ] && echo "yes" || echo "no")
```

**Classify the project:**

- **All three present** → mark project templates as "up to date, skip". Report nothing in the upgrade summary.
- **At least one missing AND `edikt_version >= 0.3.0`** → mark as "templates partially configured — /edikt:init will complete setup". Note in the summary:
  ```
  ⬆  Project templates — {n}/3 missing
     Missing: {list of missing templates}
     Fix: run /edikt:init --reset-templates to complete setup
  ```
- **All three missing AND `edikt_version < 0.3.0`** (v0.2.x legacy upgrading to v0.3.0) → this is the **grandfather flow**. Note in the summary:
  ```
  ⬆  Project templates — v0.3.0 introduces per-artifact project templates
     This project is on v{old_version} — templates have never been configured.
     v0.3.0+ requires explicit templates for /edikt:adr:new, /edikt:invariant:new,
     and /edikt:guideline:new. edikt doesn't ship a default — your project owns it.

     After upgrade: run /edikt:init to set up project templates interactively.
     You'll pick Adapt (generate from existing artifacts), Start fresh (pick a
     reference template), or Write my own for each artifact type.
  ```
- **All three missing AND `edikt_version >= 0.3.0`** (broken state — v0.3.0+ project with no templates) → note:
  ```
  ⬆  Project templates — broken state detected
     Project is on v{version} but no templates are configured.
     Fix: run /edikt:init --reset-templates immediately.
     Until then, /edikt:<artifact>:new commands will refuse per ADR-005.
  ```

**Never overwrite existing templates.** If `.edikt/templates/adr.md`, `.edikt/templates/invariant.md`, or `.edikt/templates/guideline.md` already exist, `/edikt:upgrade` must NOT touch them. They are user-owned content committed to the project's git. The only way to regenerate them is `/edikt:init --reset-templates`, which the user invokes explicitly.

**Never auto-run init.** Even when templates are missing, `/edikt:upgrade` does NOT invoke `/edikt:init` on the user's behalf. It reports the state in the summary and leaves the user in control. Init is interactive; upgrade should not drag the user through another interactive flow without consent.

**Detect the three-list schema migration opportunity** (informational only):

If any artifact files exist under `{paths.decisions}`, `{paths.invariants}`, or `{paths.guidelines}` that have a legacy `content_hash:` field in their directive sentinel block (v0.2.x schema), note:
```
ℹ Directive sentinel schema — {n} artifacts are on the v0.2.x schema
   These will migrate to the v0.3.0 three-list schema on their next
   /edikt:<artifact>:compile run. No action required — backward
   compatibility works seamlessly. Informational only.
```

This is NOT a blocking issue or a required upgrade step. The three-list schema is additive and `gov:compile` reads both the old and new formats. Users can migrate at their own pace by running `/edikt:<artifact>:compile --regenerate` when they want to.

#### 2e. Rule packs check

If `.claude/rules/` does not exist or contains no `.md` files → mark rule packs as "nothing installed, skip" (not outdated).

Otherwise, same logic as `/edikt:rules-update`:
- Compare `version:` frontmatter in installed vs template
- Only flag as outdated if installed version < template version
- Skip files without `<!-- edikt:generated -->` marker (manually edited)
- Skip files not in the registry (custom rules)
- **Hash comparison:** For files with `edikt:generated` marker, compute content hash and compare against the template. If hashes differ (content was edited but marker kept), flag as modified:
  ```
  ⚠ .claude/rules/go.md has edikt:generated marker but content differs from template.

    [1] Overwrite — replace with latest template
    [2] Keep mine — remove the marker, edikt won't touch this file again
    [3] Show diff — see what changed before deciding
  ```
  If user picks [2], remove the `<!-- edikt:generated -->` marker and report:
  ```
  ✅ .claude/rules/go.md is now yours. edikt will never overwrite it again.
  ```

### 3. Show Upgrade Summary

Show what will change in this project before touching anything:

```
EDIKT UPGRADE
─────────────────────────────────────────────────────
Hooks (.claude/settings.json)
  ⬆  SessionStart   — inline bash → script reference
  ⬆  PostToolUse    — missing, will add auto-format hook
  ⬆  Stop           — fix "Prompt hook condition was not met" error (ok:false → ok:true always)
  ⬆  PreCompact     — inline bash → script reference

Agents (.claude/agents/)
  ⬆  dba.md   — template updated
  ⬆  security.md  — template updated
  +  evaluator-headless.md — new in v0.4.0
  ✓  architect.md  — up to date

Rule packs (.claude/rules/)
  ⬆  go.md          1.0 → 1.2
  ⬆  code-quality.md 1.0 → 1.1
  ✓  testing.md      — up to date
  —  my-custom.md    — custom, skipped
  —  security.md     — manually edited, skipped

Config (.edikt/config.yaml)
  ⬆  artifacts: block missing — enables database-type-aware spec-artifacts

CLAUDE.md
  ⬆  old HTML sentinels → visible markers (Claude Code v2.1.72+ hides HTML comments)

─────────────────────────────────────────────────────
4 hook changes, 2 agents, 2 rule packs, 1 config addition, 1 CLAUDE.md migration
```

If no rule packs are installed (`.claude/rules/` is missing or empty), show:
```
Rule packs (.claude/rules/)
  —  no rule packs installed
```
Do NOT show any `⬆` icon for rules in this case.

If everything is already up to date:
```
✅ Already up to date — nothing to upgrade.
```

### 4. Confirm

Ask the user:
```
Apply these upgrades? (y/n/select)
  y      — apply all
  n      — cancel
  select — choose which sections to apply (hooks / agents / rules / config / claude.md)
```

Wait for response. If `select`, ask separately for each section.

If cancelled:
```
Upgrade cancelled — no changes made.
```

### 5. Apply Upgrades

#### Hooks

Read the current `.claude/settings.json`. Read the template.

For each outdated hook, replace ONLY that hook's entry — do not touch other hooks or non-hook settings (like `permissions`). Merge carefully:

```python
# Pseudocode
settings = read_json('.claude/settings.json')
template_hooks = read_json('~/.edikt/templates/settings.json.tmpl')['hooks']

for hook_type in ['SessionStart', 'PreToolUse', 'PostToolUse', 'Stop', 'PreCompact']:
    if hook_type needs upgrade:
        settings['hooks'][hook_type] = template_hooks[hook_type]

write_json('.claude/settings.json', settings)
```

**Never remove** hooks that exist in `settings.json` but not in the template (the user may have added their own).

#### Agents

For each outdated agent:
1. Read the installed file
2. Read the template
3. Replace the installed file with the template content

For each new **core** agent (evaluator, evaluator-headless):
1. Copy the template to `.claude/agents/{slug}.md`
2. Report: `✓ Installed evaluator-headless.md — core (required by plan harness)`

For each new **optional** agent the user accepted:
1. Copy the template to `.claude/agents/{slug}.md`
2. Report: `✓ Installed gtm.md — go-to-market strategy review`

For each new optional agent the user declined:
1. Do NOT install
2. Add slug to `agents.custom` in `.edikt/config.yaml` so future upgrades don't ask again
3. Report: `— Skipped gtm.md (added to agents.custom)`

Skip agents without a matching template. Skip user-created agents (no matching template slug).

#### CLAUDE.md sentinels

If old HTML comment sentinels are detected, migrate them in place using Edit (not Write):

- Replace `<!-- edikt:start — managed by edikt, do not edit manually -->` → `[edikt:start]: # managed by edikt — do not edit this block manually`
- Replace `<!-- edikt:start -->` (short form, if present) → `[edikt:start]: # managed by edikt — do not edit this block manually`
- Replace `<!-- edikt:end -->` → `[edikt:end]: #`

Leave all content between the sentinels untouched.

#### Config

For each missing config key, append the block to `.edikt/config.yaml`. Preserve all existing content — only add what's missing.

If `artifacts:` block is missing, append:

```yaml

artifacts:
  database:
    # Default database type for artifact generation.
    # spec-artifacts checks spec frontmatter first, then this value, then keyword-scans the spec.
    # Set by edikt:init from code signals. Change only if detection was wrong.
    # Values: sql | document | key-value | mixed | auto
    # auto = detect from spec each time (greenfield or genuinely undecided)
    default_type: auto

  fixtures:
    # Fixture format. yaml is portable — transform to your stack at implementation time.
    # Values: yaml | json | sql
    format: yaml
```

Note: the `sql.migrations.tool` sub-key is only written by `/edikt:init` when a SQL database is detected. Do not add it during upgrade — `auto` is the correct default for unknown stacks.

#### Rule packs

Same as `/edikt:rules-update` logic — replace outdated packs, skip manually edited and custom ones.

#### Compile schema check (ADR-007)

Check if the project's generated governance is stale vs the current compile schema.

1. Read the constant `COMPILE_SCHEMA_VERSION` from `~/.edikt/templates/commands/gov/compile.md` (or `commands/gov/compile.md` in the installed templates).
2. Read `.claude/rules/governance.md` (if it exists) and extract `compile_schema_version` from its YAML frontmatter.
3. Compare:
   - **Missing field** (legacy v0.1.x output): note `governance.md uses legacy version stamp — run /edikt:gov:compile to regenerate with schema v{N}`
   - **Lower than current**: note `governance.md compiled with schema v{old} (current: v{new}) — run /edikt:gov:compile to regenerate`
   - **Equal**: no note
   - **Higher than current**: note `governance.md compiled with schema v{n}, but this edikt only supports v{current}. Upgrade edikt globally first.`

Do NOT auto-run `/edikt:gov:compile`. Surface the recommendation in the upgrade summary and let the user decide. Compile is potentially expensive and may have contradictions that need review.

**Important**: Never enforce `compiled_by` or `compiled_at` equality with the current edikt version. Those fields are informational HTML comments only — they tell humans when/who produced the file, but do not drive any decision.

#### Project templates (v0.3.0+, ADR-005 + ADR-009)

**Never overwrite project templates under `.edikt/templates/`.** This is a hard contract from ADR-005:

- Existing `.edikt/templates/adr.md`, `.edikt/templates/invariant.md`, or `.edikt/templates/guideline.md` MUST NOT be touched by `/edikt:upgrade`. They are user-owned, team-shared, committed to git. The only way to modify them is direct user edit or `/edikt:init --reset-templates` (which the user invokes explicitly).
- If any of the three exists, skip it in this step. Do NOT even verify its contents — trust the user.

**When templates are missing**, the behavior depends on the grandfather state detected in Step 2e-bis:

- **Grandfather flow** (`edikt_version < 0.3.0` → upgrading to v0.3.0+): print a clear migration notice:
  ```
  📋 v0.3.0 introduces per-artifact project templates

  v0.3.0+ requires .edikt/templates/adr.md, .edikt/templates/invariant.md,
  and .edikt/templates/guideline.md for /edikt:<artifact>:new to work.
  Your project is being upgraded from v{old} and doesn't have them yet.

  This upgrade does NOT create templates automatically. Templates are a
  choice your team makes: adapt from existing artifacts, pick a reference,
  or write your own.

  Next step after upgrade:
    /edikt:init    (interactive — pick Adapt / Start fresh / Write my own
                    for each artifact type)

  Until you run init, /edikt:<artifact>:new will continue to use the
  legacy inline fallback template with a one-time warning per invocation
  (v0.2.x behavior preserved). You can migrate at your own pace.
  ```

- **Broken state** (`edikt_version >= 0.3.0` but templates missing — shouldn't happen in normal workflows but may occur if the user deleted `.edikt/templates/*`): print an error-grade notice:
  ```
  ⚠ Project templates are missing but project is on v{version}

  edikt v0.3.0+ requires project templates for new artifact creation.
  This is a broken state — until you fix it, /edikt:<artifact>:new
  commands will HARD REFUSE with an error message.

  Fix: run /edikt:init --reset-templates immediately.
  ```

- **Partially configured** (`edikt_version >= 0.3.0`, some templates present, some missing): print:
  ```
  ⚠ Some project templates are missing

  Missing: {list of missing templates}
  Fix: run /edikt:init --reset-templates to complete the setup.

  The existing templates ({list of existing}) are preserved — init
  only regenerates the missing ones.
  ```

**Bump `edikt_version` in config** after the upgrade completes successfully, so subsequent `<artifact>:new` invocations can distinguish "recently upgraded, templates not yet set up" from "legacy project on v0.2.x". Do this as the final step of the upgrade, not before — if the upgrade fails midway, the version stays at the old value so the user can retry.

**Do not run `/edikt:init` automatically.** Always leave the user in control. Upgrade reports the state; init is the next action the user takes when they're ready.

#### Config key migration: `paths.soul` → `paths.project-context` (v0.4.0)

Check if `.edikt/config.yaml` contains `soul:` under `paths:`. If found, rename the key to `project-context:` — the value stays the same.

```
ℹ Config migration: paths.soul → paths.project-context

The config key `paths.soul` has been renamed to `paths.project-context`
in v0.4.0. Your config has been updated automatically.

Old: soul: {value}
New: project-context: {value}
```

This is a safe auto-migration — the key name changes, the value and behavior are identical. Commands that read this config check for both `project-context` and `soul` (fallback) so older configs continue to work even without the migration.

#### Directive sentinel schema migration (v0.2.x → v0.3.0, ADR-008)

Similar to the project template handling: **never auto-migrate directive sentinel blocks.** Files with the legacy `content_hash:` field continue to work via gov:compile's backward compatibility path. The migration to the v0.3.0 three-list schema happens on the next `/edikt:<artifact>:compile` run for that specific file.

If the upgrade summary noted legacy directive blocks in Step 2e-bis, reinforce in the apply step:
```
ℹ Directive sentinel schema migration

{n} artifact files still use the v0.2.x directive schema (single
content_hash field). They continue to work — /edikt:gov:compile reads
both old and new formats transparently.

To migrate at your own pace:
  /edikt:adr:compile --regenerate       (migrate all ADRs)
  /edikt:invariant:compile --regenerate (migrate all Invariant Records)
  /edikt:guideline:compile --regenerate (migrate all guidelines)

Or migrate individual files:
  /edikt:adr:compile ADR-NNN

There's no rush. Legacy and new schemas coexist indefinitely until
edikt v0.4.0 at the earliest (no deprecation planned in v0.3.x).
```

Do NOT run `--regenerate` automatically. Users should migrate at their own pace.

#### Command reference migration (v0.1.x → v0.2.x)

v0.2.0 renamed 15 flat commands into namespaces. Projects initialized with v0.1.x have hardcoded references to the old flat names in `CLAUDE.md` (the intent table inside the edikt-managed block) and in compiled rule packs. These references still resolve today via deprecated stubs, but they'll break in v0.4.0 when the stubs are removed.

Apply this migration table to the following targets:

| Old (v0.1.x)           | New (v0.2.x)             |
|------------------------|--------------------------|
| `/edikt:adr`           | `/edikt:adr:new`         |
| `/edikt:invariant`     | `/edikt:invariant:new`   |
| `/edikt:compile`       | `/edikt:gov:compile`     |
| `/edikt:review-governance` | `/edikt:gov:review`  |
| `/edikt:rules-update`  | `/edikt:gov:rules-update`|
| `/edikt:sync`          | `/edikt:gov:sync`        |
| `/edikt:prd`           | `/edikt:sdlc:prd`        |
| `/edikt:spec`          | `/edikt:sdlc:spec`       |
| `/edikt:spec-artifacts`| `/edikt:sdlc:artifacts`  |
| `/edikt:plan`          | `/edikt:sdlc:plan`       |
| `/edikt:review`        | `/edikt:sdlc:review`     |
| `/edikt:drift`         | `/edikt:sdlc:drift`      |
| `/edikt:audit`         | `/edikt:sdlc:audit`      |
| `/edikt:docs`          | `/edikt:docs:review`     |
| `/edikt:intake`        | `/edikt:docs:intake`     |

**Targets** (only files edikt owns — never touch user content):

1. **CLAUDE.md managed block** — the content strictly between `[edikt:start]: #` and `[edikt:end]: #` sentinels (or the old HTML sentinels if they weren't migrated yet). Leave everything outside the sentinels untouched.
2. **Generated rule packs** — any file under `.claude/rules/` or `.claude/rules/governance/` that contains the `edikt:generated` or `edikt:compiled` marker. Skip files without the marker (those are user-written).

**Safety rules:**

- **Idempotency is critical.** Do NOT replace `/edikt:adr` if it's already followed by `:` (e.g. `/edikt:adr:new` or `/edikt:adr:compile`). Use string contexts that make the match unambiguous: backtick-wrapped (`` `/edikt:adr` ``), end of line (`/edikt:adr\n`), or punctuation-delimited (`/edikt:adr,`, `/edikt:adr.`, `/edikt:adr)`).
- **Longest first is WRONG here.** The old commands have no overlap with each other, but they DO have overlap with the new names (`/edikt:adr` is a prefix of `/edikt:adr:new`). Always match the old pattern with a non-`:` terminator.
- **Use Edit with literal strings**, not Write. Preserve line endings, trailing whitespace, and all other content. For each replacement, include enough context (at least the backtick/paren/whitespace around the token) to avoid ambiguity.
- **Skip if already migrated.** Before making any edit, grep the file for any of the NEW command names in the table. If even one new name is present (e.g. `/edikt:adr:new` exists in the file), the file was already migrated on a previous upgrade — still run the full mapping pass to catch any stragglers, but don't report it as "migrated" unless actual changes were made.

**Process for each target file:**

1. Read the file.
2. Determine the edit scope (for CLAUDE.md: the managed block only; for rule packs: the whole file if it has the `edikt:generated` marker).
3. For each row in the mapping table, search the edit scope for old-name occurrences that are NOT followed by `:`. Track the count.
4. If any matches were found, apply them via Edit with full surrounding context for disambiguation.
5. Record: filename + count of replacements.

Report the results as part of the upgrade summary:

```
Command references:
  CLAUDE.md:                               7 replacements
  .claude/rules/governance.md:             3 replacements
  .claude/rules/governance/api-design.md:  0 (already current)
```

If a project has no v0.1.x references anywhere, report `Command references: ✓ up to date` instead of the per-file breakdown.

### 6. Post-Upgrade

After applying:

1. Always update `edikt_version` in `.edikt/config.yaml` to the installed version — even if no other changes were applied:
   - If a `edikt_version:` line exists, replace it
   - If it doesn't exist (project predates versioning), add it as the first non-comment line after any leading `#` comment block at the top of the file

2. Check if linter configs exist and linter rules are outdated (template mtime > linter rule mtime):
   ```
   Linter configs found. Run /edikt:sync to regenerate linter rules.
   ```

3. **Check project templates (v0.3.0+, ADR-005 + ADR-009)**: if any of `.edikt/templates/adr.md`, `.edikt/templates/invariant.md`, or `.edikt/templates/guideline.md` is missing, surface a prominent next-step prompt in the post-upgrade output:
   ```
   📋 Project templates — /edikt:init required

   v0.3.0+ requires per-artifact project templates. This project
   doesn't have all three yet:
     Missing: {list of missing templates}

   Next step: /edikt:init (interactive setup)
     You'll pick Adapt, Start fresh, or Write my own for each
     artifact type. Existing artifacts are not touched.

   Until templates are set up, /edikt:<artifact>:new will:
     - Use the legacy inline fallback + warning (for v0.2.x-era projects
       whose edikt_version is still < 0.3.0)
     - HARD REFUSE with an error pointing at /edikt:init (for projects
       whose edikt_version is now >= 0.3.0 after this upgrade)
   ```
   This notice fires AFTER the `edikt_version` bump in step 1, so the user sees it with the correct version context.

   **This is advisory only.** Do not auto-run `/edikt:init`. Leave the user in control.

4. Output results:

If only `edikt_version` was added (everything else was already current):
```
UPGRADE COMPLETE
─────────────────────────────────────────────────────
Version:     {old or "unset"} → {new}
Hooks:       ✓ up to date
Agents:      ✓ up to date
Rule packs:  ✓ up to date

Commit to record the version:
  git add .edikt/config.yaml && git commit -m "chore: set edikt_version to {new}"

Run /edikt:doctor to verify governance health.

{If docs/architecture/assumptions.md exists:}
💡 Model capabilities may have changed. Review docs/architecture/assumptions.md
   to re-test harness assumptions.

WHAT'S NEW in {new}
─────────────────────────────────────────────────────
{content of the most recent changelog section from ~/.edikt/CHANGELOG.md}
─────────────────────────────────────────────────────

Next: Run /edikt:doctor to verify governance health.
```

If changes were applied:
```
UPGRADE COMPLETE
─────────────────────────────────────────────────────
Version:     {old} → {new}
Hooks:       4 updated
Agents:      2 updated
Rule packs:  2 updated (1 skipped — manually edited)
Config:      1 addition (artifacts: block)
CLAUDE.md:   sentinels migrated to visible format

Commit these changes to share the upgrade with your team:
  git add .claude/ .edikt/config.yaml && git commit -m "chore: upgrade edikt to {new}"

Run /edikt:doctor to verify governance health.

{If docs/architecture/assumptions.md exists:}
💡 Model capabilities may have changed. Review docs/architecture/assumptions.md
   to re-test harness assumptions.

WHAT'S NEW in {new}
─────────────────────────────────────────────────────
{content of the most recent changelog section from ~/.edikt/CHANGELOG.md}
─────────────────────────────────────────────────────

Next: Run /edikt:doctor to verify governance health.
```
