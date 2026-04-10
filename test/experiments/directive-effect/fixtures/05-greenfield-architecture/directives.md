# Governance Directives

Follow these directives in every file you write or edit.

## Non-Negotiable Constraints

These are invariants. Violation is never acceptable.

- The `domain/` package MUST NOT import from `service/`, `repository/`, `handler/`, `adapter/`, or `cmd/`. Dependencies point inward only. (ref: ARCH-001)
- ALL database access MUST go through the `repository/` package. NEVER write SQL in handlers, services, or domain. (ref: ARCH-001)
- HTTP handlers MUST be thin: decode request, call service method, encode response. NEVER put business logic or database calls in handlers. (ref: ARCH-001)
- Services MUST accept repository interfaces, NEVER concrete types. (ref: ARCH-001)
- NEVER use `func init()` for business logic — use explicit constructors. (ref: AP-001)
- NEVER use package-level mutable `var` — inject state through struct constructors. (ref: AP-002)
- NEVER concatenate SQL strings — use parameterized queries: `db.QueryContext(ctx, "SELECT ... WHERE id = $1", id)`. (ref: AP-003)

## Architecture

| Layer | Location | May depend on | NEVER depends on |
|-------|----------|---------------|------------------|
| domain | `domain/` | nothing | service, repository, handler, cmd |
| service | `service/` | domain | repository, handler, cmd |
| repository | `repository/` | domain | service, handler, cmd |
| handler | `handler/` | domain, service | repository, cmd |
| cmd | `cmd/` | everything | — |
