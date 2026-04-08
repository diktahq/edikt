# /edikt:invariant:review

Review invariant language quality — checks whether hard constraints are specific, actionable, and phrased for reliable enforcement.

This is a scoped shortcut for running `/edikt:gov:review` targeting invariants only.

## Usage

```bash
/edikt:invariant:review
/edikt:invariant:review INV-001
```

## Arguments

| Argument | Description |
|----------|-------------|
| (none) | Review all active invariants in `docs/invariants/` |
| `INV-NNN` | Review a specific invariant |

## What it checks

Invariants are held to a higher standard than ADRs — they must be non-negotiable, verifiable, and carry a clear consequence for violation.

Each invariant is evaluated on:

| Dimension | Strong | Weak |
|-----------|--------|------|
| **Specificity** | Names exact patterns, types, or operations | Vague or broadly interpretable |
| **Phrasing** | NEVER/MUST with explicit consequence | "avoid" or "try to" language |
| **Violation signal** | Describes what a violation looks like | No way to tell if violated |
| **Testability** | Checkable by static analysis, grep, or test | Cannot be verified mechanically |

## When to run

- After writing a new invariant, before it goes active
- Before running `/edikt:gov:compile` — clean invariant language produces clean enforcement

## What's next

- [/edikt:invariant:new](/commands/invariant/new) — capture a new hard constraint
- [/edikt:invariant:compile](/commands/invariant/compile) — compile invariants into governance directives
- [/edikt:gov:review](/commands/gov/review) — full governance review across all sources
