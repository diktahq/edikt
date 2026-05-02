# edikt migrate sidecars

One-shot, mandatory v0.5.x → v0.6.0 migration. Reads every governance `.md` (ADRs, Invariant Records, guidelines), lifts the in-body `[edikt:directives:start]` block into a co-located `<artifact>.edikt.yaml` sidecar, and removes the in-body block from the prose. After migration, `/edikt:gov:compile` reads sidecars only — there is no double-parser window.

`/edikt:upgrade` runs this automatically and prompts for confirmation. Use the binary subcommand directly when scripting CI flows or when you want to rehearse the plan separately.

## Usage

```bash
edikt migrate sidecars --dry-run                 # plan; required first
edikt migrate sidecars --apply                   # execute the plan
edikt migrate sidecars --apply --force           # bypass the 24h dry-run gate
edikt migrate sidecars --dry-run --json          # machine-readable plan
```

`--dry-run` and `--apply` are mutually exclusive — exactly one is required.

## Flags

| Flag | Description |
|---|---|
| `--dry-run` | Print the plan; write a gate file at `.edikt/state/migration-dry-run.json` recording the plan timestamp and the artifacts inspected. Idempotent. |
| `--apply` | Execute the plan. Refuses unless a successful `--dry-run` was recorded for this directory in the last 24 hours, or `--force` is set. |
| `--force` | Bypass the 24h dry-run gate. Test/escape hatch — production flows should run `--dry-run` first. |
| `--json` | Emit the dry-run plan or apply summary as JSON on stdout. The progress UI is suppressed when `--json` is set. |

## The 24-hour dry-run window

`--apply` checks the timestamp recorded by the most recent `--dry-run` for the current project. If the timestamp is older than 24 hours (or absent entirely), `--apply` refuses with:

```text
migrate sidecars: --dry-run required first (or pass --force).
Run: edikt migrate sidecars --dry-run
```

The window exists because sidecar generation is destructive on the prose body — the in-body sentinel block is removed atomically with the sidecar write. You should not apply a plan you haven't reviewed today. `--force` exists for CI / test rehearsal flows that have already validated the plan upstream.

The gate file at `.edikt/state/migration-dry-run.json` is local machine state. `.edikt/state/` is gitignored.

## Lift paths

The migration tool inspects each artifact's existing in-body sentinel block (when present) and routes to one of three lift paths:

| Detected schema | Trigger | Cost |
|---|---|---|
| **v0.5.x full** | `source_hash:` + `topic:` + `signals:` all present | Mechanical mapping; no LLM |
| **v0.5.x partial** | `source_hash:` present, `topic:` / `signals:` absent | Per-artifact LLM resync via locked `sidecar-extractor` agent |
| **v0.4.3 legacy** | Flat `content_hash:` + directive list | Mechanical extract for `directives[]`; LLM derivation for `topic` and `signals` |

The v0.5.x partial path is the most-common dev-branch state and is what the dogfood project hits — `topic:` was optional in v0.5.x with an LLM-grouping fallback, so plenty of projects never had it backfilled.

LLM resyncs run with continue-on-error. A failed extraction writes a partial sidecar with `topic: needs-review` rather than aborting the migration.

## Skip list and fence detection

The migration skips files whose prose **mentions** the in-body sentinel format without **using** it:

- `ADR-008-*.md` and `ADR-009-*.md` (define the in-body schema; their prose contains example blocks)
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

## Rollback

Before commit:

```bash
git checkout -- docs/architecture/decisions/ docs/architecture/invariants/ docs/guidelines/
git clean -f docs/architecture/decisions/*.edikt.yaml \
             docs/architecture/invariants/*.edikt.yaml \
             docs/guidelines/*.edikt.yaml
```

After commit: revert the migration commit, then `edikt rollback v0.6.0`. Your v0.5.x in-body sentinels come back; sidecars are left in the working tree as untracked files (delete or keep).

## Idempotency

Re-running `--apply` on an already-migrated project is a no-op. Half-completed migrations resume cleanly — if an LLM resync fails on artifact 8 of 14, fix the issue and re-run; the first 7 are no-ops, work resumes from 8.

## Natural language triggers

- "migrate sidecars"
- "upgrade my sidecars"
- "convert legacy ADRs"
- "run the v0.6.0 migration"

## Reference

- [Sidecar Migration walkthrough](/guides/sidecar-migration) — full flow with example output and edge cases
- [Upgrading to v0.6.0](/guides/v0.6.0-upgrade) — context for the mandatory migration
- ADR-027 — Sidecar architecture for governance metadata (supersedes ADR-008)
- ADR-028 — Two-phase compile (amends ADR-020)
- [`/edikt:upgrade`](/commands/upgrade) — runs this command automatically with prompt
- [`/edikt:gov:compile`](/commands/gov/compile) — refuses to run until migration is applied
