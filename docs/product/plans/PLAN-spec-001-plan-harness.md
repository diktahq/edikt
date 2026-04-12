# Plan: SPEC-001 — Plan Harness Improvements

## Overview
**Task:** Implement iteration tracking, context handoff, and criteria sidecar for the plan command
**Spec:** SPEC-001 (docs/product/specs/SPEC-001-plan-harness/spec.md)
**Total Phases:** 4
**Estimated Cost:** $0.32
**Created:** 2026-04-11

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | done   | 1/5     | 2026-04-11 |
| 2     | done   | 1/5     | 2026-04-11 |
| 3     | done   | 1/3     | 2026-04-11 |
| 4     | done   | 1/3     | 2026-04-11 |

**IMPORTANT:** Update this table as phases complete. This table is the persistent state that survives context compaction.

## Model Assignment

| Phase | Task | Model | Reasoning | Est. Cost |
|-------|------|-------|-----------|-----------|
| 1 | Iteration tracking + backoff | Sonnet | Plan template changes + backoff logic in markdown | $0.08 |
| 2 | Phase context handoff | Sonnet | Plan template additions + format examples | $0.08 |
| 3 | Criteria sidecar + PostCompact hook | Sonnet | New sidecar emission step + shell regex updates | $0.08 |
| 4 | Tests | Sonnet | Structural tests + hook regex tests | $0.08 |

## Execution Strategy

| Phase | Depends On | Parallel With |
|-------|-----------|---------------|
| 1     | None      | 2             |
| 2     | None      | 1             |
| 3     | 1         | -             |
| 4     | 1, 2, 3   | -             |

## Artifact Flow

| Producing Phase | Artifact | Consuming Phase(s) |
|-----------------|----------|---------------------|
| 1 | Updated progress table format + status values + backoff logic in `commands/sdlc/plan.md` | 3, 4 |
| 2 | Context Needed field + Artifact Flow Table in `commands/sdlc/plan.md` | 3, 4 |
| 3 | Sidecar emission step in `commands/sdlc/plan.md` + updated `templates/hooks/post-compact.sh` | 4 |

---

## Phase 1: Iteration Tracking + Backoff

**Objective:** Add Attempt column to progress table, define status values, implement backoff logic in plan command
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `ITERATION TRACKING DONE`
**Evaluate:** true
**Dependencies:** None

**Acceptance Criteria:**
- [ ] AC-1.1: Plan File Template in Reference section shows `| Phase | Status | Attempt | Updated |`
- [ ] AC-1.2: Reference section documents all 6 status values: `pending`, `in-progress`, `evaluating`, `stuck`, `done`, `skipped`
- [ ] AC-1.3: Phase-End Flow section includes: after FAIL, increment Attempt, read criteria sidecar, forward fail reasons to next generator prompt
- [ ] AC-1.4: Phase-End Flow includes escalation warning when same criterion ID has failed 3 consecutive times
- [ ] AC-1.5: Phase-End Flow includes `stuck` status transition at max attempts with human decision prompt (4 options: continue, skip, rewrite, stop)
- [ ] AC-1.6: Plan command reads `evaluator.max-attempts` from `.edikt/config.yaml` (default 5)
- [ ] AC-1.7: Plan File Template Progress section shows `attempt: "0/5"` format in initial state

**Prompt:**
```
Modify `commands/sdlc/plan.md` to add iteration tracking and backoff logic. Read the full file first.

1. PROGRESS TABLE (Reference section, ~line 310):
   Change the Plan File Template from:
     | Phase | Status | Updated |
   To:
     | Phase | Status | Attempt | Updated |
   Initial values: status = `pending`, attempt = `0/{max}` where max comes from `evaluator.max-attempts` config (default 5).

2. STATUS VALUES (add new section in Reference):
   Add a "### Status Values" section documenting:
   - `pending` — not started
   - `in-progress` — generator is working
   - `evaluating` — phase-end evaluator is running
   - `done` — all acceptance criteria PASS
   - `stuck` — max attempts reached, human decision needed
   - `skipped` — explicitly skipped by user

3. PHASE-END FLOW (Reference section, ~line 218):
   After the existing "If FAIL: report failures" instruction, add:
   
   a. Increment the Attempt column in the progress table (e.g., "1/5" → "2/5")
   
   b. Read the criteria sidecar file (`PLAN-{slug}-criteria.yaml`) if it exists.
      For each failing criterion, check `fail_count`. If `fail_count >= 3`:
      ```
      ⚠️ AC-{id} has failed 3 consecutive times.
         Last reason: {fail_reason}
         Consider: rewrite the criterion, adjust the approach, or ask for help.
      ```
   
   c. Before retrying, include failing criteria in the generator prompt:
      "Previous attempt failed. Fix these: {list of failing AC IDs and reasons}"
   
   d. If the Attempt value reaches max (e.g., "5/5"):
      Set status to `stuck`. Output:
      ```
      Phase {n} is stuck after {max} attempts.
      Options:
        1. Continue trying (increase max)
        2. Skip this phase
        3. Rewrite failing criteria
        4. Stop and review
      ```

4. CONFIG READ (step 1, ~line 37):
   Add instruction to read `evaluator.max-attempts` from `.edikt/config.yaml`.
   Default to 5 if not set. Store as MAX_ATTEMPTS for use in progress table and stuck detection.

When complete, output: ITERATION TRACKING DONE
```

---

## Phase 2: Phase Context Handoff

**Objective:** Add Context Needed field per phase, Artifact Flow Table, and phase startup directive to plan command
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `CONTEXT HANDOFF DONE`
**Evaluate:** true
**Dependencies:** None

**Acceptance Criteria:**
- [ ] AC-2.1: Phase Structure in Reference section includes `Context Needed:` as a required field per phase
- [ ] AC-2.2: Plan File Template includes an `## Artifact Flow` section between dependency graph and Phase 1
- [ ] AC-2.3: Artifact Flow Table has columns: `Producing Phase | Artifact | Consuming Phase(s)`
- [ ] AC-2.4: Step 6 (phase generation) includes instruction to populate Context Needed by analyzing spec artifacts, dependency phase outputs, and referenced ADRs
- [ ] AC-2.5: Reference section includes a phase startup directive: "Before implementing any plan phase, read every file in Context Needed"
- [ ] AC-2.6: Context Needed field lists specific file paths, not generic descriptions

**Prompt:**
```
Modify `commands/sdlc/plan.md` to add phase context handoff. Read the full file first. Also read `docs/product/specs/SPEC-001-plan-harness/phase-context-handoff-example.md` for the target format.

1. PHASE STRUCTURE (Reference section, ~line 187):
   Add `Context Needed:` to the list of required fields per phase. Description:
   "Context Needed (list of file paths the generator must read before starting this phase — spec artifacts, outputs from dependency phases, referenced ADRs)"

2. ARTIFACT FLOW TABLE (Plan File Template, ~line 310):
   Add an `## Artifact Flow` section to the template, placed AFTER the Execution Strategy table and BEFORE Phase 1. Format:
   ```markdown
   ## Artifact Flow

   | Producing Phase | Artifact | Consuming Phase(s) |
   |-----------------|----------|---------------------|
   | {n} | `{file path}` | {phase numbers} |
   ```

3. PHASE GENERATION (step 6, ~line 122):
   Add instruction: when generating each phase, populate the `Context Needed:` field by:
   - Scanning spec artifacts referenced by the phase
   - Identifying files produced by dependency phases (from the Artifact Flow Table)
   - Including any ADRs referenced in the spec
   Each entry must be a specific file path with a brief description of why it's needed.

4. PHASE STARTUP DIRECTIVE (Reference section, add new subsection):
   Add "### Phase Startup Directive":
   ```
   Before implementing any plan phase:
   1. Read every file listed in that phase's Context Needed section.
   2. If a listed file does not exist, check the progress table — the producing phase may not be complete.
   3. Do not proceed until all context files have been read.
   4. After reading, confirm you understand the relevant decisions and constraints before writing code.
   ```

5. PHASE TEMPLATE (in each phase's markdown structure):
   Add Context Needed field after Dependencies. Example:
   ```markdown
   **Context Needed:**
   - `docs/product/specs/SPEC-005/contracts/api-orders.yaml` — API contract
   - `internal/repository/orders.go` — repository from Phase 2
   - `docs/architecture/decisions/ADR-012.md` — error handling decision
   ```

When complete, output: CONTEXT HANDOFF DONE
```

---

## Phase 3: Criteria Sidecar + PostCompact Hook

**Objective:** Add sidecar emission to plan command and update PostCompact hook to extract attempt count, context files, and failing criteria
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `SIDECAR AND HOOK DONE`
**Evaluate:** true
**Dependencies:** Phase 1

**Acceptance Criteria:**
- [ ] AC-3.1: Plan command has a step 8b that emits `PLAN-{slug}-criteria.yaml` alongside the plan file
- [ ] AC-3.2: Sidecar schema matches `docs/product/specs/SPEC-001-plan-harness/plan-criteria-schema.yaml` — has `plan`, `generated`, `last_evaluated`, `phases[].criteria[].id/description/status/verify/fail_reason/fail_count`
- [ ] AC-3.3: All criteria in sidecar start with `status: pending` and `fail_count: 0`
- [ ] AC-3.4: Phase-End Flow includes instruction to update sidecar after evaluation (status, fail_reason, fail_count, last_evaluated)
- [ ] AC-3.5: PostCompact hook regex extracts Attempt column from `| Phase | Status | Attempt | Updated |` table
- [ ] AC-3.6: PostCompact output includes attempt count: "Phase 3 — API handlers (attempt 2/5)"
- [ ] AC-3.7: PostCompact reads criteria sidecar and includes last failing criteria in output
- [ ] AC-3.8: PostCompact reads Context Needed from active phase and includes file list in output
- [ ] AC-3.9: PostCompact output matches the format in `phase-context-handoff-example.md` "After" section
- [ ] AC-3.10: Sidecar file is always a sibling of the plan file (same directory)

**Prompt:**
```
Modify two files: `commands/sdlc/plan.md` and `templates/hooks/post-compact.sh`. Read both fully first. Also read these reference files:
- `docs/product/specs/SPEC-001-plan-harness/plan-criteria-schema.yaml` — sidecar schema
- `docs/product/specs/SPEC-001-plan-harness/phase-context-handoff-example.md` — PostCompact format

PART A: Plan command sidecar emission (`commands/sdlc/plan.md`)

1. Add step 8b after step 8 (write plan file):
   "After writing the plan markdown, emit `PLAN-{slug}-criteria.yaml` in the same directory.
   
   For each phase:
   - Extract acceptance criteria from the plan text
   - Assign IDs: AC-{phase}.{seq} (e.g., AC-1.1, AC-1.2)
   - If pre-flight ran (step 11), populate `verify` with proposed commands
   - Set all `status: pending`, `fail_count: 0`, `fail_reason: null`, `last_evaluated: null`
   
   The sidecar file is always a sibling of the plan file:
   `docs/product/plans/PLAN-foo.md` → `docs/product/plans/PLAN-foo-criteria.yaml`"

2. Add to Phase-End Flow (after the backoff logic added in Phase 1):
   "After evaluation, update the criteria sidecar:
   - Read `PLAN-{slug}-criteria.yaml`
   - For each criterion: update `status` (pass/fail), `last_evaluated` (ISO date), `fail_reason` (if fail)
   - Increment `fail_count` for fails (reset to 0 on pass)
   - Update phase-level `status` and `attempt`
   - Write back"

PART B: PostCompact hook (`templates/hooks/post-compact.sh`)

The current regex at line 21:
```bash
PHASE=$(grep -iE '\| *(Phase )?[0-9]+ *\|.*in[_ -]progress' "$PLAN" 2>/dev/null | head -1)
```

And the extraction at lines 23-28 splits on `|` and takes column 2 (phase number) and column 3 (status).

With the new 4-column table `| Phase | Status | Attempt | Updated |`, column 3 is now Attempt (not status). Update the extraction:

1. Keep the grep pattern — it still matches (status is in column 2, which the grep sees).

2. Update column extraction:
   - Column 2 → Phase number (unchanged)
   - Column 3 → Status text (was already this, but verify after table format change — status is column 2 in the pipe-split output, since the leading `|` creates an empty column 1)
   - Column 4 → Attempt (NEW — extract as ATTEMPT variable)

   Test with this table row: `| 3     | in-progress | 2/5 | 2026-04-11 |`
   After `sed 's/|/\n/g'`:
   - Line 1: empty (before first |)
   - Line 2: ` 3     ` → PHASE_NUM
   - Line 3: ` in-progress ` → PHASE_THEME (status)
   - Line 4: ` 2/5 ` → ATTEMPT (NEW)
   - Line 5: ` 2026-04-11 ` → date (ignored)

   Add: `ATTEMPT=$(echo "$PHASE" | sed 's/|/\n/g' | sed -n '4p' | sed 's/^ *//;s/ *$//')`

3. Read criteria sidecar for failing criteria:
   ```bash
   CRITERIA_FILE="${PLAN%.md}-criteria.yaml"
   FAIL_MSG=""
   if [ -f "$CRITERIA_FILE" ]; then
     FAILS=$(grep -B1 'status: fail' "$CRITERIA_FILE" 2>/dev/null | grep 'id:' | awk '{print $2}')
     REASONS=$(grep -A1 'status: fail' "$CRITERIA_FILE" 2>/dev/null | grep 'fail_reason:' | sed 's/.*fail_reason: //' | head -3)
     if [ -n "$FAILS" ]; then
       FAIL_MSG="Last failing criteria: $(echo $FAILS | tr '\n' ', ' | sed 's/,$//')"
     fi
   fi
   ```

4. Read Context Needed from the active phase in the plan file:
   ```bash
   CONTEXT_MSG=""
   if [ -n "$PHASE_NUM" ]; then
     CONTEXT=$(sed -n "/## Phase ${PHASE_NUM}:/,/## Phase/p" "$PLAN" 2>/dev/null | grep -A20 'Context Needed:' | grep '^ *-' | head -5 | sed 's/^ *- /  - /')
     if [ -n "$CONTEXT" ]; then
       CONTEXT_MSG="Before continuing, read:
   ${CONTEXT}"
     fi
   fi
   ```

5. Update the output format (line 26-28):
   ```bash
   PLAN_MSG="Active plan: ${PLAN_NAME}. Phase ${PHASE_NUM}"
   [ -n "$PHASE_THEME" ] && PLAN_MSG="${PLAN_MSG} — ${PHASE_THEME}"
   [ -n "$ATTEMPT" ] && PLAN_MSG="${PLAN_MSG} (attempt ${ATTEMPT})"
   PLAN_MSG="${PLAN_MSG}."
   [ -n "$FAIL_MSG" ] && PLAN_MSG="${PLAN_MSG} ${FAIL_MSG}."
   [ -n "$CONTEXT_MSG" ] && PLAN_MSG="${PLAN_MSG} ${CONTEXT_MSG}"
   PLAN_MSG="${PLAN_MSG} Re-read ${PLAN} for full phase details."
   ```

   Final output should match the "After" format in the handoff example:
   ```
   Context recovered after compaction. Active plan: PLAN-foo.
   Phase 3 — API handlers (attempt 2/5).
   Last failing criteria: AC-3.2 (no tenant_id in log calls).
   Before continuing, read:
     - docs/product/specs/SPEC-foo/contracts/api-orders.yaml
     - internal/repository/orders.go
   Invariants (2): INV-001, INV-012.
   ```

IMPORTANT: The hook must gracefully handle plans that DON'T have the Attempt column (backward compatibility). If column 4 is empty or not a number/number pattern, set ATTEMPT="" and skip it in the output.

When complete, output: SIDECAR AND HOOK DONE
```

---

## Phase 4: Tests

**Objective:** Add structural tests for all Phase 1-3 changes and PostCompact hook regex tests
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `TESTS DONE`
**Evaluate:** true
**Dependencies:** Phase 1, Phase 2, Phase 3

**Acceptance Criteria:**
- [ ] AC-4.1: New test file `test/test-v040-plan-harness.sh` exists
- [ ] AC-4.2: Test verifies plan.md progress table template has `| Phase | Status | Attempt | Updated |`
- [ ] AC-4.3: Test verifies plan.md documents all 6 status values (pending, in-progress, evaluating, stuck, done, skipped)
- [ ] AC-4.4: Test verifies plan.md Phase-End Flow references criteria sidecar
- [ ] AC-4.5: Test verifies plan.md has fail forwarding instruction ("Previous attempt failed")
- [ ] AC-4.6: Test verifies plan.md has escalation warning ("failed 3 consecutive times")
- [ ] AC-4.7: Test verifies plan.md has stuck status with human decision prompt
- [ ] AC-4.8: Test verifies plan.md has Context Needed in Phase Structure requirements
- [ ] AC-4.9: Test verifies plan.md has Artifact Flow Table in template
- [ ] AC-4.10: Test verifies plan.md has sidecar emission step (step 8b or equivalent)
- [ ] AC-4.11: Test verifies plan.md has Phase Startup Directive
- [ ] AC-4.12: PostCompact hook extracts attempt from 4-column table — test with mock table row
- [ ] AC-4.13: PostCompact hook gracefully handles 3-column table (backward compat) — test with old format
- [ ] AC-4.14: PostCompact hook reads criteria sidecar for failing criteria
- [ ] AC-4.15: PostCompact hook reads Context Needed from plan
- [ ] AC-4.16: All existing test suites still pass (`./test/run.sh`)

**Prompt:**
```
Create `test/test-v040-plan-harness.sh` — structural tests for SPEC-001 plan harness changes. Follow the pattern in existing test files (source helpers.sh, use assert_file_contains/assert_file_exists, end with test_summary).

Read these files first:
- `test/helpers.sh` — assertion functions
- `test/test-v031-artifacts.sh` — recent test pattern to follow
- `commands/sdlc/plan.md` — the modified plan command
- `templates/hooks/post-compact.sh` — the modified hook

TESTS TO WRITE:

--- Iteration Tracking (Phase 1 verification) ---

1. Plan.md progress table has Attempt column:
   assert_file_contains plan.md "| Phase | Status | Attempt | Updated |"

2. Plan.md documents all 6 status values:
   for status in pending in-progress evaluating stuck done skipped; do
     assert_file_contains plan.md "$status"
   done

3. Plan.md phase-end flow references criteria sidecar:
   assert_file_contains plan.md "criteria.yaml"

4. Plan.md has fail forwarding:
   assert_file_contains plan.md "Previous attempt failed"

5. Plan.md has escalation at 3 failures:
   assert_file_contains plan.md "failed 3 consecutive"

6. Plan.md has stuck status with options:
   assert_file_contains plan.md "stuck"
   assert_file_contains plan.md "Continue trying"

7. Plan.md reads evaluator.max-attempts:
   assert_file_contains plan.md "max-attempts"

--- Context Handoff (Phase 2 verification) ---

8. Plan.md Phase Structure includes Context Needed:
   assert_file_contains plan.md "Context Needed"

9. Plan.md template has Artifact Flow section:
   assert_file_contains plan.md "Artifact Flow"
   assert_file_contains plan.md "Producing Phase"

10. Plan.md has phase startup directive:
    assert_file_contains plan.md "Before implementing any plan phase"

--- Criteria Sidecar (Phase 3 verification) ---

11. Plan.md has sidecar emission step:
    assert_file_contains plan.md "criteria.yaml"
    assert_file_contains plan.md "sibling"  # or "same directory"

12. Plan.md phase-end updates sidecar:
    assert_file_contains plan.md "fail_count"
    assert_file_contains plan.md "fail_reason"

--- PostCompact Hook Tests ---

13. PostCompact hook extracts attempt from 4-column table:
    Create a temp plan file with a 4-column progress table:
    ```
    | 3 | in-progress | 2/5 | 2026-04-11 |
    ```
    Run the hook's extraction logic (source the relevant section or simulate).
    Verify the output contains "attempt 2/5".

14. PostCompact hook handles 3-column table (backward compat):
    Create a temp plan file with old format:
    ```
    | 3 | in-progress | 2026-04-11 |
    ```
    Verify the hook doesn't crash and outputs without attempt info.

15. PostCompact hook reads criteria sidecar:
    assert_file_contains post-compact.sh "criteria.yaml"
    assert_file_contains post-compact.sh "fail"

16. PostCompact hook reads Context Needed:
    assert_file_contains post-compact.sh "Context Needed"

17. All existing tests pass:
    This is verified by running ./test/run.sh at the end.

Run ./test/run.sh to verify all suites pass (including the new one).

When complete, output: TESTS DONE
```

---

## Artifact Coverage

```
✓ phase-context-handoff-example.md → reference only, format examples (Phase 2, 3)
✓ plan-criteria-schema.yaml → reference only, sidecar schema (Phase 3)
— No implementation artifacts (fixtures, contracts, migrations) — all changes are to command templates and hooks
```

## Known Risks

None identified by pre-flight — this plan modifies markdown templates and a shell hook. No compiled code, no external services, no security concerns.
