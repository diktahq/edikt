# /edikt:sdlc:prd

Write, continue, or transition a Product Requirements Document.

A PRD captures what to build and what done means. As of v0.6.0, every PRD is a split artifact: a `.md` narrative for humans plus a `.yaml` sidecar that's the structured source of truth for requirements, acceptance criteria, status, and revision history.

## Usage

```bash
/edikt:sdlc:prd webhook delivery with retry logic
/edikt:sdlc:prd                                  # asks what to build
/edikt:sdlc:prd PRD-005                          # continue / revise
/edikt:sdlc:prd PRD-005 ship FR-001 FR-002       # mark requirements shipped
/edikt:sdlc:prd PRD-005 cancel "priority shifted"
/edikt:sdlc:prd PRD-005 deprecate "merged into PRD-009"
/edikt:sdlc:prd PRD-005 supersede                # ≥50% rewrite
/edikt:sdlc:prd DISCOVERY-001                    # graduate a discovery
```

## Why a split artifact

LLMs corrupt structured prose at roughly 5–10% per multi-turn edit. Tables drift, sections get reordered, IDs reflow. YAML stays intact.

The narrative goes in `PRD-NNN-<slug>.md` where it reads well. The structure — FRs, ACs, protections, sync hashes, revision history — goes in `PRD-NNN-<slug>.yaml` where commands can mutate it safely.

```text
docs/product/prds/
├── PRD-005-webhook-delivery.md     ← narrative, what humans read
└── PRD-005-webhook-delivery.yaml   ← sidecar, source of truth
```

The sidecar carries a `_sync.md_hash` so drift is detectable. v1 PRDs (no sidecar) continue to work — only re-authoring with `/edikt:sdlc:prd PRD-NNN` regenerates them in v2 shape.

## The five forcing questions

Every new PRD opens with five questions. Asked one at a time. Not skippable.

1. **What's the problem behind this problem?** Don't describe the solution. Describe what breaks without this feature.
2. **How do you know someone has this problem?** Evidence, data, support tickets, interviews — or "hypothesis only" if you're starting from an informed guess.
3. **What single metric should move if this works, and what metric must NOT move?** North metric and counter metric.
4. **What must NOT change when this ships?** Existing invariants, UX patterns, contracts — seeds the Protections section.
5. **What's the riskiest assumption behind this working?**

The answers are recorded in the sidecar's `forcing_questions:` field and scored by the rubric. A PRD that passes evaluation without answering Q3 (north + counter metric) would be a rubric bug, not a valid PRD.

## Rigor calibration

Before the forcing questions, the command asks one triage question:

```text
Before we start — is this a solo project, a team feature, or a platform change?

  solo      — you or a small group, single-product scope (default)
  team      — cross-functional, multiple stakeholders, sign-off, rollout plan
  platform  — multi-tenant, compliance-sensitive, cross-product
```

Rigor sets the evaluator threshold and which optional sections appear:

| Rigor | Threshold | Adds |
|-------|-----------|------|
| `solo` | 7/10 | Base sections only |
| `team` | 8/10 | Stakeholders, dependencies, rollout plan |
| `platform` | 9/10 | NFRs, risk register, compatibility matrix |

Default is `solo`. Press enter to accept.

## Stable IDs

FRs are numbered `FR-NNN` starting at `FR-001`. ACs are numbered `AC-NNN-M` where `NNN` is the FR they belong to and `M` is the criterion index within that FR. So `FR-001` with two ACs has `AC-001-1` and `AC-001-2`.

These IDs flow downstream:

```text
PRD: FR-001 + AC-001-1, AC-001-2
       ↓
SPEC: SR-001 implements FR-001, ACs pass through unchanged + SAC-NNN added
       ↓
Plan: phases reference SR-NNN and AC-NNN-M
       ↓
Drift: maps test failures back to specific FRs
```

Renumbering breaks the chain. Supersession (≥50% rewrite) is the only intentional way to break it — see [Lifecycle](#lifecycle).

## Auto-linked invariants

After drafting requirements, the command grep's `docs/architecture/invariants/` for keyword overlap with the PRD scope and presents candidates:

```text
I found 2 existing invariants that may apply:

  [1] INV-003 — Hook JSON emission
      Relevant because: FR-002 mentions hook behavior
  [2] INV-006 — Input shape validation
      Relevant because: FR-001 accepts user input

For each, link as a protection? (1=yes, 0=no, e=edit note)
```

Confirmed links land in the sidecar's `protections:` as `{ref: INV-NNN, note: "..."}`. The Q4 answer ("what must NOT change") is also scanned for new invariant candidates — if a protection looks like a durable architectural rule, the command suggests `/edikt:invariant:new`.

Feature-scoped protections that aren't durable enough to be an invariant get `SP-NNN` IDs and stay scoped to this PRD.

## Lifecycle

PRDs evolve in place. Lifecycle transitions are dispatched by the same command.

| Verb | Use when | Effect |
|------|----------|--------|
| `ship FR-NNN ...` | Requirements have shipped | Marks FRs `shipped`. If all FRs ship, top-level status flips to `shipped`. |
| `cancel <reason>` | Work stopped before shipping | Marks PRD `cancelled`. Reason recorded. Hidden from active views. |
| `deprecate <reason>` | Was shipped/accepted, now obsolete | Marks PRD `deprecated`. File kept as historical record. |
| `supersede` | ≥50% scope rewrite | Creates a new PRD-MMM, links both directions. **Breaks the stable-ID chain.** |

Supersession is gated. The command asks four yes/no questions and refuses to proceed unless three or four are yes:

> Has the problem framing changed?
> Would ≥50% of FRs be rewritten?
> Are protections so different that old ACs don't apply?
> Have you tried continuation and found it insufficient?

For routine changes, prefer continuation (`/edikt:sdlc:prd PRD-005`), shipping FRs (`ship FR-NNN`), or deprecation. ADR-024 explains why supersession is rare.

## JSON Schema autocomplete

The sidecar template carries a `# yaml-language-server: $schema=...` header pointing at `templates/schemas/prd-sidecar.v1.schema.json`. Open the `.yaml` in VS Code, JetBrains, or Neovim with the YAML extension installed — autocomplete, validation, and tooltips work without any per-project setup.

The schema is auto-installed to `.edikt/schemas/` the first time you author a PRD.

## Drift detection

Every write recomputes `_sync.md_hash` and `_sync.yaml_hash` over the canonical content. When the `.md` is edited outside the command (typo fix, manual edit), the hash drifts. `/edikt:doctor` and `/edikt:prd:review` flag the drift so it doesn't go unnoticed.

To resync, re-run `/edikt:sdlc:prd PRD-NNN` — the command picks up the manual edits and rewrites the hashes.

## v1 PRDs

If a project has older PRDs without sidecars, they continue to work. SPEC and review commands detect sidecar absence and branch to a reduced flow. There is no forced migration. Re-authoring with `/edikt:sdlc:prd PRD-NNN` regenerates the PRD in v2 shape.

## What's next

- [/edikt:prd:review](/commands/prd/review) — re-score the PRD against the rubric
- [/edikt:sdlc:spec](/commands/sdlc/spec) — write the technical spec
- [/edikt:sdlc:discovery](/commands/sdlc/discovery) — upstream uncertainty reduction
- [PRD v2 Deep Dive](/guides/prd-v2) — full walkthrough with example sidecar
