# Migrating from v0.4.x

v0.5.0 introduces a versioned layout for `~/.edikt/`. The flat layout from v0.4.x is automatically migrated. This guide walks you through the process.

## What changes

| Before (v0.4.x) | After (v0.5.0) |
|---|---|
| `~/.edikt/hooks/` | `~/.edikt/versions/0.5.0/hooks/` (symlinked to `current/hooks`) |
| `~/.edikt/templates/` | `~/.edikt/versions/0.5.0/templates/` |
| `~/.edikt/VERSION` | `~/.edikt/lock.yaml` (tracks active, previous, pinned) |
| Direct file edits | Provenance frontmatter + upgrade diff prompt |

Your existing config (`~/.edikt/config.yaml`), ADRs, specs, plans, and project files are never touched.

## Step-by-step

### 1. Update the launcher

```bash
# Homebrew
brew upgrade edikt

# curl
curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
```

### 2. Preview the migration

```bash
edikt migrate --dry-run
```

Example output:

```
Migration chain for install at ~/.edikt/ (detected: v0.4.3):

  M1: flat → versioned layout
      Move ~/.edikt/hooks/ → ~/.edikt/versions/0.5.0/hooks/
      Move ~/.edikt/templates/ → ~/.edikt/versions/0.5.0/templates/
      Create ~/.edikt/current → versions/0.5.0
      Create ~/.edikt/lock.yaml

  M4: compile schema v1 → v2
      Invoke /edikt:gov:compile to regenerate ~/.claude/rules/governance.md

Run `edikt migrate --yes` to apply.
```

### 3. Run the migration

```bash
edikt migrate --yes
```

The migration backs up your existing `~/.edikt/` to `~/.edikt/backups/migration-<ts>/` before making any changes.

### 4. Verify

```bash
edikt doctor
edikt version
```

`edikt doctor` checks symlink health, manifest integrity, and PATH placement.

## Troubleshooting

### "edikt migrate --abort"

If something went wrong mid-migration, restore the pre-migration snapshot:

```bash
edikt migrate --abort
```

This restores from `~/.edikt/backups/migration-<ts>/`.

### Agent files not recognized

If your agents don't have `edikt_template_hash` frontmatter (pre-v0.5.0 agents), the upgrade command uses the v0.4.3 classifier to determine if they were modified. To opt into provenance tracking:

```bash
edikt doctor --backfill-provenance
```

This assumes the installed file matches the template from your stored `edikt_version`. Review the proposed changes before confirming.

### governance.md schema warning

If `/edikt:doctor` reports "governance.md schema v1 detected", run:

```bash
/edikt:gov:compile
```

This regenerates `.claude/rules/governance.md` with schema v2 sentinel blocks.

## v0.5.0 directive hardening notes (SPEC-005)

These additions are backward-compatible. No action required to keep existing governance working.

### Existing ADRs

Existing ADRs continue to parse and compile. The two new sentinel fields (`canonical_phrases`, `behavioral_signal`) are optional. When absent, they default to `[]` and `{}`.

FR-003a warns when a multi-sentence directive has no `canonical_phrases`. This is warn-only in v0.5.0 — not a blocking error. To resolve warnings, run:

```bash
/edikt:adr:review --backfill
```

This retrofits `canonical_phrases` onto existing ADRs interactively, one at a time, with per-ADR approval before writing.

### Optional: tier-2 governance benchmark

If you want adversarial directive testing:

```bash
./bin/edikt install benchmark
```

This is not required and is never run automatically. See [/edikt:gov:benchmark](/commands/gov/benchmark) for full docs.

### v0.4.x users: migrate first

If you're on v0.4.x with a flat `~/.edikt/` layout, run the layout migration before installing tier-2:

```bash
edikt migrate --yes
```

The versioned layout (M1 migration) is a prerequisite for tier-2 tool install.

---

### What if I customized hooks or templates?

The migration preserves everything in place. M1 moves files to the new versioned layout but does not overwrite them. Your customizations remain. On the next `edikt upgrade`, the upgrade command presents a 3-way diff for any file where the template changed and you have local edits.
