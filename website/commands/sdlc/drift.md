# /edikt:sdlc:drift

Verify implementation against the governance chain — spec, PRD, artifact contracts, ADRs, and invariants.

Drift detection is the verification step that closes the governance chain. It answers: does what got built match what was decided?

## Usage

```bash
/edikt:sdlc:drift SPEC-005
/edikt:sdlc:drift SPEC-005 --scope=spec
/edikt:sdlc:drift SPEC-005 --scope=adrs
/edikt:sdlc:drift SPEC-005 --output=json
```

## Arguments

| Argument | Description |
|----------|-------------|
| `SPEC-005` | SPEC identifier to check against |
| `--scope=prd` | Check PRD acceptance criteria only |
| `--scope=spec` | Check spec requirements only |
| `--scope=artifacts` | Check artifact contracts only |
| `--scope=adrs` | Check ADR compliance only |
| `--output=json` | Machine-readable output for CI |

Default scope is the full chain.

## What it checks

**Layer 1 — PRD acceptance criteria.** For each criterion in the source PRD, determines whether the implementation satisfies it.

**Layer 2 — Spec requirements.** For each component or requirement in the spec, determines whether it was implemented as specified.

**Layer 3 — Artifact contracts.** For each artifact in the spec folder:
- `data-model.*` (`.mmd`, `.schema.yaml`, or `.md`) → actual schema matches? (dba)
- `contracts/api.yaml` → actual endpoints match? (api)
- `test-strategy.md` → tests exist as specified? (qa)
- `contracts/events.yaml` → event schema matches? (architect)
- `fixtures.yaml` → test data covers scenarios? (qa)

**Layer 4 — ADR compliance.** For each ADR referenced in the spec frontmatter, checks whether the implementation follows the decision.

**Layer 5 — Invariant compliance.** For each active invariant, checks whether it's violated by any changed file.

## Severity model

| Level | Meaning |
|-------|---------|
| ✅ Compliant | Verified by reading code |
| 🟡 Likely compliant | Appears to match; can't verify deterministically |
| ⚠️ Diverged | Implementation clearly differs from the decision |
| ❓ Unknown | Not enough signal |

## Report format

Emoji summary at the top, then a single table showing ALL findings — diverged, likely, unknown, and compliant. Nothing hidden.

```text
DRIFT REPORT — SPEC-005
─────────────────────────────────────────────────
Source: SPEC-005 + 3 artifacts + 2 ADRs + 1 invariant
Scope:  full chain
Date:   2026-03-20

SUMMARY
  ✅ 14 compliant    🟡 1 likely compliant
  ⚠️  2 diverged      ❓ 0 unknown

┌───┬────────┬──────────────────────────────────────────────────────────────────┐
│ # │ Status │ Description                                                      │
├───┼────────┼──────────────────────────────────────────────────────────────────┤
│ 1 │  ⚠️    │ PRD-005: "Failed deliveries visible in admin dashboard" —        │
│   │        │ no admin endpoint for webhook status exists                      │
├───┼────────┼──────────────────────────────────────────────────────────────────┤
│ 2 │  ⚠️    │ contracts/api.yaml: POST /webhooks/retry response shape —        │
│   │        │ contract says { "queued", "delivery_ids" }, actual returns 204   │
├───┼────────┼──────────────────────────────────────────────────────────────────┤
│ 3 │  🟡    │ SPEC-005: "Handle webhook delivery failures gracefully" —        │
│   │        │ error handling exists but full retry coverage unverified         │
└───┴────────┴──────────────────────────────────────────────────────────────────┘
─────────────────────────────────────────────────

Report saved: docs/reports/drift-SPEC-005-2026-03-20.md

2 diverged finding(s). Want me to prioritize them?
```

## Persisted reports

The report is saved to `docs/reports/` automatically:

```text
docs/reports/drift-SPEC-005-2026-03-20.md
```

The filename format is `drift-{SPEC-NNN}-{YYYY-MM-DD}.md`. The report carries frontmatter with spec, scope, date, and summary counts — making it queryable by tooling.

## CI usage

```bash
/edikt:sdlc:drift SPEC-005 --output=json
echo $?   # 0 = all compliant, 1 = diverged findings exist
```

JSON output contains all findings in structured format. Wire the exit code into CI to catch regressions.

## Integration with `/edikt:sdlc:review`

When `/edikt:sdlc:review` runs and an active spec exists, it automatically appends a scoped drift check (`--scope=spec`) to the review output. You don't need to run both separately during a review cycle.

## Status filtering

Before validating, drift filters artifacts by status:

| Status | Action |
|--------|--------|
| `accepted` | Validate |
| `implemented` | Validate (verify still correct) |
| `in-progress` | Validate (check partial work) |
| `draft` | Skip — `⏭ Skipping data-model.mmd (status: draft)` |
| `superseded` | Skip — `⏭ Skipping api-v1.yaml (status: superseded)` |

Draft artifacts haven't been reviewed — validating against them produces unreliable results. Accept them first.

## Auto-promote

When drift finds zero violations for an artifact with status `in-progress`, it automatically promotes the status to `implemented`:

```text
✅ contracts/api.yaml — no drift detected
   Status promoted: in-progress → implemented
```

Only `in-progress → implemented` is auto-promoted. Artifacts at `accepted` must go through `in-progress` first (triggered by the plan command when a phase starts).

## What's next

- Fix diverged findings and run `/edikt:sdlc:drift` again
- [/edikt:sdlc:review](/commands/sdlc/review) — specialist review for the current implementation
- [/edikt:sdlc:audit](/commands/sdlc/audit) — OWASP and security audit
- [Drift Detection](/governance/drift) — full explanation of drift detection
