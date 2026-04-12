---
type: spec
id: SPEC-002
title: "Evaluator Configuration, Headless Execution, and LLM Experiment Evaluator"
status: accepted
author: Daniel Gomes
implements: PRD-001
source_prd: docs/product/prds/PRD-001-v040-harness-lifecycle-gates.md
created_at: 2026-04-11T03:00:00Z
references:
  adrs: [ADR-001, ADR-004]
  invariants: [INV-001]
  source_artifacts:
    - docs/plans/artifacts/experiment-evaluator-spec.md
---

# SPEC-002: Evaluator Configuration, Headless Execution, and LLM Experiment Evaluator

**Implements:** PRD-001 (FR-011 through FR-014, FR-022, FR-029 through FR-032)
**Date:** 2026-04-11
**Author:** Daniel Gomes

---

## Summary

This spec adds configurable evaluator execution to edikt's plan command (headless `claude -p` or subagent fallback) and integrates an LLM-based semantic evaluator into the experiment runner. Two audiences: engineers using plans get configurable, bias-free evaluation; edikt's own research gets a semantic evaluator that replaces brittle grep assertions. Both share the same skeptical evaluation principles but use separate prompt files.

## Context

The evaluator agent (`templates/agents/evaluator.md`) exists and is invoked by the plan command at two points: pre-flight (step 11, criteria validation) and phase-end (post-completion verification). Currently it runs as a subagent via the Agent tool — meaning it shares the same Claude session, which partially undermines the "fresh context" guarantee.

Anthropic's harness research recommends a generator-evaluator split where the evaluator is a separate invocation with zero shared state. Claude Code's `claude -p` flag makes this possible — headless, non-interactive, with `--bare` to skip hooks and memory.

Separately, the experiment runner (`test/experiments/directive-effect/run.sh`, internal/gitignored) uses grep assertions (`assertion.sh`) that are brittle and can't detect semantic violations. An LLM evaluator design exists as an artifact (`docs/plans/artifacts/experiment-evaluator-spec.md`) with fixture 08 already having an `evaluator-criteria.yaml` file ready.

## Existing Architecture

- **Evaluator agent:** `templates/agents/evaluator.md` (~130 lines). Subagent with `disallowedTools: [Write, Edit]`, `maxTurns: 15`. Pre-flight mode (criteria classification) and phase-end mode (PASS/FAIL with evidence). Skeptical by default.
- **Plan command:** `commands/sdlc/plan.md`. Step 11 invokes evaluator for pre-flight. Phase-end flow spawns evaluator after completion promise. Currently uses Agent tool (subagent).
- **Experiment runners:** Three directories under `test/experiments/`: `rule-compliance/` (shipped), `directive-effect/` (gitignored), `long-running/` (gitignored). The `directive-effect/run.sh` runner invokes `claude -p` for the generator, then runs `assertion.sh` for grep-based verdict.
- **Evaluator spec artifact:** `docs/plans/artifacts/experiment-evaluator-spec.md` (~355 lines). Full design for dual-mode grep+LLM evaluation, severity tiers, verdict logic, cost model.
- **Fixture 08:** `test/experiments/directive-effect/fixtures/08-long-context-invoicing/evaluator-criteria.yaml` — 8 criteria with severity tiers, ready for LLM evaluation.

## Proposed Design

### 1. Evaluator Configuration

New top-level config section in `.edikt/config.yaml`:

```yaml
evaluator:
  preflight: true          # pre-flight criteria validation (default: true)
  phase-end: true          # phase-end evaluation (default: true)
  mode: headless           # headless | subagent (default: headless)
  max-attempts: 5          # max phase retries before stuck (default: 5)
  model: sonnet            # sonnet | opus | haiku (default: sonnet)
```

The plan command MUST read these values before invoking the evaluator. When a key is absent, use the default.

**Evaluator file existence check:** Before any evaluator invocation (pre-flight or phase-end), the plan command MUST verify the evaluator template exists:
- Headless mode: check `templates/agents/evaluator-headless.md` (or `~/.edikt/templates/agents/evaluator-headless.md` for global install)
- Subagent mode: check `templates/agents/evaluator.md` (or `.claude/agents/evaluator.md`)

If the required file is missing:
```
❌ Evaluator template missing — cannot run evaluation.
   Expected: {path}
   Run: curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
   Or disable evaluation: /edikt:config set evaluator.phase-end false
```
The plan MUST NOT silently skip evaluation when the evaluator is enabled but the template is missing. This is a hard failure — the user must either reinstall or explicitly disable evaluation.

### 2. Two Evaluator Prompt Files

**Subagent template** (existing, modified): `templates/agents/evaluator.md`
- Retains frontmatter (`tools:`, `maxTurns:`, `disallowedTools:`)
- Used when `evaluator.mode: subagent`
- Plan command invokes via Agent tool (current behavior)

**Headless prompt** (new): `templates/agents/evaluator-headless.md`
- No frontmatter — plain markdown body only
- Contains the same evaluation rules, skeptical stance, output format, and constraints
- Used when `evaluator.mode: headless`
- Plan command reads this file and passes its content via `--system-prompt`

**The evaluator is an internal agent — it MUST NOT be user-overridable.** Unlike other agents (per ADR-005), the evaluator cannot be customized via `<!-- edikt:custom -->` marker, listed in `agents.custom`, or overridden via `.edikt/templates/`. The `/edikt:upgrade` command MUST always overwrite both evaluator files without prompting. `/edikt:doctor` MUST warn if either evaluator file has been manually modified (content hash mismatch against the installed template).

Both files MUST maintain the same evaluation principles:
- Skeptical by default — "assume the work is incomplete until proven otherwise"
- Every PASS requires evidence (file:line)
- Every FAIL requires a citation of what's missing
- Binary per criterion — no "partially met"
- NEVER modify code

The headless prompt MUST additionally include:
- Output format instructions (the evaluator's response is parsed, not displayed interactively)
- A note that it has zero context from the generator session

### 3. Headless Execution in Plan Command

When `evaluator.mode: headless`, the plan command MUST instruct Claude to invoke the evaluator as:

```bash
claude -p "{evaluation prompt with criteria + file list}" \
  --system-prompt "$(cat templates/agents/evaluator-headless.md)" \
  --allowedTools "Read,Grep,Glob,Bash" \
  --disallowedTools "Write,Edit" \
  --model {evaluator.model from config} \
  --output-format json \
  --bare
```

The evaluation prompt (user message) MUST include:
- The phase's acceptance criteria (from the criteria sidecar if SPEC-001 is implemented, or from plan markdown)
- The list of files modified during the phase (from `git diff --name-only` or phase output)
- The project's test command if available

The plan command MUST parse the evaluator's JSON output to extract per-criterion PASS/FAIL verdicts and update the criteria sidecar.

When `evaluator.mode: subagent`, the plan command uses the current Agent tool invocation (no changes to current behavior).

### 4. Toggle Behavior

When `evaluator.preflight: false`:
- Plan step 11 (pre-flight criteria validation) MUST be skipped entirely
- No pre-flight output is shown
- `verify` commands in the criteria sidecar are NOT populated (left null)

When `evaluator.phase-end: false`:
- Phase-end evaluation MUST be skipped
- The criteria sidecar is still emitted (it's useful as documentation)
- Phase completes immediately after the completion promise
- Context reset guidance still shows

### 5. LLM Evaluator in Experiment Runner

**File:** `test/experiments/directive-effect/run.sh`

The runner gains a second evaluation step after the grep assertion. Design per `docs/plans/artifacts/experiment-evaluator-spec.md`.

**Invocation conditions** — the LLM evaluator runs when ANY of:
1. The fixture contains `evaluator-criteria.yaml`
2. The runner is invoked with `--llm-eval` flag
3. The fixture contains `evaluator-criteria.txt` (plain-text alternative)

**Execution order:**
1. Generator runs (`claude -p "$task_prompt"`) — unchanged
2. Grep assertion (`assertion.sh`) runs — unchanged, serves as fast pre-check
3. If evaluator conditions met: collect generated code, build evaluator prompt, invoke `claude -p`
4. Parse evaluator output, write verdict file
5. LLM verdict is the final verdict when both run — overrides grep in both directions

**Code collection:**
- Find all files newer than the fixture's `go.mod` (or equivalent marker): `find $tmpdir -newer $tmpdir/go.mod -type f`
- Format as `=== filepath ===\n{file content}` blocks
- Truncate to 15 files if needed (prioritize new files by mtime)
- Exclude fixture files that existed before the generator ran

**Evaluator invocation:**

```bash
claude -p "$eval_user_prompt" \
  --system-prompt "$eval_system_prompt" \
  --allowedTools "Read,Grep,Glob,Bash" \
  --disallowedTools "Write,Edit" \
  --output-format text \
  --bare
```

The system prompt uses the same skeptical stance as the plan evaluator but is tailored for experiments (no plan context, no criteria sidecar — just criteria + code).

**Evaluator system prompt** (embedded in runner or separate file at `test/experiments/lib/evaluator-system-prompt.md`):
- "You are evaluating generated code against specific criteria."
- "Assume violations until proven otherwise."
- "For each criterion: cite file:line for PASS, cite what's missing for FAIL."
- "Output format: structured table with verdict."

### 6. Severity Tiers and Verdict Logic

Severity is assigned by the fixture author in `evaluator-criteria.yaml` via the `severity:` field per criterion.

**Three verdicts:**

| Verdict | Condition | Exit code |
|---------|-----------|-----------|
| **PASS** | All critical pass AND all important pass | 0 |
| **WEAK PASS** | All critical pass, 1+ important fail | 0 |
| **FAIL** | Any critical fails | 1 |

Informational findings are logged but NEVER affect the verdict.

WEAK PASS counts as PASS for experiment purposes (governance prevented critical violations) but surfaces a warning in the summary.

**Verdict file output** (`run-NN-eval.txt`):

```
exit: {0 or 1}
verdict: {PASS | WEAK PASS | FAIL}
evaluator: llm
critical_pass: {n}/{total}
important_pass: {n}/{total}
informational_pass: {n}/{total}
details:
{full evaluator output}
```

### 7. Token Usage Logging (FR-022)

When the LLM evaluator runs in experiments, log token usage in the run metadata:

```
evaluator_invoked: true
evaluator_model: sonnet
evaluator_verdict: PASS
evaluator_verdict_source: llm  # llm | grep | llm-override
```

Token counts are not directly available from `claude -p` output. If the output format includes token metadata (JSON mode), capture it. Otherwise, log the verdict source and model only.

## Components

### `templates/agents/evaluator.md` (modified)
- No structural changes. Remains the subagent template.
- Add a comment at the top: `<!-- Subagent mode. For headless mode, see evaluator-headless.md -->`

### `templates/agents/evaluator-headless.md` (new)
- Same evaluation rules, skeptical stance, output format, constraints as `evaluator.md`
- No frontmatter (no `tools:`, `maxTurns:`, etc.)
- Includes output format instructions for structured parsing
- Includes note about zero shared context

### `commands/sdlc/plan.md` (modified)
- Read `evaluator.*` config values at plan start
- Step 11: check `evaluator.preflight` — skip if false
- Phase-end flow: check `evaluator.phase-end` — skip if false
- Phase-end flow: check `evaluator.mode` — use headless `claude -p` or subagent Agent tool
- Read `evaluator.model` for headless `--model` flag
- Read `evaluator.max-attempts` for stuck threshold (connects to SPEC-001)

### `commands/config.md` (modified)
- Add `evaluator.*` keys to the Key Reference table (already done in v0.3.1 session)

### `test/experiments/directive-effect/run.sh` (modified)
- Add `--llm-eval` flag parsing
- Add evaluator condition detection (check for `evaluator-criteria.yaml`)
- Add code collection function (find new files, format as blocks)
- Add evaluator invocation (`claude -p` with system prompt)
- Add verdict parsing (extract PASS/WEAK PASS/FAIL from output)
- Add verdict file writing (`run-NN-eval.txt`)
- Add metadata logging (evaluator invoked, model, verdict source)
- Override logic: when LLM and grep disagree, LLM wins

### `test/experiments/lib/evaluator-system-prompt.md` (new)
- Skeptical system prompt for experiment evaluation
- Shared by all experiment runners that support LLM evaluation

### `website/governance/evaluator.md` (modified)
- Already created in v0.3.1 session with comparison table and config reference
- Update if any config keys or behavior change during implementation

## Non-Goals

- Plan harness changes (iteration tracking, context handoff, criteria sidecar) — covered in SPEC-001
- Quality gate UX — covered in SPEC-003
- Artifact lifecycle enforcement — covered in SPEC-003
- Backporting LLM evaluator to experiment fixtures 01-07 (start with 08, expand later)
- Evaluator tuning data collection (harness Phase 6 — depends on structured output from this spec)
- Token-optimized formats for evaluator input (ASON/TOON — noted for future optimization)

## Alternatives Considered

### Single evaluator template for both modes

- **Pros:** One file to maintain, no drift risk
- **Cons:** Plan command would need to strip YAML frontmatter at runtime for headless mode. Mixing agent metadata with prompt content is fragile.
- **Rejected because:** Two files are cleaner. The frontmatter is agent infrastructure, not prompt content. Maintaining both is low-cost since the evaluation rules are the same — only the packaging differs.

### Evaluator always headless (no subagent option)

- **Pros:** Simpler — one code path, always bias-free
- **Cons:** Requires `claude` CLI available in PATH (not guaranteed in all environments). Slower cold start. Can't use Claude's built-in tool routing.
- **Rejected because:** Subagent fallback is a safety net for environments where headless isn't available. The config toggle makes the trade-off explicit.

### Severity assigned by the evaluator (not fixture author)

- **Pros:** Evaluator could judge severity based on context
- **Cons:** Non-deterministic — same criterion might be critical in one run and important in another. Makes experiment comparison unreliable.
- **Rejected because:** Severity is a property of the criterion, not the judgment. The fixture author knows which constraints are non-negotiable. The evaluator judges pass/fail, not importance.

## Risks & Mitigations

| Risk | Impact | Likelihood | Mitigation | Rollback |
|---|---|---|---|---|
| Headless evaluator hallucinates file:line evidence | False passes on non-existent evidence | Medium | Skeptical prompt includes "verify files exist before citing." Add post-evaluation check: do cited files/lines exist? | Fall back to subagent mode |
| `claude -p` not available in user's PATH | Headless mode fails silently | Low | Plan command checks `which claude` before invoking headless. If not found, warn and fall back to subagent. | Config: `evaluator.mode: subagent` |
| LLM evaluator is too strict (false fails) | Wasted retries | Medium | Log to `evaluator-tuning.md` after 10 evaluations. Tune skeptical prompt based on false-fail patterns. | Reduce strictness in prompt, or switch to subagent |
| LLM evaluator is too lenient (false passes) | Missed violations | Low (skeptical prompt mitigates) | "When in doubt, FAIL" instruction. Dual-mode: grep pre-check catches obvious violations before LLM runs. | Tighten prompt, add more verify_hint guidance |
| Experiment cost increase | Higher API spend for research | Low | Sonnet default (~$0.01-0.03 per evaluation). Cost capped by experiment N (default N=2-3). | Reduce N, use Haiku, disable `--llm-eval` |

## Security Considerations

- Headless evaluator runs with `--disallowedTools "Write,Edit"` — it MUST NOT modify code
- `--bare` skips hooks and memory — no side effects from evaluation
- Evaluator system prompt is a local file, not fetched from network
- Experiment runner is internal tooling (gitignored), not shipped to users

## Performance Approach

- Headless evaluator adds ~5-15 seconds per phase evaluation (cold start + API call)
- Experiment evaluator adds ~5-15 seconds per run (same)
- Subagent mode is faster (no cold start) but weaker isolation
- Code collection truncates to 15 files to stay within context limits
- For experiments: grep pre-check is instant, LLM evaluator only runs when criteria file exists

## Acceptance Criteria

- AC-007: `--llm-eval` flag triggers LLM evaluation in experiment runner — Verify: run fixture 08 with flag, check `run-01-eval.txt` exists
- AC-008: LLM evaluator produces per-criterion PASS/FAIL with file:line evidence — Verify: inspect evaluator output in `run-01-eval.txt`
- AC-017: Evaluator prompt template contains "assume violations until proven" or equivalent — Verify: `grep -q "assume.*violations\|incomplete until proven" templates/agents/evaluator-headless.md`
- AC-018: When grep says PASS and LLM says FAIL, final verdict is FAIL. When grep says FAIL and LLM says PASS, final verdict is PASS. — Verify: create fixture where they disagree, check final verdict file
- AC-019: Fixture with `severity: informational` criterion — criterion failure does not affect verdict — Verify: run fixture where only informational criterion fails, verdict is PASS
- AC-020: WEAK PASS verdict when all critical pass but 1+ important fails — Verify: run fixture with important-only failure, check verdict string
- AC-021: `evaluator` section in config with preflight, phase-end, mode, max-attempts, model keys — Verify: `/edikt:config` shows evaluator section with all 5 keys
- AC-022: Headless evaluator runs as `claude -p` with `--bare` and `--disallowedTools "Write,Edit"` — Verify: inspect invocation in plan.md
- AC-023: When `evaluator.preflight: false`, plan skips step 11 — Verify: set config, generate plan, confirm no pre-flight output
- AC-024: When `evaluator.phase-end: false`, phase-end evaluation skipped but criteria sidecar still emitted — Verify: set config, complete phase, confirm no evaluation but YAML exists
- AC-025: Evaluator agent files are not user-overridable — `/edikt:upgrade` always overwrites them, `agents.custom` listing is ignored for evaluator — Verify: add evaluator to `agents.custom`, run upgrade, confirm files are overwritten
- AC-026: `/edikt:doctor` warns if evaluator files have been manually modified — Verify: edit evaluator.md, run doctor, check for warning
- AC-027: Plan command blocks with error when evaluator is enabled but template file is missing — Verify: delete evaluator-headless.md, run plan with `evaluator.phase-end: true`, confirm hard failure with reinstall instructions

## Testing Strategy

- **Config tests:** Verify `evaluator.*` keys are in config command's key reference. Test get/set for each key.
- **Template tests:** Verify both `evaluator.md` and `evaluator-headless.md` exist. Both contain skeptical stance language. Headless has no frontmatter.
- **Plan command tests:** Verify plan.md references `evaluator.preflight`, `evaluator.phase-end`, `evaluator.mode`. Verify headless invocation format includes `--bare` and `--disallowedTools`.
- **Experiment runner tests:** Run fixture 08 with `--llm-eval` (requires API access). Verify eval verdict file exists. Verify override logic (requires a fixture where grep and LLM disagree).
- **Severity tests:** Create a fixture with only informational failures — verdict MUST be PASS. Create a fixture with critical fail — verdict MUST be FAIL.

## Dependencies

- SPEC-001 (plan harness) — criteria sidecar provides structured input for the evaluator. If SPEC-001 ships first, the evaluator reads the sidecar. If not, it reads criteria from plan markdown.
- `evaluator.*` config keys — already added to `commands/config.md` in v0.3.1
- `claude` CLI — must be in PATH for headless mode
- `website/governance/evaluator.md` — already created in v0.3.1 with comparison table

## Open Questions

None — all questions resolved during PRD review and spec interview.

---

*Generated by edikt:spec — 2026-04-11*
