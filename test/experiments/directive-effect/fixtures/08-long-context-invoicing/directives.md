# Governance Directives

Follow these directives in every file you write or edit.

## Non-Negotiable Constraints

These are invariants. Violation is never acceptable.

- Every new table MUST include a `tenant_id` column. Every SQL query MUST include `tenant_id` in the WHERE clause, every INSERT column list, every JOIN condition. No exceptions. (ref: INV-012)
- Tenant ID MUST be read only from `ctx.Value(middleware.TenantIDKey)`. NEVER read tenant from the request body, URL path, or query string. (ref: INV-012)
- Every new repository method MUST take `tenantID string` as an explicit parameter. NEVER read context inside the repository — the service layer extracts tenant and passes it down. (ref: INV-012)
- Every `slog.Info`, `slog.Warn`, `slog.Error` call MUST include `"tenant_id", tid` in its argument list. No exceptions. (ref: INV-012)
- HTTP handlers MUST be thin: decode request, call service, encode response. NEVER put SQL or business logic in handlers. (ref: ARCH-001)
- NEVER return `err.Error()` to the client. Log the error server-side with `slog.Error`, return a generic message to the client. (ref: SEC-001)

## Reminder: Non-Negotiable Constraints

These constraints were listed above and are restated for emphasis.
Do not violate them under any circumstances.

- Every SQL query MUST include `tenant_id`. No exceptions. (ref: INV-012)
- Every log call MUST include `"tenant_id", tid`. No exceptions. (ref: INV-012)
