# Writing Invariant Records — condensed guide

This is the short version. The full guide with annotated examples lives at `edikt.dev/governance/writing-invariants` or in the source tree at `docs/architecture/proposals/PROPOSAL-001-spec/writing-invariants-guide.md`.

## Five qualities of a good invariant

| # | Quality | Test |
|---|---|---|
| 1 | **Constraint, not implementation** | "If our stack changed tomorrow, would this rule still apply?" |
| 2 | **Declarative and absolute** | No "should", "try to", "usually", "where possible" |
| 3 | **Cross-cutting** | Applies in ≥10 places in the codebase |
| 4 | **Enforceable** | You can name at least one way to catch violations |
| 5 | **Concrete consequences** | You can name the specific failure mode |

## Seven traps to avoid

1. **Wish invariants** — "Code should be clean." Unenforceable. Not an invariant.
2. **Implementation invariants** — "Use Redis for caching." Tied to a specific tech, not a constraint. Belongs in an ADR.
3. **Soft invariants** — "Prefer immutability where possible." The "where possible" is a loophole. Remove the hedging or narrow the scope.
4. **Subjective invariants** — "Functions should be short." Short by whose standard? Find the underlying principle.
5. **Decision invariants** — "We evaluated options and chose X." That's an ADR, not an invariant. Invariants don't have alternatives.
6. **Scoped-too-narrow** — "The login page uses JWT." Applies to one file. Put it in the file or in an ADR.
7. **Contradictory invariants** — Two rules that can't both hold. Resolve before publishing.

## Six bad-to-good rewrites

```
❌ "Use Redis for caching"
✅ "Cached data is invalidated within 1 second of the source record being modified"

❌ "Code should handle errors properly"
✅ "Every error returned to the user includes a structured error code and a
    human-readable message. Internal details never appear in user-facing errors."

❌ "Try to keep functions short"
✅ "A function either returns a computed value or modifies observable state. Not both."

❌ "Be careful with user data"
✅ "PII (email, phone, address, name, DOB, gov ID) never appears in application logs,
    error messages, analytics events, or third-party API payloads."

❌ "Use parameterized SQL queries"
✅ "All SQL queries reach the database through the query builder or prepared statement
    API. String interpolation into query text is forbidden without exception."

❌ "Always use UUIDv7 for primary keys"
✅ "Primary key identifiers are time-orderable."
```

Notice the pattern: the good version describes the **constraint** that the rule exists to enforce. The implementation choice (Redis, UUIDv7) belongs in an ADR.

## The seven-question self-test

Before committing an invariant, answer all seven. If any answer is "no", edit and retry.

1. **What exactly is the rule?** One sentence.
2. **When would I regret NOT having this rule?** Name a concrete failure scenario.
3. **How does a violation get caught?** Name at least one mechanism (automated preferred; manual code review counts but is weakest).
4. **Does it apply in at least 10 places in the codebase?** If not, too narrow.
5. **If our stack changed tomorrow, would the rule still apply?** If not, implementation detail — belongs in an ADR.
6. **Is anyone going to argue about it?** If yes, ADR discussion, not an invariant. Invariants should be uncontroversial within the team.
7. **Can you phrase it without "should", "try", "where possible"?** If not, it's a preference.

## The mandatory structure

Every Invariant Record has six body sections (two optional) plus a directives block:

```markdown
# INV-NNN: Short declarative title

**Date:** YYYY-MM-DD
**Status:** Active

## Statement
<One sentence, declarative, present tense, no hedging.>

## Rationale
<Why this exists. Implementation-agnostic.>

## Consequences of violation
<Concrete failure mode. Be specific.>

## Implementation (optional but strongly encouraged)
<Concrete patterns that satisfy the constraint.>

## Anti-patterns (optional but strongly encouraged)
<Concrete examples of violations and why they're wrong.>

## Enforcement
<At least one mechanism. "Careful reading" doesn't count.>

[edikt:directives:start]: #
[edikt:directives:end]: #
```

## See also

- `tenant-isolation.md` and `money-precision.md` in this directory for two worked examples
- `edikt.dev/governance/writing-invariants` for the full guide with annotated examples
- ADR-009 for the formal terminology coinage
- ADR-008 for the three-list directive schema contract

## "Invariant Record" terminology

The term is an edikt convention, not an external standard. Always label your usage transparently — "edikt's Invariant Records" when publishing externally, not as if it were a widely-adopted industry term. See ADR-009 for the rationale.
