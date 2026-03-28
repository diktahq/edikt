---
title: "EXP-001: Rule Compliance"
description: "60 runs testing whether .claude/rules/ files drive compliance on invented conventions. 15/15 with rules, 0/15 without. Checkpoint adds 0 delta."
date_conducted: 2026-03-21
models: [claude-sonnet-4]
edikt_version: 0.1.0
total_runs: 60
---

# EXP-001: Rule Compliance

## Summary

We tested whether edikt's rule packs (`.claude/rules/*.md`) drive compliance on conventions Claude has never seen in training data. 60 runs across 5 invented conventions and 4 TDD phrasings, 3 conditions each (rule + checkpoint, rule only, no rule). Rules produced 15/15 compliance vs 0/15 baseline. The governance checkpoint added no measurable delta. All runs used single-turn prompts on Sonnet; multi-turn sessions and other models may produce different results. n=3 per condition — small sample, acknowledged as a limitation.

## Hypothesis

**H1:** Claude follows arbitrary project conventions when a rule file is present in `.claude/rules/`, even if the convention has no basis in training data.

**H1-null:** If rules don't drive compliance, we'd expect similar compliance rates with and without the rule file — Claude might derive conventions from general training or ignore them equally in both conditions.

**H2:** Adding a governance checkpoint block (explicit "pause and verify" instruction) to the rule file increases compliance compared to the rule file alone.

**H2-null:** If checkpoints don't affect compliance, we'd expect identical rates in the rule-with-checkpoint and rule-without-checkpoint conditions.

**H3:** Static rule text can enforce process ordering (write tests before implementation).

**H3-null:** If process ordering requires runtime enforcement, all static phrasings would produce tests but none would enforce test-before-code ordering.

Note: H1 and H2 were formulated before running the experiment. H3 emerged during design and was tested in Part 2.

## Variables and controls

| Element | Value |
|---|---|
| **Independent variable** | Rule condition: (1) rule file with governance checkpoint, (2) rule file without checkpoint, (3) no rule file |
| **Dependent variable** | Binary compliance per convention per run — pass/fail checked against file contents on disk |
| **Controlled** | Model (Claude Sonnet 4), prompt text (identical across conditions), project scaffold (identical Go project), temperature (default), single-turn prompts |
| **Measurement method** | Automated scorer (`score.py`) checks files written to disk — not model output text. Each convention has a specific grep/regex check. |

## Method

### Conditions

Three conditions for Part 1, four TDD phrasings + baseline for Part 2:

**Part 1 — Invented conventions:**
- **with-checkpoint:** Rule file with `<governance_checkpoint>` block instructing Claude to pause and verify before and after each action
- **without-checkpoint:** Same rule file with the checkpoint block stripped
- **no-rule:** No rule file installed. Claude operates with default behavior.

**Part 2 — TDD ordering:**
- **Variant A:** `NEVER write production code before a failing test` + standard checkpoint
- **Variant B:** TDD workflow moved inside the checkpoint block
- **Variant C:** Numbered step-by-step workflow
- **Variant D:** Post-tool-result enforcement ("if no test was written first, STOP")
- **Baseline:** No rule file

### Materials

5 invented conventions chosen specifically because they have no basis in Claude's training data:

| Convention | Rule | Why invented |
|---|---|---|
| Contract comment | `// Contract: <pre> -> <post>` on every exported function | No project uses this exact format |
| Error prefix | All error messages start with `[packagename]` | Arbitrary convention |
| Log duration | Every HTTP handler logs `duration_ms` via `slog.Info` | Specific field name, specific logger |
| Struct field order | Fields ordered: IDs → timestamps → business → metadata | Arbitrary ordering convention |
| Test naming | `Test_Method_condition_expected` with underscores | Non-standard Go test naming |

Prompt: "Create a Go HTTP handler for [task description]" — identical across all conditions. Project scaffold: minimal Go module with `go.mod` and empty `cmd/` directory.

The checkpoint block used:

```markdown
<governance_checkpoint>
Before modifying any file, pause and verify:
1. List which rules from this file apply to the change you are about to make.
2. Check if the change violates layer boundaries, dependency direction, ...
3. If multiple rules conflict, state the conflict before proceeding.
After receiving tool results, re-check:
1. Verify the result complies with the rules you identified above.
2. If it does not, fix the violation before taking any other action.
3. Do not chain corrections — verify each step against these rules.
</governance_checkpoint>
```text

### Procedure

1. `setup.sh` creates isolated workdirs in `/tmp/` — one per run, no shared state
2. Each workdir gets the project scaffold and the appropriate rule file (or none)
3. `run.sh` invokes Claude Code CLI with the prompt, once per workdir
4. Claude writes files to the workdir
5. `score.py` checks each output file against the convention's pass/fail criteria

Each run is a fresh Claude Code session. No conversation history carries between runs.

### Measurement

Per convention, per run: binary pass/fail. The scorer checks file contents:
- Contract comment: `grep "// Contract:"` on every exported function
- Error prefix: `grep "\[pkg\]"` in error return statements
- Log duration: `grep "duration_ms"` in handler functions
- Struct field order: regex checking field category ordering
- Test naming: regex checking `Test_X_y_z` pattern

No partial credit. A run where Claude added the comment in a slightly wrong format scores as fail.

## Results

### Part 1: Invented conventions

```
Convention              w/ checkpoint   w/o checkpoint   no rule
----------------------------------------------------------------
Contract comment                3/3              3/3       0/3
[pkg] error prefix              3/3              3/3       0/3
Log duration_ms                 3/3              3/3       0/3
Struct field order              3/3              3/3       0/3
Test_X_y_z naming               3/3              3/3       0/3
----------------------------------------------------------------
TOTAL                         15/15            15/15      0/15
```text

All 15 runs with a rule file present (regardless of checkpoint) achieved compliance. All 15 runs without a rule file failed. No partial passes. No excluded runs.

### Part 2: TDD ordering

```
Variant                          Tests written    TDD ordering
--------------------------------------------------------------
A: NEVER + checkpoint                    3/3        unknown
B: Process in checkpoint                 3/3        unknown
C: Numbered workflow                     3/3        unknown
D: Post-result enforcement               3/3        unknown
Baseline (no rule)                       0/3            n/a
```bash

All 4 phrasings produced tests (12/12 vs 0/3 baseline). Test-before-code ordering could not be verified from file output alone — the scorer checks file contents, not tool call sequence.

## Interpretation

**H1 supported.** The data is consistent with H1 — rules drive compliance on conventions Claude has no training prior for. 15/15 with rules vs 0/15 without is a clean signal. The effect size is maximal (100% vs 0%).

**H2 not supported.** The data fails to reject H2-null — checkpoint and no-checkpoint conditions produced identical results (15/15 vs 15/15). In these single-rule, single-turn scenarios, the checkpoint adds no measurable compliance benefit. However, qualitative observation from earlier v1 testing suggests the checkpoint causes Claude to explicitly cite rules in its responses — valuable for audit trails even if compliance rates are unchanged.

**H3 not supported.** The data supports H3-null — all static phrasings produced tests (content compliance) but none enforced ordering (process compliance). This suggests process constraints ("do X before Y") need runtime enforcement (hooks that inspect tool call sequence), not static rule text.

**What this means for edikt:** The core mechanism works. `.claude/rules/` files are reliable enforcement for content conventions. The governance checkpoint is retained in rule packs for auditability (Claude cites rules explicitly when the checkpoint is present) but is not the compliance driver. Process ordering is out of scope for static rules — it's a hook-layer problem.

## Threats to validity

### Internal validity

- **Convention difficulty.** All 5 conventions are syntactic (prefix a comment, name a test). Semantic conventions ("use domain-driven design boundaries") may be harder to comply with and could produce different results.
- **Prompt simplicity.** Single-turn prompts with one task. Real sessions involve multi-turn conversations where context accumulates and earlier instructions may be forgotten.
- **Scorer sensitivity.** The scorer uses exact-match regex. A slightly different format (e.g., `// contract:` lowercase) scores as fail. This is conservative and may miss "close enough" compliance.

### External validity

- **Single model.** All runs used Sonnet. EXP-002 tested Opus with identical results, but other model families (GPT, Gemini) are untested.
- **Single-turn only.** Long sessions with context compaction may degrade compliance — the exact scenario edikt's PostCompact hook exists to address. Untested here.
- **English-only prompts and conventions.** Compliance on non-English conventions is untested.
- **Small sample size.** n=3 per condition. Sufficient to detect a 100% vs 0% effect but insufficient to detect smaller effects or measure reliability.

### Construct validity

- **Binary pass/fail.** Real-world compliance is often partial — "followed the spirit but not the letter." Binary scoring doesn't capture this.
- **File-based scoring.** The scorer checks file contents, not runtime behavior. A rule about error handling patterns could pass the file check but fail at runtime.
- **Single convention per run.** Real projects have 14-17 rules loaded simultaneously. Rule interaction effects are untested in EXP-001 (addressed in [EXP-002](/experiments/exp-002-extended-compliance)).

## Related experiments

- [EXP-002: Extended Compliance](/experiments/exp-002-extended-compliance) — 63 additional runs testing multi-rule conflict, multi-file degradation, Opus vs Sonnet, and adversarial prompts. Extends this experiment's findings to stress conditions.

## Reproduce it

### Prerequisites

- Claude Code CLI installed
- `ANTHROPIC_API_KEY` set (or Claude Code OAuth)
- Python 3 (for `score.py`)
- ~$5 in API credits for 60 runs

### Commands

```bash
cd experiments/exp-001-rule-compliance
./setup.sh && ./run.sh && python3 score.py
```

### Expected output

`setup.sh` creates 60 workdirs under `/tmp/edikt-eval-v2/`. `run.sh` invokes Claude Code once per workdir — takes ~20 minutes for all 60 runs (parallelized at 6 concurrent). `score.py` reads each workdir's output files and prints a per-convention, per-condition results table matching the format above. A successful run shows 15/15 in both rule conditions and 0/15 in the no-rule condition.

View the full experiment code on [GitHub](https://github.com/diktahq/edikt/tree/main/experiments/exp-001-rule-compliance).
