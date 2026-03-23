<!--
  Experiment write-up template for edikt.
  Full template with guidance and examples: 90 - Templates/ProductResearcher/experiment-write-up.md
  Rigor checklist at the bottom — verify before publishing.
-->
---
title: "EXP-{NNN}: {Descriptive Name}"
description: "One sentence. What was tested and the headline finding."
date_conducted: YYYY-MM-DD
models: [claude-sonnet-4-20250514]
edikt_version: 0.1.0
total_runs: 0
---

# EXP-{NNN}: {Name}

## Summary

3-5 sentences. State the question, the method, the headline result, and the primary limitation. A reader who stops here should have the essential picture.

Write this last, after the rest is done. No "this paper presents..." — state what you tested, how, what you found, and the biggest caveat.

## Hypothesis

State one or more falsifiable hypotheses. Each has a corresponding null hypothesis.

Write these before running the experiment. If written after, state that honestly.

**H1:** {Falsifiable hypothesis — specific enough that a number could disprove it}

**H1-null:** {What you'd observe if H1 is wrong}

## Variables and controls

| Element | Value |
|---|---|
| **Independent variable** | {What you manipulated} |
| **Dependent variable** | {What you measured — include how pass/fail is determined} |
| **Controlled** | {What was held constant — model, prompt, scaffold, temperature, turn count} |
| **Measurement method** | {How scoring works — automated on disk, not output text parsing} |

## Method

### Conditions

What groups/variants were tested and what differed between them.

### Materials

What prompts, rule files, project scaffolds were used. Link to repo or include inline.

### Procedure

What happened in each run, step by step. How was isolation maintained?

### Measurement

How was the dependent variable scored? What counts as pass vs fail?

The test: could a skeptical reader reproduce this with no other information?

## Results

Raw data first, then summary statistics. No interpretation in this section.

Present per-condition results, not just aggregates. Show all trials, not just averages. State sample size per condition. If any runs were excluded, state which and why.

```
Condition              Trial 1   Trial 2   Trial 3   Rate
----------------------------------------------------------
{condition-a}              x/x       x/x       x/x    xx%
{condition-b}              x/x       x/x       x/x    xx%
{control}                  x/x       x/x       x/x    xx%
```

## Interpretation

For each hypothesis, state whether the data supports or fails to reject it.

Use precise language: "the data is consistent with H1" not "we proved H1." Ground every claim in a specific number from the results.

## Threats to validity

### Internal validity

Could something other than the independent variable explain the results? (prompt leakage, scorer bugs, model memorization, convention difficulty)

### External validity

Do these results generalize? (single model, single-turn, English-only, invented vs real-world conventions, sample size)

### Construct validity

Does the measurement capture what we claim? (binary pass/fail misses partial compliance, file-based scoring misses runtime behavior)

## Related experiments

Links to experiments that extend, contradict, or build on this one.

## Reproduce it

### Prerequisites

What you need: model, CLI version, API key, OS.

### Commands

```bash
cd experiments/exp-{NNN}-{slug}
./setup.sh && ./run.sh && python3 score.py
```

### Expected output

Describe what a successful run looks like: what files get created, what the scorer prints, how long it takes, approximate cost.

---

## Rigor checklist

Verify before publishing. If any answer is "no," fix it or note it in Threats.

**Design:**
- [ ] Hypothesis written before the experiment was run
- [ ] Null hypothesis stated with specific result that would support it
- [ ] Control condition (no treatment) with results reported
- [ ] Independent, dependent, and controlled variables declared
- [ ] Sample size stated per condition
- [ ] Small sample size (< 30/condition) acknowledged as limitation

**Execution:**
- [ ] Each run isolated (no shared state)
- [ ] Measurement automated, not manual judgment
- [ ] No runs excluded without explanation
- [ ] Scorer checks artifacts on disk, not model output text

**Reporting:**
- [ ] Results section contains only data, no interpretation
- [ ] Interpretation references specific numbers from results
- [ ] All three validity threats have at least one entry
- [ ] The word "prove" does not appear
- [ ] Reproduction section includes expected output
- [ ] Summary states the primary limitation

**Honesty:**
- [ ] Experiment could have produced a negative result
- [ ] At least one threat explains why positive results might not generalize
- [ ] No pre-judging adjectives ("comprehensive," "robust," "highly effective")
