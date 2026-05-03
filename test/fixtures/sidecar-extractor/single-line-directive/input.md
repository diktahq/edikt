---
type: adr
id: ADR-099
title: "ADR-099 — Error Response Format"
status: Accepted
date: 2026-05-03
deciders: Test Author
---

# ADR-099 — Error Response Format

**Status:** Accepted
**Date:** 2026-05-03

## Context

API error responses need a consistent structure.

## Decision

All API error responses MUST use the `{ "error": { "code": string, "message": string } }` envelope.

## Consequences

### Good
- Clients can handle errors uniformly across all endpoints
