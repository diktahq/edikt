# /edikt:config

View, query, and modify project configuration. Provides discovery of all 34 configuration keys, validation on writes, and natural-language config changes.

## Usage

```bash
/edikt:config                              # show all config
/edikt:config get artifacts.versions.openapi  # show one key
/edikt:config set features.quality-gates false # change a value
```

## Show all config

Running with no arguments displays every section with current values and defaults:

```text
edikt config — my-service

 VERSION
 ──────
 edikt_version: 0.3.1

 PATHS
 ─────
 decisions:    docs/architecture/decisions   (default)
 invariants:   docs/architecture/invariants  (default)
 guidelines:   docs/guidelines               (default)
 ...

 FEATURES
 ────────
 auto-format:       true   (default)
 session-summary:   true   (default)
 quality-gates:     false  ← changed from default

 ARTIFACTS
 ─────────
 database.default_type:   sql
 versions.openapi:        3.0.0  ← changed from default (3.1.0)
 ...
```

Keys that differ from their default are highlighted.

## Get a specific key

```bash
/edikt:config get artifacts.versions.openapi
```

```text
artifacts.versions.openapi: "3.0.0"

  Default:     3.1.0
  Valid values: semver string (e.g., 3.0.0, 3.1.0)
  Used by:     /edikt:sdlc:artifacts (contracts/api.yaml)
  Description: OpenAPI spec version for generated API contracts
```

## Set a value

```bash
/edikt:config set features.quality-gates false
```

```text
✅ features.quality-gates: true → false
```

Invalid values are rejected with an explanation:

```text
Invalid value "mongo" for artifacts.database.default_type.
Valid values: sql, document, key-value, mixed, auto
```

Protected keys like `edikt_version` cannot be set directly — they're managed by `/edikt:init` and `/edikt:upgrade`.

## Full config reference

### `paths.*` — Directory layout

All paths are relative to the repo root.

| Key | Default | Description |
|-----|---------|-------------|
| `paths.decisions` | `docs/architecture/decisions` | ADR directory |
| `paths.invariants` | `docs/architecture/invariants` | Invariant Records directory |
| `paths.guidelines` | `docs/guidelines` | Team guidelines directory |
| `paths.plans` | `docs/plans` | Execution plans directory |
| `paths.specs` | `docs/product/specs` | Technical specifications directory |
| `paths.prds` | `docs/product/prds` | PRD directory |
| `paths.brainstorms` | `docs/brainstorms` | Brainstorm artifacts (gitignored by default) |
| `paths.reports` | `docs/reports` | Drift reports, audits (gitignored by default) |
| `paths.soul` | `docs/project-context.md` | Project identity file — read by `/edikt:context` |
| `paths.templates` | `.edikt/templates` | Template override directory (per ADR-005) |

### `features.*` — Optional behaviors

All default to `true`. The governance core (rules, compile, drift) is always on.

| Key | Default | Description |
|-----|---------|-------------|
| `features.auto-format` | `true` | PostToolUse hook — format files after every Write/Edit |
| `features.session-summary` | `true` | SessionStart hook — show git changes since last session |
| `features.signal-detection` | `true` | Stop hook — detect uncaptured ADR/invariant candidates |
| `features.plan-injection` | `true` | UserPromptSubmit hook — inject active plan phase into every prompt |
| `features.quality-gates` | `true` | SubagentStop hook — block on critical findings from gate agents |

### `evaluator.*` — Evaluator configuration

Controls pre-flight validation and phase-end evaluation in the plan command.

| Key | Default | Valid values | Description |
|-----|---------|-------------|-------------|
| `evaluator.preflight` | `true` | `true`, `false` | Pre-flight criteria validation — classifies criteria as TESTABLE/VAGUE/SUBJECTIVE/BLOCKED before phases start |
| `evaluator.phase-end` | `true` | `true`, `false` | Phase-end evaluation — verifies completed work meets acceptance criteria |
| `evaluator.mode` | `headless` | `headless`, `subagent` | `headless` runs a separate `claude -p` (zero shared context, works in CI). `subagent` runs within the session (faster, partial isolation). |
| `evaluator.max-attempts` | `5` | positive integer | Max phase retries before marking phase as `stuck` with human decision prompt |
| `evaluator.model` | `sonnet` | `sonnet`, `opus`, `haiku` | Model for headless evaluator invocation |

See [Evaluator](/governance/evaluator) for the headless vs subagent comparison table.

### `artifacts.*` — Artifact generation

Controls how `/edikt:sdlc:artifacts` generates design blueprints.

| Key | Default | Valid values | Description |
|-----|---------|-------------|-------------|
| `artifacts.database.default_type` | `auto` | `sql`, `document`, `key-value`, `mixed`, `auto` | Database type for data model generation. `auto` detects from spec content each time. |
| `artifacts.sql.migrations.tool` | `~` (null) | `golang-migrate`, `flyway`, `alembic`, `django`, `rails`, `prisma`, `liquibase`, `drizzle`, `knex`, `ecto`, `diesel`, `ef-core`, `raw-sql`, `~` | SQL migration tool. `~` produces generic UP/DOWN/BACKFILL/RISK format. Set by `/edikt:init` from code signals. |
| `artifacts.fixtures.format` | `yaml` | `yaml`, `json`, `sql` | Fixture format. YAML is portable — transform to your stack at implementation time. |
| `artifacts.versions.openapi` | `3.1.0` | semver string | OpenAPI spec version for `contracts/api.yaml` |
| `artifacts.versions.asyncapi` | `3.0.0` | semver string | AsyncAPI spec version for `contracts/events.yaml` |
| `artifacts.versions.json_schema` | `https://json-schema.org/draft/2020-12/schema` | JSON Schema URI | JSON Schema version for `data-model.schema.yaml` (document-mongo) |

### `sdlc.*` — SDLC settings

| Key | Default | Valid values | Description |
|-----|---------|-------------|-------------|
| `sdlc.commit-convention` | `conventional` | `conventional`, `none` | Commit message convention used in plans and session summaries |
| `sdlc.pr-template` | `false` | `true`, `false` | Whether a PR template was detected/installed |

### `agents.*` — Agent customization

| Key | Default | Description |
|-----|---------|-------------|
| `agents.custom` | `[]` | List of agent slugs to skip on `/edikt:upgrade`. Use for agents you've customized or created yourself. |

### `gates.*` — Quality gate configuration

Gates are team-level config — engineers can override findings but cannot disable gates.

```yaml
gates:
  security:
    level: critical
    agents:
      - security
  database:
    level: critical
    agents:
      - dba
```

| Field | Description |
|-------|-------------|
| `gates.{name}.level` | `critical` or `warning` — severity threshold for this gate |
| `gates.{name}.agents` | List of agent slugs that trigger this gate |

See [Quality Gates](/governance/gates) for the override flow and audit log.

### `hooks.*` — Git hook control

| Key | Default | Valid values | Description |
|-----|---------|-------------|-------------|
| `hooks.pre-push-security` | `true` | `true`, `false` | Pre-push secret scanning. Disable with `false` or one-time skip with `EDIKT_SECURITY_SKIP=1`. |

### `headless.*` — CI/headless mode

For running edikt in CI pipelines without interactive prompts.

```yaml
headless:
  answers:
    "proceed with compilation": "yes"
    "which packs to update": "all"
    "update anyway": "no"
```

| Key | Description |
|-------|-------------|
| `headless.answers` | Map of prompt substrings to auto-responses. When `EDIKT_HEADLESS=1` is set, matching prompts are answered automatically. |

### `rules` — Stack-specific rule toggles

Populated by `/edikt:init` from detected stack. Empty for markdown-only projects.

```yaml
rules:
  go: true
  chi: true
  security: true
  testing: true
  code-quality: true
```

Each key corresponds to a rule pack in `templates/rules/`. Set to `false` to disable a pack without deleting it.

### `stack` — Detected tech stack

Auto-populated by `/edikt:init`. Read-only — describes what was detected.

```yaml
stack:
  languages: [go, python]
  frameworks: [chi, django]
  databases: [postgres, redis]
```

## Environment variable overrides

These override config values at runtime without changing the file.

| Variable | Overrides | Description |
|----------|-----------|-------------|
| `EDIKT_HEADLESS=1` | `headless.answers` | Auto-answer prompts in CI |
| `EDIKT_FORMAT_SKIP=1` | `features.auto-format` | Skip auto-formatting (one-time) |
| `EDIKT_SECURITY_SKIP=1` | `hooks.pre-push-security` | Skip pre-push security scan (one-time) |
| `EDIKT_INVARIANT_SKIP=1` | — | Skip invariant check on push (one-time) |
| `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB=1` | — | Strip credentials from subprocess environments |

## What's next

- [/edikt:init](/commands/init) — set up a new project or validate your environment
- [/edikt:doctor](/commands/doctor) — validate governance health
- [Configurable Features](/governance/features) — what each feature toggle does
