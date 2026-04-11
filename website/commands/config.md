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

## Config sections

| Section | Keys | What it controls |
|---------|------|-----------------|
| `paths.*` | 10 | Directory layout for governance artifacts |
| `features.*` | 5 | Toggle optional behaviors (hooks, gates) |
| `artifacts.*` | 7 | Database type, migration tool, spec versions |
| `sdlc.*` | 2 | Commit convention, PR template |
| `agents.*` | 1 | Custom agents (skipped on upgrade) |
| `gates.*` | per-gate | Quality gate configuration |
| `hooks.*` | 1 | Git hook control |
| `headless.*` | 1 | CI/headless auto-answers |
| `rules` | per-pack | Stack-specific rule toggles |
| `stack` | 1 | Detected tech stack |

## What's next

- [/edikt:init](/commands/init) — set up a new project or validate your environment
- [/edikt:doctor](/commands/doctor) — validate governance health
- [Configurable Features](/governance/features) — what each feature toggle does
