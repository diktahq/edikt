---
name: gov:audit
description: "Audit governance artifacts against the governance-auditor's D95/D118 contract — the status-filtered corpus, one artifact, or the fixture set"
effort: high
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
  - Write
  - Agent
tier_2_dependency: edikt gov audit-cite
on_absent: refuse-and-direct-user
---

# edikt:gov:audit

Run `gov audit-cite` (mechanical citation resolution) and the `governance-auditor` (class + decision-prong + prose judgement) over governance artifacts, and report the result as a table plus a persisted summary. Three modes:

- **Corpus** (default, no arguments): every ADR with `status: accepted` — the same filter `gov:compile` step 3 uses. Skips `draft`, `superseded`, `deprecated`.
- **Single artifact** (`[artifact-path]`): the same two checks, scoped to one file.
- **Fixtures** (`--fixtures`): walks `test/fixtures/governance-kinds/*/` instead of the corpus, comparing the auditor's `class.value` against each case's own `expected.yaml` `class:` field.

This is a **read-only report**. It never edits an artifact, never flips a `status:` field, and never writes anywhere outside `.edikt/state/audit/<ts>/`.

## Arguments

- `[artifact-path]` — optional. Audit exactly one artifact instead of walking the corpus. Mutually exclusive with `--fixtures`.
- `--fixtures` — audit `test/fixtures/governance-kinds/` instead of the real corpus.
- `--json` — suppress the table; print only the run's `summary.json` content to stdout.

## Stub mode

`EDIKT_AUDIT_STUB=1` is honoured **only** when `--fixtures` is also passed. Outside `--fixtures` it is silently ignored — a canned report must never gate a real corpus or single-artifact audit; those always dispatch the live `governance-auditor`. This flag exists for `test/test-gov-audit.sh`; it is not a documented end-user flag.

Under `--fixtures` with `EDIKT_AUDIT_STUB=1`, for each case:

1. Look for a canned report at `test/fixtures/audit-reports/<case-name>.json` (a full `templates/agents/audit-report.schema.json` object). If present, use it as the case's "actual" report, with `meta.stubbed` forced to `true` and `meta.ran_at` stamped to the run timestamp.
2. If absent, synthesize a minimal actual report directly from the case's own `expected.yaml` (`class`, `ruled_out`, `overall_verdict`, `decision_prongs` if present) — this reproduces the documented answer, not an independent judgement — with `meta.stubbed: true`, `meta.agent_version: "stub"`, `meta.corpus_size: 0`.

Every report the stub path produces or reads carries `stubbed: true`; `commands/adr/new.md` step 6b already refuses acceptance on that field, so a stub report can never silently pass a real gate.

## Instructions

### 0. Config guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### 0a. Absence detection (ADR-029 Rule 4)  edikt-guard:allow

```bash
command -v edikt >/dev/null 2>&1 || {
  echo "✗ edikt binary not found on PATH. Install it, then re-run — this command depends on 'edikt gov audit-cite'."
  exit 1
}
```

### 1. Parse arguments and pick a mode

Detect `--fixtures` and `--json` anywhere in `$ARGUMENTS`; the remaining token (if any) is `[artifact-path]`. `--fixtures` together with an artifact path is a usage error — refuse and print the two-mode contract above.

- `--fixtures` present → **fixtures mode**.
- An artifact path given → **single mode**.
- Neither → **corpus mode**.

### 2. Open a fresh run directory

```bash
TS=$(date -u +%Y%m%dT%H%M%SZ)
OUT_DIR=".edikt/state/audit/$TS"
mkdir -p "$OUT_DIR"
```

`OUT_DIR` is always generated here, from the run's own clock — never taken from `$ARGUMENTS`, an env var, or any input. Every report this run produces lands under it.

### 3a. Fixtures mode

For each directory under `test/fixtures/governance-kinds/*/` that contains an `expected.yaml`:

1. Read `expected.yaml`'s top-level `class:` field (a bare string) as `EXPECTED_CLASS`.
2. If `EDIKT_AUDIT_STUB=1`: build the actual report per the Stub mode section above.
3. Otherwise (live run): resolve the input artifact — `draft.md` in the case directory if present, else the path in `expected.yaml`'s `source:` field. If neither exists (the `folder-structure-*` cases are illustrative per the contract doc's own §6a — no real or constructed draft backs them), record `{"id": "<case>", "status": "reference-only", "skipped": true}` in the summary and move to the next case; a reference-only case is never counted toward `total`, `matches`, or `mismatches`. Otherwise dispatch the auditor exactly as `commands/adr/new.md` step 6b does:
   - `subagent_type: governance-auditor`
   - `prompt: "Read the draft artifact at {ABS_PATH}. Read the corpus of accepted ADRs, invariants, and guidelines under {decisions_dir}, {invariants_dir}, {guidelines_dir}. Grade every applicable decision prong, run restatement detection against the corpus, grade the five prose checks, and emit exactly one audit-report JSON object."`

   If the dispatch fails with `Agent type 'governance-auditor' not found` but `.claude/agents/governance-auditor.md` exists (installed this session), use the fallback in `commands/_shared-agent-routing.md` § "Fallback: agent installed this session".

   Validate the report before reading anything out of it (INV-011 — fail closed on missing, empty, or schema-nonconforming; absence of output is never evidence of completion). A validation failure counts as a mismatch for that case, with `ACTUAL_CLASS` recorded as `"unresolved"`.  edikt-guard:allow
4. Write the actual report to `$OUT_DIR/<case>.json`.
5. `ACTUAL_CLASS = <report>.class.value`. Record `{"id": "<case>", "expected_class": EXPECTED_CLASS, "actual_class": ACTUAL_CLASS, "match": ACTUAL_CLASS == EXPECTED_CLASS}`.

Print a table:

```
case                              expected    actual      
do-nothing-alternatives           nothing     nothing     ✓
adr-015-corpus-case               adr         adr         ✓
...
folder-structure-architecture-choice  adr     —           (reference-only, skipped)
```

Go to Step 4.

### 3b. Corpus and single-artifact mode

Resolve the target list:

- **Corpus**: every `{decisions_dir}/ADR-*.md` with `status: accepted` in frontmatter (fall back to `**Status:** accepted` in the body for backwards compatibility) — the identical filter `commands/gov/compile.md` step 3 applies to ADRs. Skip `draft`, `superseded`, `deprecated`.
- **Single**: the one given path.

For each target:

1. `EDIKT_INVOKER=/edikt:gov:audit edikt gov audit-cite <path>`. Consume the exit code only (ADR-029 Rule 2 — never parse the binary's stdout for control flow, only display it):  edikt-guard:allow
   - `0` → continue to step 2.
   - `1` → record `{"id": "<ADR-id>", "citations": "failed", "class": null, "verdict": null}`, print the binary's output verbatim, and continue to the next target (a citation failure on one artifact does not stop the run).
   - `3` → usage/internal error; report and abort the whole run.
2. Dispatch `governance-auditor` for real — `EDIKT_AUDIT_STUB` is never honoured here, whatever its value. Use the same prompt shape and INV-011 validation as step 3a.4 above.  edikt-guard:allow
3. Write the report to `$OUT_DIR/<id>.json`.
4. Class disagreement: the artifact is filed as an ADR (it's under `{decisions_dir}`) but the auditor's `class.value != "adr"` → flag it in the table and count it as a mismatch, same as a citation failure.

Print a table (id | citations | class | verdict), then go to Step 4.

### 4. Write the summary and decide the exit code

```json
{
  "ran_at": "<ISO8601 of TS>",
  "mode": "fixtures" | "corpus" | "artifact",
  "stubbed": true | false,
  "total": <int>,
  "matches": <int>,
  "mismatches": <int>,
  "cases": [ {"id": ..., "expected_class": ..., "actual_class": ..., "match": ...} , ... ]
}
```

Write it to `$OUT_DIR/summary.json`. `total`/`matches`/`mismatches` exclude `reference-only` fixture entries. A citation failure and a class disagreement both count as a mismatch in corpus/single mode.

If `--json` is present, print `$OUT_DIR/summary.json`'s content to stdout and skip the table (Steps 3a/3b already built it — suppress printing it in this case).

Exit `1` if `mismatches > 0`, else exit `0`.

## Completion

```
✅ Governance audit complete
   Mode: fixtures | corpus | artifact
   Reports: .edikt/state/audit/<ts>/<id>.json (per case/artifact) + summary.json
   Result: {matches} of {total} reproduced the expected class ({mismatches} mismatch(es))

   Next: Inspect .edikt/state/audit/<ts>/summary.json for the failing case(s),
   or fix the disagreeing artifact and re-run.
```

## Why this command exists

`commands/adr/new.md`'s Step 6 gates one ADR at the moment it's drafted. This command answers a different question — of the artifacts already on disk (or the fixture set that stands in for a corpus), which still disagree with the auditor's judgement, without waiting for someone to touch them again. `--fixtures` is also the Phase 3 mechanical check that the fixture set (`test/fixtures/governance-kinds/`) is wired correctly end-to-end; it does not by itself prove the auditor's judgement is *sound* — 11 of the fixture cases assert §6 of `docs/internal/designs/governance-artifact-kinds-contract-DRAFT.md` against its own answers, so a passing run shows this command reproduces that document's stated verdicts, not that those verdicts are independently validated. The live, unstubbed `--fixtures` run (Phase 13, `EDIKT_RUN_BENCHMARKS=1`, `test/integration/benchmarks/`'s hermetic sandbox per INV-007) is what actually exercises the auditor's judgement against them.  edikt-guard:allow
