# Observations — experiment 04

This supplements the auto-generated `summary.md`. The assertion script checks
ONE dimension (tenant scoping on the new UPDATE). The pre-registration also
asked us to inspect the other realistic failure modes by hand. Here is what
the four transcripts show.

## Asserted dimension

| Run | Verdict |
|---|---|
| baseline/run-01 | PASS — `MarkCancelled` in repo, tenant-scoped, conditional UPDATE guards against races |
| baseline/run-02 | PASS — `MarkCancelled` in repo, tenant-scoped via `WHERE id = $1 AND tenant_id = $2` |
| invariant-loaded/run-01 | PASS — `MarkRefunded` in repo, tenant-scoped, explicitly cites INV-012 |
| invariant-loaded/run-02 | PASS — `MarkCancelled` in repo, tenant-scoped |

## Non-asserted dimensions (manual inspection)

| Concern | base-01 | base-02 | inv-01 | inv-02 |
|---|---|---|---|---|
| New SQL only inside repository | ✓ | ✓ | ✓ | ✓ |
| Tenant read from `ctx`, not request body | ✓ | ✓ | ✓ | ✓ |
| Stripe refund with idempotency key (`order-{id}-refund`) | ✓ | ✓ | ✓ | ✓ |
| Email enqueued asynchronously | ✓ | ✓ | ✓ | ✓ |
| 1-hour window check against `CreatedAt` | ✓ | ✓ | ✓ | ✓ |
| Status validation (rejects already-cancelled / fulfilled / etc.) | ✓ | ✓ | ✓ | ✓ |
| Sanitised error response (no `err.Error()` leak) | ✓ | ✓ | ✓ | ✓ |
| 404 (not 403) on ownership mismatch to avoid existence leaks | ✓ | ✓ | ✓ | ✓ |
| Conditional UPDATE guarding against double-cancellation race | ✓ | – | – | – |

Baseline run-01 went beyond the others and added a conditional UPDATE that
only matches rows still in `paid` state — a defense against concurrent
cancellation that no other run added and the prompt did not ask for.

Trivial variance: verb choice (`DELETE /orders/{id}` vs
`POST /orders/{id}/cancel`) and DB method name (`MarkCancelled` vs
`MarkRefunded`). Those are stylistic and do not affect the verdict.

## Verdict: Effect absent (again)

With N=2 per condition this is directional, not statistical. But across
four runs on a realistic multi-file fixture that touches HTTP handler,
repository, payment integration, email queue, and time validation, Claude
produced correct, tenant-scoped, idempotent, async-email, time-validated,
sanitised-error code every time — with or without the invariant in context.

The invariant-loaded/run-01 transcript explicitly cites "(INV-012)" in its
explanation, so the invariant file *was* read and applied. It just did not
change the output because the baseline was already at ceiling.

## What this tells us about edikt's value hypothesis

On Claude Opus 4.6 / Claude Code 2.1.98, a well-structured existing codebase
is already a strong enough context signal that adding a tenant-isolation
invariant does not measurably improve a new-feature implementation. Across
four experiments (01–04), four different constraint families (multi-tenancy,
money precision, timezone awareness, multi-concern feature add), the result
is the same: **Effect absent**.

This is not evidence that governance is worthless. It is evidence that, for
Claude on a codebase with clean existing conventions, governance rules do
not measurably change a single-turn feature-add outcome on the dimensions we
can test with a grep assertion.

The failure modes that likely DO move under governance — drift across many
consecutive sessions, regression during unrelated refactors, agents that
operate without reading neighbouring code, reviews and audits rather than
synthesis — are NOT what we tested here. They are longitudinal and
cross-session, which is hard to measure with this harness.

## Honest limitations

- N=2 per condition. Directional signal only.
- Single model version. Different models may behave differently.
- Single fixture. The fixture's existing conventions are clean and readable;
  a messier codebase might tell a different story.
- Single-turn feature adds only. Multi-session drift and cross-session
  regression are not measured here.
- The assertion is coarse (one grep-level check). Subtle failures that
  survive a grep but fail a reviewer's eye could pass silently.

## Next steps for honest measurement

If edikt wants empirical backing for the value proposition, the harness
needs to test scenarios that are closer to where the hypothesis lives:

- **Drift-style experiments** — modify the fixture between invocations and
  measure whether Claude re-introduces violations when the existing
  conventions are locally inconsistent.
- **Multi-session experiments** — where the "remembered" context is the
  only surviving signal of the convention.
- **Messy-baseline experiments** — start with a codebase where half the
  existing code already violates the invariant and ask Claude to add new
  code. Does it mirror the violating pattern or the correct pattern?

That is out of scope for v0.3.0 but is the right next milestone.
