---
name: edikt:sdlc:spec
description: "Technical specification from an accepted PRD"
effort: high
argument-hint: "<PRD identifier, e.g. PRD-005>"
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Bash
  - Agent
---
!`bash -c 'CFG=""; D="$PWD"; while [ "$D" != "/" ]; do [ -f "$D/.edikt/config.yaml" ] && CFG="$D/.edikt/config.yaml" && break; D=$(dirname "$D"); done; [ -z "$CFG" ] && { printf "<!-- edikt:live -->\nNext SPEC number: SPEC-001\nExisting specs: (none yet)\n<!-- /edikt:live -->\n"; exit 0; }; PROOT=$(dirname "$(dirname "$CFG")"); REL=$(grep "^  specs:" "$CFG" 2>/dev/null | awk "{print \$2}" | tr -d "\""); if [ -z "$REL" ]; then BASE=$(grep "^base:" "$CFG" 2>/dev/null | awk "{print \$2}" | tr -d "\""); BASE="${BASE:-docs}"; REL="$BASE/product/specs"; fi; case "$REL" in /*) DIR="$REL" ;; *) DIR="$PROOT/$REL" ;; esac; COUNT=$(find "$DIR" -maxdepth 1 -type d -name "SPEC-*" 2>/dev/null | wc -l | tr -d " "); NEXT=$(printf "%03d" $((COUNT + 1))); EXISTING=$(find "$DIR" -maxdepth 1 -type d -name "SPEC-*" 2>/dev/null | sort | xargs -I{} basename {} | tr "\n" "," | sed "s/,$//"); printf "<!-- edikt:live -->\nNext SPEC number: SPEC-%s\nExisting specs: %s\n<!-- /edikt:live -->\n" "$NEXT" "${EXISTING:-(none yet)}"'`

# edikt:spec

Write a technical specification from an accepted PRD. The spec is the engineering response to a product requirement — it defines HOW to build what the PRD says to build.

CRITICAL: Check immediately whether you are in plan mode:
- If you are in plan mode (you can only describe actions, not perform them), output exactly this and stop:
  ```
  ⚠️  /edikt:sdlc:spec requires interactive input and cannot run in plan mode.
  Exit plan mode first, then run /edikt:sdlc:spec again.
  ```
- If you are not in plan mode, proceed normally with the spec generation.

## Arguments

- `$ARGUMENTS` — PRD identifier (e.g., `PRD-005`) or path to the PRD file

## Instructions

### 0. Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### 1. Resolve Paths

Read `.edikt/config.yaml`. Resolve paths from the `paths:` section:

- Specs: `paths.specs` (default: `docs/product/specs`)
- PRDs: `paths.prds` (default: `docs/product/prds`)
- Decisions: `paths.decisions` (default: `docs/architecture/decisions`)
- Invariants: `paths.invariants` (default: `docs/architecture/invariants`)
- Template override: check if `.edikt/templates/spec.md` exists — if yes, use it as the output template instead of the built-in template below

The correct next SPEC number is provided at the top of this prompt in the `<!-- edikt:live -->` block. Use it exactly.

### 2. Find and Validate the PRD

If `$ARGUMENTS` is a PRD identifier (e.g., `PRD-005`):
```bash
find {BASE}/product/prds/ -name "PRD-005*" -type f
```

If `$ARGUMENTS` is a path, read it directly.

Read the PRD file. Check the frontmatter `status:` field:
- If `status: accepted` → proceed
- If `status: draft` → block:
  ```
  ⛔ PRD-005 status is "draft".
     PRDs must be accepted before generating a spec.
     Review the PRD and change status to "accepted" first.
  ```
- If no frontmatter status → treat as accepted (backwards compatibility with pre-v4 PRDs)

### 2b. Detect PRD Version (v1 vs v2)

Check if a sidecar YAML exists next to the PRD `.md`:

```bash
PRD_MD="{path/to/PRD-NNN-slug.md}"
PRD_YAML="${PRD_MD%.md}.yaml"
[ -f "$PRD_YAML" ] && echo "v2" || echo "v1"
```

- **v2 PRD (sidecar exists):** The sidecar is source of truth for FRs, ACs, protections, and cross-references. Apply the SPEC-007 FR-007 flow described in Step 7b.
- **v1 PRD (no sidecar):** Legacy shape. Warn once:
  ```
  ⚠ PRD-NNN has no .yaml sidecar (v1 shape).
    Spec will be generated without FR coverage validation, stable ID
    propagation, or bidirectional traceability. To upgrade, re-author
    with /edikt:sdlc:prd PRD-NNN.
  ```
  Proceed with the legacy flow (skip Step 7b's FR coverage + back-reference steps).

Record the detected version as `$PRD_VERSION` (v1 | v2) for downstream steps.

### 3. Scan Codebase

Before asking questions, understand what exists. Run these in parallel:

```bash
# Architecture signals
ls .claude/rules/*.md 2>/dev/null
ls .claude/agents/*.md 2>/dev/null
ls {BASE}/decisions/*.md {BASE}/architecture/decisions/*.md 2>/dev/null
ls {BASE}/invariants/*.md {BASE}/architecture/invariants/*.md 2>/dev/null
```

Read the project-context.md for project identity and stack.

Read any relevant ADRs that might constrain the spec (match ADR titles against the PRD's topic).

### 4. Interview (batched presentation per Opus 4.7 guidance)

Present 2-4 codebase-specific questions in a **single batched message** — not sequentially. Batching respects the user's attention budget; sequential questioning inflates turn count with no quality gain for planning-phase interviews. The questions should prove you understood the project, not just the PRD.

Format each question with a label:
- `[required]` — blocking; the spec cannot be written without this decision
- `[optional — default: <inferred from codebase>]` — default applied silently if skipped

Good questions reference what you found in the codebase:
- `[required] The codebase has 3 ADRs about error handling. Should this spec follow ADR-002 (wrapped errors) or propose a different approach?`
- `[optional — default: same as existing] I see a hexagonal architecture with \`domain/\`, \`port/\`, \`adapter/\` layers. Should this feature follow the same pattern?`
- `[required] There's no existing test infrastructure for integration tests. Should the spec include setting that up?`

Bad questions are generic — skip these, you can infer the answer:
- "What language should we use?" (you can see the stack)
- "What's the project about?" (you read project-context.md)

Accept a single user reply covering any subset. Apply defaults for skipped `[optional]` items. Re-ask only `[required]` items that were not answered.

### 5. Show Outline

Before routing to agents, show what the spec will cover:

```
Based on the PRD and your answers, the spec will cover:
  - Architecture: {what architectural approach}
  - Key components: {what gets built or modified}
  - Data: {schema changes, models, or "no data changes"}
  - APIs: {new endpoints, contracts, or "no API changes"}
  - Breaking changes: {any, or "none"}
  - Open questions: {count carried from PRD}

Proceed? (y/n)
```

If the user says no, ask what to change and revise the outline.

### 6. Conflict Detection

Before generating, check if the spec would contradict any existing ADR:

```
⚠️  This spec proposes {X}.
    ADR-{NNN} states: "{relevant decision}".
    {Assessment: consistent / extends / contradicts}
    {If contradicts: Consider capturing a new ADR.}
```

Surface conflicts as warnings, not blockers. The user decides whether to proceed.

### 7. Generate the Spec

**Write with enforcement-grade language.** Requirements and acceptance criteria are checked by `/edikt:sdlc:drift` — vague requirements produce unverifiable drift reports.

**Scope guidance:** Define what to build at the scope level. Leave implementation granularity to plan phases. Over-specifying at spec level causes cascading errors when early phases diverge from the spec's assumptions.

Rules for spec requirements:
1. **Requirements use MUST/MUST NOT** — not "should" or "could."
2. **Each requirement is independently testable** — it can be verified by reading code, running a test, or checking a specific condition.
3. **Acceptance criteria are binary PASS/FAIL assertions** — not "system works correctly" or "API is fast enough." Each criterion must be verifiable by grepping, running a test, or checking a specific condition.
4. **Name specific types, endpoints, fields, or patterns** — not "the API should handle errors."
5. **Acceptance criteria flow downstream** — plans inherit them per phase. The evaluator checks them at phase-end. Write criteria that a fresh reviewer (with no shared context) can verify independently.

Route to `architect` + relevant domain specialists via the Agent tool.

Create `{specs_dir}/SPEC-{NNN}-{slug}/spec.md`:

```markdown
---
type: spec
id: SPEC-{NNN}
title: {Title}
status: draft
author: {git user.name}
implements: {PRD identifier}
architecture_source:   # optional: verikt.yaml if present
created_at: {ISO8601 timestamp}
references:
  adrs: [{list of referenced ADR IDs}]
  invariants: [{list of referenced invariant IDs}]
---

# SPEC-{NNN}: {Title}

**Implements:** {PRD identifier}
**Date:** {today}
**Author:** {git user.name}

---

## Summary

{One paragraph: what this spec proposes, why, and the high-level approach.}

## Context

{Why this spec exists now. What engineering context matters beyond the PRD.
Prior art, failed approaches, constraints discovered during investigation.}

## Existing Architecture

{What exists in the codebase that this spec builds on or modifies.
Reference specific files, patterns, and conventions. 3-5 sentences max.
Skip for greenfield projects.}

## Proposed Design

{The engineering design. How components interact. What layers are involved.}

## Components

{What gets built or modified. For each:
- What it does
- Where it lives (file paths)
- How it integrates with existing code}

## Non-Goals

{What this spec explicitly does NOT address.
Features deferred, approaches rejected, scope boundaries.}

## Alternatives Considered

### {Alternative 1}
- **Pros:** {benefits}
- **Cons:** {drawbacks}
- **Rejected because:** {specific reason}

### {Alternative 2}
- **Pros:** {benefits}
- **Cons:** {drawbacks}
- **Rejected because:** {specific reason}

## Risks & Mitigations

| Risk | Impact | Likelihood | Mitigation | Rollback |
|---|---|---|---|---|
| {risk} | {impact} | {likelihood} | {mitigation} | {rollback plan} |

## Security Considerations

{Auth, data access, encryption, input validation — or "None identified."}

## Performance Approach

{Expected load, caching, optimization — or "Standard patterns sufficient."}

## Acceptance Criteria

- AC-001: {Criterion} — Verify: {automated test, command, or review method}
- AC-002: {Criterion} — Verify: {method}

## Testing Strategy

{What to test, at which layer. What's hard to test and why.}

## Dependencies

{External systems, other specs, ADRs that constrain this design.}

## Open Questions

{Unresolved items. Flag gaps explicitly:}
- NEEDS CLARIFICATION: {question that must be resolved before implementation}

---

*Generated by edikt:spec — {date}*
```

A spec should be 200-400 lines. If longer, the feature should be split. The spec is the engineering response to a PRD — it defines HOW, not WHAT.

### 7b. v2 PRD flow — seven concrete changes (SPEC-007 FR-007)

Apply these seven changes when `$PRD_VERSION == v2` (sidecar exists). Skip them for v1 PRDs — they require sidecar data that doesn't exist in the legacy shape.

**Change 1: FR coverage check.**
Read every `FR-NNN` from the PRD sidecar `requirements:`. For each FR, the spec must do one of:
- Cover it with at least one `SR-NNN` that carries `implements: FR-NNN`
- Explicitly defer it with rationale

Compute and emit a sibling YAML file `{specs_dir}/SPEC-{NNN}-{slug}/spec.yaml` (or append to existing spec frontmatter) containing:

```yaml
source_prd_coverage:
  prd: PRD-NNN
  covered:
    - fr: FR-001
      by: [SR-001, SR-002]
    - fr: FR-002
      by: [SR-003]
  deferred:
    - fr: FR-004
      rationale: "Out of scope for this spec; tracked in SPEC-MMM"
  uncovered: []   # empty list required for PASS; non-empty blocks /edikt:sdlc:drift
```

Before writing the spec, display coverage to the user:

```
FR coverage:
  FR-001 → SR-001, SR-002 ✓
  FR-002 → SR-003 ✓
  FR-003 → deferred (rationale: {text})
  FR-004 → ❌ uncovered

Continue with 1 uncovered FR? (y/n)
```

If the user proceeds with uncovered FRs, record them in `uncovered:` — `/edikt:sdlc:drift` will flag them.

**Change 2: AC pass-through with stable IDs.**
Every `AC-NNN-M` from the PRD sidecar `acceptance_criteria:` appears verbatim in the spec's YAML with unchanged IDs. The Given/When/Then text is copied exactly. SPEC may ADD new acceptance criteria using `SAC-NNN` (spec acceptance criteria) for architectural layer checks, but MUST NOT renumber, rewrite, or merge PRD ACs.

```yaml
# In SPEC yaml front-matter or companion sidecar:
acceptance_criteria:
  # From PRD (pass-through, unchanged):
  - id: AC-001-1
    fr: FR-001
    given: "..."
    when: "..."
    then: "..."
    source: prd
  # Added by SPEC (architectural):
  - id: SAC-001
    source: spec
    given: "a SQL migration is deployed"
    when: "the migration is applied"
    then: "the affected tables retain byte-equal row counts"
```

**Change 3: Stable ID propagation.**
Every SPEC requirement gets `id: SR-NNN` (SPEC requirement) and, when derived from a PRD FR, `implements: FR-NNN`. Spec-only requirements (e.g., architectural constraints with no product equivalent) omit `implements:` or set `implements: null`.

In the `.md` narrative, requirements render as:
```
### SR-001 — Implements FR-001

{requirement text — MUST/MUST NOT language}
```

**Change 4: Back-reference emission.**
After successfully writing the spec, update the PRD sidecar's `source_specs:` to include this SPEC identifier. Append, don't overwrite (a PRD may have multiple specs covering different FR subsets).

Implementation (INV-003 compliant — untrusted values passed as argv, no shell interpolation):

```bash
PRD_YAML="{path}/PRD-NNN-slug.yaml"
SPEC_ID="SPEC-NNN"
AUTHOR=$(git config user.name 2>/dev/null || echo "unknown")
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)

python3 <<'PYEOF' "$PRD_YAML" "$SPEC_ID" "$AUTHOR" "$NOW"
import sys, yaml
path, spec_id, author, now = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]

with open(path) as f:
    data = yaml.safe_load(f) or {}

# Idempotent append — don't duplicate if already linked
source_specs = data.setdefault("source_specs", [])
if spec_id not in source_specs:
    source_specs.append(spec_id)

# Append revision_history record
data.setdefault("revision_history", []).append({
    "at": now,
    "author": author,
    "action": "edited",
    "note": f"Back-reference added: source_specs += {spec_id}",
    "affected": [spec_id],
})

# Clear _sync — caller recomputes hashes after this mutation
data.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})

with open(path, "w") as f:
    yaml.safe_dump(data, f, sort_keys=False)
PYEOF
```

The python3 heredoc above appends the revision_history record inline and clears `_sync` so the caller recomputes hashes.

Recompute `_sync` hashes (SHA-256 via `python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())'` with argv, per INV-003). Then Edit the sidecar's `_sync:` block with the new hashes and ISO8601 timestamp.

Output confirmation:
```
✅ Updated PRD-NNN sidecar: source_specs += SPEC-NNN
```

**Change 5: Solution reference pass-through.**
Read `solution_references:` from the PRD sidecar. Include each reference in the spec's "## Solution References" section. The spec may add architecture diagrams, sequence diagrams, or technical prototypes as additional references (annotate with `source: spec`).

**Change 6: Protection propagation.**
Read `protections:` from the PRD sidecar. Include every linked invariant and feature-scoped protection in the spec's "## Protections" section. The spec MAY add technical-layer protections (e.g., "MUST NOT hold a database transaction across an external API call") — annotate these with `source: spec` to distinguish from inherited PRD protections.

In the spec sidecar:
```yaml
protections:
  # Inherited from PRD:
  - ref: INV-003
    source: prd
  - id: SP-001
    text: "..."
    source: prd
  # Added by SPEC:
  - id: SSP-001  # Spec-Scoped Protection
    text: "..."
    source: spec
```

**Change 7: Evaluator hook.**
After the spec is generated, run the SPEC evaluator against `.edikt/rubrics/spec.md` (auto-create with sensible defaults if absent, per Step 7 of `/edikt:sdlc:prd` rubric bootstrap pattern).

Rigor threshold inherits from the PRD's rigor: solo=70%, team=80%, platform=90%.

If below threshold after 3 iterations, proceed but surface gaps in the final output.

---

REMEMBER: The spec must include Non-Goals (explicit scope exclusions), Alternatives Considered (with rejection reasons), and Acceptance Criteria (AC-NNN with verification methods). If anything is unclear, mark it NEEDS CLARIFICATION — never invent architectural decisions.

### 8. Confirm

```
✅ Spec created: {specs_dir}/SPEC-{NNN}-{slug}/spec.md

  SPEC-{NNN}: {Title}
  Implements: {PRD identifier}
  PRD version: {v1 | v2}
  Status: draft
  References: {count} ADRs, {count} invariants

  {if v2}
  FR coverage:  {covered}/{total} covered, {deferred} deferred, {uncovered} uncovered
  ACs:          {n} from PRD (pass-through) + {m} spec-added (SAC-NNN)
  Evaluator:    {score}/{total} ({PASS | below threshold})
  Back-ref:     Updated PRD-NNN sidecar: source_specs += SPEC-NNN

  Review and change status to "accepted" when ready.
  Next: Run /edikt:sdlc:artifacts for SPEC-{NNN}
  Re-score:     /edikt:spec:review SPEC-{NNN}
```
