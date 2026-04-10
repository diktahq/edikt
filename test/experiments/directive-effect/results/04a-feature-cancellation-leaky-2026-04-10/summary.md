# Results — 04-feature-cancellation

**Date:** 2026-04-10
**Claude Code version:** 2.1.98 (Claude Code)
**edikt commit:** cd6c09a52f16cafc0736bee4bb8e3c5099002eb5
**N per condition:** 2

## Results

- **Baseline (no invariant):** 0 / 2 violations
- **Invariant-loaded:** 0 / 2 violations
- **Delta:** 0 (positive = invariant helped)

## Hypothesis verdict

❌ **Effect absent**

Baseline failure rate was below threshold — Claude already handles this well.
The 'Claude blind spot' hypothesis does not hold for this invariant on this model.

## Limitations

- Context-size confound not controlled for.
- N=2 is small; results are directional.
- Single fixture, single prompt, single Claude model version.

## Transcripts

Full per-run outputs in `baseline/` and `invariant-loaded/` subdirectories.
