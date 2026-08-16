# /edikt:adr:compile

Regenerate the sidecar for exactly one ADR.

In v0.6.0, every ADR has a co-located `<ADR>.edikt.yaml` sidecar that holds compiled directives. `:compile` regenerates that single sidecar in a fresh subagent context with a locked extraction prompt. It does not touch the prose `.md`. It does not run topic-file rendering — that's `gov:compile` Phase B.

## Usage

```bash
/edikt:adr:compile ADR-003
/edikt:adr:compile docs/architecture/decisions/ADR-003-use-postgres-for-persistence.md
```

## Arguments

| Argument | Description |
|----------|-------------|
| `ADR-NNN` or path | The ADR to recompile |

## What it does

1. Resolves the ADR path from the ID or argument
2. Dispatches the `sidecar-extractor` agent (`context: fork`, `model: sonnet`, `maxTurns: 8`, `tools: [Read, Write]`) with a locked prompt
3. The agent reads the Decision section of the prose `.md`
4. Extracts MUST/NEVER directives, derives `topic` and `signals`, captures `source_excerpts` per directive
5. Writes `<ADR>.edikt.yaml` (canonical YAML serialization — sorted keys, 2-space indent, LF line endings)

The output is one of:

```text
Regenerated ADR-003-use-postgres-for-persistence.edikt.yaml
ADR-003-use-postgres-for-persistence.edikt.yaml unchanged
```

The "unchanged" path is the idempotency contract: running `:compile` twice on an unchanged body produces a byte-equal sidecar.

## When to run

- After editing an ADR's prose body (the sidecar is now stale)
- When `/edikt:doctor` flags `MISSING` for the sidecar (ADR has no companion `.edikt.yaml`)
- Manually, before running `/edikt:gov:compile`, to avoid the Phase A subagent dispatch

You usually don't need to run this directly. `/edikt:gov:compile` auto-resyncs stale sidecars in Phase A by calling this command per artifact.

## Idempotency

`:compile` is idempotent. The agent prompt is locked; the canonical YAML serializer is deterministic; the body hash is recomputed on read. Running twice on an unchanged body produces a byte-equal sidecar. CI uses this property to detect drift.

## Forked subagent context

Generation runs in a forked subagent (`context: fork`). The dispatching session does not see other artifacts; each `:compile` call gets a clean slate. This is the v0.6.0 fix for an earlier contamination bug where one ADR's directive count dropped because the parent context had absorbed other ADRs' prose.

## What's next

- [/edikt:adr:new](/commands/adr/new) — capture a new architecture decision (creates the sidecar atomically)
- [/edikt:adr:review](/commands/adr/review) — review ADR language quality + cross-check sidecar drift
- [/edikt:gov:compile](/commands/gov/compile) — full governance compile (Phase A resync + Phase B merge)
- [Sidecar Architecture](/governance/sidecar) — what sidecars are and why
