# Plan: Upgrade Version Check

## Overview
**Task:** Add a remote version check to `/edikt:upgrade` that compares the installed version against the latest release on GitHub. If outdated, tell the user and show the install command. Skippable with `--offline`.
**Total Phases:** 2
**Estimated Cost:** $0.02
**Created:** 2026-03-28

## Progress

| Phase | Status | Updated |
|-------|--------|---------|
| 1     | -      | -       |
| 2     | -      | -       |

**IMPORTANT:** Update this table as phases complete. This table is the persistent state that survives context compaction.

## Model Assignment
| Phase | Task | Model | Reasoning | Est. Cost |
|-------|------|-------|-----------|-----------|
| 1 | Add version check to upgrade.md | sonnet | Logic changes to command prompt, multiple cases | $0.01 |
| 2 | Tests + docs + changelog | haiku | Assertions, website update, changelog entry | $0.01 |

## Execution Strategy
| Phase | Depends On | Parallel With |
|-------|-----------|---------------|
| 1     | None      | -             |
| 2     | 1         | -             |

---

## Phase 1: Add Remote Version Check to upgrade.md

**Objective:** Add a step 0 to `/edikt:upgrade` that checks the latest version from GitHub before comparing installed vs project.
**Model:** `sonnet`
**Max Iterations:** 2
**Completion Promise:** `VERSION CHECK DONE`
**Dependencies:** None

**Prompt:**

Read `commands/upgrade.md` in full.

Add a new **Step 0: Check for updates** before the current Step 1. This step runs first and may short-circuit the rest of the command.

**Step 0 logic:**

1. Check if `--offline` is in `$ARGUMENTS`. If present, skip this step entirely and proceed to Step 1. Strip `--offline` from arguments before passing to the rest of the command.

2. Fetch the latest version from GitHub:
```bash
LATEST_VERSION=$(curl -fsSL --max-time 5 "https://raw.githubusercontent.com/diktahq/edikt/main/VERSION" 2>/dev/null | tr -d '[:space:]')
```

3. Read the installed version:
```bash
INSTALLED_VERSION=$(cat ~/.edikt/VERSION 2>/dev/null | tr -d '[:space:]')
```

4. Three outcomes:

**a) Fetch failed (no network, timeout):**
```
⚠ Could not check for updates (network unavailable). Proceeding with installed version.
  To skip this check: /edikt:upgrade --offline
```
Proceed to Step 1 normally.

**b) Latest version matches installed:**
Proceed to Step 1 silently — no output needed.

**c) Latest version is newer than installed:**
```
📦 edikt {LATEST_VERSION} is available (you have {INSTALLED_VERSION}).

  Update now:
    curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash

  Then re-run /edikt:upgrade to apply changes to this project.
  To skip this check: /edikt:upgrade --offline
```
**Stop here.** Do not proceed to Step 1 — the user needs to update global templates first, otherwise the project upgrade would be based on stale templates.

Also update the command frontmatter `argument-hint` to mention `--offline`.

When complete, output: `VERSION CHECK DONE`

---

## Phase 2: Tests + Docs + Changelog

**Objective:** Add test assertions, update website, and add changelog entry.
**Model:** `haiku`
**Max Iterations:** 2
**Completion Promise:** `DOCS DONE`
**Dependencies:** Phase 1

**Prompt:**

**Tests** — add to `test/test-quality.sh` after the existing plan assertions:

```bash
# Upgrade command has remote version check
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "Check for updates" "upgrade has remote version check"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "offline" "upgrade supports --offline flag"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "raw.githubusercontent.com" "upgrade fetches from GitHub"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "max-time" "upgrade has curl timeout"
```

**Website** — update `website/commands/upgrade.md` (read it first):
- Add `--offline` to the arguments table
- Add a "Version check" section explaining the remote check behavior
- Show the three outcomes (no network, up to date, update available)

**Website upgrading guide** — update `website/guides/upgrading.md`:
- In Step 2, mention that `/edikt:upgrade` now checks for updates automatically and tells you if the installer needs to be re-run first

**Changelog** — add to `CHANGELOG.md` under a new `## v0.1.4 (unreleased)` section:
```
### Upgrade version check

`/edikt:upgrade` now checks for newer edikt releases before upgrading the project. If a newer version exists, it shows the install command and stops — ensuring project upgrades always use the latest templates. Skip with `--offline` for air-gapped environments.
```

Run `./test/run.sh` and verify all tests pass.

When complete, output: `DOCS DONE`
