# Invariant Records

**Invariant Records** (short form `INV`) are edikt's formal artifact type for documenting hard architectural constraints — rules that must hold continuously, independent of any single decision. They're the enforcement counterpart to Architecture Decision Records (ADRs).

edikt formalizes architectural invariants as a governance artifact with a committed template, compile pipeline, and enforcement integration. The template contract is defined in [ADR-009](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-009-invariant-record-terminology.md).

## Why formalize invariants?

ADRs have been formalized since [Michael Nygard's 2011 post](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions). The concept of architectural *invariants* has existed in computer science since at least [Hoare logic](https://en.wikipedia.org/wiki/Hoare_logic) in the 1960s — conditions that must hold before and after every operation. But the *documentation format* for architectural invariants was never standardized. Every team that documents invariants invents their own format.

edikt formalizes them: a committed template contract, a committed lifecycle, and compile pipeline integration that turns invariants into directives Claude follows automatically.

## How Invariant Records differ from ADRs

| Aspect | ADR | Invariant Record |
|---|---|---|
| **Artifact type** | Historical record of a decision | Living rule that must remain true |
| **Written when** | A decision is made (one-time) | A constraint needs to be enforced |
| **Alternatives considered** | Yes, central to the format | No — invariants don't have alternatives |
| **Level of abstraction** | Can be implementation-specific | Must be constraint-level, implementation-agnostic |
| **Typical source** | A team discussion or design review | Regulation, incident, foundational principle, cross-cutting architectural concern |
| **Cross-cutting** | Usually not (narrow decision) | Yes — applies to many code paths |
| **Revision** | Immutable once accepted. Supersede via new ADR. | Content immutable. Status can change (Proposed → Active → Superseded/Retired). |
| **Relationship** | ADRs document *why* a decision was made. | Invariant Records document *what must remain true* as a consequence. |

Most invariants are NOT derived from ADRs. They're cross-cutting architectural principles, regulatory constraints, or foundational product rules that exist independent of any specific decision.

## The template

The Invariant Record template is defined in [ADR-009](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-009-invariant-record-terminology.md) and draws from the "constraint, not implementation" principle — the Statement describes what must be true, not how to achieve it. The template is customizable via `.edikt/templates/invariant.md` (see [Extensibility](extensibility)).

Every Invariant Record has six body sections (two optional) plus a directives block.

```markdown
# INV-NNN: Short declarative title

**Date:** YYYY-MM-DD
**Status:** Active

## Statement

<One declarative sentence, present tense, stating the constraint.
No qualifications, no hedging.>

## Rationale

<Why this constraint exists. Regulatory requirement, lesson from
an incident, foundational architectural principle. Implementation-
agnostic.>

## Consequences of violation

<What specifically goes wrong when this is broken? Be concrete.>

## Implementation (optional but strongly encouraged)

<Concrete patterns that satisfy this invariant in the current stack.>

## Anti-patterns (optional but strongly encouraged)

<Concrete examples of violations and why they're wrong.>

## Enforcement

<At least one mechanism for catching violations. An invariant without
enforcement is a wish.>

[edikt:directives:start]: #
[edikt:directives:end]: #
```

**Four lifecycle states:**

- **Active** — currently enforced (the normal state)
- **Proposed** — under team discussion, not yet enforced
- **Superseded by INV-NNN** — replaced by a newer invariant
- **Retired (reason)** — no longer relevant, not replaced

## The constraint-not-implementation principle

The single most important rule for writing Invariant Records: **describe the constraint, not the implementation.**

**Test:** *"If our tech stack changed tomorrow, would this rule still apply?"* If yes, you're at the right level. If no, abstract up — the implementation belongs in an ADR.

```
❌ "Use UUIDv7 for primary keys"
✅ "Primary key identifiers are time-orderable"
```

UUIDv7 is today's implementation choice. The underlying constraint (time-orderability) is stable across tech changes. When UUIDv8 or a better ID scheme emerges, the invariant is unchanged — only the implementation ADR updates.

## How they compile into governance

Invariant Records contain a directive sentinel block at the bottom, populated by `/edikt:invariant:compile`. The block uses the three-list schema from [ADR-008](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-008-deterministic-compile-and-three-list-schema.md):

- **`directives:`** — auto-generated from the Statement and Enforcement sections. Uses MUST/NEVER language with literal code tokens. Statements with absolute quantifiers ("every", "all", "total") get "No exceptions." appended automatically.
- **`reminders:`** — pre-action interrupts generated from each directive. Format: "Before writing SQL -> MUST include tenant_id." These fire at the moment Claude is about to write a specific kind of code.
- **`verification:`** — grep-verifiable checklist items Claude checks before finishing. Only directives that can be verified mechanically get a checklist item.
- **`manual_directives:`** — user-added rules compile missed
- **`suppressed_directives:`** — auto directives the user rejected

`/edikt:gov:compile` reads all five lists from every Invariant Record and merges them into `.claude/rules/governance.md`. The governance file has four sections Claude reads every session:

1. **Non-Negotiable Constraints** — invariant directives at the top (primacy bias)
2. **Reminders** — pre-action interrupts aggregated from all sources
3. **Verification Checklist** — self-audit items Claude checks before finishing
4. **Reminder: Non-Negotiable Constraints** — invariant directives restated at the bottom (recency bias)

## Why directive language matters

Experiments showed that *how* the directive is phrased changes whether Claude follows it. The compile pipeline produces directives optimized for LLM compliance:

```
Prose (low compliance):
  "Log calls should include the tenant identifier"

Compiled directive (high compliance):
  "Every slog.Error call MUST include "tenant_id", tid. No exceptions. (ref: INV-012)"
```

The difference: literal code tokens (`slog.Error`, `"tenant_id"`, `tid`), MUST/NEVER language, and "No exceptions." reinforcement. Pre-registered experiments on Claude Opus 4.6 confirmed that the compiled format prevents violations the prose format misses — particularly on greenfield code and new domains where there are no existing patterns to copy.

Use `/edikt:gov:score` to measure how well your governance follows these patterns.

## Next steps

- **Read the canonical examples:** [tenant isolation](canonical-invariants/tenant-isolation.md) and [money precision](canonical-invariants/money-precision.md). Two worked examples covering different failure axes (security/isolation vs data correctness).
- **Read the writing guide:** [Writing Invariant Records](writing-invariants.md) — five qualities, seven traps, six bad-to-good rewrites, seven-question self-test, annotated canonical examples.
- **Read the formal contract:** [ADR-009](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-009-invariant-record-terminology.md) for the template contract, and [ADR-008](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-008-deterministic-compile-and-three-list-schema.md) for the directive schema.
- **Create your first one:** `/edikt:invariant:new "your constraint here"` after running `/edikt:init` to set up templates.
