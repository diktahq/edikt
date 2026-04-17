---
type: adr
id: ADR-018
title: Evaluator verdict schema — structured JSON with per-criterion evidence_type
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-04-17T00:00:00Z
references:
  adrs: [ADR-010]
  invariants: [INV-001]
  prds: []
  specs: [SPEC-005]
---

# ADR-018: Evaluator verdict schema — structured JSON with per-criterion evidence_type

**Status:** Accepted
**Date:** 2026-04-17
**Decision-makers:** Daniel Gomes

---

## Context and Problem Statement

The v0.5.0 security audit (2026-04-17) rated HI-7 on the fact that the headless evaluator (`templates/agents/evaluator-headless.md`) emitted a verdict as unstructured prose. The agent's capability self-check — "if the criterion names a shell command and Bash is not available, return BLOCKED" — was a soft instruction in the agent body with no schema enforcement. Under generator pressure (a parent session that just finished implementing a phase and wants to mark it done), the evaluator could rationalize a PASS based on read-only inspection and the plan's progress table would flip the phase to `done` with no test actually having run.

The audit also identified HI-6 — benchmark scoring bypassable via Unicode/whitespace variants — as a related case of soft enforcement. Both share the root cause: the evaluator's output is not machine-checkable, so downstream harnesses cannot enforce rules about what evidence the PASS rests on.

This ADR replaces the prose verdict with a structured JSON schema that makes the evaluator's reasoning auditable, and enforces a rule that criteria naming a shell command must PASS only with `evidence_type: "test_run"`.

## Decision Drivers

- Evaluator verdicts gate plan progress. False PASS verdicts silently ship unfinished work.
- A schema-checkable verdict is auditable; prose is not.
- Existing in-flight plans have verdicts recorded under the old prose format. A hard cutover would regress every in-progress plan to BLOCKED on the next evaluation.
- The schema must be small enough to hand-author in a markdown agent prompt (the evaluator must be able to produce it reliably without a library call).

## Considered Options

1. **Structured JSON verdict + evidence_type enforcement + grandfather existing verdicts** — the chosen approach.
2. **Hard cutover** — every existing PASS is discarded on upgrade; every phase re-evaluates.
3. **Prose verdict with a lint pass** — parse prose for keyword patterns ("Bash unavailable", "BLOCKED"). Fragile.
4. **Require two evaluators to agree** — expensive, mostly orthogonal to the core issue.

## Decision

### Verdict schema

The evaluator emits a single JSON object conforming to `templates/agents/evaluator-verdict.schema.json`:

```json
{
  "verdict": "PASS" | "BLOCKED" | "FAIL",
  "criteria": [
    {
      "id": "<criterion-id from the plan sidecar>",
      "status": "met" | "unmet" | "blocked",
      "evidence_type": "test_run" | "grep" | "file_read" | "manual",
      "evidence": "<one-line evidence string>",
      "notes": "<optional longer note>"
    }
  ],
  "meta": {
    "evaluator_mode": "headless" | "interactive",
    "grandfathered": false,
    "migrated_from": null
  }
}
```

### Evidence gate

The plan harness (`commands/sdlc/plan.md` + `templates/hooks/phase-end-detector.sh`) enforces:

- For every criterion whose `id` references a shell command in the plan's criteria sidecar (patterns: `pytest`, `bash`, `make`, `npm test`, `./test/`, `uv run`), the verdict's corresponding criterion MUST have `evidence_type: "test_run"`.
- If any required-test criterion has a different evidence_type, the overall verdict is forced to BLOCKED regardless of the evaluator's stated verdict, and the reason is logged with the unmet criterion IDs.
- `evidence_type: "grep"`, `"file_read"`, `"manual"` are valid for criteria that do not name a test command.

### Verdict storage

Structured verdicts persist as runtime state sidecars at `docs/product/plans/verdicts/<plan-slug>/<phase-id>.json`. This is a new on-disk location; it is runtime state, not governance, so INV-001 (commands and templates are markdown/yaml) does not apply. Verdicts are JSON because they are machine output intended for programmatic reading.

### Schema versioning

The verdict schema follows the same versioning discipline as ADR-007 (compile schema version). `templates/agents/evaluator-verdict.schema.json` carries a top-level `$id` field of the form `https://edikt.dev/schemas/evaluator-verdict/v<N>.json`. Additive changes (new optional fields, new enum values) do NOT bump `<N>`. Breaking changes (removed fields, changed enum semantics, required-field additions) bump `<N>` and ship with a migration routine in `bin/edikt upgrade`. Old verdicts (identified by their `$schema` URL) are never silently reinterpreted under a new schema version.

### Grandfathering

On upgrade from edikt < 0.5.0 to 0.5.0:
- For every phase currently marked `done` in any plan under `docs/product/plans/` or `docs/plans/`, the upgrade writes a stub verdict at `docs/product/plans/verdicts/<plan>/<phase>.json` with `meta.grandfathered: true` and `meta.migrated_from: "<prior-version>"`. Every criterion's `status` is set to `met` with `evidence_type: "manual"` and `evidence: "grandfathered from <prior-version> — pre-schema verdict"`.
- Grandfathered verdicts bypass the evidence gate on a one-time basis per phase.
- New evaluations (future phase completions) use the strict schema.
- The upgrade emits a banner: "N in-flight plan phases were grandfathered; new verdicts will use the structured schema."

## Alternatives Considered

### Hard cutover
- **Pros:** Clean posture immediately; no grandfather logic to maintain.
- **Cons:** Every live plan regresses. Users mid-release get dumped into re-evaluation. Adoption friction.
- **Rejected because:** a one-time grandfather is a cheap way to avoid the friction.

### Prose verdict with lint
- **Pros:** Smaller blast radius — no new artifacts.
- **Cons:** Fragile; false positives and false negatives on keyword matching; doesn't solve the fundamental "no machine-checkable evidence" problem.
- **Rejected because:** the audit finding is specifically about machine-checkability.

### Dual evaluator
- **Pros:** Catches evaluator drift.
- **Cons:** Doubles eval cost; doesn't prevent both evaluators from rationalizing the same PASS.
- **Rejected because:** the core issue is structural, not agreement-based.

## Consequences

- **Good:** Audit finding HI-7 closes. PASS without test evidence is forced to BLOCKED.
- **Good:** Evaluator output becomes auditable. `docs/product/plans/verdicts/` is a reviewable artifact directory.
- **Good:** Benchmark scoring is cleaned up in a related Phase 10 (NFKC normalization) addressing HI-6.
- **Bad:** Evaluator prompts get longer — the agent must emit structured JSON instead of prose. Token cost increases modestly per evaluation.
- **Bad:** One new directory (`docs/product/plans/verdicts/`). Mentioned here rather than a separate ADR to avoid governance bloat.
- **Neutral:** Grandfather stubs are identifiable — anyone can grep for `grandfathered: true` and see what was imported without re-verification.

## Confirmation

- `templates/agents/evaluator-verdict.schema.json` exists and validates against draft 2020-12 JSON Schema.
- `templates/agents/evaluator-headless.md` and `templates/agents/evaluator.md` instruct the agent to emit conforming JSON.
- `test/security/evaluator/test_evidence_gate.sh` passes:
  - A fixture verdict with a `pytest` criterion and `evidence_type: "test_run"` → PASS preserved.
  - A fixture verdict with a `pytest` criterion and `evidence_type: "grep"` → forced to BLOCKED.
  - A grandfathered verdict → evidence gate bypassed.
- `bin/edikt upgrade` on a v0.4.3 snapshot emits the grandfather banner with a non-zero count.

## Directives

[edikt:directives:start]: #
source_hash: pending
directives_hash: pending
compiler_version: "0.4.3"
paths:
  - "templates/agents/evaluator.md"
  - "templates/agents/evaluator-headless.md"
  - "templates/agents/evaluator-verdict.schema.json"
  - "templates/hooks/phase-end-detector.sh"
  - "commands/sdlc/plan.md"
  - "docs/product/plans/verdicts/**"
scope:
  - implementation
  - review
directives:
  - Evaluator agents MUST emit a single JSON object conforming to `templates/agents/evaluator-verdict.schema.json`. NEVER emit prose verdicts. (ref: ADR-018)
  - The plan harness MUST reject PASS unless every criterion whose id references a shell command has `evidence_type: "test_run"`. Mismatch forces the verdict to BLOCKED with a listed reason. (ref: ADR-018)
  - Verdicts are persisted at `docs/product/plans/verdicts/<plan>/<phase>.json`. This directory is runtime state, not governance, and is exempt from INV-001's markdown/yaml rule. (ref: ADR-018)
  - Upgrade from < 0.5.0 MUST grandfather existing `done` phases by writing verdict stubs with `meta.grandfathered: true`. Grandfathered verdicts bypass the evidence gate on a one-time basis. (ref: ADR-018)
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #

---

*Captured by edikt:adr — 2026-04-17*
