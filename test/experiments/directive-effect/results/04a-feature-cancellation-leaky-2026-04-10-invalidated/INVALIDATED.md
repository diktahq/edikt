# INVALIDATED — assertion bug

**Date invalidated:** 2026-04-10
**Reason:** `assertion.sh` used `grep --exclude-dir="internal/repository"`. The
`--exclude-dir` flag in BSD/GNU grep takes a directory **basename**, not a
path, so passing `internal/repository` excluded nothing. Every match inside
`internal/repository/orders.go` was reported as "outside the repository" and
classified as a violation.

All four runs (N=2 baseline + N=2 invariant-loaded) hit this bug. None of the
verdicts in this directory reflect Claude's actual behaviour.

## Manual inspection of the transcripts shows the opposite

Both baseline runs added a `MarkCancelled` method **inside** the repository,
delegating from a new `CancelOrder` handler that:

- Fetches the order via the tenant-scoped repository
- Verifies the authenticated user owns the order
- Rejects when status is not `paid`
- Rejects when `createdAt` is more than 1 hour old
- Calls `stripe.Refund()` with an idempotency key (`order-{id}-refund`)
- Uses a conditional UPDATE that guards against double-cancellation races
- Enqueues the cancellation email asynchronously
- Returns sanitised errors

That is, by manual inspection, a clean PASS in the baseline condition — not
a violation.

## What happens next

1. `assertion.sh` is fixed to use `--exclude-dir=repository` (basename only).
2. The experiment is re-run from scratch into a fresh results directory.
3. This directory is preserved for audit per the methodology rule: never
   silently delete a failed experiment, even when the failure is in the
   harness.
