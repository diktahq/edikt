# /edikt:gov:compile

Compile accepted ADRs, active invariants, and team guidelines into topic-grouped governance rule files.

The output — `.claude/rules/governance.md` (index) and `.claude/rules/governance/*.md` (topic files) — is read by Claude automatically every session. Each topic file contains full-fidelity directives for a specific domain, loaded only when relevant files are being edited.

In v0.6.0, compile runs in two phases per [ADR-028](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-028-two-phase-compile-resync-merge.md): **Phase A** (resync, conditional, LLM-backed) and **Phase B** (merge, always, deterministic). Phase B preserves [ADR-020](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-020-gov-compile-tier-2-migration.md)'s latency budget; Phase A has no SLO but emits mandatory progress UI.

## Usage

```bash
/edikt:gov:compile
/edikt:gov:compile --check
/edikt:gov:compile --no-wait
```

## Arguments

| Argument | Description |
|----------|-------------|
| (none) | Run Phase A (if stale) + Phase B; write governance index + topic rule files |
| `--check` | Phase B only; exit 1 with stale-sidecar list if any sidecar is stale |
| `--no-wait` | Fail fast (exit 1) instead of waiting on the `.edikt/state/compile.lock` |

## Two-phase architecture (v0.6.0)

### Phase A — Resync (conditional)

Runs only when one or more sidecars are stale. A sidecar is stale when the SHA-256 of its parent `.md`'s body no longer matches the body hash the sidecar was generated from (recomputed on read, never committed).

For every stale sidecar, compile dispatches a goroutine that shells out to the per-artifact `:compile` command. Concurrency is capped at 8 via a semaphore. Failures log to `.edikt/state/compile-errors.log` and don't abort the run; remaining subagents continue.

Progress UI on stderr is mandatory:

```text
Phase A — resyncing 3 stale sidecars
  ✓ ADR-001-claude-code-only           (12.4s)
  ✓ ADR-007-compile-schema-version     (18.1s)
  ⏳ ADR-022-single-go-binary-replaces… [▓▓▓░░░] ETA 22s
```

If any sidecar fails, Phase B does not run. Compile prints the aggregated failure summary and exits 1.

**Latency:** no SLO. Per-artifact resync is 30–60s p50.

### Phase B — Merge (always)

Reads every `<artifact>.edikt.yaml` under `docs/architecture/decisions/`, `docs/architecture/invariants/`, and `docs/guidelines/`. Validates against `templates/schemas/sidecar.v1.schema.json`. Groups by topic. Renders each topic file from the merged directive set with canonical serialization.

Pure deterministic merge — no LLM, no `Task`/`Agent` dispatch, no shell-out. A static-analysis test (`tools/edikt/check/no-llm-in-merge.sh`) verifies that no LLM-dispatch symbol is reachable from the Phase B code path. The check runs in CI.

**Latency budget** (preserved from ADR-020):

| Mode | Budget |
|---|---|
| Full regenerate from cold cache (50 sidecars) | `<5s` |
| No-op (all sidecars unchanged) | `<500ms` |
| `--check` mode | `<2s` |

**Diff-only rendering:** topic files carry a `_fingerprint:` field — a sorted SHA-256 of contributing sidecar paths and content hashes. If a fingerprint matches the existing file's, Phase B skips the rewrite. Modifying one sidecar therefore only rerenders its topic file.

#### The `_fingerprint:` field — stability contract

The `_fingerprint:` line in the YAML frontmatter of every `.claude/rules/governance/<topic>.md` is the short-circuit that gives Phase B its `<500ms` no-op budget. It is a sorted SHA-256 over the contributing sidecar paths and their canonical-marshaled content. Treat it as opaque, tool-owned bytes:

- **Do not hand-edit or strip the line.** Doing so forces a full re-render of that topic file on the next compile (correctness preserved; performance degraded for one run).
- **The field lives in the compiled topic file, not in the sidecar schema.** Sidecars carry source-of-truth content; the fingerprint is a derivation observable on the output side, which is where ADR-020's determinism guarantee binds.
- **The hash is canonicalized over the marshaled sidecar bytes.** Re-running compile with byte-equal input produces a byte-equal fingerprint (ADR-020 / ADR-028).
- **Why it's not in the sidecar:** the sidecar is human-reviewable structured data; embedding a hash of the rendered output in the input would couple the layers and break ADR-027's "edikt does not write to inputs" rule.

If you see fingerprints differing across runs with no apparent input change, file an issue — that's a determinism break, which is a bug per ADR-020.

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

`status` is `"ok"` on a successful run, `"error"` when compile exited non-zero (the run still completed enough to emit JSON; check `error` for the message). Phase A's `dispatched` and `stale` counts agree on a successful resync (every stale sidecar was dispatched). On Phase A failure, `phase_a.errors[]` lists each artifact that failed and `phase_b` is omitted (Phase B does not run when Phase A failed). Output shape is the contract per ADR-029 — exit codes carry status; output is for tier-2 → tier-2 piping or human consumption.

`--dry-run` is an alias for `--check` (added for parity with `migrate sidecars` and `verify` flag conventions).

## How it works

### Directive sentinels

Each governance document (ADR, invariant, guideline) can contain an LLM directive sentinel block:

```markdown
## Directives

[edikt:directives:start]: #
paths:
  - "**/adapters/redis/**"
  - "**/cache*"
scope:
  - implementation
  - design
directives:
  - Use allkeys-lru eviction policy for all Redis instances (ref: ADR-008)
  - Max key size: 1MB. Max TTL: 24h for data caches, 5min for ACL caches (ref: ADR-008)
reminders:
  - "Before configuring Redis → MUST use allkeys-lru eviction (ref: ADR-008)"
verification:
  - "[ ] Redis eviction policy is allkeys-lru (ref: ADR-008)"
[edikt:directives:end]: #
```

When sentinel blocks exist, compile reads them verbatim — no extraction, no distillation. This is full-fidelity compilation.

Run `/edikt:gov:review` to generate sentinel blocks for documents that don't have them.

### Fallback extraction

If a document has no sentinel block, compile falls back to extracting directives from the human content (the Decision section for ADRs, the Rule section for invariants). This is the v0.1.x behavior — it works but at reduced fidelity.

### Topic grouping

Directives from all sources are grouped by topic. All caching rules from different ADRs and guidelines merge into `governance/cache.md`. All database rules merge into `governance/database.md`. Each directive keeps its source reference.

### Three loading mechanisms

1. **`paths:` frontmatter** — Claude Code auto-loads the topic file when editing matching files. Platform-enforced, no reasoning step.
2. **`scope:` tags** — the routing table in `governance.md` maps activities (planning, design, review) to topic files. Claude matches its current task.
3. **Signal keywords** — the routing table lists domain keywords. Claude matches task context to signals.

## Output structure

```text
.claude/rules/
├── governance.md              ← index + invariants (always loaded)
├── governance/
│   ├── cache.md               ← all caching rules from all sources
│   ├── database.md            ← all DB rules from all sources
│   ├── multi-tenancy.md       ← all tenancy rules
│   └── architecture.md        ← cross-cutting decisions
```

### Governance index (governance.md)

Contains:
- **Non-Negotiable Constraints** — invariant directives at the top (primacy bias)
- **Routing table** — maps signals and scopes to topic files
- **Reminders** — pre-action interrupts aggregated from all sources: "Before writing SQL -> MUST include tenant_id." Capped at 10.
- **Verification Checklist** — grep-verifiable self-audit items Claude checks before finishing. Capped at 15.
- **Reminder: Non-Negotiable Constraints** — invariant directives restated at the bottom (recency bias)

The reminders and checklist are generated by `/edikt:invariant:compile` and `/edikt:adr:compile` as `reminders:` and `verification:` lists inside each sentinel block. `gov:compile` aggregates them.

### Topic files (governance/*.md)

Each contains:
- `paths:` frontmatter for auto-loading
- Full-fidelity directives with source references
- Source attribution comments

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
  Sentinel coverage: 6/8 documents (75%)
```

## Contradiction detection

Before writing, the command checks for contradictions:

```text
CONTRADICTION DETECTED
  ADR-001: "Claude Code only — no multi-tool support"
  ADR-007: "Support Cursor for rule distribution"

  Resolve before compiling. Supersede one or reconcile both.
```

Invariant conflicts are errors — invariants always win. ADR conflicts are warnings.

## Cross-reference validation

Every directive that references an ADR or invariant (`ref: ADR-NNN`, `ref: INV-NNN`) is verified against the actual source file. If the referenced identifier doesn't exist in the source document, the reference is stripped — preventing fabricated cross-references from reaching the compiled output.

This also runs in `/edikt:gov:review` when generating sentinel blocks.

## CI validation

```bash
/edikt:gov:compile --check
```

Reports contradictions, conflicts, sentinel coverage, and directive counts without writing any files.

## Migration from v0.1.x

If you have an existing flat `governance.md` from v0.1.x, running `/edikt:gov:compile` automatically migrates to the new format:

1. Detects the old format
2. Generates topic files from existing directives
3. Replaces the flat file with the new index
4. Reports what changed

For best results, run `/edikt:gov:review` first to generate directive sentinel blocks in your source documents.

## Orphan ADR detection (v0.5.0)

Compile detects ADRs with no directives and no `no-directives:` reason field.

**Warn-then-block semantics:**

1. First compile with an orphan ADR — warns with the ADR path and exits 0 (non-blocking).
2. Second consecutive compile with the same orphan (or a superset) — blocks with exit ≠ 0.

Resolve by:
- Adding a directive sentinel block to the ADR (`/edikt:adr:compile`)
- Or marking the ADR with `no-directives: <reason ≥ 10 chars>` in its frontmatter (for ADRs that are deliberately non-directive, e.g., purely contextual records)

**State persistence:**

Orphan state is tracked in `.edikt/state/compile-history.json` via atomic rename. The `.edikt/state/` directory is auto-appended to `.gitignore` — this is local machine state, not repo state.

## Directive quality checks (v0.5.0)

Before writing, compile invokes the shared directive-quality sub-procedure (`commands/gov/_shared-directive-checks.md`) — the same sub-procedure used by `/edikt:gov:review`. It covers:

- **FR-003a** — warns on multi-sentence directives without `canonical_phrases`
- **FR-003b** — warns when a `canonical_phrase` value does not appear in the directive body
- **`no-directives` reason validation** — if `no-directives:` is present, the reason must be ≥ 10 characters and not a placeholder (`tbd`, `todo`, `fix later`)

FR-003a is warn-only in v0.5.0. Hard-fail is targeted for the next release.

## Extended sentinel fields (v0.5.0)

The compile parser now reads two new optional fields from sentinel blocks:

- `canonical_phrases:` — forwarded into the compiled governance topic file verbatim; consumed by FR-003b checks
- `behavioral_signal:` — stored for `/edikt:gov:benchmark`; not included in `.claude/rules/` output

Missing fields are treated as `[]` / `{}` — fully backward-compatible.

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

- [/edikt:gov:review](/commands/gov/review) — generate directive sentinels and review language quality
- [/edikt:adr:new](/commands/adr/new) — capture an architecture decision
- [/edikt:invariant:new](/commands/invariant/new) — add a hard constraint
