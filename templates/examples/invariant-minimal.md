# INV-NNN: {Short declarative title}

**Date:** {YYYY-MM-DD}
**Status:** Active

<!--
Minimal Invariant Record template (see ADR-009 for the template contract).

This is the smallest valid Invariant Record: four required sections
(Statement, Rationale, Consequences of violation, Enforcement) plus
the directives block. Implementation and Anti-patterns sections are
omitted for brevity.

Writing guidance:
1. Describe the CONSTRAINT, not the IMPLEMENTATION.
   Good: "Primary identifiers are time-orderable."
   Bad:  "Use UUIDv7 for primary keys."
   Test: "If our stack changed tomorrow, would this still apply?"

2. Present tense, declarative, no hedging.
   Good: "Every authorization decision is logged."
   Bad:  "We should try to log authorization decisions."

3. Invariant Records are NOT derived from ADRs. They stand alone.
   If you reference an ADR, mention it in Rationale as prose.

4. An invariant without Enforcement is a wish. At least one mechanism
   (automated or manual) must exist and be named.

See invariant-full.md for a template with the Implementation and
Anti-patterns sections included. See the writing guide for the full
list of qualities, traps, and self-test questions.

See ADR-009 for the template contract.
-->

## Statement

{One declarative sentence, present tense, stating the constraint.
No qualifications, no hedging, no "usually", "where possible",
"try to". This is the rule.}

## Rationale

{Why this constraint exists. Regulatory requirement, lesson from
an incident, foundational architectural principle, first-principles
reasoning. Implementation-agnostic — state the underlying reason,
not the specific technology.}

## Consequences of violation

{What specifically goes wrong when this is broken? Data loss,
compliance failure, security hole, silent correctness bug.
Be concrete — readers should leave this section knowing the cost.}

## Enforcement

{How do we catch violations? At least one mechanism must exist:
  - Automated: test, linter, edikt directive, CI check, runtime assertion
  - Manual: code review checklist item

An invariant without enforcement is a wish.}

<!-- Directives for edikt governance. Populated by /edikt:invariant:compile. -->
[edikt:directives:start]: #
[edikt:directives:end]: #
