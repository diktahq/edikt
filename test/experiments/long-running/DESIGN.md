# Long-Running Experiments — Design

**Status:** Designed, not yet runnable
**Depends on:** PLAN-long-running-harness Phase 9 (LLM evaluator)
**Origin:** Evolved from v0.1.x exp-003 BRIEF + v0.3.0 experiment findings

## Why this suite exists

The directive-effect suite (v0.3.0) proved governance works on single-turn, fresh-context tasks. But real edikt usage involves:

- 14-17 rules loaded simultaneously (not 1-2)
- Multi-turn conversations (not single `claude -p` calls)
- Context compaction mid-session (rules re-injected via PostCompact hook)
- Real project conventions (not invented fixtures)

EXP-08 (long context) showed a directional signal: governance stabilizes output under context pressure (baseline 1/2 violations, governance 0/2). This suite formalizes and expands that finding.

## Four hypotheses

### H1: Multi-rule compliance

**Question:** Does compliance degrade when 14+ rule packs are loaded simultaneously?

**Design:**
- Fixture: a real Go service (or realistic fixture with 10+ concerns)
- Governance: full governance.md with 14+ directives across 4+ topic files
- Task: add a feature that touches multiple concern areas
- Assertion: LLM evaluator checks compliance on each directive
- N=3 per condition (with full governance vs no governance)

**Success:** compliance at or above 90% on each directive with full governance loaded.
**Failure:** "lost in the middle" effect — directives in the middle of governance.md get ignored.

**Measurement tool:** `/edikt:gov:score` context budget + per-directive compliance from LLM evaluator.

### H2: Compaction recovery

**Question:** Does compliance recover after context compaction when PostCompact hook re-injects governance?

**Design:**
- Fixture: medium Go service
- Task: multi-step feature (3+ implementation steps)
- Method: inject enough context to trigger compaction between steps
- Condition A: no governance (baseline)
- Condition B: governance loaded, PostCompact hook active
- Assertion: LLM evaluator checks compliance on code written AFTER compaction

**Success:** post-compaction compliance equals pre-compaction compliance when governance is loaded.
**Failure:** compliance drops after compaction even with PostCompact re-injection.

### H3: Multi-turn compliance

**Question:** Does compliance hold across 5+ turns in a single session?

**Design:**
- Fixture: medium Go service with existing code
- Task: 5 sequential implementation tasks in one session
- Assertion: LLM evaluator checks compliance on each turn's output
- Measure: compliance rate per turn (turn 1 vs turn 5)

**Success:** compliance on turn 5 is within 10% of turn 1.
**Failure:** later turns show significantly lower compliance (attention degradation).

### H4: Real conventions

**Question:** Do real-world project conventions achieve similar compliance rates to invented conventions?

**Design:**
- Fixtures: 3 real open-source Go/TS repos (selected for clear conventions)
- Governance: real governance extracted from each repo's patterns
- Task: add a feature following the repo's conventions
- Assertion: LLM evaluator checks whether new code follows the repo's existing patterns

**Success:** compliance rates match EXP-05-07 findings.
**Failure:** real conventions are harder to comply with because they're more nuanced.

## Infrastructure needed

| Requirement | From harness plan | Status |
|---|---|---|
| LLM evaluator | Phase 9 | Designed, not built |
| Structured criteria (YAML) | Phase 5 | Designed, not built |
| Multi-turn invocation | Not in plan | Needs design |
| Compaction triggering | Not in plan | EXP-08 system prompt approach works as proxy |
| Real-world fixture selection | Not in plan | Needs curation |

## Estimated cost

~27 runs, ~$3.00 + $0.54 LLM evaluator cost.

## Timeline

1. Build LLM evaluator (harness plan Phase 9)
2. Build H1 and H2 (can use existing `claude -p` + system prompt approach)
3. Build H3 (needs multi-turn harness)
4. Build H4 (needs fixture curation)
