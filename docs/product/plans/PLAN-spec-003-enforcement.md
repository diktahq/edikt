# Plan: SPEC-003 — Quality Gate UX Completion and Artifact Lifecycle Enforcement

## Overview
**Task:** Complete quality gate override flow and enforce artifact lifecycle across plan, drift, and doctor
**Spec:** SPEC-003 (docs/product/specs/SPEC-003-enforcement/spec.md)
**Total Phases:** 3
**Estimated Cost:** $0.24
**Created:** 2026-04-11

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | done    | 1/3    | 2026-04-11 |
| 2     | done    | 1/3    | 2026-04-11 |
| 3     | done    | 1/3    | 2026-04-11 |

**IMPORTANT:** Update this table as phases complete. This table is the persistent state that survives context compaction.

## Model Assignment

| Phase | Task | Model | Reasoning | Est. Cost |
|-------|------|-------|-----------|-----------|
| 1 | Quality gate UX | Sonnet | Hook logic + systemMessage format | $0.08 |
| 2 | Artifact lifecycle | Sonnet | Plan/drift/doctor command changes | $0.08 |
| 3 | Tests | Sonnet | Structural + functional tests | $0.08 |

## Execution Strategy

| Phase | Depends On | Parallel With |
|-------|-----------|---------------|
| 1     | None      | 2             |
| 2     | None      | 1             |
| 3     | 1, 2      | -             |

## Artifact Flow

| Producing Phase | Artifact | Consuming Phase(s) |
|-----------------|----------|---------------------|
| 1 | Updated `templates/hooks/subagent-stop.sh` | 3 |
| 1 | Updated `templates/hooks/session-start.sh` | 3 |
| 2 | Updated `commands/sdlc/plan.md` | 3 |
| 2 | Updated `commands/sdlc/drift.md` | 3 |
| 2 | Updated `commands/doctor.md` | 3 |

---

## Phase 1: Quality Gate UX

**Objective:** Add override logging to events.jsonl, re-fire prevention via session-scoped overrides, and session cleanup
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `GATE UX DONE`
**Evaluate:** true
**Dependencies:** None

**Context Needed:**
- `templates/hooks/subagent-stop.sh` — current gate logic
- `templates/hooks/session-start.sh` — session startup hook
- `docs/product/specs/SPEC-003-enforcement/spec.md` — sections 1 and 2

**Acceptance Criteria:**
- [ ] AC-1.1: subagent-stop.sh systemMessage includes explicit JSON format for events.jsonl logging with git identity fields
- [ ] AC-1.2: subagent-stop.sh writes `gate_fired` event to `~/.edikt/events.jsonl` (extend existing edikt_log_event)
- [ ] AC-1.3: systemMessage instructs Claude to write `gate_override` or `gate_blocked` event on user response
- [ ] AC-1.4: systemMessage includes exact JSON format with ts, event, agent, finding, user, email fields
- [ ] AC-1.5: subagent-stop.sh checks `~/.edikt/gate-overrides.jsonl` before firing gate
- [ ] AC-1.6: Override check matches on agent name + finding prefix (first 80 chars)
- [ ] AC-1.7: If override found, hook returns `{"continue": true}` silently
- [ ] AC-1.8: session-start.sh clears `~/.edikt/gate-overrides.jsonl` at session start
- [ ] AC-1.9: systemMessage instructs Claude to write override entry to gate-overrides.jsonl on YES

**Prompt:**
```
Read these files first:
- templates/hooks/subagent-stop.sh (the SubagentStop hook — read ALL of it)
- templates/hooks/session-start.sh (the SessionStart hook — read ALL of it)
- docs/product/specs/SPEC-003-enforcement/spec.md (sections 1: Quality Gate Override Logging, 2: Gate Re-fire Prevention)

Modify `templates/hooks/subagent-stop.sh`:

1. OVERRIDE CHECK (add BEFORE the gate firing block, ~line 101):
   Before checking if the agent is a gate and firing, check for existing override:
   ```bash
   # Check for existing override in this session
   FINDING_PREFIX=$(echo "$FINDING" | cut -c1-80)
   if [ -f "$HOME/.edikt/gate-overrides.jsonl" ]; then
     if grep -qF "\"agent\":\"${AGENT_NAME}\"" "$HOME/.edikt/gate-overrides.jsonl" 2>/dev/null; then
       if grep -qF "\"finding_prefix\":\"${FINDING_PREFIX}\"" "$HOME/.edikt/gate-overrides.jsonl" 2>/dev/null; then
         # Already overridden this session — skip silently
         printf '{"continue": true}'
         exit 0
       fi
     fi
   fi
   ```

2. EVENTS.JSONL LOGGING (update the gate_fired logging, ~line 104):
   The existing `edikt_log_event` call logs to session-signals.log. Add a SECOND write to events.jsonl:
   ```bash
   # Write to events.jsonl (structured audit log)
   TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
   mkdir -p "$HOME/.edikt" 2>/dev/null || true
   echo "{\"ts\":\"${TIMESTAMP}\",\"event\":\"gate_fired\",\"agent\":\"${AGENT_NAME}\",\"severity\":\"critical\",\"finding\":\"${ESCAPED_FINDING}\"}" >> "$HOME/.edikt/events.jsonl"
   ```

3. UPDATED SYSTEMMESSAGE (replace the GATE_MSG at ~line 111):
   ```bash
   GIT_USER=$(git config user.name 2>/dev/null || echo "unknown")
   GIT_EMAIL=$(git config user.email 2>/dev/null || echo "unknown")
   
   GATE_MSG="GATE BLOCKED: ${AGENT_NAME} found a critical issue: ${FINDING}.

Present this to the user:

⛔ GATE: ${AGENT_NAME} — critical finding
   ${FINDING}

   This gate must be resolved before proceeding.
   Override this gate? (y/n)
   Note: override will be logged with your git identity.

If the user says YES:
1. Write this EXACT line to ~/.edikt/events.jsonl:
   {\"ts\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"event\":\"gate_override\",\"agent\":\"${AGENT_NAME}\",\"finding\":\"${ESCAPED_FINDING}\",\"user\":\"${GIT_USER}\",\"email\":\"${GIT_EMAIL}\"}
2. Write this EXACT line to ~/.edikt/gate-overrides.jsonl:
   {\"ts\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"agent\":\"${AGENT_NAME}\",\"finding_prefix\":\"${FINDING_PREFIX}\"}
3. Confirm: Gate overridden. Logged to events.jsonl. Proceeding.

If the user says NO:
1. Write this EXACT line to ~/.edikt/events.jsonl:
   {\"ts\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"event\":\"gate_blocked\",\"agent\":\"${AGENT_NAME}\",\"finding\":\"${ESCAPED_FINDING}\",\"user\":\"${GIT_USER}\",\"email\":\"${GIT_EMAIL}\"}
2. Stop and let the user fix the issue."
   ```

Modify `templates/hooks/session-start.sh`:

Add near the top (after the edikt project check):
```bash
# Clear gate overrides from previous session
> "$HOME/.edikt/gate-overrides.jsonl" 2>/dev/null || true
```

When complete, output: GATE UX DONE
```

---

## Phase 2: Artifact Lifecycle Enforcement

**Objective:** Strengthen plan draft warning, add drift status filtering with auto-promote, extend doctor stale draft detection
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `LIFECYCLE DONE`
**Evaluate:** true
**Dependencies:** None

**Context Needed:**
- `commands/sdlc/plan.md` — current draft artifact warning (~line 59)
- `commands/sdlc/drift.md` — no current status filtering
- `commands/doctor.md` — current stale draft check (~line 330)
- `docs/product/specs/SPEC-003-enforcement/spec.md` — sections 3-8

**Acceptance Criteria:**
- [ ] AC-2.1: plan.md draft artifact warning lists specific artifact names and statuses
- [ ] AC-2.2: plan.md offers two options when draft artifacts found (proceed with Known Risks, or stop)
- [ ] AC-2.3: plan.md adds Known Risks section when user proceeds with draft artifacts
- [ ] AC-2.4: plan.md auto-promotes artifact status `accepted → in-progress` when phase starts
- [ ] AC-2.5: drift.md filters artifacts by status — skips `draft` with "Skipping" note
- [ ] AC-2.6: drift.md skips `superseded` artifacts with note
- [ ] AC-2.7: drift.md auto-promotes `in-progress → implemented` when no violations found
- [ ] AC-2.8: doctor.md extends stale draft check to cover spec-artifacts (not just PRDs and specs)
- [ ] AC-2.9: doctor.md parses both YAML frontmatter (`status: draft`) and comment header (`status=draft`) formats

**Prompt:**
```
Read these files first:
- commands/sdlc/plan.md (read ALL — especially step 3 governance chain, ~line 57-65)
- commands/sdlc/drift.md (read ALL — look for where agents are spawned to validate artifacts)
- commands/doctor.md (read ALL — especially line 330 area for stale draft check)
- docs/product/specs/SPEC-003-enforcement/spec.md (sections 4-8)

Modify `commands/sdlc/plan.md`:

1. STRENGTHEN DRAFT WARNING (step 3, ~line 59):
   Replace the current "If any have `status: draft`, warn and ask to proceed" with:
   ```
   If any spec-artifacts have `status: draft` in their frontmatter or comment header:
   
   ⚠️ These spec artifacts are still in draft:
      - {artifact name} (status: draft)
      - {artifact name} (status: draft)

      Draft artifacts haven't been reviewed. Planning against them
      risks implementing a design that changes after review.

      Options:
      1. Proceed anyway (plan will note artifacts are unreviewed)
      2. Stop — review and accept artifacts first
   
   If the user picks 1, add a Known Risks section to the generated plan:
   ## Known Risks
   - Planning against draft artifacts: {list}
     These may change after review. Re-plan if they do.
   ```

2. AUTO-PROMOTE ACCEPTED → IN-PROGRESS (add to phase execution instructions):
   In the Phase-End Flow or phase startup instructions, add:
   ```
   When starting a phase that references spec artifacts:
   - For each referenced artifact with status: accepted, update its frontmatter/comment header to status: in-progress
   - Do not update artifacts already in-progress or implemented
   - Log: "Status promoted: {artifact} accepted → in-progress"
   ```

Modify `commands/sdlc/drift.md`:

1. ARTIFACT STATUS FILTER (add as a new step BEFORE agents are spawned for validation):
   ```
   Before validating artifacts, filter by status:
   - Read each artifact's status from frontmatter or comment header
   - accepted → validate (this is the design intent)
   - implemented → validate (verify it's still correct)
   - in-progress → validate (check partial implementation)
   - draft → SKIP with note:
     ⏭ Skipping {artifact name} (status: draft) — accept before validating
   - superseded → SKIP with note:
     ⏭ Skipping {artifact name} (status: superseded)
   
   Only pass non-skipped artifacts to the validation agents.
   ```

2. AUTO-PROMOTE IN-PROGRESS → IMPLEMENTED (add after drift validation completes):
   ```
   After drift validation completes for an artifact:
   - If ALL criteria pass (zero violations, zero divergences) AND artifact status is in-progress:
     Update status to implemented
     Output: ✅ {artifact name} — no drift detected. Status promoted: in-progress → implemented
   - Only promote in-progress → implemented. Do NOT promote accepted → implemented directly.
   ```

   Note: status lives in different formats depending on artifact type:
   - .mmd files: `%% edikt:artifact ... status=draft`
   - .yaml files: `# edikt:artifact ... status=draft`
   - .sql files: `-- edikt:artifact ... status=draft`
   - .md files: YAML frontmatter `status: draft` or HTML comment `<!-- edikt:artifact ... status=draft -->`

Modify `commands/doctor.md`:

1. EXTEND STALE DRAFT CHECK (~line 330):
   Current check: "Check for PRDs or specs stuck in `draft` status for more than 7 days (based on file mtime)"
   
   Extend to ALSO check spec-artifacts:
   ```
   For each spec directory in {specs_path}/SPEC-*/:
     For each file that is NOT spec.md:
       Read status from frontmatter or comment header (support all 4 formats: %%, #, --, <!-- -->)
       If status is "draft" and file mtime > 7 days:
         [!!] {spec_dir}/{artifact} has been draft for {n} days — review and accept, or remove
   ```
   
   The existing PRD/spec check stays — this adds spec-artifact checking alongside it.

When complete, output: LIFECYCLE DONE
```

---

## Phase 3: Tests

**Objective:** Structural and functional tests for gate UX and artifact lifecycle changes
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `ENFORCEMENT TESTS DONE`
**Evaluate:** true
**Dependencies:** Phase 1, Phase 2

**Context Needed:**
- `templates/hooks/subagent-stop.sh` — modified in Phase 1
- `templates/hooks/session-start.sh` — modified in Phase 1
- `commands/sdlc/plan.md` — modified in Phase 2
- `commands/sdlc/drift.md` — modified in Phase 2
- `commands/doctor.md` — modified in Phase 2
- `test/helpers.sh` — assertion functions
- `test/test-v040-plan-harness.sh` — SPEC-001 test pattern
- `test/test-v040-evaluator.sh` — SPEC-002 test pattern

**Acceptance Criteria:**
- [ ] AC-3.1: `test/test-v040-enforcement.sh` exists and is executable
- [ ] AC-3.2: Tests verify subagent-stop.sh has events.jsonl write
- [ ] AC-3.3: Tests verify subagent-stop.sh has override check (gate-overrides.jsonl)
- [ ] AC-3.4: Tests verify subagent-stop.sh systemMessage has git identity fields
- [ ] AC-3.5: Tests verify session-start.sh clears gate-overrides.jsonl
- [ ] AC-3.6: Tests verify plan.md has strengthened draft warning with specific artifact listing
- [ ] AC-3.7: Tests verify plan.md has Known Risks section instruction
- [ ] AC-3.8: Tests verify plan.md has auto-promote accepted → in-progress
- [ ] AC-3.9: Tests verify drift.md has status filter (skips draft, skips superseded)
- [ ] AC-3.10: Tests verify drift.md has auto-promote in-progress → implemented
- [ ] AC-3.11: Tests verify doctor.md extends stale draft check to spec-artifacts
- [ ] AC-3.12: All existing test suites still pass (`./test/run.sh`)

**Prompt:**
```
Read these files first:
- docs/product/plans/PLAN-spec-003-enforcement.md (the full plan — you're executing Phase 3)
- templates/hooks/subagent-stop.sh (modified in Phase 1)
- templates/hooks/session-start.sh (modified in Phase 1)
- commands/sdlc/plan.md (modified in Phase 2)
- commands/sdlc/drift.md (modified in Phase 2)
- commands/doctor.md (modified in Phase 2)
- test/helpers.sh (assertion functions)
- test/test-v040-plan-harness.sh (SPEC-001 test pattern to follow)
- test/test-v040-evaluator.sh (SPEC-002 test pattern to follow)

IMPORTANT: Phases 1 and 2 must be done. Check the progress table.

Create `test/test-v040-enforcement.sh`:

```bash
#!/bin/bash
# Test: v0.4.0 SPEC-003 enforcement — quality gate UX, artifact lifecycle
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

HOOK="$PROJECT_ROOT/templates/hooks/subagent-stop.sh"
SESSION_HOOK="$PROJECT_ROOT/templates/hooks/session-start.sh"
PLAN_CMD="$PROJECT_ROOT/commands/sdlc/plan.md"
DRIFT_CMD="$PROJECT_ROOT/commands/sdlc/drift.md"
DOCTOR_CMD="$PROJECT_ROOT/commands/doctor.md"
```

Write these test sections:

--- TEST 1: Quality gate override logging ---

- subagent-stop.sh writes to events.jsonl:
  assert_file_contains "$HOOK" "events.jsonl"

- subagent-stop.sh has git identity in systemMessage:
  assert_file_contains "$HOOK" "git config user.name"
  assert_file_contains "$HOOK" "git config user.email"

- systemMessage has gate_override event format:
  assert_file_contains "$HOOK" "gate_override"

- systemMessage has gate_blocked event format:
  assert_file_contains "$HOOK" "gate_blocked"

--- TEST 2: Gate re-fire prevention ---

- subagent-stop.sh checks gate-overrides.jsonl:
  assert_file_contains "$HOOK" "gate-overrides.jsonl"

- Override check matches on agent name:
  assert_file_contains "$HOOK" "AGENT_NAME"

- Override check matches on finding prefix:
  assert_file_contains "$HOOK" "finding_prefix"
  assert_file_contains "$HOOK" "cut -c1-80"

- If override found, returns continue:
  (structural — verify the override check block exists and returns continue)

--- TEST 3: Session cleanup ---

- session-start.sh clears gate-overrides.jsonl:
  assert_file_contains "$SESSION_HOOK" "gate-overrides.jsonl"

--- TEST 4: Plan draft artifact warning ---

- plan.md has strengthened warning with artifact listing:
  assert_file_contains "$PLAN_CMD" "still in draft"
  assert_file_contains "$PLAN_CMD" "status: draft"

- plan.md offers proceed or stop:
  assert_file_contains "$PLAN_CMD" "Proceed anyway"
  assert_file_contains "$PLAN_CMD" "review and accept"

- plan.md has Known Risks section:
  assert_file_contains "$PLAN_CMD" "Known Risks"
  assert_file_contains "$PLAN_CMD" "draft artifacts"

--- TEST 5: Plan auto-promote ---

- plan.md has accepted → in-progress promotion:
  assert_file_contains "$PLAN_CMD" "in-progress"
  assert_file_contains "$PLAN_CMD" "Status promoted"

--- TEST 6: Drift status filter ---

- drift.md skips draft artifacts:
  assert_file_contains "$DRIFT_CMD" "draft"
  assert_file_contains "$DRIFT_CMD" "Skipping"

- drift.md skips superseded artifacts:
  assert_file_contains "$DRIFT_CMD" "superseded"

- drift.md validates accepted and implemented:
  assert_file_contains "$DRIFT_CMD" "accepted"
  assert_file_contains "$DRIFT_CMD" "implemented"

--- TEST 7: Drift auto-promote ---

- drift.md has in-progress → implemented promotion:
  assert_file_contains "$DRIFT_CMD" "implemented"
  assert_file_contains "$DRIFT_CMD" "no drift"

--- TEST 8: Doctor stale draft detection ---

- doctor.md checks spec-artifacts for stale drafts:
  assert_file_contains "$DOCTOR_CMD" "SPEC-"
  (verify doctor references spec directories, not just PRDs/specs)

- doctor.md parses comment header formats:
  Check that doctor references multiple status formats (at least two of: %%, #, --, <!--)

--- END ---

End with test_summary. Run ./test/run.sh to verify all suites pass.

When complete, output: ENFORCEMENT TESTS DONE
```

---

## Known Risks

Pre-flight: no specialist domains detected — this plan modifies markdown command templates and shell hooks. No database, security, or API concerns.

Note: The systemMessage approach for gate override logging relies on Claude following instructions to write JSON files. If Claude doesn't follow the format exactly, events.jsonl entries may be malformed. Mitigation: the spec includes exact JSON format strings in the systemMessage.
