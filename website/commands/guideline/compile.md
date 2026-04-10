# /edikt:guideline:compile

Generate or regenerate directive sentinel blocks for guidelines.

Reads the `## Rules` section of each guideline and produces MUST/NEVER directives with reminders and verification checklist items. Uses the same three-list schema ([ADR-008](https://github.com/diktahq/edikt/blob/main/docs/architecture/decisions/ADR-008-deterministic-compile-and-three-list-schema.md)) and hash-based caching as ADR and invariant compilation.

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

1. Reads each guideline's `## Rules` section
2. Each MUST/NEVER bullet becomes a directive
3. Soft language ("should", "prefer", "try to") is **rejected** with a warning
4. Generates `reminders:` (pre-action interrupts) and `verification:` (checklist items)
5. Writes the sentinel block with hash metadata for caching

```yaml
[edikt:directives:start]: #
source_hash: "a3b2..."
directives_hash: "9f8e..."
compiler_version: "0.3.0"
directives:
  - "Every HTTP handler MUST return Content-Type: application/json (ref: api-design)"
reminders:
  - "Before writing a handler response → MUST set Content-Type (ref: api-design)"
verification:
  - "[ ] Every handler sets Content-Type: application/json (ref: api-design)"
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #
```

## Soft language rejection

Guidelines that use hedging language are skipped:

```
⚠ Skipped soft rule in api-design.md: "Responses should be consistent"
  Guidelines should use MUST/NEVER. Either rewrite the rule or omit it.
```

## Related commands

- [`/edikt:guideline:new`](new) — create a new guideline
- [`/edikt:guideline:review`](review) — review language quality + directive LLM compliance
- [`/edikt:gov:compile`](/commands/gov/compile) — compile all sources into governance.md
- [Guidelines](/governance/guidelines) — what guidelines are and when to use them
- [Sentinel Blocks](/governance/sentinels) — the technical format
