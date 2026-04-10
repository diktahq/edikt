# ARCH-001: Strict layered architecture

**Date:** 2026-04-10
**Status:** Active

## Statement

The application follows a strict layered architecture where dependencies point inward. The domain layer has zero external imports. The service layer depends only on domain. The repository layer depends only on domain. HTTP handlers depend on domain and service. No layer bypasses the one below it.

## Rationale

Without architectural enforcement, greenfield Go services accumulate structural debt within the first implementation session. SQL appears in handlers, business logic migrates to HTTP-layer middleware, and domain types acquire infrastructure tags. By the time the second developer reads the code, the intended architecture is already invisible.

## Implementation

- domain/ — pure types and validation. Zero imports from other application packages.
- service/ — business logic. Accepts repository interfaces, never concrete types.
- repository/ — database access. All SQL lives here. Implements interfaces consumed by service.
- handler/ — HTTP transport. Thin: decode, call service, encode. No business logic.
- cmd/ — wiring and bootstrap. May import everything.

## Anti-patterns

- SQL in handlers ("just a quick query")
- Domain types with `json:"..."` HTTP tags or `sql:"..."` DB tags
- Services that import concrete repository types instead of interfaces
- Handlers with business logic (validation beyond request format, conditional branching on domain state)
- Package-level mutable state (`var db *sql.DB`)

## Enforcement

- Import analysis: `domain/` must have zero imports from `service/`, `repository/`, `handler/`
- SQL grep: SQL keywords outside `repository/` fail the check
- Handler line count: functions in `handler/` over 30 lines flag for review

<!-- Directives for edikt governance. Populated by /edikt:invariant:compile. -->
[edikt:directives:start]: #
- HTTP handlers are thin: decode request, call service, encode response. No business logic, no database calls. (ref: ARCH-001)
- The domain package imports NOTHING from any other application package. (ref: ARCH-001)
- ALL database access goes through the repository package. No SQL in handlers, services, or domain. (ref: ARCH-001)
- Services accept repository interfaces, not concrete types. (ref: ARCH-001)
- No func init() for business logic. Use explicit constructors. (ref: AP-001)
- No package-level mutable var. Inject state through struct constructors. (ref: AP-002)
- No SQL string concatenation. Use parameterized queries only. (ref: AP-003)
[edikt:directives:end]: #
