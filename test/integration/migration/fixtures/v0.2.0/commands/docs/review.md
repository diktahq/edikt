---
name: edikt:docs:review
description: "Audit documentation gaps — find what changed in code but wasn't reflected in docs"
effort: high
context: fork
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
---

# /edikt:docs:review

Audit documentation gaps — find what changed in code but wasn't reflected in docs.

## Usage

```
/edikt:docs
/edikt:docs:review audit
/edikt:docs:review audit api
/edikt:docs:review audit readme
/edikt:docs:review audit infra
```

## What it does

edikt watches for doc gaps passively (via the Stop hook) and lets you audit them on demand.

The Stop hook flags specific gaps inline when Claude writes code — never vague, always named:

```
📄 Doc gap: POST /webhooks may need docs — run `/edikt:docs:review` to review.
```

Run `/edikt:docs:review` to act on them.

## No argument — show pending gaps

```
/edikt:docs
```

Scans recent changes (git diff since last commit) for public surface changes without corresponding doc updates:
- New routes without API doc entries
- New env vars without README entries
- New services without infrastructure docs
- Removed routes still in docs (stale)

Output:
```
Doc gaps found (3):

  Missing:
  • POST /webhooks        → not in docs/api.md
  • DATABASE_POOL_SIZE    → not in README (env vars section)
  • redis                 → added to docker-compose, not in docs/infrastructure.md

  Stale:
  • GET /v1/users/:id     → removed from routes but still in docs/api.md

Run /edikt:docs:review audit to fix all, or address them individually.
```

## `audit` — full sweep with fixes

```
/edikt:docs:review audit
```

Walks through each gap interactively. For each finding, Claude drafts the missing doc section and asks for confirmation before writing.

### Scope modifiers

```
/edikt:docs:review audit api     — API routes vs API docs only
/edikt:docs:review audit readme  — env vars, install steps, service deps vs README
/edikt:docs:review audit infra   — docker-compose, k8s, queues vs infrastructure docs
```

## Instructions

### 1. Find doc files

Look for:
- `README.md`, `README.rst`, `docs/README.md`
- `docs/api.md`, `docs/api/`, `openapi.yaml`, `swagger.json`
- `docs/infrastructure.md`, `docs/architecture.md`
- `.env.example`, `docs/configuration.md`

### 2. Find public surface

Grep for:
- HTTP routes: `router.`, `app.get`, `app.post`, `http.HandleFunc`, `@Get`, `@Post`, `Route(`, `@app.route`
- Env vars: `os.Getenv`, `process.env.`, `ENV[`, `os.environ`
- CLI flags: `flag.String`, `cobra.Command`, `argparse`, `click`
- New services: `docker-compose.yml` services, Kubernetes manifests, queue definitions

### 3. Compare

For each item on the public surface, check if it appears in relevant docs. Flag:
- **Missing**: in code, not in docs
- **Stale**: in docs, not in code (removed)
- **Outdated**: in both, but the doc description doesn't match the current implementation

### 4. Report and fix

Report all findings grouped by type. For `audit` mode, draft the missing content and confirm before writing:

```
Missing: POST /webhooks (routes/webhooks.go:42)

Suggested addition to docs/api.md:

### POST /webhooks

Register a webhook endpoint.

**Request:**
```json
{
  "url": "string",
  "events": ["string"]
}
```

**Response:** 201 Created

Add this to docs/api.md? [y/n/skip]
```

### 5. What NOT to flag

- Internal functions, unexported identifiers, private routes
- Test files and test helpers
- Refactors that don't change public behavior
- Dependency version bumps (unless public API changed)
- Comments and formatting changes

### 6. Completion Signal

After reporting all findings:

```
  Next: Fix the gaps above, or run /edikt:docs:review audit to auto-fix all.
```

## Natural language triggers

- "check the docs"
- "what's not documented?"
- "audit api docs"
- "are the docs up to date?"
- "docs audit"
