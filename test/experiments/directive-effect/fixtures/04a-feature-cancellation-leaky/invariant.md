# INV-012: Tenant isolation is total

**Date:** 2026-04-09
**Status:** Active

## Statement

Every request, database query, log entry, and background job carries an authoritative tenant identifier, and every data access — read or write — is scoped to that tenant. There is no code path in the system where tenant context is optional.

## Rationale

Multi-tenant systems face silent, high-cost failures when tenant isolation breaks. Unlike crashes or exceptions, cross-tenant data leakage is invisible — queries return rows, responses land in browsers, and customers never see an error message. The failure only surfaces when a customer notices their data in someone else's view, a regulator discovers the exposure during an audit, or a forensic investigation of an incident reveals the leak weeks or months after it happened.

The constraint must be **total**. Any phrasing like "scoped by tenant except in the admin panel" or "except for background analytics jobs" creates the exact code path where a future change forgets the exception and leaks data. Exceptions become permanent loopholes. The invariant applies everywhere, without exceptions, because the cost of a single leakage incident (customer trust loss, regulatory exposure, contractual damages) is orders of magnitude higher than the cost of enforcing the constraint pervasively.

## Consequences of violation

- **Cross-tenant data leakage** — silent, often undetected for weeks or months. Once a customer has seen another tenant's data, the exposure cannot be undone.
- **Regulatory exposure** — GDPR, SOC 2 Type II, HIPAA, and most enterprise compliance frameworks treat cross-tenant data exposure as a reportable breach.
- **Customer trust collapse** — one leakage incident is often sufficient to lose an enterprise customer permanently.
- **Investigation overhead** — when a leak is discovered, reconstructing who saw what, when, and how often requires hours or days of forensic work.

## Implementation

- **Request authentication middleware** extracts the authoritative tenant ID from the signed session/JWT and binds it to the request context. The tenant ID from the request body or query parameters is never trusted.
- **Repository layer** is the sole path to the database. Every repository method accepts a tenant ID (or reads it from the request context) and injects `WHERE tenant_id = $tenant` as a non-negotiable filter on every query. Raw SQL that bypasses the repository is forbidden.
- **Structured logger** automatically includes `tenant_id` in every log event by reading it from the request context.
- **Background jobs** are always spawned with an explicit tenant context. On pickup, workers re-establish that context before processing.

## Anti-patterns

- **Raw SQL outside the repository layer.** The repository injects tenant scoping automatically. Raw SQL bypasses this and must write the filter by hand, which is easy to forget.
- **Tenant ID from request body or query parameter.** The user can send whatever they want. Only the signed session is authoritative.
- **Joining tables without scoping both sides.** Every JOIN must filter every participating table.
- **"Global" background jobs** that process multiple tenants in a single pass without re-establishing scope per tenant.

## Enforcement

- **Repository layer is the only database access path.** Raw SQL outside the repository fails the pre-push hook.
- **Repository unit tests** verify that every method rejects an empty tenant context.
- **Route middleware** rejects requests without a valid tenant-bearing session at the edge.
- **edikt directive** loaded into Claude's context: "Every data access must be tenant-scoped. Every log line must include `tenant_id`. No exceptions."

<!-- Directives for edikt governance. Populated by /edikt:invariant:compile. -->
[edikt:directives:start]: #
[edikt:directives:end]: #
