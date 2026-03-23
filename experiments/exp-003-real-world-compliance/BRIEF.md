# EXP-003: Real-World Compliance — Brief

**Status:** Planned
**Goal:** Test rule compliance under conditions that match actual edikt usage, not clean single-turn scenarios.

## Why this matters

EXP-001 and EXP-002 proved the mechanism works in controlled conditions. But real edikt sessions involve:
- 14-17 rules loaded simultaneously (not 1)
- Multi-turn conversations (not single-turn)
- Context compaction mid-session (rules re-injected via PostCompact hook)
- Real project conventions (not invented ones)
- Mixed tasks (implementation + refactoring + debugging in one session)

If compliance degrades under these conditions, the experiments we published don't represent real usage. If it holds, that's a much stronger claim.

## Hypotheses

**H1:** Compliance remains above 90% when 14+ rule packs are loaded simultaneously.
**H1-null:** Compliance degrades significantly (below 80%) with many rules — "lost in the middle" effect at the rule pack level.

**H2:** Compliance recovers after context compaction when PostCompact hook re-injects governance.
**H2-null:** Compliance drops after compaction and does not recover, even with re-injection.

**H3:** Compliance holds across multi-turn sessions (5+ turns).
**H3-null:** Later turns show lower compliance than early turns as context accumulates.

**H4:** Real project conventions (from actual codebases) achieve similar compliance rates to invented conventions.
**H4-null:** Real conventions are harder to comply with because they're more nuanced/semantic than syntactic.

## Proposed conditions

| Part | What it tests | Runs | Conditions |
|---|---|---|---|
| 1. Multi-rule load | 14 base rules loaded simultaneously | 15 | with-rules, no-rules, 5 conventions scored |
| 2. Context compaction | Same task, force compaction mid-session | 9 | pre-compaction, post-compaction, no-rules |
| 3. Multi-turn | 5-turn conversation building a feature | 9 | with-rules turn 1-5, no-rules turn 1-5 |
| 4. Real conventions | Conventions from real Go/TS projects | 15 | with-rules, no-rules, 5 real conventions |

**Estimated:** ~48 runs, ~$10-15

## Scoring challenges

- Multi-rule: need to score 5+ conventions per run simultaneously
- Compaction: need to trigger compaction programmatically (long context fill)
- Multi-turn: scorer needs to check compliance per turn, not just final state
- Real conventions: scoring is harder — real conventions are less binary than invented ones

## What to build

- `setup.sh` — creates workdirs with full rule pack sets (not single rules)
- `run.sh` — handles multi-turn scenarios (not just single prompts)
- `score.py` — per-turn scoring, multi-convention scoring
- A way to force context compaction in a test (fill context to trigger it)

## Open questions

- How to reliably trigger context compaction in a test?
- Which real conventions to test? (need conventions that are specific enough to score automatically)
- Should we test PostCompact hook re-injection specifically, or just compaction in general?
