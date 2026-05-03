---
type: adr
id: ADR-099-test
title: Test ADR with considered options and no prohibition coverage
status: accepted
created_at: 2026-05-03T00:00:00Z
---

# ADR-099-test: Test ADR with considered options and no prohibition coverage

**Status:** Accepted

## Context

We need to choose an approach for storing session tokens.

## Considered Options

### A. In-memory store

Store tokens in process memory only.

**Pros:** Fast, zero I/O.
**Cons:** Tokens lost on restart.

### B. Redis

Store tokens in a Redis cluster.

**Pros:** Durable, shared across replicas.
**Cons:** Additional infrastructure dependency.

### C. Database table

Store tokens in the primary relational database.

**Pros:** No new infrastructure.
**Cons:** High-frequency reads add load to the primary.

## Decision

We chose Redis (Option B).

## Confirmation

- Redis cluster must be provisioned before deployment.
