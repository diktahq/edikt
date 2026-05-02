# Sidecar Migration

v0.6.0 replaces in-body `[edikt:directives:start]` sentinel blocks with co-located `<artifact>.edikt.yaml` sidecars. This is a one-time, mandatory migration on the first upgrade. Once applied, `gov:compile` reads sidecars only — there is no double-parser window.

The migration is idempotent and reversible (until you commit). Run `--dry-run` first; review the plan; apply.

## Before you upgrade

Check in or stash anything in flight under `docs/architecture/decisions/`, `docs/architecture/invariants/`, or `docs/guidelines/`. The migration writes new files alongside existing ones and edits the `.md` to remove the old sentinel block, so you want a clean diff to review.

## Trigger the migration

`/edikt:upgrade` detects legacy in-body sentinels automatically and prompts you:

```text
EDIKT UPGRADE — v0.6.0 sidecar migration
─────────────────────────────────────────────────────
Detected 14 artifacts with legacy [edikt:directives:start] blocks.

  docs/architecture/decisions/
    ADR-001-claude-code-only.md           (v0.5.x schema)
    ADR-003-claude-code-feature-surface.md (v0.5.x schema)
    ADR-007-compile-schema-version.md     (v0.4.3 schema — needs LLM resync)
    ...

  docs/architecture/invariants/
    INV-001-plain-markdown-only.md        (v0.5.x schema)
    INV-003-hooks-emit-structured-json.md (v0.5.x schema)
    ...

  docs/guidelines/
    error-handling.md                     (v0.5.x schema)

Mechanical lifts: 11
LLM resyncs:       3 (will dispatch sidecar-extractor agent per artifact)

v0.6.0 requires migrating in-body sentinels to sidecars.
Apply now? [y/N]
```

Accept (`y`) and the migration runs. Decline (`N`) and `/edikt:gov:compile` refuses to run until you complete the migration.

## What gets lifted

The migration tool reads every candidate `.md` (ADRs, invariants, guidelines), looks for the `[edikt:directives:start]: #` marker outside fenced code regions, and detects the schema version per-artifact:

| Detected schema | Lift path | Cost |
|---|---|---|
| v0.5.x full / v0.6.0-rc1 (`source_hash:` + `topic:` + `signals:`) | Mechanical mapping. `directives → directives`, `topic → topic`, `signals → signals`. Hashes dropped (recomputed on read). For each directive, search the body for the verbatim text and derive `source_excerpt`. | No LLM; fast. |
| v0.5.x partial (`source_hash:` only — `topic:` / `signals:` were never backfilled) | Mechanical extract for `directives[]`. Then dispatch the `sidecar-extractor` subagent on the prose body to derive `topic` and `signals`. This is the most-common dev-branch state — `topic:` was optional in v0.5.x with an LLM-grouping fallback. | One LLM call per artifact; one-shot at upgrade time only. |
| v0.4.3 legacy (flat `content_hash:` + directive list, no topic/signals) | Mechanical extract for `directives[]`. Then dispatch the `sidecar-extractor` subagent on the prose body to derive `topic` and `signals`. | One LLM call per artifact; one-shot at upgrade time only. |

After the sidecar writes successfully, the migration removes the `[edikt:directives:start]…[edikt:directives:end]` block from the `.md`. Failures roll back: if the sidecar write fails, the `.md` is left untouched.

## Skip-list and fence detection

Some governance artifacts contain example sentinel blocks (typically ADRs documenting legacy formats). Migrate skips those via opt-in declarations. The previously hardcoded `ADR-008-`/`ADR-009-`/`SPEC-` prefix list was removed in Phase 6 of `PLAN-sidecar-review-fixes` #16; an artifact opts out by declaring its skip status on the file itself, recognized through one of two mechanisms:

- **YAML frontmatter** at the top of the `.md`:
  ```yaml
  ---
  migration: skip
  reason: documents the legacy three-list directive schema
  ---
  ```
  Or, for self-documenting ADRs:
  ```yaml
  ---
  documents_legacy_format: true
  ---
  ```

- **HTML comment marker** near the top of the body (within the first 4 KiB):
  ```html
  <!-- edikt:migration:skip reason="documents the v0.4.3 schema" -->
  ```

Both forms record an audit reason that surfaces in `bin/edikt doctor`'s sidecar health checks and in the dry-run output. The bundled `ADR-008` and `ADR-009` ship with the marker comment because their bodies contain example sentinel blocks documenting the legacy format itself.

Sentinel blocks inside fenced code regions (` ```…``` `) are documentation, not real sentinels. Fence detection runs through a CommonMark-conformant scanner that tracks fence-marker character + run-length parity (Phase 3 of `PLAN-sidecar-review-fixes` #2) — a `~~~` line inside a ``` block does not flip the state, closing a parser-confusion bypass.

If you have your own ADRs that include example sentinel blocks in fenced regions, you don't need to do anything. Bare-mention sentinels outside fences in custom files are not skipped automatically; if your project has them, add the marker comment when the dry-run flags it.

## Run the migration directly

You can run the migration without the upgrade prompt:

```bash
# Plan — required first. Writes .edikt/state/migration-dry-run.json with a timestamp.
edikt migrate sidecars --dry-run

# Apply — refuses unless --dry-run was run in this directory in the last 24h.
edikt migrate sidecars --apply

# Bypass the 24h gate (CI / test rehearsal flows that validated the plan upstream).
edikt migrate sidecars --apply --force
```

**The 24-hour dry-run window.** `--dry-run` writes a gate file to `.edikt/state/migration-dry-run.json` (gitignored, local machine state). `--apply` reads that file's timestamp; if it's older than 24 hours, apply refuses with `--dry-run required first (or pass --force)`. The window exists because sidecar generation is destructive on the prose body — you should not apply a plan you haven't reviewed today. Recommended workflow: run `--dry-run`, review, run `--apply` in the same session.

Output:

```text
$ edikt migrate sidecars --dry-run

EDIKT MIGRATE SIDECARS — DRY RUN
─────────────────────────────────────────────────────
Scope: 14 artifacts found
  Mechanical lifts:  11
  LLM resyncs:       3
  Skipped (skip-list): 3
  Skipped (no sentinel): 2

Per-file plan:
  ADR-001-claude-code-only.md
    → ADR-001-claude-code-only.edikt.yaml [v0.5.x mechanical]
    → remove in-body block (lines 142-187)
  ADR-007-compile-schema-version.md
    → ADR-007-compile-schema-version.edikt.yaml [v0.4.3 LLM resync]
    → remove in-body block (lines 89-120)
  ...

Run `edikt migrate sidecars --apply` to execute this plan.
```

## After the migration

`/edikt:gov:compile` runs cleanly:

```bash
$ /edikt:gov:compile

Phase A — no stale sidecars (skipped)
Phase B — merging 14 sidecars into 6 topic files... 142ms
✅ Compiled
```

`/edikt:doctor` reports clean:

```text
[ok]   Sidecar Health — 14/14 sidecars valid
```

Verify your `.md` files are untouched in the prose body (the `[edikt:directives:*]` block is gone, but Context, Decision, Consequences are byte-identical — that's INV-002 as a structural property now).

## Idempotency

The migration is idempotent. Re-running `--apply` with sidecars already present and bodies unchanged is a no-op. This means:

- Half-completed migrations resume cleanly. If the LLM resync fails on artifact 8 of 14, fix the issue and re-run — the first 7 are no-ops, work resumes from 8.
- CI bots and automation can run the migration command unconditionally without checking state first.

## Recovering from a bad migration

If you applied the migration and want to undo it before committing:

```bash
git checkout -- docs/architecture/decisions/ docs/architecture/invariants/ docs/guidelines/
git clean -f docs/architecture/decisions/*.edikt.yaml \
             docs/architecture/invariants/*.edikt.yaml \
             docs/guidelines/*.edikt.yaml
```

If you already committed and want to roll back to v0.5.x: revert the migration commit, then `edikt rollback v0.6.0`. Your v0.5.x in-body sentinels come back; sidecars are left in the working tree as untracked files (delete or keep).

## Edge cases

**Custom artifact paths.** If your project uses a non-default `paths.architecture` or `paths.guidelines` in `.edikt/config.yaml`, the migration follows the configured paths.

**Hand-edited `manual_directives:` and `suppressed_directives:`** (from ADR-008's three-list schema). The mechanical lift carries `manual_directives:` into `directives[]` with a `source_excerpt` pointing at the prose. `suppressed_directives:` are dropped — the v0.6.0 sidecar has a single `directives[]` array. To preserve a suppression: remove the source language from the prose body before migrating, or remove the unwanted entry from the sidecar after migration (re-running `:compile` will regenerate it unless the prose changes).

**A sidecar already exists.** If a `.edikt.yaml` is already present (e.g., from a partial v0.6.0-rc1 install), the mechanical lift overwrites it. The dry-run plan flags this so you see it before applying.

## Reference

- [Sidecar Architecture](/governance/sidecar) — what sidecars are and why
- ADR-027 — Sidecar architecture for governance metadata (supersedes ADR-008)
- ADR-028 — Two-phase compile (amends ADR-020)
- [`/edikt:upgrade`](/commands/upgrade) — runs the migration prompt
- [`/edikt:gov:compile`](/commands/gov/compile) — refuses to run until migration is applied
- [`/edikt:doctor`](/commands/doctor) — verifies sidecar health post-migration
