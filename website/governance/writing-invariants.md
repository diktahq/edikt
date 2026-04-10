# Writing Invariant Records — a guide

This guide teaches you how to write effective Invariant Records. Read the [ADR-009 coinage doc](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-009-invariant-record-terminology.md) for the formal terminology and [invariant-records.md](invariant-records.md) for the template structure. This guide is about **how to write good ones**.

---

## Why this guide exists

Invariant Records fail silently when they're written poorly. A bad invariant looks like a rule but behaves like a wish: nobody can say when it's violated, nobody can enforce it, and nobody remembers why it was written. Over time, bad invariants accumulate in the governance directory and teach readers that invariants are ignorable.

Good invariants are enforceable, unambiguous, and load-bearing. They describe constraints that will still be true a year from now, regardless of which libraries you've swapped out. They name concrete failure modes. And they specify at least one mechanism for catching violations.

This guide gives you:

- **Five qualities** that make an invariant good
- **Seven traps** that make an invariant bad
- **Six bad-to-good rewrites** showing the editing process
- **A self-test** of seven questions to ask before committing any invariant
- **Two annotated canonical examples** — tenant isolation and money precision

---

## Five qualities of a good invariant

| # | Quality | The test |
|---|---|---|
| 1 | **Constraint, not implementation** | "If our stack changed tomorrow, would this rule still apply?" If yes → good. If no → abstract up a level. |
| 2 | **Declarative and absolute** | Read it aloud. If you hear "should", "try to", "usually", "where possible", "prefer" — it's a preference, not an invariant. |
| 3 | **Cross-cutting** | "Does this rule apply in at least 10 places in the codebase?" If only one place, it belongs in that file or an ADR, not an invariant. |
| 4 | **Enforceable** | "How do I catch a violation?" If the only answer is "careful reading", you don't have an invariant — you have a hope. |
| 5 | **Concrete consequences** | "What specifically goes wrong if this is violated?" You must name the failure mode. "The code is worse" isn't a failure mode. |

All five matter. Missing any one produces a soft or untestable rule. Missing #4 is the most common cause of invariant rot.

---

## The seven traps

Common failure modes in invariant writing. Each is paired with a symptom you can spot in a draft.

### 1. Wish invariants

> "Code should be clean and readable."

No enforcement mechanism is possible. This is a value, not a rule. Symptom: if you ask "how do I test whether this is violated?", the best answer is "human judgment".

**Fix**: either drop it (it's aspirational) or narrow it to something testable ("All public functions have docstrings of at least one sentence" — now you can grep for it).

### 2. Implementation invariants

> "Use Redis for caching."

Too specific. If you swap in Memcached or a different Redis client, the invariant is wrong. The actual constraint is usually one level up. Symptom: you can imagine a future stack change that would invalidate the invariant without changing the underlying need.

**Fix**: abstract to the constraint level. "Cached data is invalidated within 1 second of the source record being modified" captures the real requirement. The Redis choice belongs in an ADR.

### 3. Soft invariants

> "Prefer immutability where possible."

The "where possible" is a loophole wide enough to drive a truck through. Every violation can claim the exception. Symptom: the rule contains hedging language like "prefer", "try", "usually", "where possible", "when practical".

**Fix**: remove the hedging and see if the rule still holds. If "Data structures are immutable" is too strong, narrow the scope instead: "Value objects in the `domain` package are immutable." Narrower scope is better than hedged scope.

### 4. Subjective invariants

> "Functions should be short."

Short by whose standard? Whose taste? Unenforceable without arbitrary thresholds. Symptom: reasonable people would disagree about whether a specific example violates the rule.

**Fix**: identify the underlying principle and phrase it concretely. "Functions should be short" usually hides "functions do one thing" — the real rule. Phrase it as a principle: "A function either returns a computed value or modifies observable state. Not both." (Command-query separation, linter-checkable.)

### 5. Decision invariants

> "We evaluated Redis, Memcached, and Hazelcast and chose Redis for its persistence story."

This is an ADR. Invariants don't have alternatives. They're the rule, not the history of how the rule was chosen. Symptom: the writing reads like "we decided X because..." instead of "X is true".

**Fix**: move the content to an ADR. If a constraint emerges from the decision, write a separate Invariant Record for the constraint that stands independently: "Cached data persists across process restarts" (the requirement Redis satisfies).

### 6. Scoped-too-narrow invariants

> "The login page uses JWT for session tokens."

Applies to one file. Put it in the file or in an ADR, not in the invariants directory. Invariants are cross-cutting by definition. Symptom: you could inline the rule as a comment in a single file and it would be complete.

**Fix**: either promote it to a cross-cutting rule ("All session tokens are JWTs"), or drop the invariant format and put the constraint in the appropriate file as a code comment or documentation.

### 7. Contradictory invariants

> "Never use caching."
> (alongside)
> "Every database query completes in under 50ms."

These two rules cannot both be satisfied in any realistic system. Every time you add an invariant, check it doesn't conflict with existing ones. `/edikt:gov:compile` has contradiction detection to catch this at compile time, but the best time to notice is before writing.

**Fix**: resolve the conflict at design time. Usually one invariant is stated too absolutely and needs scoping ("Never use caching for data with strict consistency requirements"). Or one is wrong and should be retired.

---

## Six bad-to-good rewrites

Concrete transformations showing the editing process. Use these as models when editing your own drafts.

### Rewrite 1 — Implementation to constraint

```
❌ Bad: "Use Redis for caching."

✅ Good: "Cached data is invalidated within 1 second of the source
   record being modified. Stale cache entries are never returned
   to a caller."

Why the fix: the real constraint is about staleness and invalidation,
not the specific cache technology. Redis is an implementation choice
that belongs in an ADR. The invariant is implementation-agnostic —
if you swap to Memcached or an in-memory cache, the invariant is
unchanged; only the ADR needs to update.
```

### Rewrite 2 — Wish to testable rule

```
❌ Bad: "Code should handle errors properly."

✅ Good: "Every error returned to the user includes a structured
   error code and a human-readable message. Internal details
   (stack traces, database error messages, internal identifiers)
   never appear in user-facing errors."

Why the fix: "handle errors properly" is unenforceable — no one can
say what "properly" means in a specific case. The good version is
specific (structured code + message), testable (grep for stack
traces in error responses), and the failure mode is clear
(information leakage to end users).
```

### Rewrite 3 — Subjective to principle

```
❌ Bad: "Try to keep functions short."

✅ Good: "A function either returns a computed value or modifies
   observable state. Not both."

Why the fix: "short" is subjective — short by whose standard? The
underlying principle is usually command-query separation: functions
that mix reads and writes are hard to test and reason about. The
good version captures the principle, is linter-checkable (if you
build the linter), and doesn't depend on arbitrary length thresholds.
```

### Rewrite 4 — Vague to scope-bounded

```
❌ Bad: "Be careful with user data."

✅ Good: "Personally identifiable information (PII) — email,
   phone, address, full name, date of birth, government ID
   numbers — never appears in application logs, error messages,
   analytics events, or third-party API payloads."

Why the fix: "be careful" is a value with no boundary. The good
version enumerates the scope (explicit PII list), specifies the
forbidden destinations (logs, errors, analytics, third parties),
and is enforceable (log schema validation + grep-based pre-commit
hook on sensitive field names).
```

### Rewrite 5 — Trust-based to structural

```
❌ Bad: "Use parameterized SQL queries."

✅ Good: "All SQL queries reach the database through the query builder
   or prepared statement API. String interpolation into query text is
   forbidden without exception."

Why the fix: "use parameterized queries" sounds good but leaves room
for "I forgot just this once". The good version closes the loophole
by naming the allowed paths (query builder, prepared statements)
and explicitly forbidding the one thing that causes SQL injection
(string interpolation). Enforceable by grep for concatenation
patterns in query context.
```

### Rewrite 6 — Technology-bound to level-appropriate

```
❌ Bad: "Always use UUIDv7 for primary keys."

✅ Good: "Primary key identifiers are time-orderable."

Why the fix: UUIDv7 is today's implementation of a deeper constraint.
The real requirement is that identifiers sort chronologically when
compared as natural values. UUIDv7 is one way to achieve this;
other mechanisms (ULIDs, Snowflake IDs, monotonic timestamps with
disambiguation) also satisfy it. When UUIDv8 or another improvement
emerges, the invariant is unchanged — only the ADR that picked the
specific library updates. The invariant outlasts the library choice.
```

---

## The self-test

Before committing an invariant, answer these seven questions. If you can't answer any of them clearly, the invariant isn't ready.

1. **What exactly is the rule?** Say it in one sentence. If you need two sentences, try again.

2. **When would I regret NOT having this rule?** Name a concrete failure scenario. "Things would be worse" isn't specific enough.

3. **How does a violation get caught?** Name at least one mechanism. "Code review" counts but is the weakest — prefer automated.

4. **Does it apply in at least 10 places in the codebase?** If not, it's too narrow to be an invariant. Put it in a file or an ADR.

5. **If our stack changed tomorrow, would the rule still apply?** If no, you're describing an implementation, not a constraint. Abstract up.

6. **Is anyone going to argue about it?** If yes, it's an ADR-level decision that needs discussion, not an invariant. Invariants should be uncontroversial within the team.

7. **Can you phrase it without "should", "try", "where possible", "prefer"?** If no, it's a preference, not an invariant.

Seven yes/no questions. If any answer is "no", edit and retry. Invariants are worth the extra care — the ones that survive this filter are the load-bearing rules of your architecture.

---

## Two canonical examples (annotated)

The two invariants shipped as reference examples in edikt demonstrate the template at work. Below are the full examples with commentary on why each section is written the way it is.

### Example 1: Tenant isolation is total

See [`canonical-invariants/tenant-isolation.md`](canonical-invariants/tenant-isolation.md) for the full file.

**Why the Statement is one sentence**: "Every request, query, log entry, and background job carries an authoritative tenant identifier, and every data access is scoped to that tenant." One sentence, present tense, absolute. No hedging. A reader knows immediately what the rule is without reading further.

**Why the Rationale emphasizes "total"**: the section explicitly argues against exceptions. "Scoped except for the admin panel" creates the exact code path where a leak will eventually happen. Naming this failure mode in the Rationale pre-empts the usual "can we have an exception for X" conversation.

**Why Consequences of violation is concrete**: cross-tenant data leakage is silent. The section explains why silent failures are the worst kind — they're invisible until a customer or auditor finds them. This is the "what goes wrong" story that turns an abstract rule into a load-bearing one.

**Why Implementation lists five layers**: request middleware, repository layer, structured logger, background jobs, tests. Five different places to enforce the same constraint. This isn't redundancy — it's defense in depth. A single mistake in any layer is caught by another.

**Why Anti-patterns names specific traps**: raw SQL outside the repository, tenant ID from request body, JOINs without scoping both sides. Each is a concrete mistake Claude (or a human) can make without realizing. Concrete counter-examples are more effective than abstract warnings.

**Why Enforcement has five mechanisms**: linter, unit tests, route middleware, log schema validation, edikt directive, review checklist. (That's six actually — another layer of defense.) The point isn't that you need all of these — it's that tenant isolation is important enough to justify multiple enforcement layers because no single mechanism catches everything.

### Example 2: Monetary values are fixed-point, never floating-point

See [`canonical-invariants/money-precision.md`](canonical-invariants/money-precision.md) for the full file.

**Why the Statement enumerates locations**: "in memory, in transit, at rest, in logs, in calculations, in aggregations". Money is one of those constraints people accidentally violate at layer boundaries — stored correctly as `Decimal` in the database but loaded into a `float` at the application layer. The Statement pre-empts that mistake by being explicit about every place the rule applies.

**Why the Rationale explains IEEE 754 briefly**: the "`0.1 + 0.2 == 0.30000000000000004`" example is famous but still surprising to many readers. Naming the root cause in the Rationale (IEEE 754 inexactness by design) grounds the rule in first principles, so readers understand why the constraint exists instead of just accepting it on authority.

**Why Implementation is language-specific**: the section names the correct type per language (Go's shopspring/decimal, Python's Decimal, Java's BigDecimal, .NET's decimal, etc.). This is unusually specific for an invariant — typically implementation belongs in an ADR. The rationale: money handling is widely-enough understood that the correct type per language is effectively universal, and listing them prevents the common mistake of "my language has a type called `decimal`, is that the right one?" (It depends — C# yes, Rust's f-decimal no.)

**Why Anti-patterns names the "convert to cents, compute in float, convert back" trap**: this is a specific mistake engineers make while trying to "fix" floating-point problems. Naming it explicitly in the Anti-patterns section catches the clever-but-wrong attempt at compliance.

**Why Enforcement mentions the database schema linter**: the most common violation is a `float` column in a database migration. Catching this at the migration layer is cheap and stops the problem before it enters the system. Enforcement at the database layer is often the most effective for data-type invariants.

---

## The meta-lesson

Good invariants are small, absolute, enforceable, and cross-cutting. They describe constraints that hold regardless of implementation. They name their failure modes concretely. They specify at least one way to catch violations.

Bad invariants are vague, hedged, or tied to specific technology. They read like values instead of rules. They have no enforcement. They drift over time because nobody can tell when they're violated.

**The single most important discipline**: describe the constraint, not the implementation. This one shift eliminates the most common failure mode and produces invariants that outlast your current stack.

Everything else is polish.

---

## See also

- [ADR-008](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-008-deterministic-compile-and-three-list-schema.md) — The three-list directive schema and hash caching contract
- [ADR-009](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-009-invariant-record-terminology.md) — The formal coinage of "Invariant Record"
- [invariant-records.md](invariant-records.md) — The authoritative template
- [`canonical-invariants/tenant-isolation.md`](canonical-invariants/tenant-isolation.md) — Full annotated example 1
- [`canonical-invariants/money-precision.md`](canonical-invariants/money-precision.md) — Full annotated example 2
