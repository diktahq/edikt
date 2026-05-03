# ADR-006: CLAUDE.md Sentinel Migration to Visible Markers

**Date:** 2026-03-25
**Status:** Accepted
**Supersedes:** ADR-002

## Context

ADR-002 established HTML comment sentinels (`<!-- edikt:start -->` / `<!-- edikt:end -->`) as the mechanism for safe CLAUDE.md merges. The approach worked at the shell level (grep and sed read raw files), but Claude Code v2.1.72+ introduced a change that hides HTML comments when injecting `CLAUDE.md` into Claude's context.

This made the sentinel boundaries invisible to Claude during normal conversation. If a user asks Claude to "edit my CLAUDE.md", Claude cannot see where the edikt-managed section starts and ends — it may accidentally modify or overwrite content it should not touch.

## Decision

Replace HTML comment sentinels with markdown link reference definitions, which are not stripped by Claude Code:

```
[edikt:start]: # managed by edikt — do not edit this block manually
...edikt-managed content...
[edikt:end]: #
```

The merge logic (three cases) and the Read + Edit approach from ADR-002 remain unchanged. Only the sentinel format changes.

**Backward compatibility:** Both formats are detected. `/edikt:init` migrates old HTML comment markers to the new format when re-running on an existing project. `/edikt:upgrade` explicitly detects and migrates old markers as part of its upgrade flow.

## Why this format

Markdown link reference definitions (`[label]: url`) are:
- **Not HTML comments** — Claude Code does not strip them from injected context
- **Visible to Claude** — Claude reads them as plain text and understands the boundaries
- **Invisible in rendered markdown** — link reference definitions with `#` as the target render as nothing in markdown preview, keeping CLAUDE.md clean
- **Grep-friendly** — `grep -F '[edikt:start]'` is reliable and unambiguous

## Consequences

- **Claude can see boundaries** — Claude in conversation identifies the edikt block and avoids editing it accidentally
- **Backward compatible** — old HTML comment format is still detected and auto-migrated, no manual action required for existing projects
- **Shell tooling unaffected** — grep, sed, and all hook scripts read raw files; neither format causes issues at the shell level

## Directives

[edikt:directives:start]: #
topic: hooks
paths:
  - "templates/CLAUDE.md.tmpl"
  - "commands/init.md"
  - "commands/upgrade.md"
scope:
  - implementation
directives:
  - CLAUDE.md sentinels use visible markdown link reference definitions: `[edikt:start]: #` and `[edikt:end]: #`. NEVER use HTML comment sentinels. (ref: ADR-006)
  - Detect both formats (HTML comments and link references) for backward compatibility during init and upgrade. (ref: ADR-006)
[edikt:directives:end]: #
