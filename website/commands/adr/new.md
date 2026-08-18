# /edikt:adr:new

Capture an Architecture Decision Record — from scratch or extracted from the current conversation.

## Usage

```bash
/edikt:adr:new use postgres for persistence
/edikt:adr:new                                  ← extracts from current conversation
```

## What is an ADR?

An Architecture Decision Record captures a significant technical choice with its context, reasoning, alternatives considered, and consequences. Unlike comments in code, ADRs survive refactoring and give future teammates (and the model) the "why" behind decisions.

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

Creates: `docs/architecture/decisions/{NNN}-use-postgres-for-persistence.md`

### No argument — extract from conversation

```bash
/edikt:adr:new
```

edikt reads the current conversation, extracts the last significant technical decision discussed, and creates an ADR from it. Useful when you've been discussing trade-offs and realize it's worth capturing.

## Interview prompts for new sentinel fields

When capturing a new ADR, three additional prompts follow the core decision capture prompts. They populate the `canonical_phrases` and `behavioral_signal` sentinel fields:

1. **Canonical phrases** — "What 2–3 words or short phrases should a compliant model refusal echo back? (e.g., 'never compiled', 'plain markdown', 'no build step'). Skip to leave empty."
2. **Signal type** — "Does this directive have a machine-testable violation signal? Options: `refuse_tool`, `refuse_to_write`, `cite`, `refuse_edit_matching_frontmatter`, or skip."
3. **Signal value** — (follows based on signal type selected) — e.g., tool names, path substrings, or frontmatter predicate fields.

Skipping any prompt produces empty values — `canonical_phrases: []` or `behavioral_signal: {}`. No error, no prompt repeat. You can retrofit these fields later with `/edikt:adr:review --backfill`.

## Proactive suggestions

You don't need to remember to run this. The `Stop` hook installed by `/edikt:init` watches every response for significant technical choices with trade-offs. When it detects one, it ends the response with:

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
## Decision        ← the sidecar extractor reads this section
## Consequences
## Alternatives Considered
```

The `## Decision` section is what the sidecar extractor reads to generate directives. Write it with MUST/NEVER language and literal code tokens for effective compilation. See [Writing good ADRs](/governance/writing-adrs) for guidance. In v0.6.0+ the prose template carries no in-body directives block — the generated directives land in the sibling sidecar shown under [Output](#output-v060).

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
✅ Verify: 3 of 5 passed.
```

If you create the ADR in `draft` status, the sidecar is generated anyway. Drafts are mutable; the sidecar regenerates whenever the prose changes (run `/edikt:adr:compile <id>` to refresh, or just run `/edikt:gov:compile` — Phase A auto-resyncs stale sidecars).

### Post-write verify gate

After both files are on disk, `/edikt:adr:new` shells to:

```bash
bin/edikt verify gov ADR-NNN
```

The runner walks every `directives[].verify`, `prohibitions[].verify`, and structured `verification[].verify` declared in the new sidecar and runs each as a shell command. Items without a `verify:` field are recorded as `skipped`.

- **Exit 0** → proceed silently. The verify line above is included in the success summary.
- **Exit 1** → surface a warning with the per-item failures:

  ```text
  ⚠ ADR-003 created, but 2 of 5 verify(s) failed:
    directive[1]: exit=1 — rg matched lines that violate the rule
  Fix the directive prose, edit the sidecar's verify: line, or remove it.
  The artifact is on disk at docs/architecture/decisions/ADR-003-….md.
  ```

The artifact is **never auto-deleted** on verify failure — you need the file on disk to inspect and iterate. See [`edikt verify`](/commands/verify) for the full contract.

## ADRs are immutable once accepted

Once an ADR is accepted, its content — context, decision, and consequences — must never be edited. This is a hard governance rule, compiled into directives.

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
