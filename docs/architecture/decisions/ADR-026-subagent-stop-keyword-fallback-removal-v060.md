---
type: adr
id: ADR-026
title: Accelerate ADR-023 keyword-fallback removal to v0.6.0 + carve out non-agent subagents
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-05-02T00:00:00Z
references:
  adrs: [ADR-023, ADR-018]
  invariants: [INV-001]
  prds: []
  specs: []
---

# ADR-026 — Accelerate ADR-023 keyword-fallback removal to v0.6.0 + carve out non-agent subagents

## Status

**Accepted**

## Context

ADR-023 (April 2026) restructured `subagent-stop.sh` so the SubagentStop hook reads agent identity and severity from the structured `evaluator_output` field, deprecating the legacy content-keyword fallback. Its directive set the timeline at:

- v0.6.0 — keyword fallback **deprecated**
- v0.7.0 — keyword fallback **removed**

Two things changed since ADR-023 shipped that motivate accelerating that timeline and clarifying its policy:

**1. The deprecation is unsafe in production.** Empirically observed (2026-05-02): `/edikt:status` and `/edikt:doctor` are forked slash commands (`context: fork`). Their final outputs reach the `SubagentStop` hook the same way specialist agents' outputs do. The status dashboard's `GATE ACTIVITY` section literally contains lines like `⛔ architect — critical (threshold=warning)` as a *report* on prior gate fires. The keyword fallback paths in subagent-stop.sh:

- Identified `AGENT_NAME=architect` from the keyword "architect" in the dashboard text
- Identified `SEVERITY=critical` from the keyword "critical" in the dashboard text
- Resolved threshold from `gates.architect` (= warning) instead of `gates.default`
- Fired the gate (critical ≥ warning) and returned `{"decision": "block"}`

The subagent's first stop attempt was blocked. It retried with the same content. Same false positive. Same block. After the harness gave up retrying, the subagent's terminal final response was meta-commentary about the block, not the rendered dashboard. The user never saw the dashboard.

This is not a hypothetical edge case. Any forked subagent whose content mentions agent keywords AND severity terms — which describes every dashboard, doctor report, audit report, and session summary in edikt — was vulnerable. Leaving the keyword fallback "deprecated but live" through a full minor version (v0.6.0) means shipping a known false-positive class as default behavior.

**2. ADR-023's "fall back to gates.default" policy was written assuming all SubagentStop firings are specialist-agent stops.** Claude Code's SubagentStop event fires for *any* subagent — Agent-tool invocations (specialists), forked slash commands (dashboards/audits), and future subagent classes. Specialist agents emit identifiable signals (structured `evaluator_output` per ADR-018, or canonical Claude Code payload fields like `subagent_type`). Non-agent subagents emit none of these. ADR-023's directive — "When `evaluator_output.agent` is absent, fall back to `gates.default`. NEVER silently skip the gate." — assumed the absence-of-agent case meant "legacy unstructured specialist agent." It did not anticipate "this subagent is not a specialist agent at all."

The right policy in 2026-05-02's reality: **only specialist agents are subject to gate firing. Non-agent subagents exit clean.** The discriminator is whether the payload yields an agent identity through one of the structured paths — `evaluator_output.agent` or canonical Claude Code fields (`subagent_type`, `agent_name`, `tool_name`, `agent`). If neither path yields an identity, the subagent is not a specialist agent and the hook returns `{"continue": true}` immediately.

## Decision Drivers

- The keyword fallback's gate-firing capability causes user-visible breakage in v0.6.0 today. Leaving it live until v0.7.0 ships a known regression class as default.
- ADR-023's "fall back to gates.default" policy applied a one-size-fits-all rule to a problem with two distinct cases (legacy specialist agent vs. non-agent subagent). Distinguishing them produces correct behavior for both.
- Per INV-002, ADR-023 is immutable. A targeted follow-on ADR is the right mechanism to refine its timeline and policy.

## Decision

**Remove the keyword content-grep fallback for agent identity entirely in v0.6.0. Specialist agents are identified only via structured paths: `evaluator_output.agent` (ADR-023) or canonical Claude Code payload fields. Subagents that yield no identity through either path exit clean — no gate firing.**

Operational rules:

1. **Keyword-grep agent identity removed.** The block in `templates/hooks/subagent-stop.sh` that scanned `INPUT_LOWER` for keywords like `architect`, `dba`, `security` is removed in v0.6.0 (not v0.7.0). The block also included a fallback regex (`As (Staff|Senior|Principal) [A-Za-z]+`) — this is also removed.

2. **Keyword-grep severity removed.** The block that grep'd for severity terms (`🔴|critical|CRITICAL|...`) when `evaluator_output.severity` was absent is removed (this part already shipped in commit b26f8c5 — ADR-026 documents and ratifies the change retroactively).

3. **Two structured identity paths only.** The hook accepts:
   - `evaluator_output.agent` (ADR-023 primary)
   - Canonical Claude Code payload fields: `subagent_type`, `agent_name`, `tool_name`, `agent` — set by the harness when a subagent is invoked via the Agent tool. These are NOT attacker-influenceable through subagent content.

4. **No identity → no gate.** If neither structured path yields a valid agent name (one of `architect dba security api backend frontend qa sre platform docs pm ux data performance compliance mobile seo gtm`), the hook returns `{"continue": true}` and exits. The subagent is treated as non-agent — no severity detection, no threshold lookup, no gate firing, no logging to `session-signals.log` as an AGENT event.

5. **Specialist-agent gate firing unchanged.** Subagents that DO yield an identity continue to fire gates per ADR-023 §4 threshold resolution. `gates.<agent>` lookup and `gates.default` fallback work as before.

6. **AGENT_IDENTITY_SOURCE values.** The hook's `AGENT_IDENTITY_SOURCE` variable (used in gate event logging) takes one of:
   - `"evaluator_output"` — ADR-023 structured path
   - `"payload"` — canonical Claude Code payload field
   The legacy value `"grep-fallback"` is removed from the code; it cannot appear in any new gate event.

7. **Migration path for legacy specialist agents.** Pre-v0.5.0 specialist agent templates that emit unstructured prose verdicts are no longer gated by this hook. Per ADR-018, all evaluator agents must emit structured JSON conforming to `templates/agents/evaluator-verdict.schema.json` by v0.5.0. Any project still running pre-v0.5.0 evaluator templates must update to the structured schema; their gate firing was already broken under ADR-023 keyword fallback's known false-positive surface.

## Alternatives Considered

### Keep ADR-023's v0.7.0 timeline; ship a defensive workaround in v0.6.0

- **Pros:** Honors ADR-023's stated timeline. Smaller policy surface change.
- **Cons:** A "defensive workaround" would have to identify slash-command subagents heuristically (e.g., absent `subagent_type` + presence of slash-command-shaped content) and exempt them from gate firing — itself a content-detection heuristic that risks the same false-positive class. Cleaner to remove the unsafe path entirely.
- **Rejected because:** the defensive workaround replaces one heuristic with another. Removing keyword detection altogether is the actual fix.

### Accelerate timeline only (keep "fall back to gates.default" policy)

- **Pros:** Minimal policy surface change — just shift the date.
- **Cons:** Doesn't solve the slash-command false-positive case. A non-agent subagent with no identity at all would still fire `gates.default`, which in this project is `critical`. If the subagent's content mentioned `critical` (e.g., a doctor report listing critical warnings), the gate could still fire under the keyword-severity path until that's also removed. Then `gates.default` thresholds would still apply to non-agents — defensible as a security posture but produces noisy gate events on every dashboard run.
- **Rejected because:** the false-positive class is broader than the timeline. The right policy distinguishes specialist agents from non-agent subagents.

### Remove SubagentStop hook entirely from forked slash commands

- **Pros:** No hook fires → no false positive possible.
- **Cons:** Would require Claude Code to not fire SubagentStop for slash-command forks, which is out of edikt's control. Even if achievable via configuration, it disables the hook's other purposes (logging AGENT events, telemetry).
- **Rejected because:** edikt cannot change Claude Code's harness firing behavior.

## Consequences

- **Good.** False-positive gate fires on `/edikt:status`, `/edikt:doctor`, `/edikt:session`, `/edikt:docs:review`, `/edikt:sdlc:audit`, `/edikt:sdlc:review` are eliminated. Their dashboards reach the parent session unblocked.
- **Good.** Specialist agents continue to fire gates correctly. The only path removed is the keyword-fallback identity, which was unsafe-by-design.
- **Good.** ADR-023's structured-path discipline becomes load-bearing rather than aspirational. Any agent that wants to fire a gate MUST emit `evaluator_output.agent` or be invoked via Agent tool with a recognized `subagent_type`.
- **Bad.** Pre-v0.5.0 specialist agents (if any still exist in user projects) lose gate firing. Mitigation: ADR-018 already mandated the structured schema for v0.5.0; users running older templates were already in violation of governance.
- **Bad.** ADR-023's directive set as written says "fall back to gates.default. NEVER silently skip the gate" — ADR-026 supersedes that specific directive for the non-agent case. ADR-023 remains accepted; ADR-026 carves out the non-agent subagent class from its mandate.
- **Neutral.** The CHANGELOG for v0.6.0 must call out the accelerated removal so users with custom evaluator templates know to verify they emit `evaluator_output.agent`.

## Confirmation

- `templates/hooks/subagent-stop.sh` contains no keyword-grep agent identity block. The only paths that set `AGENT_NAME` are evaluator_output (path 1) and canonical payload fields (path 2).
- `templates/hooks/subagent-stop.sh` contains no keyword-grep severity detection. When `evaluator_output.severity` is absent, `SEVERITY` stays at the default "info".
- `test/fixtures/hook-payloads/subagent-stop-no-agent.json` exists: a fixture with NO `evaluator_output` and NO canonical agent fields, content mentions agent keywords like "architect" and "critical". Expected hook output: `{"continue": true}`.
- `test/unit/hooks/test_subagent_stop.sh` includes the no-agent fixture in its FIXTURES array and passes.
- Empirical: `/edikt:status` and `/edikt:doctor` produce visible dashboards in the parent session without retry-block-loops.

## Directives


---

*Captured by edikt:adr — 2026-05-02*
