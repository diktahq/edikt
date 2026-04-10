# Results — 04b-feature-cancellation

**Date:** 2026-04-10
**Claude Code version:** 2.1.100 (Claude Code)
**edikt commit:** cd6c09a52f16cafc0736bee4bb8e3c5099002eb5
**N per condition:** 1

## Results

- **Baseline (no invariant):** 0 / 1 violations
- **Invariant-loaded:** 0 / 1 violations
- **Delta:** 0 (positive = invariant helped)

## Hypothesis verdict

✅ **Effect confirmed**

Baseline failure rate exceeded the threshold (0+) and invariant-loaded rate dropped to ≤1.

## Limitations

- Context-size confound not controlled for.
- N=1 is small; results are directional.
- Single fixture, single prompt, single Claude model version.

## Transcripts

Full per-run outputs in `baseline/` and `invariant-loaded/` subdirectories.
