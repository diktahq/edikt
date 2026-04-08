# /edikt:invariant:compile

Compile active invariants into governance directives.

This is a scoped shortcut for running `/edikt:gov:compile` with invariants as the only source. Use it when you've added or updated invariants and want to refresh directives without recompiling ADRs or guidelines.

## Usage

```bash
/edikt:invariant:compile
/edikt:invariant:compile --check
```

## Arguments

| Argument | Description |
|----------|-------------|
| (none) | Compile invariants and write updated governance rule files |
| `--check` | Validate only — report conflicts without writing |

## What it does

Reads all active invariants in `docs/invariants/` and extracts or reads sentinel directive blocks from each one. Invariants are always placed first in compiled output — they are non-negotiable constraints that take precedence over all other directives.

Superseded invariants are excluded automatically.

For full compilation across ADRs, invariants, and guidelines together, use [`/edikt:gov:compile`](/commands/gov/compile).

## When to run

After capturing a new invariant with `/edikt:invariant:new`.

## What's next

- [/edikt:invariant:new](/commands/invariant/new) — capture a new hard constraint
- [/edikt:invariant:review](/commands/invariant/review) — review invariant language quality before compiling
- [/edikt:gov:compile](/commands/gov/compile) — full governance compilation across all sources
