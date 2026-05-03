---
type: adr
id: ADR-099
title: "ADR-099 — Background Job Processing"
status: Accepted
date: 2026-05-03
deciders: Test Author
---

# ADR-099 — Background Job Processing

**Status:** Accepted
**Date:** 2026-05-03

## Context

The system needs to process jobs asynchronously without blocking the request path.

## Decision

Jobs MUST be processed in a dedicated worker goroutine pool.
Workers MUST persist job state to the jobs table before returning.

Fallback: direct synchronous processing MAY be used when the worker pool is unavailable during startup.

## Consequences

### Good
- Non-blocking request handling
- Resilient to transient failures

### Bad
- Added operational complexity
