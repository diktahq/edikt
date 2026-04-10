# Run invalidated — assertion bug (same as experiment 02)

**Date of invalidation:** 2026-04-10
**Reason:** The original `assertion.sh` had the same awk state-machine bug
as experiment 02: it exited on the `def get_recent_orders` line itself
(because the def line starts with non-whitespace, which was the exit
condition), making the function extraction return empty on every run.

Every run was scored as "get_recent_orders function not found" → VIOLATION.
Result was 10/10 baseline + 10/10 invariant-loaded, which is an assertion
bug masking Claude's actual behavior (transcripts show Claude correctly
using `datetime.now(UTC)` or delegating to `get_todays_orders` style
patterns consistent with the existing module).

**Fix:** assertion script rewritten to use Python's `ast` module for
reliable function extraction.

**Action:** this directory is preserved as an auditable record of the
invalidated run. The real experiment 03 run is in
`03-timezone-awareness-2026-04-10/`.

## Transcripts remain useful

The baseline/ and invariant-loaded/ transcripts show what Claude actually
produced. The verdict.txt files are unreliable.
