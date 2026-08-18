# Sentinel Blocks (legacy in-body format)

> **Superseded by sidecars in v0.6.0.** As of v0.6.0, generated directives live in a co-located `<artifact>.edikt.yaml` **sidecar** — not in an in-body sentinel block. See [Sidecar Architecture](sidecar) for the current contract. `/edikt:gov:compile` refuses to run on a project that still has in-body `[edikt:directives:start]` blocks; run `/edikt:upgrade` (or `edikt migrate sidecars`) to migrate. This page is preserved for projects on a pre-v0.6 layout, and for understanding the shape the migration lifts away from.

## What a sentinel block looks like

```markdown
[edikt:directives:start]: #
source_hash: "a3b2c1d0..."
directives_hash: "9f8e7d6c..."
compiler_version: "0.3.0"
paths:
  - "**/*.go"
  - "**/repository/**"
scope:
  - implementation
  - review
directives:
  - "Every SQL query MUST include `tenant_id`. No exceptions. (ref: INV-012)"
  - "NEVER write raw SQL outside `internal/repository/`. (ref: INV-012)"
reminders:
  - "Before writing SQL → MUST include `tenant_id` in WHERE clause (ref: INV-012)"
verification:
  - "[ ] Every SQL query references `tenant_id` (ref: INV-012)"
manual_directives:
  - "All new tables MUST include a `created_at` timestamp column (ref: team convention)"
suppressed_directives: []
[edikt:directives:end]: #
```

The block uses Markdown link reference definitions (`[edikt:directives:start]: #`) as the sentinel markers — chosen because they are valid Markdown that renders as nothing (invisible to readers, parseable by tools), unlike HTML comments which Claude Code v2.1.72+ hides from the model.

## Optional sentinel fields: `canonical_phrases` and `behavioral_signal`

> **Legacy placement, current consumer.** Both are top-level keys of the sentinel block — siblings of `directives:`, not properties of an individual directive. `gov-sidecar.v2` has no equivalent: its schema sets `additionalProperties: false` at both the root and the directive level, so a sidecar carrying either key fails validation. `/edikt:gov:benchmark` and the multi-sentence directive-quality warning still read them from the legacy block, which is why they are documented here and not on [Sidecar Architecture](sidecar).

Both fields are backward-compatible — missing means absent, no default, and existing artifacts compile without changes.

### `canonical_phrases`

A list of words that a compliant model refusal must echo. Substring-matched, case-insensitive.

```yaml
canonical_phrases:
  - "immutable"
  - "never edit"
  - "create a new ADR"
```

**How it's used:**

- `/edikt:adr:review` warns if a listed phrase does not appear anywhere in the directive body. Useful as a quality check: if the phrase isn't in the body, the model is unlikely to echo it in refusals.
- `/edikt:gov:compile` warns when a directive has multiple sentences but no `canonical_phrases`. This is warn-only; the intent is to prompt you to run backfill.

**How to populate:**

- Run `/edikt:adr:review --backfill` for an interactive retrofit on existing ADRs. The command proposes 2–3 candidate phrases per directive via noun/verb heuristic; you approve or edit each with `[y/n/e]`.
- `/edikt:adr:new` includes three additional interview prompts to populate both new fields on new ADRs.

### `behavioral_signal`

A machine-testable predicate consumed by `/edikt:gov:benchmark`. Defines what a violation looks like. Four signal types:

| Type | YAML key | Violated when |
|---|---|---|
| Tool refusal | `refuse_tool: [TOOL1, TOOL2]` | Model calls any of the listed tools |
| File path refusal | `refuse_to_write: [".sql", "users.sql"]` | Model writes a path containing any listed substring (case-insensitive) |
| Citation requirement | `cite: [ADR-012]` | Model refusal text does not name any of the listed IDs |
| Frontmatter-gated edit refusal | `refuse_edit_matching_frontmatter: {path_glob, frontmatter_key, frontmatter_value}` | Model edits a file whose frontmatter matches the structural predicate |

**Example — generated config is read-only:**

```yaml
behavioral_signal:
  refuse_tool:
    - Write
    - Edit
canonical_phrases:
  - "generated file"
  - "do not edit"
  - "regenerate instead"
```

Directives without `behavioral_signal` are counted as `SKIP` in `/edikt:gov:benchmark` output. Populate it the same way as `canonical_phrases`, above.

### Where the two fields sit

Both together, in a block otherwise unchanged from the shape shown at the top of this page:

```yaml
[edikt:directives:start]: #
compiler_version: "0.5.0"
paths:
  - "specs/**"
directives:
  - "Published specs are frozen. NEVER edit a spec once its state is published. (ref: INV-008)"
canonical_phrases:
  - "frozen"
  - "never edit"
behavioral_signal:
  refuse_edit_matching_frontmatter:
    path_glob: "specs/**/*.md"
    frontmatter_key: "state"
    frontmatter_value: "published"
[edikt:directives:end]: #
```

---

## The five lists

### Compile-owned (read-only for users)

| List | What it contains |
|---|---|
| `directives:` | MUST/NEVER rules extracted from the source document |
| `reminders:` | Pre-action interrupts: "Before X → check Y" |
| `verification:` | Grep-verifiable checklist items |

These are regenerated every time the source body changes. If you hand-edit `directives:`, compile detects it via hash comparison and runs an interactive interview to resolve.

### User-owned (never touched by compile)

| List | What it contains |
|---|---|
| `manual_directives:` | Rules compile missed or couldn't infer. Always ship into governance.md. |
| `suppressed_directives:` | Auto-generated rules you want to reject. Always filtered out by gov:compile. |

These survive every recompilation. Compile never reads, modifies, or deletes them. See [Extensibility](extensibility) for usage examples.

## The three metadata fields

| Field | Purpose |
|---|---|
| `source_hash` | SHA-256 of the document body (excluding the sentinel block). Detects when the human content changes, triggering recompilation. |
| `directives_hash` | SHA-256 of the `directives:` list. Detects when you hand-edit auto-generated directives (triggers the interview flow). |
| `compiler_version` | Which edikt version wrote this block. Used to detect algorithm drift across upgrades. |

## Path and scope routing

| Field | How it's used |
|---|---|
| `paths:` | Glob patterns. Claude Code auto-loads the governance topic file when editing matching files. Derived by compile from the document's domain or pinned by the author. |
| `scope:` | Activity tags (`planning`, `design`, `review`, `implementation`). In this legacy layout, fed the keyword-matched Routing Table in the single always-loaded `governance.md`. That routing table no longer exists in the current schema (`compile_schema_version: 3`) — it was replaced by the ambient core's one-line topic index plus automatic `paths:`-based loading of topic files. Invariants scope to all activities by default. |

## The merge formula

When `/edikt:gov:compile` assembles the final governance.md, it reads all lists from every source and merges:

```
effective_rules = (directives - suppressed_directives) ∪ manual_directives
```

- Your `manual_directives:` always ship — compile can't override them
- Your `suppressed_directives:` always filter — compile can't un-suppress them
- The merge is exact string match — a suppression must match the directive text exactly

## Hash-based caching

Compile doesn't call the model when nothing changed:

| State | Condition | What happens |
|---|---|---|
| **Clean** | Both hashes match stored values | Skip — no model call, no writes |
| **Body changed** | `source_hash` doesn't match | Regenerate directives from new body |
| **Hand-edited** | `source_hash` matches but `directives_hash` doesn't | Interactive interview to resolve |
| **Fresh** | No sentinel block exists | First-time generation |
| **Forced** | `--regenerate` flag passed | Regenerate regardless of hashes |

The interview flow for hand-edits gives you five options per line: move to manual, suppress, delete, edit the source, or skip. In headless/CI mode, use `--strategy=regenerate` (discard edits) or `--strategy=preserve` (skip the file).

## Where sentinels live

Sentinels are embedded in the source documents themselves — not in separate files:

```
docs/architecture/decisions/ADR-003-hexagonal.md
  ├── ## Context (human)
  ├── ## Decision (human — compile reads this)
  ├── ## Consequences (human)
  └── [edikt:directives:start/end] (the model — compile writes this)
```

One file, two audiences, clearly separated by the sentinel markers.

## Commands that interact with sentinels

| Command | Reads sentinels | Writes sentinels |
|---|---|---|
| `/edikt:adr:compile` | Yes (hashes) | Yes (directives, reminders, verification) |
| `/edikt:invariant:compile` | Yes (hashes) | Yes (directives, reminders, verification) |
| `/edikt:guideline:compile` | Yes (hashes) | Yes (directives, reminders, verification) |
| `/edikt:gov:compile` | Yes (all five lists) | No (writes governance.md, not sentinels) |
| `/edikt:gov:review` | Yes (staleness check) | No |
| `/edikt:gov:score` | Indirectly (scores compiled output) | No |

## Next steps

- [How Governance Compiles](compile) — the full pipeline from source to governance.md
- [Extensibility](extensibility) — how to use manual_directives and suppressed_directives
- [Architecture Decisions](architecture-decisions) — ADR template with sentinel block
- [Invariant Records](invariant-records) — Invariant template with sentinel block
