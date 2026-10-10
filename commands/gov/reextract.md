---
name: gov:reextract
description: "Deliberately regenerate the corpus (or named artifacts) through the locked sidecar-extractor, with a per-artifact accept/reject review before anything lands."
tier: 1
tier_2_dependency: edikt gov reextract
on_absent: refuse-and-direct-user
tier_2_dependency_2: edikt phasea plan
on_absent_2: refuse-and-direct-user
tier_2_dependency_3: edikt phasea restore
on_absent_3: refuse-and-direct-user
tier_2_dependency_4: edikt phasea verify
on_absent_4: refuse-and-direct-user
allowed-tools:
  - Read
  - Bash
  - AskUserQuestion
  - Glob
  - Task
stops:
  - id: dispatch
    class: risk
    default: "no"
    on_silence: safe-side
    prompt: "Proceed with dispatching re-extraction for the artifacts listed above?"
  - id: accept-artifact
    class: risk
    default: "skip-decide-later"
    on_silence: safe-side
    prompt: "Accept this artifact's re-extraction, reject it, or decide later?"
---

# /edikt:gov:reextract

Re-runs the locked sidecar-extractor over ADRs/invariants/guidelines whose
sidecars predate the current extraction contract, then walks the human
through each changed sidecar for an explicit accept/reject before any of it
is considered final.

**Manual, opt-in only.** This command is never invoked automatically by
`/edikt:upgrade` or any other flow — `/edikt:upgrade` may *offer* to run it
(its own Step 7, gated on real `--status` numbers and an explicit yes to the
question it asks), but the offer is declined by default and dispatch only ever
happens on an explicit human yes, here or there. EXP-006 measured
3–8 minutes of
wall-clock per artifact (parallelizable, no observed degradation at 8-way
concurrency on its sample) and explicitly flagged that the cost at true
full-corpus scale (50+ artifacts) was never confirmed — "this is the number
that should gate default-on vs opt-in, not an estimate." Until that
confirmation exists, re-extraction stays something a user deliberately
runs, never something a project is upgraded into unasked.

**Why this is safe to run repeatedly.** The tier-2 binaries already:
- operate on the sidecar co-located next to its parent doc, never a
  stripped copy — this is what lets the extractor see and preserve
  `suppressed_directives`, `manual_directives`, and `# DORMANT` comments on
  a dormant artifact (EXP-006 Finding 2/3);
- restore (`edikt phasea restore`, step 4c) `manual_directives`,
  `suppressed_directives`, `human_approved_at`, `verify`, `verify_kind`, the
  fixture paths, and `paths:`/`paths_approval:` exactly as they stood, from
  the pre-extraction pin onto the regenerated sidecar, reporting anything
  they could not confidently re-attach rather than guessing;
- is resumable — a kill mid-batch costs only the unfinished artifacts, and
  a contract-version change starts a fresh batch rather than reporting old
  work as current.

What it does NOT do on its own is let a human review and accept or reject
each artifact's result before it's treated as final. That is this command's
job.

**One preserved field has a stated limit: `actor_scope`.** It is a bare
boolean and `true` is the pin — a `false` pin is permanently unrepresentable,
because the field carries `omitempty`, so `false` is dropped when the sidecar
is written and an absent key decodes back to `false`. The two states are the
same bytes on disk, so nothing downstream could tell them apart and no restore
can recover the difference. A directive that should NOT be actor-scoped is
therefore expressed by leaving the key out, never by writing `actor_scope:
false` and expecting that to survive. This is a decision (D319), not a defect:
the alternative was changing the Go field to a pointer, which is a wider change
than the preservation contract needed. If you need a false pin to mean
something, that is an ADR, not a bug report.

## Arguments

- `$ARGUMENTS` — optional, space-separated artifact IDs (e.g. `ADR-0037
  INV-0007`) to restrict the batch via `--only`. If omitted, the batch
  covers every eligible artifact in the corpus.

## Steps

### 1. Verify binary presence

```bash
command -v edikt >/dev/null 2>&1
```

If absent, print:

```
✗ edikt not found.
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
EDIKT_INVOKER=/edikt:gov:reextract edikt gov reextract --status --json
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

<!-- stop: dispatch -->
Ask via `AskUserQuestion`: "Proceed with dispatch?" (yes / no). Stop on
"no" with no further action. With no human to answer, take the safe side —
dispatch nothing, print the work-list, and say the dispatch was declined by
default rather than answered.

### 4. Plan, fan out, restore, verify

**The binary does not dispatch.** `edikt gov reextract` computes the
work-list and emits a plan; fanning extractors out means invoking an LLM,
which INV-012 forbids a tier-2 binary from doing (ADR-030, SPEC-013). You are
the dispatcher. This is the same Phase A contract `/edikt:gov:compile` runs.

**4a — plan.**

```bash
EDIKT_INVOKER=/edikt:gov:reextract edikt gov reextract --force --json \
  $( [ -n "$ARGUMENTS" ] && for id in $ARGUMENTS; do printf -- '--only %s ' "$id"; done )
```

No `--clean-tree` — the review baseline is the snapshot written in Step 2's
description, not git history. The snapshot is taken here, before anything is
handed off, because this is the last moment the on-disk bytes are still what
extraction is about to overwrite.

Capture the JSON result. `plan_id` names the plan; `dispatched` counts what
was HANDED OFF, not what completed. `succeeded` and `failed` are zero at this
point by construction — this process performed no extraction, and a success
count for work it did not do would be a measurement nobody made. They are
filled in by step 4d.

If it refuses instead (`re-extraction could not emit a dispatch plan: …`),
report the message verbatim and stop. The refusal that needs a decision is
an earlier plan for one of these artifacts that
was never closed (dispatched and not restored, mid-retry, or FAILED) while
its sidecar has moved since that plan pinned it. The message names the plan,
its state and what to do — finish its `phasea restore` and `phasea verify`,
or copy its pin back before re-running.
Planning over it would pin unrestored or failed output as the new baseline. An
artifact whose last plan is already closed (consumed or FAILED) is planned
under a successor plan id, never the closed one's.

Read `.edikt/state/phasea/<plan_id>.plan.json` and validate it is well-formed
JSON with its four required top-level keys before acting on any part of it.

**4b — fan out, one subagent per item.**

For each entry in `items[]`, spawn **exactly one** `sidecar-extractor`
subagent via `Task`, with the prompt composed from **only** that item's
`artifact_path` and `item_id`:

```
Task(subagent_type: "sidecar-extractor",
     prompt: "<item.artifact_path> <item.item_id>")
```

**No add, drop, merge, retry-with-changes, or reword.** Never combine two
items into one context (ADR-027), never dispatch an item absent from the
plan. That covers every first dispatch; a retry is the correction pass in 4d,
whose one permitted change is the receipt's `correction_prompt` prepended.
Continue on error — one failure does not stop the rest.

**A stall is any final response other than the extractor's one-line
result.** If a subagent's final response is
anything other than exactly one `SIDECAR WRITTEN: <path>` line — including
an empty response, the stream watchdog, a kill or a crash — note its
`item_id` for `--stalled` in 4d.
Reporting an item that did write costs nothing: verify judges a written output
on its bytes and ignores the report.

**The extractor's hand-back is not evidence.** Never relay what a subagent
says it did — its count of entries or of `needs_review`, or what it kept. In
particular a hand-back may say a sidecar's scope was lost, or that it needs
re-approval with `edikt sidecar approve --kind paths`. Restore owns `paths:`:
it writes `paths:` and `paths_approval:` back exactly as they stood before the
dispatch, labels globs that no approval receipt covers as unreceipted (never as
approved), and drops a `paths:` the extractor wrote on a sidecar that had none.
A hand-back saying scope was lost or needs re-approval
is wrong — do not act on it.

**4c — restore and record.**

```bash
edikt phasea restore --plan <plan_id>
```

This reinstates human-pinned state the extraction does not carry forward
(including `paths:` as above), quarantines any regenerated sidecar that fails
the content gate, records the ADR-053 body-drift baseline, and writes one
ledger entry per artifact as it finishes — so a run killed here loses only the
work still outstanding.

It is also where "did the extractor write anything" is measured: a sidecar
byte-identical to the plan's pre-dispatch pin (or, on a retry round, to what
the previous restore left) is left exactly as found, listed as `unwritten` in
the restore receipt and ledgered failed, never done. 4d grades it
`output-missing` (`stalled` under `--stalled`) with the diagnostic
`restore observed no write since plan <plan_id>`. Run restore again after
every re-dispatch round.

**4d — verify, and act on the exit code.**

```bash
edikt phasea verify --plan <plan_id>                    # or, for the stalls 4b noted:
edikt phasea verify --plan <plan_id> --stalled <item_id>[,<item_id>...]
```

**Check whether `.edikt/state/phasea/<plan_id>.receipt.json` exists BEFORE
reading the exit code.** No receipt means the run refused before evaluating
anything: report a pre-run refusal, never "items failed". A missing receipt
and a receipt full of failures are different events.

| Exit | Meaning | What you do |
|------|---------|-------------|
| 0 | every item ok | continue to Step 5 |
| 1 | some items failed | re-dispatch each receipt item with `attempts < phasea.max_attempts_per_item` as below, then repeat 4c and 4d |
| 2 | plan rejected (unknown, malformed, already consumed, or FAILED) | abort; report verbatim; never retry |
| 3 | receipt unwritable | abort; report verbatim; never retry |
| 5 | failed-terminal | **stop and report the failing items. Do NOT retry.** The retry budget is spent |
| 6 | diverged | **stop and report. Do NOT retry** — the item got worse than its previous attempt |
| 7 | restore has not run | run 4c, then 4d again |

Exit 1 and exit 5 differ only in whether anything is still owed an attempt.
Treating 5 as 1 is an unbounded loop that looks like progress.

On exit 1, each receipt item decides its own re-dispatch — the same
correction pass `/edikt:gov:compile` runs (its § A4-bis):

- `output-missing` or `stalled` — no output to correct; re-dispatch fresh
  with the plain 4b prompt.
- a status with a `correction_prompt` — `anchor-invalid`, `length-exceeded`,
  a quarantined write (classified from its quarantine copy), or
  `lossless-loss` — prepend the `correction_prompt` verbatim to the 4b prompt;
  nothing else changes.
- any other status with an empty `correction_prompt` — report it; do not
  re-dispatch.

`lossless-loss` is the guard on this command's whole purpose: the output
dropped a directive or prohibition body whose source prose is still in the
artifact. Its `correction_prompt` names each lost body and its line range.
The remedies are that correction pass; restoring the lost entries by hand and
re-running; or, only on the user's explicit say,
`edikt phasea verify --plan <plan_id> --on-loss=accept`, which passes the item
with the loss recorded. An `ok` item can carry a diagnostic too — a prior body
matched only by the weaker lexical fallback, or `source-removed` because its
prose is gone from the artifact. Quote it in Step 5; it is not a failure.

**Report results from the receipt, never from the extractor's hand-back:**
each item's `status`, `anchors_valid`, `anchors_whitespace_only` and
`needs_review` (an absent count is zero), and the verify summary's
`needs_review: K (counted on N sidecar(s))`.

If artifacts failed, they remain pending in the ledger and nothing was
written for them; this is not a partial-corruption state.

If `load_failed_ids` is non-empty: for each, the PRIOR sidecar existed but
failed to parse — a categorically different case from an artifact that had no
prior sidecar at all. Nothing could be checked for pinned state on that
artifact, because the file that would have carried it could not be read.
Treat it with more weight in Step 6, not folded silently into the ordinary
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
   <!-- stop: accept-artifact -->
3. Ask via `AskUserQuestion`: "Accept this artifact's re-extraction?"
   (accept / reject / skip-decide-later). With no human to answer, take the
   safe side — `skip-decide-later`: the live file and its snapshot both stay
   where they are, and the artifact is reported as decided-by-default, not
   accepted.
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
needs `edikt sidecar approve`. An unrestorable `paths_approval` is a receipt
whose hash no longer covers the globs restore carried back; it has no pending
proposal behind it, so `edikt sidecar approve --kind paths` has nothing to
approve (it exits 2, pending-id not found). Compare the globs with the
pre-extraction snapshot (`.edikt/state/reextract-snapshots/<id>.edikt.yaml`)
or git history and put back the pair that matches; a genuinely new scope
enters only as `proposed_paths`, through compile, and is approved from the
pending proposal it creates. Do not attempt any of these automatically — an
unrestorable pin was left unrestorable because attaching it to a guess would
be worse than surfacing it.

## Completion

End with `✅ re-extraction reviewed: <accepted> accepted, <rejected>
rejected, <unresolved> unresolved` plus the unrestorable-pin count if any.
If anything was accepted, remind the user: `git diff` still shows the
accepted changes uncommitted — review and commit when ready. If anything
is unresolved, name it explicitly and say re-running `/edikt:gov:reextract`
will pick up where this run left off (the ledger already tracks it).

Next: after committing, run `/edikt:gov:compile` to bring the re-extracted
content into the compiled governance surfaces.
