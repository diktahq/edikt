# Experiment 04: Feature add — order cancellation

**Type:** A — multi-file feature addition into a realistic existing service.
**Pre-registered:** 2026-04-02
**N per condition:** 2 (small — verdict informed by manual transcript inspection)
**Status:** committed before any run

## Why this experiment exists

Experiments 01–03 used toy fixtures (1–6 files, a single function to write).
All three came back **Effect absent**: Claude already handled the constraint
without any invariant in context, because the surrounding code telegraphed the
answer.

That is an honest result for those fixtures, but it does not test the failure
modes that motivate edikt in the first place: long-context drift, multi-file
features that touch several concerns simultaneously, and forgetting
mid-implementation.

This experiment tests a more realistic scenario — adding a feature that spans
HTTP handler, repository, payment integration, email queue, and time
validation — and asks: when Claude has to make several decisions at once,
does it remember the project's tenant-isolation invariant on the **new** code
paths it writes?

## Hypothesis

**H1:** When asked to add a multi-concern feature to a realistic backend
service, Claude is more likely to omit tenant scoping on at least one new
code path than when the tenant-isolation invariant is loaded into context.

**H0 (null):** Tenant scoping is preserved equally well in both conditions
because Claude pattern-matches off the existing repository code.

## What "realistic" means here

The fixture is ~22 Go files representing a small backend service.

The existing repository scopes by tenant via `ctx.Value(ctxkeys.TenantID)` —
but the method names are normal (`GetByID`, `Create`, `ListByUser`), **not**
`GetByIDAndTenant`. The repository's package doc mentions tenant scoping in
one sentence. This matches how real codebases look: the convention exists,
but it isn't shouted from every identifier.

Claude has to either follow the pattern by reading the existing code, or it
has to be reminded by the invariant. We want to know which.

## Failure modes we are watching for (manual inspection)

The assertion checks ONE dimension automatically: does the new code that
modifies orders preserve tenant scoping? Transcripts will be inspected by
hand for these other realistic failures:

- Pulls tenant from the request body or URL parameter instead of context
- Writes raw SQL in the handler instead of going through the repository
- Adds a new query path that omits the tenant filter
- Calls Stripe refund without an idempotency key
- Sends the cancellation email synchronously instead of via the queue
- Returns raw `err.Error()` to the client (information leak)
- Forgets the 1-hour window check
- Returns the wrong status code on not-found / already-cancelled

These are **not** part of the pass/fail verdict — only tenant scoping is.
They are recorded as observations.

## Pass criteria (assertion-checked)

The new code that updates an order's status to "cancelled":

1. No raw `UPDATE orders` SQL outside `internal/repository/`, AND
2. Either (a) no new SQL in the repository at all — the handler delegates
   to existing tenant-scoped repository methods — or (b) any new
   `UPDATE orders` statement in the repository includes a `tenant_id`
   filter in the same statement.

## Violation criteria

- Raw SQL UPDATE on `orders` outside `internal/repository/`
- New repository SQL that touches `orders` without filtering by `tenant_id`

## Limitations stated honestly

- **N=2** per condition is too small for any statistical claim. This is a
  directional sanity check, not evidence. We chose N=2 to keep the wall-clock
  time and the manual-inspection burden tractable.
- Single fixture, single invariant, single Claude model version.
- The verdict is informed by judgment during manual transcript review.
- The fixture's existing code patterns may still be telegraphic enough that
  Claude pattern-matches perfectly in baseline. If so, that is an honest
  "effect absent" result and we report it as such.

## Why we will report whatever we find

If H0 wins again, that is a real and useful finding: it means the
realistic-codebase pattern-matching defense is strong enough that an
invariant adds little marginal value for this kind of feature add.

If H1 wins, we have one data point — not proof, but a starting place for
a longer study.

Either result is published.
