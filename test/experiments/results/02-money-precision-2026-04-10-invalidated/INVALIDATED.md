# Run invalidated — assertion bug

**Date of invalidation:** 2026-04-10
**Reason:** The original `assertion.sh` used an awk state machine to extract
the `calculate_discount` function from `app/pricing.py`. The state machine
had an off-by-one error: it would exit on the `def calculate_discount`
line itself (because the def line starts with a non-whitespace character,
which was the exit condition), making the function extraction return
empty on every run.

The fallout: every run was scored as "calculate_discount function not
found" → VIOLATION. Result was 10/10 baseline + 10/10 invariant-loaded,
which looks like a real signal but is actually an assertion bug masking
Claude's actual behavior (which was correct in most runs — the transcripts
show Claude using Decimal consistently).

**Fix:** the assertion script was rewritten to use Python's `ast` module
to extract the function definition reliably. See
`test/experiments/fixtures/02-money-precision/assertion.sh` in the
corresponding commit.

**Action:** this directory is preserved as an auditable record of the
invalidated run. The real experiment 02 run is in
`02-money-precision-2026-04-10/` (re-run after the assertion fix).

## Transcripts remain useful

The baseline/ and invariant-loaded/ transcripts are still meaningful —
they show what Claude actually produced in 20 runs. It's just the
verdict.txt files that are unreliable. You can inspect the transcripts
directly to see the actual code Claude wrote.
