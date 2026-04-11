---
name: edikt:config
description: "View and modify edikt project configuration"
effort: low
allowed-tools:
  - Read
  - Write
  - Edit
  - Bash
---

# edikt:config

View, query, and modify `.edikt/config.yaml`. Provides discovery of all configuration keys, validation on writes, and natural-language config changes.

CRITICAL: This command requires interactive input. If you are in plan mode (you can only describe actions, not perform them), output this and stop:
```
⚠️  This command requires user interaction and cannot run in plan mode.
Exit plan mode first, then run the command again.
```

## Arguments

- No argument: show all config sections with current values
- `get {key}`: show a specific key's value, default, and where it's used
- `set {key} {value}`: validate and write a config value

## Instructions

### 0. Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### No argument — Show all config

Read `.edikt/config.yaml`. Output every section with current values and defaults:

```
edikt config — {project_name or repo directory name}

 VERSION
 ──────
 edikt_version: {value}

 PATHS
 ─────
 decisions:    {value}     (default: docs/architecture/decisions)
 invariants:   {value}     (default: docs/architecture/invariants)
 guidelines:   {value}     (default: docs/guidelines)
 plans:        {value}     (default: docs/plans)
 specs:        {value}     (default: docs/product/specs)
 prds:         {value}     (default: docs/product/prds)
 brainstorms:  {value}     (default: docs/brainstorms)
 reports:      {value}     (default: docs/reports)
 soul:         {value}     (default: docs/project-context.md)

 FEATURES
 ────────
 auto-format:       {true|false}   (default: true)
 session-summary:   {true|false}   (default: true)
 signal-detection:  {true|false}   (default: true)
 plan-injection:    {true|false}   (default: true)
 quality-gates:     {true|false}   (default: true)

 ARTIFACTS
 ─────────
 database.default_type:   {value}   (default: auto)
 sql.migrations.tool:     {value}   (default: ~)
 fixtures.format:         {value}   (default: yaml)
 versions.openapi:        {value}   (default: 3.1.0)
 versions.asyncapi:       {value}   (default: 3.0.0)
 versions.json_schema:    {value}   (default: https://json-schema.org/draft/2020-12/schema)

 SDLC
 ────
 commit-convention:  {value}   (default: conventional)
 pr-template:        {value}   (default: false)

 AGENTS
 ──────
 custom: {list or none}

 GATES
 ─────
 {gate entries or "none configured"}

 HOOKS
 ─────
 pre-push-security:  {true|false}   (default: true)

 HEADLESS
 ────────
 {headless.answers entries or "not configured"}

 RULES
 ─────
 {rule toggles or "none" for markdown-only projects}

 STACK
 ─────
 {detected stack or "not set"}

Use /edikt:config get {key} for details on a specific key.
Use /edikt:config set {key} {value} to change a value.
```

For keys not present in the config file, show the default value with `(default)` label. For keys present, show the actual value.

### `get {key}` — Show key details

Read `.edikt/config.yaml`. Look up the key using dot notation (e.g., `artifacts.versions.openapi`).

Output:
```
{key}: {current value or "not set (default: {default})"}

  Default:     {default value}
  Valid values: {list of valid values}
  Used by:     {which commands read this key}
  Description: {what this key controls}
```

Use the Key Reference table below for defaults, valid values, used-by, and descriptions.

If the key doesn't exist in the reference table:
```
Unknown config key: {key}

Run /edikt:config to see all available keys.
```

### `set {key} {value}` — Validate and write

1. Look up the key in the Key Reference table.
2. If the key doesn't exist in the reference → reject:
   ```
   Unknown config key: {key}
   Run /edikt:config to see all available keys.
   ```
3. Validate the value against the key's valid values. If invalid → reject:
   ```
   Invalid value "{value}" for {key}.
   Valid values: {list}
   ```
4. Read `.edikt/config.yaml`.
5. If the key's parent section doesn't exist, create it.
6. Write the value. Preserve all existing comments and formatting.
7. Output:
   ```
   ✅ {key}: {old value} → {new value}
   ```

**Special validation rules:**
- `edikt_version`: NEVER allow setting this directly. Output: `edikt_version is managed by /edikt:init and /edikt:upgrade. Do not set it manually.`
- `paths.*`: Validate the directory exists (warn if it doesn't, but allow — init may create it later)
- `features.*`: Must be `true` or `false`
- `artifacts.database.default_type`: Must be one of `sql | document | key-value | mixed | auto`
- `artifacts.versions.openapi`: Must match pattern `N.N.N`
- `artifacts.versions.asyncapi`: Must match pattern `N.N.N`
- `sdlc.commit-convention`: Must be one of `conventional | none`

## Key Reference

| Key | Default | Valid values | Used by | Description |
|-----|---------|-------------|---------|-------------|
| `edikt_version` | — | *(read-only)* | init, doctor, upgrade | Project's edikt version |
| `base` | `docs` | any path | all commands | Base directory for governance artifacts |
| `paths.decisions` | `docs/architecture/decisions` | any path | adr:new, compile, doctor | ADR directory |
| `paths.invariants` | `docs/architecture/invariants` | any path | invariant:new, compile, doctor | Invariant Records directory |
| `paths.guidelines` | `docs/guidelines` | any path | guideline:new, compile | Team guidelines directory |
| `paths.plans` | `docs/plans` | any path | plan, status | Execution plans directory |
| `paths.specs` | `docs/product/specs` | any path | spec, artifacts | Technical specs directory |
| `paths.prds` | `docs/product/prds` | any path | prd | PRD directory |
| `paths.brainstorms` | `docs/brainstorms` | any path | brainstorm | Brainstorm artifacts (gitignored by default) |
| `paths.reports` | `docs/reports` | any path | audit, drift | Drift reports, audits (gitignored by default) |
| `paths.soul` | `docs/project-context.md` | any file path | context, init | Project identity file |
| `features.auto-format` | `true` | `true`, `false` | PostToolUse hook | Format files after edits |
| `features.session-summary` | `true` | `true`, `false` | SessionStart hook | Show changes since last session |
| `features.signal-detection` | `true` | `true`, `false` | Stop hook | Detect uncaptured ADR/invariant candidates |
| `features.plan-injection` | `true` | `true`, `false` | UserPromptSubmit hook | Inject active plan phase |
| `features.quality-gates` | `true` | `true`, `false` | SubagentStop hook | Block on critical agent findings |
| `artifacts.database.default_type` | `auto` | `sql`, `document`, `key-value`, `mixed`, `auto` | artifacts | Database type for artifact generation |
| `artifacts.sql.migrations.tool` | `~` | `golang-migrate`, `flyway`, `alembic`, `django`, `rails`, `prisma`, `liquibase`, `drizzle`, `knex`, `ecto`, `diesel`, `ef-core`, `raw-sql`, `~` | artifacts | SQL migration tool |
| `artifacts.fixtures.format` | `yaml` | `yaml`, `json`, `sql` | artifacts | Fixture format |
| `artifacts.versions.openapi` | `3.1.0` | semver string | artifacts | OpenAPI spec version |
| `artifacts.versions.asyncapi` | `3.0.0` | semver string | artifacts | AsyncAPI spec version |
| `artifacts.versions.json_schema` | `https://json-schema.org/draft/2020-12/schema` | JSON Schema URI | artifacts | JSON Schema version for document-mongo |
| `sdlc.commit-convention` | `conventional` | `conventional`, `none` | plan, session | Commit convention |
| `sdlc.pr-template` | `false` | `true`, `false` | init | PR template enabled |
| `hooks.pre-push-security` | `true` | `true`, `false` | pre-push hook | Pre-push secret scanning |
| `evaluator.preflight` | `true` | `true`, `false` | plan (step 11) | Pre-flight criteria validation before phases start |
| `evaluator.phase-end` | `true` | `true`, `false` | plan (phase evaluation) | Phase-end evaluation after phases complete |
| `evaluator.mode` | `headless` | `headless`, `subagent` | plan, evaluator agent | Execution mode — headless (separate claude -p) or subagent (forked in session) |
| `agents.custom` | `[]` | list of agent slugs | upgrade | Agents to skip on upgrade |

---

After any `set` operation, output:
```
✅ Config updated.

Next: Run /edikt:doctor to verify governance health.
```
