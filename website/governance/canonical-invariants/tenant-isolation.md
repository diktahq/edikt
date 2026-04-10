# INV-012: Tenant isolation is total

**Date:** 2026-04-09
**Status:** Active

<!--
Writing guidance (edikt convention — see ADR-009):

1. Describe the CONSTRAINT, not the IMPLEMENTATION.
   Good: "Primary identifiers are time-orderable."
   Bad:  "Use UUIDv7 for primary keys."
   Test: "If our stack changed tomorrow, would this rule still apply?"
   If yes — you're at the right level. If no — abstract up.

2. Present tense, declarative, no hedging.
   Good: "Every authorization decision is logged."
   Bad:  "We should try to log authorization decisions."

3. Invariants are NOT derived from ADRs. They stand alone. If your
   invariant references an ADR, mention it in Rationale or Implementation
   as prose — not as a structured frontmatter field.

4. An invariant without Enforcement is a wish. At least one mechanism
   (automated or manual) must exist and be named below.
-->

## Statement

Every request, database query, log entry, and background job carries an authoritative tenant identifier, and every data access — read or write — is scoped to that tenant. There is no code path in the system where tenant context is optional.

## Rationale

Multi-tenant systems face silent, high-cost failures when tenant isolation breaks. Unlike crashes or exceptions, cross-tenant data leakage is invisible — queries return rows, responses land in browsers, and customers never see an error message. The failure only surfaces when a customer notices their data in someone else's view, a regulator discovers the exposure during an audit, or a forensic investigation of an incident reveals the leak weeks or months after it happened.

The constraint must be **total**. Any phrasing like "scoped by tenant except in the admin panel" or "except for background analytics jobs" creates the exact code path where a future change forgets the exception and leaks data. Exceptions become permanent loopholes. The invariant applies everywhere, without exceptions, because the cost of a single leakage incident (customer trust loss, regulatory exposure, contractual damages) is orders of magnitude higher than the cost of enforcing the constraint pervasively.

## Consequences of violation

- **Cross-tenant data leakage** — silent, often undetected for weeks or months. Once a customer has seen another tenant's data, the exposure cannot be undone.
- **Regulatory exposure** — GDPR, SOC 2 Type II, HIPAA, and most enterprise compliance frameworks treat cross-tenant data exposure as a reportable breach. A single incident can trigger notification requirements, fines, and audit findings.
- **Customer trust collapse** — one leakage incident is often sufficient to lose an enterprise customer permanently. Enterprise buyers cannot use a system where tenant isolation is "usually" enforced.
- **Investigation overhead** — when a leak is discovered, reconstructing who saw what, when, and how often requires hours or days of forensic work across logs and database history.

## Implementation

- **Request authentication middleware** extracts the authoritative tenant ID from the signed session/JWT and binds it to the request context. The tenant ID from the request body or query parameters is never trusted.
- **Repository layer** is the sole path to the database. Every repository method accepts a tenant ID (or reads it from the request context) and injects `WHERE tenant_id = $tenant` as a non-negotiable filter on every query. Raw SQL that bypasses the repository is forbidden.
- **Structured logger** automatically includes `tenant_id` in every log event by reading it from the request context. Loggers without the tenant ID cannot be instantiated.
- **Background jobs** are always spawned with an explicit tenant context. On pickup, workers re-establish that context before processing. There are no "global" background jobs that iterate across all tenants in a single pass without re-scoping between each.
- **Tests** have a dedicated test tenant per fixture, never share tenant IDs across tests, and verify tenant scoping is respected in every database access path.

## Anti-patterns

- **Raw SQL outside the repository layer.** The repository injects tenant scoping automatically. Raw SQL bypasses this and must write the filter by hand, which is easy to forget.
- **Tenant ID from request body or query parameter.** The user can send whatever they want. Only the signed session is authoritative.
- **Joining tables without scoping both sides.** A tenant-scoped `users` table JOINed against an unscoped `audit_log` table can leak audit entries across tenants through the join. Every JOIN must filter every participating table.
- **"Global" background jobs** that process multiple tenants in a single pass without re-establishing scope per tenant.
- **Logging events "not attached to a tenant"** because they're "system events, not user events". Every event is a tenant event until proven otherwise; mark truly global events explicitly.
- **Admin interfaces that assume the admin has god-mode access.** Admin users still have a tenant scope (the admin org); cross-tenant access happens only through explicit impersonation flows, not by bypassing the filter.

## Enforcement

- **Linter / grep rule**: any raw SQL outside the repository layer fails the pre-push hook. Implemented as a simple grep for SQL keywords in source files not in the `repository/` directory.
- **Repository layer unit tests**: every repository method has a test that verifies it rejects a query constructed without a tenant filter. The test fixture explicitly passes an empty tenant ID and expects an error.
- **Route middleware**: requests without a valid tenant-bearing session are rejected at the edge, before reaching any handler. Missing tenant context is a 401, not a silent default.
- **Log schema validation**: a CI check ensures every structured log event includes `tenant_id`. Log events without the field fail the build.
- **edikt directive** loaded into Claude's context: "Every data access must be tenant-scoped. Every log line must include `tenant_id`. No exceptions. If you think you've found an exception, you haven't — ask before writing it."
- **Code review checklist**: any PR touching request handling, database access, logging, or background jobs requires explicit reviewer acknowledgment of tenant scoping. Implemented as a PR template checkbox.

Five enforcement mechanisms. Defense in depth. A single mistake in any one layer is caught by another.

<!-- Directives for edikt governance. Populated by /edikt:invariant:compile. -->
[edikt:directives:start]: #
[edikt:directives:end]: #
