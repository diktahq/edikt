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

- **Team member joining** (rules, agents, hooks already in `.claude/`): verify local setup matches config. Check for gaps (missing rule files, missing hooks, version mismatch). If everything matches:
  ```
  edikt is set up and matches your config. Nothing to do.

  Run /edikt:status to see governance dashboard.
  ```
  If gaps exist:
  ```
  edikt config exists but your local setup needs sync:
    - 2 rule packs missing from .claude/rules/
    - Hooks not installed in .claude/settings.json

  Sync your local setup? (Y/n)
  ```
  Sync only fills gaps — never overwrites existing files.

- **Reconfigure** (user explicitly wants to change settings): Before any file generation, scan existing `.claude/rules/*.md` for the `<!-- edikt:generated -->` tag. Files WITHOUT this tag have been manually customized. Show a change summary:
  ```
  Reconfiguring edikt.

  Will update:  3 rule packs (from templates)
  Will create:  2 new rule packs
  Will skip:    1 customized file (security.md — manually edited)
  Will preserve: CLAUDE.md (sentinel merge), settings.json (hook merge)

  Proceed? (y/N)
  ```
  Never overwrite files that lack the `<!-- edikt:generated -->` tag.

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
  grep -qF 'aws-sdk-go'          go.mod && echo "DB_SIGNAL: document - inferred aws-sdk-go (check .go files for DynamoDB import)"
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
  liquibase | drizzle | knex | ecto | diesel | raw-sql
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

**If existing ADRs or decision docs were detected**, offer to import them:
```
Found 3 existing architecture decisions in docs/decisions/.
Import them into edikt's governance? (Y/n)
```

If the user accepts:
- Read each existing ADR file
- Copy or move to `{decisions_path}/` (from config, default `docs/architecture/decisions/`)
- If the file doesn't follow ADR format (missing status, missing Decision section), convert it: extract the decision, add `status: accepted` frontmatter, preserve the original content
- After import: "Imported 3 ADRs. Run `/edikt:compile` to compile them into governance directives."

If the user declines, continue — remind them they can import later with `/edikt:intake`.

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

**SDLC:**

```
SDLC:
  Commits:    conventional commits (detected from git log)
  PR template: yes (GitHub repo detected)
  Tickets:    none detected
```

```
Toggle items by name (e.g. "add api", "remove chi", "add security"),
adjust SDLC (e.g. "tickets linear"), or say "looks good" to proceed:
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
  decisions: {adapted or default: docs/architecture/decisions}
  invariants: {adapted or default: docs/architecture/invariants}
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
    default_type: {WRITE the detected_db_type from Step 2 database detection, or the user's answer from the greenfield question. Write "auto" only when the user chose "Not decided yet" (option 5). Never write "unknown", never leave absent.}

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
      #         liquibase | drizzle | knex | ecto | diesel | raw-sql | ~ (null)
      tool: {WRITE the detected_db_tool when one was detected (definitive or inferred), the user's answer when asked during greenfield, or "~" when no tool was detected and the user skipped the migration tool question.}

  {MIXED type comment — when default_type is "mixed", add a comment listing each detected DB type and its source signal. Example:
    # detected: sql (lib/pq in go.mod), key-value (go-redis in go.mod)}

  fixtures:
    # Fixture format. yaml is portable — transform to your stack at implementation time.
    # Values: yaml | json | sql
    format: yaml

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

Detect the existing sentinel format before writing:
```bash
grep -qF '[edikt:start]' CLAUDE.md 2>/dev/null && echo "new"
grep -qF '<!-- edikt:start' CLAUDE.md 2>/dev/null && echo "old"
```

Three cases:
- **No CLAUDE.md** — create the file with the new `[edikt:start]` / `[edikt:end]` markers
- **CLAUDE.md exists, no edikt block** — append the edikt block (new markers) at the bottom, leave everything above untouched
- **CLAUDE.md exists, new `[edikt:start]` marker** — replace only the content between `[edikt:start]` and `[edikt:end]`, leave everything outside untouched
- **CLAUDE.md exists, old `<!-- edikt:start` marker** — replace content AND migrate sentinels to new format in the same operation

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
 EDIKT INITIALIZED
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Rules:   6 packs — code-quality, testing, security,
           error-handling, go, chi
  Agents:  5 specialists — architect, qa, backend, dba, docs
  Hooks:   auto-format on edit, context on session start,
           plan injection on every prompt, compaction recovery,
           decision detection on session end

  {If imported}: Imported: 3 ADRs from docs/decisions/

What just changed:

  Before edikt, Claude writes code with no project standards,
  no architecture awareness, and forgets everything between sessions.

  Now Claude reads your 6 rule packs before writing any code.
  Try it — ask Claude to write a function and watch it follow
  your project's error handling and testing patterns.

  Commit .edikt/, .claude/, and docs/ to git — your team gets
  identical governance automatically.

  To undo: git checkout . && rm -rf .edikt/ (before committing)
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
