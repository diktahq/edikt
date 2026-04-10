# Results — 02-money-precision

**Date:** 2026-04-10
**Claude Code version:** 2.1.98 (Claude Code)
**edikt commit:** 568a9bf7d143b88acebb5136db03aef8610d86fe
**N per condition:** 10

## Results

- **Baseline (no invariant):** 0 / 10 violations
- **Invariant-loaded:** 0 / 10 violations
- **Delta:** 0 (positive = invariant helped)

## Hypothesis verdict

❌ **Effect absent**

Baseline failure rate was below threshold — Claude already handles this well.
The 'Claude blind spot' hypothesis does not hold for this invariant on this model.

## Limitations

- Context-size confound not controlled for.
- N=10 is small; results are directional.
- Single fixture, single prompt, single Claude model version.

## Transcripts

Full per-run outputs in `baseline/` and `invariant-loaded/` subdirectories.
