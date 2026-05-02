# INV-001 — Tier-1 commands are plain markdown, no compiled code

**Status:** Active

## Rule

Every edikt **tier-1** command is a `.md` file, and every tier-1 template is `.md`, `.yaml`, or `.sh`. No TypeScript, no compiled binaries, no build step in tier-1. The tier-1 system must work by copying files into `~/.claude/commands/`.

`.sh` is included for hook templates under `templates/hooks/` only — those scripts run inside the Claude Code hook protocol and are written as POSIX bash with stdlib-only Python heredocs (no third-party runtime deps). They are still copy-installed and survive any platform change, satisfying the same contract as `.md` and `.yaml` templates.

**Tier-2 helpers** (e.g. `tools/edikt/`, `tools/<name>/`) are explicitly out of scope of this invariant per ADR-021 (Go as Tier-2 language) and ADR-022 (single Go binary replaces bash launcher). Tier-2 binaries are produced from compiled code, distributed via signed release artifacts (ADR-016), and installed opt-in via `edikt install <helper>`. Tier-1 files MUST NOT runtime-depend on tier-2 binaries — see ADR-015 for the boundary rules.

The split exists because tier-1 is what edikt's contract is built on (plain markdown, copy-installable, survives any platform change). Tier-2 is the ergonomic layer (deterministic helpers, performance-bound work, anything that benefits from a compiled binary). Both ship together; only tier-1 is invariant under this rule.

## Why

- Zero installation friction for tier-1 — copy files, done
- Claude reads markdown natively — no wrapper needed
- Contributors can read and edit tier-1 commands without a dev environment
- Tier-1 survives Claude Code API changes — worst case, edit the markdown
- Tier-2 lets us ship compiled deterministic work (compile, doctor, migrate, verify) without infecting tier-1's contract

## Related ADRs

- ADR-015 — Tier-2 tooling boundary (declares the tier-1/tier-2 split)
- ADR-021 — Go as the Tier-2 language
- ADR-022 — Single Go binary replaces bash launcher

## Enforcement

- No `package.json`, root-level `go.mod`, or build files at the **tier-1** root
- All tier-1 commands live in `commands/` as `.md` files
- Tier-1 templates are plain `.md`, `.yaml`, or (under `templates/hooks/` only) `.sh` files in `templates/`. `.sh` hook templates MUST be POSIX bash and MUST NOT depend on third-party runtimes — embedded Python heredocs are stdlib-only.
- Tier-2 code lives under `tools/<name>/` with its own `go.mod`; tier-2 binaries MUST NOT be committed to the repo (ADR-022)

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
