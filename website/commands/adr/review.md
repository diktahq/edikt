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
| `--backfill` | Interactive retrofit: populate `canonical_phrases` on existing multi-sentence ADRs |

## Soft-language markers (v0.5.0)

In addition to the four quality dimensions below, the review now flags six soft-language markers in directive bodies:

| Marker | Why it's flagged | Suggested replacement |
|---|---|---|
| `should` | Implies optionality | `MUST` |
| `ideally` | Suggests best effort | `MUST` |
| `prefer` | Non-mandatory | `MUST` (positive) or `NEVER` (negative) |
| `try to` | Effort without commitment | `MUST` |
| `might` | Probabilistic framing | Rewrite as definitive |
| `consider` | Advisory, not directive | `MUST evaluate X` or remove |

For each flagged occurrence, the review shows the directive text, the marker, and a suggested replacement using `MUST` or `NEVER` with one-clause reasoning.

## `--backfill` flag (v0.5.0)

Retrofit `canonical_phrases` onto existing multi-sentence ADRs interactively:

```bash
/edikt:adr:review --backfill
```

For each ADR with a multi-sentence directive and no `canonical_phrases`:

1. The command proposes 2–3 candidate phrases derived from a noun/verb heuristic applied to the directive body
2. It shows the rationale for each candidate
3. You approve (`y`), skip (`n`), or edit (`e`) before the field is written

The `[e]dit` option opens an inline editor for the phrase list before confirming. One ADR at a time; `Ctrl+C` to stop without losing already-completed ADRs.

After backfill, re-run `/edikt:gov:compile` to pick up the new phrases.

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
