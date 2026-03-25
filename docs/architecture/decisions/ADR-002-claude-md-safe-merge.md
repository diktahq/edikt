# ADR-002: Safe CLAUDE.md Merge via Sentinel Markers

**Date:** 2026-03-06
**Status:** Accepted (updated 2026-03-25)

## Context

edikt needs to write content into `CLAUDE.md` during `edikt:init` and when re-initializing. Projects often already have a `CLAUDE.md` with carefully crafted instructions, team conventions, tool configurations, or project-specific rules.

Overwriting the entire file is destructive and would make edikt hostile to adoption on established projects. A "Write" approach also breaks re-init — running `edikt:init` again would clobber any manual edits made since the last init.

## Decision

edikt manages its content in `CLAUDE.md` via visible text sentinel markers:

```
[edikt:start]: # managed by edikt — do not edit this block manually
...edikt-managed content...
[edikt:end]: #
```

**Three cases:**

1. **No CLAUDE.md exists** — create the file containing only the edikt block
2. **CLAUDE.md exists, no edikt block** — append the edikt block at the bottom, leave everything above untouched
3. **CLAUDE.md exists, edikt block present** — replace only the content between `[edikt:start]` and `[edikt:end]`, leave everything outside untouched

Implementation uses Read + grep to find markers, then Edit (not Write) to replace only the sentinel block.

**Backward compatibility:** `<!-- edikt:start` (old HTML comment format) is also detected and treated as a valid edikt block. `/edikt:init` migrates old markers to the new format when it encounters them. `/edikt:upgrade` detects and migrates old markers explicitly.

## Why not HTML comments

The original implementation used `<!-- edikt:start -->` HTML comments. Claude Code v2.1.72+ hides HTML comments when injecting `CLAUDE.md` into Claude's context. This meant Claude could not see the sentinel boundaries during normal conversation — asking Claude to "edit my CLAUDE.md" could result in it accidentally modifying or overwriting edikt's managed section.

The new markers use markdown link reference definition syntax (`[label]: #`). These are:
- **Visible to Claude** — not HTML comments, not stripped by Claude Code
- **Invisible in rendered markdown** — link reference definitions with no URL render as nothing in preview
- **Grep-friendly** — `grep -F '[edikt:start]'` is reliable

## Consequences

- **Safe for complex CLAUDE.md files** — team conventions, custom instructions, and project rules are never touched
- **Safe to re-run** — `edikt:init` can be run repeatedly without clobbering manual edits
- **Claude can see boundaries** — Claude in conversation can identify the edikt block and avoid editing it accidentally
- **Backward compatible** — old HTML comment format is still detected and auto-migrated on upgrade

## What is NOT in the edikt block

User-owned sections of CLAUDE.md (outside the sentinels) are never read or modified by edikt. If a user wants to customize edikt behavior, they edit inside the block — or add their own sections outside it.
