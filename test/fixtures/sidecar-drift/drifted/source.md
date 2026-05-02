---
type: adr
id: ADR-FIXTURE-D
title: Drifted fixture for sidecar cross-check
status: accepted
created_at: 2026-05-02T00:00:00Z
---

# ADR-FIXTURE-D: Drifted fixture for sidecar cross-check

**Status:** Accepted

## Decision

All Redis clients MUST use a connection pool with `max_idle` set to 50. Cache writes MUST be idempotent — NEVER perform unconditional INCR on user-input keys. Authentication checks SHOULD use a dedicated cache namespace.
