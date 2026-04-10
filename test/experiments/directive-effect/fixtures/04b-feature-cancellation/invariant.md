# INV-012: Tenant isolation is total

**Date:** 2026-04-09
**Status:** Active

## Statement

Every request, database query, log entry, and background job carries an authoritative tenant identifier, and every data access — read or write — is scoped to that tenant. There is no code path in the system where tenant context is optional.

## Rationale

Multi-tenant systems face silent, high-cost failures when tenant isolation breaks. Unlike crashes or exceptions, cross-tenant data leakage is invisible — queries return rows, responses land in browsers, and customers never see an error message. The failure only surfaces when a customer notices their data in someone else's view, a regulator discovers the exposure during an audit, or a forensic investigation of an incident reveals the leak weeks or months after it happened.

The constraint must be **total**. Any phrasing like "scoped by tenant except in the admin panel" or "except for background analytics jobs" creates the exact code path where a future change forgets the exception and leaks data.

## Consequences of violation

- **Cross-tenant data leakage** — silent, often undetected for weeks or months.
- **Regulatory exposure** — GDPR, SOC 2 Type II, HIPAA, and most enterprise compliance frameworks treat cross-tenant data exposure as a reportable breach.
- **Customer trust collapse** — one leakage incident is often sufficient to lose an enterprise customer permanently.

## Implementation

- **Request authentication middleware** extracts the authoritative tenant ID from the signed session/JWT and binds it to the request context. The tenant ID from the request body or query parameters is never trusted.
- **Service layer** reads the authoritative tenant from context at the top of every method and passes it explicitly to every downstream call — repository, audit, events, metrics, cache, logs. The repository layer does not read context; it takes tenant as an explicit parameter.
- **Structured log events** must include `tenant_id` on every log line written in a tenant-scoped flow. The logger does not add it automatically; the caller is responsible.
- **Domain events** published to the event bus must include `TenantID` in the event payload. Consumers of the event bus rely on it.
- **Audit records** must include the tenant the action was performed in.
- **Background jobs** are spawned with an explicit tenant context. On pickup, workers re-establish that context before processing.

## Anti-patterns

- **Tenant ID from the request body, URL parameter, or query string.** The user can send whatever they want. Only the verified session is authoritative.
- **Repository methods that read context internally.** The repository must be dumb — tenant scope is a service-layer responsibility that is passed in explicitly.
- **Log calls that omit `tenant_id`.** "It will be obvious from the request_id" is not good enough; tenant is the primary search dimension.
- **Events published without `TenantID`.** Downstream consumers cannot filter correctly.
- **Background jobs that run "for all tenants" in a single loop without re-establishing scope per tenant.**

## Enforcement

- **Repository unit tests** verify that every method refuses to run with an empty tenant argument.
- **Pre-push hook** greps for raw SQL outside the repository layer.
- **edikt directive** loaded into Claude's context: "Every log line in a tenant-scoped flow must include `tenant_id`. Every event must include `TenantID`. Every repository call must pass tenant explicitly. No exceptions."

<!-- Directives for edikt governance. Populated by /edikt:invariant:compile. -->
[edikt:directives:start]: #
- Repository methods take `tenantID` as an explicit parameter. The repository does not read context. Service-layer callers are responsible for passing the authoritative tenant from the verified session. (ref: INV-012)
- Every structured log call written in a tenant-scoped flow includes a `"tenant_id"` field. The logger does not add it automatically — the caller passes it explicitly on every `log.Info` / `log.Warn` / `log.Error`. (ref: INV-012)
- Every `events.Publish` call sets `TenantID` on the `events.Event` literal. Downstream consumers filter on it; omission silently breaks the pipeline. (ref: INV-012)
- Every `audit.Record` call sets `TenantID` on the `audit.Entry`. (ref: INV-012)
- Tenant identity is read only from the verified session on `ctx`. Never from the request body, URL path parameter, or query string. (ref: INV-012)
- Raw SQL that touches a tenant-scoped table outside `internal/repository/` is forbidden. All database access goes through the repository. (ref: INV-012)
[edikt:directives:end]: #
