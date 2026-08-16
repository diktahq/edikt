# The PRD Format

A PRD in edikt is a split artifact: a prose `.md` you write, and a schema-validated `.yaml` sidecar edikt maintains. That split is what makes requirements addressable — stable FR and AC IDs that a SPEC, a plan, and a drift report all reference by name, and a verify gate that refuses to mark a requirement shipped until an executable check says it is. This page covers the sidecar anatomy, the ID chain, and the lifecycle transitions.

## The split

A PRD is two files that always travel together:

```text
docs/product/prds/
├── PRD-001-renewal-reminders.md      ← prose narrative. you own it.
└── PRD-001-renewal-reminders.yaml    ← structured sidecar. edikt writes it.
```

The `.md` is the human-readable story — problem framing, rationale, forcing questions. The `.yaml` is the source of truth for everything programmatic: requirements (FRs), acceptance criteria (ACs), status, dependencies, supersession chains, sync hashes, and the optional `verify:` shell command per FR and per AC.

Both files share a stem so the pair is discoverable. `/edikt:doctor` flags any orphan — a `.md` with no `.yaml`, or vice versa.

## Stable IDs flow downstream

Every FR and AC gets a stable identifier that survives every transition:

- `FR-001`, `FR-002`, … — functional requirements
- `AC-001-1`, `AC-001-2`, … — acceptance criteria (`AC-{FR-NNN}-{index}`, so `AC-001-2` is the second AC for FR-001)

A SPEC written against the PRD references these IDs verbatim. So does a plan referencing the SPEC. So does a drift report. The ID chain means a finding in CI can trace back to the exact FR that introduced the requirement — and from there to the discovery doc or brainstorm that produced the FR.

Supersession **breaks the chain** — the new PRD gets fresh `FR-001`, `FR-002` numbering. That's why supersession is gated on a four-question test (see [Lifecycle](#lifecycle)).

## Anatomy of the sidecar

The required core, in the order `/edikt:sdlc:prd` writes it:

```yaml
schema_version: "1.0"
type: prd
id: PRD-001
title: Renewal reminders
slug: renewal-reminders
status: draft
rigor: solo
author: alex
created_at: "2026-05-21T09:14:32Z"

requirements:
  - id: FR-001
    text: "Send a renewal reminder 7 days before expiry"
    status: proposed
    verify: "bash test/integration/renewal_reminder_dispatch.sh"

acceptance_criteria:
  - id: AC-001-1
    fr: FR-001
    given: "a user with an active subscription"
    when: "their renewal date is 7 days away"
    then: "they receive an email reminder within 60 seconds"
    status: proposed
    verify: "bash test/integration/renewal_reminder.sh"

protections: []
solution_references: []
revision_history:
  - at: "2026-05-21T09:14:32Z"
    author: alex
    action: created
    note: "Initial draft"

extensions: {}
_sync:
  md_hash: ""
  yaml_hash: ""
  synced_at: ""
```

| Field | Purpose |
|---|---|
| `id` / `slug` / `title` | Stable identifier + filename slug + human title |
| `status` | `draft → accepted → in-progress → shipped` (with `evolving`, `superseded`, `deprecated`, `cancelled` as terminal states) |
| `rigor` | `solo` / `team` / `platform` — gates which sections are required (stakeholders, NFRs, risks) |
| `requirements[]` | FRs with optional `verify:` |
| `acceptance_criteria[]` | Given/When/Then with optional `verify:` |
| `protections[]` | Linked invariants (`{ref: INV-NNN}`) or feature-scoped guards (`{id: SP-NNN, text: ...}`) |
| `revision_history[]` | Append-only audit log of every mutation |
| `extensions` | User-owned key. LLM never writes here; commands never touch. Free-form. |
| `_sync` | SHA-256 of `.md` and `.yaml` at last sync. `/edikt:doctor` flags drift when the `.md` is edited out-of-band. |

The full schema is at [`templates/schemas/prd-sidecar.v1.schema.json`](https://github.com/diktahq/edikt/blob/main/templates/schemas/prd-sidecar.v1.schema.json). VS Code, JetBrains, and Neovim auto-validate via the `# yaml-language-server: $schema=...` header — autocomplete and tooltips work without per-project setup.

## The `verify:` field

Every FR and every AC may carry an optional `verify:` shell command, executed by `bin/edikt verify prd PRD-NNN`. See [Falsifiable Verification](/guides/falsifiable-verification) for the full verify-runner contract — execution model, timeout, exit codes, and `verify_kind`.

The `verify:` field is **optional**. Omit when no mechanical check is possible — the field stays absent (never write `verify: ""`; the schema rejects empty strings). The doctor's "Sidecar Verify Coverage" line surfaces gaps as soft warnings.

What `verify:` enables:

- **`/edikt:sdlc:prd PRD-NNN ship`** runs `verify prd PRD-NNN` first. Any failure refuses the ship — the sidecar is not mutated.
- **`/edikt:sdlc:prd PRD-NNN supersede`** gates similarly. Override via `--force-verify` (recorded in `revision_history`).
- **`/edikt:doctor`** reports coverage and per-PRD verify health as soft signals.

## Lifecycle

```text
   ┌──────────┐    accept    ┌──────────┐    work    ┌─────────────┐
   │  draft   │ ───────────> │ accepted │ ─────────> │ in-progress │
   └──────────┘              └──────────┘            └─────────────┘
                                                            │
                                                            │ ship FR-NNN ...
                                                            ▼
                                                       ┌─────────┐
                                                       │ shipped │
                                                       └─────────┘
                                                            │
                                                            │ optional post-ship iteration
                                                            ▼
                                                       ┌──────────┐
                                                       │ evolving │
                                                       └──────────┘

   Terminal states (from any non-terminal status):
   ┌────────────┐  ┌────────────┐  ┌───────────┐
   │ superseded │  │ deprecated │  │ cancelled │
   └────────────┘  └────────────┘  └───────────┘
```

Transitions are dispatched via the same command:

```bash
/edikt:sdlc:prd PRD-001 ship FR-001 FR-002
/edikt:sdlc:prd PRD-001 cancel "merged into PRD-007"
/edikt:sdlc:prd PRD-001 deprecate "feature retired"
/edikt:sdlc:prd PRD-001 supersede
```

- `ship` flips FRs to `status: shipped`. When all FRs ship, the top-level status flips to `shipped`. **Gated on `verify prd`.**
- `cancel` marks the PRD `cancelled`. Use when work stopped before shipping.
- `deprecate` marks the PRD `deprecated`. Use when a shipped PRD is now obsolete. **Not gated** — you may need to deprecate precisely because something failed.
- `supersede` creates a new PRD-MMM and breaks the stable-ID chain. Gated on the four-question scope test **and** on `verify prd` (override via `--force-verify`).

## Why the split

Three concrete benefits over the single-file v1 format:

1. **Programmatic queries become trivial.** "Which PRDs have shipped this quarter?" used to require regex over markdown; now it's a `jq` over every `.yaml` sidecar.
2. **The narrative stays human.** No frontmatter creep, no metadata embedded mid-prose. The `.md` reads like an essay; the `.yaml` carries everything a tool would want.
3. **The verify gate becomes mechanical.** A shell command per AC means `ship` can refuse to mutate state when the work hasn't actually landed — the discipline edikt enforces is now self-attesting.

## Migration from v1

v1 PRDs (a single `.md` with embedded frontmatter) are still readable. `/edikt:doctor` flags them as v1 silently; `/edikt:sdlc:prd PRD-NNN` (continuation mode) suggests migration on next edit. The migration is non-destructive — the original `.md` is preserved, and the sidecar is generated from its frontmatter + body.

## Related

- [`/edikt:sdlc:prd`](/commands/sdlc/prd) — author, continue, and lifecycle transitions
- [`/edikt:sdlc:spec`](/commands/sdlc/spec) — SPECs reference PRD FRs and ACs by stable ID
- [Sidecar Architecture](/governance/sidecar) — the broader sidecar model (gov / prd / spec)
- [Falsifiable Verification](/guides/falsifiable-verification) — the verify-runner contract
- [`edikt verify`](/commands/verify) — the runner that executes `verify:` commands
- [Drift detection](/commands/sdlc/drift) — closes the loop: did the implementation match the PRD?
