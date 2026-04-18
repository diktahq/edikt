---
type: adr
id: ADR-023
title: SubagentStop hook reads structured evaluator-input; evaluator agents emit structured verdicts
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-04-18T00:00:00Z
references:
  adrs: [ADR-010, ADR-014, ADR-018]
  invariants: [INV-003, INV-004, INV-006]
  specs: [SPEC-006]
---

# ADR-023 — SubagentStop structured evaluator-input contract

## Status

**Accepted**

## Context

`subagent-stop.sh` is the PostToolUse hook that intercepts evaluator agent output and blocks or passes the calling agent's action based on verdict severity. In v0.5.0, the hook parses verdict content using keyword matching over the raw evaluator text — searching for strings like "BLOCKED", "critical", or agent-domain keywords in the raw output. This has three failure modes:

1. **Domain attribution is unreliable.** Matching "security" or "performance" in free text is noisy; advisories for one agent domain can accidentally trigger another domain's gate.
2. **Severity parsing is fragile.** Keyword searches over prose output break on synonyms, re-phrasing, or translation by the LLM. A legitimate "critical" issue can be missed.
3. **No contract between evaluator agents and the hook.** Evaluator agents do not know what schema the hook expects; the hook does not know what schema the agent produces.

ADR-018 (evaluator verdict schema) established a structured JSON schema for evaluator verdicts. ADR-010 (headless evaluator) established the evaluation flow. This ADR formalizes the specific contract that `subagent-stop.sh` uses when reading hook input and defines the required output shape from evaluator agents.

## Decision

### 1. Structured hook payload contract

`subagent-stop.sh` MUST read the PostToolUse hook input JSON and extract verdict data exclusively from the `tool_result` field's structured content. The required input shape is:

```json
{
  "tool_result": {
    "content": [
      {
        "type": "text",
        "text": "<json-encoded evaluator output>"
      }
    ]
  }
}
```

The `text` field is a JSON-encoded evaluator output string (double-encoded). `subagent-stop.sh` MUST parse it as:

```json
{
  "verdict": "PASS | BLOCKED",
  "evaluator_output": {
    "agent": "<agent-domain>",
    "severity": "critical | warning | info",
    "findings": [
      {
        "rule": "<rule-id or short label>",
        "severity": "critical | warning | info",
        "description": "<human-readable description>"
      }
    ]
  }
}
```

This is the **canonical payload shape** for SPEC-006 and beyond.

### 2. Agent domain resolution

`subagent-stop.sh` MUST read `evaluator_output.agent` from the structured payload to determine the gate severity threshold. Content-based keyword detection over free text is the **legacy path** and is deprecated as of v0.6.0. It will be removed in v0.7.0.

When `evaluator_output.agent` is absent (legacy unstructured payload), the hook MUST log a warning to `events.jsonl` and fall back to `gates.default`.

### 3. Evaluator agents output contract

All evaluator agent templates (`templates/agents/evaluator-*.md`) MUST produce a JSON object conforming to the shape above. The `evaluator_output.agent` field MUST be set to the agent's domain identifier (e.g., `"security"`, `"dba"`, `"sre"`, `"architect"`, `"performance"`, `"api"`).

Evaluator agents MUST NOT emit prose verdicts. ADR-018's schema (`templates/agents/evaluator-verdict.schema.json`) is the source of truth for the JSON structure; this ADR adds the `evaluator_output.agent` field requirement on top of it.

### 4. Severity threshold resolution

`subagent-stop.sh` resolves the effective severity threshold for a given invocation in this order:

1. `EDIKT_GATE_SEVERITY_THRESHOLD` env var (if set — per-invocation override, validated against allowlist per INV-006)
2. `.edikt/config.yaml` → `gates.<agent>` (read from `evaluator_output.agent`)
3. `.edikt/config.yaml` → `gates.default`
4. Compiled-in fallback: `critical`

Severity ordering: `critical(3) > warning(2) > info(1)`. The gate fires when `finding.severity_level >= threshold_level`.

### 5. Gate-fired message format

When the gate fires, `subagent-stop.sh` MUST emit (ref: AC-030):

```
🔴 BLOCKED — <agent> gate fired (severity: <severity> ≥ threshold: <threshold>)
   To change threshold: .edikt/config.yaml  gates.<agent>: critical
```

The message MUST be assembled inside the hook process using `python3 -c 'import json; print(json.dumps(...))'` with untrusted values as argv (INV-003, INV-004).

### 6. Legacy payload path

If the hook input does not contain the structured `evaluator_output` shape (legacy unstructured payload from pre-v0.6.0 evaluators):

- Log `{"event": "legacy_payload", "hook": "subagent-stop"}` to `events.jsonl`
- Warn to stderr: `warn: legacy evaluator payload; falling back to keyword detection`
- Fall back to keyword-based domain detection (deprecated; removed in v0.7.0)

## Consequences

- `subagent-stop.sh` becomes simpler: one `python3 -m json.tool` parse replaces multiple regex operations
- Agent domain attribution is exact: the evaluator agent sets its own domain label
- Severity thresholding is exact: no string-matching false positives
- Evaluator agent templates require a one-time update to emit the `agent` field
- Legacy unstructured payloads degrade gracefully until v0.7.0 removes the fallback

[edikt:directives:start]: #
directives:
  - Subagent-stop.sh MUST read verdict and severity exclusively from the structured `evaluator_output` field in the hook payload — never from keyword matching over prose output. (ref: ADR-023)
  - Evaluator agent templates MUST set `evaluator_output.agent` to the agent's domain identifier string. (ref: ADR-023)
  - Agent domain for gate threshold resolution MUST come from `evaluator_output.agent`, not from content keyword detection. Content detection is the legacy fallback only. (ref: ADR-023)
  - When `evaluator_output.agent` is absent, log a warning to events.jsonl and fall back to `gates.default`. NEVER silently skip the gate. (ref: ADR-023)
  - Gate-fired messages MUST be JSON-assembled via `python3 -c 'import json; print(json.dumps(...))'` with severity and agent values as argv. (ref: ADR-023, INV-003)
  - Legacy keyword detection for agent domain is deprecated as of v0.6.0 and MUST be removed in v0.7.0. Do not expand the keyword list. (ref: ADR-023)
paths:
  - "templates/hooks/subagent-stop.sh"
  - "templates/agents/evaluator-*.md"
  - ".edikt/config.yaml"
scope: [implementation, review]
[edikt:directives:end]: #
