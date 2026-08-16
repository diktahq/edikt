# Sidecar Architecture

In v0.6.0, every governed artifact — every ADR, Invariant Record, and guideline — has a co-located sidecar that holds its compiled directives. edikt only ever writes to the sidecar. Your prose `.md` is never touched by `gov:compile`.

```text
docs/architecture/decisions/
├── ADR-001-api-versioning.md          ← you write this. edikt never touches it.
└── ADR-001-api-versioning.edikt.yaml  ← edikt writes this. directives live here.
```

The sidecar is a YAML file co-located with the artifact, sharing its base name. It conforms to `templates/schemas/gov-sidecar.v2.schema.json` (`schema_version: 2`). v2 replaced v1's singular `source_excerpt` with `source_excerpts[]` — an ordered array of 1..N anchors per directive — and added an optional `actor_scope: true` field that excludes a directive from write-time delivery while it still renders in ambient surfaces. A v1-shaped sidecar must be migrated (`bin/edikt migrate to-v2`) before it validates.

## Why this exists

In the legacy format, every accepted ADR carried a generated `[edikt:directives:start]` block at the bottom of its prose body. Compile mutated the file in place under an `EDIKT_COMPILE_IN_PROGRESS` bypass. Accepted ADRs are immutable — but the boundary was definitional, not structural. Every reader had to know "the sentinel block is generated, the rest is not."

That contract broke twice in v0.6.0-rc1:

1. **Cross-artifact context contamination.** Compile ran across many ADRs in one Claude session. The parent context absorbed every ADR's prose; by the time it extracted a later ADR's directives, it had already deduplicated against earlier ones and silently dropped directives.
2. **Compile coupled to the parent session.** Because compile mutated immutable files, it had to run inside the session that set `EDIKT_COMPILE_IN_PROGRESS`. External tooling — a CI workflow, a separate terminal — couldn't participate cleanly.

The sidecar pattern makes the boundary structural. Two files, two writers: you own the `.md`, edikt owns the `.edikt.yaml`. Each artifact compiles in its own fresh subagent context with a locked extraction prompt — no cross-artifact bleed.

## What's in a sidecar

```yaml
schema_version: 2
topic: hooks
path: ADR-003-database-choice.md
signals:
  - hook
  - posttooluse
  - sentinel
directives:
  - text: "Use PostToolUse hooks for auto-formatting after Write or Edit."
    source_excerpts:
      - line_start: 87
        line_end: 89
        quote: "Use PostToolUse hooks for auto-formatting after Write or Edit. Fire only on known source file extensions."
    verify: "grep -q 'PostToolUse' .claude/hooks/post-tool-use.sh"
  - text: "CLAUDE.md sentinels use visible markdown link reference definitions."
    actor_scope: true
    source_excerpts:
      - line_start: 142
        line_end: 144
        quote: "CLAUDE.md sentinels use visible markdown link reference definitions: `[edikt:start]: #` and `[edikt:end]: #`. NEVER use HTML comment sentinels."
verification:
  - text: "[ ] /post-tool-use.sh references PostToolUse"
    verify: "grep -q 'PostToolUse' .claude/hooks/post-tool-use.sh"
```

| Field | What it is |
|---|---|
| `schema_version` | `2`. Bumped only when compile needs structural changes older tooling cannot read. |
| `topic` | Kebab-case topic slug. Drives topic-grouped rule files in `.claude/rules/governance/`. |
| `path` | Relative path to the parent `.md`. Doctor verifies it resolves to the sibling. |
| `signals` | Domain keywords, lowercase and deduplicated. Legacy field — the routing table they fed was retired under ADR-059; topic membership is now driven by `paths:` matching and skill-package trigger descriptions instead. |
| `directives[].text` | The directive sentence (≤ 200 chars). |
| `directives[].source_excerpts[]` | An ordered array of 1..N verbatim quotes from the prose body, each with its own line range. v2 replaced v1's singular `source_excerpt` so a directive whose meaning spans separate sentences can anchor to all of them. Used by `:review` to detect drift. |
| `directives[].actor_scope` | **Optional v2** — `true` excludes this directive from write-time PreToolUse delivery while it still renders in ambient/topic-file surfaces. |
| `directives[].verify` | **Optional v0.6.0** — a single shell command run by `bin/edikt verify gov <ID>`. Exit 0 = the directive holds; non-zero = the directive is violated. Omit when no mechanical check is possible — the field stays absent, not empty. Same optional field is available on `prohibitions[].verify` and on the structured form of `verification[]` items. |

What's **not** in the sidecar: hashes. `source_hash`, `directives_hash`, `agent_prompt_version` — all forbidden at the root. Hashes are recomputed on read at compile time, so commits never carry a stale hash.

### `verify:` and the completion-evidence discipline

Every claim-bearing slot in a sidecar — `directives[]`, `prohibitions[]`, structured `verification[]` on gov; `requirements[]` and `acceptance_criteria[]` on PRD and SPEC — MAY carry an optional `verify:` field. The runner (`bin/edikt verify gov|prd|spec <ID>`) executes each one via `bash -c` with a 30-second timeout. Exit 0 is a pass, non-zero is a fail, and missing means "skipped".

The discipline is wired into every completion-claiming path: gov compile blocks on failure post-merge, `/edikt:sdlc:prd ship` refuses on failure, drift folds failures into its report. The doctor's "Sidecar Verify Coverage" line surfaces gaps as soft warnings — never blocks. See [`edikt verify`](/commands/verify) for the full contract.

## When sidecars regenerate

| Trigger | Command | Scope |
|---|---|---|
| New artifact | `/edikt:adr:new`, `/edikt:invariant:new`, `/edikt:guideline:new` | Creates the `(.md, .edikt.yaml)` pair atomically via a forked subagent with a locked extraction prompt. |
| Manual refresh | `/edikt:adr:compile <id>`, `/edikt:invariant:compile <id>`, `/edikt:guideline:compile <id>` | Regenerates exactly one sidecar. Idempotent — running twice on an unchanged body produces a byte-equal sidecar. |
| Compile auto-resync | `/edikt:gov:compile` Phase A | Detects stale sidecars (body hash mismatch) and dispatches per-artifact `:compile` commands in parallel (concurrency 8). |

The dispatcher is always the same: the `sidecar-extractor` agent runs in a forked subagent with a single artifact path, a locked prompt, `Read + Write` tools, and `maxTurns: 1`. The locking prevents prompt drift; the forking prevents cross-artifact contamination.

## Two-phase compile

`/edikt:gov:compile` runs in two phases:

**Phase A — Resync (conditional).** If any sidecars are stale, dispatch parallel subagents to regenerate them. Concurrency 8, continue-on-error, mandatory progress UI on stderr. No latency SLO — resync legitimately costs LLM time.

**Phase B — Merge (always).** Read every sidecar, group by topic, render `.claude/rules/governance/<topic>.md`. Pure deterministic merge. No LLM, no `Task`/`Agent` dispatch. Latency: `<5s` cold, `<500ms` no-op, `<2s` for `--check`. A static-analysis test enforces that no LLM-dispatch symbol is reachable from the merge code path.

`--check` mode skips Phase A entirely. If any sidecar is stale, `--check` exits 1 with the list of stale sidecars and a single recovery command. CI gates run `--check`.

The full latency story is on the [Compile](compile) page.

## Topic files

Phase B writes one file per topic under `.claude/rules/governance/`, plus two machine-readable surfaces alongside them:

```text
.claude/rules/governance/
├── architecture.md            ← topic file: pathless/manual directives with topic: architecture
├── hooks.md                   ← topic file: pathless/manual directives with topic: hooks
├── release.md
├── tooling.md
├── directive-index.yaml       ← glob-keyed YAML; bin/edikt hook match's exclusive input
└── manifest.yaml              ← every rendered surface, path + kind + SHA-256, no timestamp
```

Not every topic renders a topic file: a topic whose contributing sidecars declare no `paths:` at all is reachable only through its skill package (`.claude/skills/edikt-<topic>/SKILL.md`) instead.

Per ADR-066, a sidecar's `Directives`/`Prohibitions` text contributes to its topic file's compiled-directives region only when that sidecar declares **no** `paths:`. A sidecar that does declare `paths:` contributes its directive text to `directive-index.yaml` alone. In a corpus where scoping is thorough — every contributing sidecar declares `paths:` — a topic file still renders (to carry its `paths:` frontmatter and any pathless/manual content), but its compiled-directives region is empty: open and close sentinel markers with nothing between them. That's expected, not a bug.

Each topic file carries a `_fingerprint:` field in its frontmatter — a sorted SHA-256 of the contributing sidecar paths and content hashes. If a single sidecar changes, only its topic file rerenders; every other topic file is byte-equal across the two compiles. This keeps `git diff` legible after edits. `manifest.yaml` gives the same freshness guarantee across the whole render: it lists every surface's SHA-256 with no timestamp field, so it is byte-identical for byte-identical input.

## Editing rules manually

The sidecar is YAML, so you can edit it directly. Two cases come up in practice:

- **Suppressing a generated rule.** Delete the entry from `directives[]`. Re-running `/edikt:adr:compile` regenerates it from the prose, so this only sticks if you also change the prose. For permanent suppression, either remove the source language from the prose body, or open an `:override` mechanism (deferred to a later release — until then, edit the prose).
- **Adding a rule compile missed.** Add an entry to `directives[]` with a `source_excerpts[]` entry quoting the prose line that justifies it. `/edikt:<type>:review` will cross-check that the quote still appears in the body and warn on drift.

The sidecar is not a place to write rules that have no prose backing. `:review` will flag those as "extra in sidecar." If a rule is real, document it in the prose body and re-run compile.

## Doctor checks

`/edikt:doctor` runs five sidecar-health checks:

| Check | Severity |
|---|---|
| `ORPHAN` — `.edikt.yaml` with no sibling `.md` | Hard fail |
| `MISSING` — `.md` with no sibling `.edikt.yaml` | Hard fail |
| `PATH MISMATCH` — sidecar's `path:` doesn't resolve to the sibling | Hard fail |
| Schema validation failure | Hard fail |
| `directives: []` — empty sidecar (deliberately or after edit) | Soft warning |

The soft warning catches sidecars that lost their directives after a prose rewrite — a signal that you may want to re-run `:compile`.

## Migration from legacy in-body blocks

v0.6.0 reads sidecars only — there is no fallback to in-body sentinels. The first time you upgrade a project, `/edikt:upgrade` detects legacy `[edikt:directives:start]` blocks and offers `edikt migrate sidecars`. The migration:

- Lifts existing sentinel blocks into co-located sidecars
- Detects schema version per-artifact (v0.4.3 `content_hash:` legacy vs v0.5.x/v0.6.0-rc1 `source_hash:`)
- Removes the in-body block from each `.md` only after the sidecar writes successfully
- Skips known doc-mention files (files that reference the old format, SPEC-* files) and any sentinel block that lives inside a fenced code region

If you decline the prompt, `/edikt:gov:compile` refuses with a single-line actionable error directing you to `/edikt:upgrade`. There is no double-parser window.

The full walkthrough is in [Sidecar Migration](/guides/sidecar-migration).

## Reference

- Schema: `templates/schemas/gov-sidecar.v2.schema.json`
