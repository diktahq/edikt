# Experiment 08: Long-context invoicing — simulated session degradation

**Type:** Feature-addition with simulated long prior session context.
**Pre-registered:** 2026-04-10
**N per condition:** 1
**Status:** committed before any run

## Why this experiment exists

Experiments 05-07 tested single-turn, fresh-context scenarios. The
conclusion "governance doesn't help on existing codebases" was
challenged by Anthropic's harness design article, which shows context
degradation is real and progressive in long sessions.

This experiment tests whether pre-filling the context with a long
prior session (~3000 words of unrelated tasks) degrades Claude's
attention on existing code patterns enough that governance in
`.claude/rules/` makes a measurable difference.

## Design

Identical to experiment 07 (checkout + invoicing) but with a
`system-prompt.txt` injected via `--system-prompt` simulating 12
completed tasks from earlier in the session.

The governance directives are in `.claude/rules/governance.md` which
Claude Code auto-loads separately from the conversation context.

## Hypothesis

**H1:** With long prior context, Claude misses at least one tenant
discipline on new invoicing code. Governance catches it.

**H0:** Context noise doesn't degrade attention. Baseline passes.
