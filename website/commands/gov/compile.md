# /edikt:gov:compile

Compile accepted ADRs, active invariants, and team guidelines into topic-grouped governance rule files.

The output is four rendered surfaces under `.claude/rules/` (`compile_schema_version: 3`, ADR-059): an always-loaded ambient core (`.claude/rules/governance.md`), per-topic files under `.claude/rules/governance/` loaded when a matching file is edited, a glob-keyed `directive-index.yaml` that drives write-time enforcement, and a `manifest.yaml` that proves nothing drifted. A topic file's own directive region is populated only from sidecars with no `paths:` — path-scoped directive text renders in `directive-index.yaml` instead (ADR-066), so a topic file is not guaranteed to be "full-fidelity" on its own. See [Output structure](#output-structure) below.

In v0.6.0, compile runs in two phases: **Phase A** (resync, conditional, LLM-backed) and **Phase B** (merge, always, deterministic). Phase B preserves a tight latency budget; Phase A has no SLO but emits mandatory progress UI.

## Usage

```bash
/edikt:gov:compile
/edikt:gov:compile --check
/edikt:gov:compile --no-wait
```

## Arguments

| Argument | Description |
|----------|-------------|
| (none) | Run Phase A (if stale) + Phase B + post-merge verify gate; write governance index + topic rule files |
| `--check` | Phase B only; exit 1 with stale-sidecar list if any sidecar is stale. Verify gate is skipped (nothing was written). |
| `--no-wait` | Fail fast (exit 1) instead of waiting on the `.edikt/state/compile.lock` |
| `--skip-verify` | Skip the post-merge verify gate. Use sparingly — disabling the gate defeats the completion-evidence discipline. |

## Two-phase architecture (v0.6.0)

### Phase A — Resync (conditional)

Runs only when one or more sidecars are stale. A sidecar is stale when the SHA-256 of its parent `.md`'s body no longer matches the body hash the sidecar was generated from (recomputed on read, never committed).

For every stale sidecar, compile dispatches a goroutine that shells out to the per-artifact `:compile` command. Concurrency is capped at 8 via a semaphore. Failures log to `.edikt/state/compile-errors.log` and don't abort the run; remaining subagents continue.

Progress UI on stderr is mandatory:

```text
Phase A — resyncing 3 stale sidecars
  ✓ ADR-001-api-versioning           (12.4s)
  ✓ ADR-007-caching-strategy     (18.1s)
  ⏳ ADR-022-rate-limiting… [▓▓▓░░░] ETA 22s
```

If any sidecar fails, Phase B does not run. Compile prints the aggregated failure summary and exits 1.

**Latency:** no SLO. Per-artifact resync is 30–60s p50.

### Phase B — Merge (always)

Reads every `<artifact>.edikt.yaml` under `docs/architecture/decisions/`, `docs/architecture/invariants/`, and `docs/guidelines/`. Validates against `templates/schemas/gov-sidecar.v2.schema.json`. Groups by topic. Renders the four `compile_schema_version: 3` output surfaces from the merged directive set with canonical serialization — the ambient core, per-topic files, `directive-index.yaml`, and `manifest.yaml` (see [Output structure](#output-structure) below).

Pure deterministic merge — no LLM, no `Task`/`Agent` dispatch, no shell-out. A static-analysis test (`tools/edikt/check/no-llm-in-merge.sh`) verifies that no LLM-dispatch symbol is reachable from the Phase B code path. The check runs in CI.

**Latency budget:**

| Mode | Budget |
|---|---|
| Full regenerate from cold cache (50 sidecars) | `<5s` |
| No-op (all sidecars unchanged) | `<500ms` |
| `--check` mode | `<2s` |

**Diff-only rendering:** topic files carry a `_fingerprint:` field — a sorted SHA-256 of contributing sidecar paths and content hashes. If a fingerprint matches the existing file's, Phase B skips the rewrite. Modifying one sidecar therefore only rerenders its topic file.

#### The `_fingerprint:` field — stability contract

The `_fingerprint:` line in the YAML frontmatter of every `.claude/rules/governance/<topic>.md` is the short-circuit that gives Phase B its `<500ms` no-op budget. It is a sorted SHA-256 over the contributing sidecar paths and their canonical-marshaled content. Treat it as opaque, tool-owned bytes:

- **Do not hand-edit or strip the line.** Doing so forces a full re-render of that topic file on the next compile (correctness preserved; performance degraded for one run).
- **The field lives in the compiled topic file, not in the sidecar schema.** Sidecars carry source-of-truth content; the fingerprint is a derivation observable on the output side, which is where the determinism guarantee binds.
- **The hash is canonicalized over the marshaled sidecar bytes.** Re-running compile with byte-equal input produces a byte-equal fingerprint.
- **Why it's not in the sidecar:** the sidecar is human-reviewable structured data; embedding a hash of the rendered output in the input would couple the layers and break the "edikt does not write to inputs" rule.

If you see fingerprints differing across runs with no apparent input change, file an issue — that's a determinism break, which is a bug.

### Phase C — Verify gate (always, post-merge)

After Phase B succeeds, `gov compile` shells to `bin/edikt verify all` as a subprocess. The walker runs every `verify:` shell command declared in every gov / prd / spec sidecar under the configured artifact dirs.

```text
Phase B done: 3 topic files rendered
  + gov/ADR-001 — 4 passed, 0 failed, 0 skipped
  + gov/ADR-007 — 2 passed, 0 failed, 1 skipped
  + gov/INV-009 — 8 passed, 0 failed, 1 skipped

summary: 39 sidecars (0 failing); 168 items: 14 passed, 0 failed, 0 timeout, 154 skipped
```

If any verify failed or timed out, `gov compile` exits 1 with a remediation summary:

```text
error: gov compile produced output, but the verify gate found failing sidecar(s).
Re-run gov compile after fixing the failing directives or removing their verify commands.
```

Output bytes are still on disk — `gov compile` does not roll back Phase B on a verify failure. The exit code is the signal; the user fixes the failing directive or removes its `verify:` line and re-runs.

The gate is **skipped** in three cases:

- `--check` mode — nothing was written, so there's nothing to gate.
- `--json` mode — the gate's text output would corrupt the structured payload. JSON consumers should run `edikt verify all --json` themselves if they need both.
- `--skip-verify` — explicit opt-out, recorded in compile output.

Items declared without a `verify:` field are recorded as `skipped:operational` so coverage can be measured separately from health. The doctor's "Sidecar Verify Coverage" line surfaces low coverage as a soft warning.

### `--check` mode

Skips Phase A entirely. If any sidecar is stale, exits 1 with:

```text
✗ Stale sidecars: ADR-001, ADR-007, ADR-022
  Run /edikt:gov:compile to resync.
```

CI gates run `--check`. Because `--check` never dispatches a subagent, it is deterministic and fast.

### Concurrent compile

Compile takes an advisory file-lock at `.edikt/state/compile.lock`. A second invocation while one is running waits by default, or fails fast with `--no-wait`.

### `--json` (two-phase mode)

`gov compile --json` emits a single JSON document on stdout summarizing both phases. Prose progress lines are routed to stderr at low verbosity so machine-readable consumers see only the JSON object.

Shape:

```json
{
  "status": "ok",
  "phase_a": {
    "dispatched": 0,
    "stale": 0,
    "errors": []
  },
  "phase_b": {
    "topics_rendered": [],
    "topics_unchanged": ["governance/architecture.md", "governance/compile.md"],
    "index_written": false,
    "total_directives": 138
  }
}
```

`status` is `"ok"` on a successful run, `"error"` when compile exited non-zero (the run still completed enough to emit JSON; check `error` for the message). Phase A's `dispatched` and `stale` counts agree on a successful resync (every stale sidecar was dispatched). On Phase A failure, `phase_a.errors[]` lists each artifact that failed and `phase_b` is omitted (Phase B does not run when Phase A failed). Output shape is the contract — exit codes carry status; output is for machine-to-machine piping or human consumption.

`--dry-run` is an alias for `--check` (added for parity with `migrate sidecars` and `verify` flag conventions).

## How it works

### Sidecar reads (v0.6.0+)

Each governance document (ADR, invariant, guideline) has a co-located `<artifact>.edikt.yaml` sidecar that holds its compiled directives. Compile reads only sidecars — never the prose `.md`:

```yaml
# docs/architecture/decisions/ADR-008-redis-caching.edikt.yaml
schema_version: 2
topic: cache
path: ADR-008-redis-caching.md
signals: [cache, redis, ttl]
directives:
  - text: "Use allkeys-lru eviction policy for all Redis instances (ref: ADR-008)"
    source_excerpts:
      - line_start: 42
        line_end: 42
        quote: "All Redis instances MUST use allkeys-lru eviction."
  - text: "Max key size: 1MB. Max TTL: 24h for data caches, 5min for ACL caches (ref: ADR-008)"
    source_excerpts:
      - line_start: 58
        line_end: 59
        quote: "Key size: max 1MB. TTL: 24h for data caches, 5min for ACL caches."
```

Sidecar shape and lifecycle are documented under [Sidecar Architecture](/governance/sidecar). The per-artifact `:compile` commands and the `/edikt:gov:compile` Phase A regenerator both invoke the same locked `sidecar-extractor` agent in a forked subagent context — there is no cross-artifact bleed.

### Refusal on legacy in-body sentinels

If a project still has legacy in-body `[edikt:directives:start]` blocks, compile exits non-zero with a single-line actionable error directing the user to `/edikt:upgrade` (or `edikt migrate sidecars`). There is no double-parser window — compile reads sidecars only. See [`/edikt:migrate sidecars`](/commands/migrate) for the two-phase migration.

### Topic grouping

Directives from all sources are grouped by topic. All caching rules from different ADRs and guidelines merge into `governance/cache.md`. All database rules merge into `governance/database.md`. Each directive keeps its source reference.

### Two loading mechanisms

1. **`paths:` frontmatter** — Claude Code auto-loads a topic file when editing a matching file. Platform-enforced, no reasoning step.
2. **Write-time hook match** — `bin/edikt hook match` reads `directive-index.yaml` and matches the file actually being written against each directive's own glob, independent of whether the topic file happens to be loaded. This is what fires the PreToolUse deny (`must`) or PostToolUse `additionalContext` (`advisory`) described in [Two-phase architecture](#two-phase-architecture-v0-6-0) above.

There is no keyword-matched routing table in the current output (`compile_schema_version: 3`) — that mechanism was retired. See [How Governance Compiles](/governance/compile) for the full history.

## Output structure

Phase B renders four surfaces under `.claude/rules/`, per `compile_schema_version: 3` (ADR-059):

```text
.claude/rules/
├── governance.md                        ← ambient core: pathless invariants + one-line topic index (always loaded)
└── governance/
    ├── architecture.md                  ← topic file: pathless/manual content for topic "architecture"
    ├── cache.md                         ← topic file: pathless/manual content for topic "cache"
    ├── directive-index.yaml             ← glob-keyed index; bin/edikt hook match's exclusive input
    └── manifest.yaml                    ← every surface's path + kind + SHA-256, no timestamp
```

A topic whose contributing sidecars all declare `paths:` still renders a topic file (to carry the `paths:` frontmatter and its Reminders/Verification Checklist), but its `Directives`/`Prohibitions` region is empty — that content lives in `directive-index.yaml` instead, per ADR-066's single-home rule. A topic with no `paths:`-declaring or pathless sidecars at all renders no topic file and is reached only through its skill package (`.claude/skills/edikt-<topic>/SKILL.md`).

### Governance index (governance.md)

The ambient core. Contains only:
- Pathless-invariant canonical statements — the rules that hold everywhere, no exceptions, regardless of the file touched
- A one-line topic index (short task-language description per topic, no keywords)

No routing table, no aggregate reminders, no verification checklist — those were retired with schema 2. See [How Governance Compiles](/governance/compile) for the full four-surface breakdown.

### Topic files (governance/*.md)

Each contains:
- `paths:` frontmatter for auto-loading — the union of every contributing sidecar's declared globs
- A `Directives`/`Prohibitions` region, populated only from sidecars that declare no `paths:` (may be empty)
- A hand-authored `ManualDirectives` region (may also be empty)
- Always-present `Reminders` and `Verification Checklist` sections, scoped to that topic

### Directive index (directive-index.yaml) and manifest (manifest.yaml)

`directive-index.yaml` is the machine-readable surface: glob-keyed, carrying each directive's pinned `grade`, its `text`, a `falsifying_observation`, and its reminders. It's not meant for ambient reading — `bin/edikt hook match` is its only consumer. `manifest.yaml` lists every rendered surface with its SHA-256, deliberately with no timestamp, so it's byte-identical across byte-identical input and can tell a stale render from a fresh one.

## Compilation summary

```text
✅ Governance compiled

  governance/cache.md
    ← ADR-008 (§Eviction, §TTL Strategy)
    ← guideline-database.md (§Caching)

  governance/database.md
    ← ADR-003 (§Queries, §Migrations)

  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  5 ADRs + 2 invariants + 1 guideline
  → 3 topic files + index
  → 27 total directives
  Sidecar coverage: 8/8 documents (100%)
```

## Contradiction detection

Before writing, the command checks for contradictions:

```text
CONTRADICTION DETECTED
  ADR-001: "All persistence uses PostgreSQL"
  ADR-007: "Use DynamoDB for the event store"

  Resolve before compiling. Supersede one or reconcile both.
```

Invariant conflicts are errors — invariants always win. ADR conflicts are warnings.

## Cross-reference validation

Every directive that references an ADR or invariant (`ref: ADR-NNN`, `ref: INV-NNN`) is verified against the actual source file. If the referenced identifier doesn't exist in the source document, the reference is stripped — preventing fabricated cross-references from reaching the compiled output.

This also runs in `/edikt:gov:review` when scoring sidecars.

## CI validation

```bash
/edikt:gov:compile --check
```

Reports contradictions, conflicts, sidecar staleness, and directive counts without writing any files. Exits 1 if any sidecar is stale (use `/edikt:gov:compile` without flags to resync).

## Orphan ADR detection

Compile detects ADRs with no directives and no `no-directives:` reason field.

**Warn-then-block semantics:**

1. First compile with an orphan ADR — warns with the ADR path and exits 0 (non-blocking).
2. Second consecutive compile with the same orphan (or a superset) — blocks with exit ≠ 0.

Resolve by:
- Regenerating the ADR's sidecar with directives (`/edikt:adr:compile`)
- Or marking the ADR with `no-directives: <reason ≥ 10 chars>` in its frontmatter (for ADRs that are deliberately non-directive, e.g., purely contextual records)

**State persistence:**

Orphan state is tracked in `.edikt/state/compile-history.json` via atomic rename. The `.edikt/state/` directory is auto-appended to `.gitignore` — this is local machine state, not repo state.

## Directive quality checks

Before writing, compile invokes the shared directive-quality sub-procedure (`commands/gov/_shared-directive-checks.md`) — the same sub-procedure used by `/edikt:gov:review`. It covers:

- Warns on multi-sentence directives without `canonical_phrases`
- Warns when a `canonical_phrase` value does not appear in the directive body
- **`no-directives` reason validation** — if `no-directives:` is present, the reason must be ≥ 10 characters and not a placeholder (`tbd`, `todo`, `fix later`)

These checks are warn-only. Hard-fail is targeted for a future release.

## `canonical_phrases` and `behavioral_signal` (legacy, currently inert)

These two fields existed on the pre-v0.6.0 in-body sentinel block. They are **not** part of the current `gov-sidecar.v2` schema (see the sidecar shape under [Sidecar reads](#sidecar-reads-v0-6-0) above — no such fields) and compile does not read or forward them. `/edikt:gov:benchmark`'s behavioral-signal filter currently has nothing to match against and short-circuits to "all skipped." See [Sentinel Blocks](/governance/sentinels) for the legacy field reference.

## Migration check (v0.6.0)

At start, compile detects pre-v0.6.0 in-body `[edikt:directives:start]` blocks. If any are found in non-skip-list, non-fenced files, compile refuses with:

```text
✗ Migration required.
  Run /edikt:upgrade to migrate this project to v0.6.0 sidecar architecture.
```

There is no fallback to in-body sentinel parsing. v0.6.0 reads sidecars only. See [Sidecar Migration](/guides/sidecar-migration) for the walkthrough.

## When to run

Run after:
- Capturing a new ADR with `/edikt:adr:new`
- Adding an invariant with `/edikt:invariant:new`
- Updating a guideline file
- Editing the prose body of an existing accepted ADR or active invariant (Phase A will auto-resync the sidecar)

## What's next

- [/edikt:gov:review](/commands/gov/review) — review directive quality and check for contradictions
- [/edikt:adr:new](/commands/adr/new) — capture an architecture decision
- [/edikt:invariant:new](/commands/invariant/new) — add a hard constraint
