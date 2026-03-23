# ADR-002: Safe CLAUDE.md Merge via Sentinel Comments

**Date:** 2026-03-06
**Status:** Accepted

## Context

edikt needs to write content into `CLAUDE.md` during `edikt:init` and when re-initializing. Projects often already have a `CLAUDE.md` with carefully crafted instructions, team conventions, tool configurations, or project-specific rules.

Overwriting the entire file is destructive and would make edikt hostile to adoption on established projects. A "Write" approach also breaks re-init — running `edikt:init` again would clobber any manual edits made since the last init.

## Decision

edikt manages its content in `CLAUDE.md` via sentinel HTML comments:

```
<!-- edikt:start — managed by edikt, do not edit manually -->
...edikt-managed content...
<!-- edikt:end -->
```

**Three cases:**

1. **No CLAUDE.md exists** — create the file containing only the edikt block
2. **CLAUDE.md exists, no edikt block** — append the edikt block at the bottom, leave everything above untouched
3. **CLAUDE.md exists, edikt block present** — replace only the content between `<!-- edikt:start -->` and `<!-- edikt:end -->`, leave everything outside untouched

Implementation uses Read + grep to find markers, then Edit (not Write) to replace only the sentinel block.

## Consequences

- **Safe for complex CLAUDE.md files** — team conventions, custom instructions, and project rules are never touched
- **Safe to re-run** — `edikt:init` can be run repeatedly without clobbering manual edits
- **Clearly delimited** — the edikt section is easy to find, understand, and remove if needed
- **No proprietary format** — just HTML comments, readable by anyone

## What is NOT in the edikt block

User-owned sections of CLAUDE.md (outside the sentinels) are never read or modified by edikt. If a user wants to customize edikt behavior, they edit inside the block — or add their own sections outside it.
