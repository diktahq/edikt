---
type: adr
id: ADR-099
title: "ADR-099 — Configuration Loading"
status: Accepted
date: 2026-05-03
deciders: Test Author
---

# ADR-099 — Configuration Loading

**Status:** Accepted
**Date:** 2026-05-03

## Context

The system needs a deterministic configuration loading strategy.

## Decision

Configuration MUST be loaded from `.edikt/config.yaml` at startup.
The loader MUST validate all values against an allowlist regex before use.

Alternatively: environment variables MAY be used when a config file is absent.

## Consequences

### Good
- Reproducible behavior across environments
