---
name: gov:grade-compile
description: "Grade the editorial quality of compiled governance — an LLM-as-judge scores coherence, conciseness, signal-to-noise, description quality, tier assignment, and no-double-loading."
effort: medium
tier: 2
tier_2_dependency: "edikt gov grade-compile"
on_absent: "refuse-and-direct-user"
argument-hint: "[--dir <governance-dir>] [--model <id>]"
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
  - Agent
---

# edikt:gov:grade-compile

Grade the **editorial quality of your compiled governance** — the
`.claude/rules/governance/` tree that `gov compile` produces. An LLM acts as
judge, reading the compiled files and scoring them 0–10 on four dimensions:

- **coherence** — related directives grouped into the same topic file
- **conciseness** — reminders tight and actionable, no redundant noise
- **signal-to-noise** — high-priority directives surfaced, not buried
- **description quality** — each topic's registry description tells a reader mid-task whether this topic is the one they need
- **tier assignment** — each directive is delivered on the surface its scope justifies (ambient, write-time index, or skill)
- **no double loading** — each directive body appears on exactly one ambient surface

The grade is **advisory**. Compiled governance is the surface your agents read
on every task; a degraded surface silently weakens guidance even when every
underlying decision is correct. This command gives you a quality signal plus
specific, actionable findings — it never changes your governance.

Run it **after `gov compile`**. The grader runs out of the compile loop and
never affects compile determinism.

## Pre-flight Gate

Before running, confirm the tier-2 binary is available. Run:

```bash
command -v edikt >/dev/null 2>&1 || test -x bin/edikt
```

If neither resolves, **refuse and direct the user** — do not attempt a fallback:

```
❌ The `edikt` tier-2 binary is not installed.
   `gov grade-compile` needs it to resolve the rubric/schema paths and to
   validate and persist the grader's report. (The grader itself is
   dispatched from this command, not from the binary.)
   Install it, then re-run: see README.md → Install.
```

## Run

The grader is an **LLM-as-judge dispatched from here**, not from the binary.
Tier-2 never spawns an LLM (INV-012), so this command owns the dispatch and
the binary owns the deterministic half: resolving paths, validating the
agent's JSON, and persisting it (ADR-044).

### 1. Resolve the inputs

```bash
bin/edikt gov grade-compile --print-inputs
```

Exit 0 means the governance dir, rubric, report schema, and agent template all
resolved (honouring `.edikt/templates/` project overrides). **Gate on the exit
code, not the text** — per the tier-1 → tier-2 contract, this command consumes
the binary's exit status only. On non-zero, surface the message and stop: a
grader dispatched without its rubric would produce a confident, meaningless
score.

### 2. Dispatch the grader

```text
Agent(
  subagent_type: "compile-quality-grader",
  description: "Grade compiled governance",
  prompt: $PROMPT_BODY
)
```

**Prompt body construction.** Build it with `python3 -c`, passing the resolved
paths as `sys.argv` values — never by shell-string interpolation. The grader
reads the files itself; file CONTENT is never interpolated into the prompt.

```bash
PROMPT_BODY=$(python3 -c '
import json, sys
gov_dir, rubric, schema = sys.argv[1], sys.argv[2], sys.argv[3]
print("\n".join([
    f"Grade the compiled governance tree at: {gov_dir}",
    f"Rubric: {rubric}",
    f"Report schema: {schema}",
    "",
    "Read every *.md under the governance dir, then score 0-10 on each rubric",
    "dimension: coherence, conciseness, signal_to_noise, description_quality, tier_assignment, no_double_loading.",
    "",
    "Return ONE JSON object conforming to the schema. Every dimension MUST carry",
    "a real score. If you cannot grade a dimension, say so in findings and fail —",
    "do NOT emit 0 for a dimension you did not evaluate.",
]))
' "$GOV_DIR" "$RUBRIC" "$SCHEMA")
```

The instruction against emitting an unevaluated 0 is deliberate. The
predecessor to this flow dispatched from inside the binary, never unwrapped
the CLI's result envelope, and silently recorded every grade as 0/10 — a
control reporting results it never computed. The binary now rejects such a
report, and the agent is told not to produce one.

### 3. Record the result

Write the agent's JSON to a file, or pipe it:

```bash
bin/edikt gov grade-compile --record report.json
# or
printf '%s' "$AGENT_JSON" | bin/edikt gov grade-compile --record -
```

The binary validates before persisting: a missing dimension, a
`schema_version` of 0, or a score outside 0–10 is **refused with exit 1**, not
recorded. An absent grade is a failure, never a zero.

### Stub mode (testing / CI)

```bash
EDIKT_GRADE_COMPILE_STUB=1 bin/edikt gov grade-compile
```

Writes a canned fixture report — exercises the record pipeline with no agent.

## Present the results

After the run, read the latest report and surface it to the user:

1. Read the newest file under `.edikt/state/compile-quality/`.
2. Lead with the **overall** score and the four **per-dimension** scores.
3. List the **findings** — each names its dimension, a severity
   (`info` / `warning` / `critical`), a message, and an optional suggested fix.
   Prioritise `critical` and `warning` findings; group `info` at the end.
4. If any dimension scored below 6/10, call it out as the highest-leverage area
   to improve, and point at the specific files/sections named in the findings.

Do not edit governance in response to a grade unless the user asks — the grade
is a signal, and re-authoring decisions or re-running `gov compile` is the
user's call.

## Completion

End with a one-line verdict, e.g. `✅ Compile quality: 8/10 — no critical findings`.

Next: if any dimension scored below 6/10, address the files named in the findings, re-run `/edikt:gov:compile`, then re-grade.
