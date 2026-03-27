<!-- Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation. -->
<!-- edikt:artifact type=data-model spec=SPEC-TEST status=draft reviewed_by=dba -->
<!-- created_at=2026-03-27T00:00:00Z -->

# Data Model — Cache Layer

## Key Schema

| Key pattern | Value type | TTL | Purpose | Namespace |
|---|---|---|---|---|
| `session:<user_id>` | JSON object | 3600s | User session | `session` |
| `cache:result:<hash>` | JSON object | 300s | Result cache | `cache` |

## Notes

- Key separator: `:`
- Namespace rationale: sessions isolated from cache for independent eviction
- Eviction policy: LRU for cache, noeviction for session
