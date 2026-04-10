# INV-012: Tenant isolation is total

**Date:** 2026-04-09
**Status:** Active

<!--
Writing guidance (edikt convention — see ADR-009):

1. Describe the CONSTRAINT, not the IMPLEMENTATION.
2. Present tense, declarative, no hedging.
3. Invariants are NOT derived from ADRs. They stand alone.
4. An invariant without Enforcement is a wish.
-->

## Statement

Every request, database query, log entry, and background job carries an authoritative tenant identifier, and every data access is scoped to that tenant. There is no code path in the system where tenant context is optional.

## Rationale

Multi-tenant systems face silent, high-cost failures when tenant isolation breaks. Cross-tenant data leakage is invisible — queries return rows, responses land in browsers, and customers never see an error message. The failure only surfaces when a customer notices their data in someone else's view or when a forensic investigation reveals the leak.

The constraint must be total. Exceptions become permanent loopholes.

## Consequences of violation

- Cross-tenant data leakage (silent, often undetected for weeks)
- Regulatory exposure (GDPR, SOC 2, HIPAA reportable breach)
- Customer trust collapse from a single incident
- Irreversible: once seen, data cannot be un-seen

## Enforcement

- Linter rule blocking raw SQL outside the repository layer
- Repository method tests verifying tenant scope is required
- Route middleware rejecting requests without tenant-bearing sessions
- Log schema validation enforcing `tenant_id` on every log event
- edikt directive loaded into Claude's context

[edikt:directives:start]: #
source_hash: fd8a2e1c4b79d3f5a6b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f01
directives_hash: 3b5c1a9e8f7d6b2a4c0e8f6d4b2a1c3e5f7d9b1a0c2e4f6d8b0a1c3e5f7d9b1a
compiler_version: "0.3.0"
paths:
  - "**/handlers/**"
  - "**/repository/**"
  - "**/logger/**"
  - "**/jobs/**"
scope:
  - implementation
  - review
directives:
  - "Every HTTP handler modifying or reading data must extract tenant ID from the request context and pass it to the repository layer (ref: INV-012)"
  - "Raw SQL outside the repository layer is forbidden — the repository injects tenant_id filtering automatically (ref: INV-012)"
  - "Every structured log event must include tenant_id from request context (ref: INV-012)"
  - "Background jobs must re-establish tenant context on pickup; never rely on process-level tenant state (ref: INV-012)"
  - "JOINs must filter tenant_id on every participating table, not just one (ref: INV-012)"
manual_directives:
  - "When implementing new admin features, explicit cross-tenant access flows require security review approval (ref: INV-012)"
suppressed_directives: []
[edikt:directives:end]: #

<!--
This fixture is the exact form /edikt:invariant:compile should produce
from the body above. Note:
  - Six body sections (Statement, Rationale, Consequences of violation,
    plus Implementation and Anti-patterns optional — omitted here to
    show a minimal but valid invariant)
  - Three directive lists populated
  - All three hash fields present
  - compile_schema_version matches the current constant
-->
