# INV-016: Timestamps are timezone-aware

**Date:** 2026-04-09
**Status:** Active

<!--
Writing guidance (see ADR-009 for the template contract):

1. Describe the CONSTRAINT, not the IMPLEMENTATION.
2. Present tense, declarative, no hedging.
3. Invariants are NOT derived from ADRs. They stand alone.
4. An invariant without Enforcement is a wish.
-->

## Statement

All datetime values in the system — stored, transmitted, computed, compared, or logged — carry explicit timezone information. Naive datetimes (Python `datetime` without `tzinfo`) are forbidden.

## Rationale

Naive datetimes are a silent bug factory. Python's `datetime.now()` returns a naive datetime that assumes the local system's timezone — which is usually set to UTC on servers but might be anything on a developer's machine. Comparing a naive datetime against a timezone-aware one raises TypeError (if you're lucky) or produces silently wrong results in libraries that coerce (if you're not).

The constraint applies to all layers: storage uses timezone-aware types (PostgreSQL `TIMESTAMP WITH TIME ZONE`), application code uses `datetime.now(UTC)` or equivalent, API responses include timezone information in serialized form (ISO 8601 with `Z` or offset).

## Consequences of violation

- **Silent wrong results on DST transitions.** A naive datetime is interpreted in local time, which changes twice a year in most regions. A "last 24 hours" query can return 23 or 25 hours of data without warning.
- **Silent wrong results across server timezone changes.** Move your server from UTC to a different tz and all naive datetime comparisons silently change meaning.
- **TypeError in strict comparison paths.** Python raises TypeError when comparing naive and aware datetimes. This surfaces as a 500 error to users, but only on code paths that actually hit the mixed comparison — which can be deeply buried.
- **Off-by-hours bugs in reports.** "Last week" queries return slightly wrong data; aggregations drift.

## Implementation

- Use `datetime.now(UTC)` instead of `datetime.now()`. Import `UTC` from `datetime` (Python 3.11+) or use `pytz.UTC` / `timezone.utc` for older versions.
- Database columns use `TIMESTAMP WITH TIME ZONE` (`timestamptz` in Postgres). Never `TIMESTAMP WITHOUT TIME ZONE`.
- ORM configurations (SQLAlchemy, Django, etc.) have explicit `timezone=True` on datetime fields.
- API serialization uses ISO 8601 with timezone indicator (`Z` for UTC or an explicit offset like `+00:00`).

## Anti-patterns

- `datetime.now()` without a timezone argument.
- `datetime.utcnow()` — this returns a naive datetime in UTC, confusingly. Use `datetime.now(UTC)` instead.
- Storing datetimes as strings or Unix timestamps to "sidestep" the issue — this loses type safety and pushes the problem to deserialization.
- Comparing naive and aware datetimes with implicit coercion — if the library allows it, you get silently wrong results.

## Enforcement

- **Linter rule**: grep for `datetime.now()` without arguments and `datetime.utcnow()` fails pre-commit hook.
- **ORM model schema validation**: datetime fields must have `timezone=True`.
- **Database schema linter**: migrations with `TIMESTAMP WITHOUT TIME ZONE` fail pre-push.
- **edikt directive**: "All datetimes must be timezone-aware. Never use `datetime.now()` without a timezone argument."
- **Code review checklist**: datetime handling is a review checkpoint.

<!-- Directives for edikt governance. Populated by /edikt:invariant:compile. -->
[edikt:directives:start]: #
[edikt:directives:end]: #
