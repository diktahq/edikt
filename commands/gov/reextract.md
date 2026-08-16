---
name: gov:reextract
description: "Deliberately regenerate the corpus (or named artifacts) through the locked sidecar-extractor, with a per-artifact accept/reject review before anything lands."
tier: 1
tier_2_dependency: edikt gov reextract
on_absent: refuse-and-direct-user
allowed-tools:
  - Read
  - Bash
  - AskUserQuestion
  - Glob
---

# /edikt:gov:reextract

Re-runs the locked sidecar-extractor over ADRs/invariants/guidelines whose
sidecars predate the current extraction contract, then walks the human
through each changed sidecar for an explicit accept/reject before any of it
is considered final.

**Manual, opt-in only.** This command is never invoked automatically by
`/edikt:upgrade` or any other flow — `/edikt:upgrade` may *offer* to run it
(its own Step 7, gated on real `--status` numbers and an explicit
`AskUserQuestion` yes), but the offer is declined by default and dispatch
only ever happens on an explicit human yes, here or there. EXP-006 measured
3–8 minutes of
wall-clock per artifact (parallelizable, no observed degradation at 8-way
concurrency on its sample) and explicitly flagged that the cost at true
full-corpus scale (50+ artifacts) was never confirmed — "this is the number
that should gate default-on vs opt-in, not an estimate." Until that
confirmation exists, re-extraction stays something a user deliberately
runs, never something a project is upgraded into unasked.

**Why this is safe to run repeatedly.** The tier-2 binary (`bin/edikt gov
reextract`) already:
- operates on the sidecar co-located next to its parent doc, never a
  stripped copy — this is what lets the extractor see and preserve
  `suppressed_directives`, `manual_directives`, and `# DORMANT` comments on
  a dormant artifact (EXP-006 Finding 2/3);
- restores `human_approved_at`, `verify`, `verify_kind`, the fixture paths,
  and approved `paths:`/`paths_approval` from the pre-extraction sidecar
  onto the regenerated one (`internal/reextract/preserve.go`), reporting
  anything it could not confidently re-attach rather than guessing;
- is resumable — a kill mid-batch costs only the unfinished artifacts, and
  a contract-version change starts a fresh batch rather than reporting old
  work as current.

What it does NOT do on its own is let a human review and accept or reject
each artifact's result before it's treated as final. That is this command's
job.

## Arguments

- `$ARGUMENTS` — optional, space-separated artifact IDs (e.g. `ADR-0037
  INV-0007`) to restrict the batch via `--only`. If omitted, the batch
  covers every eligible artifact in the corpus.

## Steps

### 1. Verify binary presence

```bash
command -v bin/edikt >/dev/null 2>&1 || command -v edikt >/dev/null 2>&1
```

If absent, print:

```
✗ bin/edikt not found.
  This command requires the edikt tier-2 helper. Install via:
    edikt install edikt
  Then re-run /edikt:gov:reextract.
```

Stop. Do not proceed. (Frontmatter `on_absent: refuse-and-direct-user`.)

### 2. No working-tree precondition — the review no longer needs one

Earlier versions of this command required a clean working tree, because the
per-artifact review read `git diff` to show what changed, and that only
works starting from a clean baseline. That was a review-baseline choice,
not a correctness or safety property of re-extraction itself
(DESIGN-QUESTIONS-2026-08-16.md Q3).

The dispatcher now snapshots each artifact's pre-rewrite sidecar bytes to
`.edikt/state/reextract-snapshots/<artifact-id>.edikt.yaml` before
overwriting it — a git-independent review baseline. Step 5 diffs against
the snapshot, not against git history, so a dirty working tree (unrelated
uncommitted work elsewhere in the project) no longer blocks this command.

Do not check `git status` here. Do not pass `--clean-tree` in Step 4.

### 3. Show current batch status and get confirmation

```bash
bin/edikt gov reextract --status --json
```

Render the eligible/done/remaining/failed counts to the user. Then state
the cost plainly, scaled to what `--status` reported:

```
Re-extraction will dispatch the sidecar-extractor for <N remaining>
artifact(s). EXP-006 measured 3–8 minutes per artifact; expect roughly
<N * ~4 minutes serially, less in parallel> for this batch. Each artifact's
result will be shown to you individually afterward — nothing is final
until you accept it.
```

Ask via `AskUserQuestion`: "Proceed with dispatch?" (yes / no). Stop on
"no" with no further action.

### 4. Dispatch

```bash
bin/edikt gov reextract --force --json \
  $( [ -n "$ARGUMENTS" ] && for id in $ARGUMENTS; do printf -- '--only %s ' "$id"; done )
```

No `--clean-tree` — the review baseline is the snapshot written in Step 2's
description, not git history.

Capture the JSON result (`eligible`, `dispatched`, `succeeded`, `failed`,
`pins_restored`, `unrestorable`, `load_failed_ids`). If `failed > 0`, list
which artifact IDs failed (from `--status` afterward) — they remain pending
in the ledger and nothing was written for them; this is not a
partial-corruption state.

If `load_failed_ids` is non-empty: for each, the PRIOR sidecar existed but
failed to parse before dispatch — a categorically different case from an
artifact that had no prior sidecar at all. The regenerated sidecar was still
written (extraction itself succeeded), but nothing could be checked for
pinned state (approved paths, verify, verify_kind, human_approved_at, or
either fixture path) on that artifact, because the file that would have
carried it couldn't be read. This is not a normal "unresolved pin" — treat
it with more weight in Step 6 below, not folded silently into the ordinary
per-field list.

### 5. Per-artifact review

```bash
find .edikt/state/reextract-snapshots -name '*.edikt.yaml' 2>/dev/null
```

Each file here is `<artifact-id>.edikt.yaml`, a pre-rewrite snapshot. This
list is self-cleaning (see step 3/4 below), so what it contains is exactly
what still needs a decision — including anything left unresolved
(skip-decide-later) from a previous run.

For each snapshot file, in turn:

1. Derive the live sidecar's path from the snapshot's artifact ID (same
   resolution the corpus discovery already uses — `docs/architecture/
   decisions/<id>-*.edikt.yaml`, `docs/architecture/invariants/<id>-*.edikt.yaml`,
   or `docs/guidelines/<id>-*.edikt.yaml` depending on prefix).
2. Show the difference: `diff -u .edikt/state/reextract-snapshots/<id>.edikt.yaml <live-sidecar-path>`
   (or `git diff --no-index` for nicer coloring, if `git` is available — both
   work with no repo-clean precondition, since `--no-index` diffs two
   arbitrary files).
3. Ask via `AskUserQuestion`: "Accept this artifact's re-extraction?"
   (accept / reject / skip-decide-later)
4. **accept** — leave the live file as re-extracted, then delete the
   snapshot: `rm .edikt/state/reextract-snapshots/<id>.edikt.yaml`. Deleting
   on resolution is what keeps step 5's listing accurate on a future run —
   a snapshot that lingers after acceptance would show a resolved artifact
   as still-pending forever.
5. **reject** — restore the live file from the snapshot, then delete the
   snapshot:
   ```bash
   cp .edikt/state/reextract-snapshots/<id>.edikt.yaml <live-sidecar-path>
   rm .edikt/state/reextract-snapshots/<id>.edikt.yaml
   ```
   This is sufficient by itself: the reextract ledger records each
   artifact's sidecar hash at completion, and a file that no longer
   matches that hash is treated as edited-since-dispatch on the next
   `gov reextract` invocation — the same artifact will be re-offered for
   re-extraction later rather than silently staying stale forever.
6. **skip-decide-later** — leave both the live file and the snapshot in
   place, unresolved. Note it in the final summary as still needing a
   decision. Re-running `/edikt:gov:reextract` later will surface it again
   via step 5's listing without re-dispatching it (the snapshot already
   exists; nothing re-triggers extraction for an artifact that's merely
   pending review).

### 6. Surface unrestorable pins

**First, artifacts in `load_failed_ids` — these are not ordinary unrestorable
pins and must be called out on their own, before the per-field list below:**

```
🛑 N artifact(s) had a prior sidecar that FAILED TO LOAD before re-extraction
   ran — not "no prior sidecar", a real one that could not be parsed:
     <artifact_id>: <reason the load failed>
   The regenerated sidecar was written, but paths, verify, verify_kind,
   human_approved_at, and both fixture-path pins could not be checked —
   any of them may have been silently at risk before this run. Recover the
   prior content manually (git history for the sidecar path, or check
   whether a `.edikt/state/reextract-snapshots/<id>.edikt.yaml` for it
   still exists) and compare by hand before treating this artifact's
   re-extraction as final. This is a heavier review than a normal
   unrestorable pin — do not wave it through with the same accept flow
   Step 5 uses for a clean diff.
```

**Then, for the remaining `unrestorable` list** (any entry whose artifact is
NOT in `load_failed_ids` — the ordinary case), render it distinctly from the
per-artifact accept/reject above — these need a human re-approval decision
regardless of whether their artifact was accepted:

```
⚠ N pinned value(s) could not be carried forward automatically:
  <artifact_id> <field> on "<directive_text truncated>": <reason>
```

For each, state the remedy directly: a `human_approved_at`/`verify` pin
needs `bin/edikt sidecar approve`; a `paths_approval` pin needs
`bin/edikt sidecar approve --kind paths`. Do not attempt either
automatically — an unrestorable pin was left unrestorable because
attaching it to a guess would be worse than surfacing it.

## Completion

End with `✅ re-extraction reviewed: <accepted> accepted, <rejected>
rejected, <unresolved> unresolved` plus the unrestorable-pin count if any.
If anything was accepted, remind the user: `git diff` still shows the
accepted changes uncommitted — review and commit when ready. If anything
is unresolved, name it explicitly and say re-running `/edikt:gov:reextract`
will pick up where this run left off (the ledger already tracks it).

Next: after committing, run `/edikt:gov:compile` to bring the re-extracted
content into the compiled governance surfaces.
