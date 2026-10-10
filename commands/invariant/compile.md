---
name: invariant:compile
description: "Regenerate the sidecar for one Invariant Record (or all active invariants) via the sidecar-extractor agent"
effort: normal
argument-hint: "[INV-NNN | path] — omit to process all active invariants"
allowed-tools:
  - Read
  - Write
  - Glob
  - Grep
  - Bash
  - Agent
---

# edikt:invariant:compile

Regenerate the directive sidecar (`<INV-NNN>.edikt.yaml`) for one Invariant Record — or for every active invariant if no argument is given.

Per the relevant decision (sidecar architecture), directive metadata for every active Invariant Record lives in a co-located `<INV-NNN>.edikt.yaml` sidecar conforming to `templates/schemas/gov-sidecar.v2.schema.json` (v2). This command never writes to the parent `.md`. It dispatches the locked `sidecar-extractor` agent which reads the parent body and writes the sidecar.

## Arguments

- `$ARGUMENTS` — optional. One of:
  - `INV-NNN` (e.g., `INV-NNN`) — resolve to `{invariants_dir}/INV-NNN-*.md`
  - An absolute or repo-relative path to an invariant markdown file
  - Empty / omitted — regenerate every active invariant's sidecar

## Instructions

### 0. Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### 1. Config

Read `.edikt/config.yaml` to resolve `invariants_dir` (default `docs/architecture/invariants`).

### 2. Resolve target(s)

- If `$ARGUMENTS` matches `INV-NNN`, find `{invariants_dir}/INV-NNN-*.md`. If multiple match, error out asking the user to disambiguate by full path. If none match, error out with `error: no invariant matches {ARGUMENTS}`.
- If `$ARGUMENTS` is a path, use it directly. Verify the file exists and is under `{invariants_dir}`.
- If `$ARGUMENTS` is empty, list every `INV-*.md` in `{invariants_dir}` whose frontmatter has `status: active` (or no `status:` field — treated as active for backwards compatibility).

### 2b. The sidecar schema's project copy

The extractor resolves `gov-sidecar.v2` project-locally first (`.edikt/schemas/gov-sidecar.v2.schema.json`), then from the installed payload. That project copy is installed by `/edikt:init` and refreshed by `/edikt:upgrade` from the version's payload — this command never copies it. If it is absent, say so once (`schema copy absent — run /edikt:upgrade`) and continue: the extractor carries the full allowed-key contract in its own prompt and does not require the file, and it emits the `$schema` header only when the copy exists. NEVER instruct the extractor to read `templates/schemas/...` as a project-relative path: that resolves only inside the edikt repo.

### 3. Plan, dispatch, restore, verify — never dispatch the extractor directly

This command does not dispatch the extractor bare. Every regeneration — one artifact or a
corpus — goes through the same plan/receipt path `gov:compile` uses, so a dispatch that
produced nothing is a recorded failure with a receipt, never a completion message taken on
trust. The stat-before/after discipline this step used to carry asked the model to notice
an unchanged file; a plan's pre-dispatch snapshot, compared by restore, makes "unchanged" a
measured thing.

**A1 — a one-item plan for exactly this artifact:**

```bash
edikt phasea plan --artifact "{ABS_PATH_TO_INV}"
```

**If the sidecar is ABSENT, A1 REFUSES, and that refusal is doing real work.** A
re-extraction rebuilds the sidecar from the parent document — but a sidecar's
`manual_directives` and `suppressed_directives` are prose *you* wrote into the sidecar
itself. They appear nowhere in the parent, so the extractor cannot bring them back. They
survive a normal re-extraction only because the old sidecar is copied aside first and they
are carried forward from that copy. With the sidecar already gone there is nothing to copy,
so they would be destroyed — and every report afterwards would still read clean.

No tool can tell a first extraction from a lost one by looking at a file that is not there,
so it asks instead of guessing:

- the sidecar was **lost** (deleted, a bad merge, an ignore rule) — restore it first, then
  re-run. `git checkout -- <sidecar>` if it is tracked.
- it **never existed** and this is a first extraction — re-run with `--bootstrap`.

Exit 0 prints the plan path; the plan carries one item with `reason: "requested"` — it is
planned whether or not the stale detector would have selected it ("the extractor missed a
rule" is exactly the fresh case). When this artifact's last plan is already closed
(consumed, or FAILED), the request never reuses that plan's id: it derives a successor
plan, printed as `supersedes <plan_id> (<state>)` under the plan path.

Exit 4 is a refusal; report its message verbatim and stop. It covers an unknown path, an
excluded artifact, an unpinned extractor contract — and an earlier plan for this artifact
that was never closed (dispatched and not restored, mid-retry, or FAILED) while the sidecar
has moved since that plan pinned it. The message names that plan, its state and what to do
(finish its restore and verify, or copy its pin back before re-requesting). Planning over it
would pin unrestored or failed extractor output as the new baseline, and the open plan's pin
is the only copy of what its restore must reinstate.

Read `.edikt/state/phasea/<plan_id>.plan.json` and take `plan_id`, `items[0].item_id`,
`items[0].artifact_path` and `budget.max_concurrency` from it.

**A2 — dispatch, from the plan and only the plan.** The host caches agent definitions at session start: if `.claude/agents/sidecar-extractor.md` changed in this session (an upgrade, a template sync), restart the session first, or the previous definition runs and its output is judged against the new contract (D58).

Use the Agent tool:
- `subagent_type: sidecar-extractor`
- `prompt: "<items[0].artifact_path> <items[0].item_id>"`

If the dispatch fails with `Agent type 'sidecar-extractor' not found` but `.claude/agents/sidecar-extractor.md` exists (installed this session), use the fallback in `commands/_shared-agent-routing.md` § "Fallback: agent installed this session".

The fan-out cap is the plan's `budget.max_concurrency`, not a sentence here; a one-item plan
needs none.

**A stall is any final response other than the extractor's one-line result.** If the
subagent's final response is anything other than exactly one `SIDECAR WRITTEN: <path>` line
— including an empty response, the host's stream watchdog, a kill or a crash — note its
`item_id`: A4 reports it with `--stalled`. Reporting an item that did write costs nothing:
verify judges a written output on its bytes and ignores the report.

**The extractor's hand-back is not evidence.** Never relay what it says it did — its count of
entries or of `needs_review`, or what it kept. In particular it may say the sidecar's scope
was lost, or that it needs re-approval with `edikt sidecar approve --kind paths`. Restore
owns `paths:`: it writes `paths:` and `paths_approval:` back exactly as they stood before the
dispatch, labels globs that no approval receipt covers as unreceipted (never as approved),
and drops a `paths:` the extractor wrote on a sidecar that had none. A hand-back saying scope
was lost or needs re-approval is wrong — do not act on it.

**A3 — restore pinned state (not optional — A4 refuses with exit 7 without it):**

```bash
edikt phasea restore --plan <plan_id>
```

Restore is where "did the extractor write anything" is measured. It compares the sidecar
with the plan's pre-dispatch pin (and, on a retry round, with what the previous restore left).
A sidecar byte-identical to it is left exactly as found — not gated, not stamped — and listed
as `unwritten` in `.edikt/state/phasea/<plan_id>.restore.json`. Otherwise restore reinstates
what a human pinned (manual and suppressed directives, approved verifies, the `paths:` above),
quarantines a regenerated sidecar that fails the content gate, and records the body-drift
baseline. Exit 2 is a rejected plan (unknown id, consumed, FAILED, or unreadable): report the
message verbatim and stop.

**A4 — verify, and act on the exit code:**

```bash
edikt phasea verify --plan <plan_id>            # or, if A2 recorded a stall:
edikt phasea verify --plan <plan_id> --stalled <item_id>
```

**Read the receipt's existence BEFORE the exit code.** No receipt at
`.edikt/state/phasea/<plan_id>.receipt.json` is a refusal — report the message verbatim. With a
receipt: exit 0 is the sidecar written and verified; exit 1 is retryable; exit 5 or 6 is
terminal — stop and report; exit 7 means A3 did not run. On exit 1 the receipt item decides
the next dispatch:

- `output-missing` or `stalled` — there is no output to correct. Re-dispatch the item fresh
  (A2), then A3 and A4 again. When restore saw nothing written, the diagnostic reads
  `restore observed no write since plan <plan_id>`: the extractor wrote nothing — a dispatch
  that ran out of turns, or a stale agent definition cached at session start (run
  `edikt doctor` and restart the session). Never report it as "unchanged".
- a status with a `correction_prompt` — `anchor-invalid`, `length-exceeded`, a quarantined
  write (classified from its quarantine copy), or `lossless-loss` — re-dispatch as a
  correction pass per `commands/gov/compile.md` § A4-bis (the `correction_prompt` prepended,
  nothing else changed), then A3 and A4 again.
- any other status with an empty `correction_prompt` — report it; do not re-dispatch.

`lossless-loss` can come from any re-extraction of an existing sidecar: the output dropped a
directive or prohibition body whose source prose is still in the artifact. Its
`correction_prompt` names each lost body and its line range. The remedies are that correction
pass; restoring the lost entries by hand and re-running; or, only on the user's explicit say,
`edikt phasea verify --plan <plan_id> --on-loss=accept`, which passes the item with the loss
recorded.

### 4. Verify the artifact on disk — fail closed (INV-011)

**The agent's completion message is NOT evidence.** The receipt is what says the sidecar
landed. A1 copies each item's whole pre-dispatch sidecar aside before the extractor runs, and
A3 compares against that copy, so "unchanged" is measured rather than noticed. A receipt item
at `ok` means `expected_output.path` exists, parses, validates against `gov-sidecar.v2`, and
quotes its parent byte-exactly. Only an `ok` item proceeds to the comparison below.

**Report counts from the receipt, never from the extractor's hand-back:** each item's
`status`, `anchors_valid`, `anchors_whitespace_only` and `needs_review` (an absent count is
zero), and the verify summary's `needs_review: K (counted on N sidecar(s))`. An `ok` item can
carry a diagnostic — a prior body matched only by the weaker lexical fallback, or one that is
`source-removed` because its prose is gone from the artifact. Quote it; it is not a failure.

The receipt is authoritative, and it is also checkable by hand when a report reads wrong:

```bash
SIDECAR="{path-to}.edikt.yaml"
if [ ! -f "$SIDECAR" ]; then
  echo "❌ extraction FAILED for $SIDECAR — the receipt MUST say output-missing or stalled here."
  # Report this target as FAILED. NEVER report it as 'unchanged'.
fi
edikt util yaml validate "$SIDECAR" || echo "❌ extraction FAILED for $SIDECAR — exists but does not parse."
```

A sidecar whose bytes still match its pre-dispatch state is `output-missing` in the receipt
(`restore observed no write since plan <plan_id>`). Restore made that comparison, not you.
NEVER report it as 'unchanged'; idempotency is decided in 4b, on a sidecar the receipt says
was written.

### 4b. Detect idempotency

Compare the new sidecar to its prior version using canonical YAML serialization:

```bash
canonicalize() {
  edikt util yaml canonicalize "$1"
}
```

If the canonical form matches, report `unchanged`. Otherwise report `regenerated`.

The remaining source of a false `regenerated` report is the sidecar-extractor's *semantic* output: it is not byte-deterministic across LLM runs. The Go-side canonical serializer has shipped (`internal/sidecar/marshal.go`); the residual variance here is the LLM extraction, not serialization.

### 5. Confirm

For a single-target run:
```
✅ {INV-NNN}.edikt.yaml — {regenerated | unchanged}
   Source: {invariants_dir}/{INV-NNN}-{slug}.md
```

For an all-targets run:
```
Invariant sidecar regeneration:
  ✅ INV-NNN — regenerated
  ✅ INV-NNN — unchanged
  ...

  {n} regenerated, {m} unchanged, {k} skipped (revoked).
  Next: Run /edikt:gov:compile to update governance.
```

If any extractor invocation fails, list the failures and exit non-zero.

## Why this command exists

the `<INV-NNN>.md` is user-authored prose. Sidecar regeneration is the only mechanism by which the invariant's directive metadata changes. This command is invoked:

- By `/edikt:invariant:new` immediately after writing a new Invariant Record
- By the user when they edit an invariant's prose body
- By `/edikt:gov:compile`'s Phase A (resync) when it detects sidecar staleness —

This command does not run the topic-grouping merge step. After regenerating one or more sidecars, run `/edikt:gov:compile` to refresh `.claude/rules/governance/`.
