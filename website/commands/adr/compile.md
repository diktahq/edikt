# /edikt:adr:compile

Regenerate the directive sidecar for one ADR — or every accepted ADR if no argument is given.

In v0.6.0, every ADR has a co-located `<ADR>.edikt.yaml` sidecar that holds compiled directives. `:compile` regenerates it in a fresh subagent context with a locked extraction prompt. It does not touch the prose `.md`. It does not run topic-file rendering — that's `gov:compile` Phase B.

## Usage

```bash
/edikt:adr:compile ADR-003
/edikt:adr:compile docs/architecture/decisions/ADR-003-use-postgres-for-persistence.md
/edikt:adr:compile
```

## Arguments

| Argument | Description |
|----------|-------------|
| `ADR-NNN` or path | The ADR to recompile |
| (omitted) | Recompile every accepted ADR |

## What it does

1. Resolves the ADR path(s) from the ID, path, or (if omitted) every accepted ADR
2. Dispatches the `sidecar-extractor` agent (`context: fork`, `model: sonnet`, `maxTurns: 8`, `tools: [Read, Write]`) with a locked prompt — at most 2 concurrently when processing all ADRs
3. The agent reads the Decision section of the prose `.md`
4. Extracts MUST/NEVER directives, derives `topic` and `signals`, captures `source_excerpts` per directive
5. Writes `<ADR>.edikt.yaml` (canonical YAML serialization — sorted keys, 2-space indent, LF line endings)

<!-- contract-report:website-compile:start -->
Before it plans or dispatches anything, the command runs a contract check, and the check itself never refuses, pauses or asks. What you see depends on the result. When the extraction contract is validated, you see nothing. When it is rejected, or has no entry in the validation record, the command prints one line saying so before it dispatches the extractor, and repeats that line in the confirmation. When the state could not be measured (an older binary, or no readable extractor), it prints one line saying that, and the line ends with what to do about it. The command shows it to you in full and does not make that repair itself, because changing an agent file is your decision. When no extractor can be read, the compile that follows is refused by the plan step, which repeats that line first. Whenever the compile goes ahead under a contract that is not validated, the confirmation repeats the line and adds a reminder to review the sidecar's directives against the prose before relying on them.
<!-- contract-report:website-compile:end -->

For a single target:

```text
✅ ADR-003.edikt.yaml — regenerated
   Source: docs/architecture/decisions/ADR-003-use-postgres-for-persistence.md
```

For an all-targets run:

```text
ADR sidecar regeneration:
  ✅ ADR-001 — regenerated
  ✅ ADR-002 — unchanged
  ✅ ADR-003 — unchanged
  ...

  1 regenerated, 2 unchanged, 0 skipped (superseded/draft).
  Next: Run /edikt:gov:compile to update the topic-grouped governance files.
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
