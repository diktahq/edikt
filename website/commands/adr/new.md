# /edikt:adr:new

Capture an Architecture Decision Record — from scratch or extracted from the current conversation.

## Usage

```bash
/edikt:adr:new use postgres for persistence
/edikt:adr:new                                  ← extracts from current conversation
```

## Interview prompts for new sentinel fields

When capturing a new ADR, three additional prompts follow the core decision capture prompts. They populate the `canonical_phrases` and `behavioral_signal` sentinel fields:

1. **Canonical phrases** — "What 2–3 words or short phrases should a compliant model refusal echo back? (e.g., 'never compiled', 'plain markdown', 'no build step'). Skip to leave empty."
2. **Signal type** — "Does this directive have a machine-testable violation signal? Options: `refuse_tool`, `refuse_to_write`, `cite`, `refuse_edit_matching_frontmatter`, or skip."
3. **Signal value** — (follows based on signal type selected) — e.g., tool names, path substrings, or frontmatter predicate fields.

Skipping any prompt produces empty values — `canonical_phrases: []` or `behavioral_signal: {}`. No error, no prompt repeat. You can retrofit these fields later with `/edikt:adr:review --backfill`.

## What is an ADR?

An Architecture Decision Record captures a significant technical choice with its context, reasoning, alternatives considered, and consequences. Unlike comments in code, ADRs survive refactoring and give future teammates (and Claude) the "why" behind decisions.

## Two modes

### With argument — define from scratch

```bash
/edikt:adr:new use postgres for persistence
```

edikt opens a structured conversation to work through the decision:
- What problem does this solve?
- What alternatives were considered?
- Why was this chosen?
- What are the consequences?

Creates: `docs/decisions/{NNN}-use-postgres-for-persistence.md`

### No argument — extract from conversation

```bash
/edikt:adr:new
```

edikt reads the current conversation, extracts the last significant technical decision discussed, and creates an ADR from it. Useful when you've been discussing trade-offs and realize it's worth capturing.

## Proactive suggestions

You don't need to remember to run this. The `Stop` hook installed by `/edikt:init` watches every Claude response for significant technical choices with trade-offs. When it detects one, Claude ends its response with:

```text
💡 This looks like an ADR — run `/edikt:adr:new` to capture it.
```

## Template

edikt uses a template to structure the ADR. The template lookup chain:

1. **Project override** — `.edikt/templates/adr.md` (if present)
2. **edikt default** — built-in template

Customize the template by placing your own at `.edikt/templates/adr.md`. Your template is preserved across upgrades — edikt never overwrites project templates.

The default template produces:

```markdown
# ADR-NNN: Short imperative title

**Date:** YYYY-MM-DD
**Status:** Draft

## Context
## Decision        ← compile reads this section
## Consequences
## Alternatives Considered

[edikt:directives:start]: #
[edikt:directives:end]: #
```

The `## Decision` section is what the compile pipeline reads to generate directives. Write it with MUST/NEVER language and literal code tokens for effective compilation. See [Writing good ADRs](/governance/writing-adrs) for guidance.

## Output (v0.6.0)

```text
docs/architecture/decisions/
├── ADR-003-use-postgres-for-persistence.md          ← prose. you own it.
└── ADR-003-use-postgres-for-persistence.edikt.yaml  ← sidecar. edikt writes it.
```

After creating the prose `.md`, edikt dispatches the `sidecar-extractor` agent in a forked subagent (`context: fork`) with a locked extraction prompt. The agent reads the Decision section, extracts MUST/NEVER directives, and writes the co-located `<ADR>.edikt.yaml`. The pair is created atomically — if extraction fails, neither file remains.

The locked prompt + forked context prevents cross-artifact contamination: every ADR's sidecar is generated in its own fresh context with the same prompt, regardless of whether you're creating one ADR or batching ten. See [Sidecar Architecture](/governance/sidecar) for the data model.

You'll see:

```text
✅ Created ADR-003-use-postgres-for-persistence.md
✅ Generated ADR-003-use-postgres-for-persistence.edikt.yaml — review it before sharing.
```

If you create the ADR in `draft` status, the sidecar is generated anyway. Drafts are mutable; the sidecar regenerates whenever the prose changes (run `/edikt:adr:compile <id>` to refresh, or just run `/edikt:gov:compile` — Phase A auto-resyncs stale sidecars).

## ADRs are immutable once accepted

Once an ADR is accepted, its content — context, decision, and consequences — must never be edited. This is enforced by INV-002 and compiled into governance directives.

When a decision changes, create a new ADR that supersedes the old one. The old ADR's status is updated to `Superseded by ADR-NNN` — the only permitted mutation after acceptance. Draft ADRs may be freely edited before acceptance.

## What's next

- [/edikt:adr:compile](/commands/adr/compile) — compile ADR into governance directives
- [/edikt:adr:review](/commands/adr/review) — review language quality + directive LLM compliance
- [Architecture Decisions](/governance/architecture-decisions) — what ADRs are, lifecycle, how they compile
- [Writing good ADRs](/governance/writing-adrs) — guide for effective ADR writing
- [Extensibility](/governance/extensibility) — manual directives, suppressed directives, template overrides

## Natural language triggers

- "save this decision"
- "record this choice"
- "capture that"
- "let's write an ADR"
