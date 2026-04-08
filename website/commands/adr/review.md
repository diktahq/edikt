# /edikt:adr:review

Review ADR language quality — checks whether decisions are specific, actionable, and phrased for reliable enforcement.

This is a scoped shortcut for running `/edikt:gov:review` targeting ADRs only.

## Usage

```bash
/edikt:adr:review
/edikt:adr:review ADR-003
```

## Arguments

| Argument | Description |
|----------|-------------|
| (none) | Review all ADRs in `docs/decisions/` |
| `ADR-NNN` | Review a specific ADR |

## What it checks

Each directive in an ADR is scored on four dimensions:

| Dimension | Strong | Vague |
|-----------|--------|-------|
| **Specificity** | Names exact patterns, functions, or formats | Could mean anything to different readers |
| **Actionability** | One clear action, no ambiguity | No actionable instruction |
| **Phrasing** | NEVER/MUST with one-clause reason for hard constraints | Reads as a suggestion |
| **Testability** | Verifiable by grep, test, or code review | Cannot be verified |

## When to run

- After writing a new ADR, before accepting it
- Periodically — ADR language quality can drift as context accumulates

## What's next

- [/edikt:adr:new](/commands/adr/new) — capture a new architecture decision
- [/edikt:adr:compile](/commands/adr/compile) — compile ADRs into governance directives
- [/edikt:gov:review](/commands/gov/review) — full governance review across all sources
