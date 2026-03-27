---
title: Cache Feature - Redis
status: accepted
---

# Cache Feature with Redis

This feature uses Redis as the key-value cache layer.

## Requirements

Cache user sessions and computed results in Redis.

## Data Model

The system stores sessions, leaderboards, and temporary state in Redis with TTL policies.
