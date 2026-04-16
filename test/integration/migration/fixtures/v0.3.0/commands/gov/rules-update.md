---
name: edikt:gov:rules-update
description: "Check for outdated rule packs and update them"
effort: normal
allowed-tools:
  - Read
  - Write
  - Glob
  - Grep
  - Bash
  - AskUserQuestion
---

# edikt:rules-update

Compare installed rule packs against edikt's templates and offer to update stale ones.

## Instructions

### 1. Load Config

Read `.edikt/config.yaml`. If not found:
```
No edikt config found. Run /edikt:init to set up this project.
```

### 2. Find edikt Templates

Locate edikt's rule templates (either `~/.edikt/templates/rules/` for global install, or the edikt repo location). Also read `_registry.yaml` from the same directory.

If templates are not found:
```
edikt templates not found. Install edikt globally or provide the path.
```

### 3. Compare Versions

If `.claude/rules/` does not exist or contains no `.md` files:
```
No rule packs installed. Run /edikt:init to set up rules for this project.
```
Stop.

For each `.claude/rules/*.md` file:

1. Read the installed file's YAML frontmatter to get `version:`
2. Check for `<!-- edikt:generated -->` marker
3. Check if a project override exists at `.edikt/rules/{name}.md` — if yes, this pack is overridden (skip)
4. Check if the config has an `extend:` for this pack under `rules.{name}.extend` — note for later
5. Look up the pack name (filename without `.md`) in the registry
6. Categorize:
   - **Overridden:** `.edikt/rules/{name}.md` exists (project owns this pack — skip)
   - **Outdated:** installed version < registry version
   - **Up to date:** installed version == registry version
   - **Manually edited:** no `edikt:generated` marker (skip by default)
   - **No version:** file has no `version:` frontmatter (predates versioning)
   - **Custom:** pack name not in registry (user-created, skip)
   - **Extended:** config has `extend:` — base pack updates normally, extension file is untouched

### 4. Display Summary

```
Rule Pack Status
────────────────
  code-quality.md   1.0.0 → 1.1.0   (outdated)
  testing.md        1.0.0 = 1.0.0   (up to date)
  go.md             1.0.0 → 1.2.0   (outdated)
  chi.md            —     → 1.0.0   (no version — predates versioning)
  security.md       [manually edited — skipped]
  my-custom.md      [custom rule — skipped]

2 outdated, 1 unversioned, 1 up to date, 2 skipped
```

If everything is up to date:
```
All rule packs are up to date.
```

### 5. Conflict Detection

Before offering updates, check for conflicts between the new rule pack content and existing project governance:

For each outdated pack that will be updated:
1. Read the new template content
2. Read the compiled governance files (`.claude/rules/governance.md` and `.claude/rules/governance/*.md`)
3. Check for contradictions:
   - Rule pack says "NEVER do X" but a compiled ADR directive says "do X"
   - Rule pack recommends a pattern that contradicts an invariant
   - Rule pack sets conventions that conflict with existing project guidelines

4. If conflicts found, report them:
   ```
   ⚠ Conflict detected in {pack_name}.md update:

     Rule pack (new):  "NEVER use SELECT * — always enumerate columns"
     Compiled (ADR-003): "Use SELECT * for audit log queries to capture all columns"

     Options:
       [1] Proceed — the rule pack will be installed, you can override the specific rule later
       [2] Skip this pack — keep the current version
       [3] Override — create .edikt/rules/{pack_name}.md with the project's convention
   ```

5. If no conflicts: proceed silently.

### 6. Install Preview

Show exactly what will change before applying any updates:

```
Install Preview
────────────────
  code-quality.md   1.0.0 → 1.1.0
    + Added: "NEVER use console.log in production — use structured logger"
    ~ Changed: error-handling section reworded for clarity
    - Removed: deprecated jQuery patterns section

  go.md             1.0.0 → 1.2.0
    + Added: slog patterns for Go 1.22+
    ~ Changed: error wrapping now uses fmt.Errorf with %w

  Conflicts: 0
  Files to update: 2
```

Then offer options:

```
Update options:
  [1] Update all outdated packs (2 files)
  [2] Select which packs to update
  [3] Skip — no changes
```

**If updating:**

For each pack to update:
1. Read the current installed file
2. Read the new template
3. Replace the installed file with the new template content

**Manually edited files** (no `edikt:generated` marker) are always skipped unless the user explicitly asks to include them:
```
security.md was manually edited. Update anyway? This will overwrite your customizations. (y/n)
```

### 7. Output Results

```
✅ Rules updated: {n} packs

  Updated:
    code-quality.md   1.0.0 → 1.1.0
    go.md             1.0.0 → 1.2.0

  Skipped:
    security.md       (manually edited)

  Conflicts resolved: 0

  Next: Run /edikt:gov:compile to recompile governance with updated rules.
```
