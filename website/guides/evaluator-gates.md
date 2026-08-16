# Configuring Evaluator Gates

edikt enforces completion claims through three layered gates: the **evaluator agent** (LLM-mediated, judgment-heavy), the **verify runner** (mechanical, exit-code-driven), and the **stop-hook detector** (runtime safety net). This page covers how they compose, what each one decides, and how to tune them via `.edikt/config.yaml`.

The short version:

| Gate | What it judges | How |
|---|---|---|
| Evaluator | "Is this acceptance criterion satisfied?" | LLM verdict (PASS / FAIL / BLOCKED) per criterion |
| Verify runner | "Does the declared shell check actually pass?" | `bash -c "<command>"`, exit 0 = pass |
| Stop-hook detector | "Did the agent claim done without evidence?" | Regex on completion phrases + plan-state lookup, soft `systemMessage` |

The verify runner is the hardest gate — exit code 1 blocks state transitions. The evaluator is the most nuanced — it can return BLOCKED to surface ambiguity rather than guess. The stop-hook is purely advisory — it never blocks, only warns.

## The three gates in detail

### Evaluator agent

Dispatched by `/edikt:sdlc:plan` at two points in a phase's lifecycle:

- **Pre-flight** — before the phase starts. Validates that every acceptance criterion is **testable** (mechanically checkable), not vague ("works well", "feels right"). Returns BLOCKED when criteria need rewriting; the plan command refuses to start the phase until they're addressed.
- **Phase-end** — after the generator agent finishes. Reads the evidence (test outputs, file content, git diff) and emits a PASS / FAIL / BLOCKED verdict per criterion.

The evaluator is configured under `evaluator:` in `.edikt/config.yaml`:

```yaml
evaluator:
  preflight: true          # gate the phase start on criteria quality
  phase_end: true          # gate phase completion on per-criterion verdicts
  mode: headless           # "headless" (CLI, no Task tool) | "subagent" (Task fork)
  model: claude-opus-4-7   # which model to use
```

`mode: headless` is the default for CI-friendly determinism. `subagent` enables interactive subagent dispatch — useful when you want the evaluator to surface context from sibling files.

### Verify runner

Configured per-sidecar via the optional `verify:` field. Each `verify:` is a single shell command run by `bin/edikt verify`:

```yaml
# In a gov sidecar (ADR, invariant, guideline)
directives:
  - text: "..."
    source_excerpt: {...}
    verify: "! rg -P 'echo.*\\{.*\\}' templates/hooks/*.sh"

# In a PRD or SPEC sidecar
acceptance_criteria:
  - id: AC-001-1
    given: "..."
    when: "..."
    then: "..."
    verify: "bash test/integration/renewal_reminder.sh"
```

See [Falsifiable Verification](/guides/falsifiable-verification) for the full verify-runner contract — execution model, timeout, exit codes, and `verify_kind`.

Wired into every claim-bearing path:

- `bin/edikt gov compile` runs `verify all` after Phase B.
- `/edikt:adr:new`, `/edikt:invariant:new`, `/edikt:guideline:new` run `verify gov <ID>` post-write.
- `/edikt:sdlc:prd ship` and `supersede` run `verify prd <PRD-ID>` first; refuse on failure.
- `/edikt:sdlc:drift` invokes `verify spec <SPEC-ID>` and folds failures into the drift report.
- `/edikt:doctor` reports coverage as a soft signal.

There is no config switch for the verify runner — it's always on. The opt-out is per-call (`gov compile --skip-verify`, `prd supersede --force-verify`) and each opt-out is loud (recorded in `revision_history` for supersede; mentioned in compile output).

### Stop-hook completion-claim detector

Runtime safety net in `templates/hooks/stop-hook.sh`. Detects completion phrases (`✓`, `Done!`, `All tests pass`, `Phase N complete`, …) during in-progress plan phases and emits a non-blocking `systemMessage` suggesting `bin/edikt verify <plan-id> --phase <N>`.

Activation predicate (all three must hold):

1. `features.quality-gates: true` in `.edikt/config.yaml` (the default).
2. The last assistant message matches a completion phrase.
3. At least one plan under `paths.plans` has a row with `status: in-progress`.

The detector is **advisory** — `systemMessage` only, never `decision: block`. The user is the final judge. Setting `features.quality-gates: false` opts out entirely.

See [Gates → Completion-claim detector](/governance/gates#completion-claim-detector) for the full predicate.

## How the gates compose

The three gates fire at different points in the lifecycle:

```text
   Plan phase starts                    Phase ends
   ─────────────────                    ──────────
   │                                    │
   ├── pre-flight evaluator             ├── phase-end evaluator
   │   (BLOCKED → fix criteria)         │   (FAIL → blocks done flip)
   │                                    │
   └─→ generator agent runs             └─→ verify runner (--phase N)
                                            (exit 1 → blocks done flip)


   Any moment during a turn:
   ─────────────────────────
   stop-hook detector                   ── advisory only ─→ systemMessage


   Other completion paths:
   ───────────────────────
   gov compile  ───→ verify all  (post-merge)
   adr/inv/guideline:new ───→ verify gov <ID>  (post-write)
   prd ship     ───→ verify prd <PRD-ID>  (pre-transition)
   drift        ───→ verify spec <SPEC-ID>  (folded into report)
```

The evaluator and the verify runner are **independent gates** — both must pass for a plan phase to flip `done`. A phase with a hand-written verify command that exits 0 still needs the evaluator to confirm the AC is satisfied; a phase the evaluator passes still needs verify to confirm the mechanical check holds. This is intentional: the LLM's judgment and the shell's exit code catch different classes of regression.

## Tuning

### Disable the stop-hook detector

```yaml
features:
  quality-gates: false
```

This also disables the agent-driven specialist gates (`security`, `dba`, etc.) — see [features.quality-gates](/governance/features#quality-gates). If you only want the completion-claim detector off, file an issue; we'll add a separate switch.

### Disable evaluator pre-flight

```yaml
evaluator:
  preflight: false
```

Useful when prototyping a plan structure and you don't want the AC-quality gate firing on every iteration. Phase-end stays on by default.

### Bypass a verify gate (deliberate)

- `bin/edikt gov compile --skip-verify` — explicit opt-out, loud in the output.
- `/edikt:sdlc:prd PRD-NNN supersede --force-verify` — recorded in `revision_history` so the audit trail captures the override.

There is no opt-out for `verify gov` after `/edikt:adr:new` — failures are surface-only (warning, no rollback), so there's no transition to bypass. The user fixes the directive or removes the bad `verify:` line.

## When the gates conflict

You'll occasionally see an evaluator PASS alongside a verify FAIL (or vice versa). The resolution depends on which is right:

- **Evaluator PASS, verify FAIL** — usually means the verify command is mis-written (wrong path, brittle regex, missing dependency). Read the captured stderr in `.edikt/state/verify/<run>.json`, fix the command, re-run.
- **Evaluator FAIL, verify PASS** — usually means the verify command is too lenient (e.g., `! grep BAD-PATTERN .` passes when the directory is empty). Tighten the predicate.
- **Both BLOCKED** — the criterion is genuinely ambiguous. Rewrite the AC, then re-run pre-flight.

The two gates are deliberately redundant. If you find yourself disabling one to make the other quieter, something is off — the right move is to fix whichever gate is producing the false signal.

## Related

- [Falsifiable Verification](/guides/falsifiable-verification) — the verify-runner contract, `verify_kind`, and the pre-edit gate
- [`edikt verify`](/commands/verify) — the runner that backs the verify gates
- [`/edikt:sdlc:plan`](/commands/sdlc/plan) — orchestrates the evaluator gates
- [Gates](/governance/gates) — specialist agents + completion-claim detector
- [Features](/governance/features) — config switches for the gate stack
