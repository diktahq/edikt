# edikt adr enrich

**Interactively add a manual directive to an ADR or Invariant sidecar without editing the parent `.md`.** The command resolves the sidecar, shows its current manual directives, prompts for new text, validates the modal verb, auto-suggests a `(ref: ...)` tag, and delegates the write to `bin/edikt sidecar add-manual-directive`.

## Synopsis

```bash
/edikt:adr:enrich                  # prompts for the ADR/INV ID
/edikt:adr:enrich ADR-NNN
/edikt:adr:enrich INV-NNN
/edikt:adr:enrich <path-to-.md-or-.edikt.yaml>
```

## How it works

1. **Verify binary presence.** Requires the `edikt` tier-2 helper. If `bin/edikt` is absent, the command refuses and directs you to `edikt install edikt`.
2. **Read config.** Resolves `decisions_dir` (default `docs/architecture/decisions`) and `invariants_dir` (default `docs/architecture/invariants`) from `.edikt/config.yaml`.
3. **Resolve the target sidecar.** `ADR-NNN` / `INV-NNN` resolve to the matching `<ID>-*.edikt.yaml`; an `.md` path resolves to its sibling `.edikt.yaml`; an `.edikt.yaml` path is used directly.
4. **Show current manual directives.** Displays the sidecar's existing `manual_directives[]` (or "(none)").
5. **Prompt for new text** and **validate the modal verb.** The text must contain one of `MUST`, `MUST NOT`, `SHOULD`, `SHOULD NOT`, `MAY`, `NEVER`, `ALWAYS` (case-insensitive). A directive states a constraint, not a preference — text without a modal verb is rejected.
6. **Auto-suggest a `(ref: ...)` tag.** If the text has no `(ref:` parenthetical, the command offers to append `(ref: ADR-NNN + manual)`. The binary auto-tags regardless if one is still absent; this step is advisory UX.
7. **Invoke `bin/edikt sidecar add-manual-directive`.** The binary's stdout is displayed verbatim — never parsed. Exit-code handling: `0` success, `1` validation error, `2` sidecar not found, `3` duplicate detected.

## Why this command exists

`manual_directives[]` in a sidecar is the only way to add a MUST / MUST NOT rule to a governance artifact after acceptance — the parent `.md` is immutable. Without tooling, authors hand-edit YAML, which introduces formatting errors and skips the duplicate check. It is also the interactive face of the doctor WARN that names `bin/edikt sidecar add-manual-directive` as the remediation for ADRs with considered options but no prohibition coverage.

## Notes

- This command does not compile governance. After appending a directive, run `/edikt:gov:compile` to bring it into the next governance build.

## Related

- [`/edikt:adr:new`](/commands/adr/new) — capture an architecture decision.
- [`/edikt:adr:compile`](/commands/adr/compile) — regenerate an ADR sidecar.
- [`/edikt:gov:compile`](/commands/gov/compile) — include the new manual directive in compiled governance.
- [Sidecar architecture](/governance/sidecar) — how directives become matchable governance.
