---
type: adr
id: ADR-025
title: Evaluator verdict storage uses config-driven path
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-05-01T00:00:00Z
references:
  adrs: [ADR-018, ADR-010]
  invariants: [INV-001, INV-002]
  prds: []
  specs: []
supersedes: ADR-018
---

# ADR-025 — Evaluator verdict storage uses config-driven path

## Status

**Accepted**

## Context

ADR-018 introduced the structured evaluator verdict schema, the per-criterion evidence gate, and grandfathering for upgrades from < 0.5.0. Those decisions are correct and remain in force here.

ADR-018 also specified a verdict storage location:

> "Verdicts are persisted at `docs/product/plans/verdicts/<plan>/<phase>.json`. This directory is runtime state, not governance, and is exempt from INV-001's markdown/yaml rule."

The path was written as a literal string because, at authorship time (2026-04-17), this dogfooded project's `paths.plans` was `docs/product/plans/`. The storage rule was a minor sub-decision in §"Verdict storage" — and ADR-018's own §Consequences notes "Mentioned here rather than a separate ADR to avoid governance bloat," which is the moment the implicit coupling to a project-specific path got encoded as a global directive.

The bug is that `paths.plans` is configurable in `.edikt/config.yaml`. A project that customizes `paths.plans` (e.g., `docs/internal/plans/`) cannot satisfy ADR-018 directive #3 without violating its own config. ADR-018 reads as if the path were governance; in fact, only the *relationship* between plans and verdicts (verdicts live next to plans) is governance. The literal path is project state.

This ADR supersedes ADR-018 to fix that single defect. All other parts of ADR-018 — the verdict schema, the evidence gate, schema versioning, grandfathering, and the directives that follow from them — are restated here verbatim so the supersede chain replaces the full set in one step rather than leaving a partial directive set live under the old ADR ID.

## Decision Drivers

- The verdict storage path MUST resolve from `.edikt/config.yaml` so that every edikt project — default convention or custom — can comply without violating its own config.
- The verdict schema, evidence gate, and grandfathering rules from ADR-018 are correct and proven (covered by `test/security/evaluator/test_evidence_gate.sh`). They are restated, not redesigned.
- Supersession is the right mechanism per INV-002 — the original directive is wrong as written and cannot be edited in place.

## Decision

### Verdict schema (unchanged from ADR-018)

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

### Evidence gate (unchanged from ADR-018)

The plan harness (`commands/sdlc/plan.md` + `templates/hooks/phase-end-detector.sh`) enforces:

- For every criterion whose `id` references a shell command in the plan's criteria sidecar (patterns: `pytest`, `bash`, `make`, `npm test`, `./test/`, `uv run`), the verdict's corresponding criterion MUST have `evidence_type: "test_run"`.
- If any required-test criterion has a different `evidence_type`, the overall verdict is forced to BLOCKED regardless of the evaluator's stated verdict, and the reason is logged with the unmet criterion IDs.
- `evidence_type: "grep"`, `"file_read"`, `"manual"` are valid for criteria that do not name a test command.

### Verdict storage (corrected)

Structured verdicts persist as runtime state sidecars at `<paths.plans>/verdicts/<plan-slug>/<phase-id>.json`, where `<paths.plans>` is the value of `paths.plans` in `.edikt/config.yaml`. Concrete resolutions:

- Default edikt convention (`paths.plans: docs/plans`) → `docs/plans/verdicts/<plan>/<phase>.json`
- This dogfooded project (`paths.plans: docs/internal/plans`) → `docs/internal/plans/verdicts/<plan>/<phase>.json`
- Any project with custom `paths.plans` → the verdicts directory resolves alongside their plans directory

Verdicts are runtime state, not governance. INV-001 (commands and templates are markdown/yaml only) does NOT apply. Verdicts are JSON because they are machine output intended for programmatic reading.

Tooling that reads or writes verdicts MUST resolve the path through the loaded `paths.plans` config value. NEVER hardcode a literal verdicts path.

### Schema versioning (unchanged from ADR-018)

The verdict schema follows the same versioning discipline as ADR-007. `templates/agents/evaluator-verdict.schema.json` carries a top-level `$id` of the form `https://edikt.dev/schemas/evaluator-verdict/v<N>.json`. Additive changes (new optional fields, new enum values) do NOT bump `<N>`. Breaking changes (removed fields, changed enum semantics, required-field additions) bump `<N>` and ship with a migration routine in `bin/edikt upgrade`. Old verdicts (identified by their `$schema` URL) are never silently reinterpreted under a new schema version.

### Grandfathering (corrected to use resolved path)

On upgrade from edikt < 0.5.0 to ≥ 0.5.0:

- For every phase currently marked `done` in any plan under the project's resolved `paths.plans` directory, the upgrade writes a stub verdict at `<paths.plans>/verdicts/<plan>/<phase>.json` with `meta.grandfathered: true` and `meta.migrated_from: "<prior-version>"`. Every criterion's `status` is set to `met` with `evidence_type: "manual"` and `evidence: "grandfathered from <prior-version> — pre-schema verdict"`.
- Grandfathered verdicts bypass the evidence gate on a one-time basis per phase.
- New evaluations (future phase completions) use the strict schema.
- The upgrade emits a banner: "N in-flight plan phases were grandfathered; new verdicts will use the structured schema."

If an existing project still has verdicts under the legacy `docs/product/plans/verdicts/` path (because they predate this ADR) and no longer matches the project's `paths.plans`, `bin/edikt upgrade` MUST move those legacy verdicts to the resolved `<paths.plans>/verdicts/` location before running grandfathering, preserving filenames and contents byte-for-byte.

## Alternatives Considered

### Edit ADR-018 in place to fix the directive

- **Pros:** No new ADR; one fewer document.
- **Cons:** Violates INV-002 (accepted ADRs are immutable). Breaks the audit trail.
- **Rejected because:** INV-002 exists for exactly this case — when an accepted decision turns out to be wrong, supersession is the mechanism.

### Add an erratum note to ADR-018

- **Pros:** Smaller surface than a full supersede.
- **Cons:** Still mutates ADR-018; still violates INV-002. And erratum-style metadata is not a recognized concept in edikt's ADR model.
- **Rejected because:** same reason as above.

### Leave ADR-018 as-is and add an exemption clause to the directive

- **Pros:** No new ADR.
- **Cons:** "ADR-018 directive #3 applies unless the project has overridden `paths.plans`" is harder to reason about than a single corrected directive.
- **Rejected because:** the original directive should never have been path-literal in the first place. Patching with exemptions papers over the defect.

## Consequences

- **Good.** Every edikt project — default convention or custom — can satisfy the verdict storage directive without violating its own config.
- **Good.** Tooling and CI that resolve paths through `paths.plans` no longer have a special-case fallback for the verdicts directory.
- **Good.** ADR-018's substantive contributions (schema, evidence gate, grandfathering) remain in force; only the path-literal defect is corrected.
- **Bad.** One additional ADR in the supersede chain. Future readers of ADR-018 must follow the supersede pointer to ADR-025 to find the active directive set.
- **Mitigation.** `bin/edikt upgrade` includes a one-time migration step that moves any legacy `docs/product/plans/verdicts/` content to the resolved `<paths.plans>/verdicts/` for projects that customized `paths.plans` after the original ADR-018 ship.
- **Neutral.** The ADR-018 grandfathering banner wording is unchanged; only the path it references is now resolved from config.

## Confirmation

- `templates/agents/evaluator-verdict.schema.json` exists, validates against draft 2020-12 JSON Schema, and is byte-identical to the ADR-018 ship.
- `templates/agents/evaluator-headless.md` and `templates/agents/evaluator.md` instruct the agent to emit conforming JSON. No path literals in these templates.
- `test/security/evaluator/test_evidence_gate.sh` continues to pass:
  - A fixture verdict with a `pytest` criterion and `evidence_type: "test_run"` → PASS preserved.
  - A fixture verdict with a `pytest` criterion and `evidence_type: "grep"` → forced to BLOCKED.
  - A grandfathered verdict → evidence gate bypassed.
- `bin/edikt upgrade` on a v0.4.3 snapshot emits the grandfather banner with a non-zero count AND writes verdicts under the resolved `paths.plans` value, not a hardcoded path.
- `grep -rn 'docs/product/plans/verdicts' templates/ commands/ tools/` returns no matches outside of ADR-018 itself (which retains its original wording per INV-002 immutability) and CHANGELOG history.

## Directives

[edikt:directives:start]: #
source_hash: pending
directives_hash: pending
compiler_version: "0.6.0-dev"
topic: agent-rules
paths:
  - "templates/agents/evaluator.md"
  - "templates/agents/evaluator-headless.md"
  - "templates/agents/evaluator-verdict.schema.json"
  - "templates/hooks/phase-end-detector.sh"
  - "commands/sdlc/plan.md"
  - "tools/edikt/cmd/upgrade.go"
scope:
  - implementation
  - review
directives:
  - Evaluator agents MUST emit a single JSON object conforming to `templates/agents/evaluator-verdict.schema.json`. NEVER emit prose verdicts. (ref: ADR-025)
  - The plan harness MUST reject PASS unless every criterion whose id references a shell command has `evidence_type: "test_run"`. Mismatch forces the verdict to BLOCKED with a listed reason. (ref: ADR-025)
  - Verdicts are persisted at `<paths.plans>/verdicts/<plan>/<phase>.json`, where `<paths.plans>` is resolved from `.edikt/config.yaml`. NEVER hardcode a literal verdicts path. (ref: ADR-025)
  - Upgrade from < 0.5.0 MUST grandfather existing `done` phases by writing verdict stubs with `meta.grandfathered: true` under the resolved `<paths.plans>/verdicts/` path. Grandfathered verdicts bypass the evidence gate on a one-time basis. (ref: ADR-025)
  - `bin/edikt upgrade` MUST migrate any legacy `docs/product/plans/verdicts/` content to the resolved `<paths.plans>/verdicts/` location for projects whose `paths.plans` is no longer `docs/product/plans`. The migration MUST preserve filenames and contents byte-for-byte. (ref: ADR-025)
manual_directives: []
suppressed_directives: []
canonical_phrases: ["evaluator verdict", "evidence gate", "evidence_type", "grandfathered verdict", "paths.plans", "verdicts directory"]
behavioral_signal:
  cite: ["ADR-025", "ADR-018"]
[edikt:directives:end]: #

---

*Captured by edikt:adr — 2026-05-01*
