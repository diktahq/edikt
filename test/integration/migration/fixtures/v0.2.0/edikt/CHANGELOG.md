# edikt changelog

## v0.2.0 (2026-03-31)

### Intelligent Compile — topic-grouped rule files

`/edikt:compile` no longer produces a single flat `governance.md`. It now generates **topic-grouped rule files** under `.claude/rules/governance/` — each topic file contains full-fidelity directives from all sources (ADRs, invariants, guidelines), loaded automatically by path matching.

- **Directive sentinels** — ADRs and invariants can include `[edikt:directives:start/end]` blocks with pre-written LLM directives. Compile reads these verbatim — no extraction, no distillation.
- **Routing table** — `governance.md` becomes an index with invariants + a routing table. Claude matches task signals and scopes to load relevant topic files.
- **Three loading mechanisms** — `paths:` frontmatter (platform-enforced on file edits), `scope:` tags (activity-matched for planning/design/review), and signal keywords (domain-matched).
- **No directive cap** — the 30-directive limit is removed. Soft warning if a topic file exceeds 100 directives.
- **Reverse source map** — compile output shows which ADRs/guidelines contributed to which topic files.
- **Sentinel generation moved to compile** — `/edikt:compile` now generates missing sentinel blocks inline before compiling. No separate step needed. `/edikt:review-governance` is now pure language quality review + staleness detection.
- `/edikt:review-governance` redesigned — language quality review only. Detects stale sentinels and directs to compile. No longer generates anything.

### Command namespacing

edikt commands are now grouped into namespaces matching the artifacts they touch. Nested namespacing confirmed working in Claude Code.

**New structure:**
- `edikt:adr:new` / `:compile` / `:review` — ADR lifecycle
- `edikt:invariant:new` / `:compile` / `:review` — invariant lifecycle
- `edikt:guideline:new` / `:review` — guideline management
- `edikt:gov:compile` / `:review` / `:rules-update` / `:sync` — governance assembly
- `edikt:sdlc:prd` / `:spec` / `:artifacts` / `:plan` / `:review` / `:drift` / `:audit` — SDLC chain
- `edikt:docs:review` / `:intake` — documentation
- `edikt:capture` — mid-session decision sweep (new command)

**New commands:** `capture`, `guideline:new`, `guideline:review`, `adr:compile`, `adr:review`, `invariant:compile`, `invariant:review`

**Deprecated** (removed in v0.4.0): old flat names (`edikt:adr`, `edikt:compile`, `edikt:spec`, etc.) — each stub tells you the new name.

### Agent governance

All 19 agent templates now include governance frontmatter:

- **`maxTurns`** — 10 for advisory agents, 20 for code-writing agents, 15 for the evaluator.
- **`disallowedTools`** — advisory agents have `Write` and `Edit` disallowed at the platform level.
- **`effort`** — high for architect/security/qa/performance/compliance, medium for backend/frontend/dba/api/sre/docs/pm/data/platform/ux, low for gtm/seo.
- **Agent effort fixes** — `data` was `low` with `disallowedTools: [Write, Edit]` which blocked artifact creation. Fixed to `medium` with write access. `platform`, `compliance`, and `ux` effort levels corrected to match their review depth.
- **`initialPrompt`** — architect, security, and pm auto-load project context when run as main session agents.
- **New `evaluator` agent** — phase-end evaluator that verifies work against acceptance criteria with fresh context. Skeptical by default.

### Hook modernization

- **Conditional `if` field** on PostToolUse (scopes to code files only) and InstructionsLoaded (scopes to rule files only). Avoids spawning hook processes for non-matching files.
- **4 new hooks** — `StopFailure` (logs API errors), `TaskCreated` (tracks plan phase parallelism), `CwdChanged` (monorepo directory detection), `FileChanged` (warns on external governance file modifications).

### Harness improvements

- **Context reset guidance** — at phase boundaries, edikt recommends starting a fresh session. State lives in the plan file.
- **Phase-end evaluation** — evaluator agent checks acceptance criteria with binary PASS/FAIL per criterion before suggesting context reset.
- **Acceptance criteria per phase** — plans now include testable, binary assertions per phase. Specs enforce downstream flow.
- **Conditional evaluation** — `evaluate: true/false` per phase. High-effort phases evaluate by default, low-effort skip.
- **Evaluator tuning** — `docs/architecture/evaluator-tuning.md` tracks false positives/negatives for prompt refinement.
- **Harness assumptions** — `docs/architecture/assumptions.md` documents 6 testable assumptions about model limitations. `/edikt:upgrade` prompts for re-testing.

### Rule pack UX

- **Conflict detection** — `/edikt:rules-update` checks new rule packs against compiled governance before installing.
- **Install preview** — shows what will change (added/changed/removed sections) before applying updates.
- **Override transparency** — `/edikt:doctor` and `/edikt:status` report compiled governance status, sentinel coverage, and rule pack overrides.

### Installer safety

- **`--dry-run`** — preview what the installer would change without writing files.
- **Backup before overwrite** — existing files backed up to `~/.edikt/backups/{timestamp}/` before overwriting.
- **Existing install detection** — reports installed version and confirms before proceeding.

### Headless & CI foundations

- **`--json` output** — compile, drift, audit, doctor, review, and review-governance support `--json` for machine-readable output.
- **Headless mode** — `EDIKT_HEADLESS=1` with `headless-ask.sh` hook auto-answers interactive prompts for CI pipelines.
- **CI guide** — new website guide with GitHub Actions example, recommended settings, and environment variables.
- **Managed settings awareness** — `/edikt:team` detects organization-managed settings (`managed-settings.json`, `managed-settings.d/`).

### UX consistency improvements

- **Standardized completion signals** — all 25 commands end with `✅ {Action}: {identifier}` + `Next:` line.
- **Standardized error messages** — all commands that read config use the same missing-config message.
- **Config guards** — 10 additional commands now guard for missing `.edikt/config.yaml` instead of failing mid-execution.
- **Init rule preview** — step 3b shows a preview of actual rules before generating files, with customization paths taught at the moment of installation.
- **Init reconfigure protection** — content hash comparison detects edited files. Per-file `[1] Overwrite / [2] Keep mine / [3] Show diff` flow instead of silent overwrites.
- **Composite config screen** — SDLC options merged into the single combined rules/agents view. One screen, one confirmation.
- **Concrete init summary** — before/after with stack-specific examples from installed rules and agents.
- **Agent routing standardized** — all commands use `🔀 edikt: routing to {agents}` format.
- **Progress breadcrumbs** — compile, audit, review, drift, and review-governance show `Step N/M:` progress.
- **Numbered confirmation options** — letter-code choices (`[a]/[s]/[k]`) replaced with `[1]/[2]/[3]`.
- **Emoji key** — output conventions table added to CLAUDE.md template.

### Bug fixes

- **Plan ignores spec artifacts when generating phases** — `/edikt:plan` now scans the spec directory for all artifact files (fixtures, test strategy, API contracts, event contracts, migrations) and verifies each has plan coverage. Uncovered fixtures get a seeding phase, uncovered test categories get test tasks, uncovered API endpoints get a warning. A hard gate (step 6c) blocks plan writing if any artifact has no coverage — the user must add phases, defer explicitly, or cancel. Prevents silent failures where artifacts are generated but never consumed.
- **Cross-reference validation in compile and review-governance** — both commands now verify that every `(ref: INV-NNN)` and `(ref: ADR-NNN)` reference points to an actual source document. Fabricated references are stripped before writing.
- **Plan trigger not matching "let's create a plan to fix X"** — added trigger examples with trailing context ("plan to fix these issues", "plan these changes", "plan this work") so Claude matches the plan intent even when the sentence includes what to fix.
- **SessionStart hook errors on compact** — `set -euo pipefail` caused silent non-zero exits when Claude Code fires `SessionStart` after `/compact`. Relaxed to `set -uo pipefail` — the hook already guards every fallible command with `|| true`.
- **Test suite requires pyyaml** — agent and registry tests used `python3 -c "import yaml"` which fails silently when pyyaml isn't installed. Rewrote agent frontmatter checks in pure bash, registry checks to fall back to `yq`, and `assert_valid_yaml` to try `yq` when python3-yaml is unavailable.

### Platform alignment

- **Environment hardening** — `/edikt:team` checks for `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB`. Security guide documents `sandbox.failIfUnavailable`.
- **SendMessage auto-resume** — documented on website for agent resumption.

## v0.1.4 (2026-03-28)

### Brainstorm command

New `/edikt:brainstorm` command — a thinking companion for builders. Open conversation grounded in project context, with specialist agents joining as topics emerge. Converges toward a PRD or SPEC when ready. Use `--fresh` for unconstrained brainstorming that challenges existing decisions. Brainstorm artifacts saved to `docs/brainstorms/`.

### Upgrade version check

`/edikt:upgrade` now checks for newer edikt releases before upgrading the project. If a newer version exists, it shows the install command and stops — ensuring project upgrades always use the latest templates. Skip with `--offline` for air-gapped environments.

## v0.1.3 (2026-03-27)

### Flexible plan input

`/edikt:plan` now accepts any input format — natural language prompts, existing plan names, ticket IDs, SPEC identifiers, or nothing (infers from conversation context). When the intent is ambiguous (natural language or conversation context), edikt offers a choice between a full phased plan (saved to `docs/plans/`) and a quick conversational plan.

- `PLAN-NNN` input: continue from current phase, re-plan remaining phases, or create a sub-plan
- Empty input: infers from current conversation context before asking
- Natural language: offers full vs quick plan disambiguation

## v0.1.2 (2026-03-27)

### Bug fix

- **Installer prompt auto-answered when piped** — `curl | bash` triggered the interactive install mode prompt which got EOF from stdin, flashing the prompt and auto-selecting global. Now detects non-terminal stdin and defaults to global silently. Use `--project` flag for project-local install.

## v0.1.1 (2026-03-27)

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
