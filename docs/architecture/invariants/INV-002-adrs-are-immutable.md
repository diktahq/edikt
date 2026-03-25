# INV-002 — ADRs are immutable records

**Status:** Active

## Rule

Architecture Decision Records are immutable once accepted. While in `draft` status, an ADR may be freely edited. Once the status is changed to `accepted`, its content — context, decision, and consequences — must never be edited again. The only permitted mutation after acceptance is updating the `Status:` field to `Superseded by ADR-NNN` or `Deprecated`.

When a decision changes, a new ADR is written that supersedes the old one. The old ADR is updated to `Status: Superseded by ADR-NNN` and left otherwise intact.

## Why

ADRs are a historical record. Their value comes from being able to read the original reasoning at the moment the decision was made — not a retroactively corrected version. Mutating an ADR rewrites history and breaks the traceability chain: you lose the ability to understand why the old decision was made and when it changed.

This is the original intent of the ADR format as described by Michael Nygard (2011): "If a decision is reversed, we will keep the old one around, but mark it as superseded."

## Enforcement

- ADRs in `draft` status may be freely edited — they are not yet accepted
- When asked to update, fix, or improve an accepted ADR: stop and create a new ADR instead
- The new ADR must reference the superseded one (`Supersedes: ADR-NNN`)
- The old ADR status line must be updated to `Superseded by ADR-NNN`
- No other changes to the accepted ADR are permitted

## Exceptions

- Fixing a typo or broken markdown formatting in an accepted ADR is permitted if it does not change the meaning
- Adding `Superseded by ADR-NNN` to the status field is the one required mutation after acceptance
