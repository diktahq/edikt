# How Governance Compiles

edikt governance flows through a pipeline: you write decisions for humans, compile extracts directives for Claude, and Claude reads them every session. Understanding this pipeline is key to writing governance that actually gets followed.

## Two audiences, one source

Every governance document has two audiences:

| Section | Audience | Purpose |
|---|---|---|
| Statement, Rationale, Consequences, Implementation, Anti-patterns, Enforcement | **Humans** | Understand the decision, its context, why it exists, how to comply |
| Directives sentinel block (`[edikt:directives:start/end]`) | **Claude** | Short, imperative rules Claude reads and follows literally |

You write the human sections. The compile pipeline generates the Claude sections. Both live in the same file, clearly separated by sentinel markers.

## The sentinel block

Every ADR, Invariant Record, and guideline can contain a directive sentinel block:

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

## How directives are generated

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

## The three-list merge

When `/edikt:gov:compile` assembles the final governance.md, it reads all three lists from every source and merges them:

```
effective_rules = (directives - suppressed_directives) ∪ manual_directives
```

This means:
- Your `manual_directives:` always ship, even if compile doesn't generate them
- Your `suppressed_directives:` always filter, even if compile keeps regenerating them
- You have full control without editing compile's output

See [ADR-008](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-008-deterministic-compile-and-three-list-schema.md) for the formal schema.

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

Compile doesn't call Claude when nothing changed. Two hashes gate the decision:

- **`source_hash`** — SHA-256 of the human content. If unchanged since last compile → skip.
- **`directives_hash`** — SHA-256 of the directives list. If it doesn't match after regeneration → you hand-edited. Compile runs an interactive interview to resolve.

This means compile is fast (skips clean files) and safe (detects hand-edits before overwriting).

## Commands

| Command | What it does |
|---|---|
| `/edikt:invariant:compile` | Generate sentinel blocks for invariants |
| `/edikt:adr:compile` | Generate sentinel blocks for ADRs |
| `/edikt:guideline:compile` | Generate sentinel blocks for guidelines |
| `/edikt:gov:compile` | Assemble all sentinels into governance.md + topic files |
| `/edikt:gov:score` | Score the compiled output for LLM compliance |
| `/edikt:gov:review` | Review for contradictions and language quality |

The typical flow: write a decision → compile the artifact → compile governance → score quality.
