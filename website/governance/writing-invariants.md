# Writing Invariant Records — a guide

This guide teaches you how to write effective Invariant Records. Read [invariant-records.md](invariant-records.md) for the template structure. This guide is about **how to write good ones**.

---

## Why this guide exists

Invariant Records fail silently when they're written poorly. A bad invariant looks like a rule but behaves like a wish: nobody can say when it's violated, nobody can enforce it, and nobody remembers why it was written. Over time, bad invariants accumulate in the governance directory and teach readers that invariants are ignorable.

Good invariants are enforceable, unambiguous, and load-bearing. They describe constraints that will still be true a year from now, regardless of which libraries you've swapped out. They name concrete failure modes. And they specify at least one mechanism for catching violations.

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

Seven yes/no questions. If any answer is "no", edit and retry.

---

## Two canonical examples (annotated)

The two invariants shipped as reference examples in edikt (`templates/examples/invariants/`) demonstrate the template at work. Both are reproduced in full below. The record's own prose runs as normal text; the commentary explaining why each section is written the way it is follows it as an indented aside.

Their IDs — `INV-942` and `INV-902` — are deliberately out of range. An example record must never be mistakable for a real invariant, either in your repository or in edikt's own governance directory, so they carry numbers no real sequence will reach.

### Example 1: Tenant isolation is total

**INV-942: Tenant isolation is total** — Date: 2026-04-09 · Status: Active

**Statement**

Every request, database query, log entry, and background job carries an authoritative tenant identifier, and every data access — read or write — is scoped to that tenant. There is no code path in the system where tenant context is optional.

> **Why the Statement is one sentence**: one sentence, present tense, absolute. No hedging. A reader knows immediately what the rule is without reading further.

**Rationale**

Multi-tenant systems face silent, high-cost failures when tenant isolation breaks. Unlike crashes or exceptions, cross-tenant data leakage is invisible — queries return rows, responses land in browsers, and customers never see an error message. The failure only surfaces when a customer notices their data in someone else's view, a regulator discovers the exposure during an audit, or a forensic investigation of an incident reveals the leak weeks or months after it happened.

The constraint must be **total**. Any phrasing like "scoped by tenant except in the admin panel" or "except for background analytics jobs" creates the exact code path where a future change forgets the exception and leaks data. Exceptions become permanent loopholes. The invariant applies everywhere, without exceptions, because the cost of a single leakage incident (customer trust loss, regulatory exposure, contractual damages) is orders of magnitude higher than the cost of enforcing the constraint pervasively.

> **Why the Rationale emphasizes "total"**: the section explicitly argues against exceptions. "Scoped except for the admin panel" creates the exact code path where a leak will eventually happen. Naming this failure mode in the Rationale pre-empts the usual "can we have an exception for X" conversation.

**Consequences of violation**

- **Cross-tenant data leakage** — silent, often undetected for weeks or months. Once a customer has seen another tenant's data, the exposure cannot be undone.
- **Regulatory exposure** — GDPR, SOC 2 Type II, HIPAA, and most enterprise compliance frameworks treat cross-tenant data exposure as a reportable breach. A single incident can trigger notification requirements, fines, and audit findings.
- **Customer trust collapse** — one leakage incident is often sufficient to lose an enterprise customer permanently. Enterprise buyers cannot use a system where tenant isolation is "usually" enforced.
- **Investigation overhead** — when a leak is discovered, reconstructing who saw what, when, and how often requires hours or days of forensic work across logs and database history.

> **Why Consequences of violation is concrete**: cross-tenant data leakage is silent. The section explains why silent failures are the worst kind — they're invisible until a customer or auditor finds them. This is the "what goes wrong" story that turns an abstract rule into a load-bearing one.

**Implementation**

- **Request authentication middleware** extracts the authoritative tenant ID from the signed session/JWT and binds it to the request context. The tenant ID from the request body or query parameters is never trusted.
- **Service layer** reads the authoritative tenant from context at the top of every method and passes it explicitly to every downstream call — repository, audit, events, logs. The service layer is where the context-to-argument translation happens.
- **Repository layer** is the sole path to the database. Every repository method takes `tenantID` as an explicit string parameter and injects `WHERE tenant_id = $tenant` on every query. The repository does not read context — tenant scope is a service-layer responsibility passed in explicitly. Raw SQL that bypasses the repository is forbidden.
- **Structured log calls** include `"tenant_id", tid` on every `slog.Info`, `slog.Warn`, `slog.Error` call. The logger does not add it automatically — the caller passes it explicitly. Fields that travel for free are fields that get forgotten on the paths where they matter most.
- **Background jobs** are always spawned with an explicit tenant context. On pickup, workers re-establish that context from the job record before processing. There are no "global" background jobs that iterate across all tenants in a single pass without re-scoping between each.
- **Tests** have a dedicated test tenant per fixture, never share tenant IDs across tests, and verify tenant scoping is respected in every database access path.

> **Why Implementation lists six layers**: request middleware, service layer, repository layer, structured logger, background jobs, tests. Six different places to enforce the same constraint. This isn't redundancy — it's defense in depth. A single mistake in any layer is caught by another.

**Anti-patterns**

- **Raw SQL outside the repository layer.** The repository injects tenant scoping automatically. Raw SQL bypasses this and must write the filter by hand, which is easy to forget.
- **Tenant ID from request body or query parameter.** The user can send whatever they want. Only the signed session is authoritative.
- **Joining tables without scoping both sides.** A tenant-scoped `users` table JOINed against an unscoped `audit_log` table can leak audit entries across tenants through the join. Every JOIN must filter every participating table.
- **"Global" background jobs** that process multiple tenants in a single pass without re-establishing scope per tenant.
- **Logging events "not attached to a tenant"** because they're "system events, not user events". Every event is a tenant event until proven otherwise; mark truly global events explicitly.
- **Admin interfaces that assume the admin has god-mode access.** Admin users still have a tenant scope (the admin org); cross-tenant access happens only through explicit impersonation flows, not by bypassing the filter.

> **Why Anti-patterns names specific traps**: raw SQL outside the repository, tenant ID from request body, JOINs without scoping both sides. Each is a concrete mistake Claude (or a human) can make without realizing. Concrete counter-examples are more effective than abstract warnings.

**Enforcement**

- **Linter / grep rule**: any raw SQL outside the repository layer fails the pre-push hook. Implemented as a simple grep for SQL keywords in source files not in the `repository/` directory.
- **Repository layer unit tests**: every repository method has a test that verifies it rejects a query constructed without a tenant filter. The test fixture explicitly passes an empty tenant ID and expects an error.
- **Route middleware**: requests without a valid tenant-bearing session are rejected at the edge, before reaching any handler. Missing tenant context is a 401, not a silent default.
- **Log schema validation**: a CI check ensures every structured log event includes `tenant_id`. Log events without the field fail the build.
- **edikt directive** loaded into Claude's context: "Every data access must be tenant-scoped. Every log line must include `tenant_id`. No exceptions. If you think you've found an exception, you haven't — ask before writing it."
- **Code review checklist**: any PR touching request handling, database access, logging, or background jobs requires explicit reviewer acknowledgment of tenant scoping. Implemented as a PR template checkbox.

Six enforcement mechanisms. Defense in depth. A single mistake in any one layer is caught by another.

> **Why Enforcement has six mechanisms**: linter, unit tests, route middleware, log schema validation, edikt directive, review checklist. The point isn't that you need all of these — it's that tenant isolation is important enough to justify multiple enforcement layers because no single mechanism catches everything.

**The compiled sidecar**

Compiled directives for this invariant live in a co-located sidecar at `INV-942-tenant-isolation.edikt.yaml` — edikt never writes to the prose body. The illustrative sidecar shape, conforming to the `gov-sidecar.v2` schema (line numbers refer to the record file itself):

```yaml
schema_version: 2
topic: database
path: INV-942-tenant-isolation.md
signals: [sql, tenant, repository, slog, audit]
directives:
  - text: "Every SQL query MUST include `tenant_id` in the WHERE clause. No exceptions. (ref: INV-942)"
    source_excerpts:
      - line_start: 48
        line_end: 48
        quote: "Every repository method takes `tenantID` as an explicit string parameter and injects `WHERE tenant_id = $tenant` on every query."
        role: statement
  - text: "Tenant ID MUST be read only from the verified session context. NEVER from request body, URL, or query string. No exceptions. (ref: INV-942)"
    source_excerpts:
      - line_start: 46
        line_end: 46
        quote: "The tenant ID from the request body or query parameters is never trusted."
        role: statement
  - text: "Every repository method MUST take `tenantID string` as an explicit parameter. NEVER read context inside the repository. (ref: INV-942)"
    source_excerpts:
      - line_start: 48
        line_end: 48
        quote: "The repository does not read context — tenant scope is a service-layer responsibility passed in explicitly."
        role: scope
  - text: "Every `slog.Info`, `slog.Warn`, `slog.Error` call MUST include `\"tenant_id\", tid`. No exceptions. (ref: INV-942)"
    source_excerpts:
      - line_start: 49
        line_end: 49
        quote: "include `\"tenant_id\", tid` on every `slog.Info`, `slog.Warn`, `slog.Error` call."
        role: statement
  - text: "Background job workers MUST re-establish tenant context from the job record before processing. (ref: INV-942)"
    source_excerpts:
      - line_start: 50
        line_end: 50
        quote: "On pickup, workers re-establish that context from the job record before processing."
        role: statement
reminders:
  - "Before writing SQL → MUST include `tenant_id` in WHERE clause (ref: INV-942)"
  - "Before adding a log call → MUST include `\"tenant_id\", tid` (ref: INV-942)"
verification:
  - "[ ] Every SQL query references `tenant_id` (ref: INV-942)"
  - "[ ] Every `slog.*` call includes `\"tenant_id\"` (ref: INV-942)"
  - "[ ] No raw SQL outside `internal/repository/` (ref: INV-942)"
```

Every directive carries at least one entry in `source_excerpts` — under `gov-sidecar.v2` an anchorless directive is ungrounded and cannot be drift-checked, so the field is required. See [the sidecar reference](sidecar.md) for the canonical schema.

### Example 2: Monetary values are fixed-point, never floating-point

**INV-902: Monetary values are fixed-point, never floating-point** — Date: 2026-04-09 · Status: Active

**Statement**

Any value representing money — in memory, in transit, at rest, in logs, in calculations, in aggregations — is stored and operated on as fixed-point (decimal or integer minor units). Floating-point types are never used for currency, prices, totals, fees, balances, or any derived monetary value.

> **Why the Statement enumerates locations**: "in memory, in transit, at rest, in logs, in calculations, in aggregations". Money is one of those constraints people accidentally violate at layer boundaries — stored correctly as `Decimal` in the database but loaded into a `float` at the application layer. The Statement pre-empts that mistake by being explicit about every place the rule applies.

**Rationale**

Floating-point arithmetic is inexact by design. IEEE 754 cannot represent most decimal fractions exactly, which means `0.1 + 0.2` evaluates to `0.30000000000000004`, not `0.3`. Small errors like this compound silently through repeated operations. For monetary values, silent rounding errors are not acceptable at any level — they accumulate into real financial discrepancies that surface weeks or months later during reconciliation, auditing, or customer complaints.

The correct representation for money is fixed-point: either a decimal type with explicit precision (`Decimal`, `BigDecimal`, `NUMERIC(18,4)`) or an integer in the smallest currency unit (cents, pennies, satoshi). These representations perform exact arithmetic within their defined precision and never introduce rounding errors unless the programmer explicitly requests rounding.

The constraint applies **uniformly**. It is not enough to store money as `Decimal` in the database and then convert to `float` for a calculation — the conversion itself introduces the precision error. Fixed-point must be used end-to-end, across every layer of the system, without exceptions.

> **Why the Rationale explains IEEE 754 briefly**: the "`0.1 + 0.2 == 0.30000000000000004`" example is famous but still surprising to many readers. Naming the root cause in the Rationale (IEEE 754 inexactness by design) grounds the rule in first principles, so readers understand why the constraint exists instead of just accepting it on authority.

**Consequences of violation**

- **Silent financial error** — totals, fees, balances, and interest calculations drift from their correct values. The errors are invisible until reconciliation shows the system's numbers don't match an external source of truth (bank statement, payment processor, ledger).
- **Reconciliation failures** — every discrepancy requires manual investigation. A single transaction off by a fraction of a cent can trigger hours of forensic work across logs and database history.
- **Customer trust damage** — "I was charged $10.01 instead of $10.00" is a small bug with an enormous trust cost. Customers who notice billing errors question every other number the system shows them.
- **Regulatory exposure** — financial reporting with rounding errors can trigger audit findings in regulated contexts (SOX, PCI DSS, banking regulations). Auditors expect exact arithmetic; rounding errors are a red flag.
- **Irreversible damage** — once an incorrect total has been invoiced or reported to a customer, correcting it requires outreach, credits, or refunds. The cost of fixing one discrepancy is far higher than the cost of preventing all of them.

**Implementation**

- **Storage**: PostgreSQL `numeric(18, 4)` or equivalent for decimal precision. Never `real`, `double precision`, or PostgreSQL's `money` type (which has its own subtle issues). For integer-based representations, use `bigint` with a documented minor-unit precision (e.g., "amount is in cents; 100 = $1.00").
- **In-memory types**:
  - Go: `github.com/shopspring/decimal.Decimal` or an equivalent library
  - Python: `decimal.Decimal` from the standard library
  - Java / Kotlin: `java.math.BigDecimal`
  - C# / .NET: `decimal` (language primitive)
  - Rust: `rust_decimal::Decimal`
  - JavaScript / TypeScript: `big.js`, `bignumber.js`, or integer cents — never JavaScript's native `Number`, which is a 64-bit float
- **API transport**: money values travel as strings (`"10.50"`) or as integer minor units (`1050` cents). Never as floats in JSON, which JavaScript clients will parse into `Number` and silently corrupt.
- **Arithmetic**: always use the decimal library's own methods (`d.Add()`, `d.Mul()`), never the language's native operators on the decimal type's underlying representation. Never mix fixed-point and floating-point in a single expression — coercion to float destroys precision.
- **Display**: formatting for presentation (human-readable strings with currency symbols and locale-specific separators) happens at the edge of the system, not in the calculation layer. The calculation layer works only in the canonical decimal representation.

> **Why Implementation is language-specific**: the section names the correct type per language (Go's shopspring/decimal, Python's Decimal, Java's BigDecimal, .NET's decimal, etc.). This is unusually specific for an invariant — typically implementation belongs in an ADR. The rationale: money handling is widely-enough understood that the correct type per language is effectively universal, and listing them prevents the common mistake of "my language has a type called `decimal`, is that the right one?" (It depends — C# yes, Rust's f-decimal no.)

**Anti-patterns**

- **`price: float` or `amount: double` in any schema** — database, API, in-memory type, DTO. This is the most common violation.
- **`JSON.parse` on a money value in a JavaScript client.** Even if the backend sends a string like `"10.50"`, using `parseFloat` or `Number()` on it converts it to a JavaScript `Number`, which is IEEE 754 float.
- **Converting to cents, computing in float, converting back.** "It's just pennies, what could go wrong" is a classic trap. Float errors at the cents level still accumulate.
- **Rounding to 2 decimal places at the end** as a "fix" for floating-point drift. Rounding is not a substitute for precision — it masks errors but doesn't prevent them, and it introduces its own biases (round-half-to-even vs round-half-up produce different results).
- **Spreadsheets (Excel, Google Sheets) as intermediate data format.** Spreadsheet cells silently convert numeric values to floats. Exporting via spreadsheet corrupts money.
- **Using a language's "numeric" type when it's actually a float alias.** JavaScript's `Number`, TypeScript's `number`, Python's `float` are all 64-bit floats. Type names can mislead — check the underlying representation.
- **Mixing decimal types across different libraries** without explicit conversion. Two libraries' `Decimal` types may have different precision or rounding behavior; implicit conversion can silently change values.

> **Why Anti-patterns names the "convert to cents, compute in float, convert back" trap**: this is a specific mistake engineers make while trying to "fix" floating-point problems. Naming it explicitly in the Anti-patterns section catches the clever-but-wrong attempt at compliance.

**Enforcement**

- **Database schema linter**: migrations containing `float`, `real`, `double`, or `double precision` types on columns with money-like names (`price`, `amount`, `total`, `balance`, `fee`, `cost`, `revenue`, `tax`, etc.) fail the pre-push hook. Implemented as a grep-based check on migration files.
- **Type-check rule**: CI fails if any function parameter or return type for money-related symbol names is a `float`, `double`, or language-native numeric float type. Implemented via the language's type checker or a custom AST rule.
- **API schema validation**: OpenAPI / JSON Schema definitions reject `"type": "number"` with float format for money-related fields; require `"type": "string"` or integer with explicit minor-unit semantics.
- **edikt directive** loaded into Claude's context: "Money is always decimal or integer cents. Never use float, double, or JavaScript Number for currency values. If in doubt about a type, check the underlying IEEE 754 representation before accepting it."
- **Code review checklist**: PRs touching pricing, billing, financial aggregations, tax calculations, or any monetary display require explicit reviewer acknowledgment of fixed-point handling.

Five enforcement mechanisms across database, type system, API contract, LLM context, and human review. Each catches a different class of mistake.

> **Why Enforcement mentions the database schema linter**: the most common violation is a `float` column in a database migration. Catching this at the migration layer is cheap and stops the problem before it enters the system. Enforcement at the database layer is often the most effective for data-type invariants.

Compiled directives for this invariant live in a co-located sidecar at `INV-902-money-precision.edikt.yaml`. edikt never writes to the prose body. See [the sidecar reference](sidecar.md) for the contract.

---

## The meta-lesson

**The single most important discipline**: describe the constraint, not the implementation. This one shift eliminates the most common failure mode and produces invariants that outlast your current stack.

---

## Writing for LLM compliance

The invariant you write is for humans. The directive the compile pipeline produces is for Claude. Both matter, but they fail differently — a well-written invariant with poorly compiled directives won't be followed. These rules help the compile pipeline produce effective directives from your invariant.

### Use absolute language in the Statement

The compile pipeline detects absolute quantifiers in your Statement ("every", "all", "total", "no ... exception") and appends "No exceptions." to the generated directive. This prevents Claude from rationalizing edge cases.

```
Statement that triggers reinforcement:
  "Every data access is scoped to the authenticated tenant."
  → Directive: "Every data access MUST be tenant-scoped. No exceptions. (ref: INV-942)"

Statement that doesn't trigger:
  "Data access should generally be tenant-scoped."
  → Directive: "Data access MUST be tenant-scoped. (ref: INV-942)"
  → No "No exceptions." suffix — the Statement hedged, so compile can't assert absoluteness.
```

Write absolutes when you mean absolutes. "Every" and "all" in the Statement are not just prose choices — they're compile signals.

### Name specific code tokens in Enforcement

The Enforcement section is where literal code tokens should appear. Compile lifts them into directives:

```
Weak enforcement:
  "Log calls should include tenant context."
  → Directive: "Log calls MUST include tenant context. (ref: INV-942)"
  → Claude doesn't know WHAT to type.

Strong enforcement:
  "Every slog.Info, slog.Warn, slog.Error call includes \"tenant_id\", tid."
  → Directive: "Every slog.Info, slog.Warn, slog.Error call MUST include \"tenant_id\", tid. (ref: INV-942)"
  → Claude knows exactly what to type.
```

Pre-registered experiments on Claude Opus 4.6 showed that literal code tokens in directives produce measurably higher compliance than abstract descriptions — especially on greenfield code and new domains where Claude has no existing patterns to copy.

### Provide grep-verifiable checks

The compile pipeline generates verification checklist items from your Enforcement section. If you name a concrete check, it becomes a self-audit item Claude runs before finishing:

```
Enforcement: "grep -rn tenant_id internal/repository/ — every query must match"
→ Checklist: "[ ] Every SQL query in internal/repository/ references tenant_id (ref: INV-942)"
```

Invariants with grep-verifiable enforcement produce higher compliance because Claude can check its own work mechanically.

### Check directive quality with `/edikt:gov:score`

After writing and compiling an invariant, run `/edikt:invariant:review` for per-directive LLM compliance scoring, or `/edikt:gov:score` for the aggregate governance quality report. Both score directives on token specificity, MUST/NEVER usage, grep-ability, and ambiguity. Directives scoring below 5/10 get concrete rewrite suggestions.

---

## See also

- [invariant-records.md](invariant-records.md) — The authoritative template
- [Tenant isolation](#example-1-tenant-isolation-is-total) — Full annotated example 1, on this page
- [Money precision](#example-2-monetary-values-are-fixed-point-never-floating-point) — Full annotated example 2, on this page
