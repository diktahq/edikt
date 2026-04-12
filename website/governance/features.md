---
title: "Configurable Features"
description: "Toggle edikt's optional behaviors — auto-format, session summaries, signal detection, plan injection, and quality gates."
---

# Configurable Features

edikt's governance core — rules, compiled directives, drift detection, review-governance — is always on. That's the product.

Five optional behaviors run on top. Each can be turned off in `.edikt/config.yaml` for teams that want a lighter touch or have their own tooling for that concern.

## The features

```yaml
# .edikt/config.yaml
features:
  auto-format: true        # format files after every edit
  session-summary: true    # git-aware "since your last session" on start
  signal-detection: true   # detect ADR/invariant candidates on stop
  plan-injection: true     # inject active plan phase on every prompt
  quality-gates: true      # block on critical findings from gate agents
```

All default to `true`. Set any to `false` to disable.

### auto-format

Runs the appropriate formatter (gofmt, prettier, black, rubocop, rustfmt) after every file edit. Disable if your CI handles formatting or your team uses a different formatter setup.

```yaml
features:
  auto-format: false
```

Environment variable override: `EDIKT_FORMAT_SKIP=1`

### session-summary

Shows what changed since your last session when you open the project — modified files, relevant agents, active plan phase. Disable if you prefer a clean start with no preamble.

```yaml
features:
  session-summary: false
```

### signal-detection

After every Claude response, scans for uncaptured architectural decisions and suggests `/edikt:adr:new` or `/edikt:invariant:new`. Disable if the suggestions feel noisy or your team captures decisions through a different process.

```yaml
features:
  signal-detection: false
```

### plan-injection

Injects the active plan's current phase into every prompt so Claude always knows where it is in the execution plan. Disable if you're not using plans or prefer to load context manually.

```yaml
features:
  plan-injection: false
```

### quality-gates

When a specialist agent configured as a gate (e.g., `security`) finds a critical issue, it blocks progression until the finding is resolved or explicitly overridden. Disable if you want agents to advise without blocking.

```yaml
features:
  quality-gates: false
```

## Evaluator

The evaluator validates acceptance criteria at two points: pre-flight (before a phase starts) and phase-end (after completion). Both are configurable:

```yaml
evaluator:
  preflight: true          # pre-flight criteria validation
  phase-end: true          # phase-end evaluation
  mode: headless           # headless | subagent
  max-attempts: 5          # max retries before stuck
  model: sonnet            # model for headless evaluator
```

| Key | Default | What it controls |
|-----|---------|-----------------|
| `preflight` | `true` | Validates criteria are testable before the generator starts |
| `phase-end` | `true` | Verifies completed work meets acceptance criteria |
| `mode` | `headless` | `headless` runs a separate `claude -p` (zero shared context). `subagent` runs within the session. |
| `max-attempts` | `5` | Max phase retries before marking as stuck |
| `model` | `sonnet` | Model used for headless evaluator invocation |

When both `preflight` and `phase-end` are `false`, the evaluator is disabled. The criteria sidecar is still emitted.

See [Evaluator](/governance/evaluator) for the full comparison of headless vs subagent mode.

## What's always on

These are not configurable — they're the governance core:

| Feature | Why it's always on |
|---------|-------------------|
| **Rule loading** | Rules in `.claude/rules/` load automatically — this is Claude Code's behavior, not edikt's |
| **Compiled directives** | `/edikt:gov:compile` output loads as a rule file — same mechanism |
| **Drift detection** | `/edikt:sdlc:drift` is a command you run explicitly, not a background behavior |
| **Review-governance** | `/edikt:gov:review` is a command you run explicitly |
| **PreToolUse check** | Warns if `docs/project-context.md` is missing — a safety net, not a behavior toggle |
| **Context recovery** | PreCompact + PostCompact preserve plan state across compaction — disabling this would lose data |

## Event logging

edikt writes a structured event log to `~/.edikt/events.jsonl`. This is always on and not configurable — it's the audit trail.

Events logged:
- Quality gate firings and overrides (with git identity of the approver)
- Invariant violations detected by the pre-push hook
- Status changes on governance artifacts (PRD accepted, spec created, etc.)

Each entry is a JSON line with an ISO 8601 timestamp, event type, and relevant context. The file lives at the machine level (not committed to git) and is used by `/edikt:status` to show gate and agent activity for the current session.

## Checking feature status

```bash
/edikt:doctor
```

Doctor reports which features are enabled and which are disabled.

## For teams

Feature settings are in `.edikt/config.yaml` which is committed to git. The whole team shares the same configuration. If your team disables signal-detection, everyone gets a quiet stop hook.
