# ADR-010: Evaluator runs headless by default, falls back to subagent with visible warning, never silently degrades

**Date:** 2026-04-14
**Status:** Accepted
**Supersedes:** None

## Context

edikt's phase-end evaluator verifies that completed work meets acceptance criteria before context reset. It has two execution modes:

- **Headless** — invoked via `claude -p` as a separate process. Has its own permission sandbox, can execute Bash (run tests, grep, inspect). Startup cost ~5–10s.
- **Subagent** — spawned via the Agent tool inside the current session. Shares the parent's permission sandbox. Faster startup, inline visibility. Bash may be denied by the parent's sandbox even when the agent's `tools:` frontmatter declares it.

The evaluator's job is to verify. If it cannot execute commands, it cannot verify runtime behavior — only structural properties visible through Read/Grep/Glob.

**The bug surfaced during v0.4.x dogfooding.** A user invoked the evaluator directly via `Agent(subagent_type: "evaluator")` in an interactive session. Bash was denied by the session sandbox. The evaluator fell back to read-only inspection and returned PASS verdicts on acceptance criteria that required test execution. The verdict was unverified but indistinguishable from a real PASS.

Three problems compounded:

1. **Silent degradation.** The evaluator has no BLOCKED verdict. Every criterion is PASS or FAIL. When Bash is denied, read-only inspection produces PASS by default because there's nothing to contradict the code's apparent correctness.
2. **Mode bypass without warning.** `commands/sdlc/plan.md` reads `evaluator.mode` from config and routes correctly. But the evaluator agent template is publicly invocable — any caller (direct `Agent()` call, another command, a user experiment) bypasses the config entirely.
3. **No recovery surface.** Even if the user suspects a silent degradation, there is no `--eval-only` re-run, no doctor probe of the evaluator, and no progress-table state for "evaluation attempted but incomplete."

The root cause is architectural: edikt offered two modes with asymmetric capabilities and no enforced boundary between them. Headless is the only mode that can reliably verify. Subagent was kept as a fallback for environments where headless fails (missing `claude` on PATH, auth not inherited, subprocess spawning blocked, MCP servers required in evaluation context, etc.) — and those failure modes are real enough that collapsing to headless-only would trade one class of silent failure for another class of hard failure.

## Decision Drivers

- **Verification integrity is non-negotiable.** The evaluator must never return PASS without actual verification. A verifier that can silently lie is worse than no verifier.
- **Headless has real failure modes.** Missing CLI, expired auth, sandboxed environments, MCP dependencies. Collapsing to headless-only strands users whose environments can't support it.
- **Users need visibility.** When the evaluator degrades, falls back, or cannot verify, the user must see it immediately in the phase output — not in a log file, not as a silent metric.
- **Recovery must be actionable.** Every failure mode produces an output message that names the fix as a one-liner the user can paste.

## Considered Options

1. **Collapse to headless-only** — remove subagent mode entirely. Simplest mental model, zero silent-PASS risk. But strands users whose environments can't run headless.
2. **Keep both modes with user-facing config switch** — status quo. Silent degradation possible when users bypass the orchestrator.
3. **Headless default, subagent as automatic fallback with visible warnings and a BLOCKED verdict** — evaluator cannot silently PASS; fallback is always surfaced; recovery is one command.

## Decision

edikt adopts Option 3. The evaluator MUST run headless by default. The evaluator MAY fall back to subagent mode only when headless fails, and MUST emit a visible warning banner when it does. The evaluator MUST return a BLOCKED verdict — never PASS — when it cannot execute a verification step required by an acceptance criterion.

**Specific directives:**

1. **Default mode MUST be headless.** `.edikt/config.yaml` ships with `evaluator.mode: headless` as the default and recommended value. `subagent` remains a supported value for environments where headless cannot run.

2. **The evaluator agent template MUST support a BLOCKED verdict.** Both `templates/agents/evaluator.md` (subagent) and `templates/agents/evaluator-headless.md` (headless) MUST declare BLOCKED as a valid per-criterion verdict and a valid overall verdict. BLOCKED means "evaluator could not verify this criterion due to a missing capability (Bash denied, test runner missing, external dependency unavailable)."

3. **The evaluator MUST NOT return PASS when required verification steps could not execute.** If an acceptance criterion requires Bash execution and Bash is denied, the verdict for that criterion is BLOCKED. Read-only inspection does not substitute for runtime verification. Overall verdict is BLOCKED whenever any criterion is BLOCKED.

4. **`commands/sdlc/plan.md` MUST attempt headless first when `evaluator.mode: headless`.** On headless failure (spawn error, non-zero exit, auth error, timeout), it MUST fall back to subagent mode and emit a visible warning banner naming the failure reason and the remediation.

5. **`commands/sdlc/plan.md` MUST parse and surface BLOCKED verdicts in the phase evaluation output.** The progress table MUST gain a `blocked` state in addition to the existing `pass`/`fail`/`evaluating` states. A phase with BLOCKED verdicts is NOT considered verified and MUST NOT be marked complete.

6. **The subagent evaluator template MUST self-check before claiming verdicts.** Before evaluating any criterion that requires Bash, it MUST probe whether Bash is available. If denied, it MUST return BLOCKED for that criterion with an explicit recovery hint ("re-run with `evaluator.mode: headless` or via `/edikt:sdlc:plan`").

7. **`/edikt:doctor` MUST probe the evaluator path.** It MUST verify `claude` is on PATH, that a headless probe (`claude -p "echo ok"`) succeeds, that the configured evaluator template exists, and that `evaluator.mode` is set explicitly (not inferred default). Failures MUST surface with actionable remediation.

8. **Every evaluator failure mode MUST produce user-visible output.** Headless spawn failure, auth failure, Bash denial, BLOCKED verdicts, and full evaluation failure each have a defined output block with the failure reason, the criterion affected (where applicable), and the one-line command the user can run to recover.

## Alternatives Considered

### Option 1 — Collapse to headless-only

- **Pros:** Simplest mental model. Zero silent-PASS surface. One code path to maintain.
- **Cons:** Headless has real environmental failure modes (missing CLI, expired auth, subprocess restrictions, MCP dependencies). Users in those environments would have no working evaluator at all.
- **Rejected because:** Trades silent-PASS failures for silent-no-eval failures. Does not solve the root problem (visibility); just changes which users get stranded.

### Option 2 — Keep both modes with user-facing config switch, no fallback, no BLOCKED verdict

- **Pros:** Minimal change. Matches the existing config surface.
- **Cons:** Preserves every problem that produced this ADR. The evaluator can still silently PASS. Users can still bypass config by invoking the agent directly.
- **Rejected because:** This is the status quo that produced the bug.

## Consequences

- **Good:** The evaluator cannot silently lie. Every failure mode produces visible output with a recovery path. Users can trust the PASS verdict because BLOCKED exists as an honest alternative.
- **Good:** `/edikt:doctor` gains a probe that catches evaluator misconfiguration before it blocks a phase.
- **Bad:** `commands/sdlc/plan.md` grows in complexity — headless attempt, failure classification, fallback, warning emission, BLOCKED parsing, progress-table state. Worth it for verification integrity.
- **Bad:** BLOCKED is a third verdict that consumers of the evaluator output (including humans reading plan files) must learn. Mitigation: every BLOCKED verdict carries an inline recovery hint.
- **Neutral:** Subagent mode remains supported but is no longer the path `plan.md` reaches for first. Users who explicitly set `evaluator.mode: subagent` in config still get subagent, with the BLOCKED verdict available as a safety net.

## Confirmation

How to verify this decision is being followed:

- **Automated:** `templates/agents/evaluator.md` and `templates/agents/evaluator-headless.md` both contain BLOCKED in the output format and an explicit rule that execution-required criteria cannot return PASS without execution. Grep for `BLOCKED` in both files.
- **Automated:** `commands/sdlc/plan.md` contains a try-headless-then-fallback block with a visible warning banner on fallback. Grep for the banner string.
- **Automated:** `commands/doctor.md` probes the evaluator path with `claude -p "echo ok"` and reports the result. Grep for the probe invocation.
- **Manual:** Code review of any change to the evaluator flow in `plan.md` checks that BLOCKED verdicts are parsed, surfaced in the phase report, and block phase completion.
- **Manual:** Any new evaluator-invocation site (new command, new skill) is reviewed for whether it routes through `plan.md`'s orchestrator or invokes the agent directly. Direct invocations MUST be justified and MUST include the BLOCKED-verdict contract.

## Directives

[edikt:directives:start]: #
topic: agent-rules
paths:
  - templates/agents/evaluator.md
  - templates/agents/evaluator-headless.md
  - commands/sdlc/plan.md
  - commands/doctor.md
  - .edikt/config.yaml
scope:
  - implementation
  - review
  - planning
directives:
  - Evaluator default mode MUST be headless. `.edikt/config.yaml` ships with `evaluator.mode: headless`. (ref: ADR-010)
  - Evaluator templates MUST declare BLOCKED as a valid per-criterion and overall verdict. (ref: ADR-010)
  - Evaluator MUST NOT return PASS for criteria that require execution when execution was denied or unavailable. Return BLOCKED instead. (ref: ADR-010)
  - `commands/sdlc/plan.md` MUST attempt headless first when `evaluator.mode: headless`, and fall back to subagent only on headless failure with a visible warning banner. (ref: ADR-010)
  - Progress tables in plan files MUST support a `blocked` state in addition to `pass`/`fail`/`evaluating`. A phase with BLOCKED criteria is NOT verified. (ref: ADR-010)
  - Subagent-mode evaluator MUST probe Bash availability before claiming verdicts on criteria that require execution. If Bash is denied, return BLOCKED with a recovery hint. (ref: ADR-010)
  - `/edikt:doctor` MUST probe the evaluator path: `claude` on PATH, headless probe success, template presence, explicit mode config. (ref: ADR-010)
  - Every evaluator failure mode MUST produce user-visible output with the reason and a one-line recovery command. Silent degradation is forbidden. (ref: ADR-010)
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #

---

*Captured by edikt:adr — 2026-04-14*
