# edikt changelog

## v0.1.1 (2026-03-25)

### Numbered findings in reviews

All review commands now enumerate findings with IDs (#1, #2, #3...) so users can select which to address by number.

- `/edikt:plan` — pre-flight findings numbered, triage prompt: "Which findings should I address? (e.g., #1, #4 or 'all critical')"
- `/edikt:review` — implementation review findings numbered across all agents
- `/edikt:audit` — security and reliability findings numbered across sections
- `/edikt:drift` — diverged findings include triage prompt
- `/edikt:doctor` — warnings and failures numbered for easy reference

### Natural language triggers for all 24 commands

The CLAUDE.md command table now matches intent, not exact phrases. All 24 commands have natural language triggers (was 14). Each command has an intent label and broader representative examples. "Create me a plan for this ticket", "help me plan this out", "spec this out", "are we on track with the spec", "run a security audit", "check my setup" — all trigger the right command.

### Bug fixes

- **Init hook filename hallucination** — `/edikt:init` now reads the settings template exactly as-is instead of generating hook filenames. Fixes `stop-signals.sh: No such file or directory` error.
- **PostToolUse gofmt error** — `gofmt -w` failures on invalid Go syntax no longer propagate as hook errors.
- **Drift report only saving frontmatter** — `/edikt:drift` now explicitly writes the full report (frontmatter + body), not just the frontmatter.
- **Plan mode guard** — All 8 interactive commands (`init`, `plan`, `prd`, `spec`, `spec-artifacts`, `adr`, `invariant`, `intake`) now detect plan mode and tell you to exit it first, instead of silently skipping the interview.
- **Installer preserves customized commands** — `install.sh` now checks for `<!-- edikt:custom -->` before overwriting, so customized commands survive reinstall.

### spec-artifacts redesign — design blueprints with database type awareness

`/edikt:spec-artifacts` now treats every artifact as a design blueprint: it defines intent and structure, not implementation. Your code is the implementation.

**Database-type-aware data model.** The data model artifact format is now resolved from your database type:

- SQL → `data-model.mmd` (Mermaid ERD with entities, relationships, index comments)
- MongoDB/Firestore → `data-model.schema.yaml` (JSON Schema in YAML)
- DynamoDB/Cassandra → `data-model.md` (access patterns, PK/SK/GSI design)
- Redis/KV stores → `data-model.md` (key schema table with TTL and namespace)
- Mixed stacks → both artifacts, suffixed to avoid collision (`data-model-sql.mmd`, `data-model-kv.md`, etc.)

**Database type resolution — four-priority chain:** spec frontmatter `database_type:` → config `artifacts.database.default_type` → keyword scan of spec content → ask the user. Config is set automatically by `/edikt:init` from code signals.

**Native artifact formats.** API contracts are now OpenAPI 3.0 YAML (`contracts/api.yaml`). Event contracts are AsyncAPI 2.6 YAML (`contracts/events.yaml`). Fixtures are portable YAML (`fixtures.yaml`). Migrations are numbered SQL files (`migrations/001_name.sql`). No more markdown wrappers.

**Migrations are SQL-only.** Document and key-value databases never produce migration files.

**Invariant injection.** Active invariants are loaded from your governance chain, stripped of frontmatter, and injected as structured constraints into every agent prompt. Superseded invariants are excluded. Empty invariant bodies emit a warning.

**Design blueprint header.** Every generated artifact gets a format-appropriate comment header marking it as a blueprint, not implementation code.

**Config contract.** `/edikt:init` now detects database type and migration tool from code signals and writes `artifacts.database.default_type` and `artifacts.sql.migrations.tool` to config. The `artifacts:` block is now part of the standard config schema.

### HTML sentinel migration — CLAUDE.md section boundaries now visible to Claude

Claude Code v2.1.72+ hides `<!-- -->` HTML comments when injecting `CLAUDE.md` into Claude's context. The old `<!-- edikt:start -->` / `<!-- edikt:end -->` sentinels were invisible to Claude, so asking Claude to "edit my CLAUDE.md" could accidentally overwrite edikt's managed section.

New format uses markdown link reference definitions, which survive Claude Code's injection intact:

```
[edikt:start]: # managed by edikt — do not edit this block manually
...
[edikt:end]: #
```

- `/edikt:init` writes the new format on fresh installs and migrates old markers when re-running
- `/edikt:upgrade` detects and migrates old HTML sentinels as part of the upgrade flow
- Both old and new formats are detected for backward compatibility
- ADR-002 updated to document the change and rationale

### Effort frontmatter on all commands

All 24 commands now declare `effort: low | normal | high` in their frontmatter. Claude Code uses this to tune the model's thinking budget per command.

- `low` — `agents`, `context`, `mcp`, `status`, `team`
- `normal` — `adr`, `compile`, `doctor`, `init`, `intake`, `invariant`, `review-governance`, `rules-update`, `session`, `sync`, `upgrade`
- `high` — `audit`, `docs`, `drift`, `plan`, `prd`, `review`, `spec`, `spec-artifacts`

### Init improvements

- **Existing ADR import** — `/edikt:init` now detects existing architecture decisions and offers to import them into edikt's governance structure.
- **Project-local install** — `install.sh --project` installs edikt into the current project (`.claude/commands/`, `.edikt/`) instead of globally. Default is still global.
- **Database detection** — `/edikt:init` detects database type and migration tool from 30+ code signals across Go, Node, Python, Ruby, C#, Elixir, and Rust. Definitive signals (e.g., `prisma/schema.prisma`) auto-configure. Inferred signals (package dependencies) are flagged. Nothing found triggers targeted greenfield questions.

## v0.1.0 (2026-03-23)

### First public release

edikt governs your architecture and compiles your engineering decisions into automatic enforcement. It governs the Agentic SDLC from requirements to verification — closing the gap between what you decided and what gets built.

**Architecture governance & compliance**
- `/edikt:compile` reads accepted ADRs, active invariants, and team guidelines, checks for contradictions, and produces `.claude/rules/governance.md` — directives Claude follows automatically every session
- 20 rule packs (10 base, 4 lang, 6 framework) — correctness guardrails, not opinions. 14-17 instructions per pack (research-validated sweet spot)
- Domain-specific governance checkpoints with pre-action and post-result verification
- Signal detection: stop hook detects architecture decisions mid-session, suggests ADR capture
- Quality gates: configure agents as gates in `.edikt/config.yaml`. Critical findings block progression with logged override
- Pre-push invariant check: violations block the push. Override with `EDIKT_INVARIANT_SKIP=1`

**Agentic SDLC governance**
- Full traceability chain: `/edikt:prd` → `/edikt:spec` → `/edikt:spec-artifacts` → `/edikt:plan` → execute → `/edikt:drift`
- Status-gated transitions: PRD must be accepted before spec, spec before artifacts
- `/edikt:drift` compares implementation against the full chain with confidence-based severity
- CI support: `--output=json` with exit code 1 on diverged findings

**18 specialist agents**
- architect, api, backend, dba, docs, frontend, performance, platform, pm, qa, security, sre, ux, data, mobile, compliance, seo, gtm
- Used in spec review, plan pre-flight, post-implementation review, and audit

**9 lifecycle hooks**
- SessionStart: git-aware briefing with domain classification
- UserPromptSubmit: injects active plan phase into every prompt
- PostToolUse: auto-formats files after edits
- PostCompact: re-injects plan + invariants after context compaction
- Stop: regex-based signal detection for decisions, doc gaps, security
- SubagentStop: logs agent activity, enforces quality gates
- InstructionsLoaded: logs active rule packs
- PreToolUse: validates governance setup
- PreCompact: preserves plan state

**24 commands**
- Governance chain: `init`, `prd`, `spec`, `spec-artifacts`, `plan`, `drift`, `compile`
- Decisions: `adr`, `invariant`
- Review: `review`, `audit`, `review-governance`, `doctor`
- Observability: `status`, `session`, `docs`
- Setup: `context`, `intake`, `upgrade`, `rules-update`, `sync`, `team`, `mcp`, `agents`

**Research**
- 123 eval runs across 2 experiments proving rule compliance mechanism
- EXP-001: 15/15 compliance with rules vs 0/15 without on invented conventions
- EXP-002: holds under multi-rule conflict, multi-file sessions, Opus vs Sonnet, adversarial prompts
- Reproducible: `experiments/exp-001-rule-compliance/` and `experiments/exp-002-extended-compliance/`

**Website**
- Full documentation at edikt.dev
- Guides: solo engineer, teams, multi-project, greenfield, brownfield, monorepo, security, daily workflow
- Governance section: chain, gates, compile, drift, review-governance

**Zero dependencies**
- Every file is `.md` or `.yaml` — no build step, no runtime, no daemon
- `curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash`
- Claude Code only — uses platform primitives (path-conditional rules, lifecycle hooks, slash commands, specialist agents, quality gates)
