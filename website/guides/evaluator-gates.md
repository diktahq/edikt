# Configuring Evaluator Gates

Quality gates are the enforcement edge of edikt. When a specialist agent finishes evaluating a phase, the `subagent-stop.sh` hook reads the structured verdict and decides whether to block the calling agent or let it proceed. As of v0.6.0 (per ADR-023), that decision is driven by a structured contract — not keyword matching over prose.

This guide covers the gate config in `.edikt/config.yaml`, severity levels, the env-var override, what happens when a gate fires, and the audit trail. It ends with a worked example: tightening the security gate from `critical` to `warning`.

## The structured contract

In v0.5.0 and earlier, the SubagentStop hook parsed evaluator output by searching for keywords like "BLOCKED", "critical", or agent-domain words in the raw text. Three failure modes:

1. Domain attribution was unreliable. Matching "security" in free text could trigger another domain's gate.
2. Severity parsing broke on synonyms or LLM rephrasing. Real critical issues were missed.
3. There was no contract. Agents didn't know what schema the hook expected; the hook didn't know what agents produced.

ADR-023 fixes this with a typed payload. Evaluator agents now emit a JSON object on stdout, claude-code wires it into the hook input directly, and the hook reads `evaluator_output` like a struct:

```json
{
  "verdict": "BLOCKED",
  "evaluator_output": {
    "agent": "security",
    "severity": "critical",
    "findings": [
      {
        "rule": "OWASP-A01",
        "severity": "critical",
        "description": "Hardcoded JWT secret in auth/handler.go:47"
      }
    ]
  }
}
```

The hook reads `evaluator_output.agent` to look up the threshold, compares `evaluator_output.severity` to it, and either blocks or passes. No regex, no keyword lists, no false positives from synonyms.

## Configuring `gates.<agent>` thresholds

Gate thresholds live in `.edikt/config.yaml`:

```yaml
gates:
  security:    warning     # block on warning + critical
  dba:         critical    # block on critical only
  sre:         warning
  architect:   warning
  performance: critical
  api:         warning
  default:     critical    # fallback for any unlisted agent
```

Severity ordering: `critical (3) > warning (2) > info (1)`. The gate fires when:

```text
finding.severity_level >= threshold_level
```

So `gates.security: warning` blocks on `warning` and `critical`. `gates.security: critical` blocks only on `critical`. `gates.security: info` blocks on every finding the security agent surfaces.

`default` is the fallback. If the evaluator emits `agent: "audit"` and there's no `gates.audit` key, the hook uses `gates.default`.

### Defaults shipped with v0.6.0

| Agent | Default | Rationale |
|-------|---------|-----------|
| `security` | `warning` | Tighter — security warnings are usually worth pausing on |
| `dba` | `critical` | DBA findings are usually actionable but rarely block-on-warning |
| `sre` | `warning` | Operational concerns benefit from early surfacing |
| `architect` | `warning` | Architectural drift is easier to fix early |
| `performance` | `critical` | Performance regressions are easier to fix later than to spot, so default to non-blocking |
| `api` | `warning` | Contract changes often need discussion |
| `default` | `critical` | Conservative for unknown agents |

These defaults only apply when you set the `gates:` section in your config. New projects get the section automatically. Upgrades from v0.5.x don't add it — `edikt upgrade` leaves your config alone unless you explicitly opt in.

## Environment variable override

`EDIKT_GATE_SEVERITY_THRESHOLD` overrides every gate for a single invocation:

```bash
EDIKT_GATE_SEVERITY_THRESHOLD=critical /edikt:sdlc:plan SPEC-005
```

This is useful for one-off "tighten everything" or "loosen everything" runs without editing the config. The value is validated against the `critical | warning | info` allowlist (per INV-006) so you can't accidentally pass an unknown level.

Resolution order:

1. `EDIKT_GATE_SEVERITY_THRESHOLD` (per-invocation override)
2. `gates.<agent>` (matched from `evaluator_output.agent`)
3. `gates.default`
4. Compiled-in fallback: `critical`

## What happens when a gate fires

When the structured verdict's severity meets or exceeds the threshold, the hook blocks and emits a JSON `systemMessage` plus a non-zero exit:

```text
BLOCKED — security gate fired (severity: critical >= threshold: warning)
   Hardcoded JWT secret in auth/handler.go:47
   To change threshold: .edikt/config.yaml  gates.security: critical
```

The message names the agent, the resolved severity, the resolved threshold, and the finding. It also surfaces the exact config path to adjust if the threshold is tighter than you want.

You then have three options:

1. **Fix the finding.** Address the issue and continue. The next evaluation pass returns PASS and the gate clears.
2. **Override.** Approve the finding explicitly. Override is logged with your git identity (see [Audit trail](#audit-trail) below).
3. **Loosen the threshold.** Edit `.edikt/config.yaml` if the gate level is genuinely wrong for your project.

There is no path that silently skips the gate.

## Audit trail

Every gate fire is recorded to `~/.edikt/events.jsonl` as a single JSON line. So is every override and every resolution.

```jsonl
{"ts":"2026-04-25T14:22:00Z","event":"gate_fired","agent":"security","severity":"critical","finding_prefix":"Hardcoded JWT secret"}
{"ts":"2026-04-25T14:25:00Z","event":"gate_override","agent":"security","approver":"alex","reason":"local-dev only"}
```

`/edikt:doctor` reads this file and reports unresolved findings from the last 7 days plus override activity from the last 30:

```text
Gate activity:
  Unresolved: 1
    2026-04-25T14:22:00Z : security gate (critical) — no resolution recorded
  Overrides (last 30 days): 1
```

Run `/edikt:session` to sweep unresolved findings and resolve them inline.

## Legacy payloads

Pre-v0.6.0 evaluators don't emit the `evaluator_output` field. The hook detects the legacy shape, logs a `legacy_payload` event to `events.jsonl`, prints a stderr warning, and falls back to keyword detection.

```text
warn: legacy evaluator payload; falling back to keyword detection
```

The legacy path is deprecated and removed in v0.7.0. If you see the warning, your evaluator templates need refreshing — run `/edikt:upgrade` or `edikt install` to pull the latest.

## Worked example: tightening the security gate

By default, `gates.security` ships at `warning`. Suppose you want it tighter: any security finding, even `info`, should block until reviewed.

```bash
/edikt:config set gates.security info
```

After this, the next evaluator run that produces a security finding at `info` severity blocks:

```text
BLOCKED — security gate fired (severity: info >= threshold: info)
   No CSRF token on POST /admin/users
   To change threshold: .edikt/config.yaml  gates.security: critical
```

Verify with doctor:

```bash
/edikt:doctor
```

```text
[ok]   gates.security: info (was: warning)
```

To revert:

```bash
/edikt:config set gates.security warning
```

Or to relax for a single planning session without changing the config:

```bash
EDIKT_GATE_SEVERITY_THRESHOLD=critical /edikt:sdlc:plan SPEC-005
```

The override applies to every gate during that invocation, then the config defaults take over again.

## When to tighten vs loosen

There's no universal answer. A few heuristics:

- **Greenfield project, small team:** start at the defaults. Loosen if you find yourself overriding the same finding repeatedly — that's the threshold telling you it's tighter than the project's bar.
- **Production system, multi-tenant:** tighten `security` and `dba` to `info`. The cost of a missed finding is higher than the cost of a paused phase.
- **Personal project:** consider setting most gates to `critical` only. Solo work doesn't need the same friction as multi-engineer review.
- **Compliance-driven:** tighten everything. Configure `default: warning` and individually set the agents you trust further down. The audit trail in `events.jsonl` is what your auditor reads.

The point of the structured contract isn't that gates are right — it's that they're explicit. You can read the config and know exactly what blocks what, without parsing prose.

## What's next

- [/edikt:config](/commands/config) — full config reference
- [Quality Gates](/governance/gates) — the gate concept and override flow
- [Evaluator](/governance/evaluator) — headless vs subagent evaluator modes
- ADR-023 — structured evaluator-input contract (in the repo at `docs/architecture/decisions/`)
