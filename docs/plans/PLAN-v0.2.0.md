# Plan: edikt v0.2.0

**Status:** Planned
**Theme:** Claude Code Surface Sync + Real-World Compliance Research

---

## Overview

Three workstreams for v0.2.0:

1. **Codebase Pattern Learning** — edikt learns from the existing codebase and points Claude to real examples. When writing a new PR, Claude looks at how existing PRs are structured. When creating a handler, Claude finds a similar handler in the project to follow. Reduces hallucination by grounding Claude in what already exists.

2. **Claude Code Surface Sync** — Close the gap between edikt and Claude Code v2.1.72-81. Adopt new platform primitives (effort frontmatter, agent governance fields, new hooks), fix the HTML sentinel comment issue, verify security fixes.

3. **EXP-003: Real-World Compliance** — Test rule compliance under conditions that match actual edikt usage: 14+ rules loaded simultaneously, multi-turn conversations, context compaction recovery, real project conventions.

---

## Progress

| Phase | Theme | Status | Updated |
|-------|-------|--------|---------|
| 1a | Design session — codebase pattern learning | not started | — |
| 1b | Design session — Claude Code surface sync | not started | — |
| 2 | PRD-006 — v0.2.0 requirements | not started | — |
| 3 | Codebase pattern learning — scan, index, point Claude to examples | not started | — |
| 4 | HTML sentinel migration — replace `<!-- -->` with visible markers | not started | — |
| 5 | Effort frontmatter + agent governance fields | not started | — |
| 6 | New hooks (StopFailure, SessionEnd, SubagentStart) | not started | — |
| 7 | Agent resume → SendMessage migration | not started | — |
| 8 | EXP-003: Real-world compliance experiment | not started | — |
| 9 | Website + docs update for v0.2.0 | not started | — |

---

## Phase 1a: Design Session — Codebase Pattern Learning

**Input:** Phase 3 description below + codebase examples
**Output:** Design decisions for how edikt learns and surfaces patterns
**Process:** Load context → review the concept → decide on scan approach, storage, user confirmation

## Phase 1b: Design Session — Claude Code Surface Sync

**Input:** `v0.2.0 Design Session — Claude Code Surface Sync.md` in Obsidian
**Output:** Design decisions for effort frontmatter, agent governance, hooks, sentinel migration
**Process:** Load context → review each feature → decide on approach → resolve open questions

## Phase 3: Codebase Pattern Learning

**The problem:** Claude hallucates patterns, naming conventions, and structures instead of learning from the codebase. When asked to "write a new handler," Claude invents a structure rather than finding an existing handler in the project and following that pattern. When writing a PR description, Claude guesses the format rather than looking at recent merged PRs.

**The concept:** edikt scans the codebase during init and on-demand, identifies patterns, and compiles them into guidance Claude reads automatically. Examples:

- **PR template learning:** Scan recent merged PRs (via `gh pr list --state merged`), extract the structure, inject "When writing PRs, follow this pattern: [extracted template]" into compiled governance.
- **Code pattern learning:** "When creating a new HTTP handler, follow the pattern in `pkg/handler/orders.go`." Instead of rules that say "use hexagonal architecture," point Claude to a real file that demonstrates it.
- **Test pattern learning:** "Tests follow the pattern in `pkg/service/order_test.go` — table-driven tests with `testify/assert`."
- **Naming convention learning:** Scan existing files, infer naming patterns (snake_case files, camelCase functions), compile into a directive.

**What to design:**
- How does the scan work? (AST? grep? pattern matching?)
- Where do learned patterns get stored? (compiled into governance.md? separate file?)
- How does the user confirm/override learned patterns?
- How often does it re-scan? (on init only? on demand? each session?)
- How does it handle projects with inconsistent patterns?

## Phase 2: PRD-006

**Input:** Design decisions from Phase 1
**Output:** PRD-006 with numbered requirements and acceptance criteria
**Process:** `/edikt:prd`

## Phase 3: HTML Sentinel Migration

**Finding (2026-03-23):** Claude Code v2.1.72 hides `<!-- -->` HTML comments from Claude during auto-injection. edikt's commands (init, upgrade) still work because they use the Read tool and grep/sed on the raw file. **Not broken for commands.**

**But:** Claude can no longer see the sentinel boundaries during normal conversation. If a user asks Claude to "edit my CLAUDE.md", Claude doesn't know where the edikt-managed section starts and ends — it could accidentally modify edikt's block.

**Action:** Migrate sentinels from HTML comments to visible text markers that Claude can see:
- Replace `<!-- edikt:start — managed by edikt, do not edit -->` with a visible marker (e.g., `[edikt:start — managed by edikt, do not edit]` or a prose comment)
- Update ADR-002 to reflect the change
- Update `/edikt:init` and `/edikt:upgrade` to detect both old (HTML) and new (visible) markers for backwards compatibility
- Migration path: `/edikt:upgrade` auto-converts old sentinels to new format

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
