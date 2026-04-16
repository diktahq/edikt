---
name: edikt:sdlc:plan
description: "Create execution plan with interview and codebase analysis"
effort: high
argument-hint: "[task, ticket, SPEC-NNN, or PLAN-NNN to continue]"
allowed-tools:
  - Read
  - Write
  - Glob
  - Grep
  - Bash
  - Agent
  - AskUserQuestion
---
!`PLAN_DIR=$(grep "^  plans:" .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"'); if [ -z "$PLAN_DIR" ]; then BASE=$(grep "^base:" .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"' || echo "docs"); PLAN_DIR="${BASE}/plans"; fi; PLAN=$(ls -t "${PLAN_DIR}/"*.md 2>/dev/null | head -1); if [ -z "$PLAN" ]; then PLAN=$(ls -t docs/product/plans/*.md 2>/dev/null | head -1); fi; if [ -n "$PLAN" ]; then NAME=$(basename "$PLAN"); PHASE=$(grep -iE '\|.*in[_ -]progress' "$PLAN" 2>/dev/null | head -1 | tr -d '|' | xargs); printf "<!-- edikt:live -->\nActive plan: %s\nCurrent phase status: %s\n<!-- /edikt:live -->\n" "$NAME" "${PHASE:-(none in progress)}"; fi`

# edikt:plan

Create an optimized execution plan through interview and codebase analysis.

CRITICAL: This command requires live back-and-forth interview with the user. Check immediately whether you are in plan mode:
- If you are in plan mode (you can only describe actions, not perform them), output exactly this and stop:
  ```
  ⚠️  /edikt:sdlc:plan requires an interactive interview and cannot run in plan mode.
  Exit plan mode first, then run /edikt:sdlc:plan again.
  ```
- If you are not in plan mode, proceed normally with the interview.

CRITICAL: NEVER write a plan without running the pre-flight specialist review — skip it only if `--no-review` is explicitly passed.

## Arguments

- `$ARGUMENTS` — Optional. Any of: a task description, ticket ID, SPEC identifier, plan name, or nothing (triggers interview)
- `--eval-only {phase}` — Re-run evaluation for a specific phase in the active plan without re-running the generator. Used to recover from BLOCKED verdicts (ADR-010) after fixing the underlying cause (e.g. switching `evaluator.mode` to headless). `{phase}` is the phase number (1-indexed). Optionally combine with `--plan {slug}` to disambiguate when multiple plans exist. Cannot be combined with a positional task argument.
- `--plan {slug}` — Optional companion to `--eval-only`. Names the plan file by slug (e.g. `v0.4.3-evaluator-headless-default`) when multiple plans exist in `docs/plans/` or `docs/product/plans/`.

## Instructions

0. **Eval-Only routing** — if `$ARGUMENTS` contains `--eval-only N`, skip the interview, codebase analysis, pre-flight specialist review, and phase generation entirely. Route to the **Eval-Only Flow** section in the Reference and stop after it runs. See that section for the isolated flow.

1. Run `/edikt:context` logic to load project context, decisions, product context, and active rules. Read evaluator configuration from `.edikt/config.yaml`:
   - `evaluator.preflight` (default: `true`) — whether to run pre-flight criteria validation
   - `evaluator.phase-end` (default: `true`) — whether to run phase-end evaluation
   - `evaluator.mode` (default: `headless`) — `"headless"` for separate `claude -p`, `"subagent"` for Agent tool
   - `evaluator.max-attempts` (default: `5`) — max retries before stuck. Store as `MAX_ATTEMPTS` for use in the progress table and stuck detection.
   - `evaluator.model` (default: `sonnet`) — model for headless evaluator
   If the `evaluator` section is absent in config, use all defaults.

2. Determine the task from `$ARGUMENTS`. Check in order — use first match:
   - **SPEC identifier** (e.g., `SPEC-005`): find the spec folder, use spec + accepted artifacts as primary context
   - **Ticket ID** (e.g., `GLO-35`, `PROJ-123`): note it for reference, fetch details if MCP is configured
   - **Existing plan name** (e.g., `PLAN-007`, `v0.2.0`): read the plan, ask what the user wants — continue from current phase, re-plan remaining phases, or create a sub-plan for a specific phase
   - **Natural language description** (anything else): use it as the task description directly
   - **Empty**: check the current conversation for context. If a task or feature was being discussed, summarize it and confirm: "It sounds like you want to plan: {summary}. Is that right?" If no conversation context, ask: "What are you planning?"

   **Disambiguation** — when the input is natural language or inferred from conversation (not a SPEC, ticket, or PLAN reference), offer the user a choice before proceeding:
   ```
   How would you like to plan this?

   1. edikt plan — phased execution plan with model assignment, cost estimate,
      codebase analysis, and specialist pre-flight review. Saved to docs/product/plans/.
   2. Quick plan — help you think through the approach right here in conversation.
      No file, no ceremony.
   ```
   If the user picks 2, help them think through the task conversationally — don't run the rest of this command. If the user picks 1, proceed with the full flow.

3. Check the **governance chain** — only when a SPEC was resolved:
   - Read spec frontmatter for `status:`. If not `accepted`, warn the user.
   - Check for spec-artifacts in the spec folder. For each artifact file (excluding `spec.md`), read its status from frontmatter (`status: draft` between `---` markers) or comment header (`status=draft` in `%%`, `#`, `--`, or `<!-- -->` lines).

     If any artifacts have `status: draft`:
     ```
     ⚠️ These spec artifacts are still in draft:
        - {artifact filename} (status: draft)
        - {artifact filename} (status: draft)

        Draft artifacts haven't been reviewed. Planning against them
        risks implementing a design that changes after review.

        Options:
        1. Proceed anyway (plan will note artifacts are unreviewed)
        2. Stop — review and accept artifacts first
     ```

     If the user picks 1:
     - Proceed with plan generation
     - Add a `## Known Risks` section to the generated plan file:
       ```markdown
       ## Known Risks
       - Planning against draft artifacts: {comma-separated list of draft artifact filenames}
         These may change after review. Re-plan if they do.
       ```

     If the user picks 2: stop and output:
     `Review and accept the draft artifacts, then run /edikt:sdlc:plan again.`
   - If artifacts exist and are accepted, read them as planning context.
   - **Inventory all artifacts** — scan the spec directory for every file (excluding `spec.md` itself). Build an artifact inventory:
     ```bash
     ls {spec_dir}/*.yaml {spec_dir}/*.md {spec_dir}/*.mmd {spec_dir}/*.sql {spec_dir}/contracts/*.yaml {spec_dir}/migrations/*.sql 2>/dev/null | grep -v spec.md
     ```
     Categorize each artifact using the Artifact Coverage Table in the Reference section. This inventory is used in step 6b to verify every artifact has plan coverage.

4. Interview: ask 3-6 targeted questions to clarify requirements. Adapt to task type using the Interview Guidance in the Reference section. Present options where applicable.

5. Analyze the codebase using an Agent:
   ```
   Agent(
     subagent_type: "Explore",
     prompt: "Find files and patterns relevant to: {task description}. Look for existing implementations, related tests, config files, and dependencies that will be affected.",
     description: "Scan codebase for plan"
   )
   ```

6. Generate phases. For each phase, assign a model, write a detailed prompt, set a completion promise, max iterations, and dependencies. Use the Phase Structure and Model Assignment guide in the Reference section.

   When generating each phase, populate the `Context Needed:` field by:
   - Scanning spec artifacts referenced by the phase (from the inventory built in step 3)
   - Identifying files produced by dependency phases (from the Artifact Flow Table)
   - Including any ADRs referenced in the spec frontmatter or phase objectives
   Each entry must be a specific file path with a brief description of why it's needed.

6b. **Artifact coverage check** — only when a SPEC was resolved and artifacts were inventoried in step 3:

   For each artifact in the inventory, verify it maps to at least one plan phase. Use the Artifact Coverage Table in the Reference section.

   - **`fixtures*.yaml`** — must have a phase that creates seed data (SQL script, make target, or seeding logic). If missing, add a "Database seeding" phase or append seeding tasks to the final implementation phase.
   - **`test-strategy.md`** — read it. Parse test categories (unit, integration, e2e, edge cases). Either embed test tasks into each relevant implementation phase, or create dedicated test phases. Every test category must map to at least one phase.
   - **`contracts/api*.yaml`** — parse all endpoint definitions (`paths:` section). Every `path + method` pair must appear in at least one phase's prompt or task list. Emit a warning for uncovered endpoints:
     ```
     ⚠ Uncovered API endpoints:
       POST /api/v1/ai/ask (contracts/api-ai.yaml) — no phase implements this
     ```
   - **`contracts/events*.yaml`** — parse channel definitions. Every event must have a producing phase and a consuming phase.
   - **`migrations/*.sql`** — verify each migration has a corresponding phase. Already typically covered, but confirm.
   - **`data-model*.mmd`** — reference only. No phase needed (diagram). Skip.
   - **`config-spec.md`** — if present, verify configuration tasks appear in a phase.

   If any artifact has no coverage, add phases or expand existing phases to cover it. Report:
   ```
   Artifact coverage:
     ✓ fixtures.yaml → Phase 8 (database seeding)
     ✓ test-strategy.md → Phases 2, 4, 6, 8 (tests embedded)
     ✓ contracts/api.yaml → Phases 3, 4, 5 (all 12 endpoints covered)
     ✓ contracts/events.yaml → Phases 5, 6 (3 events covered)
     ⚠ contracts/api-ai.yaml → Phase 7 added (POST /api/v1/ai/ask was uncovered)
     — data-model.mmd → reference only, no phase needed
   ```

6c. **Final artifact validation** — after all phases are finalized (including any added by step 6b):

   Count artifacts with coverage vs without. If any artifact still has no phase coverage after step 6b attempted to fill gaps:

   ```
   ⛔ ARTIFACT COVERAGE INCOMPLETE

   These spec artifacts have no plan coverage:
     contracts/api-ai.yaml — POST /api/v1/ai/ask (no phase implements this)
     fixtures-solution.yaml — no seeding phase for solution data

   The plan cannot be written until all artifacts are covered.
   Add phases for the uncovered artifacts, or confirm they should be skipped.
   ```

   Do NOT write the plan file if artifacts are uncovered — ask the user to resolve first. The user can:
   - `[1]` Add phases for uncovered artifacts
   - `[2]` Mark specific artifacts as "out of scope for this plan" (adds them to a `## Deferred Artifacts` section in the plan)
   - `[3]` Cancel

   If all artifacts are covered:
   ```
   ✓ All spec artifacts have plan coverage ({n}/{n})
   ```

7. Build the dependency graph. Identify phases with no inter-dependencies and group them into execution waves (Wave 1: no dependencies, Wave 2: depends only on Wave 1, etc.).

8. Run pre-flight specialist review (skip if `--no-review` in arguments):
    - Scan the full plan text for domain signals using the Domain Signal table in the Reference section.
    - If no domains detected, output: `Pre-flight: no specialist domains detected — plan looks self-contained.` and skip to step 9.
    - Spawn all applicable specialist agents concurrently (single message, multiple Agent tool calls) using the domain-to-subagent mapping in the Reference section.
    - Each agent reads the plan, reviews from their domain lens only, and returns findings with severity.
    - Output the consolidated pre-flight review using the Pre-Flight Output Format in the Reference section.
    - If user provides updates, incorporate them into the plan phases. If user skips, note outstanding findings for the Known Risks section.

9. Run pre-flight criteria validation on every phase's acceptance criteria:
    - If `evaluator.preflight` is false, skip this step entirely. Output: "Pre-flight validation skipped (evaluator.preflight: false in config)." Proceed to step 10.
    - For each phase, invoke the evaluator agent in **pre-flight mode** (see `templates/agents/evaluator.md` Pre-flight Mode section).
    - The evaluator classifies each acceptance criterion as TESTABLE, VAGUE, SUBJECTIVE, or BLOCKED.
    - For TESTABLE criteria, the evaluator proposes a verification command.
    - If any criteria are VAGUE or SUBJECTIVE, surface the evaluator's rewrites inline — ask the user to accept or edit before finalizing the plan.
    - If the evaluator verdict is ABORT (50%+ criteria untestable), flag: "Phase {N} has untestable acceptance criteria. Rewrite before implementing."
    - This step prevents wasted iterations on criteria the evaluator cannot judge at phase end.

10. Write the plan file to `docs/product/plans/PLAN-{slug}.md` (or `docs/plans/` if product dir doesn't exist). Use the Plan File Template in the Reference section. Incorporate any findings from the pre-flight review (step 8) — add a `## Known Risks` section if there are outstanding findings the user chose not to address.

10b. **Emit criteria sidecar** — after writing the plan markdown, write `PLAN-{slug}-criteria.yaml` to the same directory.

   For each phase:
   - Extract acceptance criteria from the plan text
   - Assign IDs: `AC-{phase}.{seq}` (e.g., AC-1.1, AC-1.2)
   - If pre-flight criteria validation ran (step 9), populate `verify` with the proposed commands from the evaluator
   - Set all `status: pending`, `fail_count: 0`, `fail_reason: null`, `last_evaluated: null`

   Schema must match `docs/product/specs/SPEC-001-plan-harness/plan-criteria-schema.yaml`. Top-level fields: `plan`, `generated`, `last_evaluated: null`, `phases[]`.

   The sidecar file is always a sibling of the plan file:
   `docs/product/plans/PLAN-foo.md` → `docs/product/plans/PLAN-foo-criteria.yaml`

11. Output next steps:
   ```
   ✅ Plan saved: {path}

   Execution Strategy:
     Wave 1: Phase {n}, {m} (parallel)
     Wave 2: Phase {x}
     Wave 3: Phase {y}

   Estimated cost: ${total}

   Next: Review the plan, then execute Phase 1.
   ```

## Reference

### Interview Guidance

Adapt questions to what was provided. Skip questions the input already answers.

- **Feature work:** "Should this be behind a feature flag?", "What's the data model?"
- **Refactoring:** "What's the migration strategy?", "Can we do it incrementally?"
- **Bug fix:** "Can you reproduce it?", "What's the impact?"
- **Natural prompt** (e.g., "plan how to refactor compile"): clarify scope and constraints — "What outcome do you want?", "Any constraints I should know?", "Should this be backward compatible?"
- **Continuing a plan** (PLAN-NNN): "Which phase are you on?", "Did the approach change?", "Want to re-plan the remaining phases or create a sub-plan for one phase?"
- **From conversation context** (empty args, ongoing discussion): summarize what was discussed and confirm before interviewing — don't re-ask what was already covered

### Model Assignment

| Model | Cost/phase | Best for |
|---|---|---|
| Haiku | ~$0.01 | Database migrations, config files, simple CRUD, documentation, scripts |
| Sonnet | ~$0.08 | Business logic, UI components, API integrations, refactoring, complex tests |
| Opus | ~$0.80 | Security, algorithms, architecture, complex debugging, novel problems |

### Phase Structure

Each phase requires:
- Number (e.g., 1, 2, 3)
- Title
- Objective (one sentence)
- Model recommendation with reasoning
- Detailed prompt (full implementation instructions — be specific and self-contained)
- Completion promise (shell-safe: uppercase, numbers, spaces, dots ONLY)
- Acceptance criteria (binary PASS/FAIL assertions for the evaluator)
- Evaluate flag (true/false — whether phase-end evaluation runs)
- Max iterations (based on complexity)
- Dependencies (which phases must complete first)
- Context Needed (list of file paths the generator must read before starting this phase — spec artifacts, outputs from dependency phases, referenced ADRs)

### Phase Startup Directive

Before implementing any plan phase:
1. Read every file listed in that phase's Context Needed section.
2. If a listed file does not exist, check the progress table — the producing phase may not be complete.
3. Do not proceed until all context files have been read.
4. After reading, confirm you understand the relevant decisions and constraints before writing code.
5. If the phase references any spec artifacts in its Context Needed section, check their status:
   - For each referenced artifact with `status: accepted`, update it to `status: in-progress`:
     - `.mmd` files: change `status=accepted` to `status=in-progress` in the `%% edikt:artifact` comment
     - `.yaml` files: change `status=accepted` to `status=in-progress` in the `# edikt:artifact` comment
     - `.sql` files: change `status=accepted` to `status=in-progress` in the `-- edikt:artifact` comment
     - `.md` files: change `status: accepted` to `status: in-progress` in YAML frontmatter, or `status=accepted` to `status=in-progress` in `<!-- edikt:artifact -->` comment
   - Output: `Status promoted: {artifact} accepted → in-progress`
   - Do not update artifacts already `in-progress`, `implemented`, or `superseded`

### Acceptance Criteria Rules

Acceptance criteria are for the evaluator, not the generator. They must be:
- **Binary** — PASS or FAIL, no "partially met"
- **Testable** — verifiable by reading code, running a test, or grepping
- **Specific** — name the file, function, endpoint, or pattern
- Never subjective — "API is fast enough" fails. "GET /users responds with 200 and valid JSON" passes.

If the spec has acceptance criteria (AC-NNN), inherit them per phase. If not, generate them from the phase objectives.

### Conditional Evaluation

Each phase has an `evaluate:` flag:
- `true` (default for `effort: high` phases) — phase-end evaluator runs after completion
- `false` (default for `effort: low` phases) — skip evaluation, go straight to context reset guidance
- Author can override in either direction

### Status Values

| Status | Meaning |
|--------|---------|
| `pending` | Not started |
| `in-progress` | Generator is working |
| `evaluating` | Phase-end evaluator is running |
| `blocked` | Evaluator could not verify — missing capability (Bash denied, test runner unavailable, etc.). Phase NOT verified. |
| `done` | All acceptance criteria PASS |
| `stuck` | Max attempts reached — human decision needed |
| `skipped` | Explicitly skipped by user |

### Phase-End Flow

When a phase completes (generator outputs the completion promise):

1. **If `evaluate: true` AND `evaluator.phase-end` is true:**

   **a. EVALUATOR FILE CHECK** — before invoking, verify the template exists:
   - If `evaluator.mode` is `"headless"`: check that `templates/agents/evaluator-headless.md` exists (also check `~/.edikt/templates/agents/evaluator-headless.md` for global install)
   - If `evaluator.mode` is `"subagent"`: check that `templates/agents/evaluator.md` exists (also check `.claude/agents/evaluator.md`)
   - If the required file is missing:
     ```
     ❌ Evaluator template missing — cannot run evaluation.
        Expected: {path}
        Run: curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
        Or disable evaluation: /edikt:config set evaluator.phase-end false
     ```
     Do NOT silently skip evaluation. This is a hard failure.

   **b. PRIMARY MODE invocation:**
   - If `evaluator.mode` is `"headless"`: attempt headless first. Invoke via Bash tool:
     ```bash
     claude -p "{evaluation prompt with criteria + file list}" \
       --system-prompt "$(cat {path to evaluator-headless.md})" \
       --allowedTools "Read,Grep,Glob,Bash" \
       --disallowedTools "Write,Edit" \
       --model {evaluator.model} \
       --output-format json \
       --bare
     ```
     The evaluation prompt (user message) must include:
     - The phase's acceptance criteria (from criteria sidecar if available, or from plan markdown)
     - The list of files modified during the phase (from `git diff --name-only` or phase output)
     - The project's test command if available

     Parse the evaluator's JSON output to extract per-criterion PASS/FAIL/BLOCKED verdicts.

   - If `evaluator.mode` is `"subagent"`: go directly to subagent invocation in step c.iii below.

   **c. HEADLESS FAILURE HANDLING** (only when `evaluator.mode` was `"headless"` and the invocation above did not return a usable verdict):

   i. **Classify the failure:**
      - spawn error (`claude` CLI not found: ENOENT or "command not found")
      - non-zero exit
      - auth error (stderr contains "authentication" or "not logged in")
      - timeout (>60s no response)
      - JSON parse failure (`--output-format json` returned malformed output)

   ii. **Emit the fallback warning banner** (exact format — downstream tests grep for the header):
      ```
      ⚠ EVALUATOR FALLBACK
      ━━━━━━━━━━━━━━━━━━━━
        Headless mode failed: {reason}
        Falling back to subagent mode.
        ⚠ Bash execution may be denied in subagent mode — test criteria may return BLOCKED.
        To fix: {actionable hint based on reason, e.g.:
          - spawn error: "install Claude Code CLI or add `claude` to PATH"
          - auth error: "run `claude login` or set ANTHROPIC_API_KEY"
          - timeout: "check network / increase timeout"
          - JSON parse: "file an edikt issue — claude -p output changed"}
      ━━━━━━━━━━━━━━━━━━━━
      ```

   iii. **Invoke the subagent evaluator** via the Agent tool with the phase's acceptance criteria, code changes, and test results. This is the existing subagent path and is also the primary path when `evaluator.mode: "subagent"`.

   iv. **If the subagent also fails**, emit the hard-failure banner (exact format — downstream tests grep for the header):
      ```
      ✗ EVALUATION FAILED
      ━━━━━━━━━━━━━━━━━━━━
        Headless: {reason}
        Subagent: {reason}
        Phase {N} marked UNVERIFIED in progress table.
        Recovery:
          1. Run /edikt:doctor to diagnose evaluator setup
          2. Or skip with: /edikt:config set evaluator.phase-end false (not recommended)
      ━━━━━━━━━━━━━━━━━━━━
      ```
      Set the phase status to `stuck` with the reason "evaluation failed in both modes" and wait for user input.

   **d. Process the verdict:** Wait for PASS / FAIL / BLOCKED verdict.
   - If PASS: proceed to context reset guidance
   - If FAIL: report failures, then:

     i. **Increment the Attempt column** in the progress table (e.g., `1/{max}` → `2/{max}`).

     ii. **Check criteria sidecar** — read `PLAN-{slug}-criteria.yaml` if it exists. For each failing criterion, check `fail_count`. If `fail_count >= 3`:
        ```
        ⚠️ AC-{id} has failed 3 consecutive times.
           Last reason: {fail_reason}
           Consider: rewrite the criterion, adjust the approach, or ask for help.
        ```

     iii. **Forward failures** — before retrying, include the failing criteria in the generator prompt:
        ```
        Previous attempt failed. Fix these: {list of failing AC IDs and reasons}
        ```

     iv. **Stuck detection** — if the Attempt value has reached `MAX_ATTEMPTS` (e.g., `5/5`), set the phase status to `stuck` and output:
        ```
        Phase {n} is stuck after {max} attempts.
        Options:
          1. Continue trying (increase max)
          2. Skip this phase
          3. Rewrite failing criteria
          4. Stop and review
        ```
        Wait for the user's choice before proceeding.

     v. **Update criteria sidecar** — after evaluation, update `PLAN-{slug}-criteria.yaml`:
        - Read the sidecar file (skip silently if it doesn't exist)
        - For each criterion the evaluator judged: update `status` (one of `pass | fail | blocked`), `last_evaluated` (ISO date), `fail_reason` (if fail), `block_reason` (if blocked)
        - Increment `fail_count` for each fail (reset to 0 on pass; do NOT reset or increment on blocked)
        - Update phase-level `status` and `attempt`
        - Write back to the same file

   - If BLOCKED: evaluator could not verify one or more criteria due to a missing capability (Bash denied, test runner unavailable, etc.). This is a capability failure, not a generator failure. Retrying the generator does not help.

     i. **Do NOT increment the Attempt column.** The generator's work may be correct — we simply couldn't verify it.

     ii. **Set the phase status to `blocked`** in the progress table. Do not mark `done`, `stuck`, or advance to the next phase.

     iii. **Output each blocked criterion** with its recovery hint from the evaluator:
        ```
        AC-{id}: BLOCKED — {reason}
          Recovery: {hint from evaluator}
        ```

     iv. **Output the phase-level recovery prompt:**
        ```
        Phase {N} BLOCKED — evaluator could not verify {n} criteria.

        Options:
          1. Switch to headless mode (recommended):
             /edikt:config set evaluator.mode headless
          2. Grant Bash to subagents (session-level):
             Add "Bash" to permissions.allow in .claude/settings.local.json
          3. Skip evaluation for this phase (not recommended):
             /edikt:config set evaluator.phase-end false

        After applying a fix, re-run evaluation for this phase with:
          /edikt:sdlc:plan --eval-only {N}
        ```

     v. **Update criteria sidecar** — for blocked criteria: set `status: blocked`, record `block_reason: {reason from evaluator}`, set `last_evaluated: {ISO date}`. Do NOT increment `fail_count`. Do NOT reset `fail_count`. Update phase-level `status: blocked`. Write back.

     vi. **Do not proceed to context reset guidance.** Stop and wait for the user to apply a fix.

   Schema note for the sidecar: `status` may be one of `pending | in-progress | pass | fail | blocked`. A new optional field `block_reason` parallels `fail_reason`.

2. **If `evaluate: true` AND `evaluator.phase-end` is false:**
   Skip evaluation. Output: "Phase-end evaluation skipped (evaluator.phase-end: false in config)."
   The criteria sidecar is still updated with criteria status remaining as `pending` (not evaluated).
   Proceed directly to context reset guidance.

3. **Context reset guidance** (always, after evaluation or if `evaluate: false`):
   ```
   Phase {n} complete. For best results on Phase {n+1}:
     1. Start a new session
     2. Run /edikt:context
     3. Continue with Phase {n+1}

   State is saved in the plan file — nothing is lost.
   ```

### Eval-Only Flow

Invoked when `$ARGUMENTS` contains `--eval-only N`. This flow skips interview, codebase analysis, pre-flight review, and phase generation — it re-runs only the Phase-End Flow against an existing phase. Use it to recover from BLOCKED verdicts (ADR-010) after the user has fixed the underlying cause (e.g. switched `evaluator.mode` to headless).

1. **Parse arguments:**
   - Extract `N` from `--eval-only N`. Reject if `N` is not a positive integer: output `[FAIL] --eval-only requires a positive integer phase number (e.g. --eval-only 2)` and stop.
   - If `$ARGUMENTS` also contains a positional task argument alongside `--eval-only`, output `[FAIL] cannot combine --eval-only with a new plan task — use one or the other` and stop.
   - Extract `--plan {slug}` if present.

2. **Locate the active plan:**
   - Read `paths.plans` from `.edikt/config.yaml` (default: `docs/plans`). Candidates in order: `{paths.plans}/PLAN-*.md`, `docs/product/plans/PLAN-*.md`.
   - If `--plan {slug}` was provided: match a file whose name contains `{slug}`. If none: `[FAIL] No plan matching --plan {slug} found in {paths.plans}/ or docs/product/plans/` and stop.
   - If no `--plan` and exactly one plan file exists: use it.
   - If no `--plan` and multiple plans exist: list them and output `[FAIL] Multiple plans found. Specify which with --plan {slug}:\n  {list}` and stop.
   - If no plan file exists: `[FAIL] No plan file found. Run /edikt:sdlc:plan to create one first.` and stop.

3. **Locate phase N in the plan:**
   - Read the plan's Progress table. Confirm a row for phase N exists. If not: `[FAIL] Phase {N} does not exist in {plan}` and stop.
   - Read the plan's `## Phase {N}:` section to extract:
     - Acceptance criteria (list items under `**Acceptance Criteria:**`)
     - Files modified (from `git diff --name-only` relative to the last commit touching the phase, falling back to the phase's `Context Needed` list)

4. **Invoke the Phase-End Flow** (the existing `### Phase-End Flow` section of this command):
   - Pass phase number, criteria, and file list
   - Use the same `evaluator.mode` routing as regular evaluation — headless first if configured, fallback to subagent on headless failure, BLOCKED handling unchanged
   - All the visible output (banners, BLOCKED per-criterion rows, recovery prompt) is identical

5. **Update state for phase N only:**
   - Update the progress table row for phase N only — other rows untouched
   - Update the criteria sidecar (`PLAN-{slug}-criteria.yaml`) for phase N's criteria only
   - Do NOT advance to context reset guidance — this is a one-off re-evaluation, not phase completion

6. **Output the verdict:**
   ```
   ✓ Phase {N} re-evaluated
     Previous status: {old status}
     New status: {new status}
     {If verdict changed:}
       Criteria now passing: {list of AC-IDs}
       Criteria still failing/blocked: {list with reasons}
   ```

Constraints:
- Do not duplicate Phase-End Flow logic — always route through that section
- Do not add new config keys — reuse `evaluator.mode`, `evaluator.model`, `evaluator.phase-end`
- Do not modify phases other than N
- Do not run the pre-flight criteria validation (step 9 of main flow) — that is only for new plans

### Completion Promise Rules

Promises are used in automation, so they MUST be shell-safe:
- ONLY: uppercase letters, numbers, spaces, dots
- NO: `>`, `<`, `|`, `&`, `$`, backticks, `!`, `'`, `"`, arrows
- Keep SHORT: 2-4 words max
- Good: `PHASE 1 COMPLETE`, `MIGRATION DONE`, `API READY`, `TESTS PASSING`
- Bad: anything with special characters or lowercase

### Artifact Coverage Table

When a plan is generated from a SPEC, every artifact in the spec directory must map to plan coverage:

| Artifact pattern | Required coverage | Action if missing |
|---|---|---|
| `fixtures*.yaml` | Phase that creates seed data (SQL script, make target, or programmatic seeding) | Add "Database seeding" phase |
| `test-strategy.md` | Each test category (unit, integration, e2e, edge cases) mapped to at least one phase | Embed test tasks in implementation phases or add dedicated test phases |
| `contracts/api*.yaml` | Every `path + method` in the OpenAPI spec appears in at least one phase | Add phase for uncovered endpoints, or expand existing phases |
| `contracts/events*.yaml` (AsyncAPI) | Every channel/event has a producing phase and consuming phase | Add event handling to relevant phases |
| `migrations/*.sql` | Each migration has a corresponding phase | Usually already covered — verify |
| `data-model*.mmd` | Reference only | No phase needed (diagram) |
| `config-spec.md` | Configuration tasks appear in a phase | Add config setup to relevant phase |

CRITICAL: Artifacts that are generated but never consumed by the plan produce silent failures — features that "complete" but don't work (no seed data, no tests, missing endpoints). The artifact coverage check in step 6b prevents this.

### Domain Signal Detection

| Domain | Signals | Agent |
|---|---|---|
| Database | SQL, query, schema, migration, index, database, db, table, foreign key, join, transaction, ORM, Postgres, MySQL, SQLite, MongoDB | `dba` |
| Infrastructure | deploy, docker, kubernetes, k8s, terraform, helm, CI, CD, infra, container, Dockerfile, compose, nginx, AWS, GCP, Azure, cloud | `sre` |
| Security | auth, JWT, OAuth, payment, PCI, HIPAA, token, secret, encrypt, credential, password, permission, role, RBAC, CORS, XSS, injection | `security` |
| API | API, endpoint, REST, GraphQL, route, webhook, contract, openapi, swagger, versioning, breaking change | `api` |
| Architecture | bounded context, domain, architecture, refactor, pattern, layer, dependency, coupling, abstraction, interface, hexagonal, clean arch | `architect` |
| Performance | performance, N+1, cache, latency, throughput, slow, optimize, index, query optimization, benchmark | `performance` |

### Pre-Flight Severity

- 🔴 Critical: must address before execution (data loss, security breach, broken contract)
- 🟡 Warning: should address, not blocking
- 🟢 OK: domain looks healthy

### Pre-Flight Output Format

```
PRE-FLIGHT REVIEW
─────────────────────────────────────────────────────
Domains detected: {list} ({n} of 6 checked)

{AGENT NAME}
  #1 🔴  {finding} ({file:line if applicable})
  #2 🟡  {finding}
  #3 🟢  {positive finding}

{AGENT NAME}
  #4 🔴  {finding}
  #5 🟡  {finding}

─────────────────────────────────────────────────────
{N critical, N warnings}. Which findings should I address?
(e.g., #1, #4 or "all critical" or "skip")
```

### Plan File Template

```markdown
# Plan: {Title}

## Overview
**Task:** {description or ticket ID}
**Total Phases:** {n}
**Estimated Cost:** ${cost}
**Created:** {date}

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | pending | 0/{max} | -      |
| 2     | pending | 0/{max} | -      |

**IMPORTANT:** Update this table as phases complete. This table is the persistent state that survives context compaction.

A `blocked` status means evaluation couldn't run — see Status Values table. The phase is NOT verified until re-evaluated successfully (`/edikt:sdlc:plan --eval-only {N}`).

## Model Assignment
| Phase | Task | Model | Reasoning | Est. Cost |
|-------|------|-------|-----------|-----------|
| 1 | {task} | haiku | {why} | $0.01 |

## Execution Strategy
| Phase | Depends On | Parallel With |
|-------|-----------|---------------|
| 1     | None      | 2             |
| 2     | None      | 1             |
| 3     | 1, 2      | -             |

## Artifact Flow

| Producing Phase | Artifact | Consuming Phase(s) |
|-----------------|----------|---------------------|
| {n} | `{file path}` | {phase numbers} |

## Phase 1: {Title}

**Objective:** {brief description}
**Model:** `{model}`
**Max Iterations:** {n}
**Completion Promise:** `{SHELL SAFE PROMISE}`
**Evaluate:** {true | false}
**Dependencies:** {None or phase numbers}
**Context Needed:**
- `docs/product/specs/SPEC-NNN/contracts/api-orders.yaml` — API contract from spec artifacts
- `internal/repository/orders.go` — repository created in Phase 2
- `docs/architecture/decisions/ADR-012.md` — error handling decision

**Acceptance Criteria:**
- [ ] {Binary assertion — e.g., "Cache adapter implements get/set/delete with TTL parameter"}
- [ ] {Binary assertion — e.g., "Unit tests cover cache miss, hit, and TTL expiration"}
- [ ] {Binary assertion — e.g., "Integration test hits real Redis instance"}

**Prompt:**
```
{Full detailed implementation instructions.
Reference specific file paths, patterns to follow, tests to write.
This is where all the detail goes — be thorough.
The prompt should be self-contained: someone reading only this section
should be able to implement the phase without other context.

When complete, output: {COMPLETION PROMISE}
}
```

---

{repeat for each phase}
```
