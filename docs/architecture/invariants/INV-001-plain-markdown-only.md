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
source_hash: dd097e717f15b54b64b5959c4334f78093be9871e8ac33af0b1febc4eaf9a9cc
directives_hash: a3cd524585a3f28cefbffd7c75c4df3181605ab79d592bf26c935069da4b8509
compiler_version: "0.6.0"
paths:
  - "**/*"
scope:
  - planning
  - design
  - review
  - implementation
directives:
  - Every command and template MUST be a `.md` or `.yaml` file. No TypeScript, no compiled binaries, no build step. Installation is copy files only — no npm, no package managers. (ref: INV-001)
manual_directives: []
suppressed_directives: []
canonical_phrases:
  - "plain markdown"
  - "no build step"
  - "copy install"
behavioral_signal:
  refuse_to_write:
    - ".ts"
    - ".js"
    - ".py"
    - "package.json"
    - "tsconfig.json"
  cite:
    - "INV-001"
[edikt:directives:end]: #
