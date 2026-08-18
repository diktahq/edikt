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

Creates: `docs/architecture/invariants/INV-{NNN}-no-floats-for-money.md`

### No argument — extract from conversation

```bash
/edikt:invariant:new
```

Extracts the last hard constraint discussed in the current conversation.

## Proactive suggestions

The `Stop` hook watches every response for hard constraint signals. When it detects one, it suggests:

```text
💡 This is an invariant — run `/edikt:invariant:new` to capture it.
```

## Template

edikt uses a template to structure the Invariant Record. The template lookup chain:

1. **Project override** — `.edikt/templates/invariant.md` (if present)
2. **edikt default** — built-in template

The default template produces:

```markdown
# INV-NNN: Short declarative title

**Date:** YYYY-MM-DD
**Status:** Active

## Statement       ← the sidecar extractor reads this (absolute, declarative)
## Rationale       ← why the constraint exists
## Consequences of violation
## Implementation  ← concrete patterns (optional)
## Anti-patterns   ← what violation looks like (optional)
## Enforcement     ← at least one mechanism (required)
```

The `## Statement` section is what the extractor reads. Write it with absolute language ("every", "all", "never") — these trigger the "No exceptions." reinforcement in compiled directives. See [Writing good invariants](/governance/writing-invariants) for the full guide. In v0.6.0+ the prose template carries no in-body directives block — the generated directives land in the sibling sidecar shown under [Output](#output-v060).

## Output (v0.6.0)

```text
docs/architecture/invariants/
├── INV-001-no-floats-for-money.md           ← prose. you own it.
└── INV-001-no-floats-for-money.edikt.yaml   ← sidecar. edikt writes it.
```

After creating the prose `.md`, edikt dispatches the `sidecar-extractor` agent in a forked subagent (`context: fork`) with a locked extraction prompt. The agent reads the Statement and Enforcement sections, extracts MUST/NEVER directives, and writes the co-located `<INV>.edikt.yaml`. The pair is created atomically — if extraction fails, neither file remains.

You'll see:

```text
✅ Created INV-001-no-floats-for-money.md
✅ Generated INV-001-no-floats-for-money.edikt.yaml — review it before sharing.
✅ Verify: 2 of 3 passed.
```

Each invariant's sidecar is generated in its own fresh subagent context with the same locked prompt — there is no cross-artifact contamination. See [Sidecar Architecture](/governance/sidecar) for the data model.

### Post-write verify gate

After both files are on disk, `/edikt:invariant:new` shells to:

```bash
bin/edikt verify gov INV-NNN
```

The runner walks every `directives[].verify` and structured `verification[].verify` declared in the new sidecar and runs each as a shell command. Items without a `verify:` field are recorded as `skipped`.

- **Exit 0** → proceed silently. The verify line above is included in the success summary.
- **Exit 1** → surface a warning with the per-item failures. The artifact is **never auto-deleted** on verify failure — the file stays on disk so you can inspect and iterate, either fixing the underlying behavior or editing the sidecar to remove the bad `verify:` line.

See [`edikt verify`](/commands/verify) for the full contract.

## What's next

- [/edikt:invariant:compile](/commands/invariant/compile) — compile into governance directives
- [/edikt:invariant:review](/commands/invariant/review) — review language quality + directive LLM compliance
- [Invariant Records](/governance/invariant-records) — what they are, lifecycle, how they compile
- [Writing good invariants](/governance/writing-invariants) — five qualities, seven traps, LLM compliance
- [Example: tenant isolation](/governance/writing-invariants#example-1-tenant-isolation-is-total) — full worked example
- [Extensibility](/governance/extensibility) — manual directives, suppressed directives, template overrides

## Natural language triggers

- "that's a hard rule"
- "never do X"
- "add an invariant"
- "that must never be violated"
