---
type: spec
id: SPEC-007
title: PRD redesign — split artifact, rigor calibration, SDLC chain traceability
status: accepted
author: Daniel Gomes
created_at: 2026-04-18T00:00:00Z
references:
  adrs: [ADR-001, ADR-005, ADR-009, ADR-010, ADR-018]
  invariants: [INV-001, INV-002]
  specs: [SPEC-004, SPEC-006]
  brainstorm: docs/brainstorms/BRAIN-001-prd-as-context-bundle/
new_adrs:
  - ADR-024 (Phase 1): "PRD lifecycle asymmetry vs INV-002 — edit-in-place for features"
baseline:
  shipped:
    - SPEC-006 hook hardening + tier-2 install (v0.6.0)
    - /edikt:sdlc:prd v1 (no sidecar, flat template)
    - /edikt:sdlc:spec v1 (reads PRD but no FR coverage, no AC pass-through)
    - evaluator loop exists at plan/phase level (ADR-018)
---

# SPEC-007: PRD redesign — split artifact, rigor calibration, SDLC chain traceability

**Date:** 2026-04-18
**Author:** Daniel Gomes
**Source:** `docs/brainstorms/BRAIN-001-prd-as-context-bundle/` (all 26 decisions locked)

---

## Summary

v0.6.0 adds the second leg of the SDLC governance story. v0.5.0 established architectural
governance (ADRs, invariants, compile → enforcement). SPEC-007 establishes **product
artifact governance**: a redesigned PRD surface where requirements flow with stable IDs
through the full chain PRD → SPEC → artifacts → plan → implementation → verification.

Four workstreams:

1. **Split artifact model** — every PRD/SPEC produces two files: `.md` narrative +
   `.yaml` structural sidecar. Sidecar is source of truth for FRs, ACs, status,
   revision history, protections, and cross-references. LLMs don't corrupt YAML
   structure the way they corrupt markdown tables.

2. **Rigor-calibrated authoring** — single `build` mode with `rigor: solo | team |
   platform` flag. Five always-ask forcing questions (Cutler, Torres, Amazon,
   Intercom synthesis). Sections appear by rigor level. Generator-evaluator loop runs
   in-flight during authoring.

3. **SDLC chain traceability** — stable IDs (FR-NNN, AC-NNN-M, SR-NNN) flow verbatim
   through PRD → SPEC. SPEC emits `source_prd_coverage:` showing which FRs it
   addresses. SPEC command writes `source_specs:` back to PRD sidecar. Bidirectional
   grep-traceable chain from PRD to implementation.

4. **New command surface** — `/edikt:prd:review`, `/edikt:spec:review`,
   `/edikt:sdlc:discovery`, and transition commands (`prd:ship`, `prd:supersede`,
   `prd:deprecate`, `prd:cancel`). Review commands close the audit gap where every
   governance artifact type (ADR, INV, guideline) has a review command except PRD/SPEC.

---

## Functional Requirements

### FR-001: Split artifact generation

Every invocation of `/edikt:sdlc:prd` creates two files:
- `PRD-NNN-<slug>.md` — human-readable narrative
- `PRD-NNN-<slug>.yaml` — structural sidecar (machine-readable, command-mutated)

Both files are created in the same directory. The `.yaml` sidecar carries:

```yaml
schema_version: "1.0"
type: prd
id: PRD-NNN
title: "..."
status: draft           # draft | accepted | shipped | evolving | superseded | deprecated | cancelled
rigor: solo             # solo | team | platform
author: "..."
created_at: "ISO8601"
requirements:
  - id: FR-001
    text: "..."
    status: proposed    # proposed | accepted | shipped | deprecated
acceptance_criteria:
  - id: AC-001-1
    fr: FR-001
    given: "..."
    when: "..."
    then: "..."
    status: proposed
protections:
  - ref: INV-NNN        # linked invariant
    note: "..."
  - id: SP-001          # feature-scoped protection (no INV yet)
    text: "..."
solution_references:
  - type: figma | screenshot | url | doc
    path_or_url: "..."
    description: "..."
stakeholders: []        # team/platform rigor only
source_specs: []        # written back by /edikt:sdlc:spec
revision_history: []    # auto-updated on every command mutation
extensions: {}          # user-managed, LLM never writes here
_sync:
  md_hash: "<SHA-256 of .md file>"
  yaml_hash: "<SHA-256 of .yaml file>"
  synced_at: "ISO8601"
```

### FR-002: Rigor calibration

Three rigor levels with additive sections:

| Section | solo | team | platform |
|---------|------|------|----------|
| Problem, Users, Goals, Non-Goals, FRs, ACs, Solution Refs, Protections, Open Qs | ✓ | ✓ | ✓ |
| Stakeholders, Dependencies, Rollout Plan | | ✓ | ✓ |
| NFRs, Risk Register, Compatibility Matrix, Compliance Hooks | | | ✓ |

Default: `solo`. One triage question at authoring start.

### FR-003: Five always-ask forcing questions

Every PRD authoring session begins with these five questions (not skippable):

1. What's the problem *behind* this problem? (Cutler)
2. How do you know someone has this problem? (evidence or "hypothesis only")
3. What metric moves if this works, AND what must NOT move? (north + counter)
4. What must NOT change when this ships? (feeds Protections section)
5. What's the riskiest assumption?

"Hypothesis only" counts as a valid answer for Q2. Evaluator scores these five as
answered or unanswered; PASS requires all five answered at `solo` rigor, all five
plus rigor-level additions at `team/platform`.

### FR-004: Protection section with invariant auto-link

During PRD authoring, after the user answers "What must NOT change?":

1. Grep existing invariants for topics matching PRD scope.
2. Present relevant INV-NNN entries as suggested links (user confirms).
3. Capture any new protection the user states that looks like a durable rule → suggest
   `/edikt:invariant:new` with the protection text pre-filled.
4. Write confirmed links and feature-scoped protections to `protections:` in sidecar.

### FR-005: Stable IDs through chain

IDs assigned at creation, never reassigned:

- `FR-NNN` — PRD functional requirements (NNN = 001, 002, ...)
- `AC-NNN-M` — acceptance criteria (NNN = FR number, M = criterion index)
- `US-NNN` — user stories (optional)
- `SR-NNN` — SPEC requirements (`implements: FR-NNN`)
- `SAC-NNN` — SPEC-added acceptance criteria (architectural layer)

IDs are grep-traceable. `/edikt:sdlc:drift` detects when a FR-NNN present in the PRD
sidecar has no corresponding SR-NNN in any SPEC.

### FR-006: Generator-evaluator loop at PRD level

After the PRD draft is produced, the evaluator runs in-flight:

- Rubric: `.edikt/rubrics/prd.md` (auto-created if absent; editable per ADR-005)
- Threshold: `solo` = 70%, `team` = 80%, `platform` = 90%
- Evaluator mode mirrors `evaluator.mode` config (headless default per ADR-010)
- Loop until threshold met or max attempts (3)
- Final score shown in PRD authoring summary

### FR-007: Spec command — seven concrete changes

When `/edikt:sdlc:spec` reads a v2 PRD (has `.yaml` sidecar):

1. **FR coverage check** — every FR-NNN in the PRD sidecar must be addressed by at
   least one SR-NNN or explicitly deferred with rationale. Emit `source_prd_coverage:`
   in spec YAML listing each FR and its coverage status.
2. **AC pass-through** — G/W/T ACs from PRD sidecar appear verbatim in spec with
   unchanged IDs. SPEC adds SAC-NNN for architectural criteria.
3. **Stable ID propagation** — each SR-NNN carries `implements: FR-NNN`.
4. **Back-reference emission** — spec command appends `SPEC-NNN` to `source_specs:` in
   the PRD sidecar.
5. **Solution ref pass-through** — SPEC inherits `solution_references:` from PRD;
   can add architecture/sequence diagram refs.
6. **Protection propagation** — SPEC inherits PRD `protections:`; can add
   technical-layer protections.
7. **Evaluator hook** — SPEC evaluator runs in-flight at spec level, same pattern.

When `/edikt:sdlc:spec` reads a v1 PRD (no sidecar): proceed as today, warn once.

### FR-008: Review commands

Two new commands:

**`/edikt:prd:review [PRD-NNN]`** — re-runs PRD evaluator rubric on an existing PRD.
- Reads `.yaml` sidecar + `.md`
- Scores against `.edikt/rubrics/prd.md`
- Reports: rubric score, gaps, sidecar drift (md_hash mismatch), broken refs
- Produces actionable improvement list

**`/edikt:spec:review [SPEC-NNN]`** — same for SPEC.
- Checks `source_prd_coverage:` completeness
- Validates AC pass-through (all AC-NNN-M from PRD still present unchanged)
- Reports drift from linked PRD

### FR-009: Discovery command

**`/edikt:sdlc:discovery [BRAIN-NNN | description]`** — structured discovery doc peer
to `/edikt:brainstorm`. Produces `DISCOVERY-NNN-<slug>.md` with:

- Evidence (what do we know, what don't we know)
- Discovery Plan (what experiments/research would reduce uncertainty)
- Kill Criteria (what would make us stop)
- Assumptions Register

Graduation path: `/edikt:sdlc:discovery BRAIN-NNN` lifts an existing brainstorm doc
into a formal discovery with the additional sections populated.

### FR-010: Transition commands

Four explicit transition commands (all on `sdlc:prd:` prefix):

- **`/edikt:sdlc:prd:ship FR-001`** — marks FR-NNN entries as `status: shipped`;
  updates `revision_history:` with timestamp. Sets top-level status to `shipped` when
  all FRs shipped.
- **`/edikt:sdlc:prd:supersede PRD-001`** — creates new PRD-NNN with
  `supersedes: PRD-001`; sets `superseded_by: PRD-NNN` on original.
- **`/edikt:sdlc:prd:deprecate PRD-001`** — sets `status: deprecated` with
  `deprecated_at:` and optional `reason:`.
- **`/edikt:sdlc:prd:cancel PRD-001`** — sets `status: cancelled`; hides from active
  views. Historical record kept.

### FR-011: Schema and IDE tooling

- JSON Schema at `templates/schemas/prd-sidecar.schema.json` — enables autocomplete
  in VS Code / JetBrains / Neovim via `yaml-language-server`.
- Schema `$id` references `schema_version: "1.0"`.
- `/edikt:doctor` flags sidecars with unknown or stale `schema_version`.
- `init.md` writes `# yaml-language-server: $schema=...` comment to generated sidecars.

### FR-012: Doctor integration

New `/edikt:doctor` checks:
- **Broken refs** — linked INV-NNN doesn't exist; linked SPEC-NNN doesn't exist;
  `solution_references` path missing on disk (Figma URLs skipped by default).
- **Schema version** — sidecar has no `schema_version` or unknown version.
- **Sidecar drift** — `_sync.md_hash` doesn't match actual `.md` file hash
  (informational warning, not error).
- **Orphaned sidecars** — `.yaml` exists without matching `.md` or vice versa.

### FR-013: Migration and backward compatibility

- Grandfather: existing `.md` PRDs continue to work with `/edikt:sdlc:prd` and spec.
  No forced migration.
- `prd.md` and `prd.yaml` templates (v1) are renamed to `prd-v1.md.tmpl` on init so
  they're not overwritten.
- `/edikt:upgrade` adds v2 templates; does NOT migrate existing PRDs.

### FR-014: Plan-scoped context loading in `/edikt:context`

`/edikt:context --depth=focused` loads PRD and SPEC sidecars referenced in the **active plan phase**, not all active PRDs. The active plan is the orchestration signal for what is being worked on now. As plan phases complete and new phases activate, the loaded sidecar set changes automatically.

- Full depth: loads all PRDs (unchanged behavior).
- Focused depth: scans the current plan phase's objective, prompt, and Context Needed for `PRD-NNN` and `SPEC-NNN` identifiers; reads only those sidecars. Falls back to listing all PRD titles (no content) if no identifiers are found in the phase.
- Minimal depth: skips PRDs entirely (unchanged behavior).

This keeps the context budget lean and aligns loaded context with active work.

### FR-015: SPEC source flexibility — `source_prd`, `source_brainstorm`, `source_prompt`

`/edikt:sdlc:spec` accepts three source types, not just PRD-derived:

- **`source_prd: PRD-NNN`** — classic PRD-derived SPEC. Full FR coverage check (FR-007 seven changes apply).
- **`source_brainstorm: BRAIN-NNN`** — brainstorm-derived SPEC (technical exploration converged into a build). Coverage is advisory only: decisions and open questions from the brainstorm are surfaced as a checklist the SPEC should address. Back-reference: brainstorm doc receives `produced_specs: [SPEC-NNN]`.
- **`source_prompt: "..."`** — direct-prompt SPEC with no upstream artifact. No coverage check; evaluator judges the SPEC on its own merits. No back-reference written.

Source type is detected from the argument:
- `PRD-NNN` or path to PRD `.md` → `source_prd`
- `BRAIN-NNN` or path under `docs/brainstorms/` → `source_brainstorm`
- Free text or empty → `source_prompt`

The spec sidecar (`spec.yaml`) carries exactly one non-null source field:
```yaml
source_prd: PRD-001       # or
source_brainstorm: BRAIN-001  # or
source_prompt: "implement auth middleware refactor"
```

`templates/schemas/spec-sidecar.schema.json` validates this structure.

### FR-016: Plan frontmatter model with per-phase overrides

Plan files carry machine-readable YAML frontmatter with model assignments:

```yaml
---
type: plan
id: PLAN-{slug}
model: claude-sonnet-4-6      # plan-level default
phases:
  - id: 1
    model: claude-haiku-4-5-20251001   # per-phase override (cheaper/routine)
  - id: 2
    model: claude-opus-4-7             # per-phase override (complex/architectural)
---
```

Inheritance chain: per-phase `model` > plan-level `model` > `defaults.plan_model` in `.edikt/config.yaml` > `claude-sonnet-4-6`.

`/edikt:sdlc:plan` reads `defaults.plan_model:` from config when generating plans. Model assignment in the Phase Structure and Model Assignment table is recorded in both the frontmatter and the plan phase section (`**Model:** \`{model}\``).

This enables cost/quality optimization per phase and documents model choices as machine-readable plan metadata.

---

## Out of Scope

- Retroactive PRD flow (generating PRDs from shipped features)
- Cross-PRD dependency graph
- Community rubric sharing
- Auto-sync `.md` ↔ `.yaml` (user runs `/edikt:sdlc:prd:resync` explicitly)
- Plugin packaging / multi-platform support

---

## ADR-024 Decision Summary

**Decision:** PRDs use edit-in-place evolution, not ADR-style immutability.
**Rationale:** ADRs model discrete binary decisions. PRDs model continuous feature
evolution across releases. Forcing PRD immutability (supersede chain for every FR
change) creates artificial chains that LLMs handle worse than structured edit-in-place
per Anthropic harness findings. The asymmetry is intentional; it is not a gap.
**Scope:** PRDs and SPECs only. ADRs remain governed by INV-002.

---

## Acceptance Criteria

- [ ] `/edikt:sdlc:prd` creates `.md` + `.yaml` sidecar pair for a new PRD
- [ ] Sidecar contains FR-NNN, AC-NNN-M, protections, `_sync` block
- [ ] Five forcing questions asked and recorded before generation
- [ ] Rigor calibration produces correct optional sections
- [ ] `/edikt:sdlc:spec` emits `source_prd_coverage:` covering all FR-NNNs
- [ ] SPEC command writes `source_specs:` back to PRD sidecar
- [ ] `/edikt:prd:review` runs rubric and reports score + gaps
- [ ] `/edikt:spec:review` checks FR coverage + AC pass-through
- [ ] `/edikt:sdlc:discovery` produces DISCOVERY-NNN doc
- [ ] All four transition commands (`ship`, `supersede`, `deprecate`, `cancel`) work
- [ ] JSON Schema validates a generated sidecar
- [ ] `/edikt:doctor` reports broken refs, schema version, orphaned sidecars
- [ ] Existing v1 PRDs still load without error in spec command
- [ ] ADR-024 written and accepted
- [ ] `/edikt:context --depth=focused` loads only plan-phase-referenced PRD sidecars, not all PRDs (FR-014)
- [ ] `/edikt:sdlc:spec BRAIN-NNN` sets `source_brainstorm:` in spec sidecar and writes `produced_specs:` back to brainstorm (FR-015)
- [ ] `/edikt:sdlc:spec` with free-text arg sets `source_prompt:` in spec sidecar with no back-reference written (FR-015)
- [ ] Plan files written by `/edikt:sdlc:plan` include YAML frontmatter with `model:` and per-phase `model:` fields (FR-016)
- [ ] `templates/schemas/spec-sidecar.schema.json` validates `source_prd`/`source_brainstorm`/`source_prompt` fields (FR-015)
