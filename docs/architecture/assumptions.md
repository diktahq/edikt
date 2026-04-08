# Harness Assumptions

Every governance mechanism in edikt encodes an assumption about what the model can't do reliably on its own. As models improve, some assumptions become unnecessary overhead. This document tracks assumptions, how to test them, and when they were last validated.

Re-test assumptions when upgrading edikt or when a new model version is released.

## Active Assumptions

### A-001: Agents won't follow ADRs without compiled enforcement

**Mechanism:** `/edikt:compile` generates rule files loaded into every session.
**Test:** Run a coding task that touches an ADR-governed area without compiled rules loaded. Check if the agent follows the ADR decision anyway.
**Last tested:** —
**Result:** —

### A-002: Self-review is insufficient — need fresh evaluator

**Mechanism:** Phase-end evaluator agent with no shared context.
**Test:** Have the generator self-evaluate its work, then run the independent evaluator on the same work. Compare findings.
**Last tested:** —
**Result:** —

### A-003: Context degrades after compaction — resets are better

**Mechanism:** Guided manual resets at phase boundaries.
**Test:** Run a multi-phase plan with compaction only vs with context resets between phases. Compare output quality on later phases.
**Last tested:** —
**Result:** —

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
