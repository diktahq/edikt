# Contributing to edikt

edikt is plain markdown, YAML, and bash. No build step. No compiled code. If you can read it, you can contribute to it.

## Quick start

```bash
git clone https://github.com/diktahq/edikt
cd edikt
make dev        # links .sandbox/ to this working tree — safe, isolated
make test       # fast offline tests (~30s)
```

Everything edikt-related in your terminal will now use your local checkout. Your real `~/.edikt/` is untouched.

---

## Sandbox vs global

All `make` targets default to a **local sandbox** at `.sandbox/` inside the repo. This means:

- Your real `~/.edikt/` is never touched
- Your live Claude Code sessions are unaffected
- You can break hooks freely and just `make sandbox-clean` to reset

When you're ready to test against your real Claude Code session:

```bash
make dev-global       # points your real ~/.edikt/ at this working tree
make dev-global-off   # reverts to last real release
```

---

## Project structure

```
edikt/
├── commands/           # Slash commands (.md files, no build step)
│   ├── sdlc/           # PRD, spec, artifacts, plan, drift, review, audit
│   ├── gov/            # compile, review, score, sync, rules-update
│   ├── adr/            # new, compile, review
│   └── ...
├── templates/
│   ├── agents/         # Specialist agent templates
│   ├── hooks/          # Lifecycle hook scripts (.sh)
│   └── rules/          # Rule pack templates
├── bin/edikt           # POSIX sh launcher (versioning, migration, doctor)
├── install.sh          # Bootstrap: installs the launcher
├── test/
│   ├── run.sh          # Main test runner (all layers)
│   ├── unit/           # Layer 1: hook + launcher unit tests (bash)
│   └── integration/    # Layer 2: SDK tests + offline Python tests
└── docs/
    ├── architecture/decisions/     # ADRs
    └── architecture/invariants/    # Invariant Records
```

---

## Running tests

```bash
make test              # fast — hooks + launcher, no API key (~30s)
make test-governance   # offline governance integrity checks (~5s)
make test-regression   # regression museum, no API key (~5s)
make test-sdk          # Layer 2 SDK tests — needs claude auth or ANTHROPIC_API_KEY (~5min)
make test-all          # everything
```

For the SDK tests, either be logged in to Claude Code (`claude auth login`) or set `ANTHROPIC_API_KEY` in `test/integration/.env`:

```
ANTHROPIC_API_KEY=sk-ant-...
```

---

## Making changes

### Commands (`commands/**/*.md`)

Each command is a markdown file Claude reads as a system prompt. The filename maps to the slash command: `commands/sdlc/spec.md` → `/edikt:sdlc:spec`.

Required frontmatter:
```yaml
---
name: edikt:sdlc:spec
description: "..."
effort: low | medium | high
allowed-tools:
  - Read
  - Write
  - ...
---
```

`allowed-tools` is required — without it the command silently fails in SDK/headless mode.

### Hooks (`templates/hooks/*.sh`)

Hooks are bash scripts invoked by Claude Code lifecycle events. Each receives JSON on stdin and writes JSON to stdout.

Test a hook change:
```bash
make test-hooks
# or specifically:
bash test/unit/hooks/test_stop_hook.sh .
```

### Agents (`templates/agents/*.md`)

Agent templates are markdown files with YAML frontmatter. Required fields: `name`, `description`, `tools`, `maxTurns`, `effort`.

Read-only agents (evaluator, docs, architect) **must** list `Write` and `Edit` in `disallowedTools`. The `test-governance` suite enforces this.

### Launcher (`bin/edikt`)

The launcher is POSIX sh. Run `sh -n bin/edikt` to syntax-check before testing.

The launcher unit tests cover every subcommand:
```bash
make test-launcher
```

---

## Commit convention

```
{type}({scope}): {description}
```

Types: `feat` | `fix` | `refactor` | `test` | `docs` | `chore`

Examples:
```
feat(commands): add --sidecar-only flag to /edikt:sdlc:plan
fix(hooks): phase-end-detector warns when evaluation history missing
test(e2e): governance compile chain verified through real Claude
chore(deps): bump claude-agent-sdk to 0.1.59
```

---

## Architecture decisions (ADRs)

Significant technical choices are recorded as ADRs in `docs/architecture/decisions/`.

**Critical rules (INV-002):**
- ADRs in `draft` status can be freely edited
- Once `accepted`, an ADR is **immutable** — never edit it
- To change a decision: create a new ADR that supersedes the old one
- Update the old ADR's `Status:` line to `Superseded by ADR-NNN` — that's the only permitted edit

To capture a decision:
```
/edikt:adr:new <decision description>
```

---

## Governance compile

`governance.md` and the topic files under `.claude/rules/governance/` are **generated** — don't edit them by hand.

After changing or adding ADRs or invariants:
```
/edikt:gov:compile
```

This reads the directive sentinel blocks from every accepted ADR and active invariant and writes the topic-grouped rule files Claude reads automatically.

The compile pipeline is tested by `make test-governance`, which verifies source hashes, routing table integrity, and that every routing entry points to a real file.

---

## Before opening a PR

```bash
make test          # must be clean
make test-governance  # must be clean
```

If you're changing commands or adding new ones, also run:
```bash
make test-sdk      # needs API key — catches the allowed-tools class of bugs
```

---

## Questions

Open an issue on GitHub or ask in the repo discussions.
