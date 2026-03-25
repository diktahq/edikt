# Plan: spec-artifacts Redesign — Design Artifacts as Blueprints

## Overview
**Task:** Redesign `spec-artifacts` to treat artifacts as design blueprints: resolve database type from spec frontmatter → config → keyword scan, inject invariants as structured constraints, and produce per-type artifact formats. Update `init` to detect database stack from code and write a well-structured config. Formalise the config schema.
**Total Phases:** 4
**Estimated Cost:** $0.34
**Created:** 2026-03-25

## Progress

| Phase | Status | Updated |
|-------|--------|---------|
| 1     | ✅ Complete | 2026-03-25 |
| 2     | ✅ Complete | 2026-03-25 |
| 3     | ✅ Complete | 2026-03-25 |
| 4     | ✅ Complete | 2026-03-25 |

**IMPORTANT:** Update this table as phases complete. This table is the persistent state that survives context compaction.

## Config Contract

Phase 2 WRITES, Phase 3 READS — exact key paths both phases must use:

```yaml
artifacts:
  database:
    default_type: auto   # sql | document | key-value | mixed | auto
  sql:
    migrations:
      tool: ~            # golang-migrate | flyway | alembic | django | rails |
                         # prisma | liquibase | drizzle | knex | ecto | diesel | raw-sql | ~ (null)
                         # Only written by init when default_type is sql or mixed.
                         # Only read by spec-artifacts when resolved DB type is sql.
  fixtures:
    format: yaml         # yaml | json | sql
```

**Three rules both phases must follow:**

1. `auto` means defer to spec-level detection each time. Init writes `auto` only for greenfield where the DB genuinely hasn't been chosen. All other cases resolve to a concrete type.
2. `artifacts.sql.migrations.tool` is SQL-only. Init only writes it when the detected type is `sql` or `mixed`. spec-artifacts only reads it when the resolved DB type is `sql`. Document and key-value databases have no migration artifact.
3. Config stores `document` as a type — it never stores `document-mongo` or `document-dynamo`. Those sub-types are resolved by spec-artifacts at runtime from spec keywords (vendor names, patterns). Config cannot know the sub-type because init detects from code signals, not spec content.

## Model Assignment

| Phase | Task | Model | Reasoning | Est. Cost |
|-------|------|-------|-----------|-----------|
| 1 | Config schema | haiku | Simple YAML structure addition, no logic | $0.01 |
| 2 | init.md database detection | sonnet | Multi-signal detection logic, greenfield fallback | $0.08 |
| 3 | spec-artifacts.md redesign | sonnet | Resolve-context preamble, lookup tables, invariant injection | $0.16 |
| 4 | Test harness update | haiku | Output contract assertions against spec fixtures | $0.01 |

## Execution Strategy

| Phase | Depends On | Parallel With |
|-------|-----------|---------------|
| 1     | None      | -             |
| 2     | 1         | 3             |
| 3     | 1         | 2             |
| 4     | 2, 3      | -             |

---

## Phase 1: Config Schema

**Objective:** Add the `artifacts:` block to the config template using the Config Contract schema above, so phases 2 and 3 have a shared contract to write to and read from.
**Model:** `haiku`
**Max Iterations:** 2
**Completion Promise:** `PHASE 1 COMPLETE`
**Dependencies:** None

**Design decisions:**
- `auto` is the only valid "not set" value — communicates intent, distinguishable from null or missing.
- `artifacts.sql.migrations.tool` is nested under `sql` — it is SQL-only config. Init only writes it when the detected type is `sql` or `mixed`. If DB type is `document` or `key-value`, this key is omitted entirely.
- Config stores `document` as a type, never `document-mongo` or `document-dynamo`. Sub-type distinction is spec-artifacts' job at runtime.
- Monorepos: no per-service config. The spec frontmatter `database_type:` field handles per-spec overrides.

**Prompt:**

Read `.edikt/config.yaml`. Locate the config template that produces `.edikt/config.yaml` on init — search for `settings.json.tmpl` and `CLAUDE.md.tmpl` in `templates/`, and for any config template referenced in `commands/init.md`.

Add an `artifacts:` block to the config template with this exact structure and inline comments:

```yaml
artifacts:
  database:
    # Default database type for artifact generation.
    # spec-artifacts checks spec frontmatter first, then this value, then keyword-scans the spec.
    # Set by edikt:init from code signals. Change only if detection was wrong.
    # Values: sql | document | key-value | mixed | auto
    # auto = detect from spec each time (greenfield or genuinely undecided)
    default_type: auto

  sql:
    migrations:
      # SQL-only. Only written when default_type is sql or mixed.
      # null (~) = generic SQL with UP/DOWN/BACKFILL/RISK sections.
      # Set by edikt:init from code signals.
      # Values: golang-migrate | flyway | alembic | django | rails | prisma |
      #         liquibase | drizzle | knex | ecto | diesel | raw-sql | ~ (null)
      tool: ~

  fixtures:
    # Fixture format. yaml is portable — transform to your stack at implementation time.
    # Values: yaml | json | sql
    format: yaml
```

Also add this block to `.edikt/config.yaml` in this project (dogfooding). Use `default_type: auto` and omit `sql:` entirely since edikt has no database.

When complete, output: `PHASE 1 COMPLETE`

---

## Phase 2: init.md Database Detection

**Objective:** Extend `edikt:init` to detect database type and migration tool from code signals, write results to the `artifacts:` block using the Config Contract schema, and ask targeted questions for greenfield projects.
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `PHASE 2 COMPLETE`
**Dependencies:** Phase 1

**Config Contract reminder:** Write to `artifacts.database.default_type` and (SQL only) `artifacts.sql.migrations.tool`. No other key names. If detected type is `document` or `key-value`, omit the `sql:` block entirely.

**Design decisions:**
- Detection is confidence-based. Definitive signal (e.g. `prisma/schema.prisma`, `alembic.ini`) → auto-configure, show user what was detected. Indirect signal (dependency in package.json) → auto-configure, flag as inferred. Nothing found → ask.
- Detection is additive: Postgres + Redis signals → `default_type: mixed` with detected types listed in a comment.
- Greenfield path: 2 questions, integrated into the existing init interview. "Not decided yet" writes `auto`. Skip on migration tool writes `~`.
- Init always writes a concrete value or `auto`. It never writes `unknown` or leaves the key absent.

**Detection signal table:**

| File / pattern | DB type | Tool |
|---|---|---|
| `prisma/schema.prisma` exists | sql | prisma |
| `alembic.ini` exists | sql | alembic |
| `manage.py` exists + `*/migrations/*.py` found | sql | django |
| `flyway.conf` exists or `src/**/db/migration/*.sql` found | sql | flyway |
| `liquibase.properties` or `changelog.xml` exists | sql | liquibase |
| `go.mod` contains `lib/pq` or `jackc/pgx` | sql | - |
| `go.mod` contains `go-sql-driver/mysql` | sql | - |
| `go.mod` contains `mongo-driver` | document | - |
| `go.mod` contains `aws-sdk-go` (check for DynamoDB import in .go files) | document | - |
| `go.mod` contains `go-migrate` or `golang-migrate` | sql | golang-migrate |
| `package.json` dep `prisma` or `@prisma/client` | sql | prisma |
| `package.json` dep `mongoose` | document | - |
| `package.json` dep `@aws-sdk/client-dynamodb` | document | - |
| `package.json` dep `drizzle-orm` | sql | drizzle |
| `package.json` dep `knex` | sql | knex |
| `package.json` dep `typeorm` | sql | - |
| `package.json` dep `ioredis` or `redis` | key-value | - |
| `requirements.txt` or `pyproject.toml` contains `sqlalchemy` | sql | - |
| `requirements.txt` or `pyproject.toml` contains `pymongo` | document | - |
| `requirements.txt` or `pyproject.toml` contains `django` | sql | django |
| `Gemfile` contains `pg` or `mysql2` | sql | rails |
| `Gemfile` contains `mongoid` | document | - |
| `*.csproj` contains `EntityFramework` or `Npgsql` | sql | ef-core |
| `mix.exs` contains `ecto_sql` | sql | ecto |
| `Cargo.toml` contains `diesel` | sql | diesel |
| `Cargo.toml` contains `sqlx` | sql | raw-sql |
| `go.mod` contains `go-redis` | key-value | - |

**Prompt:**

Read `commands/init.md` in full. Understand Step 2 (Scan the Project) and Step 4 (config generation).

Add a "Database detection" sub-step inside Step 2, after framework/language detection. Use `grep -F` for all dependency name matching (not regex — safe against injection). Use file existence checks for config files. Example pattern:

```bash
# Definitive signals (high confidence)
[ -f prisma/schema.prisma ] && echo "DB: sql, tool: prisma"
[ -f alembic.ini ] && echo "DB: sql, tool: alembic"

# Dependency signals (inferred)
grep -qF '"mongoose"' package.json 2>/dev/null && echo "DB: document (inferred from mongoose)"
grep -qF 'lib/pq' go.mod 2>/dev/null && echo "DB: sql (inferred from lib/pq)"
```

Show the user what was detected before writing config:
```
Database detected:
  Type:  sql (from prisma/schema.prisma)
  Tool:  prisma (definitive)
```
or:
```
Database detected:
  Type:  sql (inferred from mongoose in package.json)
  Tool:  not detected
```

In Step 4 (config generation), populate the `artifacts:` block from detection results using the Config Contract key paths:
- Write `artifacts.database.default_type` always.
- Only write `artifacts.sql.migrations.tool` if detected type is `sql` or `mixed`. For `document` or `key-value`, omit the `sql:` block.
- If multiple DB types found, write `default_type: mixed` with a comment listing the detected types.
- If nothing detected, ask before writing:

```
Database setup (nothing detected from code):
  What database type will you use?
  1. SQL (Postgres, MySQL, SQLite, etc.)
  2. Document (MongoDB, DynamoDB, Firestore, etc.)
  3. Key-Value (Redis, DynamoDB as KV, etc.)
  4. Mixed (multiple types)
  5. Not decided yet → writes: auto
```

If user selects 1 (SQL) or 4 (Mixed), follow up with:
```
  Migration tool? (optional — press enter to skip → writes: ~)
```

If user selects 2 (Document), 3 (Key-Value), or 5 (Not decided yet), skip the migration tool question and omit `sql:` from the written config.

When complete, output: `PHASE 2 COMPLETE`

---

## Phase 3: spec-artifacts.md Redesign

**Objective:** Redesign spec-artifacts with a resolve-context preamble, lookup-table branching, full-body invariant extraction, and the "design blueprint" framing throughout.
**Model:** `sonnet`
**Max Iterations:** 4
**Completion Promise:** `PHASE 3 COMPLETE`
**Dependencies:** Phase 1

**Config Contract reminder:** Read `artifacts.database.default_type` for DB type. Read `artifacts.sql.migrations.tool` only when resolved DB type is `sql`. For `document` or `key-value`, skip migration artifact generation entirely — those databases have no SQL migrations.

**Design decisions:**

**DB type resolution — priority order (first match wins):**
1. Spec frontmatter `database_type:` field — explicit per-spec override (monorepo use case)
2. Config `artifacts.database.default_type` — if not `auto`, use it
3. Keyword scan spec content — last resort, warn user it was used
4. Still unresolved — ask the user before proceeding

**DB type keyword table (for step 3 above):**

| Spec mentions... | DB type |
|---|---|
| Postgres, MySQL, SQLite, MariaDB, SQL, relational, normalized, foreign key, JOIN | sql |
| MongoDB, Firestore, CouchDB, document store, collection, embedded document | document-mongo |
| DynamoDB, Cassandra, wide-column, HBase | document-dynamo |
| Redis, Memcached, ElastiCache, cache layer, session store, KV store | key-value |

**Invariant extraction — full body, no structure assumptions:**
Read each Active invariant file. Strip the YAML frontmatter (everything between the first `---` and second `---`). The remainder is the constraint body — take it verbatim. Do not try to find `## Rule` sections or first paragraphs. If the body is empty after stripping, emit a warning and skip that invariant:
```
⚠ INV-003 body is empty — constraint not injected. Review docs/architecture/invariants/INV-003-*.md
```

**Data model artifact lookup table:**

Single DB type — no suffix:

| DB type | File | Format description |
|---|---|---|
| sql | `data-model.mmd` | Mermaid erDiagram — entities, fields, relationships, `%% Index:` comments |
| document-mongo | `data-model.schema.yaml` | JSON Schema in YAML — `$schema`, collection, properties, required, indexes extension |
| document-dynamo | `data-model.md` | Access patterns table → entity prefixes → PK/SK/GSI design |
| key-value | `data-model.md` | Key schema table — key pattern, value type, TTL, purpose, namespace |

Mixed — suffix per type to avoid collision:

| DB type | File | Format description |
|---|---|---|
| sql | `data-model-sql.mmd` | Same as sql above |
| document-mongo | `data-model-mongo.schema.yaml` | Same as document-mongo above |
| document-dynamo | `data-model-dynamo.md` | Same as document-dynamo above |
| key-value | `data-model-kv.md` | Same as key-value above |

For mixed: detect all sub-types from spec keywords, generate one data model artifact per sub-type using suffix naming, list all generated files in the confirmation output.

Note: `data-model.md` is intentional for document-dynamo and key-value — those artifacts ARE design documents, not schema files. Metadata is embedded via format-appropriate comments (`%%` for .mmd, `#` for .yaml, `--` for .sql) — not YAML frontmatter, so tools parsing these files are unaffected.

**"Design blueprint" framing:**
Every generated artifact gets this comment header (format-appropriate: `%%` for .mmd, `#` for .yaml, `--` for .sql, HTML comment for .md):
```
Design blueprint — implement in your stack's native format.
This artifact defines intent, not implementation.
```

**Prompt:**

Read `commands/spec-artifacts.md` in full.

Restructure the command with these changes:

**1. Replace Step 2 with an expanded Step 2: Resolve Context.**

This step must complete fully before any artifact generation. Structure it as an explicit checklist Claude must work through and record:

```
### Step 2: Resolve Context

Work through this checklist before proceeding. Record each value explicitly.

**Spec validation** (existing — keep as-is):
- Read spec frontmatter, check status: accepted

**DB_TYPE** — check in priority order, use first match:
1. Spec frontmatter `database_type:` → if present, use it. Note source: spec-frontmatter.
2. Config `artifacts.database.default_type` → if not `auto`, use it. Note source: config.
3. Keyword scan spec content using the DB type keyword table → if matched, use it. Note source: keyword-scan.
4. If still unresolved → ask: "What database type does this feature use?" Note source: user.

**CONSTRAINTS** — load active invariants:
- Read all files in {invariants_dir}
- For each file where frontmatter `status: Active`:
  - Strip frontmatter (content between first and second `---`)
  - Take remaining body verbatim as constraint text
  - If body is empty → warn (see warning format above) and skip
- Build ACTIVE CONSTRAINTS block, or set to "none"

**State checkpoint — confirm before proceeding to artifact detection:**
- [ ] DB_TYPE = {one of: sql | document-mongo | document-dynamo | key-value | mixed}
       source: {spec-frontmatter | config | keyword-scan | user}
       if mixed: list all detected sub-types — these each get their own suffixed data model file
- [ ] CONSTRAINTS = {ACTIVE CONSTRAINTS block, or "none"}
- [ ] Spec status: accepted
```

**2. In Step 3 (artifact detection)**, update the data-model detection line to show the resolved file based on DB_TYPE:
```
✓ data-model.mmd — sql detected via {source} (Mermaid ERD)
```
or
```
✓ data-model.schema.yaml — document-mongo detected via keyword-scan (JSON Schema)
```
When keyword-scan was the source, add: `(⚠ detected from spec content — verify this is correct)`

**3. In Step 4 (generate artifacts)**, inject CONSTRAINTS into every agent prompt before the artifact instruction:
```
🪝 edikt: routing data-model.mmd to dba (2 active constraints applied)...
```
If CONSTRAINTS is "none", omit the count.

**4. In Step 5 (artifact templates)**, replace the single data-model template with the four-variant lookup table. Use the file names and format descriptions from the Data model artifact lookup table above. Each variant gets the "design blueprint" comment header.

**5. In Step 6 (confirmation)**, add after the file list:
```
These are design blueprints — implement them in your stack's native format.
The data model defines what exists and why. The API contract defines the interface.
The migration defines intent. Your code is the implementation.
```

**6. Find all references to `data-model.json-schema.yaml`** in the file and rename to `data-model.schema.yaml`.

When complete, output: `PHASE 3 COMPLETE`

---

## Phase 4: Test Harness Update

**Objective:** Extend the test harness with output contract assertions — given spec fixture inputs, assert expected file paths and content markers.
**Model:** `haiku`
**Max Iterations:** 2
**Completion Promise:** `PHASE 4 COMPLETE`
**Dependencies:** Phase 2, Phase 3

**Design decisions:**
- Tests are output contracts, not unit tests. The harness cannot introspect Claude's reasoning — it checks files exist at expected paths and contain expected content markers.
- Each test case needs a spec fixture file as input. Create minimal fixture specs in `test/fixtures/specs/` — one per DB type scenario.
- Tests assert file existence and key content, not exact output.

**Output contract format:**
```
GIVEN: {spec fixture with specific content}
WHEN:  spec-artifacts runs against it
THEN:  {file} exists at {path}
AND:   {file} contains {marker}
```

**Test cases to implement:**

1. **SQL path:**
   - Fixture: spec mentioning "Postgres"
   - Assert: `data-model.mmd` exists, contains `erDiagram`

2. **Document-mongo path:**
   - Fixture: spec mentioning "MongoDB"
   - Assert: `data-model.schema.yaml` exists, contains `$schema`

3. **Document-dynamo path:**
   - Fixture: spec mentioning "DynamoDB"
   - Assert: `data-model.md` exists, contains `Access Patterns`

4. **Key-value path:**
   - Fixture: spec mentioning "Redis"
   - Assert: `data-model.md` exists, contains key schema table markers

5. **Mixed path:**
   - Fixture: spec mentioning both "Postgres" and "Redis"
   - Assert: `data-model-sql.mmd` exists, contains `erDiagram`
   - Assert: `data-model-kv.md` exists, contains key schema table markers

6. **Config fallback:**
   - Fixture: spec with no DB keywords + config `default_type: sql`
   - Assert: `data-model.mmd` exists

7. **Config auto + no keywords:**
   - Fixture: spec with no DB keywords + config `default_type: auto`
   - Assert: command output contains a warning or prompt asking for DB type

8. **Active constraints injected:**
   - Fixture: one active invariant with non-empty body
   - Assert: routing output contains "active constraints applied"

9. **Empty invariant body warning:**
   - Fixture: one active invariant with empty body (only frontmatter)
   - Assert: output contains "body is empty"

10. **Superseded invariant excluded:**
    - Fixture: one invariant with `status: Superseded`
    - Assert: CONSTRAINTS block is "none" or constraint count is 0

11. **Spec-frontmatter override:**
    - Fixture: config `default_type: sql` + spec frontmatter `database_type: document-mongo`
    - Assert: `data-model.schema.yaml` exists (frontmatter wins over config)

12. **Design blueprint header present:**
    - Fixture: any accepted spec with data model trigger
    - Assert: generated artifact contains "Design blueprint" comment

Follow existing test file conventions (function names, assertion helpers, output format).

When complete, output: `PHASE 4 COMPLETE`
