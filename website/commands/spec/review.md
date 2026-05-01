# /edikt:spec:review

Re-score an existing SPEC against the rubric and validate it against its source PRD.

Use this before generating artifacts, before planning, or any time a SPEC has been edited outside `/edikt:sdlc:spec`. The five checks catch the failure modes that silently ship the wrong scope.

## Usage

```bash
/edikt:spec:review SPEC-005
```

The SPEC identifier is required. The command reads `SPEC-005/spec.md` and the spec sidecar (or YAML frontmatter), then resolves the linked PRD via the `implements:` field.

## What it checks

| Check | What it catches |
|-------|----------------|
| **Rubric score** | Same rubric `/edikt:sdlc:spec` uses at authoring time. Threshold inherits from the linked PRD's rigor. |
| **FR coverage** | Every PRD `FR-NNN` is covered by at least one spec `SR-NNN`, or explicitly deferred with rationale. |
| **AC pass-through** | Every PRD `AC-NNN-M` appears in the SPEC unchanged. Any rewording is a violation. |
| **Broken refs** | Linked ADRs, invariants, or PRDs that point to files that don't exist. |
| **Sidecar drift** | The `spec.md` was edited after the last sync. |

## Output

```text
SPEC REVIEW — SPEC-005

  Title:          Webhook delivery
  Implements:     PRD-005 (v2)
  Status:         accepted
  Rigor (from PRD): team

  Rubric score:   8/10 (team threshold: 8/10)
                  ✓ PASS

  FR coverage:    4/5 covered, 0 deferred, 1 uncovered
    ⚠ Uncovered FRs: FR-005

  AC pass-through: 7/8 unchanged
    ⚠ Modified ACs:
      • AC-001-1: given text changed from "..." to "..."

  ⚠ Broken references (1):
    • ADR-042 — not found in docs/architecture/decisions/

Next:
  • Revise SPEC:        /edikt:sdlc:spec PRD-005
  • Review linked PRD:  /edikt:prd:review PRD-005
  • Check drift:        /edikt:sdlc:drift SPEC-005
```

If everything is clean, the report ends with `✓ All checks pass.`

## FR coverage is the flagship check

A SPEC that passes the rubric but has uncovered PRD FRs is silently shipping the wrong scope. Coverage is the only structural link between PRD and SPEC that this command can verify.

For each requirement in the PRD's `requirements:`, the SPEC must do one of:

- Include it in `source_prd_coverage.covered[]` — at least one `SR-NNN` carries `implements: FR-NNN`
- Include it in `source_prd_coverage.deferred[]` with a non-empty rationale
- Include it in `source_prd_coverage.uncovered[]` — explicitly acknowledged as out-of-scope

Anything not mentioned is flagged as a missing coverage entry. The default for ambiguous gaps is fail — silent omission is the failure mode this check exists to catch.

## AC pass-through is strict

Every PRD `AC-NNN-M` must appear in the SPEC with the same `id` and byte-equal Given/When/Then text. SAC-NNN entries (spec-added architectural criteria) are additions, not modifications, and are ignored by this check.

If an AC needs clarification, revise the PRD and re-run the SPEC. Don't reword it in the SPEC — that breaks the chain the evaluator uses at phase-end.

## v1 PRDs limit what we can check

If the SPEC's source PRD is v1 (no sidecar), coverage and AC pass-through are not verifiable — there's no structured PRD to compare against. The rubric and broken-ref checks still run, but the report flags the limitation.

To upgrade the source PRD, re-author it with `/edikt:sdlc:prd PRD-NNN`.

## What's next

- [/edikt:sdlc:spec](/commands/sdlc/spec) — revise the SPEC
- [/edikt:prd:review](/commands/prd/review) — re-score the source PRD
- [/edikt:sdlc:drift](/commands/sdlc/drift) — check whether implementation matches the SPEC
- [PRD v2 Deep Dive](/guides/prd-v2) — how the FR/AC chain works end-to-end
