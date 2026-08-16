# Architecture Decision Records

Architecture Decision Records (ADRs) document the decisions behind your architecture. They're the historical record of *why* your system is built the way it is — captured once, immutable after acceptance, and compiled into directives Claude follows every session.

ADRs were formalized by [Michael Nygard in 2011](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions) as lightweight records of significant architectural decisions. The format has since been adopted widely — Joel Parker Henderson maintains a [comprehensive collection](https://github.com/joelparkerhenderson/architecture-decision-record) of templates and examples.

edikt adopts ADRs as a first-class governance artifact and extends them with a compile pipeline that transforms decisions into enforcement directives Claude follows automatically.

## ADRs vs Invariant Records vs Guidelines

| | ADR | Invariant Record | Guideline |
|---|---|---|---|
| **Documents** | A decision that was made | A constraint that must hold continuously | A team convention or preference |
| **Written when** | A decision is made (one-time) | A hard constraint needs enforcement | A pattern needs consistency |
| **Alternatives** | Yes — central to the format | No — invariants don't have alternatives | No |
| **Mutability** | Immutable once accepted. Supersede via new ADR. | Content immutable. Status can change. | Editable any time. |
| **Compiles to** | MUST/NEVER directives in topic files | Pathless invariants land in the ambient core (`.claude/rules/governance.md`); path-scoped invariant directives land in `directive-index.yaml` | Implementation directives in topic files |
| **Typical source** | Design review, team discussion, tech lead decision | Regulation, incident, foundational principle | Team agreement, code review pattern |

## The template

edikt's ADR template follows the structure from Nygard's original proposal (Context → Decision → Consequences) and adds Alternatives Considered (common in practice). Compiled directives live in a co-located sidecar (`<ADR>.edikt.yaml`), not in the prose body — see [Sidecar Architecture](sidecar). The template is customizable — see [Extensibility](extensibility).

```markdown
# ADR-NNN: Short imperative title

**Date:** YYYY-MM-DD
**Status:** Draft | Accepted | Superseded by ADR-NNN

## Context

What is the situation? What forces are at play? What problem needs solving?

## Decision

What we decided to do. This is the section the sidecar extractor reads.
Use MUST/NEVER language for hard constraints.
Name specific tools, patterns, file paths, thresholds.

## Consequences

What changes as a result of this decision?
Positive and negative. What becomes easier, what becomes harder?

## Alternatives Considered

What else was evaluated and why it was rejected?
```

## Lifecycle

- **Draft** — under discussion, not yet enforced. Editable.
- **Accepted** — decision is final. Content is immutable. Compile reads it. To change an accepted decision, create a new ADR that supersedes it.
- **Superseded by ADR-NNN** — replaced by a newer decision. Compile skips it. The superseding ADR explains what changed and why.

Once accepted, an ADR is immutable. If a decision changes, you write a new ADR — the history stays intact.

## How ADRs compile into governance (v0.6.0)

Every ADR has a co-located `<ADR>.edikt.yaml` sidecar that holds compiled directives — edikt does not write to your prose `.md`. `/edikt:adr:compile <id>` regenerates exactly that one sidecar in a fresh subagent context with a locked extraction prompt. `/edikt:gov:compile` Phase A auto-resyncs stale sidecars (parallel, concurrency 8); Phase B merges them into topic files deterministically (no LLM, no `Task` dispatch).

The `## Decision` section is what the extractor reads. It pulls every enforceable statement and transforms it into a directive:

```
Decision section (human, 150 lines):
  "Build edikt as a lean context engine targeting Claude Code exclusively.
   Other tools lack path-conditional rules, hooks, slash commands..."

Compiled directive (Claude, 1 line):
  "All public API routes MUST be versioned under /vN/. NEVER break
   a published contract. (ref: ADR-001)"
```

The compile pipeline also generates:
- **Reminders** — "Before changing an API route → MUST keep the published version stable (ref: ADR-001)"
- **Verification items** — "[ ] No breaking changes to published /v1/ endpoints (ref: ADR-001)"

These land in the directive's topic file (`.claude/rules/governance/<topic>.md`), under its `## Reminders` and `## Verification Checklist` sections — not in the ambient core (`.claude/rules/governance.md`), which carries only pathless-invariant statements and a one-line topic index.

See [How Governance Compiles](compile) for the full pipeline.

## Writing effective ADRs

The Decision section is what becomes a directive. Write it for both audiences:

**For humans** — explain the decision clearly with enough context that a new team member understands it.

**For compile** — use MUST/NEVER language, name specific things (tools, patterns, file paths, thresholds), and make each statement verifiable. [Writing good ADRs](writing-adrs) works the same Decision section through both phrasings and shows what each one compiles into.

The compile pipeline scores each generated directive on token specificity, MUST/NEVER usage, grep-ability, and ambiguity. Run `/edikt:adr:review` after writing to check both human quality and directive quality.

## User extension points

The co-located `<ADR>.edikt.yaml` sidecar has two lists you can modify:

- **`manual_directives:`** — add rules compile missed. These always ship into the topic file.
- **`suppressed_directives:`** — reject auto-generated rules you disagree with. These are always filtered out.

`directives[]` is extractor-owned but editable: add an entry with a real `source_excerpts` quote to backfill a missed rule, or delete an entry to suppress one. `/edikt:adr:review` cross-checks the sidecar against the prose body and warns on drift.

See [Extensibility](extensibility) for the full extension surface.

## Commands

| Command | What it does |
|---|---|
| `/edikt:adr:new` | Create a new ADR from natural language input |
| `/edikt:adr:compile` | Generate directive sentinel blocks |
| `/edikt:adr:review` | Review language quality + directive LLM compliance |

## Next steps

- [Writing good ADRs](writing-adrs) — how to write a Decision section that compiles well
- [Sidecar Architecture](sidecar) — what sidecars are and why (v0.6.0)
- [How Governance Compiles](compile) — the full compile pipeline (Phase A + Phase B)
- [Invariant Records](invariant-records) — hard constraints (the counterpart to ADRs)
- [Extensibility](extensibility) — manual directives, suppressed directives, overrides
- [Sidecar Migration](/guides/sidecar-migration) — upgrading a v0.4.3 project
- [Sentinel Blocks](sentinels) — the legacy in-body format (superseded by sidecars)
