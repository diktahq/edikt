# How Governance Compiles

edikt governance flows through a pipeline: you write decisions for humans, compile extracts directives for Claude, and Claude reads them every session. Understanding this pipeline is key to writing governance that actually gets followed.

## Two audiences, one source

Every governance document has two audiences:

| Surface | Audience | Purpose |
|---|---|---|
| The prose `.md` (Statement, Rationale, Consequences, Implementation, Anti-patterns, Enforcement) | **Humans** | Understand the decision, its context, why it exists, how to comply |
| The co-located `.edikt.yaml` sidecar | **Claude** | Short, imperative directives Claude reads and follows literally |

You write the prose `.md`. The compile pipeline generates the sidecar. **edikt only writes to sidecars and topic files — your prose `.md` is never modified by `gov:compile`.** That's the structural boundary introduced in v0.6.0 (ADR-027), replacing the v0.5.x in-body sentinel block. See [Sidecar Architecture](sidecar) for the full data model.

## The sentinel block (legacy, v0.5.x and earlier)

Before v0.6.0, every governance document carried an in-body directive sentinel block. It looked like this:

```markdown
## Enforcement

- grep for raw SQL outside repository/ — must return no results
- Every repository method has a test rejecting empty tenant

<!-- Directives populated by /edikt:invariant:compile -->
[edikt:directives:start]: #
source_hash: "a3b2c1..."
directives_hash: "9f8e7d..."
compiler_version: "0.3.0"
paths:
  - "**/*.go"
scope:
  - implementation
  - review
directives:
  - "Every SQL query MUST include `tenant_id` in the WHERE clause. No exceptions. (ref: INV-012)"
  - "Every `slog.Error` call MUST include `\"tenant_id\", tid`. No exceptions. (ref: INV-012)"
reminders:
  - "Before writing SQL → MUST include `tenant_id` in WHERE clause (ref: INV-012)"
verification:
  - "[ ] Every SQL query references `tenant_id` (ref: INV-012)"
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #
```

The block contains:

| Field | What it is | Who writes it |
|---|---|---|
| `directives:` | Auto-generated rules from the source document | Compile (Claude never sees the human prose — only these) |
| `reminders:` | Pre-action interrupts: "Before doing X → check Y" | Compile |
| `verification:` | Self-audit checklist items Claude checks before finishing | Compile |
| `manual_directives:` | Rules you add by hand that compile missed | You |
| `suppressed_directives:` | Auto-generated rules you rejected | You |
| `source_hash` | SHA-256 of the human content (detects body changes) | Compile |
| `directives_hash` | SHA-256 of the directives list (detects hand-edits) | Compile |

The compile pipeline owns `directives:`, `reminders:`, and `verification:`. You own `manual_directives:` and `suppressed_directives:`. Compile never touches your lists; you never need to touch compile's.

In v0.6.0, this in-body block is replaced by a co-located `<artifact>.edikt.yaml` sidecar. The schema collapses to a single `directives[]` array (per-directive `text` + `source_excerpt`); hashes are recomputed on read and never committed. See [Sidecar Architecture](sidecar). The migration tool lifts existing in-body blocks into sidecars — see [Sidecar Migration](/guides/sidecar-migration).

## How directives are generated

Generation runs in a forked subagent (`context: fork`) with a locked extraction prompt and `Read + Write` tools. Each artifact compiles in its own fresh context — there is no cross-artifact contamination. The dispatching commands are `/edikt:adr:new`, `/edikt:invariant:new`, `/edikt:guideline:new`, the per-artifact `:compile` variants, and `/edikt:gov:compile` Phase A.

### From Invariant Records

The `## Statement` section is the source. Compile preserves the declarative, absolute language and transforms it into MUST/NEVER directives:

```
Statement (human):
  "Every data access is scoped to the authenticated tenant."

Directive (Claude):
  "Every data access MUST be tenant-scoped. No exceptions. (ref: INV-012)"
```

If the Statement uses absolute quantifiers ("every", "all", "total"), compile appends "No exceptions." automatically. This prevents Claude from rationalizing edge cases.

The `## Enforcement` section contributes additional directives when it names concrete mechanisms:

```
Enforcement (human):
  "Every slog.Info, slog.Warn, slog.Error call includes \"tenant_id\", tid."

Directive (Claude):
  "Every slog.Info, slog.Warn, slog.Error call MUST include \"tenant_id\", tid. No exceptions. (ref: INV-012)"
```

Literal code tokens in the Enforcement section flow directly into directives. The more specific your enforcement, the more effective the directive.

### From ADRs

The `## Decision` section is the source. Compile extracts every enforceable statement — anything that prescribes or prohibits a behavior:

```
Decision (human, 150 lines):
  "Build edikt as a lean context engine targeting Claude Code exclusively.
   Other tools lack path-conditional rules, hooks, slash commands..."

Directive (Claude, 1 line):
  "Claude Code is the only supported platform. NEVER target Cursor, Copilot, or other tools. (ref: ADR-001)"
```

### From guidelines

The `## Rules` section is the source. Guidelines already use MUST/NEVER language. Compile lifts each bullet into a directive. Soft language ("should", "prefer") is rejected with a warning.

## The three-list merge (legacy, v0.5.x)

In v0.5.x and earlier, when `/edikt:gov:compile` assembled the final governance.md, it read three lists from every source and merged them:

```
effective_rules = (directives - suppressed_directives) ∪ manual_directives
```

In v0.6.0 this collapses to a single `directives[]` array per sidecar. To suppress a generated rule, remove the source language from the prose body and re-run compile (or, for non-mutable prose, edit the sidecar after compile — `:review` will flag the divergence as drift). To add a rule compile missed, add an entry to `directives[]` with a `source_excerpt` quoting the prose line that justifies it. See [Sidecar Architecture](sidecar) for the full editing surface. ADR-008 (the original three-list schema) is superseded by [ADR-027](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-027-sidecar-architecture-for-governance-metadata.md).

## Two-phase compile (v0.6.0)

`/edikt:gov:compile` runs in two phases. The contract is in [ADR-028](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-028-two-phase-compile-resync-merge.md), which amends [ADR-020](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-020-gov-compile-tier-2-migration.md)'s latency budget.

### Phase A — Resync (conditional)

**Trigger:** one or more sidecars are stale. A sidecar is stale when the SHA-256 of its parent `.md`'s body no longer matches the sidecar's expected body hash (recomputed on read, never committed).

**Action:** dispatch parallel subagents to regenerate every stale sidecar. Concurrency 8 (semaphore-bound). Each subagent runs the same locked extraction prompt as `/edikt:<type>:compile`. Failures log to `.edikt/state/compile-errors.log` and don't abort the run; remaining subagents continue. After all subagents finish, an aggregated failure report is printed.

**Latency:** no SLO. Per-subagent latency is 30–60s p50 — that cost is real. The compiler emits per-subagent progress on stderr (artifact name, completed/total, ETA from running p50). Silent multi-minute operation is forbidden.

```text
Phase A — resyncing 3 stale sidecars
  ✓ ADR-001-claude-code-only           (12.4s)
  ✓ ADR-007-compile-schema-version     (18.1s)
  ⏳ ADR-022-single-go-binary-replaces… [▓▓▓░░░] ETA 22s
```

If any sidecar fails, Phase B does not run and compile exits 1 with the aggregated report.

### Phase B — Merge (always)

**Action:** read every sidecar, validate against the schema, group by topic, render `.claude/rules/governance/<topic>.md`. This phase is a pure deterministic merge. No LLM, no `Task`/`Agent` dispatch, no shell-out.

**Static enforcement:** a static-analysis test (`tools/edikt/check/no-llm-in-merge.sh`) verifies that no `Agent` / `Task` / subprocess-spawning symbol is transitively reachable from the Phase B code path. The check is wired into CI so any drift fails the build.

**Latency budget** (preserved from ADR-020):

| Mode | Budget |
|---|---|
| Full regenerate from cold cache (50 sidecars) | `<5s` |
| No-op (all sidecars unchanged) | `<500ms` |
| `--check` mode | `<2s` |

Phase B writes topic files atomically (tmp → rename). Topic files carry a `_fingerprint:` field — a sorted SHA-256 of contributing sidecar paths and content hashes. If a fingerprint matches the existing file's, Phase B skips the rewrite. Modifying one sidecar therefore only rerenders its topic file; every other topic file is byte-equal across compiles.

### `--check` mode

```bash
/edikt:gov:compile --check
```

`--check` skips Phase A entirely. If any sidecar is stale, it exits 1 with a single-line actionable error directing the user to run `gov:compile`. CI gates run `--check`. Because it never dispatches a subagent, it is deterministic and fast.

### Concurrent compile

Compile takes an advisory file-lock at `.edikt/state/compile.lock`. A second invocation while one is running waits by default, or fails fast with `--no-wait`.

## What governance.md looks like

The compiled output has five sections:

```markdown
# Governance Directives

Follow these directives in every file you write or edit.

## Non-Negotiable Constraints

These are invariants. Violation is never acceptable.

- Every SQL query MUST include `tenant_id`. No exceptions. (ref: INV-012)
- Every command MUST be a `.md` or `.yaml` file. No exceptions. (ref: INV-001)

## Routing Table

| Signals | Scope | File |
|---|---|---|
| cache, redis, TTL | implementation | `governance/cache.md` |
| database, SQL, migration | implementation, review | `governance/database.md` |

## Reminders

Before acting, check the relevant constraint.

- Before writing SQL → MUST include `tenant_id` in WHERE clause (ref: INV-012)
- Before creating a file → which architecture layer does it belong to? (ref: ADR-003)
- Before returning an error → NEVER return `err.Error()` to client (ref: SEC-001)

## Verification Checklist

Before finishing, verify. If any fails, fix before submitting.

- [ ] Every SQL query references `tenant_id` (ref: INV-012)
- [ ] No SQL outside `internal/repository/` (ref: ADR-003)
- [ ] Every `slog.*` call includes `"tenant_id"` (ref: INV-012)

## Reminder: Non-Negotiable Constraints

These constraints were listed above and are restated for emphasis.
Do not violate them under any circumstances.

- Every SQL query MUST include `tenant_id`. No exceptions. (ref: INV-012)
- Every command MUST be a `.md` or `.yaml` file. No exceptions. (ref: INV-001)
```

Invariants appear at the top (primacy bias) and bottom (recency bias). Reminders fire before each action. The checklist is a final self-audit.

## Why the format matters

Pre-registered experiments on Claude Opus 4.6 showed that directive format changes whether Claude follows governance:

| Format | Compliance |
|---|---|
| Prose: "Log calls should include tenant context" | Partial — Claude misses some log calls |
| Compiled: `Every slog.Error MUST include "tenant_id", tid. No exceptions.` | Full — Claude includes tenant on every call |

The difference: MUST/NEVER language, literal code tokens Claude can type directly, and "No exceptions." reinforcement. The compile pipeline produces this format automatically from your human-readable source documents.

Use `/edikt:gov:score` to verify your governance follows these patterns.

## Hash-based caching

Phase A doesn't dispatch a subagent when a sidecar is fresh. The decision gate is recomputing the body hash and comparing it against the sidecar's expected body hash. If they match, the sidecar is fresh and Phase A skips it.

The hash is **never committed**. It's recomputed on every read. This means a sidecar's freshness is always evaluated against the current `.md`, not against a stale snapshot — which kills a class of bugs where a stale committed hash "agreed with itself."

## Migration from in-body sentinels

If your project still has `[edikt:directives:start]` blocks inside `.md` files (v0.5.x or earlier), `/edikt:gov:compile` refuses to run until you migrate. Run `/edikt:upgrade` and accept the migration prompt; or run `edikt migrate sidecars --dry-run` followed by `--apply` directly. See [Sidecar Migration](/guides/sidecar-migration) for the walkthrough.

## Commands

| Command | What it does |
|---|---|
| `/edikt:adr:new` | Create the `(ADR.md, ADR.edikt.yaml)` pair atomically |
| `/edikt:invariant:new` | Create the `(INV.md, INV.edikt.yaml)` pair atomically |
| `/edikt:guideline:new` | Create the `(guideline.md, guideline.edikt.yaml)` pair atomically |
| `/edikt:adr:compile <id>` | Regenerate exactly one ADR sidecar |
| `/edikt:invariant:compile <id>` | Regenerate exactly one invariant sidecar |
| `/edikt:guideline:compile <id>` | Regenerate exactly one guideline sidecar |
| `/edikt:gov:compile` | Phase A resync (conditional) + Phase B merge (deterministic) |
| `/edikt:gov:compile --check` | Phase B only; exit 1 on stale sidecars |
| `/edikt:gov:score` | Score the compiled output for LLM compliance |
| `/edikt:gov:review` | Review for contradictions and language quality |

The typical flow: write a decision → `:new` creates the prose + sidecar → edit the prose → `:compile` regenerates the sidecar → `gov:compile` rebuilds topic files.
