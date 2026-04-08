# /edikt:adr:compile

Compile accepted ADRs into governance directives.

This is a scoped shortcut for running `/edikt:gov:compile` with ADRs as the only source. Use it when you've added or updated ADRs and want to refresh directives without recompiling invariants or guidelines.

## Usage

```bash
/edikt:adr:compile
/edikt:adr:compile --check
```

## Arguments

| Argument | Description |
|----------|-------------|
| (none) | Compile ADRs and write updated governance rule files |
| `--check` | Validate only — report contradictions without writing |

## What it does

Reads all accepted ADRs in `docs/decisions/` and extracts or reads sentinel directive blocks from each one. Groups directives by topic and writes them into the appropriate topic files under `.claude/rules/governance/`.

For full compilation across ADRs, invariants, and guidelines together, use [`/edikt:gov:compile`](/commands/gov/compile).

## When to run

After capturing a new ADR with `/edikt:adr:new` and accepting it.

## What's next

- [/edikt:adr:new](/commands/adr/new) — capture a new architecture decision
- [/edikt:adr:review](/commands/adr/review) — review ADR language quality before compiling
- [/edikt:gov:compile](/commands/gov/compile) — full governance compilation across all sources
