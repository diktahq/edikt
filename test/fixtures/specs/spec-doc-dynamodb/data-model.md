<!-- Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation. -->
<!-- edikt:artifact type=data-model spec=SPEC-TEST status=draft reviewed_by=dba -->
<!-- created_at=2026-03-27T00:00:00Z -->

# Data Model — Event Storage

## Access Patterns

| Pattern | PK | SK | Index | Notes |
|---|---|---|---|---|
| Get event by ID | EVENT#<id> | EVENT#<id> | table | Single item lookup |
| Events by user | USER#<user_id> | EVENT#<timestamp> | table | Time-sorted user events |
| Events by type | TYPE#<type> | EVENT#<timestamp> | GSI1 | Filter by event type |

## Entity Prefixes

| Entity | PK prefix | SK prefix |
|---|---|---|
| Event | `EVENT#` | `EVENT#` |
| User events | `USER#` | `EVENT#` |

## Key Design

| Key | Pattern | Example |
|---|---|---|
| PK | `EVENT#<id>` | `EVENT#abc123` |
| SK | `EVENT#<timestamp>` | `EVENT#2026-03-27T00:00:00Z` |
| GSI1-PK | `TYPE#<type>` | `TYPE#page_view` |
| GSI1-SK | `EVENT#<timestamp>` | `EVENT#2026-03-27T00:00:00Z` |
