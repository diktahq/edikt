---
type: adr
id: ADR-FIXTURE-S
title: Synced fixture for sidecar cross-check
status: accepted
created_at: 2026-05-02T00:00:00Z
---

# ADR-FIXTURE-S: Synced fixture for sidecar cross-check

**Status:** Accepted

## Decision

All Postgres clients MUST use a connection pool with `max_open` set to 20. NEVER open a raw `database/sql.DB` per request — connection pools amortize handshake cost.
