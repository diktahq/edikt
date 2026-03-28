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
INSTALLED_VERSION=$(cat ~/.edikt/VERSION 2>/dev/null | tr -d '[:space:]')
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

#### 2d. Config check

Read `.edikt/config.yaml`. Check for missing keys that were added in newer versions:

- `artifacts:` block missing → outdated (added in v0.1.1)
- `artifacts.database.default_type` missing → outdated
- `artifacts.fixtures.format` missing → outdated

Note each missing key with a description:
- "`artifacts:` block missing — enables database-type-aware spec-artifacts"

Do NOT flag keys that exist but have unexpected values — those may be intentional user customizations.

#### 2e. Rule packs check

If `.claude/rules/` does not exist or contains no `.md` files → mark rule packs as "nothing installed, skip" (not outdated).

Otherwise, same logic as `/edikt:rules-update`:
- Compare `version:` frontmatter in installed vs template
- Only flag as outdated if installed version < template version
- Skip files without `<!-- edikt:generated -->` marker (manually edited)
- Skip files not in the registry (custom rules)

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

### 6. Post-Upgrade

After applying:

1. Always update `edikt_version` in `.edikt/config.yaml` to the installed version — even if no other changes were applied:
   - If a `edikt_version:` line exists, replace it
   - If it doesn't exist (project predates versioning), add it as the first non-comment line after any leading `#` comment block at the top of the file

2. Check if linter configs exist and linter rules are outdated (template mtime > linter rule mtime):
   ```
   Linter configs found. Run /edikt:sync to regenerate linter rules.
   ```

3. Output results:

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

WHAT'S NEW in {new}
─────────────────────────────────────────────────────
{content of the most recent changelog section from ~/.edikt/CHANGELOG.md}
─────────────────────────────────────────────────────
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

WHAT'S NEW in {new}
─────────────────────────────────────────────────────
{content of the most recent changelog section from ~/.edikt/CHANGELOG.md}
─────────────────────────────────────────────────────
```
