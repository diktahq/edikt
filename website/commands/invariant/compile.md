# /edikt:invariant:compile

Regenerate the sidecar for exactly one invariant.

In v0.6.0, every invariant has a co-located `<INV>.edikt.yaml` sidecar that holds compiled directives. `:compile` regenerates that single sidecar in a fresh subagent context with a locked extraction prompt. It does not touch the prose `.md`. It does not run topic-file rendering — that's `gov:compile` Phase B.

## Usage

```bash
/edikt:invariant:compile INV-001
/edikt:invariant:compile docs/architecture/invariants/INV-001-no-floats-for-money.md
```

## Arguments

| Argument | Description |
|----------|-------------|
| `INV-NNN` or path | The invariant to recompile |

## What it does

1. Resolves the invariant path from the ID or argument
2. Dispatches the `sidecar-extractor` agent (`context: fork`, `model: sonnet`, `maxTurns: 1`, `tools: [Read, Write]`) with a locked prompt
3. The agent reads the Statement, Implementation, and Enforcement sections of the prose `.md`
4. Extracts MUST/NEVER directives, derives `topic` and `signals`, captures `source_excerpt` per directive
5. Writes `<INV>.edikt.yaml` (canonical YAML serialization)

Output:

```text
Regenerated INV-001-no-floats-for-money.edikt.yaml
INV-001-no-floats-for-money.edikt.yaml unchanged
```

The "unchanged" path is the idempotency contract: running `:compile` twice on an unchanged body produces a byte-equal sidecar.

## When to run

- After editing an invariant's prose body (the sidecar is now stale)
- When `/edikt:doctor` flags `MISSING` for the sidecar
- Manually, before running `/edikt:gov:compile`, to avoid the Phase A subagent dispatch

You usually don't need to run this directly. `/edikt:gov:compile` auto-resyncs stale sidecars in Phase A.

## What's next

- [/edikt:invariant:new](/commands/invariant/new) — capture a new hard constraint (creates the sidecar atomically)
- [/edikt:invariant:review](/commands/invariant/review) — review language quality + cross-check sidecar drift
- [/edikt:gov:compile](/commands/gov/compile) — full governance compile (Phase A resync + Phase B merge)
- [Sidecar Architecture](/governance/sidecar) — what sidecars are and why
