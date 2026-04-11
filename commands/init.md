---
name: edikt:init
description: "Intelligent onboarding — detect project, infer architecture, install guardrails"
effort: normal
allowed-tools:
  - Read
  - Write
  - Edit
  - Bash
  - Glob
  - Grep
  - Agent
---

# edikt:init

Set up edikt governance for a project. Detects what exists, confirms with the user, and generates everything.

CRITICAL: This command requires interactive input. If you are in plan mode (you can only describe actions, not perform them), output this and stop:
```
⚠️  This command requires user interaction and cannot run in plan mode.
Exit plan mode first, then run the command again.
```

CRITICAL: NEVER guess or invent project details. If something is unclear, ask. If an artifact detection is uncertain, skip it and tell the user how to import it manually.

## Instructions

### 1. Check for Existing Setup

```bash
ls -la .edikt/ 2>/dev/null || echo "No .edikt/ directory"
```

**If `.edikt/config.yaml` exists**, determine the scenario:

- **Member onboarding** (default — user is joining an existing project):

  **Step 0 — edikt version gate (blocking):**
  ```bash
  INSTALLED=$(cat ~/.edikt/VERSION 2>/dev/null | tr -d '[:space:]' || echo "unknown")
  PROJECT=$(grep '^edikt_version:' .edikt/config.yaml | awk '{print $2}' | tr -d '"')
  ```
  - If `INSTALLED` is a valid version AND `INSTALLED` < `PROJECT`:
    ```
    ❌ edikt version mismatch — your install is older than this project requires.
       Installed: {INSTALLED}
       Required:  {PROJECT}

    Run: curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
    Then re-run /edikt:init.
    ```
    Stop here.
  - If `INSTALLED` == "unknown": warn but don't block:
    ```
    ⚠️  Could not detect installed edikt version (~/.edikt/VERSION missing).
       Proceeding with checks — results may be unreliable.
    ```

  **Steps 1–6 — Member environment checks:**

  Run these checks and collect results:

  1. **Git identity:**
     ```bash
     git config user.name && git config user.email
     ```
     - ✅ if both set — show name and email
     - ⚠️ if missing — "Set with: git config --global user.name/user.email"

  2. **Claude Code:**
     ```bash
     claude --version 2>/dev/null
     ```
     - ✅ if found — show version
     - ❌ if missing — "Install Claude Code: https://claude.ai/download"

  3. **edikt config:**
     - ✅ (already confirmed by reaching this branch)

  4. **MCP environment variables:**
     Read `.mcp.json` if it exists. For each server entry, check the `env` field for required environment variables. Check each is set:
     - ✅ `{VAR_NAME}` set
     - ⚠️ `{VAR_NAME}` not set — show where to get the value if known (e.g., GitHub tokens page)
     If `.mcp.json` doesn't exist: skip silently.

  5. **Environment hardening:**
     ```bash
     echo "$CLAUDE_CODE_SUBPROCESS_ENV_SCRUB"
     ```
     - ✅ if set to `1`
     - ⚠️ if not set — "Add to shell profile: export CLAUDE_CODE_SUBPROCESS_ENV_SCRUB=1"

  6. **Git pre-push hook:**
     ```bash
     ls .git/hooks/pre-push 2>/dev/null
     ```
     - ✅ if exists and executable
     - ⚠️ if missing — offer to install from `~/.edikt/templates/hooks/pre-push`
     If `hooks: { pre-push-security: false }` in config: show as "✅ disabled by config"

  **Step 7 — Managed settings detection:**
  ```bash
  ls ~/.claude/managed-settings.json ~/.claude/managed-settings.d/*.json 2>/dev/null
  ```
  If found:
  ```
  Organization policies detected:
    managed-settings.json — enforced by your organization
  ```
  If not found: skip silently.

  **Step 8 — Governance file gap sync:**
  Check for missing governance files (preserve existing behavior):
  - Rule packs listed in config but missing from `.claude/rules/`
  - Hooks configured but not in `.claude/settings.json`
  - Agents listed but missing from `.claude/agents/`
  If gaps exist: offer to sync (fills gaps only — never overwrites existing files).

  **Step 9 — Show shared config:**
  ```bash
  git ls-files .edikt/config.yaml .claude/rules/ .claude/agents/ .mcp.json .github/ 2>/dev/null
  ```
  ```
  Shared config (committed to git):
    .edikt/config.yaml     — project config
    .claude/rules/         — {n} rule packs
    .claude/agents/        — {n} specialist agents
    .mcp.json              — {servers} (keys not committed)
  ```

  **Step 10 — Report:**

  If failures or warnings exist:
  ```
  edikt Member Setup — {repo name}

    {all check results with ✅/⚠️/❌}

  {n} items need attention. Fix them, then re-run /edikt:init.
  ```

  If all green:
  ```
  edikt Member Setup — {repo name}

    {all check results with ✅}

  ✅ All checks passed — you're ready to go!

  Run /edikt:context to load project context.
  ```

  Note: if config contains a `team:` block (legacy from `/edikt:team`), ignore it silently. Do not read, display, or act on it.

- **Reconfigure** (user explicitly wants to change settings): Before any file generation, scan existing `.claude/rules/*.md` for customization:

  **Step 1 — Detect customizations:** For each `.claude/rules/*.md` file:
  - Check for `<!-- edikt:generated -->` tag
  - If tag is present: compute content hash and compare against the template's content hash. If hashes differ, the file was edited but the marker wasn't removed.
  - If tag is absent: file is explicitly customized (user owns it)

  **Step 2 — Show change summary with per-file options:**
  ```
  Reconfiguring edikt.

  Will create:  2 new rule packs (testing.md, api.md)
  Will update:  2 rule packs (from templates, unchanged since install)
  Will preserve: CLAUDE.md (sentinel merge), settings.json (hook merge)

  ⚠ These files have edikt:generated marker but content differs from template:
    .claude/rules/go.md — edited since install

    Per file:
    [1] Overwrite — replace with latest edikt template
    [2] Keep mine — remove the edikt:generated marker so edikt never overwrites this file again
    [3] Show diff — see what changed before deciding

  These files were manually customized (no edikt:generated marker):
    .claude/rules/security.md — will NOT be touched
  ```

  **If user picks [2] (Keep mine):** Remove the `<!-- edikt:generated -->` marker from the file and confirm:
  ```
  ✅ .claude/rules/go.md is now yours.

    edikt will never overwrite this file again.
    You'll still get update notifications via /edikt:rules-update — but updates
    are manual (you review the diff and pick what to merge).

    To go back to edikt-managed: add <!-- edikt:generated --> to the file.
  ```

  Never overwrite files that lack the `<!-- edikt:generated -->` tag. Never silently overwrite files whose content differs from the template — always ask.

### 2. Scan the Project

Show a step indicator:
```
[1/3] Scanning project...
```

Run these scans in parallel:

**Code:**
```bash
ls go.mod package.json composer.json Gemfile pyproject.toml requirements.txt Cargo.toml 2>/dev/null
find . -type f -not -path './.git/*' -not -path './node_modules/*' -not -path './vendor/*' | wc -l
git log --oneline 2>/dev/null | wc -l
```

**Build/Test/Lint:**
```bash
ls Makefile package.json Taskfile.yml justfile 2>/dev/null
ls .golangci-lint.yaml .golangci.yaml .eslintrc* eslint.config.* ruff.toml .rubocop.yml biome.json .prettierrc* 2>/dev/null
ls .github/workflows/*.yml .gitlab-ci.yml 2>/dev/null
```

**AI Config:**
```bash
ls CLAUDE.md .cursorrules .github/copilot-instructions.md .windsurfrules 2>/dev/null
ls .claude/rules/*.md 2>/dev/null
```

**Existing docs:**
```bash
find . -maxdepth 3 -name "ADR-*" -o -name "adr-*" -o -name "*decision*" 2>/dev/null | grep -v .git | grep '\.md$'
ls docs/adr/ docs/decisions/ docs/architecture/decisions/ 2>/dev/null
find . -maxdepth 3 -name "SPEC*" -o -name "spec*" -o -name "PRD*" -o -name "prd*" -o -name "design*.md" 2>/dev/null | grep -v .git | grep '\.md$'
ls docs/project-context.md docs/about.md docs/overview.md 2>/dev/null
ls .env.example .env.sample 2>/dev/null
```

**Archway detection:**
```bash
ls verikt.yaml .archway/ 2>/dev/null
```

**Commit convention detection:**
```bash
git log --oneline -20 2>/dev/null
```

**Database detection:**

Run these checks in order. Collect all signals before deciding type and tool.

```bash
# --- Definitive config/schema signals (high confidence) ---
[ -f prisma/schema.prisma ]            && echo "DB_SIGNAL: sql prisma definitive"
[ -f alembic.ini ]                     && echo "DB_SIGNAL: sql alembic definitive"
[ -f flyway.conf ]                     && echo "DB_SIGNAL: sql flyway definitive"
[ -f liquibase.properties ]            && echo "DB_SIGNAL: sql liquibase definitive"
[ -f changelog.xml ]                   && echo "DB_SIGNAL: sql liquibase definitive"

# Django: manage.py + at least one migrations directory
if [ -f manage.py ]; then
  find . -not -path './.git/*' -not -path './node_modules/*' \
    -path '*/migrations/*.py' 2>/dev/null | head -1 | grep -q . \
    && echo "DB_SIGNAL: sql django definitive"
fi

# Flyway migration directory (definitive if found without flyway.conf)
find . -not -path './.git/*' -path '*/db/migration/*.sql' 2>/dev/null | head -1 | grep -q . \
  && echo "DB_SIGNAL: sql flyway definitive"

# --- go.mod dependency signals (inferred) ---
if [ -f go.mod ]; then
  grep -qF 'lib/pq'             go.mod && echo "DB_SIGNAL: sql - inferred lib/pq"
  grep -qF 'jackc/pgx'          go.mod && echo "DB_SIGNAL: sql - inferred jackc/pgx"
  grep -qF 'go-sql-driver/mysql' go.mod && echo "DB_SIGNAL: sql - inferred go-sql-driver/mysql"
  grep -qF 'mongo-driver'        go.mod && echo "DB_SIGNAL: document - inferred mongo-driver"
  if grep -qE 'aws-sdk-go(-v2)?' go.mod; then
    grep -rqF 'dynamodb' --include='*.go' . 2>/dev/null && echo "DB_SIGNAL: document - inferred aws-sdk-go+dynamodb"
  fi
  grep -qF 'go-migrate'          go.mod && echo "DB_SIGNAL: sql golang-migrate inferred go-migrate"
  grep -qF 'golang-migrate'      go.mod && echo "DB_SIGNAL: sql golang-migrate inferred golang-migrate"
  grep -qF 'go-redis'            go.mod && echo "DB_SIGNAL: key-value - inferred go-redis"
fi

# --- package.json dependency signals (inferred) ---
if [ -f package.json ]; then
  grep -qF '"prisma"'                package.json && echo "DB_SIGNAL: sql prisma inferred prisma"
  grep -qF '"@prisma/client"'        package.json && echo "DB_SIGNAL: sql prisma inferred @prisma/client"
  grep -qF '"mongoose"'              package.json && echo "DB_SIGNAL: document - inferred mongoose"
  grep -qF '"@aws-sdk/client-dynamodb"' package.json && echo "DB_SIGNAL: document - inferred @aws-sdk/client-dynamodb"
  grep -qF '"drizzle-orm"'           package.json && echo "DB_SIGNAL: sql drizzle inferred drizzle-orm"
  grep -qF '"knex"'                  package.json && echo "DB_SIGNAL: sql knex inferred knex"
  grep -qF '"typeorm"'               package.json && echo "DB_SIGNAL: sql - inferred typeorm"
  grep -qF '"ioredis"'               package.json && echo "DB_SIGNAL: key-value - inferred ioredis"
  grep -qF '"redis"'                 package.json && echo "DB_SIGNAL: key-value - inferred redis"
fi

# --- Python dependency signals (inferred) ---
for pyfile in requirements.txt pyproject.toml; do
  if [ -f "$pyfile" ]; then
    grep -qiF 'sqlalchemy' "$pyfile" && echo "DB_SIGNAL: sql - inferred sqlalchemy ($pyfile)"
    grep -qiF 'pymongo'    "$pyfile" && echo "DB_SIGNAL: document - inferred pymongo ($pyfile)"
    grep -qiF 'django'     "$pyfile" && echo "DB_SIGNAL: sql django inferred django ($pyfile)"
  fi
done

# --- Ruby signals (inferred) ---
if [ -f Gemfile ]; then
  grep -qF 'pg'      Gemfile && echo "DB_SIGNAL: sql rails inferred pg"
  grep -qF 'mysql2'  Gemfile && echo "DB_SIGNAL: sql rails inferred mysql2"
  grep -qF 'mongoid' Gemfile && echo "DB_SIGNAL: document - inferred mongoid"
fi

# --- C# signals (inferred) ---
find . -not -path './.git/*' -name '*.csproj' 2>/dev/null | while read f; do
  grep -qF 'EntityFramework' "$f" && echo "DB_SIGNAL: sql ef-core inferred EntityFramework ($f)"
  grep -qF 'Npgsql'          "$f" && echo "DB_SIGNAL: sql ef-core inferred Npgsql ($f)"
done

# --- Elixir signals (inferred) ---
if [ -f mix.exs ]; then
  grep -qF 'ecto_sql' mix.exs && echo "DB_SIGNAL: sql ecto inferred ecto_sql"
fi

# --- Rust signals (inferred) ---
if [ -f Cargo.toml ]; then
  grep -qF 'diesel' Cargo.toml && echo "DB_SIGNAL: sql diesel inferred diesel"
  grep -qF 'sqlx'   Cargo.toml && echo "DB_SIGNAL: sql raw-sql inferred sqlx"
fi
```

After collecting all `DB_SIGNAL` lines, apply these rules to determine `detected_db_type` and `detected_db_tool`:

1. Collect unique DB types from all signals (`sql`, `document`, `key-value`).
2. If only one type → `detected_db_type` = that type.
3. If more than one type → `detected_db_type` = `mixed`.
4. If no signals → `detected_db_type` = `none` (triggers greenfield questions below).
5. For tool: if any definitive signal carries a tool name, use it. If only inferred signals carry a tool, use it but mark as inferred. If no tool in any signal → `detected_db_tool` = none.
6. If type is `document` or `key-value` (and not `mixed`), `detected_db_tool` = none regardless.

Show the user what was found before writing config. Use this format:

For a definitive detection:
```
Database detected:
  Type:  sql (from prisma/schema.prisma)
  Tool:  prisma (definitive)
```

For an inferred detection:
```
Database detected:
  Type:  document (inferred from mongoose in package.json)
  Tool:  not detected
```

For mixed:
```
Database detected:
  Type:  mixed (sql from lib/pq, key-value from go-redis in go.mod)
  Tool:  golang-migrate (inferred from golang-migrate in go.mod)
```

For nothing detected — do NOT show this block yet. Instead, ask the greenfield questions during Step 3 (Configure), integrated into the interview after stack info is confirmed:

```
Database setup (nothing detected from code):
  What database type will you use?
  1. SQL (Postgres, MySQL, SQLite, etc.)
  2. Document (MongoDB, DynamoDB, Firestore, etc.)
  3. Key-Value (Redis, DynamoDB as KV, etc.)
  4. Mixed (multiple types)
  5. Not decided yet
```

If the user selects 1 (SQL) or 4 (Mixed), follow up with:
```
  Migration tool? (press enter to skip)
  golang-migrate | flyway | alembic | django | rails | prisma |
  liquibase | drizzle | knex | ecto | diesel | ef-core | raw-sql
```

If the user selects 2, 3, or 5 — skip migration tool question entirely. Selection 5 → `default_type: auto`.

Present findings — same format for both established and greenfield:

**Established project:**
```
[1/3] Scanning project...

  Code:       Go project, 142 files
              Chi framework, PostgreSQL
  Build:      make build
  Test:       make test
  Lint:       golangci-lint (.golangci-lint.yaml)
  AI config:  CLAUDE.md (34 lines)
  Docs:       3 ADRs in docs/decisions/
  Commits:    conventional commits detected (feat/fix/chore)
```

**If existing ADRs or decision docs were detected**, capture the detected folder path (e.g. `docs/decisions/`) as `$DETECTED_DECISIONS_PATH`. Do the same for invariants if found: `$DETECTED_INVARIANTS_PATH`.

If the detected path differs from edikt's default (`docs/architecture/decisions/` / `docs/architecture/invariants/`), you MUST prompt:
```
Found 3 existing architecture decisions in docs/decisions/.

How should edikt handle them?
  [1] Adopt   — keep them at docs/decisions/ and configure edikt to use that path (default)
  [2] Migrate — move them to docs/architecture/decisions/ (edikt's default layout)
  [3] Skip    — ignore them, don't import
Choice [1]:
```

**If the user chooses [1] Adopt (or accepts the default):**
- Write `paths.decisions: docs/decisions` (the detected path, without trailing slash) to the generated `.edikt/config.yaml` — do NOT use the edikt default
- Same for invariants if detected in a non-default location
- Do NOT move or copy files — they stay where they are
- After: "Configured edikt to use docs/decisions/ for ADRs. Run `/edikt:gov:compile` to compile them into governance directives."

**If the user chooses [2] Migrate:**
- Read each existing ADR file
- Move to `docs/architecture/decisions/` (edikt's default)
- If the file doesn't follow ADR format (missing status, missing Decision section), convert it: extract the decision, add `status: accepted` frontmatter, preserve the original content
- Write the DEFAULT `paths.decisions: docs/architecture/decisions` to config
- After: "Migrated 3 ADRs to docs/architecture/decisions/. Run `/edikt:gov:compile` to compile them into governance directives."

**If the user chooses [3] Skip** (or the detected path is already the default and no prompt was shown):
- Continue with edikt's default paths
- Remind them they can import later with `/edikt:docs:intake`

**Critical:** whichever choice is made, the generated `.edikt/config.yaml` MUST reflect the actual location of the ADRs. Never leave the default path in config when ADRs live elsewhere. Otherwise `/edikt:gov:compile` and `/edikt:status` will report zero ADRs despite them existing.

### 2a.2 Detected guidelines — Adopt / Migrate / Skip

**If existing guidelines or convention docs were detected** (e.g., files with naming patterns, coding standards, style guides, best practices — often in `docs/`, `guides/`, `conventions/`, or `standards/`), capture the detected path as `$DETECTED_GUIDELINES_PATH`.

If the detected path differs from edikt's default (`docs/guidelines/`), prompt:
```
Found {n} existing guidelines/conventions in {path}.

How should edikt handle them?
  [1] Adopt   — keep them at {path} and configure edikt to use that path (default)
  [2] Migrate — move them to docs/guidelines/ (edikt's default layout)
  [3] Skip    — ignore them, don't import
Choice [1]:
```

**If the user chooses [1] Adopt:**
- Write `paths.guidelines: {detected_path}` to `.edikt/config.yaml`
- Do NOT move or copy files
- After: "Configured edikt to use {path} for guidelines. Run `/edikt:guideline:compile` to compile them into governance directives."

**If the user chooses [2] Migrate:**
- Read each guideline file
- Move to `docs/guidelines/`
- If the file doesn't follow guideline format (missing `## Rules` section), convert it: extract rules as MUST/NEVER bullets, add a `## Purpose` section from existing content
- Write `paths.guidelines: docs/guidelines` to config
- After: "Migrated {n} guidelines to docs/guidelines/. Run `/edikt:guideline:compile` to compile them into governance directives."

**If the user chooses [3] Skip:**
- Continue with edikt's default paths
- Remind them they can import later with `/edikt:docs:intake`

**After adoption or migration**, prompt to compile:
```
Would you like to compile these guidelines into governance directives now?
  [1] Yes — run /edikt:guideline:compile (recommended)
  [2] No  — I'll compile later
Choice [1]:
```

If yes, run `/edikt:guideline:compile` to generate sentinel blocks. Then remind: "Run `/edikt:gov:compile` to include these in the governance index."

**Critical:** the generated `.edikt/config.yaml` MUST reflect the actual location of guidelines. Same rule as ADRs — never leave the default path when guidelines live elsewhere.

### 2b. Project Templates (v0.3.0 Adapt mode)

This step runs once per artifact type (ADRs, Invariant Records, guidelines) and generates `.edikt/templates/<artifact>.md` files that future `/edikt:adr:new`, `/edikt:invariant:new`, and `/edikt:guideline:new` commands will use via the lookup chain documented in each `new.md` file.

Per [ADR-005](../../docs/architecture/decisions/ADR-005-extensibility-model.md), edikt does NOT ship default templates that auto-install. Per [ADR-009](../../docs/architecture/decisions/ADR-009-invariant-record-terminology.md), invariants are called "Invariant Records" (short form `INV`). Each project explicitly chooses its template during init (or writes its own), and the choice is committed to git as `.edikt/templates/<artifact>.md`.

**Re-run protection:** for each artifact type, check if `.edikt/templates/<artifact>.md` already exists. If it does, skip the template step for that artifact type (respecting any prior choice the team made). The `--reset-templates` flag on `/edikt:init` forces regeneration even if templates exist.

**For each artifact type** (ADRs, Invariant Records, guidelines), run the appropriate sub-flow below.

#### 2b.1 ADR template

**If `.edikt/templates/adr.md` already exists** (and `--reset-templates` was NOT passed) — skip this sub-step and print:
```
ADR template: using existing .edikt/templates/adr.md
```

**If existing ADRs were detected in step 2** — run the Adapt flow:

1. **Sample** 2-3 ADRs from the detected decisions path (first 3, or all if fewer than 3).
2. **Analyze structural pattern**:
   - Does the file have YAML frontmatter? Which fields (status, date, decision-makers, supersedes, etc.)?
   - What heading levels are used for the title (`#` or `##`) and for sections (`##` or `###`)?
   - What are the section names and their order? (e.g., "Context", "Decision", "Consequences" vs "Context and Problem Statement", "Decision Drivers", "Considered Options", "Decision Outcome")
   - Are sections prose or bulleted lists?
   - Is there a trailing "References" or "More Information" section?
3. **Check for consistency** across the sampled ADRs. If they all share the same structure, proceed with that structure. If they differ (e.g., 3 MADR-style and 4 Nygard-style), ask:
   ```
   Your existing ADRs have inconsistent styles:
     {n} follow MADR-style structure
     {m} follow Nygard-minimal structure

   Do you already have a team template file we should use?

     [1] Yes — point me at the file (I'll read it and use it as your project template)
     [2] No — draft a template from the majority style ({majority}) for your review
     [3] No — let me pick from the reference templates instead
   Choice [2]:
   ```
   - Option [1]: ask for the file path, read it, write it to `.edikt/templates/adr.md` verbatim (ensuring the directives sentinel block is present; add an empty block if missing).
   - Option [2]: use the majority style for the Adapt inference below.
   - Option [3]: jump to the "Start fresh" branch below.
4. **Present the three-choice prompt**:
   ```
   Found {n} existing ADRs in {detected_decisions_path} with a consistent {style_description} style:
     - Frontmatter: {yes|no} ({fields if yes})
     - Sections: {list of sections in order}
     - Prose style: {narrative | bulleted | mixed}

   How should edikt handle this style?
     [1] Adapt       — generate a project template matching your existing style (recommended)
     [2] Start fresh — ignore existing style, pick from reference templates
     [3] Write my own — skip template generation; I'll create .edikt/templates/adr.md manually

   Choice [1]:
   ```
5. **If user picks [1] Adapt**:
   - Generate `.edikt/templates/adr.md` with:
     - The detected frontmatter block (with placeholder values like `{date}`, `{status}`)
     - The detected section headings, in the same order
     - Placeholder prose in each section ("{Describe the context here}", "{State the decision}", etc.)
     - A writing guidance comment at the top noting this template was generated by Adapt mode from N existing ADRs
     - The mandatory `[edikt:directives:start]: #` / `[edikt:directives:end]: #` sentinel block at the end (empty directives list — `<artifact>:compile` populates it)
   - Write to `.edikt/templates/adr.md`.
   - Report: `Generated .edikt/templates/adr.md from detected style (based on {n} ADRs)`.
6. **If user picks [2] Start fresh**:
   - Present the reference template choice:
     ```
     Pick a reference ADR template:
       [1] Nygard-minimal — Title, Status, Context, Decision, Consequences (5 sections, short)
       [2] MADR-extended  — adds Decision Drivers, Considered Options, Pros/Cons, Confirmation

     Choice [1]:
     ```
   - Copy the chosen reference file to `.edikt/templates/adr.md` verbatim:
     - For [1] Nygard-minimal: copy `~/.edikt/templates/examples/adr-nygard-minimal.md`
     - For [2] MADR-extended: copy `~/.edikt/templates/examples/adr-madr-extended.md`
   - Report: `Copied reference template ({choice}) to .edikt/templates/adr.md`.
7. **If user picks [3] Write my own**:
   - Do not generate `.edikt/templates/adr.md`.
   - Warn:
     ```
     Skipping ADR template generation. Create .edikt/templates/adr.md manually before running /edikt:adr:new, or re-run /edikt:init to generate one.
     ```

**If no existing ADRs were detected** (greenfield) — skip the Adapt flow. Go directly to the "Start fresh" branch above, showing only the reference template choice and the "Write my own" option. There is no "Adapt" option when there's nothing to adapt from.

#### 2b.2 Invariant Record template

Per [ADR-009](../../docs/architecture/decisions/ADR-009-invariant-record-terminology.md), invariants in edikt are formally called **Invariant Records** (short form `INV`).

**If `.edikt/templates/invariant.md` already exists** (and `--reset-templates` was NOT passed) — skip and print:
```
Invariant Record template: using existing .edikt/templates/invariant.md
```

**If existing invariants were detected in step 2** — run the Adapt flow (same pattern as ADR, adapted for invariant-specific sections):

1. Sample 2-3 invariants from the detected invariants path.
2. Analyze structural pattern — frontmatter fields, heading levels, section names (Statement vs Rule, presence of Rationale/Consequences of violation/Implementation/Anti-patterns/Enforcement).
3. Check for consistency; if inconsistent, prompt the user same as ADR (team template / draft from majority / pick reference).
4. Present the three-choice prompt:
   ```
   Found {n} existing invariants in {detected_invariants_path} with a consistent {style_description} structure:
     - Sections: {list}
     - Status lifecycle: {Active/Retired | Active/Proposed/Superseded/Retired | custom}

   How should edikt handle this style?
     [1] Adapt       — generate a project Invariant Record template matching your existing style (recommended)
     [2] Start fresh — ignore existing style, pick from reference templates
     [3] Write my own — skip template generation; I'll create .edikt/templates/invariant.md manually

   Choice [1]:
   ```
5. Adapt mode: generate `.edikt/templates/invariant.md` from detected style. **Always include the writing guidance comment from ADR-009** explaining constraint-vs-implementation, present-tense declarative phrasing, and the "not derived from ADRs" principle — even if the detected style doesn't have it. This is edikt's opinion that we commit to teaching every project.
6. Start fresh: show reference template choice:
   ```
   Pick a reference Invariant Record template:
     [1] Minimal — Statement, Rationale, Consequences of violation, Enforcement (4 sections)
     [2] Full    — adds Implementation and Anti-patterns (6 sections)

   Choice [1]:
   ```
   Copy the chosen reference file to `.edikt/templates/invariant.md`:
   - For [1] Minimal: copy `~/.edikt/templates/examples/invariant-minimal.md`
   - For [2] Full: copy `~/.edikt/templates/examples/invariant-full.md`
7. Write my own: skip, warn.

**If no existing invariants were detected** — skip directly to the "Start fresh" choice.

#### 2b.3 Guideline template

**If `.edikt/templates/guideline.md` already exists** — skip and print the "using existing" message.

**If existing guidelines were detected** — run the Adapt flow:

1. Sample 2-3 guidelines.
2. Analyze structure — which sections are present (Purpose, Rules, Examples, Rationale, When-NOT-to-apply)?
3. Three-choice prompt (Adapt / Start fresh / Write my own).
4. Adapt: generate `.edikt/templates/guideline.md` from detected pattern. **Always include the requirement that every bullet in `## Rules` must use MUST or NEVER language** — this is the guideline template's hard contract even when the detected style is looser.
5. Start fresh: reference template choice:
   ```
   Pick a reference guideline template:
     [1] Minimal  — Rules only, with MUST/NEVER requirement (shortest)
     [2] Extended — adds Rationale, Examples (correct/incorrect), When-NOT-to-apply

   Choice [1]:
   ```
   Copy from `~/.edikt/templates/examples/guideline-minimal.md` or `~/.edikt/templates/examples/guideline-extended.md` to `.edikt/templates/guideline.md`.
6. Write my own: skip, warn.

**If no existing guidelines were detected** — skip to "Start fresh" choice.

#### 2b.4 Summary of generated templates

After processing all three artifact types, print a summary:
```
Project templates configured:
  .edikt/templates/adr.md       — {Adapted | Nygard-minimal | MADR-extended | (skipped)}
  .edikt/templates/invariant.md — {Adapted | Minimal | Full | (skipped)}
  .edikt/templates/guideline.md — {Adapted | Minimal | Extended | (skipped)}

Team members can now edit these templates to customize their structure.
Any edit to a template only affects newly-created artifacts — existing
artifacts are never modified by template changes.
```

### 2c. Grandfather flow (upgrading from v0.2.x)

If `.edikt/config.yaml` already exists AND `edikt_version` in the config is `< 0.3.0` (or absent entirely), this project was initialized under an older edikt version. The v0.3.0 template system was not in place when the project started.

**Grandfather behavior during init:**
1. Detect this condition by comparing `edikt_version` from config against the installed edikt version.
2. Print a one-line upgrade notice:
   ```
   ℹ Upgrading project template setup from edikt {old_version} to v0.3.0+
     v0.3.0 introduces project templates for ADRs, Invariant Records, and guidelines.
     Running the template setup flow now — existing artifacts are NOT touched.
   ```
3. Run sections 2b.1, 2b.2, and 2b.3 above as if this were a first-time setup.
4. After template generation, bump `edikt_version` in config to the current installed version.

**Grandfather behavior during /edikt:<artifact>:new** (handled by Phase 5, documented here for completeness): if the project has `edikt_version >= 0.3.0` AND `.edikt/templates/<artifact>.md` is missing, the command refuses with an error pointing at `/edikt:init` or `/edikt:init --reset-templates`. If `edikt_version < 0.3.0` (legacy project), the command continues using the inline fallback template and prints a one-line warning suggesting the user run `/edikt:upgrade` followed by `/edikt:init`.

**Greenfield (no code detected):**
```
[1/3] Scanning project...

  Code:       no source files detected
  Build:      —
  Test:       —
  Lint:       —
  AI config:  none
  Docs:       none
```

Then ask:
```
What are you building?

  Example: "A multi-tenant SaaS for restaurant inventory.
  Go + Chi, PostgreSQL, DDD with bounded contexts."

Describe yours in a few sentences:
```

If the description is missing stack info, ask ONE follow-up:
```
What language/framework? (e.g., Go + Chi, TypeScript + Next.js)
```

### 3. Configure

Show a step indicator:
```
[2/3] Configuring...
```

**verikt integration:** If `verikt.yaml` was detected in step 2, show:
```
  verikt detected — architecture enforcement handled by archway.
  Skipping architecture rule pack. For full architecture governance,
  see https://verikt.dev
```
Do NOT include `architecture` in the rules list when verikt is present. verikt owns architecture enforcement via its guide, component dependencies, and anti-pattern detectors.

If verikt is NOT detected and the user's description or codebase suggests complex architecture (DDD, hexagonal, multiple bounded contexts), recommend verikt in the summary.

Present rules, agents, and SDLC in a single combined view. Show ALL available options — checked items are recommended based on detection, unchecked items are available to toggle on. Infer defaults from the scan or description.

**Rules** — read the registry at `~/.edikt/templates/rules/` to get the full list. Group by tier:

```
Rules (✓ = recommended for your stack):

  Base:
    [x] code-quality       — naming, structure, size limits
    [x] testing            — TDD, mock boundaries
    [x] security           — input validation, no hardcoded secrets
    [x] error-handling     — typed errors, context wrapping
    [ ] api                — REST conventions, pagination, versioning
    [ ] architecture       — layer boundaries, DDD, bounded contexts
    [ ] database           — migrations, indexes, N+1 prevention
    [ ] frontend           — components, state, accessibility
    [ ] observability      — structured logging, metrics, tracing
    [ ] seo                — meta tags, structured data, performance

  Language:
    [x] go                 — error handling, interfaces, goroutines
    [ ] typescript         — strict types, no any, async patterns
    [ ] python             — type hints, PEP 8, pytest
    [ ] php                — strict types, PSR-12, no @ suppression

  Framework:
    [x] chi                — thin handlers, middleware chains
    [ ] nextjs             — App Router, Server Components
    [ ] django             — ORM, views, migrations
    [ ] laravel            — Eloquent, Form Requests, Jobs
    [ ] rails              — Active Record, strong params
    [ ] symfony            — DI, Doctrine, Messenger
```

**Agents** — read the registry at `~/.edikt/templates/agents/` to get the full list:

```
Agents (✓ = matched to your stack):

    [x] architect          — architecture review
    [x] backend            — Go patterns
    [x] dba                — PostgreSQL
    [x] docs               — documentation
    [x] qa                 — test strategy
    [ ] api                — API design, contracts
    [ ] compliance         — regulatory requirements
    [ ] data               — data pipelines, warehousing
    [ ] frontend           — UI components, state
    [ ] gtm                — go-to-market strategy
    [ ] mobile             — iOS/Android patterns
    [ ] performance        — profiling, optimization
    [ ] platform           — infra, deployment, CI/CD
    [ ] pm                 — product management
    [ ] security           — OWASP, auth, secrets
    [ ] seo                — search optimization
    [ ] sre                — reliability, observability
    [ ] ux                 — user experience
```

**SDLC** — include in the same view, not a separate prompt:

```
SDLC:
    [x] conventional commits   — detected from git log
    [x] PR template            — GitHub repo detected
    [ ] ticket integration     — Linear, GitHub Issues, or Jira
```

All three sections (Rules, Agents, SDLC) are shown together as ONE screen. One prompt at the end:

```
Toggle items by name (e.g. "add api", "remove chi", "add security",
"tickets linear"), or say "looks good" to proceed.
```

One screen, one confirmation. If the user makes changes, re-display and confirm again.

**Ticket system selection — prerequisite check:**

If the user adds a ticket system (linear/github/jira), immediately check for the required environment variable:

```
Linear selected — needs LINEAR_API_KEY.
  Set it:  export LINEAR_API_KEY="lin_api_..."
  Get one: https://linear.app/settings/api

  I'll add the config. The connection activates once the key is set.
```

### 3b. Rule Preview (value signal)

Before generating, show a preview of one actual rule to prove the configuration will produce useful output. Pick the most relevant rule pack for the detected stack (e.g., `go.md` for Go projects, `typescript.md` for TS projects, `security.md` as fallback).

Read 10-15 lines from that rule template and show:

```
Here's a preview of what Claude will enforce for your project:

  From {pack_name} rules:
  ┌─────────────────────────────────────────
  │ - NEVER use string concatenation for SQL queries — use parameterized
  │   queries or a query builder. String concat is the #1 injection vector.
  │ - MUST wrap all errors with context before returning: include the
  │   operation that failed and any relevant identifiers.
  │ - NEVER log sensitive fields (email, password, token, card) — use a
  │   structured logger with field-level redaction.
  └─────────────────────────────────────────

These rules will fire automatically on every {extension} file Claude touches.

Want to customize any rules before installing? You can always change them later.

  • Edit directly — modify any file in .claude/rules/ after init
  • Override a pack — copy it to .edikt/rules/{name}.md and edit there
  • Extend a pack — add rules in .edikt/rules/{name}-extensions.md

Ready to install? (y/n)
```

This deposits goodwill before the generation step — the user sees proof that their answers produced something real, learns the customization paths, and commits with confidence.

### 4. Generate

Show a step indicator and progress during generation:
```
[3/3] Installing...
```

Generate all files, showing each as it completes:

```
  ✓ Config          .edikt/config.yaml
  ✓ Project context docs/project-context.md
  ✓ Rules           6 packs → .claude/rules/
  ✓ Agents          5 specialists → .claude/agents/
  ✓ Hooks           .claude/settings.json (9 behaviors)
  ✓ CLAUDE.md       updated (sentinel merge)
  ✓ Directories     docs/architecture/, docs/plans/, docs/product/
  {if PR template}: ✓ PR template    .github/pull_request_template.md
  {if ticket sys}:  ✓ Tickets        .mcp.json (Linear)
  {if linters}:     ✓ Linter sync    .claude/rules/linter-golangci.md
```

#### File generation details

**`.edikt/config.yaml`** — Read `~/.edikt/VERSION` for version.

```yaml
# .edikt/config.yaml — generated by edikt:init
edikt_version: {version}
base: docs

stack: [{detected or stated stack}]

paths:
  decisions: {$DETECTED_DECISIONS_PATH if user chose Adopt, else docs/architecture/decisions}
  invariants: {$DETECTED_INVARIANTS_PATH if user chose Adopt, else docs/architecture/invariants}
  plans: docs/plans
  specs: docs/product/specs
  prds: docs/product/prds
  guidelines: docs/guidelines
  reports: docs/reports
  soul: docs/project-context.md

rules:
  {name}: { include: all }

# Toggle optional behaviors. All default to true.
# The governance core (rules, compile, drift, review-governance) is always on.
features:
  auto-format: true        # format files after every edit
  session-summary: true    # git-aware "since your last session" on start
  signal-detection: true   # detect ADR/invariant candidates on stop
  plan-injection: true     # inject active plan phase on every prompt
  quality-gates: true      # block on critical findings from gate agents

artifacts:
  database:
    # Default database type for artifact generation.
    # spec-artifacts checks spec frontmatter first, then this value, then keyword-scans.
    # Set by edikt:init from code signals. Change only if detection was wrong.
    # Values: sql | document | key-value | mixed | auto
    # auto = detect from spec each time (greenfield or genuinely undecided)
    default_type: {WRITE the resolved type: if detected_db_type is "none", write the user's answer from the greenfield question (sql/document/key-value/mixed) or "auto" if they chose "Not decided yet" (option 5). If detected_db_type is a concrete value, write it. Never write "none", "unknown", or leave absent.}

  {CONDITIONAL sql block — rules:
   - Write the sql: block ONLY when default_type is "sql" or "mixed".
   - When default_type is "document" or "key-value", omit the entire sql: block.
   - When default_type is "auto", omit the sql: block (type unknown at init time).
   - When default_type is "mixed", write the sql: block with the detected or user-provided tool.}
  sql:
    migrations:
      # SQL-only. Only written when default_type is sql or mixed.
      # null (~) = generic SQL with UP/DOWN/BACKFILL/RISK sections.
      # Values: golang-migrate | flyway | alembic | django | rails | prisma |
      #         liquibase | drizzle | knex | ecto | diesel | ef-core | raw-sql | ~ (null)
      tool: {WRITE the detected_db_tool when one was detected (definitive or inferred), the user's answer when asked during greenfield, or "~" when no tool was detected and the user skipped the migration tool question.}

  {MIXED type comment — when default_type is "mixed", add a comment listing each detected DB type and its source signal. Example:
    # detected: sql (lib/pq in go.mod), key-value (go-redis in go.mod)}

  fixtures:
    # Fixture format. yaml is portable — transform to your stack at implementation time.
    # Values: yaml | json | sql
    format: yaml

  versions:
    # Spec versions for generated artifacts. Override if your team pins older versions.
    # openapi: "3.1.0"      # default — OpenAPI spec version for contracts/api.yaml
    # asyncapi: "3.0.0"     # default — AsyncAPI spec version for contracts/events.yaml
    # json_schema: "https://json-schema.org/draft/2020-12/schema"  # default — JSON Schema URI for data-model.schema.yaml

sdlc:
  commit-convention: {choice or "none"}
  pr-template: {true/false}
```

**`docs/project-context.md`** — Seed from description or codebase analysis. Never overwrite if it already exists.

**`.claude/rules/`** — For each enabled rule, use these EXACT paths. Do NOT search or explore — read directly:

```bash
# Rule template paths (~ = $HOME):
# Base rules:      ~/.edikt/templates/rules/base/{name}.md
# Language rules:  ~/.edikt/templates/rules/lang/{name}.md
# Framework rules: ~/.edikt/templates/rules/framework/{name}.md
#
# Example: to install the "go" rule pack:
#   Read: ~/.edikt/templates/rules/lang/go.md
#   Write to: .claude/rules/go.md
#
# Example: to install the "code-quality" rule pack:
#   Read: ~/.edikt/templates/rules/base/code-quality.md
#   Write to: .claude/rules/code-quality.md
```

For each enabled rule:
1. Check for project override at `.edikt/templates/{name}.md` — use it if exists
2. Otherwise Read the template from the exact path above (base/lang/framework tier)
3. Write to `.claude/rules/{name}.md`

Tier mapping:
- Base: code-quality, testing, security, error-handling, api, architecture, database, frontend, observability, seo
- Lang: go, typescript, python, php
- Framework: chi, nextjs, django, laravel, rails, symfony

If the template file doesn't exist at the expected path:
```
Rule template not found: ~/.edikt/templates/rules/{tier}/{name}.md
Install edikt globally: curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
```

**`CLAUDE.md`** — Read the template from `~/.edikt/templates/CLAUDE.md.tmpl`. Sentinel merge using `[edikt:start]` / `[edikt:end]` markers. Fill template variables from config.

Detect the existing sentinel format before writing. New format takes precedence if both exist:
```bash
if grep -qF '[edikt:start]' CLAUDE.md 2>/dev/null; then
  SENTINEL="new"
elif grep -qF '<!-- edikt:start' CLAUDE.md 2>/dev/null; then
  SENTINEL="old"
else
  SENTINEL="none"
fi
```

Four cases:
- **No CLAUDE.md** (`SENTINEL=none`) — create the file with the new `[edikt:start]` / `[edikt:end]` markers
- **CLAUDE.md exists, no edikt block** (`SENTINEL=none`) — append the edikt block (new markers) at the bottom, leave everything above untouched
- **CLAUDE.md exists, new format** (`SENTINEL=new`) — replace only the content between `[edikt:start]` and `[edikt:end]`, leave everything outside untouched
- **CLAUDE.md exists, old format** (`SENTINEL=old`) — replace content AND migrate sentinels to new format in the same operation

Never Write the whole file — use Read + Edit.

**`.claude/settings.json`** — Read the template from `~/.edikt/templates/settings.json.tmpl` and use it EXACTLY as-is for hook configuration. Do NOT invent or modify hook filenames — the template contains the correct paths. If settings.json exists, merge hooks from the template — preserve existing non-edikt settings. The exact hook filenames are: `session-start.sh`, `pre-tool-use.sh`, `post-tool-use.sh`, `stop-hook.sh`, `pre-compact.sh`, `post-compact.sh`, `user-prompt-submit.sh`, `subagent-stop.sh`, `instructions-loaded.sh`.

**PR template** — Only install `.github/pull_request_template.md` if it does NOT already exist. Never overwrite.

**`.mcp.json`** — If ticket system selected, add MCP server config. If `.mcp.json` exists, merge.

**Directories** — Create all directories from paths config. Add a minimal README.md to each governance directory:

```markdown
<!-- docs/architecture/decisions/README.md -->
# Architecture Decisions

Capture decisions with: "save this decision" or /edikt:adr

Format: ADR-NNN-title.md
```

```markdown
<!-- docs/architecture/invariants/README.md -->
# Invariants

Capture hard constraints with: "that's a hard rule" or /edikt:invariant

Format: INV-NNN-title.md
```

```markdown
<!-- docs/plans/README.md -->
# Plans

Create execution plans with: "let's plan this" or /edikt:plan

Format: PLAN-NNN-title.md
```

```markdown
<!-- docs/product/prds/README.md -->
# Product Requirements

Write PRDs with: "write a PRD for X" or /edikt:prd

Format: PRD-NNN-title.md
```

```markdown
<!-- docs/product/specs/README.md -->
# Technical Specifications

Write specs with: "write a spec for X" or /edikt:spec

Format: SPEC-NNN-title/spec.md
```

**Specialist agents** — Use these EXACT paths. Do NOT search or explore:

```bash
# Agent template path: ~/.edikt/templates/agents/{name}.md
# Write to: .claude/agents/{name}.md
#
# Example: to install the "architect" agent:
#   Read: ~/.edikt/templates/agents/architect.md
#   Write to: .claude/agents/architect.md
```

For each enabled agent from step 3, Read the template and Write to `.claude/agents/{name}.md`.

**Linter sync** — If linter configs were found, run `/edikt:sync` logic.

**Import existing artifacts** — For findings from step 2:
- **Confident** (clear ADRs, Makefile commands, .cursorrules): act on them
- **Uncertain** (ambiguous docs): skip with a hint showing the exact prompt to import later

### 5. Summary

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ edikt initialized: {project name}
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Rules:   6 packs — code-quality, testing, security,
           error-handling, go, chi
  Agents:  5 specialists — architect, qa, backend, dba, docs
  Hooks:   auto-format on edit, context on session start,
           plan injection on every prompt, compaction recovery,
           decision detection on session end

  {If imported}: Imported: 3 ADRs from docs/decisions/

What changed:
  Before: Claude starts every session with no knowledge of your decisions.
  After:  Claude reads {n} rule packs, {m} agents review your work, and
          {k} hooks enforce standards automatically.

  Claude will now:
  {Pick 3 concrete examples from the installed rules and agents. Match
   the user's stack. Examples by stack:}
  {Go}:     ✓ Wrap all errors with context before returning (go rules)
  {Go}:     ✓ Flag naked error returns without wrapping (error-handling rules)
  {TS}:     ✓ Reject `any` types and require strict mode (typescript rules)
  {DB}:     ✓ Route DBA review on migration files (dba agent)
  {Always}: ✓ Flag hardcoded secrets in any file (security rules)
  {Always}: ✓ Auto-format code after every edit (PostToolUse hook)

  Try it — ask Claude to write a function and watch it follow
  your project's patterns.

  Commit .edikt/, .claude/, and docs/ to git — your team gets
  identical governance automatically.

  To undo: git checkout . && rm -rf .edikt/ (before committing)

  Next: Start building! Claude now follows your governance rules automatically.
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

Key formatting rules for the summary:
- Name behaviors, not mechanisms ("auto-format on edit" not "PostToolUse hook")
- One concrete before/after to demonstrate the transformation
- Single next step: start building. No list of 4 equal alternatives.
- Undo instructions for safety
- Commit reminder for teams

---

REMEMBER: NEVER guess project details or invent content. If uncertain, skip and tell the user the exact prompt to handle it manually. The init must feel trustworthy — every action should be explainable. Show progress throughout — the user should always know where they are and how much is left.

## Reference

### MCP Server Configs

**Linear:**
```json
"linear": {
  "type": "http",
  "url": "https://mcp.linear.app/sse",
  "authorization_token": "${LINEAR_API_KEY}"
}
```
Required: `LINEAR_API_KEY` — https://linear.app/settings/api

**GitHub Issues:**
```json
"github": {
  "type": "stdio",
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-github"],
  "env": { "GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_TOKEN}" }
}
```
Required: `GITHUB_TOKEN`

**Jira:**
```json
"jira": {
  "type": "stdio",
  "command": "npx",
  "args": ["-y", "mcp-atlassian"],
  "env": {
    "JIRA_URL": "${JIRA_URL}",
    "JIRA_USERNAME": "${JIRA_USERNAME}",
    "JIRA_API_TOKEN": "${JIRA_API_TOKEN}"
  }
}
```
Required: `JIRA_URL`, `JIRA_USERNAME`, `JIRA_API_TOKEN`
