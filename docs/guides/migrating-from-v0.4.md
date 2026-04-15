# Migrating from v0.4.x to v0.5.0

> **Stub** — full content ships in Phase 14. This stub exists so `edikt doctor --backfill-provenance` documentation has a canonical reference.

## Overview

v0.5.0 introduces a versioned on-disk layout (`~/.edikt/versions/<tag>/`), manifest integrity checks, and per-agent provenance frontmatter (`edikt_template_hash` + `edikt_template_version`). Existing v0.4.x installs are migrated automatically on first use via `edikt migrate`.

## Automatic migration (M1–M5)

Run `edikt migrate` (or let any subcommand trigger it) to apply:

- **M1** — move flat `~/.edikt/` layout to versioned `~/.edikt/versions/0.5.0/`
- **M2** — rewrite CLAUDE.md HTML comment sentinels to visible link-reference sentinels
- **M3** — normalise flat command file names to namespaced equivalents
- **M4** — mark governance.md for schema v2 recompile
- **M5** — add missing `paths:`, `stack:`, `gates:` keys to config.yaml

## Optional: provenance backfill (M6)

Agents installed before v0.5.0 do not carry `edikt_template_hash` frontmatter. Without it, `/edikt:upgrade` falls back to the v0.4.3 diff classifier instead of the faster provenance-first path.

To opt into backfill, run:

```sh
edikt doctor --backfill-provenance [--dry-run]
```

**This command is never automatic.** You must invoke it explicitly.

### How backfill works

For each `.claude/agents/*.md` without `edikt_template_hash`:

1. The installer version is read from the agent's `edikt_version:` frontmatter key (written by some v0.4.x releases). If absent, you are prompted to choose a version.
2. The captured source template for that version is loaded from `~/.edikt/migration-fixtures/v<version>/templates/agents/<name>.md`.
3. An md5 hash of the raw template bytes is computed (identical to the hash written at install time by v0.5.0).
4. The template is re-synthesized (path substitutions from `config.yaml`, stack filter) and compared against your installed file using Levenshtein distance.
   - **≤ 15% of installed file size** → near-match, `edikt_template_hash` and `edikt_template_version` are written.
   - **> 15% of installed file size** → the agent has been substantially customised; backfill skips it to avoid misrepresenting its provenance.
5. Events `provenance_backfilled` / `provenance_backfill_skipped` are appended to `~/.edikt/events.jsonl`.

### After backfill

Backfilled agents are treated identically to agents installed by v0.5.0 init. The `/edikt:upgrade` fast-preserve and resynth-safe-replace paths become reachable, removing the diff-classifier overhead.

### Edge case: identical template across two versions

If the source template bytes were the same in two captured versions (e.g. v0.1.0 and v0.1.4), you will be prompted to select which version to record in `edikt_template_version`. The hash value is the same regardless of your choice — only the version label differs.

### Customised agents

Agents marked with `<!-- edikt:custom -->` or listed in `.edikt/config.yaml` under `agents.custom` are skipped by `/edikt:upgrade` unconditionally; backfill skipping them is irrelevant (upgrade will not touch them either way).

---

_Full migration guide — including rollback, per-platform notes, and CI considerations — will be added in Phase 14._
