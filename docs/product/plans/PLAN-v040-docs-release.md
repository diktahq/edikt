# Plan: v0.4.0 Documentation + Release

## Overview
**Task:** Fix 12 pre-existing doc gaps, write v0.4.0 feature docs, update website, bump version, write changelog
**Total Phases:** 4
**Estimated Cost:** $0.32
**Created:** 2026-04-11

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | done    | 1/3    | 2026-04-11 |
| 2     | done    | 1/3    | 2026-04-11 |
| 3     | done    | 1/3    | 2026-04-11 |
| 4     | done    | 1/3    | 2026-04-11 |

**IMPORTANT:** Update this table as phases complete. This table is the persistent state that survives context compaction.

## Model Assignment

| Phase | Task | Model | Reasoning | Est. Cost |
|-------|------|-------|-----------|-----------|
| 1 | Fix pre-existing doc gaps | Sonnet | Find-and-replace across multiple files | $0.08 |
| 2 | v0.4.0 website feature docs | Sonnet | Update 6 website pages with new feature content | $0.08 |
| 3 | Changelog + version bump + roadmap | Sonnet | Write changelog entry, bump files, clean roadmap | $0.08 |
| 4 | Tests | Sonnet | Verify doc fixes + new content | $0.08 |

## Execution Strategy

| Phase | Depends On | Parallel With |
|-------|-----------|---------------|
| 1     | None      | 2             |
| 2     | None      | 1             |
| 3     | 1, 2      | -             |
| 4     | 1, 2, 3   | -             |

---

## Phase 1: Fix Pre-Existing Doc Gaps

**Objective:** Fix all 12 documentation gaps found by /edikt:docs:review
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `DOC GAPS FIXED`
**Evaluate:** true
**Dependencies:** None

**Acceptance Criteria:**
- [ ] AC-1.1: README.md agent count updated to correct number
- [ ] AC-1.2: README.md hook count updated to correct number
- [ ] AC-1.3: README.md command count updated (30+ or actual count)
- [ ] AC-1.4: AGENTS.md managed block uses namespaced commands (sdlc:plan, adr:new, gov:compile, etc.)
- [ ] AC-1.5: AGENTS.md "Codex" references changed to "Claude Code"
- [ ] AC-1.6: AGENTS.md repo structure comment updated from "5 edikt slash commands" to correct count
- [ ] AC-1.7: All "18 specialist agents" references updated across website (agents.md, what-is-edikt.md, specialist-agents.md, commands/agents.md, faq.md) and docs/project-context.md
- [ ] AC-1.8: website/commands/index.md includes /edikt:gov:score in Governance table
- [ ] AC-1.9: website/commands/index.md includes /edikt:config in Daily Use table
- [ ] AC-1.10: website/commands/index.md marks /edikt:team as deprecated
- [ ] AC-1.11: website/governance/features.md uses /edikt:adr:new and /edikt:invariant:new (not flat names)

**Prompt:**
```
Read these files first:
- docs/product/plans/PLAN-v040-docs-release.md (the full plan — you're executing Phase 1)
- README.md
- AGENTS.md (if it exists — check root and .claude/)
- website/commands/index.md (if it exists)
- website/governance/features.md
- website/agents.md
- website/what-is-edikt.md
- website/guides/specialist-agents.md
- website/commands/agents.md
- website/faq.md
- docs/project-context.md

Fix these 12 pre-existing documentation gaps:

1. README.md — update agent count. Check actual count:
   ls templates/agents/*.md | wc -l
   Update the line that says "18 specialist agents" to the actual count.

2. README.md — update hook count. Check actual count:
   ls templates/hooks/*.sh | wc -l
   Update the line that says "9 lifecycle hooks" to the actual count.

3. README.md — update command count. Check actual count:
   find commands/ -name "*.md" ! -path "*/deprecated/*" | wc -l
   Update "25+ commands" to the actual count or "30+ commands".

4. AGENTS.md managed block — replace all old flat command names with namespaced equivalents:
   /edikt:plan → /edikt:sdlc:plan
   /edikt:adr → /edikt:adr:new
   /edikt:compile → /edikt:gov:compile
   /edikt:prd → /edikt:sdlc:prd
   /edikt:spec → /edikt:sdlc:spec
   /edikt:spec-artifacts → /edikt:sdlc:artifacts
   /edikt:drift → /edikt:sdlc:drift
   /edikt:review-governance → /edikt:gov:review
   /edikt:review → /edikt:sdlc:review
   /edikt:audit → /edikt:sdlc:audit
   /edikt:docs → /edikt:docs:review
   /edikt:intake → /edikt:docs:intake
   /edikt:rules-update → /edikt:gov:rules-update
   /edikt:sync → /edikt:gov:sync
   /edikt:invariant → /edikt:invariant:new

5. AGENTS.md — replace ".Codex/rules/" with ".claude/rules/"

6. AGENTS.md — replace "Codex only for execution reliability" with "Claude Code only for execution reliability"

7. AGENTS.md and CLAUDE.md — update repo structure comment from "5 edikt slash commands" to actual count

8. All files referencing "18 specialist agents" — update to correct count:
   - website/agents.md (lines 3, 8)
   - website/what-is-edikt.md (line 183)
   - website/guides/specialist-agents.md (line 3)
   - website/commands/agents.md (line 23)
   - website/faq.md (line 28)
   - docs/project-context.md (line 30)
   Search for "18" near "agent" in each file to find exact locations.

9. website/commands/index.md — add /edikt:gov:score to the Governance commands table

10. website/commands/index.md — add /edikt:config to the Daily Use commands table

11. website/commands/index.md — mark /edikt:team as deprecated or replace with note:
    `/edikt:team` (deprecated) | Merged into /edikt:init and /edikt:config

12. website/governance/features.md — replace /edikt:adr with /edikt:adr:new and /edikt:invariant with /edikt:invariant:new

After making all changes, run ./test/run.sh to verify nothing broke.

When complete, output: DOC GAPS FIXED
```

---

## Phase 2: v0.4.0 Website Feature Docs

**Objective:** Update 6 website pages with v0.4.0 feature documentation
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `FEATURE DOCS DONE`
**Evaluate:** true
**Dependencies:** None

**Acceptance Criteria:**
- [ ] AC-2.1: website/commands/sdlc/plan.md documents Attempt column in progress table
- [ ] AC-2.2: website/commands/sdlc/plan.md documents Context Needed field and Artifact Flow Table
- [ ] AC-2.3: website/commands/sdlc/plan.md documents criteria sidecar (PLAN-{slug}-criteria.yaml)
- [ ] AC-2.4: website/commands/sdlc/plan.md documents stuck status and backoff logic
- [ ] AC-2.5: website/commands/sdlc/plan.md documents evaluator config (preflight, phase-end, mode toggles)
- [ ] AC-2.6: website/governance/gates.md documents override logging to events.jsonl
- [ ] AC-2.7: website/governance/gates.md documents re-fire prevention (session-scoped overrides)
- [ ] AC-2.8: website/governance/gates.md documents the override UX flow (⛔ prompt, y/n, logged with git identity)
- [ ] AC-2.9: website/governance/chain.md documents full artifact lifecycle (draft → accepted → in-progress → implemented → superseded)
- [ ] AC-2.10: website/governance/features.md documents evaluator config section with all 5 keys
- [ ] AC-2.11: website/commands/doctor.md documents spec-artifact stale draft detection
- [ ] AC-2.12: website/commands/sdlc/drift.md documents status filter (skips draft/superseded) and auto-promote

**Prompt:**
```
Read these files first:
- docs/product/plans/PLAN-v040-docs-release.md (the full plan — you're executing Phase 2)
- docs/product/specs/SPEC-001-plan-harness/spec.md (plan harness spec — for plan page content)
- docs/product/specs/SPEC-002-evaluator-experiments/spec.md (evaluator spec — for evaluator/features content)
- docs/product/specs/SPEC-003-enforcement/spec.md (enforcement spec — for gates/chain/drift/doctor content)
- website/commands/sdlc/plan.md (current plan page)
- website/governance/gates.md (current gates page)
- website/governance/chain.md (current chain page)
- website/governance/features.md (current features page)
- website/governance/evaluator.md (already created — verify still accurate)
- website/commands/doctor.md (current doctor page)
- website/commands/sdlc/drift.md (current drift page)

Update each website page with v0.4.0 features. Keep existing content — ADD sections, don't rewrite pages.

1. website/commands/sdlc/plan.md — add these sections:

   "## Iteration tracking" section:
   - Progress table now includes Attempt column: `| Phase | Status | Attempt | Updated |`
   - 6 statuses: pending, in-progress, evaluating, done, stuck, skipped
   - After evaluation fails: attempt incremented, fail reasons forwarded to next attempt
   - Same criterion failing 3 times: escalation warning
   - Max attempts (configurable via evaluator.max-attempts, default 5): phase goes stuck with 4 options

   "## Context handoff" section:
   - Each phase has a Context Needed field listing files to read before starting
   - Artifact Flow Table shows which phases produce files consumed by other phases
   - PostCompact hook injects context file list and attempt count after compaction

   "## Criteria sidecar" section:
   - Plan emits PLAN-{slug}-criteria.yaml alongside plan markdown
   - Machine-readable per-criterion status tracking (pending/pass/fail)
   - Evaluator reads and updates the sidecar after each evaluation
   - Schema reference at docs/plans/artifacts/plan-criteria-schema.yaml

   "## Evaluator configuration" section:
   - Link to /governance/evaluator for full details
   - Config reference:
     evaluator.preflight (default: true)
     evaluator.phase-end (default: true)
     evaluator.mode (headless | subagent, default: headless)
     evaluator.max-attempts (default: 5)
     evaluator.model (sonnet | opus | haiku, default: sonnet)

2. website/governance/gates.md — add these sections:

   "## Override flow" section:
   - When a gate fires, Claude presents the finding and asks for override
   - Show the ⛔ GATE prompt format
   - Override logged to ~/.edikt/events.jsonl with git identity (name + email)
   - Block also logged if user says no

   "## Re-fire prevention" section:
   - After override, same finding won't fire again in the same session
   - Session = single Claude Code invocation (start to exit)
   - Overrides cleared at session start by SessionStart hook
   - Override matching: agent name + first 80 chars of finding

   "## events.jsonl" section:
   - Location: ~/.edikt/events.jsonl (global)
   - Three event types: gate_fired, gate_override, gate_blocked
   - Schema example with ts, event, agent, finding, user, email fields
   - Queryable with jq

3. website/governance/chain.md — add or update:

   "## Artifact lifecycle" section (or update existing status references):
   - Full lifecycle: draft → accepted → in-progress → implemented → superseded
   - Transition table: who triggers each (user manual vs command auto)
   - Plan auto-promotes accepted → in-progress when phase starts
   - Drift auto-promotes in-progress → implemented when no violations
   - Plan warns on draft artifacts, drift skips them

4. website/governance/features.md — add:

   "## Evaluator" section (under the existing feature toggles):
   - evaluator.preflight: toggle pre-flight criteria validation
   - evaluator.phase-end: toggle phase-end evaluation
   - evaluator.mode: headless (separate claude -p) or subagent
   - evaluator.max-attempts: max retries before stuck
   - evaluator.model: model for headless evaluator
   - Link to /governance/evaluator for comparison table

5. website/commands/doctor.md — add:

   Under existing checks or in a new "Spec-artifact checks" section:
   - Doctor now flags spec-artifacts stuck in draft > 7 days
   - Parses both YAML frontmatter and comment header status formats
   - Covers .mmd, .yaml, .sql, .md artifacts in spec directories

6. website/commands/sdlc/drift.md — add:

   "## Status filtering" section:
   - Drift filters artifacts by status before validation
   - accepted, implemented, in-progress → validated
   - draft → skipped with note
   - superseded → skipped with note

   "## Auto-promote" section:
   - When drift finds no violations for an in-progress artifact, promotes to implemented
   - Does NOT promote accepted → implemented directly (must go through in-progress via plan)

7. website/governance/evaluator.md — verify still accurate:
   - Config section should match the final config keys
   - Comparison table should be current
   - If anything changed during implementation, update it

After making all changes, run ./test/run.sh to verify nothing broke.

When complete, output: FEATURE DOCS DONE
```

---

## Phase 3: Changelog + Version Bump + Roadmap

**Objective:** Write v0.4.0 changelog, bump VERSION and config, clean up roadmap
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `RELEASE PREP DONE`
**Evaluate:** true
**Dependencies:** Phase 1, Phase 2

**Acceptance Criteria:**
- [ ] AC-3.1: CHANGELOG.md has v0.4.0 entry above v0.3.1
- [ ] AC-3.2: Changelog covers all 3 specs: plan harness, evaluator, enforcement
- [ ] AC-3.3: Changelog mentions evaluator config (headless/subagent, toggles)
- [ ] AC-3.4: Changelog mentions quality gate override logging + re-fire prevention
- [ ] AC-3.5: Changelog mentions artifact lifecycle enforcement
- [ ] AC-3.6: VERSION file contains 0.4.0
- [ ] AC-3.7: .edikt/config.yaml edikt_version is 0.4.0
- [ ] AC-3.8: Roadmap Tier 1 items (1.1-1.3) moved to Shipped (shipped in v0.3.0)
- [ ] AC-3.9: Roadmap Tier 2 items (2.1-2.4, 2.6, 2.7) moved to Shipped (shipped in v0.4.0)
- [ ] AC-3.10: Roadmap updated date and current version

**Prompt:**
```
Read these files first:
- docs/product/plans/PLAN-v040-docs-release.md (the full plan — you're executing Phase 3)
- CHANGELOG.md (current — v0.3.1 is the latest entry)
- VERSION (current — 0.3.1)
- .edikt/config.yaml (current — edikt_version: "0.3.1")
- docs/internal/plans/ROADMAP.md (current roadmap)
- docs/product/prds/PRD-001-v040-harness-lifecycle-gates.md (PRD for changelog content)
- docs/product/specs/SPEC-001-plan-harness/spec.md (spec for changelog content)
- docs/product/specs/SPEC-002-evaluator-experiments/spec.md (spec for changelog content)
- docs/product/specs/SPEC-003-enforcement/spec.md (spec for changelog content)

Three tasks:

1. CHANGELOG — add v0.4.0 entry ABOVE v0.3.1. Structure:

   ## v0.4.0 (2026-04-11)

   ### Plan Harness: Iteration Tracking, Context Handoff, Criteria Sidecar (SPEC-001)
   - Progress table with Attempt column, 6 statuses (pending/in-progress/evaluating/done/stuck/skipped)
   - Backoff: fail reasons forwarded, escalation at 3 consecutive failures, stuck at max attempts
   - Context Needed field per phase + Artifact Flow Table
   - PostCompact hook injects context files, attempt count, and failing criteria
   - Structured criteria sidecar (PLAN-{slug}-criteria.yaml) emitted alongside plans
   - Evaluator reads/updates sidecar after each evaluation

   ### Evaluator: Headless Execution and LLM Experiment Evaluator (SPEC-002)
   - New evaluator config section: preflight, phase-end, mode, max-attempts, model
   - Headless mode: evaluator runs as separate claude -p with --bare (zero shared context)
   - Subagent fallback when headless unavailable
   - Evaluator is internal agent — not user-overridable, blocks if missing
   - LLM evaluator in experiment runner (--llm-eval flag)
   - Dual-mode: grep pre-check + LLM evaluation, LLM verdict is authoritative
   - Severity tiers: critical (blocks), important (WEAK PASS), informational (logged only)
   - Three verdicts: PASS, WEAK PASS, FAIL

   ### Enforcement: Quality Gate UX and Artifact Lifecycle (SPEC-003)
   - Gate overrides logged to ~/.edikt/events.jsonl with git identity
   - Re-fire prevention: overridden findings don't fire again within session
   - Session-scoped overrides cleared by SessionStart hook
   - Artifact lifecycle: draft → accepted → in-progress → implemented → superseded
   - Plan warns on draft artifacts with specific artifact listing + Known Risks
   - Plan auto-promotes accepted → in-progress when phase starts
   - Drift filters by status (skips draft and superseded)
   - Drift auto-promotes in-progress → implemented when no violations
   - Doctor flags spec-artifacts stuck in draft > 7 days

   ### Documentation
   - Fixed 12 pre-existing doc gaps (stale counts, old command names, missing index entries)
   - Website: updated plan, gates, chain, features, doctor, drift pages
   - New evaluator documentation page (headless vs subagent comparison)

   ### New config keys
   evaluator.preflight, evaluator.phase-end, evaluator.mode, evaluator.max-attempts, evaluator.model

   Keep it concise — reference specs for details, don't duplicate the full spec content.

2. VERSION + CONFIG — bump:
   - VERSION file: 0.3.1 → 0.4.0
   - .edikt/config.yaml: edikt_version: "0.3.1" → "0.4.0"

3. ROADMAP — clean up docs/internal/plans/ROADMAP.md:
   - Update header: last updated 2026-04-11, current version 0.4.0
   - Move Tier 1 items 1.1 (Compile Reminders), 1.2 (Quality Score), 1.3 (Pre-flight) to Shipped section — these shipped in v0.3.0
   - Move Tier 2 items 2.1 (Iteration Tracking), 2.2 (Context Handoff), 2.3 (Criteria Sidecar), 2.4 (LLM Evaluator), 2.6 (Gate UX), 2.7 (Artifact Lifecycle) to Shipped section — these shipped in v0.4.0
   - Update Shipped section:
     - v0.3.0 (2026-04-10) — released
     - v0.3.1 (2026-04-11) — team→init, config command, JSONB artifacts
     - v0.4.0 (2026-04-11) — plan harness, evaluator, enforcement
   - Keep remaining items (2.5 Multi-Platform, 3.x, 4.x) as-is
   - 1.4 Content Launch — update "Blocked by: v0.3.0 release" to "Blocked by: v0.4.0 release" (or unblock it since v0.4.0 is shipping)

After making all changes, run ./test/run.sh to verify nothing broke.

When complete, output: RELEASE PREP DONE
```

---

## Phase 4: Tests

**Objective:** Verify documentation fixes and new content are correct
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `DOCS TESTS DONE`
**Evaluate:** true
**Dependencies:** Phase 1, Phase 2, Phase 3

**Acceptance Criteria:**
- [ ] AC-4.1: test/test-v040-docs.sh exists and is executable
- [ ] AC-4.2: README agent/hook/command counts are correct
- [ ] AC-4.3: No "18 specialist agents" string in any website or docs file
- [ ] AC-4.4: No old flat command names in AGENTS.md managed block
- [ ] AC-4.5: website/commands/index.md has gov:score and config entries
- [ ] AC-4.6: CHANGELOG has v0.4.0 entry
- [ ] AC-4.7: VERSION is 0.4.0
- [ ] AC-4.8: Website plan page has Attempt, Context Needed, criteria sidecar
- [ ] AC-4.9: Website gates page has events.jsonl and re-fire prevention
- [ ] AC-4.10: Website chain page has artifact lifecycle states
- [ ] AC-4.11: Website features page has evaluator config
- [ ] AC-4.12: All existing test suites pass

**Prompt:**
```
Read these files first:
- docs/product/plans/PLAN-v040-docs-release.md (the full plan — you're executing Phase 4)
- test/helpers.sh (assertion functions)
- test/test-v040-plan-harness.sh (test pattern to follow)

IMPORTANT: Phases 1, 2, and 3 must be done. Check the progress table.

Create `test/test-v040-docs.sh`:

```bash
#!/bin/bash
# Test: v0.4.0 documentation — pre-existing fixes + v0.4.0 feature docs
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"
```

Write these test sections:

--- TEST 1: Stale count fixes ---

- No "18 specialist agents" in website or docs:
  ```bash
  STALE=$(grep -rl "18 specialist" "$PROJECT_ROOT/website/" "$PROJECT_ROOT/docs/project-context.md" "$PROJECT_ROOT/README.md" 2>/dev/null | wc -l | tr -d ' ')
  if [ "$STALE" -eq 0 ]; then
      pass "No stale '18 specialist agents' references"
  else
      fail "Found $STALE files still referencing '18 specialist agents'"
  fi
  ```

- README has updated counts (not "25+ commands", not "9 lifecycle hooks"):
  assert_file_not_contains "$PROJECT_ROOT/README.md" "25+ commands" "README command count updated"
  assert_file_not_contains "$PROJECT_ROOT/README.md" "9 lifecycle hooks" "README hook count updated"

--- TEST 2: AGENTS.md fixes ---

- No old flat command names in AGENTS.md:
  Check that AGENTS.md does not contain "/edikt:plan " (with trailing space, not /edikt:sdlc:plan)
  Check that AGENTS.md does not contain ".Codex/"

--- TEST 3: Website index ---

- assert_file_contains website/commands/index.md "gov:score" "Index has gov:score"
- assert_file_contains website/commands/index.md "config" "Index has config"
- Website index marks team as deprecated:
  ```bash
  if grep -qi "team.*deprecated\|deprecated.*team" "$PROJECT_ROOT/website/commands/index.md" 2>/dev/null; then
      pass "Index marks team as deprecated"
  else
      fail "Index should mark team as deprecated"
  fi
  ```

--- TEST 4: Changelog and version ---

- assert_file_contains "$PROJECT_ROOT/CHANGELOG.md" "v0.4.0" "Changelog has v0.4.0"
- VERSION is 0.4.0:
  ```bash
  VER=$(cat "$PROJECT_ROOT/VERSION" | tr -d '[:space:]')
  if [ "$VER" = "0.4.0" ]; then pass "VERSION is 0.4.0"; else fail "VERSION is $VER, expected 0.4.0"; fi
  ```

--- TEST 5: Website plan page ---

- assert_file_contains website/commands/sdlc/plan.md "Attempt" "Plan page has Attempt column"
- assert_file_contains website/commands/sdlc/plan.md "Context Needed" "Plan page has Context Needed"
- assert_file_contains website/commands/sdlc/plan.md "criteria.yaml" "Plan page has criteria sidecar"
- assert_file_contains website/commands/sdlc/plan.md "stuck" "Plan page has stuck status"

--- TEST 6: Website gates page ---

- assert_file_contains website/governance/gates.md "events.jsonl" "Gates page has events.jsonl"
- assert_file_contains website/governance/gates.md "re-fire\|override" "Gates page has override docs"

--- TEST 7: Website chain page ---

- assert_file_contains website/governance/chain.md "in-progress" "Chain page has in-progress state"
- assert_file_contains website/governance/chain.md "implemented" "Chain page has implemented state"

--- TEST 8: Website features page ---

- assert_file_contains website/governance/features.md "evaluator" "Features page has evaluator section"
- assert_file_contains website/governance/features.md "preflight" "Features page has preflight toggle"

--- TEST 9: Roadmap ---

- Roadmap version is 0.4.0:
  assert_file_contains "$PROJECT_ROOT/docs/internal/plans/ROADMAP.md" "0.4.0" "Roadmap references v0.4.0"

--- END ---

End with test_summary. Run ./test/run.sh to verify all suites pass.

When complete, output: DOCS TESTS DONE
```

---

## Known Risks

- Phase 1 touches many files (10+). Risk of merge conflicts if other changes land on the branch. Mitigation: run on 0.4.0-dev branch before merging.
- Agent count may change if more agents were added during v0.4.0 implementation. Check actual counts at runtime, don't hardcode.
