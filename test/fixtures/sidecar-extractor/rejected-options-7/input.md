---
type: adr
id: ADR-099
title: "ADR-099 — Logging Backend"
status: Accepted
date: 2026-05-03
deciders: Test Author
---

# ADR-099 — Logging Backend

**Status:** Accepted
**Date:** 2026-05-03

## Context

The system needs a structured logging backend.

## Considered Options

### A. zerolog (chosen)
- Pros: zero allocation, JSON output, fast
- Cons: no log sampling built in

### B. logrus
- Pros: widely used, plugin ecosystem
- Cons: allocates on every log call; deprecated by maintainer

### C. zap
- Pros: performant, structured
- Cons: complex API; requires sugar wrapper for ergonomics

### D. slog (stdlib)
- Pros: no dependency
- Cons: no JSON sink without adapter; slower than zerolog

### E. log15
- Pros: simple interface
- Cons: unmaintained since 2020; no active security patches

### F. apex/log
- Pros: middleware-style hooks
- Cons: no streaming JSON output; high memory overhead

### G. kit/log
- Pros: composable interface
- Cons: no level filtering built in; removed from active maintenance

## Decision

Structured logs MUST be emitted using zerolog.
All log entries MUST include a `level` field and a `ts` timestamp field.

## Consequences

### Good
- Minimal allocations in the hot path
