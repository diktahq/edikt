# INV-NNN: {Short declarative title}

**Date:** {YYYY-MM-DD}
**Status:** Active

<!--
Full Invariant Record template (edikt convention — see ADR-009).

This is the complete Invariant Record template with all six body
sections: Statement, Rationale, Consequences of violation,
Implementation, Anti-patterns, Enforcement. Use this when the
invariant benefits from concrete examples and counter-examples.

Writing guidance:
1. Describe the CONSTRAINT, not the IMPLEMENTATION.
   Good: "Primary identifiers are time-orderable."
   Bad:  "Use UUIDv7 for primary keys."
   Test: "If our stack changed tomorrow, would this still apply?"

2. Present tense, declarative, no hedging.

3. Invariant Records are NOT derived from ADRs. They stand alone.
   If you reference an ADR, mention it in Implementation as prose.

4. An invariant without Enforcement is a wish.

See invariant-minimal.md for a shorter template without
Implementation and Anti-patterns. See the writing guide for the full
list of qualities, traps, and self-test questions.

"Invariant Record" is an edikt convention, not an external standard.
-->

## Statement

{One declarative sentence, present tense, stating the constraint.}

## Rationale

{Why this constraint exists. Regulatory requirement, lesson from
an incident, foundational architectural principle, first-principles
reasoning. Implementation-agnostic.}

## Consequences of violation

{What specifically goes wrong when this is broken? Be concrete —
name the failure mode and its cost.}

## Implementation

{Concrete patterns that satisfy this invariant in the current stack.
If an ADR captures the specific implementation choice, reference it
here as prose: "Current implementation uses X, see ADR-055 for
rationale and alternatives considered."

This section answers "how do I follow this rule in practice?".}

## Anti-patterns

{Concrete examples of patterns that VIOLATE the invariant and why.
Especially valuable for LLMs reading the invariant as context —
concrete counter-examples prevent subtle paraphrases of the forbidden
pattern from slipping through.

List 3-5 specific traps:
- {Anti-pattern 1 with explanation}
- {Anti-pattern 2 with explanation}
- {Anti-pattern 3 with explanation}}

## Enforcement

{How do we catch violations? At least one mechanism must exist.
Acceptable mechanisms:
  - Automated: test, linter, edikt directive, CI check, runtime assertion
  - Manual: code review checklist item, PR template prompt

List them all — defense in depth is stronger than a single check:
  - {Mechanism 1}
  - {Mechanism 2}
  - {Mechanism 3}}

<!-- Directives for edikt governance. Populated by /edikt:invariant:compile. -->
[edikt:directives:start]: #
[edikt:directives:end]: #
