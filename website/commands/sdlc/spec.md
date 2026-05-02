# /edikt:sdlc:spec

Generate a technical specification — the engineering response that defines HOW to build what the source artifact says to build. Architecture decisions, trade-offs, components, data access, security, testing strategy.

## Usage

```bash
/edikt:sdlc:spec PRD-005           # full coverage check against PRD FRs
/edikt:sdlc:spec BRAIN-007         # advisory checklist from a brainstorm
/edikt:sdlc:spec "harden the rate limiter"   # free-text, self-contained spec
```

## Source flexibility

As of v0.6.0, the SPEC accepts three source types. The sidecar carries exactly one of `source_prd`, `source_brainstorm`, or `source_prompt` so traceability is unambiguous.

| Source | Coverage check | Back-reference |
|--------|---------------|----------------|
| `PRD-NNN` | Full — every FR must be covered, deferred, or explicitly uncovered | PRD sidecar's `source_specs += SPEC-NNN` |
| `BRAIN-NNN` | Advisory — decisions and open questions surface as a checklist | Brainstorm's `produced_specs += SPEC-NNN` |
| Free text | None — spec is self-contained, evaluator judges on its own merits | None — no upstream artifact |

PRD sources are the strongest path. Use brainstorm sources when you're prototyping ahead of a PRD. Use free text when the work is small enough that a PRD would be ceremony — the spec is still structured, still rubric-scored, but doesn't pretend to inherit from anything.

The structural source-of-truth lives in `templates/schemas/spec-sidecar.v1.schema.json` and provides JSON Schema autocomplete in editors that load `yaml-language-server`.

## Gate (PRD source only)

When the source is a PRD, it must have `status: accepted` before a spec can be generated:

```text
BLOCKED  PRD-005 status is "draft".
         PRDs must be accepted before generating a spec.
         Review the PRD and change status to "accepted" first.
```

This gate exists because drafting a technical specification against unresolved requirements produces wasted work. Accept the PRD first.

## What the command does

**1. Scans the codebase** — reads rules, agents, ADRs, invariants, and `docs/project-context.md` to understand what exists before asking any questions.

**2. Interviews with context** — asks 2–4 questions that prove it understood the codebase, not just the PRD. Questions reference what it found:

```text
The codebase has ADR-003 for error handling (wrapped errors with context).
Should this spec follow that pattern or propose a different approach?

I see a hexagonal architecture with domain/, port/, adapter/ layers.
Should this feature follow the same pattern?
```

**3. Shows an outline** — before routing to specialist agents, confirms what the spec will cover:

```text
Based on the PRD and your answers, the spec will cover:
  - Architecture: hexagonal, same pattern as existing code
  - Key components: WebhookService, WebhookRepository, delivery adapter
  - Data: new webhooks table + delivery_attempts table
  - APIs: POST /webhooks, POST /webhooks/retry, GET /webhooks/:id
  - Breaking changes: none
  - Open questions: 2 carried from PRD

Proceed? (y/n)
```

**4. Checks for ADR conflicts** — surfaces any contradictions between the proposed approach and existing decisions.

**5. Generates the spec** — routes to `architect` and relevant domain specialists. Produces a spec file at:

```text
docs/product/specs/SPEC-{NNN}-{slug}/spec.md
```

## v2 PRD coverage flow

When the source is a v2 PRD (sidecar present), the SPEC carries a `source_prd_coverage` block that maps every PRD FR to its disposition:

```yaml
source_prd_coverage:
  prd: PRD-005
  covered:
    - fr: FR-001
      by: [SR-001, SR-002]
    - fr: FR-002
      by: [SR-003]
  deferred:
    - fr: FR-004
      rationale: "Out of scope; tracked in SPEC-MMM"
  uncovered: []
```

ACs from the PRD pass through unchanged — same `id`, byte-equal Given/When/Then. The SPEC may add architectural acceptance criteria as `SAC-NNN` but must not renumber, rewrite, or merge PRD ACs. Coverage is verified by [/edikt:spec:review](/commands/spec/review) and at phase-end by the evaluator.

After a successful write, the command updates the PRD sidecar's `source_specs:` so traceability is bidirectional — open the PRD and you see which SPECs reference it without grep.

## Output format

```yaml
---
type: spec
id: SPEC-005
status: draft
implements: PRD-005
source_prd: PRD-005           # exactly one of these three is set
source_brainstorm: null
source_prompt: null
references:
  adrs: [ADR-001, ADR-003]
  invariants: [INV-001]
created_at: 2026-03-20T14:30:00Z
---
```

Sections:
- Summary
- Existing Architecture (what the spec builds on)
- Proposed Architecture
- Components (what gets built, where it lives, how it integrates)
- Trade-offs (alternatives considered)
- Security Considerations
- Performance Approach
- Testing Strategy
- Open Questions

## After generating

Review the spec. Change `status: draft` to `status: accepted` when ready to proceed.

Then run `/edikt:sdlc:artifacts` to generate the implementable artifacts.

```
Run /edikt:sdlc:artifacts SPEC-005 to generate implementable artifacts.
```

## What's next

- [/edikt:spec:review](/commands/spec/review) — re-score the SPEC and verify FR coverage + AC pass-through
- [/edikt:sdlc:artifacts](/commands/sdlc/artifacts) — generate data model, API contracts, migrations, test strategy
- [/edikt:sdlc:plan](/commands/sdlc/plan) — phased execution with pre-flight review
- [Governance Chain](/governance/chain) — full chain overview
