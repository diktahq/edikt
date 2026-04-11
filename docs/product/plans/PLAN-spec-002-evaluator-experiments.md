# Plan: SPEC-002 — Evaluator Configuration, Headless Execution, LLM Experiment Evaluator

## Overview
**Task:** Add evaluator config section, headless claude -p execution, and LLM evaluator in experiment runner
**Spec:** SPEC-002 (docs/product/specs/SPEC-002-evaluator-experiments/spec.md)
**Total Phases:** 4
**Estimated Cost:** $0.32
**Created:** 2026-04-11

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | done    | 1/3    | 2026-04-11 |
| 2     | done    | 1/3    | 2026-04-11 |
| 3     | pending | 0/3    | -       |
| 4     | pending | 0/3    | -       |

**IMPORTANT:** Update this table as phases complete. This table is the persistent state that survives context compaction.

## Model Assignment

| Phase | Task | Model | Reasoning | Est. Cost |
|-------|------|-------|-----------|-----------|
| 1 | Evaluator config + headless prompt | Sonnet | New config section + new template file | $0.08 |
| 2 | Plan command integration | Sonnet | Branching logic in plan.md, toggle behavior | $0.08 |
| 3 | Experiment runner LLM evaluator | Sonnet | Shell script changes, evaluator invocation, verdict parsing | $0.08 |
| 4 | Test fixtures + tests | Sonnet | Create fixture dirs, structural + functional tests | $0.08 |

## Execution Strategy

| Phase | Depends On | Parallel With |
|-------|-----------|---------------|
| 1     | None      | 3             |
| 2     | 1         | -             |
| 3     | None      | 1             |
| 4     | 1, 2, 3   | -             |

## Artifact Flow

| Producing Phase | Artifact | Consuming Phase(s) |
|-----------------|----------|---------------------|
| 1 | `templates/agents/evaluator-headless.md` | 2, 3 |
| 1 | `evaluator.*` config keys in `commands/config.md` | 2 |
| 3 | `test/experiments/lib/evaluator-system-prompt.md` | 4 |
| 3 | Modified `test/experiments/directive-effect/run.sh` | 4 |

## Artifact Coverage

```
✓ evaluator-test-fixtures.yaml → Phase 4 (create actual fixture directories from scenarios)
— experiment-evaluator-spec.md → reference only, no phase needed
```

---

## Phase 1: Evaluator Config + Headless Prompt File

**Objective:** Create the headless evaluator prompt template and ensure evaluator config keys are properly documented
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `EVALUATOR CONFIG DONE`
**Evaluate:** true
**Dependencies:** None

**Context Needed:**
- `templates/agents/evaluator.md` — existing subagent template (the source of evaluation rules)
- `commands/config.md` — config command (evaluator keys already added in v0.3.1)
- `website/governance/evaluator.md` — evaluator docs page (already created)
- `docs/product/specs/SPEC-002-evaluator-experiments/spec.md` — the spec

**Acceptance Criteria:**
- [ ] AC-1.1: `templates/agents/evaluator-headless.md` exists with NO YAML frontmatter
- [ ] AC-1.2: Headless prompt contains "assume the work is incomplete until proven otherwise" or equivalent skeptical stance
- [ ] AC-1.3: Headless prompt contains output format instructions for structured parsing
- [ ] AC-1.4: Headless prompt contains note about zero shared context with generator
- [ ] AC-1.5: Headless prompt contains same evaluation rules as `evaluator.md` (binary PASS/FAIL, evidence required, never modify code)
- [ ] AC-1.6: `evaluator.md` has comment pointing to headless version
- [ ] AC-1.7: `commands/config.md` Key Reference has all 5 evaluator keys (preflight, phase-end, mode, max-attempts, model)

**Prompt:**
```
Read these files first:
- templates/agents/evaluator.md (the existing subagent evaluator — full content)
- commands/config.md (the config command — check evaluator keys in Key Reference)
- docs/product/specs/SPEC-002-evaluator-experiments/spec.md (section 2: Two Evaluator Prompt Files)
- website/governance/evaluator.md (evaluator docs — already created)

Create `templates/agents/evaluator-headless.md`:

This is the system prompt for headless evaluator execution via `claude -p --system-prompt`. It must NOT have YAML frontmatter (no `---` block, no `tools:`, no `maxTurns:`).

Content must include:

1. Identity: "You are a phase-end evaluator. You verify whether completed work meets the acceptance criteria. You have ZERO shared context with the agent that did the work — you evaluate from scratch."

2. Skeptical stance: "Default stance: skeptical. Assume the work is incomplete until proven otherwise."

3. Evaluation rules (same as evaluator.md):
   - Every criterion gets explicit PASS or FAIL — no "partially met"
   - FAIL requires specific citation: what's missing, what file, what line
   - PASS requires evidence: test name, file:line, grep result
   - Do not rationalize failures
   - Run tests if a test command is available
   - NEVER modify code — you evaluate, you don't fix

4. Output format (structured for machine parsing):
   ```
   PHASE EVALUATION
   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

     AC-001: {criterion}
       PASS — {evidence: file:line, test name}

     AC-002: {criterion}
       FAIL — {what's missing, where it should be}

   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
     Verdict: {PASS | FAIL}
     Passed: {n}/{total} criteria
   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   ```

5. Note: "You are running in headless mode via claude -p. You have no conversation history, no memory, no hooks. Your only inputs are the acceptance criteria and the code files."

6. Constraints: same as evaluator.md (NEVER modify code, NEVER soften findings, NEVER approve unverified work, read evaluator-tuning.md if it exists)

7. Pre-flight mode section: same classification rules (TESTABLE/VAGUE/SUBJECTIVE/BLOCKED) and output format as evaluator.md

Then modify `templates/agents/evaluator.md`:
- Add a comment at the top (after the frontmatter closing `---`):
  `<!-- Subagent mode. For headless mode, see evaluator-headless.md -->`

Verify `commands/config.md` already has these evaluator keys in the Key Reference table:
- evaluator.preflight (default: true)
- evaluator.phase-end (default: true)
- evaluator.mode (default: headless)
- evaluator.max-attempts (default: 5)
- evaluator.model (default: sonnet)
If any are missing, add them.

When complete, output: EVALUATOR CONFIG DONE
```

---

## Phase 2: Plan Command Integration

**Objective:** Modify plan command to read evaluator config and branch between headless and subagent execution
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `PLAN INTEGRATION DONE`
**Evaluate:** true
**Dependencies:** Phase 1

**Context Needed:**
- `templates/agents/evaluator-headless.md` — created in Phase 1
- `templates/agents/evaluator.md` — existing subagent template
- `commands/sdlc/plan.md` — the plan command (read ALL of it)
- `.edikt/config.yaml` — to understand config format
- `docs/product/specs/SPEC-002-evaluator-experiments/spec.md` — sections 3 and 4

**Acceptance Criteria:**
- [ ] AC-2.1: Plan command step 1 reads `evaluator.*` config values (preflight, phase-end, mode, max-attempts, model)
- [ ] AC-2.2: Plan step 11 checks `evaluator.preflight` — skips pre-flight if false
- [ ] AC-2.3: Phase-End Flow checks `evaluator.phase-end` — skips evaluation if false, still emits criteria sidecar
- [ ] AC-2.4: Phase-End Flow checks `evaluator.mode` — uses headless `claude -p` when "headless", Agent tool when "subagent"
- [ ] AC-2.5: Headless invocation includes `--bare`, `--disallowedTools "Write,Edit"`, `--model {evaluator.model}`, `--output-format json`
- [ ] AC-2.6: Plan command checks evaluator template exists before invocation — blocks with error if missing
- [ ] AC-2.7: Evaluator template existence check uses correct path per mode (headless → evaluator-headless.md, subagent → evaluator.md)

**Prompt:**
```
Read these files first:
- commands/sdlc/plan.md (the plan command — read ALL of it, especially step 1, step 11, and Phase-End Flow in Reference section)
- templates/agents/evaluator-headless.md (created in Phase 1)
- templates/agents/evaluator.md (existing subagent template)
- .edikt/config.yaml (current config — note the evaluator section may not exist yet in dogfood config)
- docs/product/specs/SPEC-002-evaluator-experiments/spec.md (sections 3: Headless Execution, 4: Toggle Behavior)

Modify `commands/sdlc/plan.md`:

1. STEP 1 (context loading, ~line 37):
   Add instruction to read evaluator config from `.edikt/config.yaml`:
   ```
   Read evaluator configuration:
   - evaluator.preflight (default: true) — whether to run pre-flight criteria validation
   - evaluator.phase-end (default: true) — whether to run phase-end evaluation
   - evaluator.mode (default: headless) — "headless" for separate claude -p, "subagent" for Agent tool
   - evaluator.max-attempts (default: 5) — max retries before stuck
   - evaluator.model (default: sonnet) — model for headless evaluator
   If the evaluator section is absent, use all defaults.
   ```

2. STEP 11 (pre-flight criteria validation, ~line 158):
   Add check at the start:
   ```
   If evaluator.preflight is false, skip this step entirely. Output:
   "Pre-flight validation skipped (evaluator.preflight: false in config)."
   ```

3. PHASE-END FLOW (Reference section, ~line 218):
   Replace the current evaluation invocation with mode-aware logic:

   ```
   When a phase completes (generator outputs the completion promise):

   1. If evaluate: true AND evaluator.phase-end is true:
      
      a. EVALUATOR FILE CHECK — before invoking:
         - If evaluator.mode is "headless": check templates/agents/evaluator-headless.md exists
           (or ~/.edikt/templates/agents/evaluator-headless.md for global install)
         - If evaluator.mode is "subagent": check templates/agents/evaluator.md exists
           (or .claude/agents/evaluator.md)
         - If missing, output:
           ❌ Evaluator template missing — cannot run evaluation.
              Expected: {path}
              Run: curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
              Or disable evaluation: /edikt:config set evaluator.phase-end false
           Do NOT silently skip. This is a hard failure.

      b. HEADLESS MODE (evaluator.mode = "headless"):
         Invoke via Bash tool:
         claude -p "{evaluation prompt with criteria + file list}" \
           --system-prompt "$(cat {path to evaluator-headless.md})" \
           --allowedTools "Read,Grep,Glob,Bash" \
           --disallowedTools "Write,Edit" \
           --model {evaluator.model} \
           --output-format json \
           --bare
         
         The evaluation prompt (user message) must include:
         - The phase's acceptance criteria
         - The list of files modified during the phase
         - The project's test command if available
         
         Parse the JSON output to extract per-criterion PASS/FAIL verdicts.

      c. SUBAGENT MODE (evaluator.mode = "subagent"):
         Spawn the evaluator agent via Agent tool with the phase's acceptance criteria,
         code changes, and test results. (This is the current behavior — no change needed.)

      d. Process verdict: PASS → context reset guidance. FAIL → report failures + backoff logic.

   2. If evaluate: true AND evaluator.phase-end is false:
      Skip evaluation. Output: "Phase-end evaluation skipped (evaluator.phase-end: false)."
      The criteria sidecar is still updated with status "pending" (not evaluated).
      Proceed to context reset guidance.

   3. Context reset guidance (always):
      {existing context reset block — no change}
   ```

When complete, output: PLAN INTEGRATION DONE
```

---

## Phase 3: Experiment Runner LLM Evaluator

**Objective:** Add LLM evaluator to the experiment runner with dual-mode grep+LLM, severity tiers, and verdict logic
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `EXPERIMENT EVALUATOR DONE`
**Evaluate:** true
**Dependencies:** None

**Context Needed:**
- `test/experiments/directive-effect/run.sh` — the experiment runner (gitignored, exists locally)
- `docs/product/specs/SPEC-002-evaluator-experiments/experiment-evaluator-spec.md` — full evaluator design
- `docs/product/specs/SPEC-002-evaluator-experiments/evaluator-test-fixtures.yaml` — test scenarios
- `test/experiments/directive-effect/fixtures/08-long-context-invoicing/evaluator-criteria.yaml` — existing criteria file
- `test/experiments/directive-effect/fixtures/08-long-context-invoicing/assertion.sh` — existing grep assertion

**Acceptance Criteria:**
- [ ] AC-3.1: Runner accepts `--llm-eval` flag
- [ ] AC-3.2: Runner detects `evaluator-criteria.yaml` in fixture directory as auto-trigger
- [ ] AC-3.3: Grep assertion runs FIRST, LLM evaluator runs SECOND when conditions met
- [ ] AC-3.4: Code collection finds files newer than go.mod, formats as `=== filepath ===` blocks, truncates to 15 files
- [ ] AC-3.5: Evaluator invocation uses `claude -p` with `--bare`, `--disallowedTools "Write,Edit"`, `--output-format text`
- [ ] AC-3.6: `test/experiments/lib/evaluator-system-prompt.md` exists with skeptical stance
- [ ] AC-3.7: Verdict parsing extracts PASS/WEAK PASS/FAIL from evaluator output
- [ ] AC-3.8: LLM verdict overrides grep verdict when both run (LLM is authoritative)
- [ ] AC-3.9: Severity logic: any critical fail → FAIL, all critical pass + important fail → WEAK PASS, all pass → PASS
- [ ] AC-3.10: Verdict file written as `run-NN-eval.txt` with exit, verdict, evaluator, critical_pass, important_pass fields
- [ ] AC-3.11: Metadata includes evaluator_invoked, evaluator_model, evaluator_verdict, evaluator_verdict_source

**Prompt:**
```
Read these files first:
- test/experiments/directive-effect/run.sh (the experiment runner — read ALL of it)
- docs/product/specs/SPEC-002-evaluator-experiments/experiment-evaluator-spec.md (the full evaluator design — sections: Architecture, Invocation, Input format, System prompt, Verdict logic, Cost model, Integration)
- test/experiments/directive-effect/fixtures/08-long-context-invoicing/evaluator-criteria.yaml (existing criteria)
- test/experiments/directive-effect/fixtures/08-long-context-invoicing/assertion.sh (existing grep assertion)

Modify `test/experiments/directive-effect/run.sh` to add LLM evaluation:

1. FLAG PARSING:
   Add `--llm-eval` flag to the argument parser. Store as LLM_EVAL=true/false.

2. EVALUATOR CONDITION DETECTION:
   In the per-run function, after the generator completes:
   ```bash
   EVAL_ENABLED=false
   if [ "$LLM_EVAL" = true ]; then EVAL_ENABLED=true; fi
   if [ -f "$FIXTURE_DIR/evaluator-criteria.yaml" ]; then EVAL_ENABLED=true; fi
   if [ -f "$FIXTURE_DIR/evaluator-criteria.txt" ]; then EVAL_ENABLED=true; fi
   ```

3. GREP ASSERTION (unchanged — runs first):
   The existing assertion.sh invocation stays as-is. Capture its exit code as GREP_VERDICT.

4. CODE COLLECTION (new, after grep):
   ```bash
   if [ "$EVAL_ENABLED" = true ]; then
     CODE_BLOCK=""
     MARKER_FILE=$(find "$TMPDIR" -name "go.mod" -o -name "package.json" -o -name "Cargo.toml" | head -1)
     if [ -n "$MARKER_FILE" ]; then
       NEW_FILES=$(find "$TMPDIR" -newer "$MARKER_FILE" -type f \
         ! -path "*/.*" ! -name "*.log" ! -name "*.txt" \
         | head -15)
       for f in $NEW_FILES; do
         REL_PATH="${f#$TMPDIR/}"
         CODE_BLOCK="${CODE_BLOCK}=== ${REL_PATH} ===
   $(cat "$f")

   "
       done
     fi
   fi
   ```

5. EVALUATOR SYSTEM PROMPT:
   Create `test/experiments/lib/evaluator-system-prompt.md`:
   ```
   You are evaluating generated code against specific criteria. You have zero context from the code generator — you see this code for the first time.

   Default stance: skeptical. Assume violations exist until you prove otherwise.

   For each criterion:
   - PASS: cite evidence (file:line showing the criterion is met)
   - FAIL: cite what's missing (file:line showing the gap, or "file not found")

   Severity rules:
   - critical: failure blocks the verdict
   - important: failure degrades to WEAK PASS
   - informational: logged only, never affects verdict

   Output format (MUST follow exactly):
   EXPERIMENT EVALUATION
   ━━━━━━━━━━━━━━━━━━━━━
     C-01 [{severity}]: {statement}
       {PASS|FAIL} — {evidence or gap}
     C-02 [{severity}]: {statement}
       {PASS|FAIL} — {evidence or gap}
   ━━━━━━━━━━━━━━━━━━━━━
     Critical:      {n}/{total} pass
     Important:     {n}/{total} pass
     Informational: {n}/{total} pass
     Verdict:       {PASS | WEAK PASS | FAIL}
   ━━━━━━━━━━━━━━━━━━━━━

   When in doubt, FAIL. A false pass is worse than a false fail.
   ```

6. EVALUATOR INVOCATION:
   ```bash
   EVAL_SYSTEM=$(cat test/experiments/lib/evaluator-system-prompt.md)
   EVAL_CRITERIA=$(cat "$FIXTURE_DIR/evaluator-criteria.yaml")
   EVAL_PROMPT="Evaluate the following generated code against these criteria.

   CRITERIA:
   ${EVAL_CRITERIA}

   GENERATED CODE:
   ${CODE_BLOCK}"

   EVAL_OUTPUT=$(claude -p "$EVAL_PROMPT" \
     --system-prompt "$EVAL_SYSTEM" \
     --allowedTools "Read,Grep,Glob,Bash" \
     --disallowedTools "Write,Edit" \
     --output-format text \
     --bare 2>&1)
   ```

7. VERDICT PARSING:
   ```bash
   LLM_VERDICT=$(echo "$EVAL_OUTPUT" | grep -oE 'Verdict: *(PASS|WEAK PASS|FAIL)' | head -1 | sed 's/Verdict: *//')
   CRITICAL_PASS=$(echo "$EVAL_OUTPUT" | grep -oE 'Critical: *[0-9]+/[0-9]+' | head -1)
   IMPORTANT_PASS=$(echo "$EVAL_OUTPUT" | grep -oE 'Important: *[0-9]+/[0-9]+' | head -1)

   # Determine exit code
   if [ "$LLM_VERDICT" = "PASS" ]; then
     LLM_EXIT=0
   else
     LLM_EXIT=1
   fi

   # LLM overrides grep when both run
   FINAL_VERDICT="$LLM_VERDICT"
   FINAL_EXIT="$LLM_EXIT"
   VERDICT_SOURCE="llm"
   ```

   If LLM evaluation fails (no output, parse error):
   ```bash
   if [ -z "$LLM_VERDICT" ]; then
     echo "WARNING: LLM evaluator failed to produce verdict. Falling back to grep."
     FINAL_VERDICT=$([ "$GREP_EXIT" -eq 0 ] && echo "PASS" || echo "FAIL")
     FINAL_EXIT="$GREP_EXIT"
     VERDICT_SOURCE="grep-fallback"
   fi
   ```

8. VERDICT FILE:
   Write `run-NN-eval.txt`:
   ```
   exit: {FINAL_EXIT}
   verdict: {FINAL_VERDICT}
   evaluator: llm
   critical_pass: {CRITICAL_PASS}
   important_pass: {IMPORTANT_PASS}
   verdict_source: {VERDICT_SOURCE}
   details:
   {EVAL_OUTPUT}
   ```

9. METADATA:
   Append to the run's metadata:
   ```
   evaluator_invoked: true
   evaluator_model: sonnet
   evaluator_verdict: {FINAL_VERDICT}
   evaluator_verdict_source: {VERDICT_SOURCE}
   ```

10. OVERRIDE LOGIC:
    Replace the existing final verdict assignment with:
    ```bash
    if [ "$EVAL_ENABLED" = true ] && [ -n "$LLM_VERDICT" ]; then
      # LLM is authoritative
      RUN_EXIT=$LLM_EXIT
      RUN_VERDICT=$LLM_VERDICT
    else
      # Grep only
      RUN_EXIT=$GREP_EXIT
      RUN_VERDICT=$([ "$GREP_EXIT" -eq 0 ] && echo "PASS" || echo "VIOLATION")
    fi
    ```

When complete, output: EXPERIMENT EVALUATOR DONE
```

---

## Phase 4: Test Fixtures + Tests

**Objective:** Create actual test fixture directories from evaluator-test-fixtures.yaml scenarios and write structural + functional tests
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `EVALUATOR TESTS DONE`
**Evaluate:** true
**Dependencies:** Phase 1, Phase 2, Phase 3

**Context Needed:**
- `docs/product/specs/SPEC-002-evaluator-experiments/evaluator-test-fixtures.yaml` — 5 test scenarios
- `templates/agents/evaluator-headless.md` — created in Phase 1
- `templates/agents/evaluator.md` — modified in Phase 1
- `commands/sdlc/plan.md` — modified in Phase 2
- `test/experiments/directive-effect/run.sh` — modified in Phase 3
- `test/experiments/lib/evaluator-system-prompt.md` — created in Phase 3
- `test/helpers.sh` — assertion functions
- `test/test-v040-plan-harness.sh` — SPEC-001 test pattern

**Acceptance Criteria:**
- [ ] AC-4.1: `test/test-v040-evaluator.sh` exists and is executable
- [ ] AC-4.2: Test verifies `evaluator-headless.md` exists with no YAML frontmatter
- [ ] AC-4.3: Test verifies `evaluator-headless.md` contains skeptical stance language
- [ ] AC-4.4: Test verifies `evaluator.md` has comment pointing to headless version
- [ ] AC-4.5: Test verifies `commands/config.md` has all 5 evaluator keys
- [ ] AC-4.6: Test verifies `plan.md` references `evaluator.preflight` and `evaluator.phase-end` and `evaluator.mode`
- [ ] AC-4.7: Test verifies `plan.md` has headless invocation format with `--bare` and `--disallowedTools`
- [ ] AC-4.8: Test verifies `plan.md` has evaluator file existence check with error message
- [ ] AC-4.9: Test verifies experiment runner has `--llm-eval` flag
- [ ] AC-4.10: Test verifies `test/experiments/lib/evaluator-system-prompt.md` exists with skeptical stance
- [ ] AC-4.11: Test verifies experiment runner has verdict parsing for PASS/WEAK PASS/FAIL
- [ ] AC-4.12: Test verifies experiment runner writes `run-NN-eval.txt`
- [ ] AC-4.13: All existing test suites still pass (`./test/run.sh`)

**Prompt:**
```
Read these files first:
- docs/product/specs/SPEC-002-evaluator-experiments/evaluator-test-fixtures.yaml (5 test scenarios)
- templates/agents/evaluator-headless.md (created in Phase 1)
- templates/agents/evaluator.md (modified in Phase 1)
- commands/sdlc/plan.md (modified in Phase 2)
- commands/config.md (evaluator keys)
- test/experiments/directive-effect/run.sh (modified in Phase 3)
- test/experiments/lib/evaluator-system-prompt.md (created in Phase 3)
- test/helpers.sh (assertion functions)
- test/test-v040-plan-harness.sh (SPEC-001 test pattern to follow)

Create `test/test-v040-evaluator.sh`:

```bash
#!/bin/bash
# Test: v0.4.0 SPEC-002 evaluator — config, headless prompt, plan integration, experiment runner
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"
```

Write these test sections:

--- TEST 1: Headless evaluator prompt ---
- evaluator-headless.md exists
- evaluator-headless.md has NO YAML frontmatter (first line is NOT "---")
- evaluator-headless.md contains skeptical stance ("assume.*incomplete" or "assume.*violations")
- evaluator-headless.md contains output format instructions ("PHASE EVALUATION")
- evaluator-headless.md contains zero context note ("zero.*context" or "no.*shared.*context")
- evaluator-headless.md contains "NEVER modify code"
- evaluator-headless.md contains pre-flight mode section

--- TEST 2: Subagent evaluator updated ---
- evaluator.md has comment pointing to headless ("evaluator-headless.md")

--- TEST 3: Config command has evaluator keys ---
- config.md contains "evaluator.preflight"
- config.md contains "evaluator.phase-end"
- config.md contains "evaluator.mode"
- config.md contains "evaluator.max-attempts"
- config.md contains "evaluator.model"

--- TEST 4: Plan command integration ---
- plan.md references "evaluator.preflight"
- plan.md references "evaluator.phase-end"
- plan.md references "evaluator.mode"
- plan.md has headless invocation with "--bare"
- plan.md has "--disallowedTools"
- plan.md has evaluator file existence check ("Evaluator template missing")
- plan.md has toggle skip message ("evaluator.preflight: false" or "skipped")

--- TEST 5: Experiment runner ---
- run.sh contains "--llm-eval"
- run.sh contains "evaluator-criteria.yaml"
- run.sh contains "WEAK PASS"
- run.sh contains "claude -p"
- run.sh contains "verdict_source"

--- TEST 6: Experiment evaluator system prompt ---
- test/experiments/lib/evaluator-system-prompt.md exists
- Contains skeptical stance
- Contains severity rules ("critical", "important", "informational")
- Contains output format ("EXPERIMENT EVALUATION")

--- TEST 7: Website evaluator docs ---
- website/governance/evaluator.md exists
- Contains "headless"
- Contains "subagent"
- Contains config reference ("evaluator.preflight")

End with test_summary. Run ./test/run.sh to verify all suites pass.

When complete, output: EVALUATOR TESTS DONE
```

---

## Known Risks

Pre-flight: no specialist domains detected — this plan modifies markdown templates, a shell hook, and a shell script. No database, security, or API concerns.

Note: Phase 3 modifies a gitignored file (`test/experiments/directive-effect/run.sh`). The implementer must have this file locally. If it doesn't exist, Phase 3 cannot execute — check first.
