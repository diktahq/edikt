# /edikt:invariant:new

Capture a hard architectural constraint that must never be violated.

## Usage

```bash
/edikt:invariant:new no floats for money
/edikt:invariant:new                         ← extracts from current conversation
```

## What is an invariant?

An invariant is not a guideline — it's a rule where violation causes real harm: data corruption, security breaches, double-charges, domain boundary violations. Invariants are non-negotiables.

**Good invariants:**
- "All monetary amounts stored as integer cents. Never use float64 for money."
- "Domain package imports only stdlib. No HTTP, no SQL, no framework types."
- "All payment operations require an idempotency key."

**Not invariants** (use `.claude/rules/` instead): preferences, "try to" statements, style guidelines, things that are just good practice.

## Two modes

### With argument — define from scratch

```bash
/edikt:invariant:new no floats for money
```

edikt creates the invariant with precise language: what the rule is, why it exists, and what violation looks like.

Creates: `docs/invariants/INV-{NNN}-no-floats-for-money.md`

### No argument — extract from conversation

```bash
/edikt:invariant:new
```

Extracts the last hard constraint discussed in the current conversation.

## Proactive suggestions

The `Stop` hook watches every Claude response for hard constraint signals. When it detects one, Claude suggests:

```text
💡 This is an invariant — run `/edikt:invariant:new` to capture it.
```

## Template

edikt uses a template to structure the Invariant Record. The template lookup chain:

1. **Project override** — `.edikt/templates/invariant.md` (if present)
2. **edikt default** — built-in template per [ADR-009](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-009-invariant-record-terminology.md)

The default template produces:

```markdown
# INV-NNN: Short declarative title

**Date:** YYYY-MM-DD
**Status:** Active

## Statement       ← compile reads this (absolute, declarative)
## Rationale       ← why the constraint exists
## Consequences of violation
## Implementation  ← concrete patterns (optional)
## Anti-patterns   ← what violation looks like (optional)
## Enforcement     ← at least one mechanism (required)

[edikt:directives:start]: #
[edikt:directives:end]: #
```

The `## Statement` section is what compile reads. Write it with absolute language ("every", "all", "never") — these trigger the "No exceptions." reinforcement in compiled directives. See [Writing good invariants](/governance/writing-invariants) for the full guide.

## Output

```text
docs/invariants/
└── INV-001-no-floats-for-money.md
```

After creating the invariant, edikt automatically runs `/edikt:invariant:compile` to generate the directive sentinel block with directives, reminders, and verification checklist items. Your new invariant is immediately ready for `/edikt:gov:compile`.

## What's next

- [/edikt:invariant:compile](/commands/invariant/compile) — compile into governance directives
- [/edikt:invariant:review](/commands/invariant/review) — review language quality + directive LLM compliance
- [Invariant Records](/governance/invariant-records) — what they are, lifecycle, how they compile
- [Writing good invariants](/governance/writing-invariants) — five qualities, seven traps, LLM compliance
- [Example: tenant isolation](/governance/canonical-invariants/tenant-isolation) — full worked example
- [Extensibility](/governance/extensibility) — manual directives, suppressed directives, template overrides

## Natural language triggers

- "that's a hard rule"
- "never do X"
- "add an invariant"
- "that must never be violated"
