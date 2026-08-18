# How Governance Compiles

edikt governance flows through a pipeline: you write decisions for humans, compile extracts directives for the model, and the model reads them every session. Understanding this pipeline is key to writing governance that actually gets followed.

**At a glance:**

- You write prose for humans (an ADR, invariant, or guideline). Compile generates a co-located `.edikt.yaml` sidecar for the model — it never edits your prose.
- `/edikt:gov:compile` runs in two phases: **Phase A** regenerates any stale sidecar (the only step that calls an LLM), **Phase B** deterministically renders the output — no LLM, always runs.
- Phase B produces four surfaces: an **ambient core** loaded on every edit, **topic files** loaded when a matching path is touched, a **directive index** that drives write-time enforcement, and a **manifest**.
- A `must`-grade directive blocks the write before it happens. An `advisory`-grade one lets the write through and adds context after.

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
- **Scoped topic files** — `.claude/rules/governance/<topic>.md`, one per topic, loaded automatically when the frontmatter `paths:` glob matches a touched file. Each carries a `Directives`/`Prohibitions` region (may be empty — see [the single-home rule](#the-single-home-rule-scoped-directive-text-renders-exactly-once) below), a hand-authored `ManualDirectives` region (may also be empty), and always-present `Reminders` and `Verification Checklist` sections built from that topic's directives. A topic whose contributing sidecars declare no `paths:` at all renders no topic file — it's reached only through its skill package instead.
- **The directive index** — `.claude/rules/governance/directive-index.yaml`. A single glob-keyed YAML file, not meant for ambient reading — it is `bin/edikt hook match`'s exclusive input, read by the write-time hook shims (see [The write-time split](#the-write-time-split-what-happens-to-a-directive-when-a-write-happens) below). Each entry under a glob key carries an `id`, a pinned `grade` (`must` or `advisory`, decided once at compile time — consumers never re-derive it), the literal directive `text`, a `falsifying_observation`, and its `reminders`. A single directive can appear under every glob it matches, not once per directive.
- **The manifest** — `.claude/rules/governance/manifest.yaml`. Lists every surface this compile produced, each with the SHA-256 of its bytes, and nothing else — no timestamp, deliberately, so the manifest is byte-identical for byte-identical input and freshness checking can tell stale from recompiled. It cannot hash itself and omits itself from its own surface list.

A reader can tell which surface they're looking at from its shape alone:

- Ambient core — `paths: "**/*"`, almost no body.
- Topic file — a topic-scoped `paths:` list, a `Directives`/`Prohibitions`/`ManualDirectives` triad, always a Reminders + Verification Checklist tail.
- Directive index — pure YAML keyed by glob, never prose.
- Manifest — a flat list of `path` + `kind` + `sha256` triples.

Skill packages (`.claude/skills/edikt-<topic>/SKILL.md`) are a fifth thing worth knowing about, even though they're not one of the four rendered surfaces above — they're a trigger-loaded surface, read in full on task-language selection rather than passively included by path, and it's where a fully-unscoped topic's directives live when no topic file exists to carry them. They're still listed in `manifest.yaml` (`kind: "skill-package"`, one entry per topic).

No rendered surface carries a "compiled at" timestamp field — a timestamp would turn every no-op compile into a diff, so it was deliberately left out to keep output byte-deterministic.

## The single-home rule: scoped directive text renders exactly once

A scoped directive has two possible homes — its topic file, and `directive-index.yaml` — and without a rule pinning it to exactly one, it can end up delivered by both. Measured directly on a real 50-sidecar project: 78 of 110 directive lines in that project's topic files were exact duplicates of entries already in `directive-index.yaml`, both surfaces scoped to overlapping `paths:` globs and both firing on the same edit.

The two surfaces aren't accidental duplicates — each is an independent delivery channel with its own contract:

- **`directive-index.yaml`** — `hook match`'s exclusive input. Fires precisely on the file being touched, matched per-directive against that directive's own glob. Drives the write-time hooks: a synchronous PreToolUse deny for `must`-grade directives, a PostToolUse `additionalContext` for `advisory`-grade ones.
- **Topic file** — the native ambient rule-loading surface. The whole file loads into context whenever a touched path matches the topic's `paths:` frontmatter, independent of the hook — passive background context, not a guaranteed synchronous delivery.

When a scoped directive is delivered by both, the second delivery adds token cost without adding coverage, since the hook's guarantee is strictly the stronger of the two.

**The precise rule: "scoped" means the sidecar declares a `paths:` field.** A sidecar's `Directives`/`Prohibitions` text renders into its topic file's compiled-directives region only when the sidecar declares no `paths:`. A sidecar that *does* declare `paths:` sends its directive and prohibition text to `directive-index.yaml` alone — the topic file's compiled-directives region omits it entirely.

One consequence worth knowing: a topic file whose contributing sidecars all declare `paths:` renders an *empty* compiled-directives region — open/close sentinel markers with nothing between them. That's the rule working as designed on a corpus where scoping is thorough, not a bug or a sign the corpus is thin.

Two things are unaffected by this rule either way:
- **Pathless/unscoped directive text** still renders into the topic file's body — a sidecar with no `paths:` has no glob for the hook to key on, so the topic file is the only place it can reach a reader.
- **`ManualDirectives`** — hand-authored directly into the sidecar, never extracted, no other delivery channel — always render into the topic file's manual region regardless of whether that sidecar also declares `paths:`.

## The write-time split: what happens to a directive when a write happens

Schema 3's ambient-token reduction opens a gap: a directive scoped to, say, `tools/**/*.go` only reaches a reader when the host happens to load that topic file, which depends on the host's own rules-loading behavior, not on the write actually happening. That's acceptable for an advisory rule. It is not acceptable for a `must`-grade rule, because the entire point of an invariant is that it applies whether or not anyone remembered to load it.

The answer is a channel that fires *because a write is happening*, keyed on the file being written, independent of ambient loading — two hooks over one matcher, split by grade:

| Grade | Hook | Shape |
|---|---|---|
| `must` | PreToolUse | **deny**, naming the matched directive; the write does not happen |
| `advisory` | PostToolUse | `additionalContext`; the write already happened |

A `must`-grade directive is enforced *before* the write completes: the PreToolUse hook blocks the tool call with a deny that names the specific matched directive, and the write does not go through until the agent complies. An `advisory`-grade directive is delivered *after* the write completes: the PostToolUse hook injects `additionalContext` carrying the directive text, but nothing was blocked — the write already happened, and the injected text is guidance for what comes next, not a gate on what just occurred.

This split is grounded in a direct behavioral measurement: advisory text appended after the fact was read and largely ignored, while a deny naming the rule and forcing a retry was complied with. The two paths are kept disjoint deliberately — a `must` rule that also appeared in advisory context would be delivered twice, and the weaker of the two deliveries is the one an agent learns to route around.

The grade itself (`must` vs. `advisory`) is decided once, at compile time, and pinned directly into `directive-index.yaml`'s `grade` field — consumers of the index never re-derive it. Grade derivation is keyed off the directive's actual modal verb, not a keyword scan.

The write-time tier is deliberately fail-open rather than fail-closed: this code runs inside PreToolUse, and failing closed would mean a governance bug blocks every write in the user's editor until someone diagnoses it — a worse outcome than a missed injection.

Fail-open must never be *silent*, though. These all produce the exact same observable as a correct "no directive applies here" pass — a write that proceeds with nothing injected:

- The binary is absent
- The index is missing
- The index is corrupt
- The path is unresolvable

So every invocation is journaled with a distinctly-typed outcome, even when the observable behavior looks identical.

This tier only covers `file_path`-bearing tool calls. A write performed through Bash (redirection, `tee`, in-place stream editing) carries no `file_path` and is not matched by this tier at all — a named and accepted gap, not a silent one.

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
