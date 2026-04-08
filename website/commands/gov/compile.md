# /edikt:gov:compile

Compile accepted ADRs, active invariants, and team guidelines into topic-grouped governance rule files.

The output — `.claude/rules/governance.md` (index) and `.claude/rules/governance/*.md` (topic files) — is read by Claude automatically every session. Each topic file contains full-fidelity directives for a specific domain, loaded only when relevant files are being edited.

## Usage

```bash
/edikt:gov:compile
/edikt:gov:compile --check
```

## Arguments

| Argument | Description |
|----------|-------------|
| (none) | Compile and write governance index + topic rule files |
| `--check` | Validate only — report contradictions and conflicts without writing |

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

Contains only:
- Invariants (always loaded, universal, non-negotiable)
- Routing table mapping signals and scopes to topic files
- Invariant reminder at the bottom (recency reinforcement)

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

## When to run

Run after:
- Capturing a new ADR with `/edikt:adr:new`
- Adding an invariant with `/edikt:invariant:new`
- Updating a guideline file
- Running `/edikt:gov:review` to generate or update sentinels

## What's next

- [/edikt:gov:review](/commands/gov/review) — generate directive sentinels and review language quality
- [/edikt:adr:new](/commands/adr/new) — capture an architecture decision
- [/edikt:invariant:new](/commands/invariant/new) — add a hard constraint
