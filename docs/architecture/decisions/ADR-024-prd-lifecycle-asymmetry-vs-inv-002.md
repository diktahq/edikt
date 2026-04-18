---
type: adr
id: ADR-024
title: PRD lifecycle asymmetry — edit-in-place vs INV-002 immutability
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-04-18T00:00:00Z
references:
  adrs: [ADR-009]
  invariants: [INV-002]
  specs: [SPEC-007]
  brainstorm: docs/brainstorms/BRAIN-001-prd-as-context-bundle/
---

# ADR-024 — PRD lifecycle asymmetry: edit-in-place vs INV-002 immutability

## Status

**Accepted**

## Context

INV-002 governs ADRs: once `status: accepted`, an ADR is immutable. Changes require superseding via a new ADR that points back at the old one. This works for ADRs because ADRs model **discrete binary decisions** — "we chose X over Y" — where the decision itself either holds or is replaced. The supersede chain is the audit trail: read backward to see how the team's thinking evolved.

PRDs do not model discrete decisions. They model **continuous feature evolution**: a product requirement is authored, scoped, refined, released in phases, ships, evolves over multiple releases, and eventually deprecates. A single requirement inside a single PRD routinely transitions through `proposed → accepted → shipped → deprecated` over months. Forcing an ADR-style supersession chain on every refinement creates artificial artifacts that answer the wrong question: "what was written in April vs what was written in June" rather than "what is this product doing right now."

Anthropic's harness research (referenced in `docs/internal/reference_anthropic_harness_design.md`) and prior dogfood experiments (`docs/internal/project_experiment_findings.md`) converge on the same point: LLMs handle **single evolving documents with structured status markers** more reliably than chains of superseding documents. Compound supersession chains increase context length, multiply cross-references to check for consistency, and increase the rate of LLM-introduced contradictions when reasoning across the chain.

Applying INV-002 verbatim to PRDs would optimize for audit trail symmetry at the cost of LLM usability and authoring ergonomics — an outcome inconsistent with edikt's core premise that governance artifacts must be LLM-legible first.

## Decision

**PRDs and SPECs use edit-in-place evolution. ADRs remain governed by INV-002. The asymmetry is intentional.**

Operational rules:

1. **Per-entry status markers.** Each requirement (`FR-NNN`), acceptance criterion (`AC-NNN-M`), and protection carries its own `status: proposed | accepted | shipped | deprecated` field. The top-level PRD `status:` reflects the aggregate state.
2. **Revision history is structured.** Every command-driven mutation to a PRD/SPEC sidecar appends a record to `revision_history:` in the YAML sidecar. Git diff remains the deepest audit trail.
3. **Edit-in-place is the default.** Adding/refining/shipping/deprecating FRs is done via transition commands (`prd:ship`, `prd:deprecate`, etc.), not via supersession.
4. **Supersede is reserved for ≥50% rewrites.** `/edikt:sdlc:prd:supersede` creates a new `PRD-NNN` when the problem framing has changed or the scope shift is so large that the old document is misleading to read. The old PRD keeps `superseded_by:`; the new one keeps `supersedes:`.
5. **ADRs are unchanged.** INV-002 remains in force for ADRs. This ADR does not weaken or scope INV-002; it carves PRDs and SPECs out of the ADR immutability model.
6. **Evaluator semantics respect per-entry status.** When the PRD/SPEC evaluator validates implementation coverage, it scores only `shipped` entries. `proposed` entries are ignored by drift checks; they have not been built yet by design.

## Consequences

- **Positive.** PRDs remain readable as a single current document rather than a chain. LLM reasoning over PRD content stays reliable as features evolve. Transition commands keep structural mutations consistent across the project.
- **Positive.** The seam between "architecture governance" (ADRs, invariants) and "product governance" (PRDs, SPECs) is explicit. New contributors learning the system get a clear mental model: decisions are immutable, features evolve.
- **Negative.** The asymmetry must be documented in every place that references artifact lifecycles, otherwise new contributors expect PRDs to behave like ADRs.
- **Negative.** Git diff is the only deep audit trail for PRD evolution. Teams that want external approval records need the `stakeholders:` sign-off integration (SPEC-007 §24) as a separate mechanism.
- **Mitigation.** `commands/sdlc/prd.md` and `commands/adr/new.md` both reference this ADR in their output footers so the asymmetry is visible at the authoring moment.

## Alternatives Considered

- **Apply INV-002 to PRDs.** Rejected: optimizes audit symmetry at the cost of LLM ergonomics; forces every FR refinement into a supersession chain.
- **Create INV-NNN for PRD edit-in-place.** Rejected: the PRD model is not "never changes" but "always changes via structured mutations." That is a decision (ADR), not a rule (invariant).
- **Mode-switch PRDs.** Rejected: letting projects opt into ADR-style immutability for PRDs creates two incompatible mental models. The v0.6.0 redesign picks one and commits.

[edikt:directives:start]: #
directives:
  - PRDs and SPECs MUST evolve via in-place YAML sidecar mutations using per-entry status markers (proposed | accepted | shipped | deprecated). (ref: ADR-024)
  - Supersede (create new PRD-NNN) is reserved for ≥50% scope rewrites or problem-framing shifts. Routine FR refinements MUST use transition commands instead. (ref: ADR-024)
  - ADRs remain immutable per INV-002. The PRD edit-in-place model does NOT apply to ADRs. (ref: ADR-024, INV-002)
  - Evaluator scoring MUST consider only FRs and ACs with status: shipped when validating implementation coverage. Proposed entries are not drift. (ref: ADR-024)
manual_directives: []
suppressed_directives: []
canonical_phrases: ["edit-in-place", "per-entry status", "supersede", "revision_history", "PRD lifecycle"]
behavioral_signal:
  cite: ["ADR-024", "INV-002"]
paths: ["docs/product/prds/**", "docs/product/specs/**", "commands/sdlc/prd.md", "commands/sdlc/spec.md", "commands/sdlc/prd/**"]
scope: [planning, design, review]
[edikt:directives:end]: #
