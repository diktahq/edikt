---
name: upgrade-pin
description: "Update .edikt/config.yaml's edikt_version to match the currently active install"
effort: low
context: fork
allowed-tools:
  - Bash
  - Read
tier_2_dependency: edikt upgrade-pin
on_absent: refuse-and-direct-user
---

# edikt:upgrade-pin

Update this project's `.edikt/config.yaml` `edikt_version:` field to match the currently active edikt version. All other config content is preserved byte-for-byte.

Use this when you see the warning:

```
warn: this project pins edikt X.Y.Z but the active version is A.B.C
      Run `edikt upgrade-pin` inside the project to update the pin.
```

## When to run

- After installing a new edikt version that changes the launcher (e.g. v0.5.x → v0.6.x), if you've decided to adopt it in this project.
- After running `/edikt:upgrade` to align the project pin with what's actually loaded.
- When a teammate bumped the active version but the committed `.edikt/config.yaml` is stale.

## When NOT to run

- If you intentionally pin this project to an older version (e.g. you need v0.5.x's behavior and the active install is v0.6.x). In that case, run `edikt use <pinned-version>` instead to switch the active install down to match.

## Instructions

### 1. Confirm current state

```bash
edikt version          # what's active
cat .edikt/config.yaml | grep edikt_version  # what the project pins
```

If they already match, report "already aligned" and stop.

### 2. Run the pin update

```bash
edikt upgrade-pin
```

The binary walks up from `cwd` to find `.edikt/config.yaml`, updates/appends the `edikt_version` field, and exits 0. All other lines in the file are preserved.

### 3. Verify and stage

```bash
git diff .edikt/config.yaml   # confirm only edikt_version changed
```

Suggest committing in a focused commit:

```
chore(edikt): pin to vX.Y.Z to match installed launcher
```

### 4. Recovery

If `bin/edikt` is missing:

```
edikt binary not found. Bootstrap via /edikt:upgrade in Claude Code (primary path).
```

If `.edikt/config.yaml` is not found in the cwd or any ancestor:

```
No .edikt/config.yaml found. Are you in an edikt-managed project? Run /edikt:init.
```

## Reference

### Natural-language triggers

- "fix the pin warning"
- "update the project pin"
- "align edikt version with my install"

### Relationship to /edikt:upgrade

- `/edikt:upgrade` runs the full upgrade flow (fetch new version → migrate sidecars if needed → update hooks/agents/CLAUDE.md → bump pin).
- `/edikt:upgrade-pin` does only the last step (bump the pin). Useful when the version is already activated but the project pin drifted.

### Notes

Per the architectural principle "slash commands are the primary user surface always," users should invoke `/edikt:upgrade-pin`, not `edikt upgrade-pin` directly. The binary remains fully discoverable via `edikt --help`.
