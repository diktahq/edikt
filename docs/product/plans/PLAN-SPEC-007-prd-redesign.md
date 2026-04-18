# Plan: SPEC-007 — PRD redesign (split artifact, rigor calibration, SDLC chain traceability)

## Overview

**Task:** Implement SPEC-007 on branch `0.6.0-dev`
**Implements:** SPEC-007
**Spec:** `docs/product/specs/SPEC-007-prd-redesign/spec.md`
**Total Phases:** 9
**Estimated Cost:** ~$1.30
**Created:** 2026-04-18

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | -      | -       | -       |
| 2     | -      | -       | -       |
| 3     | -      | -       | -       |
| 4     | -      | -       | -       |
| 5     | -      | -       | -       |
| 6     | -      | -       | -       |
| 7     | -      | -       | -       |
| 8     | -      | -       | -       |
| 9     | -      | -       | -       |

**IMPORTANT:** Update this table as phases complete. This table is the persistent state that survives context compaction.

## Model Assignment

| Phase | Task | Model | Reasoning | Est. Cost |
|-------|------|-------|-----------|-----------|
| 1 | ADR-024 — PRD lifecycle asymmetry | `haiku` | ADR writing, well-defined decision | $0.01 |
| 2 | Templates + JSON schema | `sonnet` | YAML schema design, template authoring | $0.08 |
| 3 | `/edikt:sdlc:prd` v2 rewrite | `opus` | Complex interview flow, sidecar generation, evaluator loop | $0.80 |
| 4 | `/edikt:sdlc:spec` v2 updates | `sonnet` | 7 concrete changes to existing command | $0.08 |
| 5 | Review commands (`prd:review`, `spec:review`) | `sonnet` | Two new commands, rubric evaluation pattern | $0.08 |
| 6 | `/edikt:sdlc:discovery` command | `sonnet` | New command, duplicate-not-wrap of brainstorm | $0.08 |
| 7 | Transition commands (ship/supersede/deprecate/cancel) | `haiku` | Structural YAML mutations, low complexity | $0.04 |
| 8 | Doctor integration (broken refs, schema, drift) | `sonnet` | Extension of existing doctor checks | $0.08 |
| 9 | CHANGELOG + test pass | `haiku` | Documentation + green-light verification | $0.02 |

## Execution Strategy

| Phase | Depends On | Parallel With | Wave |
|-------|-----------|---------------|------|
| 1     | —         | 2, 6          | 1    |
| 2     | —         | 1, 6          | 1    |
| 3     | 1, 2      | —             | 2    |
| 4     | 3         | 5, 7          | 3    |
| 5     | 3         | 4, 7          | 3    |
| 6     | —         | 1, 2          | 1    |
| 7     | 3         | 4, 5          | 3    |
| 8     | 4         | —             | 4    |
| 9     | all       | —             | 5    |

**Wave 1 (parallel):** 1, 2, 6 — independent foundations
**Wave 2:** 3 — prd command v2 (depends on ADR + templates)
**Wave 3 (parallel):** 4, 5, 7 — spec updates + review commands + transitions
**Wave 4:** 8 — doctor integration (depends on spec changes)
**Wave 5:** 9 — CHANGELOG + test pass

---

## Phase 1: ADR-024 — PRD lifecycle asymmetry

**Objective:** Write and accept ADR-024 capturing the decision that PRDs use
edit-in-place evolution rather than ADR-style immutability.
**Model:** `haiku`
**Max Iterations:** 2
**Completion Promise:** `PHASE 1 COMPLETE`
**Dependencies:** None

**Prompt:**
```
Implement Phase 1 of SPEC-007 on branch 0.6.0-dev.

Write docs/architecture/decisions/ADR-024-prd-lifecycle-asymmetry-vs-inv-002.md

Follow the six-section ADR format from ADR-009:
- Title: ADR-024 — PRD lifecycle asymmetry: edit-in-place vs INV-002 immutability
- Status: accepted
- Context: ADRs model discrete binary decisions (accepted once, immutable per INV-002).
  PRDs model continuous feature evolution — requirements get refined, scoped, shipped,
  deprecated over multiple releases. Applying INV-002-style immutability to PRDs
  forces supersession chains for routine feature changes that LLMs handle worse than
  structured edit-in-place (Anthropic harness findings: markdown corruption is higher
  with compound supersession chains; single-document context is more reliable).
- Decision: PRDs and SPECs use edit-in-place evolution with per-entry status markers
  (proposed | accepted | shipped | deprecated) and a structured revision_history: log.
  Supersede (create new PRD-NNN) is reserved for ≥50% scope rewrites or problem-framing
  shifts. ADRs remain governed by INV-002. The asymmetry is intentional.
- Consequences: PRDs accumulate history in-place; git diff is the audit trail.
  Evaluator scores shipped entries only when validating implementation. Old-shape PRDs
  (v1, no sidecar) continue to work; no forced migration.
- Directive sentinel block:
    paths: ["docs/product/prds/**", "docs/product/specs/**", "commands/sdlc/prd.md"]
    scope: [planning, design]
    directives:
      - PRDs MUST evolve via in-place YAML sidecar mutations with per-entry status markers. Use supersede only for ≥50% scope rewrites. (ref: ADR-024)
      - ADRs remain immutable per INV-002. The PRD edit-in-place model does NOT apply to ADRs. (ref: ADR-024)
    manual_directives: []
    suppressed_directives: []

When complete, output: PHASE 1 COMPLETE
```

---

## Phase 2: Templates + JSON schema

**Objective:** Create `templates/prd.md.tmpl` v2, `templates/prd.yaml.tmpl`, and
`templates/schemas/prd-sidecar.schema.json`. These are the canonical artifacts that
`/edikt:sdlc:prd` uses at generation time.
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `PHASE 2 COMPLETE`
**Dependencies:** None

**Prompt:**
```
Implement Phase 2 of SPEC-007 on branch 0.6.0-dev.

Read docs/product/specs/SPEC-007-prd-redesign/spec.md for the full FR-001–FR-013
requirements. Read the existing templates/prd.md.tmpl (v1 shape) to understand
what's there.

## Part A: templates/prd.md.tmpl (v2)

Rename existing templates/prd.md.tmpl → templates/prd-v1.md.tmpl (preserve v1).
Write new templates/prd.md.tmpl with this shape:

```markdown
# {{id}}: {{title}}

**Status:** {{status}}
**Rigor:** {{rigor}}
**Author:** {{author}}
**Created:** {{created_at}}
**Sidecar:** [{{id}}-{{slug}}.yaml]({{id}}-{{slug}}.yaml)

## Problem

{{problem_statement}}

## Users

{{user_archetypes}}

## Goals

| Metric | Target | Counter-metric |
|--------|--------|----------------|
| {{north_metric}} | {{target}} | {{counter}} |

## Non-Goals

- {{non_goal}}

## Requirements

<!-- Requirements are source-of-truth in the .yaml sidecar. This section mirrors them for readability. -->

{{requirements_mirror}}

## Acceptance Criteria

<!-- ACs are source-of-truth in the .yaml sidecar. Mirrored here for readability. -->

{{ac_mirror}}

## Solution References

{{solution_references}}

## Protections

<!-- What must NOT change when this ships. Linked invariants + feature-scoped protections. -->

{{protections}}

{{#if team_or_platform}}
## Stakeholders

{{stakeholders}}

## Dependencies

{{dependencies}}
{{/if}}

{{#if platform}}
## NFRs

{{nfrs}}

## Risk Register

{{risks}}
{{/if}}

## Open Questions

- {{open_question}}

---
*Sidecar: [{{id}}-{{slug}}.yaml](./{{id}}-{{slug}}.yaml) — source of truth for structured data.*
*To ship requirements: `/edikt:sdlc:prd:ship FR-NNN`*
*To review: `/edikt:prd:review {{id}}`*
```

## Part B: templates/prd.yaml.tmpl

Write templates/prd.yaml.tmpl with this exact shape (replace {{}} with template vars):

```yaml
# yaml-language-server: $schema=.edikt/schemas/prd-sidecar.schema.json
schema_version: "1.0"
type: prd
id: "{{id}}"
title: "{{title}}"
status: draft
rigor: "{{rigor}}"
author: "{{author}}"
created_at: "{{created_at}}"
requirements:
  - id: FR-001
    text: "{{fr_001_text}}"
    status: proposed
acceptance_criteria:
  - id: AC-001-1
    fr: FR-001
    given: "{{given}}"
    when: "{{when}}"
    then: "{{then}}"
    status: proposed
protections: []
solution_references: []
stakeholders: []
source_specs: []
revision_history:
  - at: "{{created_at}}"
    author: "{{author}}"
    note: "Initial draft"
extensions: {}
_sync:
  md_hash: ""
  yaml_hash: ""
  synced_at: ""
```

## Part C: templates/schemas/prd-sidecar.schema.json

Create the directory templates/schemas/ and write prd-sidecar.schema.json.
The schema must validate the prd.yaml.tmpl structure:
- Required: schema_version, type, id, title, status, rigor, author, created_at
- status enum: draft | accepted | shipped | evolving | superseded | deprecated | cancelled
- rigor enum: solo | team | platform
- requirements: array of {id (pattern FR-\d+), text, status}
- acceptance_criteria: array of {id (pattern AC-\d+-\d+), fr, given, when, then, status}
- protections: array of {ref: string} or {id: string, text: string}
- source_specs: array of strings
- revision_history: array of {at, author, note}
- extensions: object (additionalProperties: true)
- _sync: {md_hash, yaml_hash, synced_at}

Use JSON Schema draft 2020-12.

When complete, output: PHASE 2 COMPLETE
```

---

## Phase 3: `/edikt:sdlc:prd` v2 rewrite

**Objective:** Rewrite `commands/sdlc/prd.md` to generate split artifacts, run five
forcing questions, apply rigor calibration, fill protection section with invariant
auto-links, and run the generator-evaluator loop.
**Model:** `opus`
**Max Iterations:** 5
**Completion Promise:** `PHASE 3 COMPLETE`
**Dependencies:** Phases 1, 2

**Prompt:**
```
Implement Phase 3 of SPEC-007 on branch 0.6.0-dev.

Read:
- docs/product/specs/SPEC-007-prd-redesign/spec.md (FR-001 through FR-006, FR-013)
- templates/prd.md.tmpl (v2, written in Phase 2)
- templates/prd.yaml.tmpl (written in Phase 2)
- docs/architecture/decisions/ADR-024 (Phase 1)
- docs/brainstorms/BRAIN-001-prd-as-context-bundle/working-notes.md (design decisions)
- commands/sdlc/prd.md (current v1 implementation)
- commands/sdlc/plan.md (for evaluator loop pattern reference)

## Rewrite commands/sdlc/prd.md

The command becomes a structured authoring session producing TWO files. Keep the
shell preprocessing block (live PRD number injection) unchanged. All other content
is a full rewrite.

### Frontmatter

```
---
name: edikt:sdlc:prd
description: "Write a Product Requirements Document for a feature"
effort: high
argument-hint: "<feature description or PRD-NNN to continue>"
allowed-tools:
  - Read
  - Write
  - Edit
  - Bash
  - Glob
  - Grep
  - Agent
---
```

### Instruction flow

**Step 0: Config guard** — same as today.

**Step 1: Resolve context**
- Read .edikt/config.yaml. Resolve paths.
- If $ARGUMENTS starts with PRD-NNN: load existing PRD + sidecar for continuation.
- Else: this is a new PRD. Use live-injected PRD number.

**Step 2: Rigor triage** (one question, answered before anything else)
Ask: "Is this a solo project, a team feature, or a platform change? (solo / team / platform)"
Set rigor. Default solo if blank. This determines which sections appear.

**Step 3: Five forcing questions** (never skippable)
Ask all five, one at a time, waiting for answers:
1. "What's the problem *behind* this problem? Don't describe the solution — describe what breaks without it."
2. "How do you know someone has this problem? Paste evidence, or just say 'hypothesis only'."
3. "What single metric moves if this works? And what metric must NOT move?"
4. "What must NOT change when this ships?" — capture as Protections (see Step 4b)
5. "What's the riskiest assumption about this working?"

Record all answers. They seed the PRD body.

**Step 4a: Draft generation**
Using the forcing question answers + $ARGUMENTS description, draft the full PRD:
- `.md` narrative using templates/prd.md.tmpl v2
- `.yaml` sidecar using templates/prd.yaml.tmpl
- Assign FR-001, FR-002, ... sequentially from requirements identified
- Assign AC-001-1, AC-001-2, ... from acceptance criteria
- Fill Goals section from Q3 (north metric + counter)

**Step 4b: Protection section auto-link**
After drafting:
1. Grep docs/architecture/invariants/ for invariants related to the PRD's topic.
2. Present relevant INV-NNN items: "I found these invariants that may apply — confirm each:"
3. For any protection the user stated in Q4 that looks like a durable rule, say:
   "This protection looks like a durable invariant. Want me to create one?
   `/edikt:invariant:new` with: <protection text>"
4. Write confirmed refs to protections: in sidecar.

**Step 5: Evaluator loop**
Load (or auto-create) .edikt/rubrics/prd.md. Run the evaluator against the draft.
Threshold: solo=70%, team=80%, platform=90%.
If below threshold: show gaps, revise, re-evaluate. Max 3 iterations.
Use evaluator.mode from config (headless default per ADR-010).

**Step 6: Write files**
Write PRD-NNN-<slug>.md and PRD-NNN-<slug>.yaml to paths.prds directory.
Compute SHA-256 of each file, write to _sync block in sidecar.

**Step 7: Output summary**
```
✅ PRD-NNN created

  docs/product/prds/PRD-NNN-<slug>.md   — narrative
  docs/product/prds/PRD-NNN-<slug>.yaml — sidecar

  FRs:  {count} requirements
  ACs:  {count} acceptance criteria  
  Protections: {count} ({n} linked invariants, {m} feature-scoped)
  Evaluator: {score}% ({PASS|needs work})

Next: Run /edikt:sdlc:spec PRD-NNN to write the technical spec.
```

## INV compliance
- INV-001: This is a .md file (command). The prd.yaml sidecar written by the command
  is a data artifact, not a command — INV-001 does not restrict data files.
- INV-003/004: No hook JSON emission in this command.

When complete, output: PHASE 3 COMPLETE
```

---

## Phase 4: `/edikt:sdlc:spec` v2 updates

**Objective:** Add the seven SPEC-007 FR-007 changes to `commands/sdlc/spec.md`:
FR coverage check, AC pass-through, stable IDs, back-reference emission,
solution ref pass-through, protection propagation, evaluator hook.
**Model:** `sonnet`
**Max Iterations:** 4
**Completion Promise:** `PHASE 4 COMPLETE`
**Dependencies:** Phase 3

**Prompt:**
```
Implement Phase 4 of SPEC-007 on branch 0.6.0-dev.

Read:
- docs/product/specs/SPEC-007-prd-redesign/spec.md §FR-007
- commands/sdlc/spec.md (current implementation)
- docs/brainstorms/BRAIN-001-prd-as-context-bundle/working-notes.md §11

## Changes to commands/sdlc/spec.md

Add a version-detection branch early in the instruction flow:

```
After reading the PRD, check if a .yaml sidecar exists:
- IF PRD-NNN-<slug>.yaml exists → v2 path (apply all 7 changes below)
- ELSE → v1 path (proceed as today, warn once:
  "⚠ PRD-NNN has no .yaml sidecar (v1 shape). Spec will be generated without
  FR coverage validation or stable ID propagation. Run /edikt:sdlc:prd to
  regenerate with sidecar.")
```

### v2 path additions

**1. FR coverage check** (before spec generation)
- Read all FR-NNN from PRD sidecar requirements:
- For each FR-NNN, spec must include at least one SR-NNN with implements: FR-NNN
  OR a rationale for deferral.
- After spec generation, emit source_prd_coverage: in the spec's YAML:
  ```yaml
  source_prd_coverage:
    prd: PRD-NNN
    covered:
      - fr: FR-001
        by: [SR-001, SR-002]
    deferred:
      - fr: FR-003
        rationale: "Out of scope for this spec; tracked in SPEC-NNN"
    uncovered: []
  ```

**2. AC pass-through**
Copy all AC-NNN-M entries from PRD sidecar verbatim into spec YAML as
acceptance_criteria with unchanged IDs. SPEC adds SAC-NNN for architectural ACs.

**3. Stable ID propagation**
Each SPEC requirement gets: id: SR-NNN, implements: FR-NNN (or null for
spec-only requirements with no PRD FR match).

**4. Back-reference emission**
After writing the spec, update the PRD sidecar:
  source_specs: [SPEC-NNN]  # append, don't overwrite
Update _sync hashes after mutation.
Output: "✅ Updated PRD-NNN sidecar: source_specs → [SPEC-NNN]"

**5. Solution ref pass-through**
Read solution_references: from PRD sidecar. Include in spec under
"## Solution References" section. Spec can add architecture/sequence diagram refs.

**6. Protection propagation**
Read protections: from PRD sidecar. Include in spec under "## Protections" section.
Spec can add technical-layer protections (annotate with source: spec).

**7. Evaluator hook**
After spec generation, run evaluator against .edikt/rubrics/spec.md (auto-create
if absent). Threshold same as PRD: solo=70%, team=80%, platform=90%.

When complete, output: PHASE 4 COMPLETE
```

---

## Phase 5: Review commands

**Objective:** Create `commands/prd/review.md` (`/edikt:prd:review`) and
`commands/spec/review.md` (`/edikt:spec:review`). These close the audit gap
(every other governance artifact type has a review command).
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `PHASE 5 COMPLETE`
**Dependencies:** Phase 3

**Prompt:**
```
Implement Phase 5 of SPEC-007 on branch 0.6.0-dev.

Read:
- docs/product/specs/SPEC-007-prd-redesign/spec.md §FR-008
- commands/adr/review.md (existing review command — follow its pattern)
- docs/brainstorms/BRAIN-001-prd-as-context-bundle/working-notes.md §8

## Create commands/prd/review.md

Frontmatter:
  name: edikt:prd:review
  description: "Re-run PRD evaluator rubric on an existing PRD"
  effort: low
  argument-hint: "<PRD-NNN>"
  allowed-tools: [Read, Grep, Glob]

Instruction flow:
1. Config guard.
2. Resolve PRD path from $ARGUMENTS (e.g., PRD-001 → docs/product/prds/PRD-001-*.md).
3. Read .md and .yaml sidecar. If no sidecar: warn "v1 PRD — limited review available".
4. Load .edikt/rubrics/prd.md (auto-create if absent with sensible defaults).
5. Score PRD against rubric. Report:
   - Rubric score (n/total)
   - Gaps (which rubric items fail)
   - Sidecar drift: compare _sync.md_hash to actual SHA-256 of .md file.
     If mismatch: "⚠ .md file has changed since sidecar was last synced.
     Run /edikt:sdlc:prd:resync PRD-NNN to update hashes."
   - Broken refs: any linked INV-NNN that doesn't exist in invariants dir.
   - FR coverage: any FR-NNN with status: proposed and source_specs: empty
     (never specced) — flag as "unstarted requirements"
6. Output actionable improvement list:
   ```
   /edikt:prd:review PRD-001

   Score: 7/10
   Rigor: team

   Gaps:
     • Q3 (north metric) — not answered
     • Protections — empty (run /edikt:sdlc:prd to add)
   
   ⚠ Sidecar drift — .md changed 2026-04-17, sync: 2026-04-15
   ⚠ FR-003 — proposed, never specced

   2 broken refs:
     INV-999: not found
   ```

## Create commands/spec/review.md

Frontmatter:
  name: edikt:spec:review
  description: "Re-run SPEC evaluator and check PRD coverage"
  effort: low
  argument-hint: "<SPEC-NNN>"
  allowed-tools: [Read, Grep, Glob]

Instruction flow:
1. Config guard.
2. Resolve SPEC path.
3. Read spec (both files if v2).
4. If source_prd_coverage: present: check completeness.
   Report any FR-NNN with uncovered: not-empty.
5. If acceptance_criteria: present: verify all AC-NNN-M from linked PRD are present
   unchanged. Report any missing or modified ACs.
6. Run evaluator against .edikt/rubrics/spec.md.
7. Report score + gaps + broken refs.

When complete, output: PHASE 5 COMPLETE
```

---

## Phase 6: `/edikt:sdlc:discovery` command

**Objective:** Create `commands/sdlc/discovery.md` — structured discovery doc
peer to `/edikt:brainstorm`. Duplicated, not wrapped.
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `PHASE 6 COMPLETE`
**Dependencies:** None

**Prompt:**
```
Implement Phase 6 of SPEC-007 on branch 0.6.0-dev.

Read:
- docs/product/specs/SPEC-007-prd-redesign/spec.md §FR-009
- commands/brainstorm.md (for contrast — discovery is NOT a wrapper)
- docs/brainstorms/BRAIN-001-prd-as-context-bundle/working-notes.md §9

## Create commands/sdlc/discovery.md

Frontmatter:
  name: edikt:sdlc:discovery
  description: "Structured discovery doc — define what you know, what you don't, and how to find out"
  effort: medium
  argument-hint: "<description or BRAIN-NNN to lift>"
  allowed-tools: [Read, Write, Bash, Glob]

Shell preprocessing: inject next DISCOVERY number from docs/product/discovery/ or
docs/brainstorms/ directory (count DISCOVERY-NNN-* or BRAIN-NNN-* files).

Instruction flow:

1. Config guard.
2. If $ARGUMENTS is a BRAIN-NNN: load the existing brainstorm doc as seed content.
   Else: start fresh from the description.
3. Interview (4 questions — these differ from PRD forcing questions):
   a. "What do you know for certain? (facts, data, prior research)"
   b. "What are you most uncertain about? (ordered by impact on the decision)"
   c. "What would change your mind? (kill criteria — if X, we stop)"
   d. "What's the smallest experiment that reduces the biggest uncertainty?"
4. Generate DISCOVERY-NNN-<slug>.md with sections:
   - Context (what this is about)
   - Known (from Q3a)
   - Unknown (from Q3b, ordered by impact)
   - Kill Criteria (from Q3c)
   - Discovery Plan (from Q3d — table: Experiment | Method | Success Signal | Timeline)
   - Assumptions Register (list assumptions with confidence level)
   - Outcome (blank at creation; filled when discovery concludes)
5. Write to docs/product/discovery/ (create dir if absent, add to .gitignore default).
6. Output:
   ```
   ✅ DISCOVERY-001 created: docs/product/discovery/DISCOVERY-001-<slug>.md

   Key uncertainties: {count}
   Kill criteria: {count}
   Discovery plan: {count} experiments

   When ready to build: /edikt:sdlc:prd DISCOVERY-001
   ```

Note: /edikt:sdlc:prd should accept a DISCOVERY-NNN argument and pre-populate
PRD sections from discovery content (add this capability to Phase 3's prd command
if it wasn't included — check prd.md).

When complete, output: PHASE 6 COMPLETE
```

---

## Phase 7: Transition commands

**Objective:** Create four transition commands: `commands/sdlc/prd/ship.md`,
`commands/sdlc/prd/supersede.md`, `commands/sdlc/prd/deprecate.md`,
`commands/sdlc/prd/cancel.md`.
**Model:** `haiku`
**Max Iterations:** 3
**Completion Promise:** `PHASE 7 COMPLETE`
**Dependencies:** Phase 3

**Prompt:**
```
Implement Phase 7 of SPEC-007 on branch 0.6.0-dev.

Read docs/product/specs/SPEC-007-prd-redesign/spec.md §FR-010.

Create the directory commands/sdlc/prd/ if it doesn't exist.
Create these four files:

## commands/sdlc/prd/ship.md

name: edikt:sdlc:prd:ship
description: "Mark PRD requirements as shipped"
effort: low
argument-hint: "<PRD-NNN> [FR-001 FR-002 ...]"
allowed-tools: [Read, Edit, Bash]

Instructions:
1. Config guard.
2. Resolve PRD sidecar from $ARGUMENTS.
3. If specific FR-NNN args given: mark only those as status: shipped.
   If no FRs given: show list of non-shipped FRs and ask which to mark.
4. Update revision_history: with {at: now, author: git-user, note: "Marked FR-NNN shipped"}.
5. If all FRs are now shipped: set top-level status: shipped.
6. Recompute _sync hashes. Write sidecar.
7. Output: "✅ PRD-NNN: FR-001, FR-002 → shipped. Status: shipped"

## commands/sdlc/prd/supersede.md

name: edikt:sdlc:prd:supersede
description: "Supersede a PRD with a new one (≥50% scope change)"
effort: medium
argument-hint: "<PRD-NNN-to-supersede>"
allowed-tools: [Read, Write, Edit, Bash]

Instructions:
1. Config guard.
2. Load the PRD to supersede. Confirm: "PRD-NNN will be superseded. Continue? (y/n)"
3. Run /edikt:sdlc:prd flow to create new PRD (pre-populate from old PRD as seed).
4. After new PRD is created (PRD-MMM):
   - Set old sidecar: status: superseded, superseded_by: PRD-MMM, superseded_at: now.
   - Set new sidecar: supersedes: PRD-NNN.
   - Update revision_history on both.
5. Output: "✅ PRD-NNN superseded by PRD-MMM"

## commands/sdlc/prd/deprecate.md

name: edikt:sdlc:prd:deprecate
description: "Deprecate a PRD (feature abandoned or no longer relevant)"
effort: low
argument-hint: "<PRD-NNN> [reason]"
allowed-tools: [Read, Edit, Bash]

Instructions:
1. Config guard.
2. Read reason from $ARGUMENTS or ask: "Why is this being deprecated?"
3. Set sidecar: status: deprecated, deprecated_at: now, deprecated_reason: <reason>.
4. Update revision_history.
5. Recompute _sync hashes. Write sidecar.
6. Output: "✅ PRD-NNN deprecated. Reason: <reason>"

## commands/sdlc/prd/cancel.md

name: edikt:sdlc:prd:cancel
description: "Cancel a PRD (work stopped before shipping)"
effort: low
argument-hint: "<PRD-NNN> [reason]"
allowed-tools: [Read, Edit, Bash]

Instructions:
1. Config guard.
2. Read reason from $ARGUMENTS or ask: "Why is this being cancelled?"
3. Set sidecar: status: cancelled, cancelled_at: now, cancelled_reason: <reason>.
4. Update revision_history.
5. Recompute _sync hashes. Write sidecar.
6. Output: "✅ PRD-NNN cancelled. File kept as historical record."

Note: For all transition commands, SHA-256 hashing is done via:
  python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' <path>
This is INV-003 compliant (python3 with argv, no shell interpolation).

When complete, output: PHASE 7 COMPLETE
```

---

## Phase 8: Doctor integration

**Objective:** Add four new `/edikt:doctor` checks: broken refs, schema version,
sidecar drift, and orphaned sidecars (FR-012).
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `PHASE 8 COMPLETE`
**Dependencies:** Phase 4

**Prompt:**
```
Implement Phase 8 of SPEC-007 on branch 0.6.0-dev.

Read:
- docs/product/specs/SPEC-007-prd-redesign/spec.md §FR-012
- commands/doctor.md (current implementation)

Add a new "PRD/SPEC artifact health" section to commands/doctor.md after the
existing fixture characterization check. Four checks in sequence:

### Check: Orphaned sidecars
Find docs/product/prds/*.yaml without a matching *.md and vice versa.
Report each pair with ERROR severity.

### Check: Schema version
Read schema_version from each .yaml sidecar. If absent or not "1.0":
  WARN: "PRD-NNN sidecar has no schema_version (v1 shape). No action required."
Note: v1 PRDs (no .yaml at all) are silently skipped.

### Check: Sidecar drift
For each PRD-NNN-<slug>.yaml with a non-empty _sync.md_hash:
  - Compute actual SHA-256 of PRD-NNN-<slug>.md
  - Compare to _sync.md_hash
  - If mismatch: WARN: "PRD-NNN: .md edited since last sync ({date}).
    Run /edikt:sdlc:prd:resync PRD-NNN to update."

### Check: Broken refs
For each PRD sidecar's protections:, source_specs:, supersedes:, superseded_by::
  - INV-NNN ref: verify file exists in invariants dir. ERROR if missing.
  - SPEC-NNN ref: verify directory exists in specs dir. WARN if missing.
  - solution_references path_or_url starting with /: verify path exists. WARN if missing.
  - Figma URLs (figma.com): skip (network check opt-in only).

Output format (append to existing doctor output):
```
PRD/SPEC ARTIFACT HEALTH
  Orphaned sidecars: {n}
  Schema gaps: {n} (v1 shape, no action needed)
  Sidecar drift: {n}
  Broken refs: {n}

  {error/warn details}
```

When complete, output: PHASE 8 COMPLETE
```

---

## Phase 9: CHANGELOG + test pass

**Objective:** Add SPEC-007 section to CHANGELOG. Update ROADMAP to mark
BRAIN-001-prd as implemented. Run full test suite and confirm green.
**Model:** `haiku`
**Max Iterations:** 3
**Completion Promise:** `PHASE 9 COMPLETE`
**Dependencies:** All phases

**Prompt:**
```
Implement Phase 9 of SPEC-007 on branch 0.6.0-dev.

Read:
- CHANGELOG.md (the v0.6.0 section at the top — append to it, don't replace)
- docs/internal/plans/ROADMAP.md (find BRAIN-001-prd entry, mark implemented)
- docs/product/plans/PLAN-SPEC-007-prd-redesign.md (this file — mark all phases ✅)

## Part A: CHANGELOG

Append to the existing v0.6.0 section (after the SPEC-006 content, before the
v0.5.0 section). Add:

### PRD redesign — split artifact, rigor calibration, SDLC chain traceability (SPEC-007)

Summarize all 13 FRs from SPEC-007. Key bullets:
- Split artifact (.md + .yaml sidecar)
- Rigor calibration (solo/team/platform)
- Five forcing questions
- Protection section with invariant auto-link
- Stable IDs (FR-NNN, AC-NNN-M, SR-NNN) through chain
- Generator-evaluator loop at PRD and SPEC level
- /edikt:prd:review + /edikt:spec:review
- /edikt:sdlc:discovery command
- Transition commands (ship/supersede/deprecate/cancel)
- Doctor integration (broken refs, schema, drift, orphans)
- JSON Schema for sidecar (IDE autocomplete)
- ADR-024 — PRD lifecycle asymmetry (edit-in-place vs INV-002)
- Migration: grandfather (v1 PRDs continue to work)

## Part B: ROADMAP update

In docs/internal/plans/ROADMAP.md, find the BRAIN-001-prd entry in the source
document index table and update its status from "Draft" to "Implemented — SPEC-007".

## Part C: Plan progress table

Update all 9 rows in PLAN-SPEC-007-prd-redesign.md to ✅ pass.

## Part D: Test pass

Run: bash test/run.sh

If any tests fail, diagnose and fix before marking this phase complete.

When complete, output: PHASE 9 COMPLETE
```

---

## Known Risks

- **prd.yaml.tmpl is a data artifact, not a command.** INV-001 governs commands (`.md`
  files under `commands/`). Template data files under `templates/` are not commands —
  the `.yaml` sidecar extension is valid. This is consistent with existing
  `templates/settings.json.tmpl`.
- **Evaluator loop at PRD level is new territory.** The evaluator was previously only
  used at plan/phase level. The rubric `.edikt/rubrics/prd.md` is auto-created on
  first run — it will be generic. Review it after first use.
- **Back-reference write to PRD sidecar.** The spec command writing back to the PRD
  sidecar crosses a file ownership boundary. Use EDIKT_COMPILE_IN_PROGRESS=1 if the
  pre-tool-use hook blocks the write (the sidecar is not a managed region per INV-005,
  but double-check).
