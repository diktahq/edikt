# INV-012: Tenant isolation is total

**Date:** 2026-04-10
**Status:** Active

## Statement

Every request, database query, log entry, and background job carries an authoritative tenant identifier, and every data access is scoped to that tenant. There is no code path where tenant context is optional.

## Rationale

Multi-tenant systems face silent, high-cost failures when tenant isolation breaks. Cross-tenant data leakage is invisible — queries return rows, responses land in browsers, and customers never see an error. The failure surfaces weeks later when a customer sees another tenant's data.

## Implementation

- Request middleware extracts tenant from verified session, binds to context.
- Repository/store methods take tenantID as an explicit parameter.
- Every SQL WHERE clause on tenant-scoped tables includes tenant_id.
- Structured logs include tenant_id on every call (not auto-populated).
- Job poller scopes by tenant — no "all tenants in one pass" queries.
- Background workers re-establish tenant context from the job record.

## Anti-patterns

- Tenant from request body or URL parameter (untrusted source).
- Repository reads tenant from context internally (hides the dependency).
- Log calls without tenant_id ("you can find it from request_id" is not good enough).
- Poller that iterates all tenants in a single unscoped query.
- Worker that processes a job without setting tenant scope first.

<!-- Directives for edikt governance. Populated by /edikt:invariant:compile. -->
[edikt:directives:start]: #
- Every SQL query includes tenant_id in the WHERE clause. No cross-tenant queries. (ref: INV-012)
- Tenant ID from authenticated context only, never from request body or URL. (ref: INV-012)
- Every log call includes "tenant_id" as an explicit field. (ref: INV-012)
- Job poller scopes by tenant. No "poll all tenants in one pass." (ref: INV-012)
- Repository/store methods take tenantID as an explicit parameter. (ref: INV-012)
- Background workers re-establish tenant context from the job record. (ref: INV-012)
[edikt:directives:end]: #
