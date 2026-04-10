# edikt Experiments

Three experiment suites measuring how governance affects AI agent output.

## Suites

### [rule-compliance/](rule-compliance/) — "Does the rule get followed?"

v0.1.x era. Tests whether rule packs in `.claude/rules/` cause Claude to follow conventions it wouldn't follow without them. 123 eval runs across two experiments. Result: rules drive 100% compliance on conventions Claude has never seen in training.

- **exp-001** — 60 runs across 5 scenarios (security, Go errors, architecture layers, testing, Next.js). 15/15 with rules, 0/15 without.
- **exp-002** — 63 runs across 4 stress conditions (contradictory rules, multi-file, Opus vs Sonnet, adversarial prompts). Near-100% compliance held.

### [directive-effect/](directive-effect/) — "Does the directive change the output?"

v0.3.0 era. Tests whether compiled governance directives (MUST/NEVER language, sentinel blocks) change what Claude builds on greenfield, new-domain, and long-context tasks.

8 experiments, 4 scenario types:

| Scenario | Experiments | Finding |
|---|---|---|
| Existing codebase | 01-04b | Effect absent — code patterns self-teach |
| Greenfield | 05-06 | **Effect present** — governance prevents architecture/tenant violations |
| New domain on existing | 07 | **Effect present** — governance catches log/SQL misses |
| Long context | 08 (N=2) | **Effect present** — governance stabilizes under context pressure |

Key finding: directive format matters. MUST/NEVER + literal code tokens outperforms prose.

### [long-running/](long-running/) — "Does governance help across sessions?"

Designed, not yet run. Tests multi-turn compliance, context compaction recovery, multi-rule load, and real-world conventions. Requires harness improvements (LLM evaluator, structured criteria) from [PLAN-long-running-harness](../../docs/plans/PLAN-long-running-harness.md).

Four hypotheses from the original v0.1.x exp-003 BRIEF, evolved with v0.3.0 findings:
- H1: Multi-rule compliance (14+ rules loaded)
- H2: Compaction recovery (PostCompact re-injection)
- H3: Multi-turn compliance (5+ turns)
- H4: Real conventions (from actual codebases)

## Methodology

All experiments follow pre-registration discipline:
- Design committed before running
- Human-natural prompts (no contamination words)
- Committed assertions (can't change after seeing results)
- Honest negative results published (experiments 01-04b all showed "effect absent")
- Invalidated runs preserved with audit notes (never silently deleted)

## Running experiments

```bash
# Directive-effect suite
EDIKT_EXP_N=2 ./test/experiments/directive-effect/run.sh 06-greenfield-tenant

# Rule-compliance suite (manual — see rule-compliance/README.md)
cd test/experiments/rule-compliance/exp-001-scenarios
bash run.sh
```
