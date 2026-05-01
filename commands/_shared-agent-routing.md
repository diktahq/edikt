---
name: _shared-agent-routing
description: "Shared domain signal detection and specialist spawning procedure — referenced by plan, review, drift"
tier: 1
---

# Shared Agent Routing

This document defines the domain signal table and specialist spawning procedure used by `/edikt:sdlc:plan`, `/edikt:sdlc:review`, and `/edikt:sdlc:drift` for pre-flight specialist review. Reference it instead of duplicating the routing logic.

## Domain Signal Table

| Domain | Signals | Agent subagent_type |
|--------|---------|---------------------|
| Database | SQL, query, schema, migration, index, database, db, table, foreign key, join, transaction, ORM, Postgres, MySQL, SQLite, MongoDB | `dba` |
| Infrastructure | deploy, docker, kubernetes, k8s, terraform, helm, CI, CD, infra, container, Dockerfile, compose, nginx, AWS, GCP, Azure, cloud | `sre` |
| Security | auth, JWT, OAuth, payment, PCI, HIPAA, token, secret, encrypt, credential, password, permission, role, RBAC, CORS, XSS, injection | `security` |
| API | API, endpoint, REST, GraphQL, route, webhook, contract, openapi, swagger, versioning, breaking change | `api` |
| Architecture | bounded context, domain, architecture, refactor, pattern, layer, dependency, coupling, abstraction, interface, hexagonal, clean arch | `architect` |
| Performance | performance, N+1, cache, latency, throughput, slow, optimize, index, query optimization, benchmark | `performance` |

## detect_signals(text) procedure

Scan `text` (all phase prompts + objectives + titles) for domain keywords from the table above.

Return a list of detected domains.

If no domains detected, output:
```
Pre-flight: no specialist domains detected — plan looks self-contained.
```
and stop.

## spawn_specialists(signals, context) procedure

For each detected domain, use the Agent tool to spawn the matching specialist concurrently (single message, multiple Agent tool calls):

- database → `subagent_type: "dba"`
- infrastructure → `subagent_type: "sre"`
- security → `subagent_type: "security"`
- api → `subagent_type: "api"`
- architecture → `subagent_type: "architect"`
- performance → `subagent_type: "performance"`

Each specialist:
1. Reads the task/plan/review content
2. Reviews from their domain lens ONLY
3. Returns findings with severity:
   - 🔴 Critical: must address before execution (data loss, security breach, broken contract)
   - 🟡 Warning: should address, not blocking
   - 🟢 OK: domain looks healthy

## consolidate_findings(results[]) procedure

Output the consolidated pre-flight review using this format:

```
PRE-FLIGHT REVIEW
─────────────────────────────────────────────────────
Domains detected: {list} ({n} of 6 checked)

{AGENT NAME}
  🔴  {finding}
  🟡  {finding}
  🟢  {positive finding}

─────────────────────────────────────────────────────
{N critical, N warnings}. Address before executing?
Type your updates now, or press enter to proceed with known risks noted.
```

If the user provides updates → incorporate them.
If the user skips → add a `## Known Risks` section to the artifact listing the outstanding findings.
