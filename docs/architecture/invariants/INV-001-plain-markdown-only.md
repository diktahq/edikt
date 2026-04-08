# INV-001 — Commands are plain markdown, no compiled code

**Status:** Active

## Rule

Every edikt command is a `.md` file. No TypeScript, no compiled binaries, no build step. The system must work by copying files into `~/.claude/commands/`.

## Why

- Zero installation friction — copy files, done
- Claude reads markdown natively — no wrapper needed
- Contributors can read and edit commands without a dev environment
- Survives Claude Code API changes — worst case, edit the markdown

## Enforcement

- No `package.json`, `go.mod`, or build files at the root
- All commands live in `commands/` as `.md` files
- Templates are plain `.md` files in `templates/`

## Directives

[edikt:directives:start]: #
paths:
  - "**/*"
scope:
  - planning
  - design
  - review
  - implementation
directives:
  - Every command and template MUST be a `.md` or `.yaml` file. No TypeScript, no compiled binaries, no build step. Installation is copy files only — no npm, no package managers. (ref: INV-001)
[edikt:directives:end]: #
