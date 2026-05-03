---
type: adr
id: ADR-099
title: "ADR-099 — React Component File Organization"
status: Accepted
date: 2026-05-03
deciders: Test Author
---

# ADR-099 — React Component File Organization

**Status:** Accepted
**Date:** 2026-05-03

## Context

The frontend codebase has grown and needs consistent component file conventions.

## Decision

All React components MUST be colocated with their test files under `src/components/**/*.{ts,tsx}`.
Component index files MUST use barrel exports following the `src/components/**/index.{ts,tsx}` pattern.

## Consequences

### Good
- Clear ownership of each component's test coverage
