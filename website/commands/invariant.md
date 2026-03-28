# /edikt:invariant

Capture a hard architectural constraint that must never be violated.

## Usage

```bash
/edikt:invariant no floats for money
/edikt:invariant                         ← extracts from current conversation
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
/edikt:invariant no floats for money
```

edikt creates the invariant with precise language: what the rule is, why it exists, and what violation looks like.

Creates: `docs/invariants/INV-{NNN}-no-floats-for-money.md`

### No argument — extract from conversation

```bash
/edikt:invariant
```

Extracts the last hard constraint discussed in the current conversation.

## Proactive suggestions

The `Stop` hook watches every Claude response for hard constraint signals. When it detects one, Claude suggests:

```text
💡 This is an invariant — run `/edikt:invariant` to capture it.
```

## Output

```text
docs/invariants/
└── INV-001-no-floats-for-money.md
```

Invariants are loaded by `/edikt:context` **at all depth levels** — they're always in Claude's context because they're always non-negotiable.

## Natural language triggers

- "that's a hard rule"
- "never do X"
- "add an invariant"
- "that must never be violated"
