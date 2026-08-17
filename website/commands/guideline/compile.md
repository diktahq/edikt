# /edikt:guideline:compile

Generate or regenerate the co-located `<guideline>.edikt.yaml` sidecar for a guideline.

Reads the `## Rules` section of each guideline and produces MUST/NEVER directives with reminders and verification checklist items. Uses the v0.6.0 sidecar schema — directives live in the sibling sidecar, never in the prose body. Staleness is recomputed on read against the prose body; no committed hash.

## Usage

```bash
/edikt:guideline:compile                      # all guidelines
/edikt:guideline:compile error-handling       # single guideline by slug
/edikt:guideline:compile --regenerate         # force fresh generation
```

## Arguments

| Argument | Description |
|----------|-------------|
| (none) | Process all guidelines |
| `{slug}` | Process a single guideline |
| `--regenerate` | Force regeneration regardless of hash match |
| `--strategy=regenerate` | Headless: discard hand-edits, rewrite from body |
| `--strategy=preserve` | Headless: skip files with hand-edits |

## How it works

1. Reads each guideline's `## Rules` section.
2. Each MUST/NEVER bullet becomes a directive.
3. Soft language ("should", "prefer", "try to") is **rejected** with a warning.
4. Generates `reminders:` (pre-action interrupts) and `verification:` (checklist items).
5. Writes the canonical sidecar at `{guideline}.edikt.yaml` (siblings the prose).

```yaml
# docs/guidelines/api-design.edikt.yaml
schema_version: 2
topic: api-design
path: api-design.md
signals: [http, handler, json]
directives:
  - text: "Every HTTP handler MUST return Content-Type: application/json (ref: api-design)"
    source_excerpts:
      - line_start: 12
        line_end: 12
        quote: "Every HTTP handler MUST return Content-Type: application/json on success and error."
reminders:
  - "Before writing a handler response → MUST set Content-Type (ref: api-design)"
verification:
  - "[ ] Every handler sets Content-Type: application/json (ref: api-design)"
manual_directives: []
suppressed_directives: []
```

## Soft language rejection

Guidelines that use hedging language are skipped:

```
⚠ Skipped soft rule in api-design.md: "Responses should be consistent"
  Guidelines should use MUST/NEVER. Either rewrite the rule or omit it.
```

## Sidecar regeneration

`:compile` regenerates exactly one `<guideline>.edikt.yaml` sidecar in a fresh subagent context. For a single target:

```text
✅ error-handling.edikt.yaml — regenerated
```

For an all-targets run:

```text
  ✅ error-handling — regenerated
  ✅ http-handlers — unchanged
  ...

  1 regenerated, 1 unchanged.
```

The agent prompt is locked; each artifact compiles in its own forked subagent (`context: fork`) so there is no cross-artifact contamination. Byte-equal regeneration on an unchanged body is the goal, but unlike ADR/invariant compile — where the canonical serializer has shipped — guideline compile's canonical form is still approximate, so "unchanged" isn't yet a hard guarantee here.

You usually don't need to run this directly. `/edikt:gov:compile` Phase A auto-resyncs stale sidecars by dispatching this command per guideline.

See [Sidecar Architecture](/governance/sidecar) for the full schema and legacy → v0.6.0 transition.

## Related commands

- [`/edikt:guideline:new`](new) — create a new guideline (creates the sidecar atomically)
- [`/edikt:guideline:review`](review) — review language quality + cross-check sidecar drift
- [`/edikt:gov:compile`](/commands/gov/compile) — full governance compile (Phase A resync + Phase B merge)
- [Sidecar Architecture](/governance/sidecar) — what sidecars are and why
- [Guidelines](/governance/guidelines) — what guidelines are and when to use them
