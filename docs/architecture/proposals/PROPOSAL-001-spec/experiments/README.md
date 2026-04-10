# Experiments — PROPOSAL-001 / v0.3.0

This directory contains the pre-registered experiments for v0.3.0's validation of the "Invariant Records help Claude follow cross-cutting rules" hypothesis.

## Purpose

**Primary goal**: engineering validation. Do invariants actually do what we think they do? Does loading an invariant into Claude's context reduce the rate at which Claude violates the rule?

**Secondary goal**: if the answer is yes, we have real evidence that can inform release notes, blog posts, or case studies. If the answer is no, we learn something we didn't know and either iterate the feature or reframe the narrative.

## Methodology

Publication-grade discipline applied as a shield against self-deception, not as preparation for external peer review. The distinction matters:

- **We're not writing a paper.** Presentation is informal (notebook-style results, no statistical tests, no academic writeup).
- **We are applying the discipline that would make results trustworthy even to ourselves.** Pre-registration, human-natural prompts, committed assertions before running, negative results honestly reported.

The rigor is for **us**, to prevent the unconscious biases that creep in when engineers validate their own tools:

- Subtly tuning prompts until Claude fails in the "right" way
- Re-running until variance lands favorably
- Adjusting assertion criteria post-hoc
- Quietly dropping experiments that didn't confirm the hypothesis

If we later decide the results are worth publishing formally, the rigor is already there. Upgrade presentation only.

## The loop

```
design experiment (fixture + prompt + assertion)
       │
       ├── commit design to git BEFORE running
       │
       ▼
    run N=10 per condition
       │
       ▼
    capture transcripts + pass/fail counts
       │
       ▼
    look at results honestly
       │
       ├── effect clear → learn, optionally share
       ├── effect mixed → iterate prompt or invariant
       ├── effect absent → hypothesis wrong, update thinking
       └── effect inverted → invariant made things worse, investigate
       │
       ▼
    decide: blog, release notes, more experiments, or move on
```

"Run", "capture", "look honestly" are disciplined. "Learn" and "decide" are reactive. The discipline is at the gates that prevent self-deception; the downstream decisions respond to whatever the data shows.

## Methodological commitments

Non-negotiable process rules. Violating any of these means the experiment is invalid and must be re-run.

1. **Pre-registration before running.** The experiment design (fixture, prompt, invariant, assertion logic, expected outcomes) is committed to git BEFORE any run. No post-hoc adjustments to the design.

2. **Human-natural prompts, reviewed for contamination.** Every prompt is read with the question "would a real engineer write this, and does any word in it hint at the invariant?" Words that hint at the invariant (e.g., "tenant", "secure", "safely", "proper", "precise", "decimal") are contamination and must be removed.

3. **Committed assertion logic.** The pass/fail check is written BEFORE the experiment runs and committed alongside the design. "Run and then figure out what 'pass' means" is post-hoc rationalization.

4. **N=10 per condition.** Ten runs in baseline (no invariant in context), ten runs with the invariant loaded. Smaller N risks mistaking noise for signal.

5. **Model version pinned and recorded.** Every run records the Claude Code version and Claude model version. Results are only comparable within the same version.

6. **Full transcripts preserved.** Every one of the 20 runs per experiment saves its full output. Summaries show the pass/fail counts, but transcripts are the evidence — readers (including future us) can inspect what actually happened.

7. **No quiet deletions.** If an experiment doesn't work out — baseline rate was too low to matter, invariant didn't help, something was contaminated — we keep the results and write up what we learned. We don't re-run silently and pretend the first run didn't happen.

8. **Iteration is allowed, but honestly.** If after seeing results we realize the experiment design was flawed (ambiguous assertion, contaminated prompt), we're allowed to redesign and re-run. But the original run stays in the results file with a note explaining why we re-ran.

## Known limitations (acknowledged up front)

Not flaws — just honest scope bounds on what these experiments can and cannot tell us.

1. **Context-size confound.** The invariant-loaded condition has ~400 more tokens in Claude's context than the baseline. Any observed effect could be "the invariant helped" OR "Claude is more careful with more context regardless of what the extra context contains". A proper control would be a third condition with an equivalent-length unrelated document in context. We do not run this control in v0.3.0 because the expected effect is dramatic enough that context-size alone shouldn't explain it. If results are ambiguous, we add the control condition in a follow-up.

2. **Single model, single Claude Code version.** Results are specific to the pinned versions. They may not generalize to other versions. We document which version was used and re-run if the model or tool upgrades materially.

3. **N=10 is small.** Ten runs per condition is enough to see a dramatic effect (e.g., 7/10 vs 0/10) but not enough to distinguish subtle effects (e.g., 4/10 vs 3/10). If the first experiment shows a subtle effect, we either increase N or acknowledge the result is directional, not conclusive.

4. **Single prompt per experiment.** A different phrasing of the same task might produce different results. We're not running a prompt-variance study; we're testing one natural phrasing.

5. **Single fixture per experiment.** A different project structure or language might produce different results. We're not running a generalizability study.

These limitations don't invalidate the experiments — they just bound what we can claim. An experiment that shows "Claude failed 7/10 times on this specific fixture with this specific prompt on this specific model" is still meaningful, even if we can't generalize to "Claude always fails at multi-tenancy".

## The three experiments

See individual design files:

- [`01-multi-tenancy.md`](01-multi-tenancy.md) — Does Claude scope database queries to tenant IDs?
- [`02-money-precision.md`](02-money-precision.md) — Does Claude use `Decimal` or `float` for money?
- [`03-timezone-awareness.md`](03-timezone-awareness.md) — Does Claude use timezone-aware datetimes?

Each file contains the pre-registered design: fixture setup, prompt (verbatim), invariant file (verbatim), assertion logic, expected outcomes, and placeholder for results.

## Infrastructure

All experiments share a common runner and directory structure. See [`runner-spec.md`](runner-spec.md) for the shared infrastructure contract.

## Reporting format

After running, each experiment produces a results file with:

- Date, Claude Code version, Claude model version
- N per condition
- Pass/fail counts per condition
- Sample transcripts (highlights; full transcripts linked)
- Hypothesis verdict (confirmed / weak / absent / inverted)
- Honest assessment: what we learned, what we'd do differently

Results files live next to the design files, named `01-multi-tenancy-results-YYYY-MM-DD.md`, etc.

## What we do with results

### Possible outcomes and responses

| Outcome | Response |
|---|---|
| **Strong effect confirmed** (e.g., baseline 7/10, invariant-loaded 0/10) | Update writing guide with real evidence. Potentially add a "Claude failure rates" section to the website. Consider a blog post citing the case study. Include in v0.3.0 release notes as empirical validation. |
| **Weak effect** (e.g., baseline 5/10, invariant-loaded 3/10) | Result is directional but not conclusive. Investigate: try a harder prompt, a different fixture, or acknowledge that this specific invariant isn't a dramatic case. Don't overclaim. |
| **No effect** (baseline ≤2/10) | Modern Claude is good enough at this task. The specific "Claude blind spot" hypothesis doesn't hold for this invariant on this model. The invariant still has value for teams — rules need to be written down — but the "catching Claude failures" framing doesn't apply. |
| **Inverted effect** (invariant-loaded > baseline) | Invariant made things worse. Real problem — investigate. Maybe the invariant wording confused Claude, maybe it triggered a different failure mode. Document and either fix the invariant or fix the feature. |

**In all cases**: the v0.3.0 feature ships regardless. The feature (template adaptation, three-list schema, etc.) is valuable even if the experimental hypothesis about Claude failure modes doesn't hold up.

## When to run

Recommended: during v0.3.0 release validation, after all phases have landed but before tagging the release. Results are committed alongside the release and inform the release notes.

If results reveal something significant that needs a feature change, the release can be delayed. If results are clean, the release ships with the evidence attached.

## See also

- [`../invariant-record-template.md`](../invariant-record-template.md) — The template these experiments test
- [`../writing-invariants-guide.md`](../writing-invariants-guide.md) — The guide whose claims these experiments would validate
- [ADR-008](../../../decisions/ADR-008-deterministic-compile-and-three-list-schema.md) — The compile mechanism that loads invariants into Claude's context
- [ADR-009](../../../decisions/ADR-009-invariant-record-terminology.md) — The formal coinage of "Invariant Record"
