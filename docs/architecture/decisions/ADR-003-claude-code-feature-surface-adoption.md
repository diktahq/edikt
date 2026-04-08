# ADR-003: Adopt Remaining Claude Code Feature Surface

**Status:** Accepted
**Date:** 2026-03-08
**Deciders:** Daniel Gomes

## Context

An audit of edikt against the full Claude Code feature surface revealed that edikt v1 used roughly 15% of what Claude Code offers. edikt v2 addressed the biggest gaps (specialist agents, Stop hook, auto-memory, MCP wiring). Three significant capabilities remain unused:

1. **`PostToolUse` hook** — fires after every tool call; could auto-trigger linting/formatting after edits
2. **`` !`command` `` preprocessing** — shell commands evaluated before a slash command runs; could inject live project state (next ADR number, current plan phase, existing invariant list) into commands
3. **`context: fork`** on commands — runs the command in an isolated subagent, keeping main context clean

Additionally, the linter-aware rule generation (PRD exists, not implemented) requires a new command `edikt:sync` to stay in sync after the initial generation.

## Decision

### PostToolUse hook — adopt for auto-format/lint

Add a `PostToolUse` hook that fires after `Write` or `Edit` on source files. If a formatter is detected in the project (gofmt, prettier, black, rustfmt), run it automatically. This closes the feedback loop: Claude writes code → it's immediately formatted → no manual step needed.

**Noise control:** Only fire when the edited file matches a known source extension. Suppress when writing markdown, YAML, JSON config files — formatting those is rarely the right behavior.

**Disable:** `EDIKT_FORMAT_SKIP=1` env var, or `hooks: { post-tool-use: false }` in `.edikt/config.yaml`.

### `!`command`` preprocessing — adopt for live data injection

Use shell preprocessing in slash commands where live project state matters:

- `/edikt:adr` — inject next available ADR number by counting `docs/decisions/*.md`
- `/edikt:invariant` — inject next available INV number
- `/edikt:prd` — inject next available PRD number
- `/edikt:plan` — inject active plan name and current phase from progress table

This eliminates the "Claude guesses the wrong number" problem for sequential artifacts.

### `context: fork` — adopt selectively

Add `context: fork` frontmatter to commands that do read-only analysis and shouldn't affect the main session.


- `/edikt:doctor` — validation/audit work, no code changes
- `/edikt:status` — dashboard read, no side effects
- `/edikt:docs audit` — doc scanning, no writes

Do NOT fork commands that need to write files (`edikt:init`, `edikt:context`, `edikt:adr`, etc.) — forked subagents can't write back to the parent session.

### Linter-aware rules — implement as `edikt:sync`

Implement linter config → AI rules translation as a new command `edikt:sync`. Runs during `edikt:init` for brownfield projects (auto), and manually when linter config changes.

## Alternatives Considered

**PostToolUse: run formatter in Stop hook instead**
The Stop hook fires after Claude's full response, not after each file write. A response may write multiple files — formatting all of them in Stop would require knowing which files were changed. PostToolUse has the file path directly in context.

**Preprocessing: hardcode placeholder numbers**
Claude could start from `NNN` and let the user rename. But this causes friction and breaks automation. Shell preprocessing is zero-friction.

**`context: fork`: apply to all commands**
Some commands (`edikt:context`, `edikt:init`) need to write to the session's working memory. Forked agents can't do that. Apply only where there's clear benefit.

## Consequences

- `PostToolUse` hook fires on every edit — teams with slow formatters may notice latency; the disable option is essential
- Preprocessing requires files to exist at command invocation time — commands that create their own directories need to handle the case where no files exist yet (count = 0 → start at 001)
- `context: fork` means doctor/status results are returned as agent output, not written to main context — this is fine for read-only commands
- `edikt:sync` adds a 12th command — update all command counts in docs

## Directives

[edikt:directives:start]: #
paths:
  - "templates/hooks/**"
  - "templates/settings.json.tmpl"
  - "commands/**"
scope:
  - implementation
directives:
  - Use PostToolUse hooks for auto-formatting after Write or Edit. Fire only on known source file extensions. Respect `EDIKT_FORMAT_SKIP=1` and config disable. (ref: ADR-003)
  - Use shell preprocessing in slash commands to inject live project state (next ADR/INV/PRD number, active plan phase). NEVER guess or hardcode sequential identifiers. (ref: ADR-003)
  - Agents run in forked subagents (`context: fork`), invoked by edikt on domain signals. They do not coordinate parallel execution — that is Claude Code's responsibility. (ref: ADR-003)
[edikt:directives:end]: #

## Related

- PRD: Linter-Aware Rule Generation (`docs/product/prds/PRD-linter-aware-rules.md`)
- PRD: Claude Code Feature Gap Closure (`docs/product/prds/PRD-claude-code-feature-gaps.md`)
