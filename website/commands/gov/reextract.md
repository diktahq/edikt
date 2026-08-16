# /edikt:gov:reextract

Deliberately regenerates the corpus — or a named subset — through the locked sidecar-extractor, then walks you through every changed sidecar for an explicit accept/reject before any of it is considered final.

**Manual, opt-in only.** This command is never invoked automatically by [`/edikt:upgrade`](/commands/upgrade) or any other flow. Re-extraction runs only when you deliberately run it.

## Why this exists

[`/edikt:gov:compile`](/commands/gov/compile)'s Phase A resyncs a sidecar when it's *stale* — when the SHA-256 of its parent document no longer matches the hash the sidecar was generated from. That mechanism has no way to detect a different kind of drift: when the *extractor itself* changes (a new prompt version, better grounding rules, a wider turn budget) but every existing sidecar's anchors still match their source prose. Nothing looks stale, so nothing resyncs — even though every sidecar in the corpus was written by a contract that no longer applies.

`/edikt:gov:reextract` is the deliberate forcing mechanism for that case. It re-runs the current extractor over the corpus regardless of staleness, then puts every resulting change in front of you before it lands.

## Usage

```bash
/edikt:gov:reextract
/edikt:gov:reextract ADR-0037 INV-0007
```

## Arguments

| Argument | Description |
|----------|--------------|
| (none) | Re-extract every eligible artifact in the corpus |
| `ADR-NNN INV-NNN ...` | Space-separated artifact IDs — restricts the batch to just these, via `--only` |

## Preconditions

### Clean working tree required

Re-extraction rewrites sidecars in place, and the per-artifact review step reads `git diff` to show you what changed. That only works from a known baseline. If `git status --porcelain` reports anything, the command stops and asks you to commit or stash first — it will not stash on your behalf.

### Cost estimate before you commit

Before dispatching, the command shows the current batch status (eligible / done / remaining / failed counts) and states the cost plainly, derived from EXP-006's measurement: **3–8 minutes per artifact**, median ~4.3 minutes, roughly tracking prose length rather than old directive count. You're asked to confirm before anything is dispatched.

EXP-006 measured this on an 8-artifact sample run concurrently with no observed degradation — it explicitly states that cost at true full-corpus scale (50, then 100+ sidecars) was never confirmed, and that this is "the number that should gate default-on vs opt-in, not an estimate." That's the reason this command stays opt-in rather than something a project gets upgraded into.

## What it does

1. **Verifies `bin/edikt` is present.** If absent, refuses and directs you to install it (`tier_2_dependency: edikt gov reextract`, `on_absent: refuse-and-direct-user`).
2. **Requires a clean working tree** (see above).
3. **Shows batch status and the cost estimate**, then asks for confirmation.
4. **Dispatches** `bin/edikt gov reextract --force --clean-tree`, restricted to `--only <id>` artifacts if you named any.
5. **Walks every changed sidecar for review** — one at a time, `git diff` shown, accept or reject:
   - **Accept** — the re-extracted file stays as-is. No action needed.
   - **Reject** — `git checkout -- <file>` reverts just that one file. This is sufficient by itself: the reextract ledger records each artifact's sidecar hash at dispatch time, and a reverted file no longer matches that hash — so it reads as edited-since-dispatch and gets naturally re-offered on a future run, with no separate rollback bookkeeping needed.
   - **Skip / decide later** — leaves the change in place, unresolved, and named in the final summary. Nothing is committed until every artifact has an explicit decision.
6. **Surfaces unrestorable pins separately** from the accept/reject choice (see below).

## Pin preservation

The extractor is a locked prompt that never sees the sidecar it's about to replace — deliberately, so it can't reproduce a past mistake forever. The consequence is that a raw re-extraction would wipe anything a human pinned onto the previous sidecar: approved `paths:`, `paths_approval:`, per-directive `verify` / `verify_kind`, `human_approved_at`, and fixture paths.

`bin/edikt gov reextract` restores these automatically before a sidecar is recorded as done:

- **Top-level `paths:` and `paths_approval:`** restore verbatim — they belong to the artifact, not to any one directive.
- **Per-directive pins** (`verify`, `verify_kind`, `human_approved_at`, fixture paths) are matched back to their directive by exact text first, then by a normalized noun-phrase match — but only when that match is *unambiguous* (exactly one candidate).
- **Anything that can't be confidently reattached is reported, never guessed.** Attaching an approved `verify` command to the wrong directive would be worse than losing it: the sidecar would then claim a human approved something they never actually saw.

Unrestorable pins appear in their own section of the command's output, each naming its concrete remedy — a `human_approved_at` / `verify` pin needs `bin/edikt sidecar approve`; a `paths_approval` pin needs `bin/edikt sidecar approve --kind paths`. Neither is applied automatically.

## Completion

```text
✅ re-extraction reviewed: 6 accepted, 1 rejected, 2 unresolved
⚠ 1 pinned value could not be carried forward automatically
```

If anything was accepted, the changes are still uncommitted — `git diff` shows them; review and commit when ready. If anything is unresolved, re-running `/edikt:gov:reextract` picks up where this run left off — the ledger already tracks per-artifact completion.

## Not to be confused with `/edikt:gov:advisory-review`

These two commands sound similar and do opposite things:

- **`/edikt:gov:reextract` writes.** It re-runs extraction against the current extractor contract and, on accept, mutates the sidecar on disk. This is how you actually fix a stale or under-extracted corpus.
- **[`/edikt:gov:advisory-review`](/commands/gov/advisory-review) is read-only.** It judges whether existing compiled sidecars *look* stale — via a forked judge — and only ever suggests. It never writes, regenerates, or touches a sidecar.

If you want to know cheaply and non-destructively whether your corpus looks stale, run `/edikt:gov:advisory-review` first. If you've decided to actually fix it, run `/edikt:gov:reextract`.

## When to run

- After an extractor contract change (a new `sidecar-extractor` prompt version) that you want reflected in existing sidecars, not just new ones
- After `/edikt:gov:advisory-review` flags the corpus as looking stale and you want to act on it
- Never as part of routine upgrades — this is a deliberate, occasional operation, not a step in `/edikt:upgrade`

## What's next

- [/edikt:gov:compile](/commands/gov/compile) — bring the re-extracted content into the compiled governance surfaces after committing
- [/edikt:gov:advisory-review](/commands/gov/advisory-review) — check for staleness without writing anything
- [/edikt:gov:review](/commands/gov/review) — review the language quality of the re-extracted directives
