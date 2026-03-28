# /edikt:spec-artifacts

Generate implementable design artifacts from an accepted technical specification. Each artifact is treated as a **design blueprint** — it defines intent and structure, not implementation. Your code, migrations, and schema files are the implementation.

## Usage

```bash
/edikt:spec-artifacts SPEC-005
/edikt:spec-artifacts path/to/spec-folder/
```

Pass a SPEC identifier (e.g., `SPEC-005`) or the path to the spec folder.

## Gate

The spec must have `status: accepted` before artifacts can be generated:

```text
⛔ SPEC-005 status is "draft".
   Specs must be accepted before generating artifacts.
   Review the spec and change status to "accepted" first.
```

## How context is resolved

Before generating anything, the command works through a checklist and records its state explicitly:

**Database type** — resolved in priority order (first match wins):

1. Spec frontmatter `database_type:` — per-spec override, useful in monorepos
2. Config `artifacts.database.default_type` — set by `/edikt:init` from code signals
3. Keyword scan of spec content — last resort; warns you when used
4. Still unresolved — asks you directly

**Active invariants** — loaded from your governance chain. For each `status: Active` invariant, the body is stripped of frontmatter and injected as a structured constraint into every artifact agent prompt. If an invariant body is empty, you get a warning:

```text
⚠ INV-003 body is empty — constraint not injected. Review docs/architecture/invariants/INV-003-*.md
```

The command outputs a state checkpoint before proceeding so you can verify DB type and constraint count before generation begins.

## What gets generated

The command scans the spec and shows what it will generate:

```text
Based on SPEC-005, these artifacts are relevant:
  ✓ data-model.mmd        — sql detected via config (Mermaid ERD)
  ✓ contracts/api.yaml    — API endpoint references
  ✓ test-strategy.md      — testing strategy section
  ✗ migrations/           — no schema changes
  ✗ contracts/events.yaml — no messaging patterns
  ✗ config-spec.md        — no config changes

Generate 3 artifacts? (y/n)
```

Detection is automatic. Confirm or adjust before generation begins. Migrations are only generated for SQL and mixed database types — document and key-value stores never produce migration files.

## Database-aware data model

The data model artifact format depends on your resolved database type:

| DB type | File | Format |
|---------|------|--------|
| SQL (Postgres, MySQL, SQLite) | `data-model.mmd` | Mermaid ERD — entities, fields, relationships, index comments |
| MongoDB, Firestore, CouchDB | `data-model.schema.yaml` | JSON Schema in YAML — collection, properties, required, indexes |
| DynamoDB, Cassandra, HBase | `data-model.md` | Access patterns table → entity prefixes → PK/SK/GSI design |
| Redis, Memcached, ElastiCache | `data-model.md` | Key schema table — key pattern, value type, TTL, purpose |

For projects using multiple database types (e.g., Postgres + Redis), both artifacts are generated with suffixes to avoid collision:

| Sub-type | File |
|---------|------|
| SQL | `data-model-sql.mmd` |
| MongoDB | `data-model-mongo.schema.yaml` |
| DynamoDB | `data-model-dynamo.md` |
| Redis / KV | `data-model-kv.md` |

## All artifact types

| Artifact | Triggered by | Reviewed by |
|----------|-------------|-------------|
| *(see above)* | database, model, schema, entity, table, column, field, relationship | dba |
| `contracts/api.yaml` | API, endpoint, route, REST, GraphQL, request, response, contract | api |
| `contracts/proto/` | gRPC, protobuf, service definition | api |
| `migrations/` *(SQL and mixed only)* | migration, schema change, ALTER, CREATE TABLE | dba |
| `contracts/events.yaml` | event, queue, Kafka, RabbitMQ, pub/sub, webhook delivery | architect |
| `test-strategy.md` | test, testing strategy, coverage, unit test, integration test | qa |
| `config-spec.md` | config, environment variable, feature flag, configuration | sre |
| `fixtures.yaml` | seed, fixture, test data, sample data (also auto-triggers with data model) | qa |

API contracts are OpenAPI 3.0 YAML. Event contracts are AsyncAPI 2.6 YAML. Fixtures are portable YAML — transform to your stack (SQL seeds, Prisma seed.ts, factory definitions) at implementation time.

## Invariant injection

When active invariants exist, every agent prompt includes a structured constraint block:

```text
🪝 edikt: routing data-model.mmd to dba (2 active constraints applied)...
```

The constraints are pulled verbatim from your invariant bodies and injected before the artifact instruction. Superseded invariants are excluded automatically.

## Output location

All artifacts live in the spec's folder:

```text
docs/product/specs/SPEC-005-webhook-delivery/
├── spec.md
├── data-model.mmd           ← or .schema.yaml / .md depending on DB type
├── contracts/
│   ├── api.yaml
│   └── events.yaml
├── migrations/
│   └── 001_create_webhooks.sql
├── fixtures.yaml
└── test-strategy.md
```

Each artifact includes a design blueprint header comment in format-appropriate syntax (`%%` for `.mmd`, `#` for `.yaml`, `--` for `.sql`, HTML comment for `.md`):

```text
Design blueprint — implement in your stack's native format.
This artifact defines intent, not implementation.
```

## Config reference

Database type is detected and written by `/edikt:init` from code signals. You can also set it manually in `.edikt/config.yaml`:

```yaml
artifacts:
  database:
    default_type: sql   # sql | document | key-value | mixed | auto
  sql:
    migrations:
      tool: golang-migrate  # golang-migrate | prisma | alembic | django | rails | etc.
  fixtures:
    format: yaml
```text

`auto` means the command detects from spec content each time. Use it for greenfield projects or monorepos where each spec sets its own `database_type:` in frontmatter.

## After generating

Review each artifact. Accept them individually by changing `status: draft` to `status: accepted` in the frontmatter. All artifacts must be accepted before running `/edikt:plan`.

```
These are design blueprints — implement them in your stack's native format.
The data model defines what exists and why. The API contract defines the interface.
The migration defines intent. Your code is the implementation.
```

## What's next

- [/edikt:plan](/commands/plan) — generate phased execution plan with pre-flight specialist review
- [/edikt:drift](/commands/drift) — verify implementation against these artifacts after building
- [Governance Chain](/governance/chain) — full chain overview
