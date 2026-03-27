---
name: edikt:spec-artifacts
description: "Generate implementable artifacts from an accepted spec"
effort: high
---

# edikt:spec-artifacts

Generate implementable artifacts (data model, API contracts, migrations, test strategy) from an accepted technical specification. Each artifact is reviewed by the appropriate domain specialist.

CRITICAL: This command requires interactive input. If you are in plan mode (you can only describe actions, not perform them), output this and stop:
```
⚠️  This command requires user interaction and cannot run in plan mode.
Exit plan mode first, then run the command again.
```

## Arguments

- `$ARGUMENTS` — SPEC identifier (e.g., `SPEC-005`) or path to the spec folder

## Instructions

### 1. Resolve Paths

Read `.edikt/config.yaml`. Specs directory from `paths.specs` (default: `docs/product/specs/`). Invariants directory from `paths.invariants` (default: `docs/architecture/invariants/`).

### 2. Resolve Context

Work through this checklist before proceeding. Record each value explicitly.

**Spec validation** — find the spec folder and read `{spec_folder}/spec.md`. Check frontmatter `status:`:
- If `status: accepted` → proceed
- If `status: draft` → block:
  ```
  ⛔ SPEC-005 status is "draft".
     Specs must be accepted before generating artifacts.
     Review the spec and change status to "accepted" first.
  ```
- If no frontmatter status → treat as accepted (backwards compatibility)

**DB_TYPE** — check in priority order, use first match:
1. Spec frontmatter `database_type:` → if present, use it. Note source: `spec-frontmatter`.
2. Config `artifacts.database.default_type` → if not `auto`, use it. Note source: `config`.
3. Keyword scan spec content using the DB type keyword table below → if matched. Note source: `keyword-scan`.
4. Still unresolved → ask the user. Note source: `user`.

**Subtype resolution** — if DB_TYPE is `document` (generic, from config or user), resolve to a concrete subtype before proceeding:
- Keyword scan spec content for vendor names using the DB type keyword table below
- If MongoDB, Firestore, CouchDB, or "collection" found → `document-mongo`
- If DynamoDB, Cassandra, HBase, or "wide-column" found → `document-dynamo`
- If no vendor detected → ask: "Your config says document database — which type? (1) MongoDB/Firestore (2) DynamoDB/Cassandra"
- Note: config stores `document` (generic) because init detects from code signals, not spec content. Subtype resolution always happens at spec-artifacts runtime.

**CONSTRAINTS** — load active invariants:
- Read all files in `{invariants_dir}` (from config `paths.invariants`)
- For each file where frontmatter `status:` is `active` or `Active` (case-insensitive):
  - Strip frontmatter (content between first `---` and second `---`)
  - Take remaining body verbatim
  - If body empty → emit warning and skip:
    ```
    ⚠ INV-NNN body is empty — constraint not injected. Review docs/architecture/invariants/INV-NNN-*.md
    ```
- Build ACTIVE CONSTRAINTS block, or set to `none`

**State checkpoint — confirm before proceeding:**
```
- [ ] DB_TYPE = {one of: sql | document-mongo | document-dynamo | key-value | mixed}
       source: {spec-frontmatter | config | keyword-scan | user}
       if mixed: list all detected sub-types
- [ ] CONSTRAINTS = {ACTIVE CONSTRAINTS block text, or "none"}
- [ ] Spec status: accepted
```

### 3. Detect Relevant Artifacts

Scan the spec content for artifact triggers. Show the user what will be generated:

```
Based on SPEC-005, these artifacts are relevant:
```

Apply these detection rules:

| If the spec mentions... | Artifact | Primary agent | Secondary |
|---|---|---|---|
| database, model, schema, entity, table, column, field, relationship | *(see data model lookup table)* | dba | architect |
| API, endpoint, route, REST, GraphQL, request, response, contract | `contracts/api.yaml` | api | architect |
| gRPC, protobuf, proto, service definition | `contracts/proto/` | api | engineer |
| migration, schema change, ALTER, CREATE TABLE | `migrations/` *(sql or mixed only)* | dba | sre |
| event, message, queue, Kafka, RabbitMQ, pub/sub, webhook delivery | `contracts/events.yaml` | architect | sre |
| test, testing strategy, coverage, unit test, integration test | `test-strategy.md` | qa | engineer |
| config, environment variable, feature flag, configuration | `config-spec.md` | sre | engineer |
| seed, fixture, test data, sample data, development data (also auto-triggers when data-model is generated) | `fixtures.yaml` | qa | dba |

**Data model file — resolve from DB_TYPE:**

Single DB type (no suffix):

| DB_TYPE | File | Format |
|---|---|---|
| sql | `data-model.mmd` | Mermaid erDiagram |
| document-mongo | `data-model.schema.yaml` | JSON Schema in YAML |
| document-dynamo | `data-model.md` | Access patterns + PK/SK/GSI design |
| key-value | `data-model.md` | Key schema table |

Mixed (suffix per sub-type to avoid collision):

| Sub-type | File |
|---|---|
| sql | `data-model-sql.mmd` |
| document-mongo | `data-model-mongo.schema.yaml` |
| document-dynamo | `data-model-dynamo.md` |
| key-value | `data-model-kv.md` |

**migrations/ is only generated when DB_TYPE is `sql` or `mixed`. Skip it entirely for `document` and `key-value`.**

Show the list with checkmarks. Include DB_TYPE source warning when applicable:

```
  ✓ data-model.mmd — sql detected via config (Mermaid ERD)
  ✓ contracts/api.yaml — API endpoint references
  ✓ test-strategy.md — testing strategy section
  ✗ migrations/ — no schema changes
  ✗ contracts/events.yaml — no messaging patterns
  ✗ config-spec.md — no config changes
```

If DB_TYPE was resolved via keyword-scan, add a warning beneath the data model line:
```
   ⚠ detected from spec content — verify this is correct
```

Wait for confirmation before proceeding.

### 4. Generate Artifacts

For each confirmed artifact, route to the primary + secondary agent via the Agent tool.

Each agent receives:
- The spec content
- The source PRD content (read from `source_prd:` in spec frontmatter)
- The project-context.md for project context
- Any referenced ADRs
- The ACTIVE CONSTRAINTS block (if CONSTRAINTS is not `none`) injected before the artifact-specific instruction, in this format:

```
ACTIVE CONSTRAINTS (from governance — these override artifact defaults):
- [INV-001] {full invariant body verbatim}
- [INV-003] {full invariant body verbatim}
```

- Instruction to produce the specific artifact type

Show routing as it happens. Include constraint count when CONSTRAINTS is not `none`:
```
🪝 edikt: routing data-model.mmd to dba (2 active constraints applied)...
🪝 edikt: routing contracts/api.yaml to api...
```

If CONSTRAINTS is `none`, omit the constraint count entirely.

### 5. Artifact Templates

Each artifact lives in the spec's folder. Use native formats — not markdown wrappers.

Each generated artifact gets a design blueprint header. Native-format artifacts (`.mmd`, `.yaml`, `.sql`) use format-appropriate comments. Markdown artifacts (`test-strategy.md`, `config-spec.md`) use YAML frontmatter instead:

**Comment-header artifacts** (`.mmd`, `.yaml`, `.sql`, non-frontmatter `.md`):
- `.mmd`: `%% Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation.`
- `.yaml`: `# Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation.`
- `.sql`: `-- Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation.`
- `.md` (data model): `<!-- Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation. -->`

**Frontmatter artifacts** (`test-strategy.md`, `config-spec.md`): use YAML frontmatter with `type: artifact`, `artifact_type:`, `status: draft`, `reviewed_by:`.

**data-model.mmd** — Mermaid ERD (when DB_TYPE is `sql`):
```
%% Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation.
%% edikt:artifact type=data-model spec=SPEC-{NNN} status=draft reviewed_by=dba
%% created_at={ISO8601 timestamp}
erDiagram
    {ENTITY} {
        uuid id PK
        {type} {field} "{constraints}"
    }
    {ENTITY} ||--o{ {OTHER_ENTITY} : "{relationship}"
```

Include one entity block per entity. Add `PK`, `FK`, `UK` markers on key fields. List all relationships with cardinality. Add `%% Index: {field} — {rationale}` comments for recommended indexes.

**data-model.schema.yaml** — JSON Schema in YAML (when DB_TYPE is `document-mongo`):
```yaml
# Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation.
# edikt:artifact type=data-model spec=SPEC-{NNN} status=draft reviewed_by=dba
# created_at={ISO8601 timestamp}
$schema: "https://json-schema.org/draft/07/schema#"
collection: "{collection_name}"
title: "{EntityName}"
type: object
required:
  - {required_field}
properties:
  _id:
    type: string
    description: "MongoDB ObjectId or custom shard key"
  {field}:
    type: {type}
    description: "{description}"
indexes:
  - fields: [{field}]
    unique: {true|false}
    reason: "{why this index exists}"
```

**data-model.md** — Access pattern design (when DB_TYPE is `document-dynamo`):
```markdown
<!-- Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation. -->
<!-- edikt:artifact type=data-model spec=SPEC-{NNN} status=draft reviewed_by=dba -->
<!-- created_at={ISO8601 timestamp} -->

# Data Model — {Feature Name}

## Access Patterns

| Pattern | PK | SK | Index | Notes |
|---|---|---|---|---|
| {description} | {PK value} | {SK value} | table | {notes} |
| {description} | {PK value} | {SK value} | GSI1 | {notes} |

## Entity Prefixes

| Entity | PK prefix | SK prefix |
|---|---|---|
| {EntityName} | `{PREFIX}#` | `{PREFIX}#` |

## Key Design

| Key | Pattern | Example |
|---|---|---|
| PK | `{ENTITY}#{id}` | `USER#abc123` |
| SK | `{ENTITY}#{field}` | `USER#abc123` |
| GSI1-PK | `{field}` | `{value}` |
| GSI1-SK | `{field}` | `{value}` |
```

**data-model.md** — Key schema (when DB_TYPE is `key-value`):
```markdown
<!-- Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation. -->
<!-- edikt:artifact type=data-model spec=SPEC-{NNN} status=draft reviewed_by=dba -->
<!-- created_at={ISO8601 timestamp} -->

# Data Model — {Feature Name}

## Key Schema

| Key pattern | Value type | TTL | Purpose | Namespace |
|---|---|---|---|---|
| `{namespace}:{id}` | JSON object | {seconds or none} | {description} | `{ns}` |
| `{namespace}:{id}:lock` | string | 30s | distributed lock | `{ns}` |

## Notes

- Key separator: `:`
- Namespace rationale: {why these namespaces}
- Eviction policy: {LRU / LFU / noeviction / etc}
```

For `mixed` DB_TYPE, generate one data model artifact per detected sub-type using the suffix naming convention (`data-model-sql.mmd`, `data-model-mongo.schema.yaml`, `data-model-dynamo.md`, `data-model-kv.md`).

**contracts/api.yaml** — OpenAPI 3.0:
```yaml
# Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation.
# edikt:artifact type=api-contract spec=SPEC-{NNN} status=draft reviewed_by=api
# created_at={ISO8601 timestamp}
openapi: "3.0.0"
info:
  title: "{Feature Name} API"
  version: "0.1.0"
paths:
  /{resource}:
    {method}:
      summary: "{what this endpoint does}"
      operationId: {camelCaseId}
      security:
        - {authScheme}: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [{required_fields}]
              properties:
                {field}:
                  type: {type}
                  description: "{description}"
      responses:
        "200":
          description: "{success description}"
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/{ResponseType}"
        "400":
          description: "{bad request condition}"
        "404":
          description: "{not found condition}"
components:
  schemas:
    {TypeName}:
      type: object
      properties:
        {field}:
          type: {type}
  securitySchemes:
    {authScheme}:
      type: {http|apiKey|oauth2}
```

**contracts/events.yaml** — AsyncAPI 2.6:
```yaml
# Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation.
# edikt:artifact type=event-contract spec=SPEC-{NNN} status=draft reviewed_by=architect
# created_at={ISO8601 timestamp}
asyncapi: "2.6.0"
info:
  title: "{Feature Name} Events"
  version: "0.1.0"
channels:
  {topic.or.queue.name}:
    publish:
      operationId: {EventName}Published
      summary: "{what triggers this event}"
      message:
        name: {EventName}
        payload:
          type: object
          required: [{required_fields}]
          properties:
            {field}:
              type: {type}
              description: "{description}"
    x-producer: "{service/component}"
    x-consumers:
      - "{service/component}"
    x-ordering: "{ordering guarantees or FIFO/UNORDERED}"
    x-idempotency: "{idempotency strategy}"
```

**migrations/** — numbered SQL files, one per logical change (sql and mixed only):
```sql
-- Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation.
-- edikt:artifact type=migration spec=SPEC-{NNN} status=draft reviewed_by=dba
-- created_at={ISO8601 timestamp}
-- migration: 001_{descriptive_name}
-- description: {what this migration does}

-- === UP ===
{SQL DDL statements}

-- === DOWN ===
{SQL rollback statements — required}

-- === BACKFILL ===
-- {data transformation SQL, or: none required}

-- === RISK ===
-- Lock duration: {estimated}
-- Data volume: {estimated rows affected}
-- Deployment notes: {zero-downtime considerations}
```

Name files `migrations/001_{descriptive_name}.sql`. Increment the number for each subsequent migration.

**test-strategy.md** — design document (stays markdown):
```markdown
---
type: artifact
artifact_type: test-strategy
spec: SPEC-{NNN}
status: draft
created_at: {ISO8601 timestamp}
reviewed_by: qa
---

# Test Strategy — {feature name}

## Unit Tests

| Component | What to test | Priority |
|---|---|---|
| {component} | {behavior} | {high/medium/low} |

## Integration Tests

| Scenario | Components involved | Priority |
|---|---|---|
| {scenario} | {components} | {high/medium/low} |

## Edge Cases

{Edge cases identified from the spec and PRD acceptance criteria}

## Coverage Target

{What coverage looks like for this feature}
```

**config-spec.md** — design document (stays markdown):
```markdown
---
type: artifact
artifact_type: config-spec
spec: SPEC-{NNN}
status: draft
created_at: {ISO8601 timestamp}
reviewed_by: sre
---

# Configuration Spec — {feature name}

## Environment Variables

| Variable | Type | Default | Required | Description |
|---|---|---|---|---|
| {VAR_NAME} | {type} | {default} | {yes/no} | {description} |

## Feature Flags

| Flag | Default | Description | Rollout plan |
|---|---|---|---|
| {flag_name} | {default} | {description} | {plan} |
```

**fixtures.yaml** — seed data in portable YAML (stack-agnostic, transform to SQL/Prisma/factory at implementation time):
```yaml
# Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation.
# edikt:artifact type=fixtures spec=SPEC-{NNN} status=draft reviewed_by=qa
# created_at={ISO8601 timestamp}
# Transform to your stack's format at implementation time:
# SQL seeds, Prisma seed.ts, factory definitions, pytest fixtures, etc.

scenarios:
  - name: "{scenario name}"
    purpose: "{what this enables: dev env | integration tests | demo}"
    entities:
      - entity: {EntityName}
        records:
          - id: {uuid or placeholder}
            {field}: {value}
            {field}: {value}
            _note: "{why this specific data matters}"
          - id: {uuid or placeholder}
            {field}: {value}
            _note: "{edge case or behavior this covers}"

  - name: "{edge case scenario}"
    purpose: "edge case coverage"
    entities:
      - entity: {EntityName}
        records:
          - id: {uuid or placeholder}
            {field}: {boundary_value}
            _note: "{e.g. max-length string for validation testing}"

relationships:
  - "{EntityA}.{fk_field} references {EntityB}.id"
  - "Create order: {EntityA} before {EntityB}"
```

---

REMEMBER: Every artifact must be reviewed by the appropriate specialist agent. NEVER generate an artifact without routing it to the agent listed in the detection table. The spec must be accepted before artifacts are generated.

### 6. Confirm

```
✅ Artifacts created in {spec_folder}/

  {data-model file}         — {format}, reviewed by dba
  contracts/api.yaml        — OpenAPI 3.0, reviewed by api
  contracts/events.yaml     — AsyncAPI 2.6, reviewed by architect
  migrations/001_{name}.sql — SQL migration, reviewed by dba
  fixtures.yaml             — seed data, reviewed by qa
  test-strategy.md          — test design, reviewed by qa

  Status: draft
  Review and accept each artifact before planning.
  To accept: change status=draft to status=accepted in the comment header,
  or status: draft to status: accepted in frontmatter artifacts.
  Run /edikt:plan to create an execution plan.
```

These are design blueprints — implement them in your stack's native format.
The data model defines what exists and why. The API contract defines the interface.
The migration defines intent. Your code is the implementation.

---

## Reference Tables

### DB Type Keyword Table

Use this for priority-3 keyword scan. First match wins per DB type — if multiple types match, resolved DB_TYPE is `mixed`.

| Spec mentions... | DB type |
|---|---|
| Postgres, MySQL, SQLite, MariaDB, SQL, relational, normalized, foreign key, JOIN | sql |
| MongoDB, Firestore, CouchDB, document store, collection, embedded document | document-mongo |
| DynamoDB, Cassandra, wide-column, HBase | document-dynamo |
| Redis, Memcached, ElastiCache, cache layer, session store, KV store | key-value |

### Data Model Artifact Lookup Table

**Single DB type — no suffix:**

| DB_TYPE | File | Format |
|---|---|---|
| sql | `data-model.mmd` | Mermaid erDiagram — entities, fields, relationships, `%% Index:` comments |
| document-mongo | `data-model.schema.yaml` | JSON Schema in YAML — `$schema`, collection, properties, required, indexes |
| document-dynamo | `data-model.md` | Access patterns table → entity prefixes → PK/SK/GSI design |
| key-value | `data-model.md` | Key schema table — key pattern, value type, TTL, purpose, namespace |

**Mixed — suffix per sub-type to avoid collision:**

| Sub-type | File |
|---|---|
| sql | `data-model-sql.mmd` |
| document-mongo | `data-model-mongo.schema.yaml` |
| document-dynamo | `data-model-dynamo.md` |
| key-value | `data-model-kv.md` |

### Migration Generation Rules

| DB_TYPE | Generate migrations/ |
|---|---|
| sql | yes |
| mixed | yes (sql sub-type only) |
| document-mongo | no |
| document-dynamo | no |
| key-value | no |
