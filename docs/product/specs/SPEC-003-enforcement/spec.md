---
type: spec
id: SPEC-003
title: "Quality Gate UX Completion and Artifact Lifecycle Enforcement"
status: accepted
author: Daniel Gomes
implements: PRD-001
source_prd: docs/product/prds/PRD-001-v040-harness-lifecycle-gates.md
created_at: 2026-04-11T03:30:00Z
references:
  adrs: [ADR-001, ADR-004, ADR-005]
  invariants: [INV-001]
---

# SPEC-003: Quality Gate UX Completion and Artifact Lifecycle Enforcement

**Implements:** PRD-001 (FR-015 through FR-021)
**Date:** 2026-04-11
**Author:** Daniel Gomes

---

## Summary

This spec completes the quality gate override flow (logged overrides, re-fire prevention) and makes artifact lifecycle enforcement uniform across plan, drift, and doctor commands. The gate hook gains structured logging and session-scoped override memory. SDLC commands gain status-aware artifact handling.

## Context

Quality gates shipped in v0.2.0 (PRD-005 R7). The SubagentStop hook (`templates/hooks/subagent-stop.sh`) detects critical findings from gate agents and blocks progression via a `systemMessage` that asks Claude to present the finding. Two pieces are missing: the override isn't logged with identity, and the gate re-fires on every subsequent agent response in the same session.

Artifact lifecycle states (`draft`, `accepted`) are used by the spec and artifacts commands as gates (spec requires PRD accepted, artifacts requires spec accepted). But the plan command's draft warning (line 59) is advisory only — it asks to proceed rather than blocking. Drift doesn't filter by status at all. Doctor checks for stale drafts on PRDs and specs but not on spec-artifacts.

## Existing Architecture

- **SubagentStop hook:** `templates/hooks/subagent-stop.sh` (~119 lines). Detects agent name, severity, and findings. Gate logic at lines 95-115: checks `gates:` config, fires `systemMessage` with block decision. Logs to `session-signals.log` but not to structured `events.jsonl`.
- **Plan command:** `commands/sdlc/plan.md`. Line 59: "Check for spec-artifacts in the spec folder. If any have `status: draft`, warn and ask to proceed." — advisory, not blocking.
- **Drift command:** `commands/sdlc/drift.md`. No artifact status filtering. Validates all artifacts regardless of status.
- **Doctor command:** `commands/doctor.md`. Line 330: "Check for PRDs or specs stuck in `draft` status for more than 7 days (based on file mtime)." Covers PRDs and specs but not spec-artifacts.

## Proposed Design

### 1. Quality Gate Override Logging (FR-015)

When a gate fires and the user overrides it, Claude MUST write an entry to `~/.edikt/events.jsonl`.

**Updated systemMessage format:**

```
GATE BLOCKED: {agent_name} found a critical issue: {finding}.

Present this to the user:

⛔ GATE: {agent_name} — critical finding
   {finding}

   This gate must be resolved before proceeding.
   Override this gate? (y/n)
   Note: override will be logged with your git identity.

If the user says YES:
1. Write this JSON line to ~/.edikt/events.jsonl:
   {"ts":"{ISO8601}","event":"gate_override","agent":"{agent_name}","finding":"{finding}","user":"{git user.name}","email":"{git user.email}"}
2. Write this JSON line to ~/.edikt/gate-overrides.jsonl:
   {"ts":"{ISO8601}","agent":"{agent_name}","finding_prefix":"{first 80 chars of finding}"}
3. Confirm: "Gate overridden. Logged to events.jsonl. Proceeding."

If the user says NO:
1. Write this JSON line to ~/.edikt/events.jsonl:
   {"ts":"{ISO8601}","event":"gate_blocked","agent":"{agent_name}","finding":"{finding}","user":"{git user.name}","email":"{git user.email}"}
2. Stop and let the user fix the issue.
```

**events.jsonl schema:**

```jsonl
{"ts":"2026-04-11T10:30:00Z","event":"gate_fired","agent":"security","severity":"critical","finding":"SQL injection in orders/handler.go:47"}
{"ts":"2026-04-11T10:31:00Z","event":"gate_override","agent":"security","finding":"SQL injection in orders/handler.go:47","user":"Daniel Gomes","email":"daniel@example.com"}
{"ts":"2026-04-11T10:45:00Z","event":"gate_blocked","agent":"dba","finding":"Missing rollback in migration 003","user":"Daniel Gomes","email":"daniel@example.com"}
```

The hook itself writes `gate_fired` events (already implemented via `edikt_log_event`). Claude writes `gate_override` and `gate_blocked` events via the systemMessage instructions.

**File location:** `~/.edikt/events.jsonl` — global, not per-project. Events include enough context (finding text, agent name) to trace back to the project. Global makes it easier to audit override patterns across all projects.

### 2. Gate Re-fire Prevention (FR-016)

Before firing a gate, the hook MUST check if the same finding was already overridden in this session.

**Override memory file:** `~/.edikt/gate-overrides.jsonl`

**Check logic in subagent-stop.sh:**

```bash
# Check for existing override (session = current Claude invocation)
SESSION_START=$(stat -f%m ~/.edikt/gate-overrides.jsonl 2>/dev/null || echo "0")
FINDING_PREFIX=$(echo "$FINDING" | cut -c1-80)

if [ -f ~/.edikt/gate-overrides.jsonl ]; then
  # Match: same agent + finding prefix starts with the stored prefix
  if grep -q "\"agent\":\"${AGENT_NAME}\"" ~/.edikt/gate-overrides.jsonl 2>/dev/null &&
     grep -q "\"finding_prefix\":\"${FINDING_PREFIX}\"" ~/.edikt/gate-overrides.jsonl 2>/dev/null; then
    # Already overridden — skip silently
    printf '{"continue": true}'
    exit 0
  fi
fi
```

**Session scoping:** A session is a single Claude Code invocation (start to exit). The `gate-overrides.jsonl` file is cleared at session start by the SessionStart hook. This ensures overrides don't carry across sessions.

**SessionStart hook addition** (`templates/hooks/session-start.sh`):

```bash
# Clear gate overrides from previous session
> ~/.edikt/gate-overrides.jsonl 2>/dev/null || true
```

### 3. Artifact Lifecycle States

**Full lifecycle:** `draft → accepted → in-progress → implemented → superseded`

| Transition | Trigger | Who |
|-----------|---------|-----|
| `draft → accepted` | User changes `status:` in frontmatter | Manual |
| `accepted → in-progress` | Plan starts a phase referencing this artifact | Auto (FR-020) |
| `in-progress → implemented` | Drift finds no violations for this artifact | Auto (FR-021) |
| `any → superseded` | User creates replacement artifact | Manual |

### 4. Plan: Strengthen Draft Artifact Warning (FR-017)

**File:** `commands/sdlc/plan.md`

Current behavior (line 59): "If any have `status: draft`, warn and ask to proceed."

New behavior: The warning MUST be more prominent and include the specific artifacts:

```
⚠️ These spec artifacts are still in draft:
   - data-model.mmd (status: draft)
   - contracts/api.yaml (status: draft)

   Draft artifacts haven't been reviewed. Planning against them
   risks implementing a design that changes after review.

   Options:
   1. Proceed anyway (plan will note artifacts are unreviewed)
   2. Stop — review and accept artifacts first
```

If the user proceeds, the plan MUST include a `## Known Risks` note:

```markdown
## Known Risks
- Planning against draft artifacts: data-model.mmd, contracts/api.yaml
  These may change after review. Re-plan if they do.
```

### 5. Plan: Auto-Promote Accepted → In-Progress (FR-020)

When a plan phase starts execution and references a spec artifact, the plan command SHOULD update that artifact's `status:` frontmatter from `accepted` to `in-progress`.

This is a SHOULD requirement — implemented as an instruction in the plan command, not enforced by a hook. The plan command's phase execution instructions MUST include:

```
When starting a phase that references spec artifacts:
- For each referenced artifact with status: accepted, update to status: in-progress
- Do not update artifacts that are already in-progress or implemented
```

### 6. Drift: Filter by Status (FR-018)

**File:** `commands/sdlc/drift.md`

Before validating artifacts against implementation, drift MUST filter by status:

- `accepted` — validate (this is the design intent)
- `implemented` — validate (verify it's still correct)
- `in-progress` — validate (check partial implementation)
- `draft` — **SKIP** with note:
  ```
  ⏭ Skipping data-model.mmd (status: draft) — accept before validating
  ```
- `superseded` — **SKIP** with note:
  ```
  ⏭ Skipping contracts/api-v1.yaml (status: superseded by api-v2.yaml)
  ```

Add this filter as a step before artifact routing (before step 9 in drift.md where agents are spawned).

### 7. Drift: Auto-Promote In-Progress → Implemented (FR-021)

When drift completes and ALL criteria pass for an artifact (zero violations, zero divergences), drift SHOULD update that artifact's `status:` frontmatter from `in-progress` to `implemented`.

```
✅ contracts/api.yaml — no drift detected
   Status promoted: in-progress → implemented
```

Only promote `in-progress → implemented`. Do NOT promote `accepted → implemented` (it must go through `in-progress` first via the plan).

### 8. Doctor: Extend Stale Draft Detection (FR-019)

**File:** `commands/doctor.md`

Current check (line 330): "Check for PRDs or specs stuck in `draft` status for more than 7 days (based on file mtime)."

Extend to also check spec-artifacts:

```bash
# Check spec-artifacts for stale drafts
for spec_dir in {specs_path}/SPEC-*/; do
  for artifact in "$spec_dir"*.mmd "$spec_dir"*.yaml "$spec_dir"contracts/*.yaml "$spec_dir"migrations/*.sql; do
    # Read status from frontmatter or comment header
    # If status: draft and mtime > 7 days: report
  done
done
```

Output:
```
[!!] SPEC-005/data-model.mmd has been draft for 12 days — review and accept, or remove
[!!] SPEC-005/contracts/api.yaml has been draft for 12 days — review and accept, or remove
```

Note: spec-artifacts use comment headers (`%% edikt:artifact ... status=draft`) not YAML frontmatter. The check MUST parse both formats:
- YAML frontmatter: `status: draft` between `---` markers
- Comment headers: `status=draft` in `%% edikt:artifact`, `# edikt:artifact`, `-- edikt:artifact`, or `<!-- edikt:artifact -->` lines

## Components

### `templates/hooks/subagent-stop.sh` (modified)
- Updated systemMessage with explicit override logging instructions
- Add override check before firing gate (read `gate-overrides.jsonl`)
- Write `gate_fired` event to `events.jsonl` (extend existing `edikt_log_event`)

### `templates/hooks/session-start.sh` (modified)
- Clear `~/.edikt/gate-overrides.jsonl` at session start

### `commands/sdlc/plan.md` (modified)
- Strengthen draft artifact warning (line 59) with specific artifact list and options
- Add auto-promote instruction for `accepted → in-progress`

### `commands/sdlc/drift.md` (modified)
- Add artifact status filter step before agent routing
- Skip `draft` and `superseded` artifacts with notes
- Add auto-promote instruction for `in-progress → implemented`

### `commands/doctor.md` (modified)
- Extend stale draft check to include spec-artifacts
- Parse both YAML frontmatter and comment header status formats

## Non-Goals

- Plan harness changes (iteration tracking, context handoff, criteria sidecar) — covered in SPEC-001
- Evaluator configuration and headless execution — covered in SPEC-002
- Gate configuration UI (gates are team-level config in `.edikt/config.yaml` — no command to manage them)
- Status transition validation (preventing invalid transitions like `implemented → draft`) — deferred, low risk since transitions are manual or auto-promoted in one direction
- Events.jsonl viewer or dashboard — the file is plain JSONL, queryable with `jq`

## Alternatives Considered

### Gate overrides via hook return value (not systemMessage)

- **Pros:** Hook-level enforcement — Claude can't skip the logging
- **Cons:** The hook can't accept interactive input. It returns JSON and exits. There's no mechanism for the hook to wait for user input and then write the override.
- **Rejected because:** The hook fires BEFORE Claude processes the response. The override happens AFTER the user answers. The systemMessage is the only way to bridge this — Claude handles the UX, the hook handles the blocking.

### Per-project events.jsonl

- **Pros:** Events scoped to the project, easier to review per-repo
- **Cons:** `.edikt/events.jsonl` would be in the repo directory — risk of accidental commit. Or in `.edikt/` which is committed. Neither is ideal for audit logs.
- **Rejected because:** `~/.edikt/events.jsonl` is global, outside any repo, never accidentally committed. The finding text provides enough context to trace back to the project.

### Block plan entirely on draft artifacts (not just warn)

- **Pros:** Stronger enforcement — can't plan against unreviewed designs
- **Cons:** Too rigid. Sometimes you want to sketch a plan to validate the artifacts themselves. Blocking prevents this legitimate workflow.
- **Rejected because:** A prominent warning with explicit acknowledgment is the right balance. The Known Risks note in the plan creates traceability.

## Risks & Mitigations

| Risk | Impact | Likelihood | Mitigation | Rollback |
|---|---|---|---|---|
| Claude doesn't follow systemMessage logging instructions | Overrides not logged | Medium | Make instructions very explicit with exact JSON format. Test with real gate fires. | Manual review of events.jsonl after sessions |
| gate-overrides.jsonl not cleared on session start | Stale overrides suppress real gates | Low | SessionStart hook clears the file. If hook doesn't fire (--bare mode), file accumulates but entries are matched by finding prefix — different findings still fire. | Delete file manually |
| Auto-promote changes artifact status unexpectedly | User confused by status change they didn't make | Low | Log the promotion in plan/drift output with clear message. Only promote in one direction (never downgrade). | User can manually revert status in frontmatter |
| Comment header parsing for spec-artifacts is fragile | Doctor misses stale drafts | Medium | Support all 4 comment formats (mermaid, yaml, sql, html). Test with real artifacts from existing specs. | Fall back to mtime-only check |

## Security Considerations

- `events.jsonl` contains git identity (name + email) — this is already public in git commits. No new exposure.
- Gate findings may contain code snippets (e.g., "SQL injection in handler.go:47"). This is expected — the file is local to the user's machine.
- `gate-overrides.jsonl` is ephemeral (cleared per session). No persistent security-sensitive data.

## Performance Approach

Standard patterns sufficient. Override check is a single `grep` on a small file. Stale draft detection iterates spec directories — negligible for typical project sizes (< 20 specs).

## Acceptance Criteria

- AC-009: Gate override writes entry to `~/.edikt/events.jsonl` with git name and email — Verify: trigger gate, override, `jq '.event' ~/.edikt/events.jsonl` shows `gate_override`
- AC-010: Second gate fire on same finding within same session is skipped — Verify: trigger same gate twice, second returns `{"continue": true}`
- AC-011: `/edikt:sdlc:plan` warns when artifacts have `status: draft` with specific artifact list — Verify: create draft artifact, run plan, check warning lists the artifact name
- AC-012: `/edikt:sdlc:drift` skips artifacts with `status: draft` with note — Verify: run drift with draft artifact, confirm "Skipping" note and artifact not validated
- AC-013: `/edikt:doctor` reports spec-artifacts stuck in draft > 7 days — Verify: create draft artifact with old mtime, run doctor, check warning
- AC-028: `gate-overrides.jsonl` is cleared at session start — Verify: write entry to file, start new session, file is empty
- AC-029: Gate systemMessage includes explicit JSON format for events.jsonl logging — Verify: inspect systemMessage text in subagent-stop.sh
- AC-030: Plan includes Known Risks note when user proceeds with draft artifacts — Verify: proceed past draft warning, inspect plan for Known Risks section
- AC-031: Drift skips `superseded` artifacts with note — Verify: mark artifact superseded, run drift, confirm skipped
- AC-032: Drift auto-promotes `in-progress → implemented` when no violations found — Verify: run drift on clean artifact, check frontmatter changed

## Testing Strategy

- **Hook tests:** Test subagent-stop.sh with mock critical finding — verify systemMessage format, verify override check logic, verify events.jsonl write
- **Session clear test:** Verify session-start.sh clears gate-overrides.jsonl
- **Plan tests:** Verify draft artifact warning shows specific artifact names. Verify Known Risks section appears when proceeding.
- **Drift tests:** Verify draft artifacts are skipped. Verify superseded artifacts are skipped. Verify auto-promote on clean drift.
- **Doctor tests:** Verify stale draft detection covers spec-artifacts. Test both YAML frontmatter and comment header parsing.

## Dependencies

- SubagentStop hook (`templates/hooks/subagent-stop.sh`) — modified in v0.3.1 (seniority prefix fix). Build on that version.
- SessionStart hook (`templates/hooks/session-start.sh`) — already exists, needs one-line addition.
- Plan command step 3 (governance chain check) — already has draft warning at line 59. Strengthen it.
- Doctor command decision graph checks — already have stale draft detection at line 330. Extend to spec-artifacts.
- events.jsonl — new file, created on first gate event. `~/.edikt/` directory already exists.

## Open Questions

- RESOLVED: events.jsonl is global (`~/.edikt/events.jsonl`), not per-project — decided during PRD review.

---

*Generated by edikt:spec — 2026-04-11*
