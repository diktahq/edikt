# PRD v2 Deep Dive

The flagship change in v0.6.0 is how PRDs are written. This guide explains why the format changed, what the five forcing questions are doing, how rigor calibration works, how the FR/AC chain flows through to specs and tests, and what the sidecar buys you in your editor.

If you just want the command reference, see [/edikt:sdlc:prd](/commands/sdlc/prd). This is the longer read.

## Why split markdown + YAML

Prior PRDs were a single markdown file. That worked until Claude started editing them.

LLMs corrupt structured prose under multi-turn editing at roughly 5–10% per session. Tables drift. Section ordering reflows. ID schemes silently renumber. By turn 30 of a long planning session, the PRD that started as the source of truth no longer matches what the spec, plan, or evaluator are reading.

The fix is structural: separate what humans read from what commands mutate.

```text
docs/product/prds/
├── PRD-005-webhook-delivery.md     ← narrative, prose, diagrams
└── PRD-005-webhook-delivery.yaml   ← FRs, ACs, status, hashes
```

The `.md` is what you read. It renders well in any markdown viewer, looks fine on GitHub, and reads in PR review. The `.yaml` is what `/edikt:sdlc:prd PRD-005 ship FR-001` mutates — structured, schema-validated, byte-stable across edits.

Both files share a slug. The sidecar's `_sync.md_hash` records the SHA-256 of the `.md` at last sync, so drift between them is detectable. `/edikt:doctor` and `/edikt:prd:review` flag drift; re-running `/edikt:sdlc:prd PRD-005` resyncs.

## The five forcing questions

Every new PRD opens with five questions, asked one at a time. Skipping isn't an option — if you try, the command says "this question is not optional; a short answer or 'hypothesis only' is fine."

The questions are not skippable because the rubric scores them. A PRD that passes evaluation without answering Q3 (north + counter metric) would be a rubric bug, not a valid PRD.

### Q1 — What's the problem behind the problem?

> Don't describe the solution. Describe what breaks without this feature.

The trap this catches: starting from "we should build a webhook system" without naming what fails when there isn't one. The answer that earns the rubric point names a concrete failure mode — a thing that goes wrong, that someone notices, that has a cost.

### Q2 — How do you know someone has this problem?

> Evidence, data, support tickets, interviews — or "hypothesis only" if you're starting from an informed guess.

Evidence beats hypothesis, but hypothesis is honest. Both score the rubric point. What fails is silent absence — claiming users want X without surfacing where that came from.

### Q3 — What single metric should move, and what metric must NOT?

The north metric is what success looks like. The counter metric is the guardrail — the thing you'd be okay shipping if it didn't move, but you'd cancel the project if it did.

Counter metrics force the team to admit what success could break. "More signups" with no counter metric is how you accidentally optimize signups by degrading retention.

### Q4 — What must NOT change when this ships?

This seeds the Protections section. Existing invariants, UX patterns users depend on, contracts with external systems. The command auto-links matching invariants from `docs/architecture/invariants/` after you answer.

Q4 also catches new invariant candidates. If you wrote "admin APIs must always require 2FA" as a protection, the command suggests `/edikt:invariant:new` — promote the protection from PRD-scope to project-wide.

### Q5 — What's the riskiest assumption?

Naming the assumption that, if false, makes the whole thing pointless. Usually it's a behavioral assumption ("users will adopt this") or a technical one ("CRDT memory cost stays under N").

Riskiest assumptions are what the discovery doc was for, if you ran one — graduate `DISCOVERY-NNN` into a PRD and Q5 is pre-populated from your Unknown section.

## Rigor calibration

Before the forcing questions, the command asks one triage question:

```text
solo      — you or a small group, single-product scope (default)
team      — cross-functional, multiple stakeholders, sign-off, rollout plan
platform  — multi-tenant, compliance-sensitive, cross-product
```

Rigor sets the evaluator threshold and which optional sections appear:

| Rigor | Threshold | Adds to the PRD |
|-------|-----------|-----------------|
| `solo` | 7/10 | Base sections only |
| `team` | 8/10 | Stakeholders, dependencies, rollout plan |
| `platform` | 9/10 | NFRs with measurable targets, risk register, compatibility matrix |

The default is `solo`. It exists because rigor without scope produces theater — a single-engineer side project doesn't need a stakeholder matrix. Triage stops the rubric from punishing legitimate scope.

For `team` and `platform`, the additional sections are required by the rubric. A team-rigor PRD without stakeholders fails the threshold. A platform-rigor PRD without measurable NFRs fails. The optional-but-required pattern is intentional: when the scope demands the section, the section is mandatory.

## The FR/AC stable-ID chain

Functional requirements are numbered `FR-NNN` starting at `FR-001`. Acceptance criteria use `AC-NNN-M` where `NNN` matches the FR they belong to. So `FR-001` with two ACs has `AC-001-1` and `AC-001-2` — not `AC-001` and `AC-002`.

The format isn't aesthetic. It's how the IDs flow through the rest of the SDLC:

```text
PRD: FR-001 — Webhook delivery retries on 5xx
     AC-001-1 — Given a 503, when retried, then succeeds within 3 attempts
     AC-001-2 — Given a 503, when retried, then backoff is exponential
        ↓
SPEC: SR-001 implements FR-001
      AC-001-1, AC-001-2 — pass through unchanged
      SAC-001 — added: retry queue uses durable storage
        ↓
Plan: Phase 4 — implement SR-001
      Acceptance: AC-001-1 (run integration test X)
                  AC-001-2 (assert backoff factor in Y)
                  SAC-001  (verify queue durability)
        ↓
Drift: maps test failure on AC-001-2 back to PRD-005 FR-001
```

ACs use Given/When/Then format with an explicit `verify:` field — the method that checks the criterion is met:

```yaml
- id: AC-001-1
  fr: FR-001
  given: "a webhook endpoint returning 503"
  when: "delivery is retried"
  then: "retry succeeds within 3 attempts"
  verify: "send test event with mock 503 endpoint; assert retry_count == 2"
  status: proposed
```

The `verify` field is what `/edikt:sdlc:plan` uses to populate the criteria sidecar. Without it, the evaluator can't pass-or-fail the criterion at phase end.

## Sidecar `_sync` drift detection

Every write recomputes `_sync.md_hash` and `_sync.yaml_hash` over the canonical content:

```yaml
_sync:
  md_hash: "a3f5..."
  yaml_hash: "b8c2..."
  synced_at: "2026-04-15T09:30:00Z"
```

When the `.md` is edited outside the command — typo fix, a manual cleanup, a teammate's PR — the hash drifts. `/edikt:prd:review` and `/edikt:doctor` flag the drift:

```text
⚠ Sidecar drift:
  .md edited since last sync (2026-04-15T09:30:00Z).
  To update hashes: re-author with /edikt:sdlc:prd PRD-005
```

Drift is informational, not an error. The PRD is still valid; the sync record is stale. Re-running `/edikt:sdlc:prd PRD-005` picks up the manual edits and rewrites the hashes.

## JSON Schema autocomplete

The sidecar template carries this header:

```yaml
# yaml-language-server: $schema=../../../.edikt/schemas/prd-sidecar.v1.schema.json
```

That comment is read by the `yaml-language-server` extension. Open the `.yaml` in any editor with the extension installed and you get autocomplete, validation, and tooltips:

| Editor | Extension |
|--------|-----------|
| VS Code | "YAML" by Red Hat |
| JetBrains (IntelliJ, GoLand, etc.) | Built-in YAML schema support |
| Neovim | `yaml-language-server` via mason or LSP |
| Vim | coc-yaml |

The schema is auto-installed to `.edikt/schemas/prd-sidecar.v1.schema.json` the first time you author a PRD in the project. No per-project setup.

## Example: a real PRD pair

`PRD-005-webhook-delivery.md`:

```markdown
---
id: PRD-005
title: Webhook delivery with retry
status: accepted
rigor: team
---

# PRD-005: Webhook delivery with retry

## Problem

Customers integrating with our API rely on webhooks to react to events. When
our delivery fails (their server returns 5xx, network drops), the event is
lost. Last quarter we logged 1,200 lost events across 8 customer accounts.

## Users affected

- Integration partners (12 active, growing)
- Internal teams using webhooks for event-driven flows (Orders, Billing)

## North metric
Webhook delivery success rate — target: 99.5%

## Counter metric
P99 delivery latency — must not exceed 5s

## Requirements

- **FR-001** — Webhook delivery retries on 5xx with exponential backoff
- **FR-002** — Retried deliveries are idempotent on the receiver side
- **FR-003** — Failed deliveries after max retries land in a dead-letter queue
```

`PRD-005-webhook-delivery.yaml`:

```yaml
# yaml-language-server: $schema=../../../.edikt/schemas/prd-sidecar.v1.schema.json
schema_version: "1.0"
type: prd
id: PRD-005
title: Webhook delivery with retry
slug: webhook-delivery
status: accepted
rigor: team
author: alex
created_at: 2026-04-15T09:00:00Z

requirements:
  - id: FR-001
    text: "Webhook delivery retries on 5xx with exponential backoff"
    status: proposed
  - id: FR-002
    text: "Retried deliveries are idempotent on the receiver side"
    status: proposed
  - id: FR-003
    text: "Failed deliveries after max retries land in a dead-letter queue"
    status: proposed

acceptance_criteria:
  - id: AC-001-1
    fr: FR-001
    given: "a webhook endpoint returning 503"
    when: "delivery is retried"
    then: "succeeds within 3 attempts"
    verify: "integration test: mock 503 endpoint; assert retry_count <= 3"
    status: proposed

protections:
  - ref: INV-006
    note: "delivery payload validation must remain strict"

forcing_questions:
  problem_behind_problem: "1,200 lost events / quarter across 8 accounts; integrations silently break"
  evidence_or_hypothesis: "8 support tickets in last 90 days; lost events visible in audit log"
  north_metric_and_counter: "99.5% delivery success / p99 latency <= 5s"
  must_not_change: "delivery payload schema; existing webhook auth"
  riskiest_assumption: "receivers will implement idempotency keys correctly"

stakeholders:
  - name: Alex
    role: Engineering lead
  - name: Jordan
    role: Customer Success

dependencies: []
nfrs: []
risks: []
open_questions: []
source_specs: []
revision_history:
  - at: 2026-04-15T09:00:00Z
    author: alex
    action: created
    note: "Initial draft — rigor: team"

_sync:
  md_hash: "a3f5..."
  yaml_hash: "b8c2..."
  synced_at: 2026-04-15T09:00:00Z
```

The `.md` is what gets shared in Slack and reviewed in PRs. The `.yaml` is what `/edikt:sdlc:spec PRD-005`, `/edikt:prd:review PRD-005`, and `/edikt:sdlc:prd PRD-005 ship FR-001` actually read and mutate.

## Lifecycle verbs

PRDs evolve in place (per ADR-024). Lifecycle transitions are dispatched by the same command:

```bash
/edikt:sdlc:prd PRD-005 ship FR-001 FR-002         # mark FRs shipped
/edikt:sdlc:prd PRD-005 cancel "priority shifted"  # never shipped
/edikt:sdlc:prd PRD-005 deprecate "merged into PRD-009"  # was shipped, now obsolete
/edikt:sdlc:prd PRD-005 supersede                  # ≥50% rewrite
```

The verbs are not symmetric:

- `ship` is the happy path. Mark FRs shipped as they land. When all ship, the PRD's top-level status flips to `shipped` automatically.
- `cancel` means the work stopped before any FR shipped. Hypothesis was wrong, priority shifted, scope merged into something else.
- `deprecate` means the PRD was shipped or accepted, and now it's obsolete. The file stays as historical record.
- `supersede` is rare. Reserved for ≥50% scope rewrites that break the stable-ID chain. The command gates it behind four yes/no questions and refuses to proceed unless three or four are yes.

For routine FR additions or revisions, prefer continuation: `/edikt:sdlc:prd PRD-005` (no verb). The command loads the existing PRD, asks what you're doing, and applies the relevant subset of the authoring flow.

## What this earns you

A PRD that survives multi-turn editing without drift. A rubric that scores forcing-question answers, not vibes. A sidecar that schema-validates, that downstream commands trust, that an editor autocompletes. A stable-ID chain that traces test failures back to specific FRs.

The PRD becomes the engineering blueprint, not the marketing pitch. Spec, plan, drift detection — they all read from the same structured source.

## What's next

- [/edikt:sdlc:prd](/commands/sdlc/prd) — command reference
- [/edikt:prd:review](/commands/prd/review) — re-score a PRD
- [/edikt:sdlc:spec](/commands/sdlc/spec) — write the technical spec next
- [Configuring Evaluator Gates](./evaluator-gates.md) — what runs at phase end
