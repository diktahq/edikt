# /edikt:invariant:compile

Regenerate the directive sidecar for one invariant — or every active invariant if no argument is given.

In v0.6.0, every invariant has a co-located `<INV>.edikt.yaml` sidecar that holds compiled directives. `:compile` regenerates it in a fresh subagent context with a locked extraction prompt. It does not touch the prose `.md`. It does not run topic-file rendering — that's `gov:compile` Phase B.

## Usage

```bash
/edikt:invariant:compile INV-001
/edikt:invariant:compile docs/architecture/invariants/INV-001-no-floats-for-money.md
/edikt:invariant:compile
```

## Arguments

| Argument | Description |
|----------|-------------|
| `INV-NNN` or path | The invariant to recompile |
| (omitted) | Recompile every active invariant |

## What it does

1. Resolves the invariant path(s) from the ID, path, or (if omitted) every active invariant
2. Dispatches the `sidecar-extractor` agent (`context: fork`, `model: sonnet`, `maxTurns: 8`, `tools: [Read, Write]`) with a locked prompt — at most 2 concurrently when processing all invariants
3. The agent reads the Statement, Implementation, and Enforcement sections of the prose `.md`
4. Extracts MUST/NEVER directives, derives `topic` and `signals`, captures `source_excerpts` per directive
5. Writes `<INV>.edikt.yaml` (canonical YAML serialization)

<!-- contract-report:website-compile:start -->
Before it plans or dispatches anything, the command runs a contract check, and the check itself never refuses, pauses or asks. What you see depends on the result. When the extraction contract is validated, you see nothing. When it is rejected, or has no entry in the validation record, the command prints one line saying so before it dispatches the extractor, and repeats that line in the confirmation. When the state could not be measured (an older binary, or no readable extractor), it prints one line saying that, and the line ends with what to do about it. The command shows it to you in full and does not make that repair itself, because changing an agent file is your decision. When no extractor can be read, the compile that follows is refused by the plan step, which repeats that line first. Whenever the compile goes ahead under a contract that is not validated, the confirmation repeats the line and adds a reminder to review the sidecar's directives against the prose before relying on them.
<!-- contract-report:website-compile:end -->

For a single target:

```text
✅ INV-001.edikt.yaml — regenerated
   Source: docs/architecture/invariants/INV-001-no-floats-for-money.md
```

For an all-targets run:

```text
Invariant sidecar regeneration:
  ✅ INV-001 — regenerated
  ✅ INV-002 — unchanged
  ...

  1 regenerated, 1 unchanged, 0 skipped (revoked).
  Next: Run /edikt:gov:compile to update the topic-grouped governance files.
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
