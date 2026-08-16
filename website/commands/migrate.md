# edikt migrate

Two migrations live under `edikt migrate`. `sidecars` lifts legacy in-body directive blocks into co-located sidecars; `to-v2` rewrites v1 single-anchor sidecars into the v2 multi-anchor shape. Both are one-shot and idempotent, and `/edikt:gov:compile` refuses to run until each has been applied.

## edikt migrate sidecars

One-shot, mandatory migration to the v0.6.0 sidecar format. Reads every governance `.md` (ADRs, Invariant Records, guidelines) and **structurally strips** the in-body `[edikt:directives:start]` block into a co-located `<artifact>.edikt.yaml` sidecar. The sidecar is a skeleton — `topic: needs-extraction`, empty `directives: []`, with the legacy block's contents preserved verbatim under a transient `migration_preserved:` field. Canonical directive content is produced by `/edikt:gov:compile` afterwards (Phase B). After migration, `/edikt:gov:compile` reads sidecars only — there is no double-parser window.

`/edikt:upgrade` orchestrates both phases (structural strip + canonical extraction) automatically as a single user-visible step. Use the binary subcommand directly when scripting CI flows or when you want to rehearse the strip separately.

### Usage

```bash
edikt migrate sidecars --dry-run                 # plan; required first
edikt migrate sidecars --apply                   # execute the plan
edikt migrate sidecars --apply --force           # bypass the 24h dry-run gate
edikt migrate sidecars --dry-run --json          # machine-readable plan
```

`--dry-run` and `--apply` are mutually exclusive — exactly one is required.

### Flags

| Flag | Description |
|---|---|
| `--dry-run` | Print the plan; write a gate file at `.edikt/state/migration-dry-run.json` recording the plan timestamp and the artifacts inspected. Idempotent. |
| `--apply` | Execute the plan. Refuses unless a successful `--dry-run` was recorded for this directory in the last 24 hours, or `--force` is set. |
| `--force` | Bypass the 24h dry-run gate. Test/escape hatch — production flows should run `--dry-run` first. |
| `--json` | Emit the dry-run plan or apply summary as JSON on stdout. The progress UI is suppressed when `--json` is set. |

### The 24-hour dry-run window

`--apply` checks the timestamp recorded by the most recent `--dry-run` for the current project. If the timestamp is older than 24 hours (or absent entirely), `--apply` refuses with:

```text
migrate sidecars: --dry-run required first (or pass --force).
Run: edikt migrate sidecars --dry-run
```

The window exists because sidecar generation is destructive on the prose body — the in-body sentinel block is removed atomically with the sidecar write. You should not apply a plan you haven't reviewed today. `--force` exists for CI / test rehearsal flows that have already validated the plan upstream.

The gate file at `.edikt/state/migration-dry-run.json` is local machine state. `.edikt/state/` is gitignored.

### Two-phase migration

The migration is split at a clean architectural boundary. `edikt migrate sidecars` does structural cleanup only; `/edikt:gov:compile` produces canonical content. Both phases together complete the migration.

#### Phase A — `edikt migrate sidecars --apply` (pure Go, no LLM)

For every artifact with an in-body sentinel block (whatever shape — v0.5.x-full, v0.5.x-partial, v0.4.3-legacy, pre-v0.4, hand-edited, unknown), Phase A:

1. Parses the in-body block.
2. Writes a skeleton sidecar with `topic: needs-extraction`, empty `directives: []`, and the legacy block's contents preserved verbatim under a transient top-level `migration_preserved:` field:
   ```yaml
   migration_preserved:
     schema_detected: "v0.5x-full" | "v0.5x-partial" | "v0.4.3-legacy" | "unknown"
     directives: [...]                # verbatim from sentinel
     manual_directives: [...]
     suppressed_directives: [...]
     reminders: [...]
     verification: [...]
     topic: <legacy>                  # hint, optional
     signals: [...]                   # hint, optional
   ```
3. Strips the in-body sentinel block from the parent `.md` (only after the sidecar writes successfully).

After the per-artifact loop, a **post-migration verification step** scans every migrated `.md` and fails the run with a per-file list if any column-0 `[edikt:directives:start]: #` survived. Documentation files that opt out via skip-list (`<!-- edikt:migration:skip -->` or frontmatter `documents_legacy_format: true`) are excluded by design. The `.edikt/backups/` directory holds the pre-migration `.md` so recovery is straightforward if this gate fires.

#### Phase B — `/edikt:gov:compile` (dispatches sidecar-extractor)

The compile's Phase A treats every `migration_preserved`-bearing sidecar as stale (`IsStale` returns true unconditionally when the transient field is present) and dispatches the `sidecar-extractor` agent per artifact. The extractor follows seven preservation rules (see `templates/agents/sidecar-extractor.md` § "On migration-preserved baselines"):

1. `migration_preserved.directives` → output `directives[]` verbatim (extractor may add new entries derived from prose; MUST NOT drop or rephrase preserved entries).
2. `migration_preserved.manual_directives` → output `manual_directives[]` verbatim.
3. `migration_preserved.suppressed_directives` → output `suppressed_directives[]` verbatim.
4. `migration_preserved.reminders` → output `reminders[]` verbatim (may append from `## Confirmation` / `## Enforcement` prose).
5. `migration_preserved.verification` → output `verification[]` verbatim (same append rule).
6. `migration_preserved.topic` and `.signals` are HINTS; the extractor synthesizes canonical values from prose when the hints don't fit.
7. The extractor MUST NOT include `migration_preserved:` in its output sidecar.

After the dispatch, a **post-extractor lossless gate** (`--on-loss=auto`, defaults to `abort` in CI / `accept` in TTY) calls `lossless.CheckLosslessAgainstDirectives(MigrationPreserved.directives, sidecar)` per artifact. Any preserved (modality, ref_id, normalized noun-phrase) tuple missing from the extractor's output is a loss; the gate surfaces a per-sidecar report and either aborts non-zero (CI safety) or warns and continues (interactive use). Once the check captures comparison data, compile **strips `migration_preserved:` from every sidecar deterministically** — steady-state sidecars never carry the transient field.

**All sentinel shapes follow the same code path.** Phase A's schema detection becomes audit metadata only (recorded in `migration_preserved.schema_detected` for diagnostics). No branching, no skip-list for "unknown" shapes — every sentinel becomes `migration_preserved` content for the extractor to interpret.

### Resolving regressions

Strict mode records the losses described above as a machine-readable manifest:

```bash
bin/edikt migrate sidecars --strict --report-json .edikt/state/v060-strict-report.json
```

`/edikt:sidecar:regenerate` consumes that manifest. It **auto-fixes LOST items by re-running the extractor, and writes FACTUAL and DEGRADED items to a worklist for manual review.**

```bash
/edikt:sidecar:regenerate
/edikt:sidecar:regenerate --manifest <path>     # default: .edikt/state/v060-strict-report.json
```

#### How it works

1. **Verify binary presence.** Requires the `edikt` tier-2 helper; refuses and directs you to `edikt install edikt` if absent.
2. **Resolve the manifest.** Uses `--manifest` or defaults to `.edikt/state/v060-strict-report.json`. If the manifest is missing, the command runs `bin/edikt migrate sidecars --strict` to produce one — exit 0 means no regressions and the command stops.
3. **Parse and group items by category** — `LOST`, `FACTUAL`, `DEGRADED` — and print a summary count across all affected artifacts.
4. **Auto-regenerate LOST items.** For each unique parent artifact (deduplicated — multiple LOST items from one artifact yield a single call), dispatches the locked `sidecar-extractor` subagent, up to 4 concurrently. The extractor rewrites the sidecar in place.
5. **Verify LOST items are resolved.** Re-runs `bin/edikt migrate sidecars --strict`. Exit 0 means LOST items cleared; non-zero prints a follow-up instruction.
6. **Write FACTUAL and DEGRADED items to a worklist** at `docs/internal/v060-manual-review.md` as a deduplicated checkbox list — these are modality drift and verification abstraction that require human judgement, not auto-regeneration.
7. **Report exit status** — `INCOMPLETE` while any worklist item is unchecked, or `COMPLETE` once all regressions are resolved.

#### What gets auto-fixed vs. flagged

| Category | Meaning | Handling |
|---|---|---|
| `LOST` | A directive, `paths:`, or `scope:` was dropped during migration | Auto-fixed by re-running the extractor |
| `FACTUAL` | Modality drift (e.g. MUST became SHOULD) | Written to the manual-review worklist |
| `DEGRADED` | A verification was abstracted or weakened | Written to the manual-review worklist |

#### Notes

- The binary's output is displayed verbatim and never parsed — only its exit code is inspected.
- After regenerating, run `/edikt:gov:compile` so the refreshed sidecars land in compiled governance.

### Skip list and fence detection

The migration skips files whose prose **mentions** the in-body sentinel format without **using** it:

- Files that document the in-body schema itself (their prose contains example blocks)
- `SPEC-*.md` (any spec file)
- Any sentinel block inside a fenced code region (` ``` … ``` ` or `~~~ … ~~~`) — fence detection runs through a markdown parser; only document-level blocks are lifted

You can also opt files out by adding frontmatter:

```yaml
---
migration: skip
documents_legacy_format: true
---
```

…or by inserting a marker comment at the top of the body:

```markdown
<!-- edikt:migration:skip reason="example block in prose" -->
```

Custom files outside the skip list that contain a bare-mention sentinel will be flagged in the dry-run plan — add the file to one of the opt-out mechanisms above before applying.

### Rollback

Before commit:

```bash
git checkout -- docs/architecture/decisions/ docs/architecture/invariants/ docs/guidelines/
git clean -f docs/architecture/decisions/*.edikt.yaml \
             docs/architecture/invariants/*.edikt.yaml \
             docs/guidelines/*.edikt.yaml
```

After commit: revert the migration commit, then `edikt rollback` (or `edikt use <prior-version>`). Your legacy in-body sentinels come back; sidecars are left in the working tree as untracked files (delete or keep).

### Idempotency

Re-running `--apply` on an already-migrated project is a no-op. Half-completed migrations resume cleanly — if an LLM resync fails on artifact 8 of 14, fix the issue and re-run; the first 7 are no-ops, work resumes from 8.

## edikt migrate to-v2

Separate, now-required migration that rewrites each governance sidecar's `directives[].source_excerpt` and `prohibitions[].source_excerpt` (the v1 single-anchor shape) into the v2 `source_excerpts[]` list, and bumps `schema_version` to `2`. Pure structural cleanup — no LLM, deterministic, idempotent.

Existing anchors are carried verbatim as the single element of the new list; this migration never invents a second anchor. Richer multi-anchor grounding only arrives when an artifact is next re-extracted by `/edikt:gov:compile` — not here.

It never writes a parent `.md` file, and edits only the two keys it owns (plus a stale `yaml-language-server $schema` comment pointing at the v1 schema, if present) — human-approved fields (`verify:`, `human_approved_at`, approved `paths:`) survive byte-identical.

### Usage

```bash
edikt migrate to-v2                 # convert every v1-shaped sidecar in one deterministic pass
edikt migrate to-v2 --dry-run       # report what would change, without writing
```

The command takes no positional arguments.

### Flags

| Flag | Description |
|---|---|
| `--dry-run` | Report what would change without writing anything. For each discovered sidecar, checks whether conversion would change its bytes and tallies converted vs. already-v2. |

### What triggers it

`/edikt:gov:compile`'s Phase A refuses to dispatch the sidecar-extractor while any sidecar in the corpus is still v1-shaped. The extractor writes `gov-sidecar.v2` (`source_excerpts[]`); dispatching against a mixed corpus would convert artifacts one at a time by accident, leaving the corpus in two shapes at once with no record of which is which. If you run `/edikt:gov:compile` before this migration, it stops before the first dispatch with:

```text
error: refusing to dispatch the extractor — N of M sidecar(s) still carry the v1 single-anchor shape:
  <artifact-id>
  ...
The extractor writes gov-sidecar.v2 (source_excerpts[]). Dispatching now would convert
the corpus one artifact at a time, by accident, leaving it in two shapes at once.
Run `bin/edikt migrate to-v2` first — it converts every sidecar in one deterministic pass.
```

Run `edikt migrate to-v2` (optionally with `--dry-run` first) to convert the whole corpus in one pass, then re-run `/edikt:gov:compile`.

### Output

Reports the denominator, not just the hits — "0 converted" out of 0 discovered is a broken scan, not a finished migration:

```text
migrate to-v2: converted 12 of 60 sidecar(s); 48 already v2, 3 artifact(s) have no sidecar
  + ADR-001
  + ADR-014
  ...
```

Artifacts with no sidecar on disk (superseded or skip-marked artifacts legitimately have none) are counted separately rather than erroring or being silently dropped from the count. The command exits non-zero if zero sidecars are discovered under the project root at all.

## Natural language triggers

For `sidecars`:

- "migrate sidecars"
- "upgrade my sidecars"
- "convert legacy ADRs"
- "run the sidecar migration"

For `to-v2`:

- "migrate to v2"
- "convert sidecars to v2"
- "run the schema v2 migration"

## Reference

- [Sidecar Migration walkthrough](/guides/sidecar-migration) — full flow with example output and edge cases
- [Upgrading to v0.7.0](/guides/v0.7.0-upgrade) — the real upgrade path, including both migrations as steps
- [Sidecar architecture](/governance/sidecar) — sidecar shape and the extractor's role
- [`/edikt:upgrade`](/commands/upgrade) — runs `migrate sidecars` automatically with prompt
- [`/edikt:gov:compile`](/commands/gov/compile) — refuses to run until `sidecars` is applied, and refuses Phase A dispatch until `to-v2` is applied
