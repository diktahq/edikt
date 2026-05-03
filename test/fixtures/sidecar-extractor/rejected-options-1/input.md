---
type: adr
id: ADR-099
title: "ADR-099 — Event Serialization Format"
status: Accepted
date: 2026-05-03
deciders: Test Author
---

# ADR-099 — Event Serialization Format

**Status:** Accepted
**Date:** 2026-05-03

## Context

We need a serialization format for internal events.

## Considered Options

### A. JSON (chosen)
- Pros: human-readable, broad tooling support, easy to debug
- Cons: verbose for high-throughput scenarios

## Decision

Events MUST be serialized as JSON.
All event payloads MUST include a `schema_version` field.

## Consequences

### Good
- Easy to debug with standard tools
