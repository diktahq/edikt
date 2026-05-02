# edikt sidecar schemas

This directory ships the JSON Schemas that govern edikt's `*.edikt.yaml` sidecar files. Each schema is versioned in its filename (`<artifact>.vN.schema.json`) so that schema evolution stays explicit and parallel-deployable.

## Files (current)

| File | Purpose |
|---|---|
| `sidecar.v1.schema.json` | Governance sidecar (ADR / invariant / guideline) per ADR-027. The on-disk shape compiled by `edikt gov compile`. |
| `prd-sidecar.v1.schema.json` | PRD sidecar per SPEC-007. Co-located with `<id>.md` to capture structured requirements / ACs. |
| `spec-sidecar.v1.schema.json` | SPEC sidecar per SPEC-007. Co-located with `<id>.md`. |

## Versioning policy

Schemas in this directory follow a simple, parallel-shipped versioning model:

1. **Each schema's filename carries its major version.** A schema named `*.v1.schema.json` is frozen at v1 once it ships in a release. Adding properties, tightening constraints, or changing required keys is a v2 — a new file (`*.v2.schema.json`) ships alongside v1.
2. **Within a major version, only additive non-breaking changes are allowed.** Adding optional properties is in scope. Adding required keys, removing keys, or changing key semantics is a major-version bump.
3. **Major versions ship in parallel during a deprecation window.** When v2 ships, v1 stays in this directory for at least one full release cycle so consumers have time to migrate. The old file is removed in a later release after a documented migration window.
4. **The on-disk `schema_version:` integer in a sidecar is the runtime contract.** A reader uses the integer to dispatch — `schema_version: 1` reads with v1 rules, `schema_version: 2` reads with v2 rules. Filename is documentation; integer is enforcement.
5. **`$id` reflects the major version.** v1 schemas use `https://edikt.dev/schemas/<artifact>/v1.json`; v2 will use `.../v2.json`. The two `$id`s never collide so cached references in editors stay correct across the migration window.

## Why filenames carry the version

Editor tooling (`yaml-language-server`, ajv, JetBrains, Neovim) caches schema content keyed off the file path. Renaming the file when the schema changes major version forces a cache invalidation; users get the new validation rules without any per-project intervention. The unversioned form (`<artifact>.schema.json`) used through v0.5.x was renamed to the v1 form in v0.6.0 per Phase 5 of `PLAN-sidecar-review-fixes.md` (#31).

## When to bump major version

Bump when any of the following is true:

- A required key is added, renamed, or removed.
- A type changes (e.g., string → array of strings).
- An enum loses a member.
- A pattern tightens such that previously-valid values become invalid.
- The semantics of an existing key change (the YAML parses, but the meaning differs).

Do not bump for:

- New optional keys with safe defaults.
- New enum members.
- Description / title / `$id` cosmetic changes.
- Adding `examples` or `default` annotations.

## Bumping checklist (when v2 lands)

1. Copy `*.v1.schema.json` to `*.v2.schema.json`. Update `$id` to `.../v2.json`.
2. Apply the breaking change to the v2 file. Leave v1 untouched.
3. Add a "v1 → v2 migration" section to this README documenting the diff and the equivalent `migrate sidecars --to v2` command (or equivalent).
4. Update consumers (Go validator, ajv CI step, editor headers, doctor checks) to dispatch on the integer `schema_version:` field.
5. Document the deprecation window in `CHANGELOG.md` and the upgrade guide.
