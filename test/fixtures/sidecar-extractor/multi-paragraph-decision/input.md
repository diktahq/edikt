---
type: adr
id: ADR-099
title: "ADR-099 — API Versioning"
status: Accepted
date: 2026-05-03
deciders: Test Author
---

# ADR-099 — API Versioning

**Status:** Accepted
**Date:** 2026-05-03

## Context

The public API needs a versioning strategy to support backward-compatible evolution.

## Decision

All API routes MUST include a version prefix in the URL path (e.g., `/api/v1/`).

Breaking changes MUST be introduced under a new version prefix; existing versions MUST remain functional for a minimum of six months after deprecation notice.

Each version MUST have its own OpenAPI schema file at `docs/api/v<N>/openapi.yaml`.

Deprecated versions MUST return a `Deprecation` response header with the sunset date on every request.

## Consequences

### Good
- Clients can migrate at their own pace
