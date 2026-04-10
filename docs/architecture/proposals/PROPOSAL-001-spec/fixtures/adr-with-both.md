# ADR-042: Use Redis for session cache

**Date:** 2026-04-09
**Status:** Accepted

## Context

Our session store has been postgres, which is working but adds ~8ms of median latency to every authenticated request and doesn't scale well beyond 50k concurrent sessions. We need a lower-latency option for the session cache.

## Decision

Use Redis 7 as the primary session store. Postgres becomes the source of truth only for long-lived refresh tokens.

## Consequences

Positive: sub-millisecond session lookups, horizontal scaling via Redis Cluster.
Negative: one more stateful service to operate, potential for session loss on Redis restart (mitigated by AOF persistence).

[edikt:directives:start]: #
source_hash: d7f3a4e1c9b82e5f0a1b3c4d5e6f7890a1b2c3d4e5f6789012345678901234567
directives_hash: 8e2a3b4c5d6e7f8091a2b3c4d5e6f7081a2b3c4d5e6f7081a2b3c4d5e6f7081a
compiler_version: "0.3.0"
paths:
  - "**/*.go"
  - "**/session/**"
scope:
  - implementation
directives:
  - "Store session data in Redis, not Postgres (ref: ADR-042)"
  - "Long-lived refresh tokens go in Postgres; session data does not (ref: ADR-042)"
  - "Enable AOF persistence on the Redis session store (ref: ADR-042)"
  - "Use Redis Cluster for horizontal scaling"
manual_directives:
  - "In integration tests, use a real Redis container (not a mock) to catch serialization bugs (ref: ADR-042)"
  - "Session TTL is 24 hours — do not reduce without consulting the security team (ref: ADR-042)"
suppressed_directives:
  - "Use Redis Cluster for horizontal scaling"
[edikt:directives:end]: #

<!--
Gov:compile effective rules for this artifact:
  (directives - suppressed_directives) ∪ manual_directives =
    - "Store session data in Redis, not Postgres (ref: ADR-042)"
    - "Long-lived refresh tokens go in Postgres; session data does not (ref: ADR-042)"
    - "Enable AOF persistence on the Redis session store (ref: ADR-042)"
    - "In integration tests, use a real Redis container (not a mock) to catch serialization bugs (ref: ADR-042)"
    - "Session TTL is 24 hours — do not reduce without consulting the security team (ref: ADR-042)"

  The suppressed "Use Redis Cluster for horizontal scaling" is filtered out.
  The three auto directives survive the filter and union with the two manual directives.
-->
