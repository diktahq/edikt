# ADR-004: Advisor Orchestration Pattern

**Status:** Accepted
**Date:** 2026-03-08
**Deciders:** Daniel Gomes

## Context

edikt v2 introduced 17 specialist agents (architect, dba, sre, security, etc.) but they are entirely passive — users must manually invoke them. This creates the same problem edikt was built to solve: you have to *remember* to involve the right expert at the right time.

The vision is "Claude behaves like a senior engineer who's been on the team for months." A senior engineer knows when to pull in the DBA before a migration, knows to flag a security concern before it ships, knows who needs to review what. Passive agents don't deliver this.

edikt v3 introduces **proactive advisor orchestration**: agents are automatically routed to relevant tasks based on domain signal detection, without user invocation.

## Decision

### Advisor orchestration is in scope. Execution orchestration is not.

The distinction is precise and must be maintained:

**Advisors (in scope):**
- Read plans, code, and conversation
- Return findings, warnings, and recommendations
- Never write files, never modify state
- Run in isolated forked subagents (context: fork)
- Invoked automatically by edikt based on domain signals

**Executors (out of scope):**
- Write code, modify files, run commands
- Coordinate multi-agent parallel execution
- Manage worktrees or parallel contexts
- That is Claude Code's native responsibility

### Domain Signal Detection

edikt detects domain signals from task descriptions, plan content, and changed files:

| Signal Keywords | Agent Invoked |
|----------------|---------------|
| SQL, query, schema, migration, index, database | dba |
| deploy, docker, kubernetes, terraform, infra, CI, helm | sre |
| auth, JWT, OAuth, payment, PCI, HIPAA, token, secret | security |
| API, endpoint, REST, GraphQL, route, contract | api |
| bounded context, domain, architecture, refactor | architect |
| performance, N+1, cache, latency, throughput | performance |

### Three Advisor Moments

Proactive advisory fires at three moments in the workflow:

1. **Session start** — git-aware summary of what changed since last session, which domains are affected, which agents are relevant
2. **Pre-flight** — after `/edikt:plan` creates a plan, relevant specialists review it before execution begins
3. **Post-implementation** — `/edikt:review` routes to specialists based on what was actually built

### Severity Model

Advisor findings use a three-level severity model:
- 🔴 **Critical** — must be addressed before shipping (security vulnerability, data loss risk, broken contract)
- 🟡 **Warning** — should be addressed, not blocking (missing index, no rollback, test gap)
- 🟢 **OK** — domain looks healthy, notable positive patterns

## Alternatives Considered

**Full execution orchestration**
Auto-route tasks to agents that write code, not just review it. Rejected: violates edikt's "no execution orchestration" principle from ADR-001. Claude Code handles execution natively via worktrees and subagents.

**PostToolUse advisory hooks**
Fire advisor agents after each file write to catch domain issues mid-edit. Rejected: PostToolUse hooks can only run shell commands, not Claude agents. Would require nested `claude -p` calls — messy, slow, breaks session context.

**Manual invocation only (status quo)**
Keep agents passive, require users to invoke them. Rejected: creates the same "you have to remember" problem edikt was built to solve.

## Consequences

- Pre-flight review adds latency to `/edikt:plan` — acceptable because plan creation is not time-critical
- `/edikt:review` requires git diff — only meaningful after commits or with staged changes
- Domain signal detection is heuristic (keyword-based) — false positives possible, false negatives acceptable (better to over-route than miss a critical domain)
- Agent findings are advisory — users always decide whether to act on them
- Two new commands added: `/edikt:audit` (security), `/edikt:review` (post-implementation)
- Two enhanced workflows: `/edikt:plan` (pre-flight), session start hook (git-aware)
- One new command added: `/edikt:session` (end-of-session sweep)

## Directives

[edikt:directives:start]: #
paths:
  - "templates/agents/**"
  - "commands/review.md"
  - "commands/plan.md"
  - "commands/audit.md"
scope:
  - planning
  - design
  - review
directives:
  - Specialist agents are advisors only — they read, analyze, and return findings. They NEVER write files or modify state. (ref: ADR-004)
  - Agents run in forked subagents (`context: fork`). They do not coordinate parallel execution — that is Claude Code's responsibility. (ref: ADR-004)
  - Domain signal detection is keyword-based. Over-routing is acceptable; under-routing is not. (ref: ADR-004)
[edikt:directives:end]: #

## Related

- ADR-001: edikt Architecture (no execution orchestration)
- PLAN-006: edikt v3 — Proactive Intelligence
- PRD: Proactive Advisor System
