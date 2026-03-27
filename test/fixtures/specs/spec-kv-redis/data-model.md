<!-- Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation. -->
<!-- edikt:artifact type=data-model spec=SPEC-TEST status=draft reviewed_by=dba -->
<!-- created_at=2026-03-27T00:00:00Z -->

# Data Model — Cache Layer

## Key Schema

| Key pattern | Value type | TTL | Purpose | Namespace |
|---|---|---|---|---|
| `session:<user_id>` | JSON object | 3600s | User session data | `session` |
| `cache:<resource>:<id>` | JSON object | 300s | Computed result cache | `cache` |
| `leaderboard:<board_id>` | sorted set | none | Real-time leaderboard | `leaderboard` |
| `lock:<resource>:<id>` | string | 30s | Distributed lock | `lock` |

## Notes

- Key separator: `:`
- Namespace rationale: sessions are isolated from cache to allow independent eviction
- Eviction policy: LRU for cache namespace, noeviction for session namespace
