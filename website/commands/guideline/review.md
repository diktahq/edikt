# /edikt:guideline:review

Review guideline language quality — checks whether team standards are specific, actionable, and written for consistent enforcement.

This is a scoped shortcut for running `/edikt:gov:review` targeting guidelines only.

## Usage

```bash
/edikt:guideline:review
/edikt:guideline:review guideline-api-response-casing
```

## Arguments

| Argument | Description |
|----------|-------------|
| (none) | Review all guidelines in `docs/guidelines/` |
| A guideline slug or filename | Review a specific guideline |

## What it checks

Each guideline directive is scored on four dimensions:

| Dimension | Strong | Vague |
|-----------|--------|-------|
| **Specificity** | Names exact patterns, formats, or locations | Could mean anything to different readers |
| **Actionability** | One clear action, no ambiguity | No actionable instruction |
| **Phrasing** | Active voice with clear scope | Passive or hedged language |
| **Testability** | Verifiable by code review or tooling | Cannot be verified |

## When to run

- After writing a new guideline, before it's compiled
- Periodically — guidelines can accumulate vague language over time

## What's next

- [/edikt:guideline:new](/commands/guideline/new) — capture a new team standard
- [/edikt:gov:compile](/commands/gov/compile) — compile guidelines into governance directives
- [/edikt:gov:review](/commands/gov/review) — full governance review across all sources
