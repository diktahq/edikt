# ADR-009: Invariant Record Terminology

**Date:** 2026-04-09
**Status:** Accepted
**Supersedes:** None

## Context

edikt has three compiled governance artifacts: ADRs, invariants, and guidelines. Each has a distinct role:

- **ADRs** document architectural decisions — historical records of a choice made at a point in time, with alternatives considered and consequences accepted.
- **Invariants** document hard constraints that must not be violated — living rules that hold continuously, independent of any single decision.
- **Guidelines** document team conventions and soft recommendations.

"Architecture Decision Record" (ADR) is a well-established term, coined by Michael Nygard in 2011 and widely adopted. It has a recognizable short form (`ADR`), a stable file-naming convention (`ADR-001-title.md`), and cite-ability in both technical writing and tooling.

**Invariants do not have an equivalent term.** The concept exists in multiple domains (formal methods, Design by Contract, Domain-Driven Design), but none of these provide a standardized *documentation format* for invariants as first-class governance artifacts. Teams that document invariants typically invent their own format, with their own short form, their own identifier conventions, and their own template.

This creates three problems:

1. **No cite-ability.** "We use invariants for architectural constraints" is a vague claim. "We use Invariant Records" would be as precise as "we use ADRs".
2. **No consistency across edikt projects.** Without a named artifact type, users invent their own conventions, defeating the point of edikt's opinionated template system.
3. **No parallelism with ADRs.** Users naturally expect a matching artifact type: *"You have ADRs — what's the equivalent for enforcement rules?"* Without an answer, the governance story feels asymmetric.

**Guidelines do not need the same treatment.** "Guideline" is an established English word with its intended meaning. It's understood in context. Coining "Guideline Record" would be vocabulary inflation without a proportional benefit.

## Decision

edikt coins **"Invariant Record"** as the formal name for a governance artifact documenting a hard architectural constraint.

**Short form:** `INV`.

**File naming:** `INV-NNN-short-title.md` (e.g., `INV-012-tenant-isolation-is-total.md`), matching the `ADR-NNN-*.md` pattern.

**Location:** under `{paths.invariants}` in `.edikt/config.yaml` (default: `docs/architecture/invariants/`).

**Authority:** edikt convention. This term is coined by edikt and is not imported from an external standard. Unlike "Architecture Decision Record" (which we inherit from Nygard), "Invariant Record" is edikt's own formalization of a previously-unnamed artifact type. We commit to maintaining the term and its template contract, and we label it transparently as an edikt convention in all documentation.

## Why `INV` and not `IR`

`IR` has too many established meanings in software engineering:

- **Intermediate Representation** — compilers, LLVM, program analysis
- **Information Retrieval** — academic field
- **Incident Response** — security and operations
- **Infrared** — hardware contexts

A reader encountering "IR-042" would have to context-switch to figure out what's meant. `INV` has no such collision: it's recognizable as an abbreviation for "invariant" without ambiguity, and it matches the natural way invariants are already numbered in dogfood usage across many edikt-governed projects (`INV-001`, `INV-002`, etc.).

Three letters, unambiguous, consistent with the existing `ADR-NNN` convention. `INV` wins.

## The contract that comes with the term

Coining "Invariant Record" is a commitment to a specific template and semantics. The full template is documented in [`PROPOSAL-001-spec/invariant-record-template.md`](../../internal/product/prds/PRD-001-spec/invariant-record-template.md), but the minimum contract is:

- **Frontmatter:** Date, Status (Active | Proposed | Superseded by INV-NNN | Retired with reason)
- **Six body sections:**
  - **Statement** — one declarative sentence describing the constraint, present tense, no hedging
  - **Rationale** — why this matters, implementation-agnostic
  - **Consequences of violation** — concrete failure mode if the rule is broken
  - **Implementation** (optional) — concrete patterns that satisfy the constraint
  - **Anti-patterns** (optional) — concrete examples of violations and why they're wrong
  - **Enforcement** — at least one mechanism for catching violations (automated test, linter, edikt directive, review checklist, or runtime assertion)
- **Directives block** — `[edikt:directives:start]: #` / `[edikt:directives:end]: #`, populated by `/edikt:invariant:compile` per ADR-008

An Invariant Record without an Enforcement section is a wish, not an invariant. The template enforces the distinction.

## Relationship to ADRs

Invariant Records are **not derived from ADRs**. In practice, most invariants are cross-cutting architectural principles, regulatory constraints, or foundational product rules — they do not trace back to a specific decision. Occasionally an invariant emerges from an ADR (e.g., an ADR rejects a technology and the corresponding invariant codifies the prohibition), but this is the exception rather than the rule.

When an invariant does reference an ADR, the reference goes in the **Rationale** or **Implementation** section as prose, not as a structured frontmatter field. This keeps the two artifact types cleanly separated:

- **ADRs answer**: "Why did we choose this approach?"
- **Invariant Records answer**: "What must remain true in this system?"

## Level of abstraction — constraint, not implementation

An Invariant Record describes a **constraint**, not an **implementation**. The test is: *"If our tech stack changed tomorrow, would this rule still apply?"* If yes, the invariant is at the right level. If no, it's an implementation detail that belongs in an ADR.

**Example of an implementation-level (weak) invariant:**

> "Always use UUIDv7 for primary keys."

When UUIDv8 emerges and becomes preferred, the invariant is wrong. The rule isn't stable.

**Example of a constraint-level (good) invariant:**

> "Primary key identifiers are time-orderable."

UUIDv7 is a current implementation that satisfies this. If UUIDv8 replaces it, the invariant is unchanged — only the implementation ADR updates.

This level-of-abstraction principle is baked into the writing guide and into the template's inline writing guidance comment. Users learn it the first time they read their own generated template file.

## No coinage for guidelines

We considered coining "Guideline Record" (GR or GL) for symmetry. Decided against it.

Reasons:

1. **"Guideline" is an established English word** with its intended meaning. Coining "Guideline Record" adds cognitive load without adding clarity.
2. **Guidelines are softer than ADRs or invariants.** They're recommendations, not decisions or enforced rules. Formalizing them with a coined acronym overstates their weight.
3. **No collision pressure.** "Guideline" is specific enough on its own. ADR and INV need their short forms because "the decision we made" and "the rule" are too vague.
4. **Diminishing returns on coinages.** edikt already has "Agentic SDLC" (the category) and now "Invariant Record" (the artifact). A third coined term starts to feel like vocabulary inflation.

Guidelines stay as guidelines. They can still use numeric IDs if desired (`GL-001-function-length.md`), but `GL` is a file naming convention, not a coined term.

## Consequences

**Positive:**

- **Citeability.** "We use Invariant Records to enforce tenant isolation" is as precise as "we use ADRs to document decisions."
- **Consistency across edikt projects.** A named artifact type with a committed template means users inherit edikt's convention rather than inventing their own.
- **Parallelism with ADRs.** The governance story becomes symmetric: ADRs for decisions, Invariant Records for constraints. Users can reason about the two as a matched pair.
- **Brand equity.** If the term gains traction, "Invariant Record" becomes associated with edikt the same way ADRs are associated with Nygard. This earns credibility over time without claiming it upfront.
- **Precise documentation vocabulary.** The writing guide (separate artifact) can explicitly distinguish "what makes a good Invariant Record" from "what makes a good ADR", because the terms are now cleanly separable.

**Negative:**

- **Coinage is a commitment.** Once the term ships, we can't change it without breaking cite-ability and user expectations. The template is frozen (extensions only, never removals) unless a future ADR formally supersedes this one.
- **Honest labeling required.** We commit to transparently labeling "Invariant Record" as an edikt convention in all documentation, so users don't mistake it for an imported external standard like ADR is.
- **Additional vocabulary for users to learn.** New edikt users encounter a term they haven't seen elsewhere. Mitigated by the writing guide, which explains the term in its first paragraph.

## Directives

[edikt:directives:start]: #
source_hash: pending
directives_hash: pending
compiler_version: "0.3.0"
topic: compile
paths:
  - "commands/invariant/new.md"
  - "commands/invariant/compile.md"
  - "commands/invariant/review.md"
  - "commands/init.md"
  - "commands/doctor.md"
  - "website/governance/**"
  - "templates/examples/invariants/**"
scope:
  - implementation
  - design
directives:
  - "The formal name for an invariant governance artifact is 'Invariant Record'. The short form is 'INV'. File naming is INV-NNN-short-title.md. Never use 'IR' as a short form. (ref: ADR-009)"
  - "Invariant Records MUST contain the six-section body structure: Statement, Rationale, Consequences of violation, Implementation (optional), Anti-patterns (optional), Enforcement. An Invariant Record without an Enforcement section is invalid. (ref: ADR-009)"
  - "Invariant Records describe CONSTRAINTS, not IMPLEMENTATIONS. Use the test 'if our tech stack changed tomorrow, would this still apply?' to verify the level of abstraction. Implementation-specific rules belong in ADRs. (ref: ADR-009)"
  - "Invariant Records are NOT derived from ADRs. They are independent artifacts. When an invariant references an ADR, the reference goes in the Rationale or Implementation section as prose, not as a structured frontmatter field. (ref: ADR-009)"
  - "When documenting Invariant Records (in docs, website, or code comments), transparently label the term as 'an edikt convention, not an external standard'. Do not claim external authority. (ref: ADR-009)"
  - "Guidelines do NOT receive a coined term in v0.3.0. They remain as 'guideline' without a formal short form. (ref: ADR-009)"
[edikt:directives:end]: #
