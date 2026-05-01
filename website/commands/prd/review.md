# /edikt:prd:review

Re-score an existing PRD against the rubric and report rubric gaps, sidecar drift, broken refs, and unstarted requirements.

Use this when a PRD has been sitting open, when its `.md` has been hand-edited, or before a planning session — to catch issues the original draft passed but the current state doesn't.

## Usage

```bash
/edikt:prd:review PRD-005
```

The PRD identifier is required. The command reads `PRD-005-<slug>.md` and `PRD-005-<slug>.yaml`, runs four independent checks, and writes a single review record into the sidecar's `revision_history`.

## What it checks

| Check | What it catches |
|-------|----------------|
| **Rubric score** | The same rubric `/edikt:sdlc:prd` uses at authoring time. Re-applied against the current state. |
| **Sidecar drift** | The `.md` was edited after the last sync. The sidecar `_sync.md_hash` no longer matches the file on disk. |
| **Broken refs** | Linked invariants (`INV-NNN`), specs (`SPEC-NNN`), or supersede chains that point to files that no longer exist. |
| **Unstarted FRs** | Functional requirements still `proposed` with no SPEC covering them. Surfaces silent backlog. |

## Output

```text
PRD REVIEW — PRD-005

  Title:          Webhook delivery with retry
  Status:         accepted
  Rigor:          team
  Version:        v2

  Rubric score:   8/10 (team threshold: 8/10)
                  ✓ PASS

  ⚠ Sidecar drift:
    .md edited since last sync (2026-04-12T14:22:00Z).
    To update hashes: re-author with /edikt:sdlc:prd PRD-005

  ⚠ Broken references (1):
    • INV-042 — not found in docs/architecture/invariants/

  ⚠ Unstarted FRs (2):
    • FR-003 — proposed, no SPEC covers it yet
    • FR-005 — proposed, no SPEC covers it yet

Next:
  • Revise PRD:  /edikt:sdlc:prd PRD-005
  • Ship FRs:    /edikt:sdlc:prd PRD-005 ship FR-NNN
  • Write spec:  /edikt:sdlc:spec PRD-005
```

If everything is clean, the report ends with `✓ All checks pass.`

## Rigor thresholds

The score is calibrated to the PRD's `rigor:` field, set at authoring time:

| Rigor | Threshold |
|-------|-----------|
| `solo` | 7/10 |
| `team` | 8/10 |
| `platform` | 9/10 |

Higher rigor sets a tighter bar — platform-grade PRDs need NFRs, a risk register, and a compatibility matrix on top of the base rubric.

## v1 PRDs

PRDs without a `.yaml` sidecar (v1 shape) get a reduced review — rubric score and broken-ref checks only. Sidecar drift and FR coverage need the structured sidecar, which v1 doesn't have.

To upgrade a v1 PRD to v2, re-author it with `/edikt:sdlc:prd PRD-NNN` — the command detects the missing sidecar and offers to regenerate.

## What it does not do

- It does not re-run the forcing questions. Those are scored once at authoring; review re-scores the current state, not the conversation.
- It does not edit FRs or ACs. The only mutation is appending a review record to `revision_history` — the audit trail of who reviewed when and what they found.
- It does not auto-fix broken refs. You decide whether to remove the reference, repair the linked artifact, or supersede.

## Why review is read-heavy

The rubric is the contract. If the current PRD passes review, the SPEC and plan can trust it. If it fails, you know exactly which line to fix before downstream work compounds the gap.

Run review before writing a SPEC. Run it again before planning. Run it any time the PRD has been edited outside the `/edikt:sdlc:prd` flow.

## What's next

- [/edikt:sdlc:prd](/commands/sdlc/prd) — revise the PRD or ship requirements
- [/edikt:sdlc:spec](/commands/sdlc/spec) — write the technical spec once review passes
- [PRD v2 Deep Dive](/guides/prd-v2) — what the rubric actually scores
