---
name: gov:compile
description: "Compile ADRs and invariants into governance directives"
effort: high
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
  - Write
  - Task
tier_2_dependency: edikt phasea plan
on_absent: refuse-and-direct-user
tier_2_dependency_2: edikt phasea restore
on_absent_2: refuse-and-direct-user
tier_2_dependency_3: edikt phasea verify
on_absent_3: refuse-and-direct-user
---

# edikt:gov:compile

Compile accepted ADRs, active invariants, and team guidelines into topic-grouped governance rule files under `.claude/rules/governance/`, with a routing index at `.claude/rules/governance.md`.

## Compile Schema

**`COMPILE_SCHEMA_VERSION = 3`**

This integer identifies the output format contract of this command. It is independent of edikt's marketing version.

- `3` — the four-surface render. Ambient core (`governance.md`: pathless-invariant canonical statements plus a one-line topic index, no routing table, no aggregate reminder or verification sections), scoped topic files under `governance/`, and per-topic skill packages at `.claude/skills/edikt-<topic>/SKILL.md`. Accompanied by `manifest.yaml` listing every rendered surface with its SHA-256, and by `delivery-map.yaml`, the path->directive scope record (which directive governs which path, and which surface delivers it — it delivers nothing itself). The glob-keyed `directive-index.yaml` this version once rendered beside them is retired: edikt no longer injects rule text into a write in progress — only the ambient core and topic files deliver rules, and they deliver before any edit is attempted, never at the moment of the edit itself. A compile renders none and `edikt upgrade` removes a stale copy. No timestamps in any rendered surface. Current.

Bump this constant ONLY when the output structure changes in a way that older tooling cannot read. Prose updates, bug fixes, and new directives do NOT bump the schema. When bumped, write a new ADR to document the migration.

Each source document has a co-located `<artifact>.edikt.yaml` sidecar that holds compiled directives. Compile reads sidecars only — never the prose `.md`.

A project with no sidecars is in one of two states, and they need opposite remedies. Never conflate them:

| State | Signal | Remedy |
|---|---|---|
| **Pre-migration** | legacy `[edikt:directives:start]: #` blocks present in artifact bodies | refuse; direct the user to `/edikt:upgrade`, which runs `migrate sidecars` to lift them |
| **Never-initialised** | no sentinels anywhere — the artifacts predate edikt (hand-written ADRs, no edikt history) | bootstrap: extract a sidecar from each artifact's prose via the per-artifact `:compile` commands |

Routing a never-initialised project to migration is a dead end: `migrate sidecars` finds nothing to lift, reports `0 sidecars to create` and exits 0, and compile still has no sidecars. That loop is what blocked adoption on existing codebases. The `edikt` binary makes the same distinction and bootstraps missing sidecars itself, so tier-1 and tier-2 agree on who handles this case.

CRITICAL: NEVER write governance files that contain contradictions — detect and report them before writing, and abort or confirm with the user.

## Arguments

- `--check` — validate only; **never writes** a sidecar, a rendered surface, or the trust store. Emits a verdict line on stderr (`anchor drift: N stale (M sidecar(s))`). Exits 1 on any stale sidecar — including anchor drift the mechanical autorepair could fix: that drift is reported as `stale:` (`not written (--check) — run edikt gov compile`) and listed in the JSON `phase_a.repairable` / `phase_a.repairable_ids`. Run `edikt gov compile` to apply the repair. For CI.
- `--trust` — record this project in the user trust store before the post-compile verify gate. Refused with `--check` / `--dry-run` (exit 2): a non-writing flag cannot record trust.
- `--dry-run` — alias for `--check`. Use the flag that matches your mental model.
- `--json` — output structured JSON (see Reference). In two-phase mode emits a single object with `phase_a` and `phase_b` summaries; `phase_b` is **always** a populated object, never `null` (Claude Code consumers rely on this contract).
- `--no-wait` — fail fast instead of blocking on a held `compile.lock`.
- `--legacy` — **DEPRECATED (v0.7.0 removal).** Force the legacy in-body sentinel compile path even when sidecars exist. Preserved for the v0.5.x→v0.6.0 transition window only. Refuses to clobber when zero directives are resolved (defense against accidental wipe of curated `governance.md`).

All paths are resolved dynamically from `.edikt/config.yaml`. Defaults are never hardcoded at the dispatcher — projects with customized `paths.decisions`, `paths.invariants`, or `paths.guidelines` work correctly.

## Instructions

0. If `.edikt/config.yaml` does not exist, output:
   ```
   No edikt config found. Run /edikt:init to set up this project.
   ```
   And stop.

0a. **Pre-v0.6.0 sentinel gate.** Before any other work, refuse to run when legacy in-body sentinels remain in the project. v0.6.0 reads sidecars only — there is no double-parser fallback.

    Scan for the marker `[edikt:directives:start]: #` outside fenced regions and outside the documentation skip-list. The `edikt` binary handles fence detection and skip-list correctly:

    ```bash
    EDIKT_INVOKER=/edikt:gov:compile edikt migrate sidecars --dry-run > /tmp/edikt-sidecar-precheck.out 2>&1
    PRECHECK_EXIT=$?
    ```

    - If `PRECHECK_EXIT == 0` AND output contains `0 sidecars to create` — nothing to migrate, continue to Step 1. **This includes a project whose artifacts have no sidecars at all.** A `0 sidecars to create` result means there are no legacy sentinels to lift, NOT that the project is compiled. Missing sidecars are handled by step 12's bootstrap, not by migration — do not refuse here.
    - Otherwise — refuse with a single-line actionable error and exit 1:
      ```
      ✗ Migration required. Run /edikt:upgrade to migrate this project to v0.6.0 sidecar architecture.
      ```
      Do NOT print the dry-run plan here — `/edikt:upgrade` shows it. Keep this gate's output to one line so CI logs stay readable.

    NEVER fall back to in-body sentinel parsing. The pre-flight gate is the only path for legacy projects.

    NEVER send a user to `/edikt:upgrade` or `migrate sidecars` when the dry-run reported `0 sidecars to create`. Migration has nothing to do in that case; it exits 0 without acting, and the user lands back here with the same refusal. That loop is the reason this gate is scoped to sentinel presence alone.

0b. If `--json` is in `$ARGUMENTS`, output only the JSON format at the end — no progress indicators, no emoji, no prose.

1. Display progress: `Step 1/5: Reading source documents...`

1b. The version stamped into `compiled_by` is what `edikt version` reports — the active binary's own version, resolved by the launcher. A `.edikt/config.yaml` pin that disagrees with it is `edikt doctor`'s finding ("the pin is AHEAD/BEHIND the active version"), not this command's; compile stamps what ran.

2. Read `.edikt/config.yaml`. Resolve paths from the `paths:` section using the Path Defaults in the Reference section.

3. Read source documents:
   - **ADRs:** include if `status: accepted`. Skip `draft`, `superseded`, `deprecated`. Fall back to checking `**Status:** accepted` in the body for backwards compatibility.
   - **Invariants:** include if `status: active` or no status (backwards compatibility). Skip `status: revoked`.
   - **Guidelines:** include all `.md` files from the guidelines directory. No status filtering. Each filename (without `.md`) becomes the section label.

4. Display progress: `Step 2/5: Checking for contradictions...`

5. Detect contradictions between accepted ADRs: direct contradictions ("use X" vs "never use X"), scope conflicts, approach conflicts. Use the Contradiction Detection examples in the Reference section as a guide for how to report them.

6. Also check: superseded ADRs still referenced by active specs or plans; invariants that conflict with accepted ADRs; guidelines that conflict with ADRs or invariants. Conflicts between guidelines and invariants are errors (invariants always win). Conflicts between guidelines and ADRs are warnings.

7. If `--check` flag: report all contradictions and conflicts, then output the Check Output Format from the Reference section and stop (don't write).

8. If contradictions found and not `--check`: report them and ask user to proceed anyway or abort.

### Extract Directives

9. Display progress: `Step 3/5: Grouping directives by topic...`

10. For each source document (ADR, invariant, guideline), look for its co-located sidecar: `{same_dir}/{basename}.edikt.yaml`. sidecars are the authoritative source — `gov:compile` MUST read sidecars only. A legacy in-body block is never an input: step 0a refuses the compile while one is present.

11. **If sidecar exists:** read the sidecar YAML. The sidecar conforms to `templates/schemas/gov-sidecar.v2.schema.json` (v2):

    - **`directives[]`** — auto-extracted by `/edikt:<artifact>:compile` from the source body. Each entry has `text` and `source_excerpts[]` (an ordered array of 1..N anchors).
    - **`manual_directives[]`** — user-authored rules. Optional (defaults to `[]`). Preserved across sidecar regenerations. Always included in the effective rule set.
    - **`suppressed_directives[]`** — auto directives the user rejected. Optional (defaults to `[]`). Preserved across sidecar regenerations. Subtracted from `directives` at compile time.
    - **`reminders[]`** — pre-action reminders. Optional. Rendered per topic — into the topic file or skill package.
    - **`verification[]`** — verification checklist items. Optional. Rendered per topic — into the topic file or skill package.
    - **`topic`** — the topic key for grouping (e.g., `ai`, `backend`, `privacy`).
    - **`signals[]`** — routing keywords retained for search and for the benchmark's attack corpus. They no longer render a routing table: topic routing is the registry description.

    Compute the **effective rule set** using the directive merge formula:

    ```
    effective_rules = (directives[].text - suppressed_directives) ∪ manual_directives
    ```

    Where `-` is set difference by exact string match, and `∪` is set union preserving document order. Duplicates across both lists are de-duplicated; the first occurrence's source reference is kept.

    **Within-artifact contradiction detection**: a directive and a manual directive that contradict each other → warn but include both. A suppressed directive that matches nothing in `directives[]` → warn (stale suppression).

12. **If no sidecar exists, or a sidecar is stale** (fresh artifact, never compiled, quote drift, or a whole project adopting edikt on pre-existing docs): run the Phase A contract below. Then re-run step 11.

    This is the bootstrap path, and it applies whether ONE artifact needs extraction or ALL of them do. A project adopting edikt on hand-written ADRs arrives here with every artifact missing — that is expected, not an error state.

    **Extraction is tier-1's job, not the binary's.** `edikt gov compile` computes the work-list and emits a plan; it does NOT dispatch. Fanning extractors out means invoking an LLM, which INV-012 forbids a tier-2 binary from doing (ADR-030). You are the dispatcher.

    **Step A1 — plan.**

    ```bash
    edikt phasea plan
    ```

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

    Exit 1 on `plan` means there is nothing to extract — skip to step 11; it is an answer, not a failure. Exit 4 is a refusal: the budget ceiling; the extractor contract unresolved — either `extractor definition not found; looked at: <every path>` (install the agent) or `definition found at <path> but carries no prompt_version` (pin it), or two pinned copies that disagree; no `.edikt/config.yaml` above the working directory; the plan this corpus state derives was already consumed or failed — the corpus path never reuses a closed plan's id, and requesting the artifacts by name (`edikt phasea plan --artifact <path>`, or the per-artifact compile command) derives a successor plan instead; or an earlier plan for an item was never closed (dispatched and not restored, mid-retry, or FAILED) while its sidecar has moved since that plan pinned it — the message names the plan, its state and what to do, because planning over it would pin unrestored or failed output as the new baseline. Report the message verbatim and stop. Never re-run to get past a refusal.

    Read `.edikt/state/phasea/<plan_id>.plan.json`. Validate it is well-formed JSON with `schema_version`, `plan_id`, `created_from`, `budget` and `items` before acting on any part of it. A malformed plan is never partially acted on.

    **Step A2 — fan out, one subagent per item, at most `budget.max_concurrency` in flight.**

    The cap is the plan's `budget.max_concurrency` (from `phasea.max_concurrency`, default 2 —
    D19's measured-safe number, dated by `FINDING-2026-09-02-fanout-stall.md`). Read it from the
    plan you are executing; it is not a sentence in any command.

    **A stall is any final response other than the extractor's one-line result.** If a
    subagent's final response is anything other than exactly one `SIDECAR WRITTEN: <path>` line
    — including an empty response, the stream watchdog, a kill or a crash — report its
    `item_id` to A4 with `--stalled`, so the receipt records `stalled` (retryable) rather than a
    bare `output-missing`. Reporting an item that did write costs nothing: verify judges a
    written output on its bytes and ignores the report.

    For each entry in `items[]`, spawn **exactly one** `sidecar-extractor` subagent via `Task`. Compose the prompt from **only** that item's `artifact_path` and `item_id` — nothing else:

    ```
    Task(subagent_type: "sidecar-extractor",
         prompt: "<item.artifact_path> <item.item_id>")
    ```

    **No add, drop, merge, retry-with-changes, or reword.** Do not skip an item. Do not combine two items into one subagent context — every artifact's extraction still runs in its own fresh subagent context, never shared with another artifact's, and a merged context is how one artifact's prose leaks into another's sidecar. Do not dispatch an item that is not in `items[]`. This rule covers every first dispatch. A retry is A4-bis's correction pass, whose one permitted change is the receipt's `correction_prompt` prepended to the same two fields — never a prompt you fixed, annotated or summarised yourself.

    **The extractor's hand-back is not evidence.** Never relay what a subagent says it did — its count of entries or of `needs_review`, or what it kept; report counts from the receipt, never from the extractor's hand-back (A4-bis). In particular a hand-back may say a sidecar's scope was lost, or that it needs re-approval with `edikt sidecar approve --kind paths`. Restore owns `paths:`: it writes `paths:` and `paths_approval:` back exactly as they stood before the dispatch, labels globs that no approval receipt covers as unreceipted (never as approved), and drops a `paths:` the extractor wrote on a sidecar that had none. A hand-back saying scope was lost or needs re-approval is wrong — do not act on it.

    Continue on error: one subagent failing does not stop the fan-out of the rest. `phasea verify` is what decides what actually landed, and it can only do that if every item was attempted.

    **Step A3 — restore pinned state. NOT optional; Step A4 refuses without it.**

    ```bash
    edikt phasea restore --plan <plan_id>
    ```

    This step writes `.edikt/state/phasea/<plan_id>.restore.json`, and Step A4 refuses with exit 7 when it is absent. Nothing else enforced the order: a run that went fan-out → verify produced per-item results and correction prompts with no sign the pinned state had never come back. The extractor's input is the parent document, and its contract does not carry forward what a human pinned on the previous sidecar — it may not write `paths:` at all — so a regeneration arrives without it. Restore puts it back (manual and suppressed directives, approved verifies, `paths:` and `paths_approval:` as above), quarantines any regenerated sidecar that fails the content gate, and records the body-drift baseline. Run it before verify, once for the whole plan, and again after every re-dispatch round.

    Restore is also where "did the extractor write anything" is measured. It compares each sidecar with the plan's pre-dispatch pin (and, on a retry round, with what the previous restore left). A sidecar byte-identical to it is left exactly as found — not gated, not stamped — and listed as `unwritten` in the restore receipt; A4 grades it `output-missing` (`stalled` under `--stalled`) with the diagnostic `restore observed no write since plan <plan_id>`. Exit 2 is a rejected plan (unknown id, consumed, FAILED, or unreadable): report it verbatim and stop.

    **Step A4 — verify, and act on the exit code.**

    ```bash
    edikt phasea verify --plan <plan_id>                          # or, for subagents that died:
    edikt phasea verify --plan <plan_id> --stalled <item_id>[,<item_id>...]
    ```

    A4's report may end with a `concurrency (approximate, ...)` line: outputs written closer
    together than the cap allows, from extractor write times recorded by restore (end times,
    not dispatch starts) — captured before restore's own writes move them. With too few
    recorded times it says `concurrency: not measured` instead. It is reported, never
    enforced, and never changes the exit code.

    **Read the receipt's existence BEFORE interpreting the exit code.** If `.edikt/state/phasea/<plan_id>.receipt.json` does not exist, the run refused before evaluating anything — report that as a pre-run refusal, never as "items failed". A missing receipt and a receipt full of failures are different events, and only one of them says anything about the extraction.

    | Exit | Meaning | What you do |
    |------|---------|-------------|
    | 0 | every item ok | proceed to step 11 |
    | 1 | some items failed | re-dispatch each receipt item with `attempts < phasea.max_attempts_per_item` per A4-bis (below), then repeat A3 and A4 |
    | 2 | plan rejected (unknown, malformed, already consumed, or FAILED) | abort; report the message verbatim; never retry |
    | 3 | receipt unwritable | abort; report the message verbatim; never retry |
    | 5 | failed-terminal | **stop and report the failing items. Do NOT retry.** Exit 5 means the retry budget is spent; re-dispatching burns tokens against an item that has already been given every attempt it is owed |
    | 6 | diverged | **stop and report. Do NOT retry**, even though attempts remain. The item produced MORE defects than its previous attempt on the same axis; the remaining budget was deliberately left unspent because a retry that is getting worse does not improve by being run again |
    | 7 | restore has not run | run Step A3, then A4 again |

    Exit 1 and exit 5 differ only in whether anything is still owed a retry, and treating 5 as 1 is an unbounded loop that looks like progress.

    #### A4-bis. Re-dispatch as a CORRECTION PASS, never as a bare re-run

    **A re-dispatch MUST carry `correction_prompt` from the receipt item.** Read
    it out of `.edikt/state/phasea/<plan_id>.receipt.json` and prepend it to the
    extractor prompt. Do not paraphrase it and do not summarise it. A2's rule
    still holds for everything else: the prepend is the only permitted change
    to the item's two fields.

    This is not a refinement. Until the receipt carried a diagnostic, the retry
    resent the identical prompt against the identical artifact with no knowledge
    that it had failed or why — the diagnostic was computed at verify time and
    held in memory, and the retry reads the receipt. An unseeded retry draws its
    second sample from the same distribution as the first, so "different" is as
    likely to be worse as better. Measured, it was: one over-cap directive
    became four, across ~280k tokens and ~3.4 hours that produced nothing.

    Each item carries what the next dispatch needs:

    | Field | Use |
    |-------|-----|
    | `correction_prompt` | prepend verbatim; empty means the axis admits no mechanical correction — do NOT re-dispatch, report instead (except `output-missing` / `stalled`, below) |
    | `defect_axis` | `length` and `anchor` are correctable; `schema` and `parse` are not |
    | `defect_count` | compared against the previous attempt to detect divergence |
    | `anchors_valid` / `anchors_broken` / `anchors_whitespace_only` | a length failure with valid anchors needs sentences split, not a re-extraction; whitespace-only faults are quotes whose bytes differ from the parent only in whitespace |
    | `needs_review` | the count of `needs_review` entries in the written sidecar — the number to report |
    | `convergence` | `diverged` means stop; `improved` means the correction is working |

    An empty `correction_prompt` on a non-ok item is a refusal to guess, not an
    oversight. Re-dispatching without one reproduces the exact defect this
    section exists to remove. The one exception is an item with no output to
    correct: `output-missing` or `stalled` is re-dispatched fresh, with the
    plain A2 prompt.

    What arrives with a `correction_prompt`:

    - **A quarantined item arrives classified.** Restore moves a regenerated
      sidecar that fails the content gate into `.edikt/state/phasea/<plan_id>.quarantine/`;
      verify classifies it from that copy — `anchor-invalid`, `length-exceeded`,
      or `schema-invalid` (an over-claim carries `defect_axis: overclaim`) — and its `correction_prompt`
      names each faulting entry by the parent's line range (for a whitespace-only
      quote, the parent's exact bytes). It is never `output-missing`.
    - **`lossless-loss` can come from any re-extraction of an existing sidecar.**
      The output dropped a directive or prohibition body whose source prose is
      still in the artifact; the `correction_prompt` names each lost body and its
      line range. Remedies: the correction pass; restore the lost entries by
      hand and re-run; or, only on the user's explicit say,
      `edikt phasea verify --plan <plan_id> --on-loss=accept`, which passes the
      item with the loss recorded. An `ok` item can carry a diagnostic too — a
      prior body matched only by the weaker lexical fallback, or `source-removed`
      because its prose is gone from the artifact. Quote it; it is not a failure.

    **Report results from the receipt, never from the extractor's hand-back:**
    each item's `status`, `anchors_valid`, `anchors_whitespace_only` and
    `needs_review` (an absent count is zero), and the verify summary's
    `needs_review: K (counted on N sidecar(s))`.

    **Headless mode** (`EDIKT_HEADLESS=1` or no `/dev/tty`): do not fan out. Print the plan path and the explicit `/edikt:adr:compile`-style run-these list, and exit non-zero.

### Schema Completeness Gate

12a. **After reading all sidecars, validate that required fields are present.**

For each loaded sidecar, verify: `schema_version`, `topic`, `path`, `signals`, `directives` are all present (presence-check only — content validation is the sidecar validator's job). A sidecar missing any required field indicates a partial write or schema version mismatch.

If any sidecar is incomplete, abort with:
```
✗ {n} sidecar(s) are missing required fields:
    {path}: missing [{field1}, {field2}]

Re-run /edikt:adr:compile {ADR-NNN} (or invariant/guideline compile) to regenerate.
Then re-run /edikt:gov:compile.
```

Do NOT partial-write `governance.md` on an incomplete input set.

### Validate Cross-References

12b. For every extracted directive that references a specific invariant ID (INV-NNN), ADR ID (ADR-NNN), or other named artifact — verify the reference exists in the source document. Read the source file and confirm the referenced identifier appears in it. If it does not appear, strip the fabricated reference from the directive (keep the directive text if it's otherwise accurate). Never include a cross-reference that hasn't been confirmed in the actual source file.

### Directive-Quality Pass

12c. **After** the contradiction-detection pass (steps 5–8) and **before** grouping, run the shared directive-quality sub-procedure from `commands/gov/_shared-directive-checks.md` for every accepted ADR and active invariant.

For each source document that has a parsed sentinel block, invoke `edikt gov directive-check` (the tier-2 subcommand documented in `_shared-directive-checks.md §Tier-2 Subcommand`) once per directive in `directives:` and once per directive in `manual_directives:`. Pass:

```json
{
  "adr_id": "<ADR-NNN or INV-NNN>",
  "directive_body": "<directive line text>",
  "canonical_phrases": ["<phrase1>", ...],
  "no_directives_reason": "<reason string or null>"
}
```

Collect all returned warning lines across all documents. If any warnings were produced, output them under a `### Directive-quality warnings` header in the compile output:

```
### Directive-quality warnings

[WARN] ADR-NNN: directive has 2 sentences but no canonical_phrases — run /edikt:adr:review --backfill
[WARN] ADR-NNN: canonical_phrase "atomic rename" not found in directive body
```

**Grace period:** exit 0 even when warnings are present. Do NOT block compilation due to directive-quality warnings in v0.6.0. The header is surfaced so users are aware; it is not an error.

If no warnings were produced, skip the header entirely (do not emit an empty section).

### Orphan Detection Pass

12d. **After** the directive-quality pass (step 12c), run the orphan-detection and history-comparison pass.

**Two-layer atomicity model:**
- **Outer layer:** the compile operation as a whole is serialized by the existing `lock.yaml + flock` pattern in `edikt`. This prevents two concurrent compiles from racing on source files or the governance output. Phase 7 does not change this layer.
- **Inner layer:** the state file `.edikt/state/compile-history.json` is protected specifically from torn writes by using write-to-tempfile + `os.rename()`. If the process crashes between the write and the rename, the previous state file remains intact — safe toward re-warning rather than a silent skip. The `.tmp` file may exist and is safe to remove manually.

#### Pass 1: Orphan collection

Walk all accepted ADRs and active invariants. For each one, check whether:
- The parsed `directives` list AND `manual_directives` list are both empty (i.e., the effective rule set produces zero directives), AND
- The source document's frontmatter does NOT contain a `no-directives:` key with a valid reason.

A "valid reason" is defined by `_shared-directive-checks.md §Check C`: ≥ 10 characters, not in `{tbd, todo, fix later}` (case-insensitive), non-empty after strip.

If both conditions are true, add the ADR/INV ID to the **current orphan set**.

#### Pass 1b: `no-directives:` misuse check (N3,
docs/internal/audits/TRIAGE-2026-08-20-bok-services-governance-projection.md)

`no-directives:` is a **warning suppressor**, read only by Pass 1 above — it
never affects which directives compile. A field report documented an
operator reading the name as an instruction ("this artefact contributes no
directives"), applying it to an artefact whose effective rule set was
actually non-empty, seeing no error, and reporting suppression complete
while all directives kept compiling. This pass exists to catch exactly that
misreading, at the moment it happens rather than by counting refs in
compiled output later.

While walking the same accepted ADRs and active invariants as Pass 1, for
each one where the frontmatter contains a `no-directives:` key (with or
without a valid reason) **AND** the effective rule set (parsed `directives`
list plus `manual_directives` list) is **non-empty**, emit:

```
[WARN] ADR-NNN: no-directives: key present but N directive(s) are still compiling —
    this key only suppresses the empty-ruleset warning above; it has no effect on
    which directives project. To actually stop directives from compiling, use
    suppressed_directives instead.
```

Collect these under a `### no-directives misuse warnings` header, same
placement convention as the directive-quality warnings above (step 12c):
skip the header entirely if none were produced. Same **grace period** as
directive-quality warnings — exit 0 even when present; this pass never
blocks compilation.

#### Pass 2: History comparison and write

Delegate the five-rule orphan-set state machine to the tier-2 helper. This is an authorized tier-2 orchestration call; the implementation is pure Go (no LLM dispatch).

```bash
EDIKT_INVOKER=/edikt:gov:compile edikt gov compile-history \
    --orphans "$EDIKT_ORPHAN_IDS" \
    --history-path ".edikt/state/compile-history.json" \
    --edikt-version "$(edikt version)"
```

**Inputs (set by the caller):**
- `EDIKT_ORPHAN_IDS` — comma-separated list of orphan IDs (e.g. `ADR-NNN,INV-NNN`). Empty = no orphans this run.
- `--edikt-version` — `$(edikt version)`, the active binary's own version (stamps the `edikt_version` field).

**Exit-code contract (— output is informational, exit code is the contract):**
- `0` — first detection, subset/recovered, superset, different-reset, or no-orphans. Compile continues.
- `1` — consecutive scenario (same orphan set as previous run). BLOCK — compile MUST exit non-zero and MUST NOT write governance output.
- `2` — input-validation refusal (invalid orphan ID, traversal in `--history-path`).

The subcommand handles atomicity (tmp + rename), corrupt-history recovery (warn + treat as absent), and deterministic ordering of the orphan list.  Rule 2 do not parse stdout — display verbatim.

If exit is non-zero (scenario 2), the overall compile command MUST exit non-zero and MUST NOT proceed to write governance output.

If exit is 0, continue to step 13 (group by topic).

**`.gitignore` management:**

After the orphan script completes (regardless of exit code), run the tier-2 bootstrap subcommand. This is an authorized orchestration call; trailing-slash variants (`.edikt/state/` vs `.edikt/state`) are deduplicated inside the helper.

```bash
EDIKT_INVOKER=/edikt:gov:compile edikt gov gitignore-bootstrap --project-root "$EDIKT_PROJECT_ROOT"
```

**`.gitignore` note:** The helper checks for both `.edikt/state/` (with trailing slash) and `.edikt/state` (without) before appending, to avoid duplicates under trailing-slash normalization variants. If the file is absent, it is created. If the entry is already present in any recognized form, the file is left unchanged. Exit code is the contract — output is informational.

### Group by Topic

13. Analyze all **effective_rules** across all source documents (computed in step 11 via the three-list merge formula) and group them by topic. A topic is a domain area — caching, database, multi-tenancy, authentication, file storage, architecture (cross-cutting), etc.

    Grouping rules:
    - Effective rules from different sources about the same domain go into the same topic file
    - Each rule keeps its source reference (`ref: ADR-NNN, §Eviction`) whether it originated from `directives:` or `manual_directives:` — readers of governance.md cannot tell which list a rule came from, only which source document it references
    - If a rule doesn't fit an obvious topic, group it under `architecture.md` (cross-cutting)
    - Invariants are special — their effective rules go into `governance.md` (the index), not into topic files
    - Across source documents: if the same rule string appears in multiple effective sets, de-duplicate by exact string match. Keep the first occurrence's source reference.

13a. **Topic and signals come from the sidecar.** Under v0.6.0+, `topic` and `signals[]` live on the sidecar — they are written by the `sidecar-extractor` agent when the sidecar is generated or resynced (Phase A of `edikt gov compile`, dispatched by per-artifact `:compile` commands). The slash command MUST NOT write back to the parent `.md`; edikt does not mutate user-authored prose. If a sidecar is missing `topic`, the resolution is to regenerate the sidecar (`/edikt:adr:compile <id>` or similar), not to edit the prose.

### Derive Path Patterns

14. Display progress: `Step 4/5: Scanning codebase for path patterns...`

15. For each topic file, determine the `paths:` frontmatter:
    - **If pinned in the sidecar:** use the author's paths verbatim
    - **If not pinned:** scan the project directory structure to find where code related to this topic lives. Generate glob patterns matching those locations.
    - Use the `paths:` YAML list format (one glob per line)

16. For each topic file, determine the `scope:` metadata carried in its frontmatter:
    - **If specified in the sidecar:** use the author's scopes
    - **If not specified:** derive from content — architecture/cross-cutting decisions get `[planning, design, review]`, implementation-specific rules get `[implementation]`

### Write Output

17. Display progress: `Step 5/5: Writing governance files...`

18. Write topic rule files to `.claude/rules/governance/`:

    Each file follows this format:
    ```markdown
    ---
    paths:
      - "**/*.go"
      - "**/adapters/postgres/**"
    compile_schema_version: 3
    ---
    <!-- edikt:compiled — generated by /edikt:gov:compile, do not edit manually -->
    <!-- topic: {topic name} -->
    <!-- sources: {list of source documents that contributed} -->
    <!-- compiled_by: edikt v{edikt_version} -->

    # {Topic Name}

    - {directive} (ref: {source})
    - {directive} (ref: {source})
    ```

19. Write the governance index to `.claude/rules/governance.md`:

    ```markdown
    ---
    paths: "**/*"
    compile_schema_version: 3
    ---
    <!-- edikt:compiled — generated by /edikt:gov:compile, do not edit manually -->
    <!-- compiled_by: edikt v{edikt_version} -->

    # Governance Directives

    Follow these directives in every file you write or edit.

    ## Non-Negotiable Constraints

    These are invariants. Violation is never acceptable.

    - {invariant directive} (ref: INV-NNN)

    ## Topics

    Everything else is scoped. Each line says when a task needs that topic; read
    the named file when it matches what you are about to do.

    - **{topic}** — {approved registry description from .edikt/topics.yaml}
    ```

    Never render a routing table here, and never invent a description — an unapproved topic shows as pending.

    **A description is capped at 160 characters** (`topicregistry.MaxDescriptionChars`,
    and `maxLength` in `topic-registry.v1.schema.json`). Over-length is refused with
    `error: description is N chars (max 160)`. The cap is not stylistic: every approved
    description renders into the ambient topic index, which loads on **every** edit in
    every session, and 160 characters is roughly the 25-token per-topic budget that
    index is priced at. A description that needs more room is usually a topic that
    should be split.

20. If any single topic file exceeds 100 directives, warn:
    ```
    ⚠ {topic}.md has {count} directives. Large rule files may dilute compliance.
      Consider splitting into subtopics or running /edikt:gov:review to tighten language.
    ```

22. Log the compilation event:
    ```bash
    edikt hook event --type compile \
      --data '{"adrs_compiled":{n},"invariants_compiled":{m},"guidelines_compiled":{g},"topics":{t},"total_directives":{total},"sidecar_coverage":"{pct}%"}'
    ```

23. Output the compilation summary with reverse source map:
    ```
    ✅ Governance compiled

      governance/{topic}.md
        ← {source document} ({sections contributed})
        ← {source document} ({sections contributed})

      governance/{topic}.md
        ← {source document} ({sections contributed})

      ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
      {n} ADRs + {m} invariants + {g} guidelines
      → {t} topic files + index
      → {total} total directives
      Sidecar coverage: {with_sidecars}/{total_sources} documents ({pct}%)

      {If sidecars_regenerated > 0}:
      ⚙ {sidecars_regenerated} sidecars regenerated by Phase A (sidecar-extractor agent).
        Run /edikt:gov:review to review directive language quality.

      Claude will read these directives automatically in every session.

      Next: Run /edikt:gov:review to review directive language quality.
    ```

24. This command should be suggested (not auto-run) after `/edikt:adr:new` or `/edikt:invariant:new` creates or modifies a document. Add to those commands' output: `Run /edikt:gov:compile to compile the sidecars into governance.`

---

REMEMBER: NEVER write governance files with contradictions. Invariants go in the index (governance.md), not topic files — they are always loaded. Topic files contain domain-specific directives grouped from all sources. The ADRs and source documents are the source of truth — compiled output is the enforcement format, never hand-edit it.

## Reference

### Path Defaults

| Key | Default |
|---|---|
| `paths.decisions` | `docs/architecture/decisions` |
| `paths.invariants` | `docs/architecture/invariants` |
| `paths.guidelines` | `docs/guidelines` |

### Contradiction Detection Examples

```
⚠️  Contradiction detected:
    ADR-NNN: "Claude Code only — no multi-tool support"
    ADR-NNN: "Support Cursor for rule distribution"

    Resolve before compiling. Supersede one or reconcile both.
```

```
⚠️  Conflict between guideline and ADR:
    guidelines/testing.md: "Always mock the database in all tests"
    ADR-NNN: "Integration tests must hit a real database, no mocks"

    Source: guidelines/testing.md (line 12) vs the ADR (Decision section)
    Action: Scope the guideline to unit tests only, or amend the ADR.
```

```
⚠️  Conflict between guideline and invariant:
    guidelines/dependencies.md: "Use lodash for utility functions"
    INV-NNN: "No runtime dependencies"

    Source: guidelines/dependencies.md (line 5) vs the invariant (Rule section)
    Action: Remove the guideline — invariants are non-negotiable.
```

### JSON Output Format (two-phase mode)

`--json` in default or `--check` mode emits a single object. `phase_b` is **always** a populated object — never `null` — so consumers can distinguish "ran and produced no changes" from "did not run" via the `ran` field.

```json
{
  "status": "ok",
  "phase_a": {
    "dispatched": 0,
    "stale": 0,
    "stale_ids": [],
    "bootstrap": 0,
    "repairable": 0,
    "repairable_ids": [],
    "errors": []
  },
  "phase_b": {
    "ran": true,
    "topics_rendered": [],
    "topics_unchanged": [],
    "index_written": false,
    "total_directives": 27
  }
}
```

In `--check --json` mode, `phase_b.ran` is `false` (the merge is skipped). Over drifted anchors that autorepair resolves, the check fails and names them — `repairable_ids` is the subset of `stale_ids` that `edikt gov compile` fixes without an extraction, and is always an array (`[]` when none):

```json
{
  "status": "error",
  "phase_a": {"dispatched": 0, "stale": 2, "stale_ids": ["ADR-NNN", "INV-NNN"], "bootstrap": 0, "repairable": 1, "repairable_ids": ["ADR-NNN"], "errors": []},
  "phase_b": {"ran": false, "topics_rendered": [], "topics_unchanged": [], "index_written": false, "total_directives": 0},
  "error": "stale sidecars in --check mode"
}
```

On error, `status: "error"` and `error: "<message>"` are set; `phase_a`/`phase_b` remain populated with whatever state was reached.

### JSON Output Format (legacy mode, `--legacy`)

```json
{
  "status": "success",
  "topics": [{"name": "cache", "file": "governance/cache.md", "directives": 12, "sources": ["ADR-NNN", "guideline-database.md"]}],
  "invariants": [{"id": "INV-NNN", "directive": "..."}],
  "sentinel_coverage": {"with": 5, "total": 7, "percent": 71},
  "contradictions": [],
  "total_directives": 27
}
```

### Check Output Format (prose, default mode `--check`)

`--check` never writes. When no sidecar is stale it emits the verdict line, then the body-drift, conflict and render-freshness lines:

```
edikt gov compile --check: anchor drift: 0 stale (14 sidecar(s))
```

The stale count is computed, not printed as a constant. If any sidecar is stale — whether its drift needs extraction or autorepair could fix it — each one gets a `stale:` line and the run exits 1:

```
  stale: ADR-NNN — anchor drift, 2 anchor(s) mechanically repairable; not written (--check) — run edikt gov compile
  stale: INV-NNN — directive[1].anchor[0]: quote not found at lines 40-41
error: 2 sidecar(s) stale — 1 mechanically repairable (run edikt gov compile), 1 need extraction (run /edikt:gov:compile in Claude Code)
```

A partially repairable sidecar is listed once, with the anchor that is still stale after the repair. `/edikt:adr:review`, `/edikt:invariant:review` and `/edikt:gov:review` read these `stale:` lines.

Legacy mode (`--legacy --check`) emits the longer prose summary:

```
/edikt:gov:compile --check

  Sources: {n} ADRs ({m} accepted), {j} invariants ({l} active), {g} guidelines
  Sentinel coverage: {with_sentinels}/{total} documents
  Contradictions: {count}
  Conflicts: {count} (guideline vs ADR/invariant)
  Topics: {count} would be generated
  Directives: {count} would be generated

  {If contradictions: list them}
  {If clean: "All clear — governance compiles cleanly."}
```
