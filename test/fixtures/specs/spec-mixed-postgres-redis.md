---
title: Mixed Feature - Postgres and Redis
status: accepted
---

# Mixed Feature with Postgres and Redis

This feature uses Postgres for persistent data and Redis for caching.

## Requirements

Store user data in Postgres and cache results in Redis for performance.

## Data Model

The system stores primary data in Postgres with relational constraints, and uses Redis for session caching and leaderboard storage.
