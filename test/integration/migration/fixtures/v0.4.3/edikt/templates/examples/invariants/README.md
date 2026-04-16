# Canonical Invariant Record examples

Two worked examples of Invariant Records in the ADR-009 format. These demonstrate what good invariants look like in real projects — what to write, how to structure the sections, and how enforcement mechanisms stack up.

- [`tenant-isolation.md`](tenant-isolation.md) — multi-tenant data scoping (INV-012)
- [`money-precision.md`](money-precision.md) — fixed-point monetary values (INV-008)

Both examples follow edikt's Invariant Record template. They exist to ground the abstract template in concrete invariants you can read, copy, adapt, or use as a learning reference.

## When to read these

- **Learning what good invariants look like**: read both front-to-back. The structure is the same (Statement, Rationale, Consequences of violation, Implementation, Anti-patterns, Enforcement) but the content shows how the same shape applies to very different constraints.
- **Writing your own invariant**: copy one of these as a starting point, then adapt. Most invariants fit the 6-section shape; start there and only deviate when you have a reason.
- **Understanding the "constraint, not implementation" principle**: both examples demonstrate this. Tenant isolation is a constraint ("every access is scoped") that could be implemented with many different auth libraries. Money precision is a constraint ("never use floating-point") that has language-specific implementations but the same underlying rule.

## For the full writing guidance

See [`WRITING-GUIDE.md`](WRITING-GUIDE.md) in this directory for a condensed version of the writing guide — the five qualities of a good invariant, the seven traps to avoid, and the seven-question self-test to run before committing.

The full writing guide with annotated examples lives on edikt.dev under `/governance/writing-invariants` (website) and in the proposal spec at `docs/internal/product/prds/PRD-001-spec/writing-invariants-guide.md` (source tree).

## About the Invariant Record format

edikt formalizes architectural invariants as "Invariant Records" (short form `INV`) — a committed template with Statement, Rationale, Enforcement sections and a compile pipeline that turns them into directives. The template parallels ADRs (Michael Nygard, 2011) but is focused on constraints rather than decisions. See [ADR-009](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-009-invariant-record-terminology.md) for the formal template contract.

## Using these as project templates

If you run `/edikt:init` and pick "Start fresh → Use a reference example" for invariants, these files become the starting point for your project's `.edikt/templates/invariant.md`. You can customize from there.

You can also copy them directly as actual Invariant Records for your own project if tenant isolation or money precision apply to your domain. They're not just teaching material — they're production-grade constraints that many projects will want to adopt wholesale.
