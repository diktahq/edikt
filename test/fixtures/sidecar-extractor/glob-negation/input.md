---
type: adr
id: ADR-099
title: "ADR-099 — Go Test File Exclusions"
status: Accepted
date: 2026-05-03
deciders: Test Author
---

# ADR-099 — Go Test File Exclusions

**Status:** Accepted
**Date:** 2026-05-03

## Context

Linters and coverage tools need to exclude generated test helpers from analysis.

## Decision

All production Go source files MUST reside under `internal/**/*.go`.
The linter MUST exclude generated test helpers matching `!**/_test.go` from static analysis passes.
Coverage reports MUST exclude files matching `!**/testdata/**` from threshold calculations.

## Consequences

### Good
- Clean separation between production and generated test code
