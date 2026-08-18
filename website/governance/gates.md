# Quality Gates

Quality gates are enforcement checkpoints. When a specialist agent detects a critical finding during a plan phase, review, or audit, it blocks progression until the finding is resolved.

You don't trigger gates. They fire automatically — via the SubagentStop hook when an agent completes work, via pre-flight review when a plan phase begins, or via an explicit audit. When a gate fires, the model presents it.

## What they catch

Gates fire on findings that shouldn't proceed:

- Hardcoded secrets or credentials
- Migrations without rollback (DOWN migration)
- API breaking changes without version strategy
- Security vulnerabilities identified by OWASP scan
- Missing indexes on high-query columns
- Authentication boundary violations

Not all findings trigger a gate. Findings are classified by severity:

| Severity | Behavior |
|----------|----------|
| `CRITICAL` | Blocks progression. Must be resolved or explicitly overridden. |
| `WARNING` | Surfaces in review output. Does not block. |
| `PASS` | No action required. |

## What you see when a gate fires

```text
GATE: security — critical finding
   Hardcoded JWT secret in auth/handler.go:47

   This gate must be resolved before proceeding.
   Override this gate? (y/n)
   Note: override will be logged with your git identity.
```

The agent that raised the finding is named. The file and line are specific. The override prompt requires explicit acknowledgment.

Fix the issue and continue, or respond `y` to log the override. There is no path that skips the gate silently.

## Overrides

Overrides are available — but they are not silent. Every override is logged to `~/.edikt/events.jsonl` with the git name and email of the engineer who approved it. Declining is logged too. Engineers can override individual findings; every one of them stays traceable.

This is intentional. Gates are not about blocking work — they're about making enforcement visible. An override says "I know about this finding and I'm proceeding anyway." That's a legitimate decision. The log captures it.

Override log format:

```text
GATE OVERRIDE
  Finding:    Hardcoded JWT secret in auth/handler.go:47
  Agent:      security
  Approved:   alex <alex@example.com>
  Date:       2026-03-20
  Reason:     development environment only — will be removed before merge
```

Override activity is visible in the governance dashboard. Ask "what's our status?" to see gate history.

## Configuring gates

Gates are configured in `.edikt/config.yaml` under `gates:` — a flat map from specialist agent name to severity threshold, plus a `default:` fallback for any agent not listed:

```yaml
gates:
  security: warning    # security findings warrant early attention
  dba: critical         # DB changes — block only on critical
  sre: warning
  architect: warning
  performance: critical
  api: warning
  default: critical     # for any agent not listed above
```

To change a specialist's severity threshold, edit its value directly. `critical` blocks progression; `warning` surfaces the finding without blocking. Separately, `features.quality-gates` in the same config file (default `true`) turns the whole mechanism on or off.

## When gates fire

Gates are checked at three points:

1. **Pre-flight review** — before plan execution begins, specialist agents review the plan. Critical findings from pre-flight block the plan from starting.

2. **SubagentStop hook** — after each specialist agent completes work during execution, findings are evaluated. Critical findings pause execution.

3. **Explicit audit** — ask for a security and quality audit. All relevant agents scan the current implementation.

**Command reference:** `/edikt:sdlc:audit`

## Re-fire prevention

After you override a finding, it won't fire again for the rest of your session. A session is a single Claude Code invocation — when you close it and reopen it, the override expires and the gate fires again (reminding you the issue is still there).

Overrides are stored in `~/.edikt/gate-overrides.jsonl` and cleared by the SessionStart hook at the beginning of each session.

## Audit log

All gate events are logged to `~/.edikt/events.jsonl`:

```jsonl
{"ts":"2026-04-11T10:30:00Z","event":"gate_fired","agent":"security","severity":"critical","finding":"SQL injection in orders/handler.go:47"}
{"ts":"2026-04-11T10:31:00Z","event":"gate_override","agent":"security","finding":"SQL injection in orders/handler.go:47","user":"Daniel Gomes","email":"daniel@example.com"}
```

Three event types: `gate_fired` (hook detected critical finding), `gate_override` (user chose to proceed), `gate_blocked` (user chose to stop and fix). Query with jq:

```bash
jq 'select(.event == "gate_override")' ~/.edikt/events.jsonl
```

## Completion-claim detector (v0.6.0)

A non-blocking soft signal in `templates/hooks/stop-hook.sh`. Unlike the agent-driven specialist gates above, this detector is a regex on the last assistant message paired with a quick scan of plan files.

Activation predicate (all three must hold):

1. `features.quality-gates: true` in `.edikt/config.yaml` (the default; the flag opts the detector out).
2. The last assistant message matches a case-insensitive completion phrase regex: `✓`, `✅`, `Done!`, `All tests pass`, `All green`, `Build clean`, `Perfect`, `Looks good`, `Should work`, `Should be fine`, `Phase N complete`, `complete!`.
3. At least one plan under `paths.plans` (default `docs/internal/plans/`) has a progress-table row with `status: in-progress`.

When all three hold, the hook emits:

```text
⚠ Completion claim detected during an in-progress plan phase.
  Run: bin/edikt verify <plan-id> --phase <N> to verify before declaring done.
```

The message uses `systemMessage` only — **never `decision: block`**. The user is the final judge; the warning is advisory. Plan-id and phase are parsed with a strict allowlist regex and double-validated in bash before flowing into the JSON template (no raw message text ever reaches the model-facing channel). See [features.quality-gates](/governance/features#quality-gates) for the opt-out.

## Pre-edit gate

The completion-claim detector above is a soft, non-blocking signal fired at Stop. A separate mechanism — the `verify-gate` PreToolUse hook — watches the same class of edit (flipping a sidecar to `passes: true`, marking a plan row `done`, ticking an `AC-NNN` checkbox) and acts *before* the edit lands. Unlike the specialist-agent gates described elsewhere on this page, its enforcement is not fixed at "block": it reads a four-value posture (`block | warn | educate | disabled`, default `warn`) from `features.evidence-gate` in `.edikt/config.yaml`, and only `block` actually refuses the write (ADR-062).

See [Configurable Features → evidence-gate](/governance/features#evidence-gate) for the posture table, and [Falsifiable verification → The pre-edit gate](/guides/falsifiable-verification#the-pre-edit-gate) for the bypass envelope and full mechanics.

## CI integration

For CI pipelines, run `/edikt:sdlc:drift --output=json`. Exit code `1` if any diverged findings exist. This integrates with any CI system that checks exit codes.

Quality gates during development prevent CI failures from being the first signal of a problem. The gate fires while the engineer is still in the session — not after a push.
