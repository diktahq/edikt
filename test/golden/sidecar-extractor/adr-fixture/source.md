---
type: adr
id: ADR-FIXTURE-A
title: Use PostgreSQL for transactional persistence
status: accepted
decision-makers: [Test Author]
created_at: 2026-05-02T00:00:00Z
references:
  adrs: []
  invariants: []
  prds: []
  specs: []
---

# ADR-FIXTURE-A: Use PostgreSQL for transactional persistence

**Status:** Accepted

## Context

The product needs a transactional store with ACID guarantees, JSON document support for flexible schemas, and a strong operational ecosystem on the major clouds. We compared PostgreSQL, MySQL, and CockroachDB.

## Decision

All transactional data MUST be stored in PostgreSQL 16 or later. NEVER write transactional data to MongoDB, DynamoDB, or any non-relational store. Schema migrations MUST run via `golang-migrate` with up and down SQL pairs committed to the repository.

## Consequences

- ACID semantics are guaranteed across all transactional writes.
- A single store simplifies backup, restore, and observability.
- Cross-region replication is the team's responsibility — Postgres native replication is the supported path; bolt-on layers are not.
