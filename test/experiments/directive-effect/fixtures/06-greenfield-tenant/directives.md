# Governance Directives

Follow these directives in every file you write or edit.

## Non-Negotiable Constraints

These are invariants. Violation is never acceptable.

- Every new table MUST include a `tenant_id` column. Every SQL query MUST include `tenant_id` in the WHERE clause, every INSERT column list, every JOIN condition. No exceptions. (ref: INV-012)
- Tenant ID MUST be read only from `ctx.Value` on the authenticated request context. NEVER read tenant from the request body, URL path, or query string. (ref: INV-012)
- Every new repository/store method MUST take `tenantID string` as an explicit parameter. NEVER read context inside the repository. (ref: INV-012)
- Every `slog.Info`, `slog.Warn`, `slog.Error` call MUST include `"tenant_id", tid` in its argument list. No exceptions. (ref: INV-012)
- The job poller MUST scope its polling query by `tenant_id`. NEVER poll all tenants in one unscoped pass. (ref: INV-012)
- Background job workers MUST re-establish tenant context from the job record before processing. NEVER run in a "no tenant" state. (ref: INV-012)
