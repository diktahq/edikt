# Sidecar Migration

The per-artifact walkthrough of how `edikt migrate sidecars` and the follow-up `/edikt:gov:compile` lift a legacy in-body `[edikt:directives:start]` block into a v0.6.0 co-located `<artifact>.edikt.yaml` sidecar. This page complements [`edikt migrate sidecars`](/commands/migrate) — that page is the flag reference; this one is the narrative.

The whole flow is two phases: **structural strip** (pure Go, no LLM, instant) and **canonical extraction** (LLM-backed, runs as part of `/edikt:gov:compile`). They are independent on the wire; `/edikt:upgrade` runs both as one user-visible step.

## What you'll see before migration

A legacy ADR carries its directive metadata inline:

```markdown
# ADR-007 — Caching strategy

**Status:** Accepted

## Decision

Cache entries MUST declare an explicit TTL; nothing is cached indefinitely.

[edikt:directives:start]: # managed by edikt — do not edit this block manually
topic: caching
signals: [cache, ttl, redis]
directives:
  - "Cache entries MUST declare an explicit TTL. (ref: ADR-007)"
manual_directives: []
suppressed_directives: []
reminders: []
verification:
  - "[ ] Every cache write passes an explicit TTL argument (ref: ADR-007)"
[edikt:directives:end]: #
```

That sentinel block is the unit being migrated. Phase A strips it and produces a sidecar; Phase B (via the extractor agent) populates the sidecar canonically.

## Phase A — structural strip

Triggered by `edikt migrate sidecars --apply`. For each governance `.md` with an in-body sentinel block, Phase A:

1. **Parses the legacy block.** Detects which schema flavor it is (`v0.5x-full`, `v0.5x-partial`, `v0.4.3-legacy`, `unknown`) and decodes the lists.
2. **Writes a skeleton sidecar.** `<artifact>.edikt.yaml` lands next to the `.md`. The skeleton has `topic: needs-extraction`, empty `directives: []`, and the legacy block's lists preserved verbatim under a transient `migration_preserved:` field:

   ```yaml
   schema_version: 1
   topic: needs-extraction
   path: docs/architecture/decisions/ADR-007-caching-strategy.md
   signals: []
   directives: []
   migration_preserved:
     schema_detected: v0.5x-full
     directives:
       - "Cache entries MUST declare an explicit TTL. (ref: ADR-007)"
     manual_directives: []
     suppressed_directives: []
     reminders: []
     verification:
       - "[ ] Every cache write passes an explicit TTL argument (ref: ADR-007)"
     topic: caching
     signals: [cache, ttl, redis]
   ```

3. **Strips the in-body block.** The `[edikt:directives:start] … [edikt:directives:end]` lines are removed from the parent `.md` only after the sidecar write succeeds. The pre-migration body lives at `.edikt/backups/<timestamp>/` for recovery.

Phase A is **pure Go, no LLM, no `Task`/`Agent` dispatch**. A static-analysis test verifies no LLM-dispatch symbol is reachable from the migrate code path.

### Post-migration verification

After the per-artifact loop, the migration scans every modified `.md` for any column-0 `[edikt:directives:start]: #` line that might have survived. A surviving block (rare — only happens on unparseable shapes outside the skip list) fails the run with a per-file list and points you at the backup directory.

## Phase B — canonical extraction via `/edikt:gov:compile`

After Phase A, every migrated sidecar carries `migration_preserved:` and `topic: needs-extraction`. `/edikt:gov:compile`'s Phase A treats any sidecar with `migration_preserved` as **stale unconditionally** and dispatches the `sidecar-extractor` agent for each.

The extractor runs in a forked subagent context with a locked prompt. It reads the parent `.md`, synthesizes a canonical `topic`, populates `signals[]` from the prose, lifts every preserved directive verbatim into `directives[]`, and may append new directives derived from the prose body that weren't captured in the legacy block. Seven preservation rules apply:

1. `migration_preserved.directives` → `directives[]` verbatim; the extractor may add new entries but MUST NOT drop or rephrase preserved ones.
2. `migration_preserved.manual_directives` → `manual_directives[]` verbatim.
3. `migration_preserved.suppressed_directives` → `suppressed_directives[]` verbatim.
4. `migration_preserved.reminders` → `reminders[]` verbatim; may append from `## Confirmation` / `## Enforcement` prose.
5. `migration_preserved.verification` → `verification[]` verbatim; same append rule.
6. `migration_preserved.topic` and `.signals` are HINTS only; the extractor synthesizes canonical values from prose when the hints don't fit.
7. The extractor MUST NOT include `migration_preserved:` in its output sidecar — the field is transient by design.

### Lossless gate

After the dispatch, the compile checks every output sidecar against the corresponding `migration_preserved.directives`. The check tokenizes each directive into `(modality, ref_id, normalized noun phrase)` and looks for every preserved tuple in the output. Any tuple missing is a **loss**.

The behaviour is governed by `--on-loss`:

| Value | When |
|---|---|
| `abort` (default in CI / non-TTY) | Exit non-zero with a per-sidecar loss report. The output sidecar is still written; you decide whether to accept or fix. |
| `accept` (default in TTY) | Warn and continue. Loss is reported; the run completes. |
| `auto` | Detect TTY → `accept`; non-TTY → `abort`. |

Once the lossless check completes, compile **strips `migration_preserved:` from every sidecar deterministically**. Steady-state sidecars never carry the transient field.

### Sample compile output

```text
edikt gov compile

Phase A — resyncing 14 stale sidecar(s) at concurrency=8
  [1/14] ADR-001  PASS in 31s
  [2/14] ADR-007  PASS in 28s
  [3/14] ADR-009  PASS in 33s
  ... (10 more)
  [14/14] INV-008  PASS in 22s
Phase A done: 14 ok, 0 failed in 1m 12s

Lossless gate:
  ✓ All 14 sidecars preserved every directive from migration_preserved.

Phase B — merging 14 sidecars into governance/
  → governance/architecture.md (12 directives)
  → governance/compile.md      (9 directives)
  → governance/hooks.md        (15 directives)
  → governance/release.md      (4 directives)
  → governance/security.md     (8 directives)
  → governance/tooling.md      (6 directives)
  → governance.md              (index + invariants)
Phase B done: 6 topic files + index, 138 total directives, 1.9s

Sidecar verify coverage — 0/14 sidecars (0%); items 0/138 (0%); 0 passed, 0 failing, 138 skipped.
```

The 0% verify coverage is expected on the first compile after migration — adding `verify:` lines is post-migration work. The doctor's coverage line is the metric to watch as the project authors them in.

## Skip list and fence detection

Some files mention the in-body sentinel format without using it (the ADRs that defined the format, SPEC files, documentation). Those are excluded by default:

- **Built-in skip list:** `ADR-008-*.md`, `ADR-009-*.md`, and any `SPEC-*.md`.
- **Code-fence detection:** any sentinel block inside a fenced ` ``` … ``` ` (or `~~~`) region is ignored. A markdown parser walks each file; only document-level (column-0) blocks are lifted.
- **Frontmatter opt-out:** add `migration: skip` or `documents_legacy_format: true` to a file's frontmatter.
- **Inline opt-out:** add `<!-- edikt:migration:skip reason="..." -->` near the top of the body.

Files outside the skip list that contain a bare-mention sentinel are flagged in the dry-run plan — opt them out before applying.

## The 24-hour dry-run gate

`edikt migrate sidecars --apply` refuses to run unless `--dry-run` was recorded for the same project in the last 24 hours. The gate file lives at `.edikt/state/migration-dry-run.json` (gitignored). Override via `--force` for CI flows that validated the plan upstream.

The window exists because sidecar generation is destructive on the prose body — the in-body sentinel block is removed atomically. You should not apply a plan you haven't reviewed today.

## Edge cases

**Half-completed migration.** If the LLM resync fails on artifact 8 of 14, fix the underlying issue and re-run `/edikt:gov:compile`. The first 7 sidecars are no-ops (their `migration_preserved:` has already been stripped); work resumes from 8.

**A `.md` was hand-edited between migrate and compile.** Sidecar's `migration_preserved.directives` is the ground truth — the extractor preserves every entry verbatim. If you removed a directive from the prose, add it to `suppressed_directives[]` (or `manual_directives[]` if you've decided it stays).

**An artifact's sentinel was custom-shaped.** Phase A's `schema_detected: unknown` records the shape; the extractor handles it the same way (the schema_detected value is audit metadata, not a branch). If you see post-extraction loss on an `unknown`-shape artifact, file an issue with the original sentinel block.

**You don't want this artifact migrated.** Add `migration: skip` to its frontmatter before running migrate, or insert the inline `<!-- edikt:migration:skip -->` marker. The file stays in its legacy shape; `/edikt:gov:compile` will refuse the project as a whole until either the file is migrated or marked as skip.

## Rollback

Before the migration commit lands:

```bash
git checkout -- docs/architecture/ docs/guidelines/
git clean -f docs/architecture/**/*.edikt.yaml docs/guidelines/*.edikt.yaml
```

After the commit lands:

1. Revert the migration commit (`git revert <sha>`).
2. `edikt rollback` (or `edikt use <prior-version>`) to switch the active launcher version back.
3. Your legacy in-body sentinels return; the sidecars remain in your working tree as untracked files (delete or keep).

The `.edikt/backups/<timestamp>/` directory holds pre-migration `.md` snapshots for at least 30 days — recovery without git is possible if needed.

## Related

- [`edikt migrate sidecars`](/commands/migrate) — flag reference + JSON output shape
- [Upgrading to v0.7.0](/guides/v0.7.0-upgrade) — the orchestrated path via `/edikt:upgrade`, including this migration as one step
- [`/edikt:gov:compile`](/commands/gov/compile) — the second phase
- [Sidecar Architecture](/governance/sidecar) — the data model migrate produces
