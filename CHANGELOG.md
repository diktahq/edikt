# edikt changelog

## v0.5.0 (2026-04-17)

### Security hardening (PLAN-v0.5.0-security-hardening)

Full security audit findings closed before release. Six new invariants (INV-003 through INV-008) and four new ADRs (ADR-016 through ADR-019) capture the failure classes so they can't recur. Source audit: `docs/reports/security-audit-v0.5.0-2026-04-17.md`.

#### Critical fixes

- **Hooks no longer concatenate JSON via shell.** Every hook that emits protocol JSON or writes `events.jsonl` now uses `python3 json.dumps` with untrusted values passed as argv. Attacker-controlled file paths, error messages, config answers, and agent findings can no longer corrupt or inject into the hook protocol. (INV-003; closes CRIT-1, CRIT-2, CRIT-4, CRIT-5, HI-1, HI-2, MED-1)
- **Hooks no longer instruct Claude to execute shell built from untrusted text.** `subagent-stop.sh` redesigned: the hook writes `events.jsonl` itself; the `systemMessage` is static with no agent-derived substring. The previous design had a single-quote breakout that reached RCE. (INV-004; closes CRIT-1 RCE vector)
- **Plan filenames and evaluator model names are shape-validated before flowing into `claude -p` argv.** A plan file named `PLAN-x"; rm -rf ~; ".md` or a config `EVAL_MODEL` of `sonnet --bypass` no longer reaches the evaluator prompt. (INV-006; closes CRIT-3)
- **Release integrity: Sigstore keyless signing of `SHA256SUMS`.** Every release now ships `SHA256SUMS.sig.bundle` produced by the workflow's GitHub OIDC identity. Installers verify with `cosign verify-blob` against a regex identity matching the release workflow at any tag. (ADR-016; closes CRIT-6, CRIT-7)
- **README install URL pinned to a specific tag.** `raw.githubusercontent.com/.../main/` is forbidden (INV-008). A push to `main` can no longer affect new installs. (closes CRIT-7)

#### High-severity fixes

- **Sentinel regions are guarded by byte-range overlap, not regex.** An Edit whose `old_string` is a non-sentinel line inside a managed region is now blocked. Bypass requires explicit `EDIKT_COMPILE_IN_PROGRESS=1` or `EDIKT_MIGRATION_IN_PROGRESS=1`. (INV-005; closes HI-4)
- **Evaluator verdicts are now structured JSON with per-criterion `evidence_type`.** The plan harness rejects `PASS` unless every criterion that names a shell command has `evidence_type: "test_run"`. A coerced PASS (“just trust me, I ran the tests manually”) is forced to `BLOCKED`. Existing in-flight plans are grandfathered on upgrade. (ADR-018; closes HI-7)
- **`settings.json.tmpl` ships with a deny-by-default `permissions` block.** 23 deny patterns cover destructive Bash (rm -rf roots, chmod 777, sudo, fork bomb, dd, mkfs, force-push to main), plaintext HTTP fetches, and sensitive file reads (/etc/shadow, SSH keys, AWS credentials). 17 explicit allow entries cover edikt's operational tools. `defaultMode: askBeforeAllow`. See `docs/guides/permissions.md`. (ADR-017; closes HI-9)
- **Benchmark sandboxes are hermetic.** `runner.py` no longer copies the host's `~/.claude/settings.json` or hooks into the benchmark project. `setting_sources` restricted to `["project"]`. A curated minimal `settings.json` is written per sandbox. (INV-007; closes HI-10)
- **Benchmark JSONL outputs are redacted.** `tool_calls[*].tool_input.content` is replaced with `<redacted:len=N>`, responses are length-capped at 4096 chars, and the write aborts if any credential pattern (sk-ant-, Bearer, -----BEGIN, AKIA, ghp_) matches in the serialized output. (closes HI-11)
- **Stop-hook suggestions no longer embed attacker substrings.** Matched route paths and env var names are no longer interpolated into `systemMessage`. Suggestions are static ("📄 New HTTP route referenced — consider /edikt:docs:review"). (closes HI-5)
- **Benchmark scoring normalizes with NFKC + casefold + whitespace-strip.** `evil.PY ` (trailing space) and `evil.tѕ` (Cyrillic `s`) are no longer false-PASS. (INV-006; closes HI-6)

#### Medium & low fixes (selection)

- Test harness `.env` loader allowlists keys (`ANTHROPIC_*`, `CLAUDE_*`, `EDIKT_*`). Dangerous prefixes (`LD_`, `DYLD_`, `PATH`, `PYTHONPATH`, `PYTHONSTARTUP`) raise a loud error. (MED-2)
- Claude session detection requires JSON with a credential-carrying field; empty sessions/*.json no longer satisfies the auth gate. (MED-3)
- Retry classifier whitelists specific exception classes; 401 auth errors re-raise immediately instead of being classified as upstream outages. (LOW-9)
- `migrate_m2_claudemd_sentinels` closes the TOCTOU window via `O_NOFOLLOW` on open + re-check before atomic rename. (HI-3)
- Ancestor-walk bounded at `$HOME` with ownership check, so `/tmp/.edikt/config.yaml` planted by another local user on a shared system is ignored. (MED-6)
- `events.jsonl` chmod 0600 on create — no longer world-readable. (MED-8)
- `post-tool-use.sh` validates the file path against `[A-Za-z0-9_./-]` before invoking the formatter. (MED-14)
- New attack corpus entries: `evaluator_coercion`, `sentinel_escape`, `agent_identity_spoof`, plus per-directive `must_cite_*` variants.

#### Governance (new)

- **INV-003** — Hooks emit structured JSON, never shell-concatenated strings.
- **INV-004** — Hooks must not instruct Claude to execute shell built from untrusted text.
- **INV-005** — Managed-region integrity is verified before overwrite (byte-range for markdown, sidecar hash for JSON).
- **INV-006** — Externally-controlled inputs are shape-validated before use.
- **INV-007** — Benchmark and test sandboxes are hermetic.
- **INV-008** — Release install URLs are tag-pinned, never branch-tracking.
- **ADR-016** — Release integrity and Sigstore keyless signing (supersedes ADR-013).
- **ADR-017** — Default permissions posture in `settings.json.tmpl`.
- **ADR-018** — Evaluator verdict schema with per-criterion `evidence_type`.
- **ADR-019** — Narrow carve-out of ADR-014 for four security-rewritten hooks.

#### Breaking changes (upgrade notes)

- **New default permissions may prompt.** First-time Claude invocations of `curl http://` (non-TLS) or other denied patterns now produce a permission prompt. Allow once if legitimate. Override via the `userPermissions` top-level key (outside the managed block).
- **`/edikt:gov:compile` may flip some prior PASS phases to BLOCKED.** The new evidence gate requires `test_run` evidence for test-command criteria. Existing `done` phases are grandfathered automatically on first upgrade (`meta.grandfathered: true`) and retain their verdict.
- **Install URL changed.** Update bookmarks and CI scripts from `raw.githubusercontent.com/.../main/install.sh` to `releases/download/v0.5.0/install.sh`.
- **Install now requires cosign for full verification.** Users without cosign can set `EDIKT_INSTALL_INSECURE=1` to bypass (prints a loud banner). Recommended: install cosign first.

#### Remaining follow-ups (not blocking v0.5.0)

- **Homebrew tap auto-merge** still runs after CI pass; a future release adds a `production` environment gate with reviewer approval.
- **Markdown-embedded Python extraction** (10+ sites in `test/integration/`) will move to `test/_lib/` in a follow-up.
- **Hash anchor seeding** for managed markdown regions (LOW-3) lands with the next `/edikt:gov:compile` pass.
- **Upgrade rollback subcommand** (`bin/edikt rollback v0.5.0` beyond payload-only) is tracked for v0.5.x.

---

### Directive hardening + governance benchmark (SPEC-005)

#### Added

- **New directive sentinel fields: `canonical_phrases` and `behavioral_signal`.** Both fields are backward-compatible — missing fields parse as `[]` or `{}` respectively. Existing ADRs keep working without changes. (ref: SPEC-005, Phase 5)
- **`/edikt:gov:benchmark` — tier-2 adversarial benchmark command.** Runs attack prompts against every ADR/invariant with a populated `behavioral_signal` block. Install with `./bin/edikt install benchmark` — never bundled in `install.sh` per ADR-015. Ships `tools/gov-benchmark/` Python helper (SDK invocation, SIGINT handling, sandbox builder) and four attack templates under `templates/attacks/`. (ref: SPEC-005, Phase 9)
- **`/edikt:adr:review --backfill` — one-shot canonical_phrases rollout.** Interactive flow to retrofit `canonical_phrases` onto existing multi-sentence ADRs. Proposes 2–3 candidate phrases per directive with heuristic rationale; per-ADR `[y/n/e]` approval before writing. (ref: SPEC-005, Phase 3)
- **`/edikt:gov:compile` — orphan ADR detection with warn-then-block semantics.** First compile with a no-directives ADR warns (exit 0); second consecutive compile with the same orphan set blocks (exit ≠ 0). Resolve by adding directives or marking `no-directives: <reason ≥ 10 chars>`. State persisted in `.edikt/state/compile-history.json` via atomic rename. `.edikt/state/` auto-appended to `.gitignore`. (ref: SPEC-005, Phase 7)
- **`/edikt:doctor` — routing-table source-file check.** Verifies every ADR/INV referenced in the `.claude/rules/governance.md` routing table exists on disk. Exits non-zero with the literal missing path when a source file is absent. (ref: SPEC-005, Phase 2)
- **Six-marker soft-language scan in `/edikt:adr:review`.** Flags `should`, `ideally`, `prefer`, `try to`, `might`, `consider` in directive bodies. Suggests `MUST` or `NEVER` replacements per flag. (ref: SPEC-005, Phase 3)
- **Interview prompts in `/edikt:adr:new` for both new sentinel fields.** Three additional questions after the existing decision-capture prompts populate `canonical_phrases` and `behavioral_signal.refuse_tool` / `refuse_to_write` / `cite`. Skipping any prompt produces empty values, never an error. (ref: SPEC-005, Phase 4)
- **Shared directive-quality sub-procedure** at `commands/gov/_shared-directive-checks.md`. Called by both `/edikt:gov:compile` and `/edikt:gov:review`. Covers FR-003a (multi-sentence without canonical_phrases), FR-003b (canonical_phrase not found in body), and `no-directives` reason validation. (ref: SPEC-005, Phase 6)
- **ADR-015 — tier-2 tooling carve-out.** Formalizes the tier-1 / tier-2 distinction: INV-001 holds verbatim for all core commands; optional tools may depend on packages, provided install is explicit via `edikt install <tool>`. (ref: SPEC-005, Phase 1)

#### Migration

- **FR-003a is warn-only in v0.5.0.** Multi-sentence directives without `canonical_phrases` produce a warning on compile but do NOT block the run. Run `/edikt:adr:review --backfill` to retrofit existing ADRs. After running backfill, the warnings will resolve.
- **FR-003a hard-fail is targeted for the next release.** Plan your upgrade: run backfill before upgrading.

#### Baseline

- Dogfood benchmark: **2/2 PASS** on INV-001 + INV-002 under Opus 4.7. Full run at `docs/reports/governance-benchmark-20260417T102800Z/`. 15/17 directives SKIP (no `behavioral_signal` populated — expected pre-backfill state). Run `/edikt:adr:review --backfill` then re-run `/edikt:gov:benchmark` to expand coverage.
- Compare your own results with `/edikt:gov:benchmark` after upgrading.

#### Known risks

- **Tier-2 install model is new.** This release introduces both the concept (ADR-015) and its first instance (`/edikt:gov:benchmark`) in the same release. Report issues; expect point releases for UX refinement.
- **Sandbox parity is soft-enforced by AC test, not code reuse.** Any edit to `tools/gov-benchmark/sandbox.py::build_project` requires a paired edit in `commands/gov/benchmark.md` and `test/integration/benchmarks/runner.py`. A docstring invariant and parity lint warn when the two diverge.
- **Discriminative-power tests against stubbed models are a lower bound.** Real-world attack-prompt quality is only validated by the dogfood benchmark run against live Opus 4.7.
- **Backfill heuristic may propose weak phrases.** The noun/verb heuristic surfaces candidates; review them rather than rubber-stamping. The `[e]dit` option is offered per-ADR.

---

### Stability (SPEC-004)

#### Testing

- **Layer 1 — Hook unit tests.** 9 hook test suites (session-start, stop-hook, post-compact, pre-compact, pre-tool-use, post-tool-use, user-prompt-submit, subagent-stop, instructions-loaded). Each pipes a JSON fixture directly to the real hook script and diffs the output. Runs by default; opt out with `EDIKT_SKIP_HOOK_TESTS=1`.
- **Layer 2 — Agent SDK integration tests.** 6 tests covering `/edikt:init`, plan phase execution, post-compact context recovery, upgrade customization preservation, spec preprocessing, and evaluator blocked verdict. Run against real Claude via `claude-agent-sdk`. Auth via claude subscription session or `ANTHROPIC_API_KEY` (ADR-012). Regression museum: 4 tests locking in v0.4.x bugs.
- **Layer 3 — Sandboxed runner.** All tests redirect `$HOME`, `$EDIKT_HOME`, and `$CLAUDE_HOME` to a per-run temp tree. No test contaminates the developer's live `~/.edikt/` or `~/.claude/`.
- **Governance integrity tests.** Offline tests verifying ADR/invariant sentinel block hashes, routing table linkage, governance.md structure, config schema completeness, and feature toggle default-on behavior.
- **Agent role tests.** Validates that read-only agents (evaluator, docs, architect) disallow Write/Edit, writer agents are not blocked, `maxTurns` is within bounds, and all registry slugs have template files.
- **Config toggle tests.** Layer 1 tests for every `features.*` toggle — verifies the off state, the default-on state, and the no-config silent-exit behavior.
- **Real payload hook smoke tests.** Installs real hooks from `templates/hooks/` (not stubs) and verifies they are executable and respond correctly to baseline payloads.
- **CI gate.** `.github/workflows/test.yml`: Layers 1 + 3 on every PR (fast, free). Layer 2 on tag push (requires `ANTHROPIC_API_KEY` secret).

#### Versioning and rollback

- **Shell launcher `edikt`.** `bin/edikt` is a POSIX sh launcher that manages versioned payload installs at `~/.edikt/versions/<tag>/`. Subcommands: `install`, `use`, `list`, `version`, `upgrade`, `rollback`, `prune`, `doctor`, `uninstall`, `dev link/unlink`, `migrate`.
- **Versioned layout.** Payloads live at `~/.edikt/versions/<tag>/`. `current` symlink points at the active generation. `lock.yaml` tracks active, previous, and pinned versions.
- **Multi-version migration.** M1 (flat→versioned), M2 (HTML→markdown-link sentinels), M3 (flat command names→namespaced), M5 (config.yaml schema additions), M4 (compile schema v1→v2). Run order enforced: M1→M2→M3→M5→M4.
- **Rollback is payload-only.** `edikt rollback` reverts `current` to the previous generation. Migrations (M1-M5) are permanent and are not rolled back.
- **`edikt migrate --dry-run`.** Shows the full migration chain without mutating anything.

#### Distribution

- **Homebrew tap.** `brew install diktahq/tap/edikt` installs the launcher. `edikt install` fetches the payload. `brew upgrade edikt` updates the launcher; `edikt upgrade` updates the payload independently.
- **Release automation.** `.github/workflows/release.yml` builds launcher + payload tarballs, generates `SHA256SUMS` (ADR-013), uploads as GitHub Release assets, bumps the Homebrew formula on a staging branch, waits for tap CI, auto-merges on success.

#### Init and provenance

- **Provenance frontmatter.** Every generated file (agents, rules, CLAUDE.md block) carries `edikt_template_hash` (MD5 of the raw template before substitution) and `edikt_template_version` in its frontmatter.
- **Upgrade provenance-first flow.** On upgrade, compares stored hash to current template hash. Unchanged template → silent skip. Template changed, user didn't edit → auto-apply. User edited + template moved → 3-way diff prompt (ADR-011 regression guard).
- **`<!-- edikt:custom -->` marker.** Files marked with this are always skipped on upgrade, regardless of template changes.
- **Stack filters and path substitutions.** `[if:stack:go]...[/if:stack:go]` markers filter agent sections. Path substitutions apply `_substitutions.yaml` defaults to configured paths.

#### Breaking changes

- `~/.edikt/` layout changed from flat to versioned. Run `edikt migrate --yes` to upgrade from v0.4.x. See [Migrating from v0.4](website/guides/migrating-from-v0.4.md).
- `features.auto-format`, `features.signal-detection`, `features.plan-injection` config keys must now be explicit in `.edikt/config.yaml`; hooks default-on when absent.
- Hook output migrated from plaintext to JSON protocol (ADR-014). User-visible message content is preserved byte-for-byte inside `{"systemMessage": ...}` / `{"additionalContext": ...}` wrappers. No action required unless you consumed raw hook stdout in custom tooling.
- `pre-compact.sh` hook removed. Its single echo reminder is covered by `/edikt:session`. Remove `PreCompact` from any manual `.claude/settings.json` customizations.

#### Claude Code parity (ADR-014)

- **Hook protocol migration.** 7 plaintext-emitting hooks (`pre-tool-use`, `session-start`, `post-tool-use`, `post-compact`, `subagent-stop`, `stop-failure`, `user-prompt-submit`) now emit JSON conforming to the Claude Code hook protocol. Characterization fixtures regenerated via `verified_by` commands per SPEC-004 §14.
- **New hook events.** Settings template wires `SessionEnd`, `SubagentStart`, `TaskCompleted`, `WorktreeCreate`, `WorktreeRemove` (v2.1.78–v2.1.84). Each ships with a characterization fixture pair.
- **pre-tool-use `updatedInput` transformation.** Hook now emits `{"decision": "block"}` when an edit would damage `[edikt:start]: #` or `[edikt:directives:start]: #` sentinel blocks, protecting compiled governance from accidental user edits.
- **task-created plan-phase tracking.** `TaskCreated` / `TaskCompleted` emit structured events to `~/.edikt/events.jsonl` so plan progress can be reconstructed.
- **Agent `initialPrompt` rollout.** 17 agents gained the `initialPrompt` frontmatter field; 3 already had it. All use positive framing per Opus 4.7 best-practices guidance.
- **Opt-in statusline.** New `statusLine` block in the settings template emits `ADRs: N | INVs: M | Drift: K` when `.edikt/config.yaml: features.statusline: true`.
- **Preprocessor hardening.** The `!` live block in 5 commands (`adr:new`, `invariant:new`, `sdlc:prd`, `sdlc:plan`, `sdlc:spec`) is now cwd-agnostic, zsh-safe (uses `find` instead of glob), and applies the correct `${BASE:-docs}` fallback. New regression test suite in `test/unit/test-preprocessor-robustness.sh` and `test/integration/regression/test_preprocessor_cwd_and_shell.py`.
- **Prompt-caching env var guidance.** `website/getting-started.md` documents `ENABLE_PROMPT_CACHING_1H` and `FORCE_PROMPT_CACHING_5M` (v2.1.108) for long sessions with heavy governance reads.
- Full adoption matrix at [docs/internal/claude-code-parity.md](docs/internal/claude-code-parity.md).

#### Migration notes

See [website/guides/migrating-from-v0.4.md](website/guides/migrating-from-v0.4.md) for the full walkthrough.

---

## v0.4.3 (2026-04-14)

### Bug fixes

- **Phase-end evaluator now actually runs.** The phase-end evaluator relied on Claude voluntarily following instructions in plan.md to invoke it. When users executed plan phases directly (the common flow), the evaluator was never triggered. Added `phase-end-detector.sh` — a new Stop hook that detects phase completion signals in Claude's output, finds the in-progress phase from the active plan, and auto-invokes the headless evaluator with the phase's acceptance criteria. Logs `phase_completion_detected` and `phase_evaluation` events to `~/.edikt/events.jsonl`.
  - Detection patterns: "Phase N complete/done/finished/implemented", "Implemented phase N", "PHASE N DONE" completion promise format
  - Respects `evaluator.phase-end: false` config to disable
  - Test override: `EDIKT_EVALUATOR_DRY_RUN=1` to detect without invoking claude -p, `EDIKT_SKIP_PHASE_EVAL=1` to skip entirely

- **Upgrade no longer silently overwrites user customizations.** `/edikt:upgrade` compared installed agents against current templates using a simple hash diff and reported any difference as "template updated ⬆" — misleading language that prompted users to accept and lose their customizations. Now classifies diffs into three buckets:
  - **PURE EXPANSION** — template added content, no user content removed. Auto-applied.
  - **PATH SUBSTITUTION** — only paths differ (e.g., `docs/architecture/decisions/` → `adr/`). Flagged as user divergence.
  - **USER DIVERGENCE** — installed file has content not in the template. Prompts individually with diff preview and options: apply template (lose customizations), keep mine (add `<!-- edikt:custom -->` marker), or skip.

- **Evaluator could silently degrade to read-only PASS.** When invoked as a subagent (directly via the Agent tool, or as a fallback from headless), the evaluator inherited the parent session's permission sandbox — which may deny Bash even when the agent's `tools:` frontmatter declares it. With no way to signal "I couldn't verify this," the evaluator fell back to read-only inspection and returned PASS verdicts on acceptance criteria that required test execution. Captured in [ADR-010](docs/architecture/decisions/ADR-010-evaluator-headless-default-visible-fallback.md).

### Features

- **BLOCKED verdict (ADR-010).** Both evaluator templates (`templates/agents/evaluator.md` and `templates/agents/evaluator-headless.md`) now declare BLOCKED as a valid per-criterion and overall verdict. Rule added: "if a criterion requires execution and execution is unavailable, verdict is BLOCKED — never PASS." The subagent template gained a Capability Self-Check section that probes Bash availability before claiming verdicts.

- **Visible evaluator fallback (ADR-010).** `/edikt:sdlc:plan` now attempts headless first when `evaluator.mode: headless`, falls back to subagent on headless failure with a visible `⚠ EVALUATOR FALLBACK` banner naming the reason and recovery hint, and emits a `✗ EVALUATION FAILED` banner when both modes fail. BLOCKED verdicts now surface per-criterion with recovery hints; the progress table gained a `blocked` state. No silent degradation paths remain.

- **Doctor evaluator probe (ADR-010).** `/edikt:doctor` now probes the evaluator: checks `claude` CLI is on PATH, runs a headless sanity call (`claude -p "echo ok"`), verifies both evaluator templates exist, and reports whether `evaluator.mode` is explicitly set. Each failure has actionable remediation (`claude login`, `/edikt:upgrade`, `/edikt:config set evaluator.mode headless`).

- **`--eval-only {phase}` flag on `/edikt:sdlc:plan` (ADR-010).** Re-run evaluation on a specific phase without re-running the generator. Recovery path for BLOCKED verdicts after the user has fixed the underlying cause (e.g. switching `evaluator.mode` to headless). Routes through the existing Phase-End Flow — no verdict-logic duplication. Optionally combines with `--plan {slug}` when multiple plans exist.

### Governance

- ADR-010 captures the decision and its directives: headless default, subagent as fallback, BLOCKED over silent PASS, visible warnings, doctor probe, no silent degradation.

### Tests

- 17 new tests in `test-phase-end-detector.sh` covering completion pattern detection, config respect, loop prevention, correct phase selection, event logging, and no-false-positive cases.
- 11 new assertions in `test-v040-evaluator.sh` covering BLOCKED verdicts, Capability Self-Check, never-PASS rule, parent-sandbox warning, fallback/failed banners, `--eval-only` flag documentation, and doctor evaluator probe.

## v0.4.2 (2026-04-13)

### Bug fixes

- **Spec preprocessing.** Blank line between frontmatter and `!`` preprocessing block caused shell corruption. Added missing `argument-hint`.
- **Plan pre-flight skipped.** Pre-flight specialist review and criteria validation (steps 10-11) were ordered after the "Next: execute Phase 1" conclusion (step 9). Claude naturally stopped at the conclusion. Reordered: pre-flight is now steps 8-9, write file is step 10, output is step 11.
- **Audit inline mode.** `--no-edikt` jump target said "step 6" (agent-spawning) but inline audit mode was at step 11. Fixed.
- **Gov review premature conclusion.** "Next: Run /edikt:gov:compile" appeared before staleness detection still needed to run. Moved to actual conclusion.

### Tests

- 15 preprocessing format regression tests (no blank lines, argument-hint, awk integrity)
- 5 step ordering regression tests (plan, audit, review)
- 24 evaluator flow tests (pre-flight + phase-end + bypass protection)
- Version check no longer hardcoded

## v0.4.1 (2026-04-12)

### Bug fixes

- **Upgrade: new agent detection.** `/edikt:upgrade` now detects agent templates added in newer versions. Core agents (evaluator, evaluator-headless) are installed automatically. Optional agents are offered to the user with a description — declined agents are added to `agents.custom` so future upgrades skip them.
- **Upgrade: config migration.** `paths.soul` renamed to `paths.project-context`. Upgrade auto-migrates existing configs.
- **CodeRabbit review fixes.** Subagent-stop override check now matches agent + finding on the same line (was two independent greps). WEAK PASS exit code corrected to 0. .gitignore negation patterns fixed. BSD-only stat removed from SPEC-003. Agent count corrected to 18 across website docs.

### Documentation

- Updated `project-context.md` for v0.4.0: hook count (9→13), agent count, quality gates, plan harness features.

## v0.4.0 (2026-04-11)

### Plan Harness: Iteration Tracking, Context Handoff, Criteria Sidecar

The plan command now tracks failure history, carries context across phase boundaries, and emits a machine-readable criteria sidecar.

- **Iteration tracking:** progress table with Attempt column (`N/max`), 6 statuses (`pending`, `in-progress`, `evaluating`, `done`, `stuck`, `skipped`). After each evaluation failure, fail reasons are forwarded to the next attempt. Escalation warning at 3 consecutive failures on the same criterion. Phase goes `stuck` at max attempts (configurable, default 5) with human decision prompt.
- **Context handoff:** each phase has a `Context Needed` field listing files to read before starting. Artifact Flow Table maps producing phases to consuming phases. PostCompact hook re-injects context files, attempt count, and failing criteria after compaction.
- **Criteria sidecar:** `PLAN-{slug}-criteria.yaml` emitted alongside plan markdown. Per-criterion status tracking (pending/pass/fail), verification commands, fail counts. Evaluator reads and updates the sidecar — no markdown parsing needed.

### Evaluator: Headless Execution and Configuration

The evaluator now runs as a separate `claude -p` invocation with zero shared context from the generator session.

- **Evaluator config:** new `evaluator` section in `.edikt/config.yaml` with 5 keys: `preflight` (toggle pre-flight), `phase-end` (toggle evaluation), `mode` (headless or subagent), `max-attempts` (stuck threshold), `model` (sonnet/opus/haiku).
- **Headless mode (default):** evaluator runs via `claude -p --bare` with `--disallowedTools "Write,Edit"`. Fresh process, no shared context, no self-evaluation bias. Falls back to subagent when headless unavailable.
- **Protected agent:** evaluator templates are not user-overridable. Upgrade always overwrites them. Doctor warns on modifications. Plan blocks if template is missing.
- **LLM evaluator in experiments:** `--llm-eval` flag in experiment runner. Dual-mode: grep pre-check + LLM evaluation. LLM verdict is authoritative when both run. Three verdicts: PASS, WEAK PASS (all critical pass but important fails), FAIL. Severity tiers: critical (blocks), important (degrades), informational (logged only).

### Enforcement: Quality Gate UX and Artifact Lifecycle

Quality gates now log overrides with accountability, and artifact lifecycle is enforced uniformly across the SDLC chain.

- **Gate override logging:** overrides written to `~/.edikt/events.jsonl` with git identity (name + email). Three event types: `gate_fired`, `gate_override`, `gate_blocked`.
- **Re-fire prevention:** overridden findings don't fire again within the same session. Overrides cleared at session start.
- **Artifact lifecycle:** full status chain `draft → accepted → in-progress → implemented → superseded`. Plan auto-promotes `accepted → in-progress` when phase starts. Drift auto-promotes `in-progress → implemented` when no violations found.
- **Plan draft warning:** lists specific draft artifacts by name, offers proceed (with Known Risks) or stop.
- **Drift status filter:** skips `draft` and `superseded` artifacts, validates the rest.
- **Doctor:** flags spec-artifacts stuck in draft > 7 days. Parses both YAML frontmatter and comment header status formats.

### Breaking changes

- **Config key rename:** `paths.soul` → `paths.project-context`. `/edikt:upgrade` auto-migrates existing configs. Commands fall back to `soul` if `project-context` is not found.

### Documentation

- Updated `project-context.md` for v0.4.0: hook count (9→13), agent count (20→19), quality gates, plan harness features, context vs enforcement framing
- Fixed 12 pre-existing documentation gaps (stale agent/hook/command counts, old command names in AGENTS.md, missing index entries)
- Updated website: plan, gates, chain, features, doctor, drift pages with v0.4.0 features
- Removed stale AGENTS.md (Codex convention — edikt is Claude Code only per ADR-001)

### New config keys

```yaml
evaluator:
  preflight: true       # pre-flight criteria validation
  phase-end: true       # phase-end evaluation
  mode: headless        # headless | subagent
  max-attempts: 5       # max retries before stuck
  model: sonnet         # model for headless evaluator
```

## v0.3.1 (2026-04-11)

### Bug fixes

- **Init: guidelines path.** `/edikt:init` now writes `paths.guidelines` correctly.
- **VERSION stamp.** `VERSION` file updated to match release tag.
- **PRINCIPAL prefix.** Compile output no longer prefixes directives with `PRINCIPAL:`.
- **Review output.** `/edikt:sdlc:review` output formatting fixed.
- **SubagentStop hook: seniority prefix.** The fallback agent detection pattern matched "As Principal Architect" → `principal-architect` instead of `architect`, breaking slug lookup and gate matching. Now extracts only the role word.
- **Missing page.** Added `/edikt:guideline:compile` website page (was dead link).
- **Test fixes.** All 25 suites pass after v0.3.0 regressions.

### Artifact generation: JSONB support and domain class diagram

`/edikt:sdlc:artifacts` now handles projects using JSONB aggregate storage (common DDD pattern in PostgreSQL) and generates a domain class diagram alongside the data model.

- **Storage strategy detection.** When DB type is `sql` or `mixed`, the command scans spec content and migrations for JSONB signals (`jsonb`, `json column`, `aggregate storage`, `embedded entity`, `nested entity`, etc.). Detected strategy is shown in the state checkpoint and routing output.
- **Three entity modes in `data-model.mmd`.** When storage strategy is `jsonb-aggregate`, the ERD distinguishes physical tables (normal), JSONB-embedded entities (relationship label contains `jsonb`), and reference-only entities from external bounded contexts (relationship label contains `ref`). Makes nested structure visible instead of hiding it in JSONB column comments.
- **Domain class diagram (`model.mmd`).** New artifact type, always generated alongside the data model regardless of DB type. Mermaid `classDiagram` showing aggregate roots, value objects, entities, inheritance, composition, and domain methods. Reviewed by the architect agent.

### Configurable artifact spec versions

Artifact templates now use configurable spec versions instead of hardcoded values. Defaults updated to latest stable:

| Format | Previous | Now (default) |
|---|---|---|
| OpenAPI | 3.0.0 | **3.1.0** |
| AsyncAPI | 2.6.0 | **3.0.0** |
| JSON Schema | draft-07 | **2020-12** |

Teams can pin older versions in `.edikt/config.yaml`:

```yaml
artifacts:
  versions:
    openapi: "3.0.0"       # pin for tooling compatibility
    asyncapi: "2.6.0"      # pin if not ready for 3.0 breaking changes
    json_schema: "https://json-schema.org/draft/07/schema#"
```

The AsyncAPI template was updated for the 3.0 structure (separate `channels` and `operations` blocks replacing `publish`/`subscribe`). When pinning `asyncapi: "2.6.0"`, the agent uses the 2.x structure.

### New `/edikt:config` command

View, query, and modify `.edikt/config.yaml` with discovery, validation, and natural-language changes.

- **No args** — show all 34 config keys with current values and defaults
- **`get {key}`** — show a specific key's value, default, valid values, and which commands use it
- **`set {key} {value}`** — validate and write, with per-key validation rules

Protected keys like `edikt_version` cannot be set directly. Invalid values are rejected with explanation.

### `/edikt:team` deprecated — merged into init + config

`/edikt:team` served two purposes that belong elsewhere:
- **Member onboarding** → now in `/edikt:init`'s "existing project" path
- **Config management** → now in `/edikt:config`

When `/edikt:init` detects an existing `.edikt/config.yaml`, it runs member environment validation instead of saying "already initialized":
1. **Version gate** — blocks if installed edikt < project's `edikt_version`
2. **Environment checks** — git identity, Claude Code, MCP env vars (read dynamically from `.mcp.json`), `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB`, pre-push hook, managed settings
3. **Governance gap sync** — missing rules/hooks/agents offered for install
4. **Shared config display** — what's committed to git

The `team:` config block is no longer used. Legacy blocks in existing configs are ignored silently. The deprecated stub redirects to init and will be removed in v0.5.0.

## v0.3.0 (2026-04-10)

### Project Adaptation (ADR-008, ADR-009)

edikt can now adapt to existing projects. The compile pipeline supports a **three-list directive schema** (ADR-008) with hash-based caching, and introduces **Invariant Records** as the formal governance artifact for hard constraints (ADR-009).

- **Three-list schema:** every compiled sentinel block now carries `directives:` (auto-generated), `manual_directives:` (user-authored), and `suppressed_directives:` (user-rejected). The merge formula `effective = (directives - suppressed) ∪ manual` gives users full control over what ships without losing compile automation. Hash-based caching (`source_hash` + `directives_hash`) skips Claude calls when nothing changed.
- **Invariant Records (ADR-009):** formalized the governance artifact for non-negotiable constraints. Formalized "Invariant Records" as the governance artifact for hard constraints (short form: INV). Template follows Statement/Rationale/Enforcement structure. Compile extracts directives from the Statement section, preserving declarative absolute language.
- **Extensibility plumbing:** template lookup chain (`project .edikt/templates/` → inline fallback), `/edikt:guideline:compile` command, auto-chain (`<artifact>:new` runs `<artifact>:compile`).
- **Init style detection:** detects project style (flat, layered, monorepo) during init. Adapt mode for existing `.edikt/` directories. Template-less refusal for v0.3.0+ projects.
- **Flexible prose input:** ADR/invariant/guideline creation accepts natural language with automatic reference extraction to existing governance.
- **Doctor + upgrade integration:** doctor reports template overrides and stale hashes. Upgrade respects project templates.

### Compile Improvements

Experiment-driven improvements to the compile output format. These changes improve how well Claude follows governance directives.

- **"No exceptions." reinforcement:** invariant directives derived from absolute-language Statements ("every", "all", "total") now get "No exceptions." appended. Experiments showed this phrase prevents Claude from rationalizing edge cases.
- **Reminders sentinel (`[edikt:reminders:start/end]`):** compile now generates pre-action interrupts: "Before writing SQL → MUST include tenant_id." Aggregated into a `## Reminders` section in governance.md. Capped at 10.
- **Verification checklist:** compile generates a `## Verification Checklist` section with grep-verifiable items Claude checks before finishing. Capped at 15 items.
- **Per-directive LLM compliance scoring** in `/edikt:invariant:review`, `/edikt:adr:review`, `/edikt:guideline:review`: scores each compiled directive on token specificity, MUST/NEVER usage, grep-ability, ambiguity, and friction risk. Manual directives held to the same standard.
- **New `/edikt:gov:score` command:** aggregate governance quality report — context budget, compliance metrics, manual directive health. JSON output for CI.

### Pre-flight Criteria Validation

The evaluator agent now supports a **pre-flight mode** that validates acceptance criteria BEFORE the generator starts. Classifies each criterion as TESTABLE/VAGUE/SUBJECTIVE/BLOCKED and proposes verification commands. The plan command (step 11) invokes pre-flight automatically, preventing wasted iterations on untestable criteria.

### Experiments

Pre-registered experiments measuring whether governance directives change Claude's output on real coding tasks. 8 experiments across 4 scenario types.

| Scenario | Baseline | Governance | Effect |
|---|---|---|---|
| Existing codebase (EXP 01-04) | PASS | PASS | Absent — code patterns self-teach |
| Greenfield (EXP 05-06) | VIOLATION | PASS | **Present** — governance prevents architecture/tenant violations |
| New domain on existing (EXP 07) | VIOLATION | PASS | **Present** — governance catches log/SQL misses |
| Long context (EXP 08, N=2) | 1/2 VIOLATION | 0/2 PASS | **Present** — governance stabilizes under context pressure |

Key findings: governance has measurable effect on greenfield and new-domain code. Directive format matters — MUST/NEVER with literal code tokens outperforms prose. Long context degrades convention compliance; governance in `.claude/rules/` survives because it's loaded separately from the conversation. Full methodology and results in `test/experiments/`.

### New commands

- `/edikt:gov:score` — aggregate governance quality scoring for CI

### Architecture Decisions

- **ADR-008:** Deterministic compile and three-list schema
- **ADR-009:** Invariant Record template formalization

## v0.2.3 (2026-04-09)

### Compile schema version (ADR-007)

`/edikt:gov:compile` now stamps generated governance files with a **compile schema version** — a small integer independent of edikt's marketing version — instead of the edikt version at compile time.

**Problem this fixes:** before v0.2.3, `.claude/rules/governance.md` was stamped with `version: "<edikt-version>"`, conflating two unrelated cadences. Every edikt point release (even pure bug fixes) implied governance was stale and needed regeneration, but the compile output format hadn't actually changed. In the dogfood repo, we kept hand-editing `governance.md`'s version via `sed` on each release to keep a test green — the file ended up lying about its own provenance (version said v0.2.2 but the compile timestamp was frozen at March 25).

**New format** (see [ADR-007](docs/architecture/decisions/ADR-007-compile-schema-version.md)):

```yaml
---
paths: "**/*"
compile_schema_version: 2
---
<!-- edikt:compiled — generated by /edikt:gov:compile, do not edit manually -->
<!-- compiled_by: edikt v0.2.3 -->
<!-- compiled_at: 2026-04-09T10:30:00Z -->
```

Three fields, three purposes:

- **`compile_schema_version`** (YAML, enforced) — identifies the output format contract. `1` = v0.1.x flat governance, `2` = v0.2.x topic-grouped rule files. `/edikt:doctor` checks it against the constant declared in `commands/gov/compile.md` and recommends `/edikt:gov:compile` only when the format has actually changed.
- **`compiled_by`** (HTML comment, informational) — which edikt version ran compile. Diagnostic only, never enforced.
- **`compiled_at`** (HTML comment, informational) — ISO8601 timestamp. Diagnostic only, never enforced.

**Consequences:**
- No more false-positive staleness warnings on point releases. Users only see "regenerate governance" when the compile schema actually changed.
- Point releases can ship bug fixes without implying anything about compile output compatibility.
- `/edikt:doctor` gets smarter about stale governance detection.
- `/edikt:upgrade` has a new step that checks the project's schema version against the installed compile schema and recommends (but does not auto-run) regeneration when they diverge.
- Dogfooding stops hand-editing `governance.md`'s version field. The dogfood file now uses the new format honestly.

### Installer UX fixes

Three bug reports from real installs, all fixed in the same release.

- **No prompt on `curl | bash`.** The interactive "global vs project" prompt was skipped silently when stdin was a pipe (the common `curl -fsSL ... | bash` invocation). Now the installer reads from `/dev/tty` when available, so the prompt fires even when stdin is consumed by the curl pipe. Falls back to `--global` only when there's no TTY at all (CI, fully redirected).
- **Commands duplicated across global and project locations.** When a user installed globally in a directory that already had a project-local edikt install (either from a prior `--project` run, or from the dogfood repo itself), Claude Code ended up loading commands from both `~/.claude/commands/edikt/` and `.claude/commands/edikt/`, producing duplicates in the skill list. The installer now detects this condition at startup and emits a yellow warning pointing at the exact paths and the `rm -rf` to clean them up. Never auto-deletes.
- **No detection of existing install before project install.** If a user ran `install.sh --project` in a directory where `~/.edikt/VERSION` already existed, the two installs would silently overlap. Same detection now fires a warning for this case too. Both detection paths share the same `HAS_GLOBAL` / `HAS_PROJECT` flags.
- **New test scenarios in `test/test-install-e2e.sh`** — scenarios 6 and 7 cover the duplication-warning paths (6 = global install with leftover project files; 7 = project install with existing global install). Total scenarios now: 7. Total assertions: 28.

### Tests

- **New `test/test-v023-regressions.sh`** (21 assertions) — verifies ADR-007 exists, `COMPILE_SCHEMA_VERSION` is declared in compile.md, output templates emit the new format, doctor.md checks the schema version, upgrade.md documents the migration step, and the dogfood governance file matches the constant.
- **`test-e2e.sh` version check refactored** — no longer enforces `GOV_VER == FILE_VER`. Instead it validates that `compile_schema_version` in the dogfood governance file matches the `COMPILE_SCHEMA_VERSION` constant in `commands/gov/compile.md`.

## v0.2.2 (2026-04-08)

Critical bug-fix release. The v0.2.1 installer was silently broken on the v0.1.x → v0.2.x upgrade path.

### Installer: upgrade from v0.1.x was silently broken

- **`((BACKUP_COUNT++))` under `set -euo pipefail` killed the installer on the first backup.** Postfix `++` returns the pre-increment value (`0` on the first call), which bash's `set -e` treats as a failure and exits the script. Symptoms: the cleanup loop removed *nothing*, the new namespaced commands were *never* installed, old flat files stayed in place, and the installer exited without any error message. This shipped in v0.2.1 and affected everyone upgrading from v0.1.x via `curl | bash`. Fixed by using `BACKUP_COUNT=$((BACKUP_COUNT + 1))`.
- **New integration test** (`test/test-install-e2e.sh`) — 22 assertions across five scenarios: fresh install, upgrade from v0.1.x, user-customized file preservation, network failure abort, and repeated-install idempotency. Shims `curl` with a mock that serves files from the local repo, so the full `install.sh` runs end-to-end against a fake `$HOME` in `/tmp`. This is the test we wished existed before v0.2.0 shipped — it caught the v0.2.1 regression immediately.

### `/edikt:upgrade`: migrate v0.1.x command references

- **New step 5 in `/edikt:upgrade`: rewrite old flat command references in project files to their new namespaced equivalents.** Projects initialized with v0.1.x have hardcoded references to `/edikt:adr`, `/edikt:plan`, `/edikt:compile`, etc. in their `CLAUDE.md` managed block and in compiled rule packs. Previously, `/edikt:upgrade` only migrated the *sentinel format* (HTML → visible) and left the *content* inside the sentinels untouched. Now upgrade runs a targeted string-replace across all 15 moved commands, scoped to edikt-owned content only (the CLAUDE.md managed block and rule pack files marked with `edikt:generated` or `edikt:compiled`). User content outside the managed blocks is never touched.
- **Idempotent and safe:** the instruction tells Claude to match only occurrences NOT already followed by `:`, using surrounding context (backticks, punctuation, end-of-line) for disambiguation. Running upgrade twice is a no-op.

## v0.2.1 (2026-04-08)

Bug-fix release following v0.2.0 field reports.

### Installer upgrade path

- **Old flat commands no longer linger after upgrade.** v0.1.x installed commands like `~/.claude/commands/edikt/adr.md`, `plan.md`, `compile.md` at the top level. v0.2.0 moved them into namespaces but the installer never removed the old files, so users saw both `/edikt:adr` (stale) and `/edikt:adr:new` (new) in their command list. The installer now deletes the 15 moved v0.1.x commands before installing new files, with backup. User-customized files (marked with `<!-- edikt:custom -->`) are preserved.
- **Silent curl failures now abort the install.** Every `curl -o` call now goes through a `_fetch` helper that enforces `--retry 2`, `--max-time 30`, non-empty download verification, and exits with an error on failure. Previously a network blip during `curl | bash` could leave files partially updated without any warning.

### `/edikt:init` ADR path adoption

- **init now configures `paths.decisions` to match detected ADR locations.** Previously, init detected existing ADRs in folders like `docs/decisions/` and offered to import them, but the import flow hardcoded the destination to edikt's default (`docs/architecture/decisions/`) and never wrote the detected path into `.edikt/config.yaml`. Users ended up with ADRs in one place and edikt looking for them somewhere else — `/edikt:gov:compile` and `/edikt:status` reported zero ADRs.
- New prompt: **[1] Adopt** (keep ADRs where they are, configure edikt to use that path), **[2] Migrate** (move to edikt's default layout), **[3] Skip**. Same flow for invariants.

### Command documentation cleanup

- **Seniority prefixes removed from `/edikt:sdlc:review` reviewer lenses.** The command documentation still labeled agents as `Principal DBA`, `Staff SRE`, `Staff Security`, `Senior API`, `Principal Architect`, `Senior Performance` — inconsistent with the agent templates which dropped seniority prefixes in v0.2.0. Now just `DBA`, `SRE`, `Security`, `API`, `Architect`, `Performance`.

### Website content

- **Fixed 10 dead links in `website/governance/chain.md`, `website/governance/compile.md`, `website/governance/drift.md`, and `website/commands/brainstorm.md`** — they referenced old flat command paths (`/commands/prd`, `/commands/spec`, `/commands/plan`, etc.) that broke the v0.2.0 VitePress deploy. Now use namespaced paths (`/commands/sdlc/prd`, `/commands/gov/compile`, etc.).

### Test coverage

- New `test/test-v021-regressions.sh` — 36 assertions guarding against all five v0.2.1 bugs so they can't silently return.

## v0.2.0 (2026-03-31)

### Intelligent Compile — topic-grouped rule files

`/edikt:compile` no longer produces a single flat `governance.md`. It now generates **topic-grouped rule files** under `.claude/rules/governance/` — each topic file contains full-fidelity directives from all sources (ADRs, invariants, guidelines), loaded automatically by path matching.

- **Directive sentinels** — ADRs and invariants can include `[edikt:directives:start/end]` blocks with pre-written LLM directives. Compile reads these verbatim — no extraction, no distillation.
- **Routing table** — `governance.md` becomes an index with invariants + a routing table. Claude matches task signals and scopes to load relevant topic files.
- **Three loading mechanisms** — `paths:` frontmatter (platform-enforced on file edits), `scope:` tags (activity-matched for planning/design/review), and signal keywords (domain-matched).
- **No directive cap** — the 30-directive limit is removed. Soft warning if a topic file exceeds 100 directives.
- **Reverse source map** — compile output shows which ADRs/guidelines contributed to which topic files.
- **Sentinel generation moved to compile** — `/edikt:compile` now generates missing sentinel blocks inline before compiling. No separate step needed. `/edikt:review-governance` is now pure language quality review + staleness detection.
- `/edikt:review-governance` redesigned — language quality review only. Detects stale sentinels and directs to compile. No longer generates anything.

### Command namespacing

edikt commands are now grouped into namespaces matching the artifacts they touch. Nested namespacing confirmed working in Claude Code.

**New structure:**
- `edikt:adr:new` / `:compile` / `:review` — ADR lifecycle
- `edikt:invariant:new` / `:compile` / `:review` — invariant lifecycle
- `edikt:guideline:new` / `:review` — guideline management
- `edikt:gov:compile` / `:review` / `:rules-update` / `:sync` — governance assembly
- `edikt:sdlc:prd` / `:spec` / `:artifacts` / `:plan` / `:review` / `:drift` / `:audit` — SDLC chain
- `edikt:docs:review` / `:intake` — documentation
- `edikt:capture` — mid-session decision sweep (new command)

**New commands:** `capture`, `guideline:new`, `guideline:review`, `adr:compile`, `adr:review`, `invariant:compile`, `invariant:review`

**Deprecated** (removed in v0.4.0): old flat names (`edikt:adr`, `edikt:compile`, `edikt:spec`, etc.) — each stub tells you the new name.

### Agent governance

All 19 agent templates now include governance frontmatter:

- **`maxTurns`** — 10 for advisory agents, 20 for code-writing agents, 15 for the evaluator.
- **`disallowedTools`** — advisory agents have `Write` and `Edit` disallowed at the platform level.
- **`effort`** — high for architect/security/qa/performance/compliance, medium for backend/frontend/dba/api/sre/docs/pm/data/platform/ux, low for gtm/seo.
- **Agent effort fixes** — `data` was `low` with `disallowedTools: [Write, Edit]` which blocked artifact creation. Fixed to `medium` with write access. `platform`, `compliance`, and `ux` effort levels corrected to match their review depth.
- **`initialPrompt`** — architect, security, and pm auto-load project context when run as main session agents.
- **New `evaluator` agent** — phase-end evaluator that verifies work against acceptance criteria with fresh context. Skeptical by default.

### Hook modernization

- **Conditional `if` field** on PostToolUse (scopes to code files only) and InstructionsLoaded (scopes to rule files only). Avoids spawning hook processes for non-matching files.
- **4 new hooks** — `StopFailure` (logs API errors), `TaskCreated` (tracks plan phase parallelism), `CwdChanged` (monorepo directory detection), `FileChanged` (warns on external governance file modifications).

### Harness improvements

- **Context reset guidance** — at phase boundaries, edikt recommends starting a fresh session. State lives in the plan file.
- **Phase-end evaluation** — evaluator agent checks acceptance criteria with binary PASS/FAIL per criterion before suggesting context reset.
- **Acceptance criteria per phase** — plans now include testable, binary assertions per phase. Specs enforce downstream flow.
- **Conditional evaluation** — `evaluate: true/false` per phase. High-effort phases evaluate by default, low-effort skip.
- **Evaluator tuning** — `docs/architecture/evaluator-tuning.md` tracks false positives/negatives for prompt refinement.
- **Harness assumptions** — `docs/architecture/assumptions.md` documents 6 testable assumptions about model limitations. `/edikt:upgrade` prompts for re-testing.

### Rule pack UX

- **Conflict detection** — `/edikt:rules-update` checks new rule packs against compiled governance before installing.
- **Install preview** — shows what will change (added/changed/removed sections) before applying updates.
- **Override transparency** — `/edikt:doctor` and `/edikt:status` report compiled governance status, sentinel coverage, and rule pack overrides.

### Installer safety

- **`--dry-run`** — preview what the installer would change without writing files.
- **Backup before overwrite** — existing files backed up to `~/.edikt/backups/{timestamp}/` before overwriting.
- **Existing install detection** — reports installed version and confirms before proceeding.

### Headless & CI foundations

- **`--json` output** — compile, drift, audit, doctor, review, and review-governance support `--json` for machine-readable output.
- **Headless mode** — `EDIKT_HEADLESS=1` with `headless-ask.sh` hook auto-answers interactive prompts for CI pipelines.
- **CI guide** — new website guide with GitHub Actions example, recommended settings, and environment variables.
- **Managed settings awareness** — `/edikt:team` detects organization-managed settings (`managed-settings.json`, `managed-settings.d/`).

### UX consistency improvements

- **Standardized completion signals** — all 25 commands end with `✅ {Action}: {identifier}` + `Next:` line.
- **Standardized error messages** — all commands that read config use the same missing-config message.
- **Config guards** — 10 additional commands now guard for missing `.edikt/config.yaml` instead of failing mid-execution.
- **Init rule preview** — step 3b shows a preview of actual rules before generating files, with customization paths taught at the moment of installation.
- **Init reconfigure protection** — content hash comparison detects edited files. Per-file `[1] Overwrite / [2] Keep mine / [3] Show diff` flow instead of silent overwrites.
- **Composite config screen** — SDLC options merged into the single combined rules/agents view. One screen, one confirmation.
- **Concrete init summary** — before/after with stack-specific examples from installed rules and agents.
- **Agent routing standardized** — all commands use `🔀 edikt: routing to {agents}` format.
- **Progress breadcrumbs** — compile, audit, review, drift, and review-governance show `Step N/M:` progress.
- **Numbered confirmation options** — letter-code choices (`[a]/[s]/[k]`) replaced with `[1]/[2]/[3]`.
- **Emoji key** — output conventions table added to CLAUDE.md template.

### Bug fixes

- **Plan ignores spec artifacts when generating phases** — `/edikt:plan` now scans the spec directory for all artifact files (fixtures, test strategy, API contracts, event contracts, migrations) and verifies each has plan coverage. Uncovered fixtures get a seeding phase, uncovered test categories get test tasks, uncovered API endpoints get a warning. A hard gate (step 6c) blocks plan writing if any artifact has no coverage — the user must add phases, defer explicitly, or cancel. Prevents silent failures where artifacts are generated but never consumed.
- **Cross-reference validation in compile and review-governance** — both commands now verify that every `(ref: INV-NNN)` and `(ref: ADR-NNN)` reference points to an actual source document. Fabricated references are stripped before writing.
- **Plan trigger not matching "let's create a plan to fix X"** — added trigger examples with trailing context ("plan to fix these issues", "plan these changes", "plan this work") so Claude matches the plan intent even when the sentence includes what to fix.
- **SessionStart hook errors on compact** — `set -euo pipefail` caused silent non-zero exits when Claude Code fires `SessionStart` after `/compact`. Relaxed to `set -uo pipefail` — the hook already guards every fallible command with `|| true`.
- **Test suite requires pyyaml** — agent and registry tests used `python3 -c "import yaml"` which fails silently when pyyaml isn't installed. Rewrote agent frontmatter checks in pure bash, registry checks to fall back to `yq`, and `assert_valid_yaml` to try `yq` when python3-yaml is unavailable.

### Platform alignment

- **Environment hardening** — `/edikt:team` checks for `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB`. Security guide documents `sandbox.failIfUnavailable`.
- **SendMessage auto-resume** — documented on website for agent resumption.

## v0.1.4 (2026-03-28)

### Brainstorm command

New `/edikt:brainstorm` command — a thinking companion for builders. Open conversation grounded in project context, with specialist agents joining as topics emerge. Converges toward a PRD or SPEC when ready. Use `--fresh` for unconstrained brainstorming that challenges existing decisions. Brainstorm artifacts saved to `docs/brainstorms/`.

### Upgrade version check

`/edikt:upgrade` now checks for newer edikt releases before upgrading the project. If a newer version exists, it shows the install command and stops — ensuring project upgrades always use the latest templates. Skip with `--offline` for air-gapped environments.

## v0.1.3 (2026-03-27)

### Flexible plan input

`/edikt:plan` now accepts any input format — natural language prompts, existing plan names, ticket IDs, SPEC identifiers, or nothing (infers from conversation context). When the intent is ambiguous (natural language or conversation context), edikt offers a choice between a full phased plan (saved to `docs/plans/`) and a quick conversational plan.

- `PLAN-NNN` input: continue from current phase, re-plan remaining phases, or create a sub-plan
- Empty input: infers from current conversation context before asking
- Natural language: offers full vs quick plan disambiguation

## v0.1.2 (2026-03-27)

### Bug fix

- **Installer prompt auto-answered when piped** — `curl | bash` triggered the interactive install mode prompt which got EOF from stdin, flashing the prompt and auto-selecting global. Now detects non-terminal stdin and defaults to global silently. Use `--project` flag for project-local install.

## v0.1.1 (2026-03-27)

### Numbered findings in reviews

All review commands now enumerate findings with IDs (#1, #2, #3...) so users can select which to address by number.

- `/edikt:plan` — pre-flight findings numbered, triage prompt: "Which findings should I address? (e.g., #1, #4 or 'all critical')"
- `/edikt:review` — implementation review findings numbered across all agents
- `/edikt:audit` — security and reliability findings numbered across sections
- `/edikt:drift` — diverged findings include triage prompt
- `/edikt:doctor` — warnings and failures numbered for easy reference

### Natural language triggers for all 24 commands

The CLAUDE.md command table now matches intent, not exact phrases. All 24 commands have natural language triggers (was 14). Each command has an intent label and broader representative examples. "Create me a plan for this ticket", "help me plan this out", "spec this out", "are we on track with the spec", "run a security audit", "check my setup" — all trigger the right command.

### Bug fixes

- **Init hook filename hallucination** — `/edikt:init` now reads the settings template exactly as-is instead of generating hook filenames. Fixes `stop-signals.sh: No such file or directory` error.
- **PostToolUse gofmt error** — `gofmt -w` failures on invalid Go syntax no longer propagate as hook errors.
- **Drift report only saving frontmatter** — `/edikt:drift` now explicitly writes the full report (frontmatter + body), not just the frontmatter.
- **Plan mode guard** — All 8 interactive commands (`init`, `plan`, `prd`, `spec`, `spec-artifacts`, `adr`, `invariant`, `intake`) now detect plan mode and tell you to exit it first, instead of silently skipping the interview.
- **Installer preserves customized commands** — `install.sh` now checks for `<!-- edikt:custom -->` before overwriting, so customized commands survive reinstall.

### spec-artifacts redesign — design blueprints with database type awareness

`/edikt:spec-artifacts` now treats every artifact as a design blueprint: it defines intent and structure, not implementation. Your code is the implementation.

**Database-type-aware data model.** The data model artifact format is now resolved from your database type:

- SQL → `data-model.mmd` (Mermaid ERD with entities, relationships, index comments)
- MongoDB/Firestore → `data-model.schema.yaml` (JSON Schema in YAML)
- DynamoDB/Cassandra → `data-model.md` (access patterns, PK/SK/GSI design)
- Redis/KV stores → `data-model.md` (key schema table with TTL and namespace)
- Mixed stacks → both artifacts, suffixed to avoid collision (`data-model-sql.mmd`, `data-model-kv.md`, etc.)

**Database type resolution — four-priority chain:** spec frontmatter `database_type:` → config `artifacts.database.default_type` → keyword scan of spec content → ask the user. Config is set automatically by `/edikt:init` from code signals.

**Native artifact formats.** API contracts are now OpenAPI 3.0 YAML (`contracts/api.yaml`). Event contracts are AsyncAPI 2.6 YAML (`contracts/events.yaml`). Fixtures are portable YAML (`fixtures.yaml`). Migrations are numbered SQL files (`migrations/001_name.sql`). No more markdown wrappers.

**Migrations are SQL-only.** Document and key-value databases never produce migration files.

**Invariant injection.** Active invariants are loaded from your governance chain, stripped of frontmatter, and injected as structured constraints into every agent prompt. Superseded invariants are excluded. Empty invariant bodies emit a warning.

**Design blueprint header.** Every generated artifact gets a format-appropriate comment header marking it as a blueprint, not implementation code.

**Config contract.** `/edikt:init` now detects database type and migration tool from code signals and writes `artifacts.database.default_type` and `artifacts.sql.migrations.tool` to config. The `artifacts:` block is now part of the standard config schema.

### HTML sentinel migration — CLAUDE.md section boundaries now visible to Claude

Claude Code v2.1.72+ hides `<!-- -->` HTML comments when injecting `CLAUDE.md` into Claude's context. The old `<!-- edikt:start -->` / `<!-- edikt:end -->` sentinels were invisible to Claude, so asking Claude to "edit my CLAUDE.md" could accidentally overwrite edikt's managed section.

New format uses markdown link reference definitions, which survive Claude Code's injection intact:

```
[edikt:start]: # managed by edikt — do not edit this block manually
...
[edikt:end]: #
```

- `/edikt:init` writes the new format on fresh installs and migrates old markers when re-running
- `/edikt:upgrade` detects and migrates old HTML sentinels as part of the upgrade flow
- Both old and new formats are detected for backward compatibility
- ADR-002 updated to document the change and rationale

### Effort frontmatter on all commands

All 24 commands now declare `effort: low | normal | high` in their frontmatter. Claude Code uses this to tune the model's thinking budget per command.

- `low` — `agents`, `context`, `mcp`, `status`, `team`
- `normal` — `adr`, `compile`, `doctor`, `init`, `intake`, `invariant`, `review-governance`, `rules-update`, `session`, `sync`, `upgrade`
- `high` — `audit`, `docs`, `drift`, `plan`, `prd`, `review`, `spec`, `spec-artifacts`

### Init improvements

- **Existing ADR import** — `/edikt:init` now detects existing architecture decisions and offers to import them into edikt's governance structure.
- **Project-local install** — `install.sh --project` installs edikt into the current project (`.claude/commands/`, `.edikt/`) instead of globally. Default is still global.
- **Database detection** — `/edikt:init` detects database type and migration tool from 30+ code signals across Go, Node, Python, Ruby, C#, Elixir, and Rust. Definitive signals (e.g., `prisma/schema.prisma`) auto-configure. Inferred signals (package dependencies) are flagged. Nothing found triggers targeted greenfield questions.

## v0.1.0 (2026-03-23)

### First public release

edikt governs your architecture and compiles your engineering decisions into automatic enforcement. It governs the Agentic SDLC from requirements to verification — closing the gap between what you decided and what gets built.

**Architecture governance & compliance**
- `/edikt:compile` reads accepted ADRs, active invariants, and team guidelines, checks for contradictions, and produces `.claude/rules/governance.md` — directives Claude follows automatically every session
- 20 rule packs (10 base, 4 lang, 6 framework) — correctness guardrails, not opinions. 14-17 instructions per pack (research-validated sweet spot)
- Domain-specific governance checkpoints with pre-action and post-result verification
- Signal detection: stop hook detects architecture decisions mid-session, suggests ADR capture
- Quality gates: configure agents as gates in `.edikt/config.yaml`. Critical findings block progression with logged override
- Pre-push invariant check: violations block the push. Override with `EDIKT_INVARIANT_SKIP=1`

**Agentic SDLC governance**
- Full traceability chain: `/edikt:prd` → `/edikt:spec` → `/edikt:spec-artifacts` → `/edikt:plan` → execute → `/edikt:drift`
- Status-gated transitions: PRD must be accepted before spec, spec before artifacts
- `/edikt:drift` compares implementation against the full chain with confidence-based severity
- CI support: `--output=json` with exit code 1 on diverged findings

**18 specialist agents**
- architect, api, backend, dba, docs, frontend, performance, platform, pm, qa, security, sre, ux, data, mobile, compliance, seo, gtm
- Used in spec review, plan pre-flight, post-implementation review, and audit

**9 lifecycle hooks**
- SessionStart: git-aware briefing with domain classification
- UserPromptSubmit: injects active plan phase into every prompt
- PostToolUse: auto-formats files after edits
- PostCompact: re-injects plan + invariants after context compaction
- Stop: regex-based signal detection for decisions, doc gaps, security
- SubagentStop: logs agent activity, enforces quality gates
- InstructionsLoaded: logs active rule packs
- PreToolUse: validates governance setup
- PreCompact: preserves plan state

**24 commands**
- Governance chain: `init`, `prd`, `spec`, `spec-artifacts`, `plan`, `drift`, `compile`
- Decisions: `adr`, `invariant`
- Review: `review`, `audit`, `review-governance`, `doctor`
- Observability: `status`, `session`, `docs`
- Setup: `context`, `intake`, `upgrade`, `rules-update`, `sync`, `team`, `mcp`, `agents`

**Research**
- 123 eval runs across 2 experiments proving rule compliance mechanism
- EXP-001: 15/15 compliance with rules vs 0/15 without on invented conventions
- EXP-002: holds under multi-rule conflict, multi-file sessions, Opus vs Sonnet, adversarial prompts
- Reproducible: `test/experiments/rule-compliance/exp-001-scenarios/` and `test/experiments/rule-compliance/exp-002-scenarios/`

**Website**
- Full documentation at edikt.dev
- Guides: solo engineer, teams, multi-project, greenfield, brownfield, monorepo, security, daily workflow
- Governance section: chain, gates, compile, drift, review-governance

**Zero dependencies**
- Every file is `.md` or `.yaml` — no build step, no runtime, no daemon
- `curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash`
- Claude Code only — uses platform primitives (path-conditional rules, lifecycle hooks, slash commands, specialist agents, quality gates)
