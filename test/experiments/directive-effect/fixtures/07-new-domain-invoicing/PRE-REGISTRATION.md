# Experiment 07: New domain (invoicing) on existing checkout service

**Type:** Feature-addition — new domain built from scratch within an existing service.
**Pre-registered:** 2026-04-10
**N per condition:** 1
**Status:** committed before any run

## Why this experiment exists

Experiments 05-06 showed governance effect on GREENFIELD builds. But real
projects aren't blank slates. The realistic scenario: an existing codebase
with its own conventions, and Claude must build an entirely new feature
domain (not just add a method to existing code).

The existing checkout service has "imperfect" conventions:
- Repository reads tenant from ctx.Value (not explicit params)
- Some log calls include tenant_id, some don't
- Checkout handler returns err.Error() to the client

Claude must build the FULL invoicing stack: domain types, database table,
repository with SQL, service logic (line items, tax), HTTP handlers, routes.
The invariant specifies stricter discipline than the existing code shows.

## Hypothesis

**H1:** Without governance, Claude copies the existing conventions
(ctx-based tenant, inconsistent logging, err.Error() leak) into the
new invoicing code. At least one dimension fails.

**H0:** Claude improves on the existing conventions even without
governance — builds clean invoicing code on its own.

## Asserted dimensions

1. **SQL scoping** — invoice queries reference tenant_id
2. **Repo params** — new invoice repo methods take tenantID explicitly
   (not from context — stricter than the existing cart/order repos)
3. **Log tenant** — log calls in invoice code include tenant_id
