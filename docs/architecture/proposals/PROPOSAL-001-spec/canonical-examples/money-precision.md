# INV-008: Monetary values are fixed-point, never floating-point

**Date:** 2026-04-09
**Status:** Active

<!--
Writing guidance (edikt convention — see ADR-009):

1. Describe the CONSTRAINT, not the IMPLEMENTATION.
2. Present tense, declarative, no hedging.
3. Invariants are NOT derived from ADRs. They stand alone.
4. An invariant without Enforcement is a wish.
-->

## Statement

Any value representing money — in memory, in transit, at rest, in logs, in calculations, in aggregations — is stored and operated on as fixed-point (decimal or integer minor units). Floating-point types are never used for currency, prices, totals, fees, balances, or any derived monetary value.

## Rationale

Floating-point arithmetic is inexact by design. IEEE 754 cannot represent most decimal fractions exactly, which means `0.1 + 0.2` evaluates to `0.30000000000000004`, not `0.3`. Small errors like this compound silently through repeated operations. For monetary values, silent rounding errors are not acceptable at any level — they accumulate into real financial discrepancies that surface weeks or months later during reconciliation, auditing, or customer complaints.

The correct representation for money is fixed-point: either a decimal type with explicit precision (`Decimal`, `BigDecimal`, `NUMERIC(18,4)`) or an integer in the smallest currency unit (cents, pennies, satoshi). These representations perform exact arithmetic within their defined precision and never introduce rounding errors unless the programmer explicitly requests rounding.

The constraint applies **uniformly**. It is not enough to store money as `Decimal` in the database and then convert to `float` for a calculation — the conversion itself introduces the precision error. Fixed-point must be used end-to-end, across every layer of the system, without exceptions.

## Consequences of violation

- **Silent financial error** — totals, fees, balances, and interest calculations drift from their correct values. The errors are invisible until reconciliation shows the system's numbers don't match an external source of truth (bank statement, payment processor, ledger).
- **Reconciliation failures** — every discrepancy requires manual investigation. A single transaction off by a fraction of a cent can trigger hours of forensic work across logs and database history.
- **Customer trust damage** — "I was charged $10.01 instead of $10.00" is a small bug with an enormous trust cost. Customers who notice billing errors question every other number the system shows them.
- **Regulatory exposure** — financial reporting with rounding errors can trigger audit findings in regulated contexts (SOX, PCI DSS, banking regulations). Auditors expect exact arithmetic; rounding errors are a red flag.
- **Irreversible damage** — once an incorrect total has been invoiced or reported to a customer, correcting it requires outreach, credits, or refunds. The cost of fixing one discrepancy is far higher than the cost of preventing all of them.

## Implementation

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

## Anti-patterns

- **`price: float` or `amount: double` in any schema** — database, API, in-memory type, DTO. This is the most common violation.
- **`JSON.parse` on a money value in a JavaScript client.** Even if the backend sends a string like `"10.50"`, using `parseFloat` or `Number()` on it converts it to a JavaScript `Number`, which is IEEE 754 float.
- **Converting to cents, computing in float, converting back.** "It's just pennies, what could go wrong" is a classic trap. Float errors at the cents level still accumulate.
- **Rounding to 2 decimal places at the end** as a "fix" for floating-point drift. Rounding is not a substitute for precision — it masks errors but doesn't prevent them, and it introduces its own biases (round-half-to-even vs round-half-up produce different results).
- **Spreadsheets (Excel, Google Sheets) as intermediate data format.** Spreadsheet cells silently convert numeric values to floats. Exporting via spreadsheet corrupts money.
- **Using a language's "numeric" type when it's actually a float alias.** JavaScript's `Number`, TypeScript's `number`, Python's `float` are all 64-bit floats. Type names can mislead — check the underlying representation.
- **Mixing decimal types across different libraries** without explicit conversion. Two libraries' `Decimal` types may have different precision or rounding behavior; implicit conversion can silently change values.

## Enforcement

- **Database schema linter**: migrations containing `float`, `real`, `double`, or `double precision` types on columns with money-like names (`price`, `amount`, `total`, `balance`, `fee`, `cost`, `revenue`, `tax`, etc.) fail the pre-push hook. Implemented as a grep-based check on migration files.
- **Type-check rule**: CI fails if any function parameter or return type for money-related symbol names is a `float`, `double`, or language-native numeric float type. Implemented via the language's type checker or a custom AST rule.
- **API schema validation**: OpenAPI / JSON Schema definitions reject `"type": "number"` with float format for money-related fields; require `"type": "string"` or integer with explicit minor-unit semantics.
- **edikt directive** loaded into Claude's context: "Money is always decimal or integer cents. Never use float, double, or JavaScript Number for currency values. If in doubt about a type, check the underlying IEEE 754 representation before accepting it."
- **Code review checklist**: PRs touching pricing, billing, financial aggregations, tax calculations, or any monetary display require explicit reviewer acknowledgment of fixed-point handling.

Five enforcement mechanisms across database, type system, API contract, LLM context, and human review. Each catches a different class of mistake.

<!-- Directives for edikt governance. Populated by /edikt:invariant:compile. -->
[edikt:directives:start]: #
[edikt:directives:end]: #
