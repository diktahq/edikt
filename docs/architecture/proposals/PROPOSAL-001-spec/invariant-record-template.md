# Invariant Record template (final, v0.3.0)

This is the authoritative template for Invariant Records as of ADR-009. Use this exact structure when writing a new invariant, when generating a project template via init's Adapt mode (which starts from this structure and adjusts for detected style), or when shipping reference examples in `templates/examples/invariants/`.

Changes to this template require a new ADR superseding ADR-009.

---

## The template

````markdown
# INV-NNN: <Short declarative title>

**Date:** YYYY-MM-DD
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

See docs/architecture/proposals/PROPOSAL-001-spec/writing-invariants-guide.md
for the full writing guide.
-->

## Statement

<One declarative sentence, present tense, stating the constraint. No
qualifications, no hedging, no "usually", "where possible", "try to".
This is the rule.>

## Rationale

<Why this constraint exists. Regulatory requirement, lesson from an
incident, foundational architectural principle, first-principles
reasoning. Implementation-agnostic — state the underlying reason, not
the specific technology being enforced.>

## Consequences of violation

<What specifically goes wrong when this is broken? Data loss?
Compliance failure? Security hole? Silent correctness bug?
Architectural coupling that's hard to reverse? Be concrete — readers
should leave this section knowing the cost of ignoring the rule.>

## Implementation

<OPTIONAL but strongly encouraged when the invariant benefits from
concrete examples.

Describe patterns that satisfy this invariant in the current stack.
If an ADR captures the specific implementation choice, reference it
here as prose (e.g., "Current implementation uses X, see ADR-055 for
the rationale and alternatives considered.").

This section answers "how do I follow this rule in practice?"-->

## Anti-patterns

<OPTIONAL but strongly encouraged when concrete counter-examples
clarify the rule.

List specific patterns that VIOLATE the invariant and explain why.
Especially valuable for LLMs reading the invariant as context —
concrete counter-examples prevent subtle paraphrases of the forbidden
pattern from slipping through.>

## Enforcement

<REQUIRED. How do we catch violations?

At least one mechanism must exist. Acceptable mechanisms:
  - Automated: test, linter, edikt directive loaded into Claude's
    context, CI check, runtime assertion, pre-commit hook
  - Manual: code review checklist item, PR template prompt

An invariant without enforcement is a wish, not a rule. If you
cannot name even a manual enforcement mechanism, the invariant
isn't ready to publish.>

<!-- Directives for edikt governance. Populated by /edikt:invariant:compile. -->
[edikt:directives:start]: #
[edikt:directives:end]: #
````

---

## Required vs optional sections

| Section | Required? | Notes |
|---|---|---|
| Frontmatter (Date, Status) | ✅ Required | Status is one of: `Active`, `Proposed`, `Superseded by INV-NNN`, `Retired (reason)` |
| Writing guidance comment | ✅ Required | Keep the comment verbatim so users see the principles when editing |
| Statement | ✅ Required | One sentence. Present tense. No hedging. |
| Rationale | ✅ Required | Implementation-agnostic. Why this matters. |
| Consequences of violation | ✅ Required | Concrete failure mode. |
| Implementation | ⚪ Optional but strongly encouraged | When the invariant needs examples to be actionable. |
| Anti-patterns | ⚪ Optional but strongly encouraged | When counter-examples clarify the rule. |
| Enforcement | ✅ Required | At least one mechanism named. Manual counts; automated is preferred. |
| Directives block | ✅ Required | Populated by `/edikt:invariant:compile`. Starts empty. |

Six body sections total (Statement, Rationale, Consequences of violation, Implementation, Anti-patterns, Enforcement), of which two are optional but encouraged. Four required sections plus frontmatter plus the directives block.

---

## Status lifecycle

An Invariant Record moves through these states:

```
   (new)           (team review)
     │                  │
     v                  v
  Proposed ───────> Active ─────> Superseded by INV-NNN
                      │                    │
                      │                    │ (replacement becomes authoritative)
                      │
                      └───────> Retired (reason)
                                 (no replacement; need went away)
```

- **Proposed** — under team discussion. `/edikt:gov:compile` skips this state; directives are not enforced yet.
- **Active** — currently enforced. The normal state. `/edikt:gov:compile` reads and applies directives.
- **Superseded by INV-NNN** — replaced by a newer invariant. The replacement is authoritative. `/edikt:gov:compile` skips this state; the Superseded entry remains as historical record.
- **Retired (reason)** — no longer relevant, not replaced. Requirements changed, system was rewritten, etc. `/edikt:gov:compile` skips this state.

Status line carries metadata inline. No separate frontmatter fields for the replacement link or retirement reason — it's all in the `Status:` line.

**Examples:**

```markdown
**Status:** Active
```

```markdown
**Status:** Proposed
```

```markdown
**Status:** Superseded by INV-027
```

```markdown
**Status:** Retired (2026-03-15 — multi-tenancy requirement removed when platform shifted to single-tenant SaaS)
```

---

## Interaction with `/edikt:invariant:compile`

When `/edikt:invariant:compile INV-NNN` runs, it:

1. Reads the artifact body (everything above the directives block)
2. Generates auto directives from Statement + Rationale + Consequences of violation
3. Writes them to the `directives:` list inside the sentinel block
4. Computes and writes `source_hash`, `directives_hash`, `compiler_version`
5. Does NOT touch `manual_directives:` or `suppressed_directives:`

The user can then add rules Claude missed to `manual_directives:` or suppress wrong auto-generated rules via `suppressed_directives:`. All user edits are preserved across re-compiles. See ADR-008 for the complete compile contract.

---

## Relationship to ADRs — key distinction

**ADRs document decisions. Invariant Records document constraints.**

| Aspect | ADR | Invariant Record |
|---|---|---|
| Artifact type | Historical record | Living rule |
| Written once | Yes (decision is made once) | Yes (constraint is stated once) |
| Revisable? | No — once accepted, immutable. Supersede via new ADR. | Content is immutable. Status can change (Proposed → Active → Superseded/Retired). |
| Alternatives considered? | Yes, central to the format | No — invariants don't have alternatives, they have enforcement |
| Level of abstraction | Can be implementation-specific | Must be constraint-level, implementation-agnostic |
| Typical source | A specific team discussion or design review | Regulatory requirement, incident lesson, foundational principle, cross-cutting architectural concern |
| Cross-cutting? | Usually not (narrow decision) | Yes — applies to many code paths and files |
| Directives block? | Yes (v0.2.0+, ADR-007 schema) | Yes (same schema, per ADR-008) |

**Most invariants are NOT derived from ADRs.** When they are, the reference is prose in the Rationale or Implementation section, not a structured field.

---

## When to write an Invariant Record vs an ADR

Quick decision tree:

- Are you documenting **a choice made** (with alternatives, tradeoffs)? → **ADR**
- Are you documenting **a rule that must hold** (regardless of the specific implementation)? → **Invariant Record**
- If both: the ADR captures the decision, a separate Invariant Record (if needed) captures the constraint the decision established. They reference each other but stand independently.

Example:

- **ADR-055**: "Use Redis as the session store" (context, alternatives, decision, consequences — historical record)
- **INV-022**: "Session data has sub-10ms median access latency" (the constraint the Redis decision satisfies — lives independently, could be satisfied by a different technology in the future)

The ADR can be superseded when you switch away from Redis. The invariant stays — it's the underlying constraint, not the implementation choice.

---

## Edge cases

### What if I have an invariant with no enforceable mechanism?

You don't have an invariant. You have a value, a preference, or a hope. Either:

1. Find a way to enforce it — even code review checklist counts
2. Downgrade it to a guideline (soft recommendation)
3. Don't document it as an invariant until enforcement exists

### What if the invariant is broken temporarily (e.g., legacy code exception)?

Create a new invariant that includes the exception in its Statement, or add the exception explicitly to the Implementation section. Do not hedge the original invariant with "except in legacy module X" — that creates a permanent loophole.

Better: a separate Invariant Record for the legacy area with a Retire date when the migration completes.

### What if two invariants conflict?

`/edikt:gov:compile` detects contradictions during compile and warns. Resolve by:

1. Reviewing both for wording precision (often the conflict is phrasing, not substance)
2. If real conflict: supersede one with a new invariant that reconciles them
3. If one is genuinely wrong: Retire it and keep the other

Contradictory invariants are a real failure mode — the contradiction detection is a guardrail.

### What if I want to propose an invariant but the team hasn't agreed?

Write it with `Status: Proposed`. It lives in the invariants directory, it's visible to the team, but `/edikt:gov:compile` skips it. When the team agrees, flip the status to Active. When the team rejects, Retire it with the reason in the status line.

---

## See also

- [ADR-008](../../decisions/ADR-008-deterministic-compile-and-three-list-schema.md) — The three-list schema and hash caching contract
- [ADR-009](../../decisions/ADR-009-invariant-record-terminology.md) — The coinage of the term
- [`writing-invariants-guide.md`](writing-invariants-guide.md) — Full writing guide with qualities, traps, rewrites, self-test
- [`canonical-examples/tenant-isolation.md`](canonical-examples/tenant-isolation.md) — Worked example 1
- [`canonical-examples/money-precision.md`](canonical-examples/money-precision.md) — Worked example 2
