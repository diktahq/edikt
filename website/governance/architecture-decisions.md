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
| **Compiles to** | MUST/NEVER directives in topic files | Non-negotiable constraints (top + bottom of governance.md) | Implementation directives in topic files |
| **Typical source** | Design review, team discussion, tech lead decision | Regulation, incident, foundational principle | Team agreement, code review pattern |

## The template

edikt's ADR template follows the structure from Nygard's original proposal (Context → Decision → Consequences) and adds Alternatives Considered (common in practice) plus a directive sentinel block (edikt extension for compile integration). The template is customizable — see [Extensibility](extensibility).

```markdown
# ADR-NNN: Short imperative title

**Date:** YYYY-MM-DD
**Status:** Draft | Accepted | Superseded by ADR-NNN

## Context

What is the situation? What forces are at play? What problem needs solving?

## Decision

What we decided to do. This is the section compile reads.
Use MUST/NEVER language for hard constraints.
Name specific tools, patterns, file paths, thresholds.

## Consequences

What changes as a result of this decision?
Positive and negative. What becomes easier, what becomes harder?

## Alternatives Considered

What else was evaluated and why it was rejected?

[edikt:directives:start]: #
[edikt:directives:end]: #
```

## Lifecycle

- **Draft** — under discussion, not yet enforced. Editable.
- **Accepted** — decision is final. Content is immutable (INV-002). Compile reads it. To change an accepted decision, create a new ADR that supersedes it.
- **Superseded by ADR-NNN** — replaced by a newer decision. Compile skips it. The superseding ADR explains what changed and why.

Once accepted, an ADR is immutable. This is enforced by [INV-002](invariant-records). If a decision changes, you write a new ADR — the history stays intact.

## How ADRs compile into governance (v0.6.0)

Every ADR has a co-located `<ADR>.edikt.yaml` sidecar that holds compiled directives — edikt does not write to your prose `.md` (ADR-027 makes the boundary structural). `/edikt:adr:compile <id>` regenerates exactly that one sidecar in a fresh subagent context with a locked extraction prompt. `/edikt:gov:compile` Phase A auto-resyncs stale sidecars (parallel, concurrency 8); Phase B merges them into topic files deterministically (no LLM, no `Task` dispatch — see [ADR-028](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-028-two-phase-compile-resync-merge.md)).

The `## Decision` section is what the extractor reads. It pulls every enforceable statement and transforms it into a directive:

```
Decision section (human, 150 lines):
  "Build edikt as a lean context engine targeting Claude Code exclusively.
   Other tools lack path-conditional rules, hooks, slash commands..."

Compiled directive (Claude, 1 line):
  "Claude Code is the only supported platform. NEVER target Cursor,
   Copilot, or other AI coding tools. (ref: ADR-001)"
```

The compile pipeline also generates:
- **Reminders** — "Before choosing a platform → MUST use Claude Code only (ref: ADR-001)"
- **Verification items** — "[ ] No Cursor/Copilot configuration files in the repo (ref: ADR-001)"

These land in governance.md's `## Reminders` and `## Verification Checklist` sections.

See [How Governance Compiles](compile) for the full pipeline.

## Writing effective ADRs

The Decision section is what becomes a directive. Write it for both audiences:

**For humans** — explain the decision clearly with enough context that a new team member understands it.

**For compile** — use MUST/NEVER language, name specific things (tools, patterns, file paths, thresholds), and make each statement verifiable:

```
Weak (compiles poorly):
  "We should probably use hexagonal architecture."

Strong (compiles well):
  "The domain layer MUST NOT import from infrastructure packages.
   Business logic lives in domain/. Adapters live in adapter/.
   No framework imports in domain/."
```

The compile pipeline scores each generated directive on token specificity, MUST/NEVER usage, grep-ability, and ambiguity. Run `/edikt:adr:review` after writing to check both human quality and directive quality.

## User extension points

The compiled sentinel block has two lists you can modify:

- **`manual_directives:`** — add rules compile missed. These always ship into governance.md.
- **`suppressed_directives:`** — reject auto-generated rules you disagree with. These are always filtered out.

The `directives:`, `reminders:`, and `verification:` lists are compile-owned — read-only for users. If you hand-edit `directives:`, compile detects it via hash comparison and runs an interactive interview to resolve (move to manual, suppress, or discard).

See [Extensibility](extensibility) for the full extension surface.

## Commands

| Command | What it does |
|---|---|
| `/edikt:adr:new` | Create a new ADR from natural language input |
| `/edikt:adr:compile` | Generate directive sentinel blocks |
| `/edikt:adr:review` | Review language quality + directive LLM compliance |

## Recent ADRs

- **ADR-027** — Sidecar architecture for governance metadata. Supersedes ADR-008. Generated directives live in co-located `<artifact>.edikt.yaml` sidecars; edikt no longer writes to ADR/INV/guideline `.md` files.
- **ADR-028** — Two-phase compile (Phase A resync + Phase B merge). Amends ADR-020's latency budget — Phase B preserves it; Phase A is a new conditional resync phase with no SLO but mandatory progress UI.

## Next steps

- [Sidecar Architecture](sidecar) — what sidecars are and why (v0.6.0)
- [How Governance Compiles](compile) — the full compile pipeline (Phase A + Phase B)
- [Invariant Records](invariant-records) — hard constraints (the counterpart to ADRs)
- [Extensibility](extensibility) — manual directives, suppressed directives, overrides
- [Sentinel Blocks](sentinels) — the technical format (legacy v0.5.x)
- [Sidecar Migration](/guides/sidecar-migration) — upgrading a v0.5.x or v0.4.3 project
