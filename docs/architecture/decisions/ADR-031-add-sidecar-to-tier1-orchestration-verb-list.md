---
type: adr
id: ADR-031
title: Add `sidecar` to the ADR-029 tier-1 orchestration verb list
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-05-03T00:00:00Z
references:
  adrs: [ADR-029, ADR-027]
  invariants: [INV-002, INV-006]
  prds: []
  specs: []
amends: ADR-029
---

# ADR-031 — Add `sidecar` to the ADR-029 tier-1 orchestration verb list

## Status

**Accepted**

## Context

ADR-029 permits tier-1 markdown commands to invoke `bin/edikt <subcommand>` for orchestration purposes, under four rules. Rule 3 enumerates the permitted verbs exhaustively: `verify`, `doctor`, `migrate`, `gov compile`, `install`, `upgrade`, `use`, `rollback`. Adding a new verb requires an ADR amending ADR-029.

Phase 7 of PLAN-v060-governance-accuracy ships two artifacts:

1. `bin/edikt sidecar add-manual-directive` — a tier-2 Go subcommand that appends an entry to `manual_directives[]` in an existing `<artifact>.edikt.yaml` sidecar, without touching the parent `.md` (INV-002).
2. `/edikt:adr:enrich` (tier-1 markdown) — the interactive face of the above. Its Step 8 invokes `bin/edikt sidecar add-manual-directive` and reads exit code only (ADR-029 Rule 2).

`sidecar` is not in ADR-029's current verb list. The `/edikt:adr:enrich` call at Step 8 would be an undocumented Rule 3 violation without this amendment.

## Decision

**Amend ADR-029 Rule 3 to add `bin/edikt sidecar <subcommand>` as a permitted orchestration verb group.**

The amended verb list:

- `bin/edikt verify <plan-id> [--phase N]`
- `bin/edikt doctor`
- `bin/edikt migrate <subcommand>`
- `bin/edikt gov compile`
- `bin/edikt install <helper>`
- `bin/edikt upgrade`
- `bin/edikt use`
- `bin/edikt rollback`
- `bin/edikt sidecar <subcommand>` ← **added by this ADR**

The addition is intentionally broad at the `sidecar` group level (not pinned to `add-manual-directive` only), because future sidecar subcommands (`diff`, planned for Phase 6) will also be invoked from tier-1 markdown. Pinning to a specific subcommand would require another amendment for each addition; permitting the `sidecar` group once is cleaner and lower maintenance overhead.

All four ADR-029 rules still hold for every `sidecar` subcommand invocation:

1. **Absence detection** — `/edikt:adr:enrich` Step 1 explicitly checks `command -v bin/edikt` before proceeding.
2. **Exit code only** — Step 8 passes output verbatim and never parses its shape.
3. **Enumerated verb** — `sidecar` is now in the list.
4. **Failure mode documented** — `on_absent: refuse-and-direct-user` is declared in `/edikt:adr:enrich`'s frontmatter.

## Consequences

- **Good — Phase 7 ships contract-clean.** `/edikt:adr:enrich`'s `bin/edikt sidecar add-manual-directive` call is now an authorized orchestration call.
- **Good — Phase 6 (`sidecar diff`) needs no further amendment.** The `sidecar` group permit covers the planned diff subcommand without another ADR.
- **Neutral — ADR-029 frontmatter updated.** ADR-029's `amended_by` field gains `ADR-031`.

## ADR-029 Amendment

ADR-029 frontmatter gains `amended_by: [ADR-031]`. Per INV-002, no other content of ADR-029 is modified.

## Confirmation

- `bin/edikt sidecar add-manual-directive --help` exits 0.
- `/edikt:adr:enrich` frontmatter declares `tier_2_dependency: edikt` and `on_absent: refuse-and-direct-user`.
- `go vet ./cmd/...` clean.
- All `TestAddManual_*` tests pass.
