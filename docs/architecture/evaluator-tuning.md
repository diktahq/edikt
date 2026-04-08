# Evaluator Tuning

This document tracks evaluator accuracy over time. After each phase-end evaluation, record whether the findings were accurate. Use this data to refine the evaluator agent prompt.

## Calibration Notes

_Add notes here when the evaluator produces inaccurate results. These notes are read by the evaluator agent at the start of each evaluation._

## Log

| Date | Phase | Verdict | Accurate? | Notes |
|------|-------|---------|-----------|-------|
| — | — | — | — | _No evaluations recorded yet_ |

## False Positives (evaluator failed work that was correct)

_Record cases where the evaluator said FAIL but the work was actually complete. These indicate the evaluator is too strict._

## False Negatives (evaluator passed work that later broke)

_Record cases where the evaluator said PASS but issues were discovered later. These indicate the evaluator is too lenient._

## Prompt Refinements

| Date | Change | Reason |
|------|--------|--------|
| — | Initial prompt | Skeptical by default, binary PASS/FAIL |
