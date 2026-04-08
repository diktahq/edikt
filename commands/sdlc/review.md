---
name: edikt:sdlc:review
description: "Post-implementation specialist review — routes to relevant domain agents based on what was built"
effort: high
context: fork
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
  - Agent
---

You are performing a post-implementation specialist review. Your job is to analyze what was built, detect which specialist domains are involved, and produce a consolidated review report.

CRITICAL: NEVER spawn agents for domains not detected from the changed files — only route to specialists where the file patterns match. ALWAYS output the routing announcement before spawning agents.

## Arguments

- `$ARGUMENTS` — optional scope (see Scope Definitions), `--no-edikt` for inline mode, `--json` for machine-readable output

## Instructions

0. If `--json` is in `$ARGUMENTS`, output only the JSON format at the end — no progress indicators, no emoji, no prose.

1. Check `$ARGUMENTS` for `--no-edikt`. If present, strip it and note inline mode (no agents).

2. Display progress: `Step 1/3: Detecting changed domains...`

3. Determine scope from `$ARGUMENTS` using the Scope Definitions in the Reference section. Run the appropriate git diff command to get the changed file list and full diff.

4. If no git is available or no changes detected, output: `No changes detected to review.` and stop.

5. Classify changed files by path and name patterns using the Domain Detection table in the Reference section. Only route to domains with at least one matching file.

6. If `--no-edikt` was passed: run each detected domain's review inline using the Reviewer Lenses in the Reference section. Do not spawn agents. Output results using the Output Format in the Reference section.

7. Display progress: `Step 2/3: Routing to specialist agents...`

8. Otherwise, output the routing announcement before spawning:
   ```
   🔀 edikt: routing to {agent-1}, {agent-2} (parallel)
   ```

9. Spawn all applicable specialist agents concurrently in a single message (multiple Agent tool calls). Each agent prompt must include: the git diff or relevant file contents, the specific review lens for that domain from the Reference section, and the expected output format with severities.

10. After specialist review, check if an active spec exists:
   ```bash
   ls {specs_dir}/SPEC-*/spec.md 2>/dev/null | head -1
   ```
   If a spec exists, run drift detection using `/edikt:sdlc:drift` logic with `--scope=spec` and append findings under a "DRIFT CHECK" section. If no spec exists, skip silently.

11. Display progress: `Step 3/3: Consolidating findings...`

12. Consolidate all agent findings and output using the Output Format in the Reference section.

## Reference

### JSON Output Format

```json
{
  "status": "findings",
  "domains": ["security", "dba"],
  "findings": [
    {"id": 1, "severity": "critical", "agent": "security", "text": "...", "file": "src/auth.go:23"}
  ],
  "summary": {"critical": 1, "warning": 3, "ok": 2}
}
```

### Scope Definitions

| Argument | Command | Description |
|---|---|---|
| (none) | `git diff HEAD~1` | Review last commit |
| `--staged` | `git diff --staged` | Review staged changes |
| `--branch` | `git diff main...HEAD` | Review full branch |
| A file path | Read that path | Review specific file or directory |

### Domain Detection Table

| File pattern | Domain | Agent |
|---|---|---|
| `*.sql`, `*migration*`, `*schema*` | database | `dba` |
| `docker-compose*`, `Dockerfile*`, `*.tf`, `k8s/*`, `helm/*` | infrastructure | `sre` |
| `*auth*`, `*jwt*`, `*oauth*`, `*payment*`, `*token*`, `*security*` | security | `security` |
| `*route*`, `*handler*`, `*controller*`, `*api*`, `*endpoint*`, `*webhook*` | api | `api` |
| `*architect*`, `*domain*`, `*bounded*` | architecture | `architect` |
| `*perf*`, `*benchmark*`, `*cache*`, `*optimize*` | performance | `performance` |

### Reviewer Lenses

**Principal DBA**
- Schema correctness and migration safety
- Query efficiency and N+1 risks
- Missing indexes on queried columns
- Transaction boundaries
- Missing rollback migrations

**Staff SRE**
- Deployment readiness and health checks
- Rollback capability
- Observability: logging and metrics coverage
- Resource limits defined

**Staff Security**
- OWASP Top 10 scan of changed code
- Hardcoded secrets or credentials
- Input validation gaps
- Auth gaps on new endpoints
- Exposed sensitive data

**Senior API**
- Contract stability and breaking changes
- Missing or outdated documentation
- Versioning strategy
- Response schema consistency

**Principal Architect**
- Bounded context violations
- Dependency direction correctness
- Pattern consistency with existing codebase
- Technical debt introduced

**Senior Performance**
- N+1 query patterns introduced
- Missing caching opportunities
- Algorithmic complexity concerns
- Benchmark-worthy hot paths

### Severity Model

- 🔴 Critical: must address before shipping (data loss, security breach, broken contract)
- 🟡 Warning: should address, not blocking
- 🟢 OK: domain looks healthy

**Agent header summary MUST list ALL non-zero severity counts.** If an agent has 1 critical and 1 warning, the header says `🔴 1 🟡 1` — never omit a level. The footer totals ALL 🔴 and 🟡 across all agents.

### Output Format

```
IMPLEMENTATION REVIEW — {date}
─────────────────────────────────────────────────────
Scope: {n} files changed ({scope description})
Domains: {detected domains}

{AGENT NAME}              {severity counts: 🔴 N 🟡 N 🟢 N — list ALL non-zero levels}
  #1 🔴  {finding — specific, actionable, with file reference}
  #2 🟡  {finding}
  #3 🟢  {area that looks good}

{AGENT NAME}              {severity counts}
  #4 🔴  {finding}
  #5 🟡  {finding}

─────────────────────────────────────────────────────
{total: N critical, N warnings — count ALL 🔴 and 🟡 across all agents}
{If critical: "Which findings should I address? (e.g., #1, #4 or 'all critical')"}
{If all clean: "✅ All domains clear — looks good to ship."}
{If all clean: "  Next: No issues found. Ship with confidence."}
```

If a relevant agent is not installed:
```
PRINCIPAL DBA — not installed (run /edikt:agents add dba)
```

### Drift Check Output Format

```
DRIFT CHECK — SPEC-005
  ✅ 6 requirements compliant
  ⚠️  1 diverged: agent memory on security missing
  Action: add memory:project to security.md
```
