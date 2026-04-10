# INV-012: Tenant isolation is total

**Date:** 2026-04-10
**Status:** Active

## Statement

Every data access is scoped to the authenticated tenant. New tables, queries, log lines, and service methods must carry tenant context explicitly.

## Implementation

- New tables include a `tenant_id` column.
- New repository methods take `tenantID` as an explicit parameter (not from context).
- Every SQL WHERE clause filters by `tenant_id`.
- Structured logs include `"tenant_id"` on every call.
- Handlers never read tenant from the request body — only from the verified session context.

[edikt:directives:start]: #
- Every SQL query includes tenant_id in the WHERE clause. New tables include a tenant_id column. (ref: INV-012)
- Tenant ID from authenticated context only, never from request body or URL. (ref: INV-012)
- Every log call includes "tenant_id" as an explicit field. (ref: INV-012)
- New repository methods take tenantID as an explicit parameter, not from context. (ref: INV-012)
- Handlers are thin: decode, call service, encode. No SQL, no business logic. (ref: ARCH-001)
- Error responses are sanitized. Never return err.Error() to the client. (ref: SEC-001)
[edikt:directives:end]: #
