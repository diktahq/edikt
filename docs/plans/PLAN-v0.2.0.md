# Plan: edikt v0.2.0

**Status:** Planned
**Theme:** Claude Code Surface Sync + Real-World Compliance Research

---

## Overview

Two workstreams for v0.2.0:

1. **Claude Code Surface Sync** — Close the gap between edikt and Claude Code v2.1.72-81. Adopt new platform primitives (effort frontmatter, agent governance fields, new hooks), fix the HTML sentinel comment issue, verify security fixes.

2. **EXP-003: Real-World Compliance** — Test rule compliance under conditions that match actual edikt usage: 14+ rules loaded simultaneously, multi-turn conversations, context compaction recovery, real project conventions.

---

## Progress

| Phase | Theme | Status | Updated |
|-------|-------|--------|---------|
| 1 | Design session — Claude Code surface sync | not started | — |
| 2 | PRD-006 — v0.2.0 requirements | not started | — |
| 3 | HTML sentinel fix (critical — may be broken now) | verified — not broken | 2026-03-23 |
| 4 | Effort frontmatter + agent governance fields | not started | — |
| 5 | New hooks (StopFailure, SessionEnd, SubagentStart) | not started | — |
| 6 | Agent resume → SendMessage migration | not started | — |
| 7 | EXP-003: Real-world compliance experiment | not started | — |
| 8 | Website + docs update for v0.2.0 | not started | — |

---

## Phase 1: Design Session

**Input:** `v0.2.0 Design Session — Claude Code Surface Sync.md` in Obsidian
**Output:** Design decisions for all 7 features
**Process:** Load context → review each feature → decide on approach → resolve open questions

## Phase 2: PRD-006

**Input:** Design decisions from Phase 1
**Output:** PRD-006 with numbered requirements and acceptance criteria
**Process:** `/edikt:prd`

## Phase 3: HTML Sentinel Fix (Critical)

**Why critical:** Claude Code v2.1.72 hides `<!-- -->` HTML comments from Claude during auto-injection. edikt uses `<!-- edikt:start -->` / `<!-- edikt:end -->` for safe CLAUDE.md merge (ADR-002). This could be broken NOW for any project using edikt.

**Test:** Run `/edikt:init` on a fresh project, check if sentinels are visible to Claude, check if `/edikt:upgrade` can find and replace between sentinels.

**If broken:** Replace with non-HTML markers or move to a separate file.

## Phase 4: Effort + Agent Governance

**Effort:** Add `effort:` frontmatter to all 24 commands (low/medium/high).
**Agent governance:** Add `maxTurns:` and `disallowedTools:` to all 18 agent templates.

## Phase 5: New Hooks

Evaluate and implement: StopFailure, SessionEnd, SubagentStart, TaskCompleted, ConfigChange.
Highest value: SessionEnd (auto-session-sweep).

## Phase 6: Agent Migration

Search all templates for `resume` parameter usage. Replace with `SendMessage` pattern.

## Phase 7: EXP-003

**Brief:** `experiments/exp-003-real-world-compliance/BRIEF.md`
**Scope:** ~48 runs testing multi-rule, compaction, multi-turn, real conventions.
**Output:** EXP-003 write-up on website using experiment template.

## Phase 8: Website + Docs

Update website for v0.2.0 changes. CHANGELOG entry. Version bump.

---

## Dependencies

- Phase 1 (design) blocks Phase 2 (PRD)
- Phase 2 (PRD) blocks Phases 4-6
- Phase 3 (sentinel fix) is independent — can run in parallel, highest urgency
- Phase 7 (EXP-003) is independent — can run anytime
- Phase 8 (docs) depends on all other phases completing
