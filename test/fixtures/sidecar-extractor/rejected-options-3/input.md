---
type: adr
id: ADR-099
title: "ADR-099 — State Persistence Strategy"
status: Accepted
date: 2026-05-03
deciders: Test Author
---

# ADR-099 — State Persistence Strategy

**Status:** Accepted
**Date:** 2026-05-03

## Context

The system needs to persist state across restarts.

## Considered Options

### A. SQLite (chosen)
- Pros: embedded, zero external dependency, ACID compliant
- Cons: no concurrent writers

### B. Redis
- Pros: fast, supports pub/sub
- Cons: external dependency required; data loss on crash without AOF

### C. In-memory map
- Pros: simplest implementation
- Cons: all state lost on restart; not suitable for durable governance state

## Decision

State MUST be persisted to SQLite using the standard library database/sql driver.
All writes MUST use transactions to ensure atomicity.

## Consequences

### Good
- No external dependency
