---
title: "Configurable Features"
description: "Toggle edikt's optional behaviors — auto-format, session summaries, signal detection, plan injection, and quality gates."
---

# Configurable Features

edikt's governance core — rules, compiled directives, drift detection, review-governance — is always on. That's the product.

Six optional behaviors run on top. Five are on/off flags; the sixth, `evidence-gate`, is a four-value posture dial rather than a boolean. Each can be turned off (or set to its most permissive value) in `.edikt/config.yaml` for teams that want a lighter touch or have their own tooling for that concern.

## The features

```yaml
# .edikt/config.yaml
features:
  auto-format: true        # format files after every edit
  session-summary: true    # git-aware "since your last session" on start
  signal-detection: true   # detect ADR/invariant candidates on stop
  plan-injection: true     # inject active plan phase on every prompt
  quality-gates: true      # block on critical findings from gate agents
  evidence-gate: warn      # posture for the pre-edit verify-gate hook: block | warn | educate | disabled
```

The five boolean features default to `true`; `evidence-gate` defaults to `warn`. Set a boolean to `false` to disable it, or set `evidence-gate` to one of its other three postures.

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

After every response, scans for uncaptured architectural decisions and suggests `/edikt:adr:new` or `/edikt:invariant:new`. Disable if the suggestions feel noisy or your team captures decisions through a different process.

```yaml
features:
  signal-detection: false
```

### plan-injection

Injects the active plan's current phase into every prompt so the model always knows where it is in the execution plan. Disable if you're not using plans or prefer to load context manually.

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

Setting `quality-gates: false` also disables the **stop-hook completion-claim detector** (v0.6.0). When enabled (the default), the stop hook scans each assistant message for completion phrases (`✓`, `Done!`, `All tests pass`, `Phase N complete`, …) and — if there's an in-progress plan phase — emits a non-blocking `systemMessage` suggesting `bin/edikt verify <plan-id> --phase <N>`. With the flag off, the detector stays silent. See [Gates](/governance/gates#completion-claim-detector) for details.

### evidence-gate

Governs the `verify-gate` PreToolUse hook — a separate mechanism from `quality-gates` above. It watches for completion-claim edits (flipping a sidecar to `passes: true`, marking a plan row `done`, ticking an `AC-NNN` checkbox) and evaluates them against a posture, not a boolean. Four values, and only one of them actually refuses the write (ADR-062):

| posture | write | stdout | audit record |
|---|---|---|---|
| `block` | refused | `hookSpecificOutput.permissionDecision: "deny"` | `gate.deny` |
| `warn` (default) | allowed, with a model-facing warning | `hookSpecificOutput.additionalContext` carrying the warning | `gate.warn` |
| `educate` | allowed | `{"continue": true}` | `posture.educate` |
| `disabled` | allowed | `{"continue": true}` | none — short-circuited before any state I/O |

```yaml
features:
  evidence-gate: warn      # block | warn | educate | disabled
```

An unrecognised or unreadable value resolves to `warn`, not `block` — the gate's safe state is the shipped default, not the strictest posture. Only `block` refuses a completion-claim edit that lacks a fresh verify report; under `warn` the edit still lands, and the model gets a warning on the same turn instead. `/edikt:doctor` reports the resolved posture (`block` / `warn` / `educate` / `disabled`), not a binary enabled/bypassed state.

See [Falsifiable verification → The pre-edit gate](/guides/falsifiable-verification#the-pre-edit-gate) for the bypass envelope and full mechanics, and [Gates → Pre-edit gate](/governance/gates#pre-edit-gate) for how it relates to the specialist-agent gates on that page.

## Evaluator

The evaluator validates acceptance criteria at two points: pre-flight (before a phase starts, checking the criteria are testable) and phase-end (after completion, verifying the work meets them). Both points, plus the execution mode, retry ceiling, and model, are configured under `evaluator:` in `.edikt/config.yaml`. Setting both `preflight` and `phase-end` to `false` disables it — the criteria sidecar is still emitted.

See [Evaluator](/governance/evaluator) for the config keys, their defaults, and the headless-vs-subagent comparison.

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
