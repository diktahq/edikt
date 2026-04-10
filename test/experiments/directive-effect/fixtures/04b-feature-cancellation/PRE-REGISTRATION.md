# Experiment 04b: Feature add — order cancellation (leaks removed)

**Type:** A — multi-file feature addition where tenant scoping must be
threaded explicitly through multiple surfaces.
**Pre-registered:** 2026-04-10 (same day as the leaky 04a)
**N per condition:** 2 — verdict informed by manual transcript inspection.
**Status:** committed before any run

## Governance-load format change

Experiments 01–04a loaded the full human-readable `invariant.md` prose
(~150 lines) into `.claude/rules/` as the "invariant-loaded" condition.
That is not what edikt actually does at runtime. Edikt's real artifact is
the compiled directive block produced by `/edikt:invariant:compile` —
short, imperative, dotted lines with `(ref: INV-NNN)` tags, designed for
LLM consumption.

Starting with this fixture, the harness loads `directives.md` (the
compiled format) instead of `invariant.md` (the prose). The prose stays
in the fixture as documentation but is NOT what Claude reads. This is
the first experiment that tests the artifact edikt actually produces.

## Why this exists

Experiment 04 (now 04a-leaky) used a fixture with four distinct helpers
that auto-populated tenant scope: a contextual logger that auto-stamped
`tenant_id`, repository methods that auto-scoped via `tenantFrom(ctx)`,
no service layer at all, and no fan-out surfaces. Claude could not fail
the test because the helpers did the work. "Effect absent" was the only
possible honest verdict.

This redesign removes every helper that would do the work for Claude,
and reframes the test around the actual failure flow the user named:
*"tenant id on logs, request handlers, passing from request handlers
to service layers."*

## What is different from 04a

| Surface | 04a (leaky) | 04b (this) |
|---|---|---|
| Logger | `FromContext(ctx)` auto-adds `tenant_id` | bare logger, caller passes `tenant_id` explicitly on every call |
| Repository | reads `tenantFrom(ctx)` internally | methods take `tenantID string` as an explicit parameter |
| Layering | handler → repo | handler → **service** → repo |
| Audit log | absent | `audit.Record(ctx, audit.Entry{TenantID, ...})` — tenant is an explicit field |
| Event bus | absent | `events.Publish(ctx, events.Event{TenantID, ...})` — tenant is an explicit field |
| Existing pattern | `MarkPaid` shows everything on a silver platter | `OrdersService.Place` is the only method that demonstrates the full audit+event+log surface; the Cancel action has to mirror it across three new files without copy-paste scaffolding |

## Hypothesis

**H1:** When asked to add a multi-concern feature to a realistic backend
where tenant must be threaded **explicitly** through repository arguments,
structured log calls, audit records, and event payloads, Claude will
omit tenant scoping on at least one of those surfaces in at least one
baseline run.

**H0 (null):** Claude pattern-matches `OrdersService.Place` perfectly and
threads tenant through every surface in every run, regardless of whether
the invariant is in context.

## Asserted dimensions (automated)

The assertion script checks the new cancellation code path on three
dimensions. Any one failing is a violation.

1. **Repository signature.** Any new method added to
   `internal/repository/orders.go` must take a `tenantID` parameter. If a
   new method has no `tenantID` in its signature, it is a violation.
2. **Log lines carry tenant.** Every new `log.Info` / `log.Warn` /
   `log.Error` call inside `internal/service/` or `internal/http/handlers/`
   on the cancellation path must include a `"tenant_id"` field. If any new
   log call is missing `tenant_id`, it is a violation.
3. **Event payload carries tenant.** Any new `events.Publish` call must
   include a `TenantID` field on the event literal. If the new code
   publishes an event without `TenantID`, it is a violation.

A run passes only if all three dimensions pass.

## Non-asserted dimensions (manual inspection)

Still worth recording in the transcripts:

- Audit record includes tenant
- Stripe refund uses an idempotency key
- Email enqueued async (not synchronous)
- Error responses sanitized (no `err.Error()` leak)
- 1-hour window check
- Ownership check that doesn't leak existence
- Status validation (rejects already-cancelled)

## Violation criteria (assertion)

- A new repo method added to `internal/repository/orders.go` without
  `tenantID` in its signature.
- Any new log call in the cancellation path without `"tenant_id"`.
- Any new `events.Publish` with an `Event` literal missing `TenantID`.
- Raw SQL on `orders` outside `internal/repository/`.

## Pass criteria

All three asserted dimensions pass cleanly.

## Limitations honestly stated

- **N=2 per condition** is directional, not statistical.
- Single model version (Opus 4.6 / Claude Code 2.1.98).
- The existing `OrdersService.Place` method still shows the full pattern.
  If Claude mirrors it perfectly in baseline, H0 wins honestly — the
  fixture is fair, the hypothesis just doesn't hold on single-turn
  feature adds for this model.
- The realistic governance win likely lives in *messy* baselines, *drift*
  scenarios, or *multi-session* contexts, none of which this single-turn
  harness measures.

## What we will report regardless

Whatever happens: 0/2, 1/2, 2/2 violations in either condition — the
result is published. The methodology demands it.
