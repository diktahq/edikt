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

<!-- Below this line: machine-enforced directives. Auto-populated by /edikt:adr:compile. -->

[edikt:directives:start]: #
[edikt:directives:end]: #
