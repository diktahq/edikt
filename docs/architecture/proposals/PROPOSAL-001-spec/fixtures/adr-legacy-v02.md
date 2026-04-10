# ADR-042: Use Redis for session cache

**Date:** 2026-03-15
**Status:** Accepted

## Context

Our session store has been postgres, which is working but adds ~8ms of median latency to every authenticated request and doesn't scale well beyond 50k concurrent sessions. We need a lower-latency option for the session cache.

## Decision

Use Redis 7 as the primary session store. Postgres becomes the source of truth only for long-lived refresh tokens.

## Consequences

Positive: sub-millisecond session lookups, horizontal scaling via Redis Cluster.
Negative: one more stateful service to operate, potential for session loss on Redis restart (mitigated by AOF persistence).

[edikt:directives:start]: #
paths:
  - "**/*.go"
scope:
  - implementation
directives:
  - "Store session data in Redis, not Postgres (ref: ADR-042)"
  - "Long-lived refresh tokens go in Postgres; session data does not (ref: ADR-042)"
[edikt:directives:end]: #

<!--
This fixture represents a v0.2.x-format ADR with a legacy directive block.
It has:
  - No source_hash
  - No directives_hash
  - No compiler_version
  - No manual_directives (implicit empty)
  - No suppressed_directives (implicit empty)

Expected behavior when /edikt:adr:compile (v0.3.0+) runs on this file:
  1. Detect missing hash fields → treat as first-compile for this block.
  2. Run Claude to regenerate directives: from body.
  3. Write the new three-list schema with all required fields.
  4. No user intervention. Silent migration.

/edikt:gov:compile (v0.3.0+) reading this file:
  - Treats manual_directives and suppressed_directives as empty lists.
  - Processes the directives: list normally.
  - No errors, no warnings. Backward compatible.
-->
