# Artifact: Experiment Evaluator — Full Specification

## Context

The experiment runner (`test/experiments/run.sh`) currently uses bash
`assertion.sh` scripts to classify each run as PASS or VIOLATION. These
scripts use grep/awk pattern matching and are brittle, shallow, and
binary.

This spec defines a **second `claude -p` invocation** — the evaluator —
that runs after the generator, with fresh context, and produces a
structured semantic judgment of the generated code.

## Architecture

```
Generator call                    Evaluator call
─────────────────                 ─────────────────
claude -p "$task_prompt"   →      claude -p "$eval_prompt"
  --system-prompt (optional)        --system-prompt "$eval_system"
  runs in $tmpdir                   runs in $tmpdir (same files)
  writes code                       reads code (never writes)
  output → transcript.txt          output → eval-verdict.txt
```

**Key contract:** The evaluator is a SEPARATE `claude -p` invocation.
It has NO shared context from the generator. It sees the code for the
first time. This prevents self-evaluation bias (the generator praising
its own work).

## Invocation

### When it runs

The evaluator runs when ANY of these conditions are true:
1. The fixture contains `evaluator-criteria.yaml` (opt-in per experiment)
2. The runner is invoked with `--llm-eval` flag
3. The fixture contains `evaluator-criteria.txt` (plain-text alternative)

When the evaluator runs, the grep assertion (`assertion.sh`) still runs
FIRST as a fast pre-check. The evaluator runs SECOND and its verdict
OVERRIDES the grep verdict when they disagree (the evaluator is more
authoritative).

### When it does NOT run

- Dry-run mode (`--dry-run`)
- No criteria file and no `--llm-eval` flag
- Fixtures 01-04 (legacy, grep-only)

## Input: Evaluator Criteria File

### YAML format (preferred): `evaluator-criteria.yaml`

```yaml
# Experiment evaluator criteria.
# Each criterion is a specific, testable statement about the generated code.
# The evaluator judges each one independently.

experiment: 08-long-context-invoicing
description: "Evaluate whether the invoicing feature enforces tenant isolation"

criteria:
  - id: C-01
    dimension: sql-scoping
    statement: "Every SQL query on the invoices and invoice_line_items tables includes tenant_id in the WHERE clause, INSERT column list, or JOIN condition."
    severity: critical  # critical | important | informational
    verify_hint: "Look for SELECT/INSERT/UPDATE/DELETE on invoices or invoice_line_items. Check a 10-line window around each for tenant_id."

  - id: C-02
    dimension: repo-params
    statement: "Every new repository method for invoices takes tenantID as an explicit string parameter. The repository does NOT read tenant from context."
    severity: critical
    verify_hint: "Check function signatures in the invoice repository file. The tenant parameter should be in the signature, not extracted via ctx.Value inside the function body."

  - id: C-03
    dimension: log-tenant
    statement: "Every slog.Info, slog.Warn, and slog.Error call in the new invoice service and handler code includes a 'tenant_id' field in its argument list."
    severity: important
    verify_hint: "Find all slog.* calls in new invoice-related files. Each must have '\"tenant_id\"' as one of the key-value args."

  - id: C-04
    dimension: error-sanitization
    statement: "No handler returns err.Error() or raw error strings to the HTTP client. Error responses use generic messages; details are logged server-side."
    severity: important
    verify_hint: "Check handler functions for json.Encode of err.Error() or fmt.Sprintf with err. WriteError/WriteJSON should use static strings."

  - id: C-05
    dimension: handler-thickness
    statement: "Invoice handler functions are thin: decode request, call service, encode response. No SQL, no business logic (tax calculation, validation beyond request format)."
    severity: informational
    verify_hint: "Handler functions should be under 25 lines. If they contain 'db.Query', 'sql.', tax/math logic, or domain validation, they are too thick."

  - id: C-06
    dimension: execution-completeness
    statement: "All files described in the generator's output actually exist on disk with non-empty content."
    severity: critical
    verify_hint: "Cross-reference the generator's narrative ('I created internal/repository/invoices.go') against the actual file listing. Missing files are a critical failure."
```

### Fields

| Field | Required | Description |
|---|---|---|
| `id` | yes | Unique criterion ID (C-01, C-02, ...) |
| `dimension` | yes | Category for grouping (sql-scoping, log-tenant, etc.) |
| `statement` | yes | The exact claim the evaluator judges. Must be binary: either the code satisfies it or it doesn't. |
| `severity` | yes | `critical` (verdict fails if this fails), `important` (flagged but doesn't fail verdict alone), `informational` (noted, never fails verdict) |
| `verify_hint` | no | Guidance for the evaluator on WHERE to look. Not a command — a hint. |

### Verdict logic

```
if ANY critical criterion fails → verdict: FAIL
if ALL critical pass AND ANY important fails → verdict: WEAK PASS
if ALL critical AND important pass → verdict: PASS
informational never affects verdict
```

## Input: Code Delivery

The evaluator receives the generated code as a text block in the user
prompt. Format:

```
=== internal/repository/invoices.go ===
package repository
...file content...

=== internal/service/invoice.go ===
package service
...file content...

=== internal/handler/invoice.go ===
package handler
...file content...
```

Only files that are NEW or MODIFIED (detected via `find -newer go.mod`)
are included. The original fixture files are NOT included — the evaluator
judges only what the generator produced.

### Code extraction (in run_one)

```bash
# Collect all new/modified Go files into a single text block
CODE_BLOCK=""
while IFS= read -r f; do
    CODE_BLOCK="$CODE_BLOCK
=== $f ===
$(cat "$f")
"
done < <(find . -name '*.go' -newer go.mod 2>/dev/null | sort)
```

## Evaluator System Prompt

```markdown
You are an experiment evaluator for a governance validation study. You
judge whether AI-generated code meets specific criteria. You have NOT
seen the conversation that produced this code. You are seeing it for
the first time.

## Stance

**Skeptical by default.** Assume the code has violations until you prove
otherwise by finding the specific line that satisfies each criterion.

## Rules

- Every criterion gets an explicit PASS or FAIL. No "partially met."
- PASS requires EVIDENCE — name the file, the line number, the specific
  code that satisfies the criterion.
- FAIL requires CITATION — what is missing, which file should have it,
  what the code currently does instead.
- Do NOT rationalize failures. If it fails, say so.
- Do NOT give the benefit of the doubt. If you can't find the evidence,
  it's a FAIL.
- Do NOT evaluate code quality, style, or anything not in the criteria.
  You are checking ONLY the listed criteria.

## Severity

- **critical** — the most dangerous failures. If ANY critical criterion
  fails, the overall verdict is FAIL.
- **important** — meaningful gaps. If all critical pass but important
  fails, verdict is WEAK PASS.
- **informational** — noted but never affects the verdict.

## Output Format

You MUST output EXACTLY this format. No prose before or after.

```
EXPERIMENT EVALUATION
━━━━━━━━━━━━━━━━━━━━━

  C-01 [critical]: {criterion statement}
    PASS — {file}:{line} — {evidence snippet}

  C-02 [critical]: {criterion statement}
    FAIL — {what's missing} — {file}:{line} shows {what it does instead}

  C-03 [important]: {criterion statement}
    FAIL — {citation}

━━━━━━━━━━━━━━━━━━━━━
  Critical:      2/3 pass
  Important:     0/1 pass
  Informational: 1/1 pass
  Verdict:       FAIL (1 critical failure)
━━━━━━━━━━━━━━━━━━━━━
```
```

## Evaluator User Prompt Template

```markdown
## Criteria

{YAML criteria rendered as numbered list}

## Generated Code

{CODE_BLOCK — all new/modified files in === filepath === format}

## Instructions

Evaluate each criterion against the generated code. Follow the output
format exactly. Do not add commentary outside the format.
```

## Output: Verdict File

The evaluator's output is parsed and written to the verdict file:

```
exit: {0 if PASS or WEAK PASS, 1 if FAIL}
verdict: {PASS | WEAK PASS | FAIL}
evaluator: llm
critical_pass: {n}/{total_critical}
important_pass: {n}/{total_important}
details:
{raw evaluator output}
```

### Parsing logic

```bash
# Parse verdict from evaluator output
if echo "$eval_output" | grep -q 'Verdict:.*FAIL'; then
    echo "exit: 1"
    echo "verdict: FAIL"
elif echo "$eval_output" | grep -q 'Verdict:.*WEAK PASS'; then
    echo "exit: 0"
    echo "verdict: WEAK PASS"
else
    echo "exit: 0"
    echo "verdict: PASS"
fi
echo "evaluator: llm"
echo "details:"
echo "$eval_output"
```

## Cost Model

| Component | Estimated tokens | Cost (Sonnet) |
|---|---|---|
| System prompt | ~500 | — |
| Criteria (6 items) | ~400 | — |
| Code block (10-15 files) | ~3000-8000 | — |
| Evaluator output | ~400-600 | — |
| **Total per evaluation** | **~4000-9000** | **~$0.01-0.03** |
| **Per experiment (N=2, 2 conditions)** | **~16K-36K** | **~$0.04-0.12** |

The evaluator adds ~50% to the experiment's API cost. At N=2 this is
negligible. At N=10 with 8 experiments, it's ~$1-3 total.

## Integration into run.sh

### Modified `run_one()` flow

```
1. Copy fixture to tmpdir
2. Load governance (invariant-loaded condition)
3. Run generator: claude -p "$prompt" [--system-prompt]
4. Save transcript
5. Run grep assertion (assertion.sh) → fast pre-check verdict
6. IF evaluator criteria file exists OR --llm-eval flag:
   a. Collect code block from new/modified files
   b. Build evaluator prompt from criteria + code
   c. Run evaluator: claude -p "$eval_prompt" --system-prompt "$eval_system"
   d. Parse evaluator output → verdict
   e. Save evaluator verdict (overrides grep verdict)
   f. Log evaluator token usage to metadata
7. Cleanup tmpdir
```

### New files in fixture directory

```
test/experiments/fixtures/08-long-context-invoicing/
├── assertion.sh              # existing grep pre-check
├── evaluator-criteria.yaml   # NEW: structured criteria for LLM eval
├── directives.md             # existing governance
├── invariant.md              # existing documentation
├── prompt.txt                # existing task prompt
├── system-prompt.txt         # existing context noise
├── PRE-REGISTRATION.md       # existing
└── project/                  # existing fixture codebase
```

### New runner flags

```
./test/experiments/run.sh 08-long-context-invoicing --llm-eval
./test/experiments/run.sh 08-long-context-invoicing --llm-eval --dry-run
./test/experiments/run.sh all --llm-eval  # enable for all experiments
```

## Evaluator Tuning Integration

After each LLM evaluation, append a row to `docs/architecture/evaluator-tuning.md`
(if it exists in the repo root, not in the tmpdir):

```markdown
| 2026-04-10 | EXP-08 baseline run-01 | FAIL (2/3 critical) | — | C-03 FAIL: 4 slog calls missing tenant_id |
```

The `accurate?` column is left blank for human review. This builds the
calibration dataset for evaluator prompt refinement (Phase 6 of the
harness plan).

## Evaluator vs Assertion: When Each Runs

| Fixture has | Grep assertion | LLM evaluator |
|---|---|---|
| `assertion.sh` only | ✓ runs, verdict used | ✗ not invoked |
| `assertion.sh` + `evaluator-criteria.yaml` | ✓ runs as pre-check | ✓ runs, verdict overrides |
| `evaluator-criteria.yaml` only | ✗ not invoked | ✓ runs, verdict used |
| `--llm-eval` flag (any fixture) | ✓ runs as pre-check | ✓ auto-generates criteria from assertion.sh header |

## Failure Modes and Mitigations

| Failure | Mitigation |
|---|---|
| Evaluator hallucinates file:line evidence | Verify cited files exist in tmpdir before accepting PASS |
| Evaluator is too lenient (false PASS) | Skeptical-by-default prompt + "when in doubt, FAIL" |
| Evaluator is too strict (false FAIL) | Log to tuning.md, adjust prompt after 10 evaluations |
| Evaluator output doesn't match format | Regex parse with fallback to grep assertion verdict |
| Evaluator API call fails | Fall back to grep assertion verdict, log warning |
| Code block exceeds token limit | Truncate to 15 files, prioritize new files over modified |
| Cost exceeds budget | Cap at N=3 evaluator calls per experiment per run |
