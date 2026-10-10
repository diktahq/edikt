---
name: migrate
description: "Lift legacy in-body sentinel blocks into sidecars, verify nothing was dropped, recompile — the whole migration procedure, in one place"
tier: 1
tier_2_dependency: edikt migrate
on_absent: refuse-and-direct-user
allowed-tools:
  - Read
  - Write
  - Bash
  - Task
  - Glob
  - Grep
stops:
  - id: apply-migration
    class: risk
    default: "no"
    on_silence: safe-side
    prompt: "Apply this migration plan now? (the lift is destructive on the prose body)"
---

# /edikt:migrate

Migration is state-based and per artifact: an artifact needs migrating when its `.md` still carries an in-body `[edikt:directives:start]: # … [edikt:directives:end]: #` block, or when the sidecar beside it is still `schema_version: 1`. Nothing else — not the project's pinned version, not the launcher's history — decides it. Every steady-state command (compile, doctor, init, upgrade, the extractor) is migration-blind: it refuses or hands off to this command, and this command carries the whole lift → verify → recompile procedure, the recovery path, the dry-run gate and the exit-code contract.

`/edikt:upgrade` runs the scanner and, when it returns files, prints one screen and points here. Nothing in upgrade applies a migration.

## Arguments

- none — the plan comes from the scanner
- `--manifest <path>` — regression-manifest mode (Step 5) against an existing strict report

## The two-phase model

Migration is a CLEANUP, not an extraction. Phase A is deterministic and pure Go; Phase B needs a live model.

**Phase A — the lift (`edikt migrate lift --apply`, pure Go, no LLM).** It does one of two things per flagged artifact. An artifact whose `.md` still carries an in-body block has the block stripped and a `<artifact>.edikt.yaml` written in the current `gov-sidecar.v2` shape (multi-anchor `source_excerpts[]`); the block's contents — directives, `manual_directives`, `suppressed_directives`, reminders, verification, topic and signal hints — are carried **verbatim** under a transient `migration_preserved:` field on the new sidecar. An existing v1-shaped sidecar is converted to v2 in the same pass, in place and losslessly: it keeps its `directives` and gets no `migration_preserved`. There is no separate schema step. `migrate sidecars` is accepted as a deprecated alias of `migrate lift` for one release.

**This phase alone leaves the project with ZERO enforceable directives from every artifact that carried an in-body block.** The sidecar the lift writes has `directives: []` only for an artifact whose `.md` carried an in-body block; its old content sits only in `migration_preserved`, which nothing compiles or enforces. A project that stops here has silently disabled that governance, and the lift exits 0 with a success line when it does. Phase B is not an optional follow-up: this command is incomplete until it has run. A converted v1 sidecar stays enforceable, but Phase B may still re-extract it (planned as `stale-quote-drift` when its anchors drifted), with no migration baseline — `phasea verify`'s survival check is its loss guard (Step 3d).

**Phase B — the recompile (`/edikt:gov:compile`, dispatches `sidecar-extractor`).** Compile's Phase A treats every `migration_preserved`-bearing sidecar as stale and dispatches the extractor against it; the extractor takes the preserved lists as the canonical baseline (the "On migration-preserved baselines" rules), anchors each entry to real prose, and writes the final sidecar with `migration_preserved` gone. `phasea verify`'s lossless-check refuses an output that dropped a preserved directive (`lossless-loss`, retryable; `--on-loss=accept` records the loss instead, only on an explicit decision). Steady-state sidecars never carry the transient field.

**Phase B needs a live model.** Inside a Claude Code session — including a headless `claude -p "/edikt:migrate"` — the session dispatches the extractor and this just works. Run as the raw `edikt gov compile` from a shell with no session, the binary never dispatches a model itself: it emits the dispatch plan and refuses (`phase A: N artifact(s) await tier-1 extraction (plan <path>)`), or — in a project where `/edikt:init` never installed `.claude/agents/sidecar-extractor.md` — refuses to emit a plan at all and names the paths it looked at; either way `migration_preserved` stays untouched and recoverable until a session runs the plan. There is no way to complete Phase B without a model; that is a limit of the flow, not a bug.

## Steps

### 0. The binary

```bash
EDIKT_BIN="${EDIKT_BIN:-edikt}"   # the launcher, by name, on PATH
command -v "$EDIKT_BIN" >/dev/null 2>&1 || command -v edikt >/dev/null 2>&1 || EDIKT_BIN=""
```

If no binary resolves, print and stop (exit 1) — nothing here can be done by hand:

```
✗ edikt not found.
  Install edikt (install.sh) or activate a version, then re-run /edikt:migrate.
```

### 1. The plan — one dry-run answers both questions, and its exit code is the control

`migrate lift --dry-run` plans BOTH halves in one pass: artifacts that still carry an in-body block and need a sidecar, and sidecars still at schema_version 1 that need converting. Show its output verbatim; decide only from the exit code (exit codes are the contract; prose is shown, never parsed).

```bash
# edikt-test-block: migrate-plan-check
# Input: EDIKT_BIN. Output: PLAN_CHECKED (1 = the dry-run ran; 0 = it did not, RED).
PLAN_CHECKED=0
PLAN="$(mktemp)"
EDIKT_INVOKER=/edikt:migrate "$EDIKT_BIN" migrate lift --dry-run > "$PLAN" 2>&1
PLAN_RC=$?
cat "$PLAN"
if [ "$PLAN_RC" -eq 0 ]; then
  PLAN_CHECKED=1
else
  echo "🔴 the migration plan was not produced: '$EDIKT_BIN migrate lift --dry-run' exited $PLAN_RC — rebuild or upgrade the binary (one without 'migrate lift' predates the lifter)"
fi
```

- `PLAN_CHECKED=0` — nothing was measured. Stop with exit 1; the remedy is the binary, not the project. Never print "nothing to migrate".
- `PLAN_CHECKED=1` — the plan above is what Step 2 applies. Do not decide from its counts: a plan that lists nothing is applied as a no-op in the same idempotent pass (a second `--apply` is byte-identical), so the offer is always safe to make and the user answers from what they read.

**The 24-hour dry-run gate.** The dry-run writes `.edikt/state/migration-dry-run.json` (timestamp, artifacts inspected). `--apply` refuses unless a dry-run for the same artifacts ran within 24 hours — the strip is destructive on the prose body, and the window is what makes "review the plan, then apply" the only path. `--force` bypasses it; that is an escape hatch for tests, not a workflow.

### 2. Apply

Show the plan, then ask — this is a risk confirmation, and its safe side is "no":

<!-- stop: apply-migration -->
```
The dry-run above is the migration plan: in-body sentinel blocks become
co-located *.edikt.yaml sidecars, and any v1-shaped sidecar is converted to v2.

Apply this plan now? [y/N]
```

**With no human to answer** (a headless run, CI, a background agent), fall
through to deferred — never auto-apply a destructive lift without an
interactive confirmation. Print the deferred block below and exit 2, and say
the answer was taken by default rather than given.

**On `y` / `yes`:**

```bash
EDIKT_INVOKER=/edikt:migrate "$EDIKT_BIN" migrate lift --apply
```

- exit 0 — Phase A is done. The lift's last step re-scans every migrated `.md` and fails the run with a per-file list if any column-0 `[edikt:directives:start]: #` survived the strip; seeing none is the proof. Print:
  ```
  ✅ Lift applied (Phase A): blocks stripped into v2 sidecars with migration_preserved; v1 sidecars converted to v2 in place.
     Every artifact whose block was stripped has ZERO enforceable directives until Phase B runs. Running Phase B now.
  ```
  Continue to Step 3.
- non-zero — print the lift's output verbatim and stop with exit 1. Recovery is from version control: the strip and the sidecar write are one atomic step per artifact, so a failed run leaves each artifact either untouched or fully lifted; `git checkout -- <artifact>.md` restores a block, and re-running the lift is idempotent.

**On `N` / `no` / empty:**

```
Migration deferred. Nothing was changed.
Re-run /edikt:migrate when ready. Until then, /edikt:gov:compile refuses per artifact on the legacy blocks it finds, and doctor reports them.
```

Exit 2.

### 3. Phase B — the recompile, with the migrate-time suffix

The extractor template is migration-blind: it carries no rule about `migration_preserved`. The rules travel with THIS command, appended to the dispatch of a lifted sidecar only — a steady-state extraction never sees them. So this command runs the Phase A fan-out itself, the same plan/receipt contract `/edikt:gov:compile` uses, and adds one thing: the suffix below on every item whose plan `reason` is `stale-migration-preserved`.

**Step 3a — plan.** `edikt phasea plan`. The exit code is the contract:

**Exit codes — the whole `phasea` group, one table.** The same number means
different things per verb, and a caller that assumes otherwise retries a
refusal or accepts a loss. A normal outcome prints once on stdout and is never
prefixed "error:"; a failure prints once, on stderr.

| Verb | 0 | 1 | 2 | 3 | 4 | 5 | 6 |
|---|---|---|---|---|---|---|---|
| `plan` | a plan was written (path printed) | nothing to plan — the corpus is current; **normal**, not a failure | usage/internal error | — | refused: ceiling, unresolvable root, the extractor contract unresolved, a terminal plan id, or an unclosed earlier plan | — | — |
| `restore` | pinned state reinstated | — | the plan was rejected (id mismatch, consumed, FAILED, unreadable) | the plan is malformed | — | — | — |
| `verify` | every item passed; the plan is consumed | items incomplete — retryable, re-dispatch and verify again | the plan was rejected | the plan is malformed | — | an item failed terminally — do NOT re-dispatch against this plan | an item diverged — worse than the previous attempt; stop |
| `verify` (cont.) | | | | | | | **7 — restore has not run for this plan.** The pinned state the fan-out overwrote (approved paths, approval receipts, human-approved verifies, suppressions) was never reinstated, so every item result would describe a corpus missing what a human decided. Run `edikt phasea restore --plan <id>`; verify refuses rather than restoring, because a verify that mutates what it judges cannot be re-run to check the mutation. |

Exit 1 here means nothing to extract — skip to Step 3e. Exit 4 is a refusal: report the message verbatim and stop. Two of them name an earlier plan. Either the plan this corpus state derives was already consumed or failed — do not re-request a lifted artifact by name to get past it, because a requested item plans as `requested`, not `stale-migration-preserved`, and would be dispatched without the suffix and without the lossless proof. Or an earlier plan for an item was never closed (dispatched and not restored, mid-retry, or FAILED) while its sidecar has moved since that plan pinned it; the message names the plan, its state and what to do (finish its restore and verify, or copy its pin back), because planning over it would pin unrestored or failed output as the new baseline. Read `.edikt/state/phasea/<plan_id>.plan.json` and validate it carries `schema_version`, `plan_id`, `created_from`, `budget` and `items` before acting on any part of it.

**Step 3b — fan out, at most `budget.max_concurrency` in flight (2 — a 3-wide fan-out stalls dispatches on the stream watchdog and drops artifacts without a failure line).** For each entry in `items[]`, spawn exactly one `sidecar-extractor` subagent via `Task`. The prompt is the item's `artifact_path` and `item_id` — and, for an item whose `reason` is `stale-migration-preserved`, the suffix appended after a blank line:

```
Task(subagent_type: "sidecar-extractor",
     prompt: "<item.artifact_path> <item.item_id>")                       # reason: missing | stale-quote-drift

Task(subagent_type: "sidecar-extractor",
     prompt: "<item.artifact_path> <item.item_id>\n\n<SUFFIX below>")   # reason: stale-migration-preserved
```

No add, drop, merge or reword of items; one fresh context per artifact; continue on error — `phasea verify` decides what landed. The suffix follows the plan reason on every dispatch of an item — the first and every retry — and never rides on any other item.

**A stall is any final response other than the extractor's one-line result.** If a subagent's final response is anything other than exactly one `SIDECAR WRITTEN: <path>` line — including an empty response, the stream watchdog, a kill or a crash — report its `item_id` to Step 3d with `--stalled`. Reporting an item that did write costs nothing: verify judges a written output on its bytes and ignores the report.

**The extractor's hand-back is not evidence.** Never relay what a subagent says it did — its count of entries or of `needs_review`, or what it carried over from the baseline. In particular a hand-back may say a sidecar's scope was lost, or that it needs re-approval with `edikt sidecar approve --kind paths`. Restore owns `paths:`: it writes `paths:` and `paths_approval:` back exactly as they stood before the dispatch, labels globs that no approval receipt covers as unreceipted (never as approved), and drops a `paths:` the extractor wrote on a sidecar that had none. A hand-back saying scope was lost or needs re-approval is wrong — do not act on it.

**Step 3c — restore pinned state, and it is NOT optional.** `edikt phasea restore --plan <plan_id>` reinstates the approved fields from the pre-dispatch snapshots and writes `.edikt/state/phasea/<plan_id>.restore.json`. Step 3d refuses without that receipt (exit 7): the fan-out overwrote every sidecar, and verifying before the pinned state is back measures a corpus missing everything a human decided — approved paths, approval receipts, human-approved verifies, suppressions. `gov compile` documents suppression as preserved across regenerations, and this step is that preservation. Restore is also where "did the extractor write anything" is measured: a sidecar byte-identical to the pre-dispatch pin is left as found and listed as `unwritten`, and Step 3d grades it `output-missing` (`stalled` under `--stalled`) with the diagnostic `restore observed no write since plan <plan_id>`. Run restore again after every re-dispatch round. Exit 2 is a rejected plan (unknown id, consumed, FAILED, or unreadable): report it and stop.

**Step 3d — verify, with the lossless proof.** `edikt phasea verify --plan <plan_id> [--stalled <ids>]`. Read the receipt's existence before the exit code: no `.edikt/state/phasea/<plan_id>.receipt.json` is a pre-run refusal, never "items failed". For a lifted item the verifier compares the pre-dispatch snapshot's `migration_preserved.directives` against the extractor's output — the baseline is no longer on disk, the extractor was told not to write it back — and records `lossless-loss` (a preserved directive dropped or rephrased; retryable) or `lossless-unmeasured` (no baseline in the snapshot; not a pass). For any re-extracted item, lifted or not — a converted v1 sidecar planned as `stale-quote-drift`, say — the survival check also compares the pre-dispatch sidecar's directive and prohibition bodies with the output, and records `lossless-loss` when a body is gone while its source prose is still in the artifact. Either loss carries a `correction_prompt` naming what was lost. On a loss or `lossless-unmeasured`, this is a risk stop whose safe side is to keep asking:

```
The extractor's output for {item} lost {n} directive(s): {list}
  y — re-dispatch this item as a correction pass (retry)
  N — stop here; restore the entries by hand, then re-run /edikt:migrate
  accept — keep the output as is and record the loss (`phasea verify --on-loss=accept`)
```

On `y`, and on any other exit-1 item, the re-dispatch follows `/edikt:gov:compile`'s § A4-bis: the receipt's `correction_prompt` prepended verbatim, nothing else changed (an `output-missing` or `stalled` item has no output to correct and is re-dispatched fresh). A lifted item keeps the suffix; a non-lifted item is re-dispatched **without** the suffix — the migration rules describe a baseline its sidecar never had. Then Step 3c and 3d again.

Headless: never `accept` on silence; stop with exit 1 and the receipt naming the item. Exit 0 from verify consumes the plan.

**Report results from the receipt, never from the extractor's hand-back:** each item's `status`, `anchors_valid`, `anchors_whitespace_only` and `needs_review` (an absent count is zero), and the verify summary's `needs_review: K (counted on N sidecar(s))`.

**Step 3e — compile.** `edikt gov compile` — every sidecar is now canonical (no `migration_preserved` remains), so this is the deterministic Phase B merge that renders the surfaces.

**THE SUFFIX** — this block, verbatim, is what Step 3b appends for a lifted item:

<!-- edikt:migrate-suffix:start — appended by /edikt:migrate to the dispatch of a LIFTED sidecar only -->
## On migration-preserved baselines (appended by /edikt:migrate for a lifted sidecar)

The `<name>.edikt.yaml` beside this artifact was written by `edikt migrate lift` and carries a
`migration_preserved:` object: the artifact's previous governance state, lifted verbatim from its
legacy in-body block. Those lists are the **canonical baseline** for this extraction — the
user's prior governance that the migration explicitly chose to carry forward.

**Mandatory preservation rules — apply BEFORE doing any extraction from prose:**

1. **`migration_preserved.directives` → your output `directives`.** Each
   entry MUST appear in your output's `directives[]` with `text` matching
   verbatim. You MAY add new directives derived from prose for content
   the preserved list doesn't cover, but you MUST NOT drop, rephrase,
   re-order, or merge preserved entries. For each preserved entry,
   anchor a `source_excerpts` entry by locating the most relevant span
   in the parent `.md` body (search for the directive's noun phrase,
   anchor on the matching line range). If no anchor is findable, set
   `source_excerpts` to a single-element list
   `[{line_start: 1, line_end: 1, quote: "<verbatim
   text truncated to 200 chars>"}` — drift detection treats this
   default-fallback shape as "no anchor available" rather than stale.

2. **`migration_preserved.manual_directives` → your output `manual_directives`.**
   Copy each entry verbatim. These are user-authored overrides that the
   sidecar-extractor MUST NEVER touch — same contract as the
   non-migration steady-state extraction.

3. **`migration_preserved.suppressed_directives` → your output `suppressed_directives`.**
   Copy verbatim. Same never-touch contract.

4. **`migration_preserved.reminders` → your output `reminders`.** Copy
   verbatim. You MAY append additional reminders derived from
   `## Confirmation` (ADRs) or `## Enforcement` (INVs) sections if they
   cover items the preserved list doesn't already include.

5. **`migration_preserved.verification` → your output `verification`.** Same
   pattern: copy verbatim, MAY append from prose.

6. **`migration_preserved.topic` and `.signals`** are HINTS, not
   mandatory. If they exist and look reasonable for the current prose,
   prefer them. If the prose has clearly shifted away from those hints,
   synthesise fresh values per the standard extraction rules below.

7. **DO NOT include `migration_preserved:` in your output sidecar.** It
   is a transient field consumed by you and stripped by Phase B of
   compile. Your output is the canonical sidecar; `migration_preserved`
   is the input baseline only.

When applied, the lifted artifact's `directives`/`manual_directives`/`suppressed_directives`/
`reminders`/`verification` are at least as complete as the legacy block — the user never
silently loses governance state on migration. `edikt phasea verify` proves it against the
pre-dispatch snapshot and reports `lossless-loss` when it does not hold.
<!-- edikt:migrate-suffix:end -->

### 4. Grade the recompile — do not skip

Phase B just re-extracted every lifted artifact. Compile's regression check asks only whether the migration destroyed anything; it says nothing about how well the new sidecars capture the prose. Run `/edikt:gov:grade-compile` and print its line:

```
📊 Compile-quality grade: {overall}/10 ({dimension}: {score}/10, ...)
```

Advisory only — a low grade blocks nothing — but it must be visible, not merely available.

### 5. The regression manifest — when a lift was lossy

For a corpus whose earlier migration is suspected lossy, or after a loss in Step 3, strict mode records the losses as a machine-readable manifest:

```bash
EDIKT_INVOKER=/edikt:migrate "$EDIKT_BIN" migrate lift --strict --report-json .edikt/state/migration-strict-report.json
```

Exit 0 means no regressions (print `No regressions detected. Sidecars are current.` and stop); exit 1 means LOST or FACTUAL items, exit 2 DEGRADED. Read the manifest and group items by `category`:

- **LOST** (a directive, paths or scope dropped): auto-fix. Deduplicate by parent artifact and re-extract each unique artifact through its per-artifact compile command (`/edikt:adr:compile`, `/edikt:invariant:compile` or `/edikt:guideline:compile` with the artifact's path), which plans, dispatches, restores and verifies it — never a bare extractor dispatch. Then re-run the strict check; exit 0 means LOST is cleared, otherwise print the non-zero and its output verbatim.
- **FACTUAL** (modality drift) and **DEGRADED** (verification abstraction): a human decides. Append each to `docs/internal/migration-manual-review.md` as a checkbox line — `- [ ] {path} :: {category}.{field} :: expected={expected} actual={actual} :: line {start}-{end}` with the quote beneath — deduplicated by (path, category, field, expected).

Then:

```
Auto-regenerated {K} sidecars (LOST).
{M} items written to docs/internal/migration-manual-review.md (FACTUAL/DEGRADED — manual review required).
```

Status is `INCOMPLETE` while any worklist line is unchecked (exit 3), `COMPLETE` when none is or the file does not exist.

`--manifest <path>` skips the strict run and consumes an existing report.

## Exit codes

| exit | meaning |
|---|---|
| 0 | the plan was applied and Phase B completed; or there was nothing to lift and the corpus is current |
| 1 | refused: no binary, the plan could not be produced, the lift failed, or a sentinel survived the strip — nothing partial is left behind |
| 2 | deferred by the user (or headless with no one to answer); nothing changed |
| 3 | regressions remain in the manual-review worklist |

## Completion

End with `✅ Migration complete — {n} artifact(s) lifted, Phase B compiled, grade {g}/10` or the one-line reason for exit 1, 2 or 3.

Next: run `/edikt:gov:compile` if Phase B did not complete here, then `/edikt:doctor` — it reports any artifact still carrying a legacy block and any manual-review item left unchecked.
