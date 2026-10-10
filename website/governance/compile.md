# How Governance Compiles

edikt governance flows through a pipeline: you write decisions for humans, compile extracts directives for the model, and the model reads them every session. Understanding this pipeline is key to writing governance that actually gets followed.

**At a glance:**

- You write prose for humans (an ADR, invariant, or guideline). Compile generates a co-located `.edikt.yaml` sidecar for the model — it never edits your prose.
- `/edikt:gov:compile` runs in two phases: **Phase A** regenerates any stale sidecar (the only step that calls an LLM), **Phase B** deterministically renders the output — no LLM, always runs.
- Phase B produces four surfaces: an **ambient core** loaded on every edit, **topic files** loaded when a matching path is touched, a **delivery map** recording which directive governs which path, and a **manifest**.
- Every directive reaches the model *before* it writes, through the core and the topic files. Grade (`must` vs. `advisory`) is derived from the directive's own modal force and still decides which surface carries it; the finished diff is reviewed against every directive at phase end.

The rest of this page is the detail behind each of those four points.

## Two audiences, one source

Every governance document has two audiences:

| Surface | Audience | Purpose |
|---|---|---|
| The prose `.md` (Statement, Rationale, Consequences, Implementation, Anti-patterns, Enforcement) | **Humans** | Understand the decision, its context, why it exists, how to comply |
| The co-located `.edikt.yaml` sidecar | **The model** | Short, imperative directives the model reads and follows literally |

You write the prose `.md`. The compile pipeline generates the sidecar. **edikt only writes to sidecars and topic files — your prose `.md` is never modified by `gov:compile`.** That's the structural boundary introduced in v0.6.0, replacing the legacy in-body sentinel block. See [Sidecar Architecture](sidecar) for the full data model.

## The sentinel block (legacy in-body format)

Before v0.6.0, every governance document carried its directives in an in-body `[edikt:directives:start]`/`[edikt:directives:end]` block instead of a sidecar — the same lists (`directives:`, `reminders:`, `verification:`, `manual_directives:`, `suppressed_directives:`), embedded in the prose file itself rather than co-located next to it. Sentinels haven't disappeared from edikt — the rendered output surfaces (topic files, `governance.md`, and the managed block in `CLAUDE.md`) still use them to bound the region compile owns — but source documents (ADRs, invariants, guidelines) no longer carry one. See [Sentinel Blocks](sentinels) for the full legacy format and field reference, and [Sidecar Migration](/guides/sidecar-migration) to migrate a pre-v0.6 project.

## How directives are generated

Generation runs in a forked subagent (`context: fork`) with a locked extraction prompt and `Read + Write` tools. Each artifact compiles in its own fresh context — there is no cross-artifact contamination. The dispatching commands are `/edikt:adr:new`, `/edikt:invariant:new`, `/edikt:guideline:new`, the per-artifact `:compile` variants, and `/edikt:gov:compile` Phase A.

### From Invariant Records

The `## Statement` section is the source. Compile preserves the declarative, absolute language and transforms it into MUST/NEVER directives:

```
Statement (human):
  "Every data access is scoped to the authenticated tenant."

Directive (the model):
  "Every data access MUST be tenant-scoped. No exceptions. (ref: INV-012)"
```

If the Statement uses absolute quantifiers ("every", "all", "total"), compile appends "No exceptions." automatically. This prevents the model from rationalizing edge cases.

The `## Enforcement` section contributes additional directives when it names concrete mechanisms:

```
Enforcement (human):
  "Every slog.Info, slog.Warn, slog.Error call includes \"tenant_id\", tid."

Directive (the model):
  "Every slog.Info, slog.Warn, slog.Error call MUST include \"tenant_id\", tid. No exceptions. (ref: INV-012)"
```

Literal code tokens in the Enforcement section flow directly into directives. The more specific your enforcement, the more effective the directive.

### From ADRs

The `## Decision` section is the source. Compile extracts every enforceable statement — anything that prescribes or prohibits a behavior:

```
Decision (human, 150 lines):
  "Build edikt as a lean context engine for coding harnesses, starting
   with Claude Code — it has path-conditional rules, hooks, and slash
   commands other harnesses still lack..."

Directive (the model, 1 line):
  "Every public API route MUST be versioned (/v1/, /v2/). NEVER break a published contract. (ref: ADR-001)"
```

### From guidelines

The `## Rules` section is the source. Guidelines already use MUST/NEVER language. Compile lifts each bullet into a directive. Soft language ("should", "prefer") is rejected with a warning.

## The three-list merge (legacy)

In the legacy format, when `/edikt:gov:compile` assembled the final governance.md, it read three lists from every source and merged them:

```
effective_rules = (directives - suppressed_directives) ∪ manual_directives
```

In v0.6.0 this collapses to a single `directives[]` array per sidecar. To suppress a generated rule, remove the source language from the prose body and re-run compile (or, for non-mutable prose, edit the sidecar after compile — `:review` will flag the divergence as drift). To add a rule compile missed, add an entry to `directives[]` with a `source_excerpt` quoting the prose line that justifies it. See [Sidecar Architecture](sidecar) for the full editing surface.

## Two-phase compile (v0.6.0)

`/edikt:gov:compile` runs in two phases.

### Phase A — Resync (conditional)

**Trigger:** one or more sidecars are stale. A sidecar is stale when the SHA-256 of its parent `.md`'s body no longer matches the sidecar's expected body hash (recomputed on read, never committed).

**Action:**
- Dispatches parallel subagents (concurrency 8) to regenerate every stale sidecar, each running the same locked extraction prompt as `/edikt:<type>:compile`.
- A subagent failure logs to `.edikt/state/compile-errors.log` without aborting the run — the rest keep going.
- Once everything finishes, an aggregated failure report prints.

**Latency:** no SLO. Per-subagent latency is 30–60s p50 — that cost is real, so the compiler emits per-subagent progress on stderr (artifact name, completed/total, ETA from running p50) rather than running silently for minutes.

```text
Phase A — resyncing 3 stale sidecars
  ✓ ADR-001-api-versioning           (12.4s)
  ✓ ADR-007-caching-strategy     (18.1s)
  ⏳ ADR-022-rate-limiting… [▓▓▓░░░] ETA 22s
```

If any sidecar fails, Phase B does not run and compile exits 1 with the aggregated report.

### Phase B — Merge (always)

**Action:** read every sidecar, validate against the schema, group by topic, and render the four output surfaces of `compile_schema_version: 3` (see [The four-surface render](#the-four-surface-render-compile-schema-version-3) below). This phase is a pure deterministic merge — no LLM, no `Task`/`Agent` dispatch, no shell-out.

**Static enforcement:** a static-analysis test (`tools/edikt/check/no-llm-in-merge.sh`) verifies that no `Agent` / `Task` / subprocess-spawning symbol is transitively reachable from the Phase B code path. The check is wired into CI so any drift fails the build.

**Latency budget:**

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

## The four-surface render (`compile_schema_version: 3`)

`compile_schema_version: 3` is the current output-format contract of `/edikt:gov:compile` — a version field tooling can check to confirm compatibility, independent of edikt's own release version. Schema 3 replaced a single always-loaded `governance.md` with four distinct rendered surfaces, each doing one job:

- **Ambient core** — `.claude/rules/governance.md`. Loaded on every edit (`paths: "**/*"`). It carries only pathless-invariant canonical statements — the handful of rules that hold everywhere, no exceptions, regardless of what file is touched — plus a one-line topic index. No routing table, no aggregate reminders, no verification checklist.

  Each topic's line in that index is its **approved registry description**, and a description is capped at **160 characters**. Approving a longer one is refused with `error: description is N chars (max 160)`. The cap exists because this file loads on every edit in every session: 160 characters is roughly the 25-token per-topic budget the ambient index is priced at. A description that will not fit usually signals a topic that should be split rather than compressed.
- **Scoped topic files** — `.claude/rules/governance/<topic>.md`, one per topic, loaded automatically when the frontmatter `paths:` glob matches a touched file. Each carries a `Directives`/`Prohibitions` region (may be empty, meaning a topic with no directives — see [the single-home rule](#the-single-home-rule-scoped-directive-text-renders-exactly-once) below), a hand-authored `ManualDirectives` region (may also be empty), and always-present `Reminders` and `Verification Checklist` sections built from that topic's directives. A topic whose contributing sidecars declare no `paths:` at all renders no topic file — it's reached only through its skill package instead.
- **The delivery map** — `.claude/rules/governance/delivery-map.yaml`. A single glob-keyed YAML file, not meant for ambient reading and deliberately carrying no directive text: it is the routing record, not a delivery surface. Each entry under a glob key names a directive's `id`, its pinned `grade` (`must` or `advisory`, decided once at compile time — consumers never re-derive it), its `class`, its `topic`, and the `surface` that delivers it. It is the one source of path→directive scope, read by `bin/edikt hook match`, `edikt gov scope`, the session tally and the prune counter. A single directive appears under every glob it matches, not once per directive.
- **The manifest** — `.claude/rules/governance/manifest.yaml`. Lists every surface this compile produced, each with the SHA-256 of its bytes, and nothing else — no timestamp, deliberately, so the manifest is byte-identical for byte-identical input and freshness checking can tell stale from recompiled. It cannot hash itself and omits itself from its own surface list.

A reader can tell which surface they're looking at from its shape alone:

- Ambient core — `paths: "**/*"`, almost no body.
- Topic file — a topic-scoped `paths:` list, a `Directives`/`Prohibitions`/`ManualDirectives` triad, always a Reminders + Verification Checklist tail.
- Delivery map — pure YAML keyed by glob, never prose, and never directive text.
- Manifest — a flat list of `path` + `kind` + `sha256` triples.

Skill packages (`.claude/skills/edikt-<topic>/SKILL.md`) are a fifth thing worth knowing about, even though they're not one of the four rendered surfaces above — they're a trigger-loaded surface, read in full on task-language selection rather than passively included by path, and it's where a fully-unscoped topic's directives live when no topic file exists to carry them. They're still listed in `manifest.yaml` (`kind: "skill-package"`, one entry per topic).

No rendered surface carries a "compiled at" timestamp field — a timestamp would turn every no-op compile into a diff, so it was deliberately left out to keep output byte-deterministic.

## The single-home rule: scoped directive text renders exactly once

A compiled directive has exactly one home. It renders into its topic file — the file the host loads whenever a touched path matches that topic's `paths:` frontmatter — or, when a topic's contributing sidecars declare no globs at all and it therefore has no rules file, into that topic's skill package. Nowhere else. The ambient core carries only the pathless-invariant statements that hold on every edit, and `delivery-map.yaml` carries routing, not text.

The rule earns its keep because the alternative is measurable duplication. Measured directly on a real 50-sidecar project: 78 of 110 directive lines in that project's topic files were exact duplicates of entries in a second surface, both scoped to overlapping `paths:` globs and both firing on the same edit. A directive delivered twice costs tokens twice and covers nothing extra.

**The precise rule: a sidecar's `Directives`/`Prohibitions` text renders into its topic file's compiled-directives region, whether or not the sidecar declares `paths:`.** A topic file carries every directive of its topic.

**What "one home" meant through v0.8.x, and why it changed.** ADR-066 drew the line elsewhere: a sidecar that declared `paths:` sent its directive and prohibition text to `directive-index.yaml` and *only* there, to be delivered by a hook at the moment of a matching edit, leaving the topic file's compiled-directives region empty. That reading rested on the edit-time channel being the stronger of the two deliveries. ADR-083 retired the channel on the measurement below, so the surviving delivery is now the only delivery: the 482 path-scoped entries that lived in the index on edikt's own corpus were re-homed into their topic files, and the index is no longer rendered at all.

One consequence worth knowing, and it inverted with that change: an empty compiled-directives region used to mean "this topic's directives are all path-scoped and live in the index." It now means what it looks like — a topic with no directives.

Two things are unaffected either way:
- **Pathless/unscoped directive text** renders into the topic file's body, as it always did.
- **`ManualDirectives`** — hand-authored directly into the sidecar, never extracted, no other delivery channel — always render into the topic file's manual region.

## How a directive reaches the model

Schema 3's ambient-token reduction opens a gap: a directive scoped to, say, `tools/**/*.go` only reaches a reader when the host loads that topic file. edikt closes it structurally — the topic file's `paths:` frontmatter is what the host matches against the file being worked on, so the rule is in context *before* an edit is attempted, which is the only point at which a rule can still change what gets written.

| Surface | When it loads | What it carries |
|---|---|---|
| Ambient core | Every edit | The pathless invariants that hold everywhere, plus a one-line topic index |
| Topic file | The host matches a touched path against its `paths:` frontmatter | Every directive of that topic, its Reminders, its Verification Checklist |
| Skill package | The task language selects it — before anything has been touched | A topic with no scoped rules file, in full |
| `delivery-map.yaml` | Never — it is not a delivery surface | Routing only: which directive governs which path, at which grade, through which surface |

Grade survives all of this and still means what it meant. `must` is a guardrail and `advisory` is context; grade is derived once, at compile time, from the directive's actual modal force rather than a keyword scan, and it is pinned onto the delivery-map entry where the phase-end verifier reads it. Consumers never re-derive it. What grade no longer selects is a *moment* of enforcement.

### The channel that was retired

Through v0.8.x a second channel fired *because* an edit was happening: `bin/edikt hook match` read `directive-index.yaml`, and a `must`-grade match became a synchronous PreToolUse refusal naming the matched directive, while an `advisory`-grade one became PostToolUse `additionalContext` after the fact. **ADR-083 retired it**, and the measurement behind the retirement is blunt. Against a no-governance floor where 9 of 9 benchmark subjects finished, and an ambient-only arm where 7 of 7 finished, the arm carrying both surfaces finished **1 of 9**. One migration edit matched 25 rules and 8.9–33.5 KB of payload, roughly 60 % of it duplicating the always-loaded core the model already had, opening on a currency-arithmetic rule with nothing to do with the change. The channel was bypassable by construction, too: its matcher was `Write|Edit`, so when an agent was refused an `Edit` to a governed file it completed the identical edit through `Bash` and nothing saw it.

So nothing is injected as an edit lands, and **an edit that violates a rule is not stopped, not warned about, and not annotated at that moment.** Verification moved to the point where the work is whole enough to judge: `/edikt:gov:verify-diff` forks a read-only `governance-verifier` over the finished diff and the compiled directives and returns a per-directive `PASS` / `FAIL` / `NEEDS_REVIEW`, and post-flight runs it as its L2 layer at phase end. ADR-083 names it the only verification surface.

`PreToolUse` still runs, and it keeps exactly one job — the same one it had before, untouched by the retirement: the INV-005 managed-region guard. It refuses an `Edit` or `Write` that would overwrite bytes edikt generated — the managed blocks in `CLAUDE.md` and `settings.json`, and the wholly-generated compiled surfaces — and for a write arriving through `Bash`, which the hook never sees, an integrity sidecar notices the rewritten region afterwards instead. That is edikt guarding its own files. It is not a rule check on your code, and it does not grow into one.

## What the retired schema-2 render looked like

Before schema 3, edikt compiled to a single always-loaded `.claude/rules/governance.md` carrying every directive, every time, for every edit. That file was 657 directives across four fixed sections, in this order, closed with a fifth block that restated the first section a second time:

```markdown
# Governance Directives

## Non-Negotiable Constraints
- Every SQL query MUST include `tenant_id`. No exceptions. (ref: INV-012)

## Routing Table

| Signals | Scope | File |
|---|---|---|
| cache, redis, TTL | implementation | `governance/cache.md` |
| database, SQL, migration | implementation, review | `governance/database.md` |

## Reminders
- Before writing SQL → MUST include `tenant_id` in WHERE clause (ref: INV-012)

## Verification Checklist
- [ ] Every SQL query references `tenant_id` (ref: INV-012)

## Reminder: Non-Negotiable Constraints
- Every SQL query MUST include `tenant_id`. No exceptions. (ref: INV-012)
```

All five sections loaded on every single edit, regardless of relevance — including the bottom block, which just restated the first section again "for emphasis." The Routing Table alone ran to roughly 629 keyword terms (~3,119 estimated tokens) and was measurably less precise than what replaced it.

| Schema-2 section | Replaced by | What changed |
|---|---|---|
| Non-Negotiable Constraints | Ambient core's single constraint | Same role, but no longer bundled with three other always-loaded sections |
| Routing Table (629 keyword terms) | One-line topic index + automatic `paths:` loading + skill-package matching | Keyword matching → structural path matching |
| Reminders (aggregate, always loaded) | Per-topic Reminders section | Scope shrank from "every edit" to "edits that match this topic" |
| Verification Checklist (aggregate, always loaded) | Per-topic Verification Checklist section | Same shrink — content didn't disappear, its scope did |
| Bottom restatement block | *(nothing)* | Dropped outright — the ambient core states its constraints exactly once |

Any description of a single `governance.md` with a routing table, or of directives as always loaded regardless of the file being touched, is describing this retired model.

## Why the format matters

Pre-registered experiments on Claude Opus 4.6 (edikt's current harness) showed that directive format changes whether the model follows governance:

| Format | Compliance |
|---|---|
| Prose: "Migrations should include a rollback step" | Partial — the model ships some migrations with no `down()` |
| Compiled: `Every migration file MUST define down(). NEVER ship a migration with no rollback path.` | Full — the model writes the rollback every time |

The difference: MUST/NEVER language, literal code tokens the model can type directly, and "No exceptions." reinforcement. The compile pipeline produces this format automatically from your human-readable source documents.

Use `/edikt:gov:score` to verify your governance follows these patterns.

## Hash-based caching

Phase A doesn't dispatch a subagent when a sidecar is fresh. The decision gate is recomputing the body hash and comparing it against the sidecar's expected body hash. If they match, the sidecar is fresh and Phase A skips it.

The hash is **never committed**. It's recomputed on every read. This means a sidecar's freshness is always evaluated against the current `.md`, not against a stale snapshot — which kills a class of bugs where a stale committed hash "agreed with itself."

## Migration from in-body sentinels

If your project still has `[edikt:directives:start]` blocks inside `.md` files (legacy format), `/edikt:gov:compile` refuses to run until you migrate. Run `/edikt:upgrade` and accept the migration prompt; or run `edikt migrate sidecars --dry-run` followed by `--apply` directly. See [Sidecar Migration](/guides/sidecar-migration) for the walkthrough.

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
