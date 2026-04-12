# Reference: Phase Context Handoff

This document shows the concrete format additions to plan templates
from Phase 3 of the Long-Running Harness plan.

## Artifact Flow Table (new section in plan template)

Added between the dependency graph and Phase 1 details:

```markdown
## Artifact Flow

| Producing Phase | Artifact | Consuming Phase(s) |
|-----------------|----------|---------------------|
| 1 | `templates/agents/evaluator.md` (pre-flight section) | 5 |
| 2 | Updated progress table format | 3 (PostCompact hook) |
| 3 | Updated PostCompact hook | — (consumed by runtime) |
| 5 | `plan-criteria.yaml` schema | 6 (evaluator tuning) |
```

## Context Needed Field (new field per phase)

```markdown
## Phase 5: Structured Criteria Sidecar

**Objective:** ...
**Dependencies:** Phase 1
**Context Needed:**
- `templates/agents/evaluator.md` — pre-flight section added in Phase 1
- `docs/plans/artifacts/plan-criteria-schema.yaml` — reference schema
- `commands/sdlc/plan.md` — current plan command (to add sidecar emission)
- `docs/architecture/evaluator-tuning.md` — tuning framework (to understand integration point)
```

## PostCompact Hook Output (updated format)

Before (current):
```
Context recovered after compaction. Active plan: PLAN-foo.
Phase 3 — API handlers.
Invariants (2): INV-001, INV-012.
```

After (with context handoff):
```
Context recovered after compaction. Active plan: PLAN-foo.
Phase 3 — API handlers (attempt 2/5).
Last failing criteria: AC-3.2 (no tenant_id in log calls).
Before continuing, read:
  - docs/product/specs/SPEC-foo/contracts/api-orders.yaml
  - internal/repository/orders.go
  - docs/architecture/decisions/ADR-012.md
Invariants (2): INV-001, INV-012.
```

## Phase Startup Governance Directive

Added to plan execution rules:

```markdown
Before implementing any plan phase:
1. Read every file listed in that phase's Context Needed section.
2. If a listed file does not exist, check the progress table — the
   producing phase may not be complete.
3. Do not proceed until all context files have been read.
4. After reading, confirm you understand the relevant decisions and
   constraints before writing code.
```
