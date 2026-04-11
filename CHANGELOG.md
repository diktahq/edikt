# edikt changelog

## v0.4.0 (2026-04-11)

### Plan Harness: Iteration Tracking, Context Handoff, Criteria Sidecar

The plan command now tracks failure history, carries context across phase boundaries, and emits a machine-readable criteria sidecar.

- **Iteration tracking:** progress table with Attempt column (`N/max`), 6 statuses (`pending`, `in-progress`, `evaluating`, `done`, `stuck`, `skipped`). After each evaluation failure, fail reasons are forwarded to the next attempt. Escalation warning at 3 consecutive failures on the same criterion. Phase goes `stuck` at max attempts (configurable, default 5) with human decision prompt.
- **Context handoff:** each phase has a `Context Needed` field listing files to read before starting. Artifact Flow Table maps producing phases to consuming phases. PostCompact hook re-injects context files, attempt count, and failing criteria after compaction.
- **Criteria sidecar:** `PLAN-{slug}-criteria.yaml` emitted alongside plan markdown. Per-criterion status tracking (pending/pass/fail), verification commands, fail counts. Evaluator reads and updates the sidecar — no markdown parsing needed.

### Evaluator: Headless Execution and Configuration

The evaluator now runs as a separate `claude -p` invocation with zero shared context from the generator session.

- **Evaluator config:** new `evaluator` section in `.edikt/config.yaml` with 5 keys: `preflight` (toggle pre-flight), `phase-end` (toggle evaluation), `mode` (headless or subagent), `max-attempts` (stuck threshold), `model` (sonnet/opus/haiku).
- **Headless mode (default):** evaluator runs via `claude -p --bare` with `--disallowedTools "Write,Edit"`. Fresh process, no shared context, no self-evaluation bias. Falls back to subagent when headless unavailable.
- **Protected agent:** evaluator templates are not user-overridable. Upgrade always overwrites them. Doctor warns on modifications. Plan blocks if template is missing.
- **LLM evaluator in experiments:** `--llm-eval` flag in experiment runner. Dual-mode: grep pre-check + LLM evaluation. LLM verdict is authoritative when both run. Three verdicts: PASS, WEAK PASS (all critical pass but important fails), FAIL. Severity tiers: critical (blocks), important (degrades), informational (logged only).

### Enforcement: Quality Gate UX and Artifact Lifecycle

Quality gates now log overrides with accountability, and artifact lifecycle is enforced uniformly across the SDLC chain.

- **Gate override logging:** overrides written to `~/.edikt/events.jsonl` with git identity (name + email). Three event types: `gate_fired`, `gate_override`, `gate_blocked`.
- **Re-fire prevention:** overridden findings don't fire again within the same session. Overrides cleared at session start.
- **Artifact lifecycle:** full status chain `draft → accepted → in-progress → implemented → superseded`. Plan auto-promotes `accepted → in-progress` when phase starts. Drift auto-promotes `in-progress → implemented` when no violations found.
- **Plan draft warning:** lists specific draft artifacts by name, offers proceed (with Known Risks) or stop.
- **Drift status filter:** skips `draft` and `superseded` artifacts, validates the rest.
- **Doctor:** flags spec-artifacts stuck in draft > 7 days. Parses both YAML frontmatter and comment header status formats.

### Documentation

- Fixed 12 pre-existing documentation gaps (stale agent/hook/command counts, old command names in AGENTS.md, missing index entries)
- Updated website: plan, gates, chain, features, doctor, drift pages with v0.4.0 features
- Removed stale AGENTS.md (Codex convention — edikt is Claude Code only per ADR-001)

### New config keys

```yaml
evaluator:
  preflight: true       # pre-flight criteria validation
  phase-end: true       # phase-end evaluation
  mode: headless        # headless | subagent
  max-attempts: 5       # max retries before stuck
  model: sonnet         # model for headless evaluator
```

## v0.3.1 (2026-04-11)

### Bug fixes

- **Init: guidelines path.** `/edikt:init` now writes `paths.guidelines` correctly.
- **VERSION stamp.** `VERSION` file updated to match release tag.
- **PRINCIPAL prefix.** Compile output no longer prefixes directives with `PRINCIPAL:`.
- **Review output.** `/edikt:sdlc:review` output formatting fixed.
- **SubagentStop hook: seniority prefix.** The fallback agent detection pattern matched "As Principal Architect" → `principal-architect` instead of `architect`, breaking slug lookup and gate matching. Now extracts only the role word.
- **Missing page.** Added `/edikt:guideline:compile` website page (was dead link).
- **Test fixes.** All 25 suites pass after v0.3.0 regressions.

### Artifact generation: JSONB support and domain class diagram

`/edikt:sdlc:artifacts` now handles projects using JSONB aggregate storage (common DDD pattern in PostgreSQL) and generates a domain class diagram alongside the data model.

- **Storage strategy detection.** When DB type is `sql` or `mixed`, the command scans spec content and migrations for JSONB signals (`jsonb`, `json column`, `aggregate storage`, `embedded entity`, `nested entity`, etc.). Detected strategy is shown in the state checkpoint and routing output.
- **Three entity modes in `data-model.mmd`.** When storage strategy is `jsonb-aggregate`, the ERD distinguishes physical tables (normal), JSONB-embedded entities (relationship label contains `jsonb`), and reference-only entities from external bounded contexts (relationship label contains `ref`). Makes nested structure visible instead of hiding it in JSONB column comments.
- **Domain class diagram (`model.mmd`).** New artifact type, always generated alongside the data model regardless of DB type. Mermaid `classDiagram` showing aggregate roots, value objects, entities, inheritance, composition, and domain methods. Reviewed by the architect agent.

### Configurable artifact spec versions

Artifact templates now use configurable spec versions instead of hardcoded values. Defaults updated to latest stable:

| Format | Previous | Now (default) |
|---|---|---|
| OpenAPI | 3.0.0 | **3.1.0** |
| AsyncAPI | 2.6.0 | **3.0.0** |
| JSON Schema | draft-07 | **2020-12** |

Teams can pin older versions in `.edikt/config.yaml`:

```yaml
artifacts:
  versions:
    openapi: "3.0.0"       # pin for tooling compatibility
    asyncapi: "2.6.0"      # pin if not ready for 3.0 breaking changes
    json_schema: "https://json-schema.org/draft/07/schema#"
```

The AsyncAPI template was updated for the 3.0 structure (separate `channels` and `operations` blocks replacing `publish`/`subscribe`). When pinning `asyncapi: "2.6.0"`, the agent uses the 2.x structure.

### New `/edikt:config` command

View, query, and modify `.edikt/config.yaml` with discovery, validation, and natural-language changes.

- **No args** — show all 34 config keys with current values and defaults
- **`get {key}`** — show a specific key's value, default, valid values, and which commands use it
- **`set {key} {value}`** — validate and write, with per-key validation rules

Protected keys like `edikt_version` cannot be set directly. Invalid values are rejected with explanation.

### `/edikt:team` deprecated — merged into init + config

`/edikt:team` served two purposes that belong elsewhere:
- **Member onboarding** → now in `/edikt:init`'s "existing project" path
- **Config management** → now in `/edikt:config`

When `/edikt:init` detects an existing `.edikt/config.yaml`, it runs member environment validation instead of saying "already initialized":
1. **Version gate** — blocks if installed edikt < project's `edikt_version`
2. **Environment checks** — git identity, Claude Code, MCP env vars (read dynamically from `.mcp.json`), `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB`, pre-push hook, managed settings
3. **Governance gap sync** — missing rules/hooks/agents offered for install
4. **Shared config display** — what's committed to git

The `team:` config block is no longer used. Legacy blocks in existing configs are ignored silently. The deprecated stub redirects to init and will be removed in v0.5.0.

## v0.3.0 (2026-04-10)

### Project Adaptation (ADR-008, ADR-009)

edikt can now adapt to existing projects. The compile pipeline supports a **three-list directive schema** (ADR-008) with hash-based caching, and introduces **Invariant Records** as the formal governance artifact for hard constraints (ADR-009).

- **Three-list schema:** every compiled sentinel block now carries `directives:` (auto-generated), `manual_directives:` (user-authored), and `suppressed_directives:` (user-rejected). The merge formula `effective = (directives - suppressed) ∪ manual` gives users full control over what ships without losing compile automation. Hash-based caching (`source_hash` + `directives_hash`) skips Claude calls when nothing changed.
- **Invariant Records (ADR-009):** formalized the governance artifact for non-negotiable constraints. Formalized "Invariant Records" as the governance artifact for hard constraints (short form: INV). Template follows Statement/Rationale/Enforcement structure. Compile extracts directives from the Statement section, preserving declarative absolute language.
- **Extensibility plumbing:** template lookup chain (`project .edikt/templates/` → inline fallback), `/edikt:guideline:compile` command, auto-chain (`<artifact>:new` runs `<artifact>:compile`).
- **Init style detection:** detects project style (flat, layered, monorepo) during init. Adapt mode for existing `.edikt/` directories. Template-less refusal for v0.3.0+ projects.
- **Flexible prose input:** ADR/invariant/guideline creation accepts natural language with automatic reference extraction to existing governance.
- **Doctor + upgrade integration:** doctor reports template overrides and stale hashes. Upgrade respects project templates.

### Compile Improvements

Experiment-driven improvements to the compile output format. These changes improve how well Claude follows governance directives.

- **"No exceptions." reinforcement:** invariant directives derived from absolute-language Statements ("every", "all", "total") now get "No exceptions." appended. Experiments showed this phrase prevents Claude from rationalizing edge cases.
- **Reminders sentinel (`[edikt:reminders:start/end]`):** compile now generates pre-action interrupts: "Before writing SQL → MUST include tenant_id." Aggregated into a `## Reminders` section in governance.md. Capped at 10.
- **Verification checklist:** compile generates a `## Verification Checklist` section with grep-verifiable items Claude checks before finishing. Capped at 15 items.
- **Per-directive LLM compliance scoring** in `/edikt:invariant:review`, `/edikt:adr:review`, `/edikt:guideline:review`: scores each compiled directive on token specificity, MUST/NEVER usage, grep-ability, ambiguity, and friction risk. Manual directives held to the same standard.
- **New `/edikt:gov:score` command:** aggregate governance quality report — context budget, compliance metrics, manual directive health. JSON output for CI.

### Pre-flight Criteria Validation

The evaluator agent now supports a **pre-flight mode** that validates acceptance criteria BEFORE the generator starts. Classifies each criterion as TESTABLE/VAGUE/SUBJECTIVE/BLOCKED and proposes verification commands. The plan command (step 11) invokes pre-flight automatically, preventing wasted iterations on untestable criteria.

### Experiments

Pre-registered experiments measuring whether governance directives change Claude's output on real coding tasks. 8 experiments across 4 scenario types.

| Scenario | Baseline | Governance | Effect |
|---|---|---|---|
| Existing codebase (EXP 01-04) | PASS | PASS | Absent — code patterns self-teach |
| Greenfield (EXP 05-06) | VIOLATION | PASS | **Present** — governance prevents architecture/tenant violations |
| New domain on existing (EXP 07) | VIOLATION | PASS | **Present** — governance catches log/SQL misses |
| Long context (EXP 08, N=2) | 1/2 VIOLATION | 0/2 PASS | **Present** — governance stabilizes under context pressure |

Key findings: governance has measurable effect on greenfield and new-domain code. Directive format matters — MUST/NEVER with literal code tokens outperforms prose. Long context degrades convention compliance; governance in `.claude/rules/` survives because it's loaded separately from the conversation. Full methodology and results in `test/experiments/`.

### New commands

- `/edikt:gov:score` — aggregate governance quality scoring for CI

### Architecture Decisions

- **ADR-008:** Deterministic compile and three-list schema
- **ADR-009:** Invariant Record template formalization

## v0.2.3 (2026-04-09)

### Compile schema version (ADR-007)

`/edikt:gov:compile` now stamps generated governance files with a **compile schema version** — a small integer independent of edikt's marketing version — instead of the edikt version at compile time.

**Problem this fixes:** before v0.2.3, `.claude/rules/governance.md` was stamped with `version: "<edikt-version>"`, conflating two unrelated cadences. Every edikt point release (even pure bug fixes) implied governance was stale and needed regeneration, but the compile output format hadn't actually changed. In the dogfood repo, we kept hand-editing `governance.md`'s version via `sed` on each release to keep a test green — the file ended up lying about its own provenance (version said v0.2.2 but the compile timestamp was frozen at March 25).

**New format** (see [ADR-007](docs/architecture/decisions/ADR-007-compile-schema-version.md)):

```yaml
---
paths: "**/*"
compile_schema_version: 2
---
<!-- edikt:compiled — generated by /edikt:gov:compile, do not edit manually -->
<!-- compiled_by: edikt v0.2.3 -->
<!-- compiled_at: 2026-04-09T10:30:00Z -->
```

Three fields, three purposes:

- **`compile_schema_version`** (YAML, enforced) — identifies the output format contract. `1` = v0.1.x flat governance, `2` = v0.2.x topic-grouped rule files. `/edikt:doctor` checks it against the constant declared in `commands/gov/compile.md` and recommends `/edikt:gov:compile` only when the format has actually changed.
- **`compiled_by`** (HTML comment, informational) — which edikt version ran compile. Diagnostic only, never enforced.
- **`compiled_at`** (HTML comment, informational) — ISO8601 timestamp. Diagnostic only, never enforced.

**Consequences:**
- No more false-positive staleness warnings on point releases. Users only see "regenerate governance" when the compile schema actually changed.
- Point releases can ship bug fixes without implying anything about compile output compatibility.
- `/edikt:doctor` gets smarter about stale governance detection.
- `/edikt:upgrade` has a new step that checks the project's schema version against the installed compile schema and recommends (but does not auto-run) regeneration when they diverge.
- Dogfooding stops hand-editing `governance.md`'s version field. The dogfood file now uses the new format honestly.

### Installer UX fixes

Three bug reports from real installs, all fixed in the same release.

- **No prompt on `curl | bash`.** The interactive "global vs project" prompt was skipped silently when stdin was a pipe (the common `curl -fsSL ... | bash` invocation). Now the installer reads from `/dev/tty` when available, so the prompt fires even when stdin is consumed by the curl pipe. Falls back to `--global` only when there's no TTY at all (CI, fully redirected).
- **Commands duplicated across global and project locations.** When a user installed globally in a directory that already had a project-local edikt install (either from a prior `--project` run, or from the dogfood repo itself), Claude Code ended up loading commands from both `~/.claude/commands/edikt/` and `.claude/commands/edikt/`, producing duplicates in the skill list. The installer now detects this condition at startup and emits a yellow warning pointing at the exact paths and the `rm -rf` to clean them up. Never auto-deletes.
- **No detection of existing install before project install.** If a user ran `install.sh --project` in a directory where `~/.edikt/VERSION` already existed, the two installs would silently overlap. Same detection now fires a warning for this case too. Both detection paths share the same `HAS_GLOBAL` / `HAS_PROJECT` flags.
- **New test scenarios in `test/test-install-e2e.sh`** — scenarios 6 and 7 cover the duplication-warning paths (6 = global install with leftover project files; 7 = project install with existing global install). Total scenarios now: 7. Total assertions: 28.

### Tests

- **New `test/test-v023-regressions.sh`** (21 assertions) — verifies ADR-007 exists, `COMPILE_SCHEMA_VERSION` is declared in compile.md, output templates emit the new format, doctor.md checks the schema version, upgrade.md documents the migration step, and the dogfood governance file matches the constant.
- **`test-e2e.sh` version check refactored** — no longer enforces `GOV_VER == FILE_VER`. Instead it validates that `compile_schema_version` in the dogfood governance file matches the `COMPILE_SCHEMA_VERSION` constant in `commands/gov/compile.md`.

## v0.2.2 (2026-04-08)

Critical bug-fix release. The v0.2.1 installer was silently broken on the v0.1.x → v0.2.x upgrade path.

### Installer: upgrade from v0.1.x was silently broken

- **`((BACKUP_COUNT++))` under `set -euo pipefail` killed the installer on the first backup.** Postfix `++` returns the pre-increment value (`0` on the first call), which bash's `set -e` treats as a failure and exits the script. Symptoms: the cleanup loop removed *nothing*, the new namespaced commands were *never* installed, old flat files stayed in place, and the installer exited without any error message. This shipped in v0.2.1 and affected everyone upgrading from v0.1.x via `curl | bash`. Fixed by using `BACKUP_COUNT=$((BACKUP_COUNT + 1))`.
- **New integration test** (`test/test-install-e2e.sh`) — 22 assertions across five scenarios: fresh install, upgrade from v0.1.x, user-customized file preservation, network failure abort, and repeated-install idempotency. Shims `curl` with a mock that serves files from the local repo, so the full `install.sh` runs end-to-end against a fake `$HOME` in `/tmp`. This is the test we wished existed before v0.2.0 shipped — it caught the v0.2.1 regression immediately.

### `/edikt:upgrade`: migrate v0.1.x command references

- **New step 5 in `/edikt:upgrade`: rewrite old flat command references in project files to their new namespaced equivalents.** Projects initialized with v0.1.x have hardcoded references to `/edikt:adr`, `/edikt:plan`, `/edikt:compile`, etc. in their `CLAUDE.md` managed block and in compiled rule packs. Previously, `/edikt:upgrade` only migrated the *sentinel format* (HTML → visible) and left the *content* inside the sentinels untouched. Now upgrade runs a targeted string-replace across all 15 moved commands, scoped to edikt-owned content only (the CLAUDE.md managed block and rule pack files marked with `edikt:generated` or `edikt:compiled`). User content outside the managed blocks is never touched.
- **Idempotent and safe:** the instruction tells Claude to match only occurrences NOT already followed by `:`, using surrounding context (backticks, punctuation, end-of-line) for disambiguation. Running upgrade twice is a no-op.

## v0.2.1 (2026-04-08)

Bug-fix release following v0.2.0 field reports.

### Installer upgrade path

- **Old flat commands no longer linger after upgrade.** v0.1.x installed commands like `~/.claude/commands/edikt/adr.md`, `plan.md`, `compile.md` at the top level. v0.2.0 moved them into namespaces but the installer never removed the old files, so users saw both `/edikt:adr` (stale) and `/edikt:adr:new` (new) in their command list. The installer now deletes the 15 moved v0.1.x commands before installing new files, with backup. User-customized files (marked with `<!-- edikt:custom -->`) are preserved.
- **Silent curl failures now abort the install.** Every `curl -o` call now goes through a `_fetch` helper that enforces `--retry 2`, `--max-time 30`, non-empty download verification, and exits with an error on failure. Previously a network blip during `curl | bash` could leave files partially updated without any warning.

### `/edikt:init` ADR path adoption

- **init now configures `paths.decisions` to match detected ADR locations.** Previously, init detected existing ADRs in folders like `docs/decisions/` and offered to import them, but the import flow hardcoded the destination to edikt's default (`docs/architecture/decisions/`) and never wrote the detected path into `.edikt/config.yaml`. Users ended up with ADRs in one place and edikt looking for them somewhere else — `/edikt:gov:compile` and `/edikt:status` reported zero ADRs.
- New prompt: **[1] Adopt** (keep ADRs where they are, configure edikt to use that path), **[2] Migrate** (move to edikt's default layout), **[3] Skip**. Same flow for invariants.

### Command documentation cleanup

- **Seniority prefixes removed from `/edikt:sdlc:review` reviewer lenses.** The command documentation still labeled agents as `Principal DBA`, `Staff SRE`, `Staff Security`, `Senior API`, `Principal Architect`, `Senior Performance` — inconsistent with the agent templates which dropped seniority prefixes in v0.2.0. Now just `DBA`, `SRE`, `Security`, `API`, `Architect`, `Performance`.

### Website content

- **Fixed 10 dead links in `website/governance/chain.md`, `website/governance/compile.md`, `website/governance/drift.md`, and `website/commands/brainstorm.md`** — they referenced old flat command paths (`/commands/prd`, `/commands/spec`, `/commands/plan`, etc.) that broke the v0.2.0 VitePress deploy. Now use namespaced paths (`/commands/sdlc/prd`, `/commands/gov/compile`, etc.).

### Test coverage

- New `test/test-v021-regressions.sh` — 36 assertions guarding against all five v0.2.1 bugs so they can't silently return.

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
- Reproducible: `test/experiments/rule-compliance/exp-001-scenarios/` and `test/experiments/rule-compliance/exp-002-scenarios/`

**Website**
- Full documentation at edikt.dev
- Guides: solo engineer, teams, multi-project, greenfield, brownfield, monorepo, security, daily workflow
- Governance section: chain, gates, compile, drift, review-governance

**Zero dependencies**
- Every file is `.md` or `.yaml` — no build step, no runtime, no daemon
- `curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash`
- Claude Code only — uses platform primitives (path-conditional rules, lifecycle hooks, slash commands, specialist agents, quality gates)
