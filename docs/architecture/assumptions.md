# Harness Assumptions

Every governance mechanism in edikt encodes an assumption about what the model can't do reliably on its own. As models improve, some assumptions become unnecessary overhead. This document tracks assumptions, how to test them, and when they were last validated.

Re-test assumptions when upgrading edikt or when a new model version is released.

## Active Assumptions

### A-001: Agents won't follow ADRs without compiled enforcement

**Mechanism:** `/edikt:compile` generates rule files loaded into every session.
**Test:** Run a coding task that touches an ADR-governed area without compiled rules loaded. Check if the agent follows the ADR decision anyway.
**Last tested:** 2026-04-10
**Result:** Partially confirmed.

**Evidence (v0.3.0 experiments, Opus 4.6 / Claude Code 2.1.98):**
- EXP-05 (greenfield architecture): Baseline produced flat structure with SQL everywhere. Governance-loaded produced clean layered architecture. **Effect present.**
- EXP-06 (greenfield tenant): Baseline missed `tenant_id` on status update query. Governance-loaded passed all checks. **Effect present.**
- EXP-07 (new domain on existing codebase): Baseline violations on log tenant. Governance improved SQL + repo params. **Effect present.**
- EXP-01–04b (existing well-structured codebases): Both conditions passed. **Effect absent** — existing code patterns were sufficient.

**Conclusion:** Compiled enforcement matters on **greenfield and new-domain** code where no existing patterns exist. On well-structured codebases with clean conventions, the code itself teaches Claude the patterns. The assumption holds for new code, not for additions to existing code.

**Caveat:** All experiments used `claude -p` (single-turn, fresh context, N=1-2). The assumption likely holds more strongly on long sessions with context degradation (see A-003).

### A-002: Self-review is insufficient — need fresh evaluator

**Mechanism:** Phase-end evaluator agent with no shared context.
**Test:** Have the generator self-evaluate its work, then run the independent evaluator on the same work. Compare findings.
**Last tested:** —
**Result:** —

### A-003: Context degrades after compaction — resets are better

**Mechanism:** Guided manual resets at phase boundaries.
**Test:** Run a multi-phase plan with compaction only vs with context resets between phases. Compare output quality on later phases.
**Last tested:** 2026-04-10
**Result:** Inconclusive — directional evidence only.

**Evidence (v0.3.0 experiments, Opus 4.6 / Claude Code 2.1.98):**
- EXP-08 (long-context invoicing, N=2): Injected ~1200 words of prior session context via `--system-prompt` to simulate a deep session.
  - Baseline: 1/2 violations (log calls missing `tenant_id` — the convention Claude follows in fresh context)
  - Governance-loaded: 0/2 violations (consistent pass)
  - Governance stabilized output under context pressure.
- Compared to EXP-07 (same task, fresh context): Baseline sometimes passed, sometimes failed. Long context made baseline failures more frequent.

**Conclusion:** Context noise degrades convention compliance — specifically on secondary concerns (log fields) rather than primary concerns (SQL). Governance directives in `.claude/rules/` survive context noise because they're loaded separately. This is directional evidence that governance value increases with context length.

**Caveat:** Experiment 08 doesn't isolate compaction vs resets. It tests "governance under context pressure" not "compaction vs fresh context." The specific mechanism (resets > compaction) remains untested. Anthropic's research confirms the mechanism independently.

### A-004: Acceptance criteria improve phase completion quality

**Mechanism:** Specs and plans include binary PASS/FAIL acceptance criteria per phase.
**Test:** Run the same plan with and without acceptance criteria. Compare evaluator pass rates.
**Last tested:** —
**Result:** —

### A-005: Evaluator skepticism catches more failures than neutral evaluation

**Mechanism:** Evaluator prompt says "assume the work is incomplete until proven otherwise."
**Test:** Run the evaluator with neutral prompt vs skeptical prompt on the same work. Compare false pass rates.
**Last tested:** —
**Result:** —

### A-006: Topic-grouped rules outperform flat governance files

**Mechanism:** `/edikt:compile` generates topic files loaded by path matching.
**Test:** Run the same coding task with flat governance.md (old format) vs topic-grouped rules. Compare directive compliance rates.
**Last tested:** —
**Result:** —

## Retired Assumptions

_None yet. As models improve, assumptions that are no longer necessary will be moved here with the retirement date and evidence._
