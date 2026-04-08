# ADR-001 — Use snake_case for all filenames

**Status:** Accepted
**Date:** 2026-01-15

## Context

The project has inconsistent filename conventions. Some files use camelCase, others use kebab-case.

## Decision

All filenames in the project must use snake_case. This applies to source files, test files, and configuration files. No exceptions for language conventions.

## Consequences

- Existing files must be renamed during migration
- CI checks will enforce the convention

## Directives

[edikt:directives:start]: #
paths:
  - "**/*"
scope:
  - implementation
directives:
  - All filenames MUST use snake_case. This applies to source files, test files, and configuration files — no exceptions for language conventions. (ref: ADR-001)
[edikt:directives:end]: #
