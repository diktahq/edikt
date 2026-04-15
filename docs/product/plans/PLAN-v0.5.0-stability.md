# Plan: v0.5.0 Stability Release

## Overview

**Task:** Implement SPEC-004 (v0.5.0 stability release) — testing infrastructure, versioning & rollback, Homebrew distribution, init provenance, multi-version migration.
**Source spec:** `docs/product/specs/SPEC-004-v050-stability/spec.md`
**Source PRD:** `docs/product/prds/PRD-002-v050-stability-release.md`
**Total Phases:** 14
**Estimated Cost:** ~$3.21 (3 opus + 10 sonnet + 1 haiku)
**Created:** 2026-04-14

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | done | 1/5 | 2026-04-14      |
| 2     | done (partial — fixtures + wiring; hook tests gated on 11b) | 1/5 | 2026-04-14 |
| 3     | done | 1/5 | 2026-04-15 |
| 4     | done | 1/5 | 2026-04-15 |
| 5     | done | 1/5 | 2026-04-15 |
| 6     | done | 1/5 | 2026-04-15      |
| 7     | pending | 0/5 | -      |
| 8     | pending | 0/5 | -      |
| 9     | pending | 0/5 | -      |
| 10    | pending | 0/5 | -      |
| 11    | pending | 0/5 | -      |
| 11b   | pending (was 2b.i — characterize hooks: rewrite fixtures against actual behavior, sandbox-staged, flip `EDIKT_ENABLE_HOOK_JSON_TESTS=1`) | 0/5 | - |
| 12    | pending | 0/5 | -      |
| 13    | pending | 0/5 | -      |
| 14    | pending | 0/5 | -      |

**Deferred to v0.6.0:** Phase 2b.ii (hook semantic rewrites — subagent-stop structured evaluator input per ADR-010, session-start/user-prompt wording alignment, stop-hook command renames). See `docs/internal/plans/ROADMAP.md` v0.6.0 section.

**IMPORTANT:** Update this table as phases complete. This table is the persistent state that survives context compaction.

## Model Assignment

| Phase | Task | Model | Reasoning | Est. Cost |
|-------|------|-------|-----------|-----------|
| 1 | Test runner sandbox isolation | haiku | Mechanical shell rewrite, `HOME` redirect | $0.01 |
| 2 | Layer 1 hook unit tests + fixtures | sonnet | Test design requires judgment on fixture shape | $0.08 |
| 3 | Launcher core + versioned layout | opus | Largest surface; state management, symlinks, rollback, EDIKT_ROOT abstraction, min_payload_version; INV-001 risk | $0.80 |
| 4 | M1 migration (flat→versioned) | sonnet | Data preservation + crash recovery | $0.08 |
| 5 | install.sh rewrite (canonical cross-major path) | sonnet | User-facing bootstrap; elevated from haiku because it owns the v0.4.x→v0.5.0 migration dispatch | $0.08 |
| 6 | Launcher extended CLI | sonnet | prune, upgrade, upgrade-pin, dev link/unlink + slash-upgrade redirect | $0.08 |
| 7 | Multi-version migration M1-M5 + capture.sh | opus | Cross-version knowledge, five source layouts, frozen-fixture correctness | $0.80 |
| 8 | Project-mode install parity | sonnet | Mirrors global via EDIKT_ROOT; hook path rewriting | $0.08 |
| 9 | Init provenance | sonnet | Path substitution + stack filters + hash-before-substitution contract | $0.08 |
| 10 | Upgrade provenance-first flow | opus | Complex comparison + 3-way diff; data-loss risk if wrong | $0.80 |
| 11 | M6 provenance backfill | sonnet | Contract already defined by phases 9 + 10 | $0.08 |
| 11b | Hook fixture characterization (was 2b.i) | sonnet | Rewrite fixtures against actual hook behavior now that all hook-touching phases (7's M4 rule recompile) have landed; sandbox-stage for determinism; flip `EDIKT_ENABLE_HOOK_JSON_TESTS=1` | $0.08 |
| 12 | Layer 2 integration tests + regression museum | sonnet | Pytest + SDK + fuzzy-match snapshots + code-path assertions | $0.08 |
| 13 | Homebrew formula + release automation | sonnet | Ruby DSL + homebrew-releaser + tap CI isolation | $0.08 |
| 14 | Docs + website refresh + CI + doctor --report | sonnet | Writing quality + cross-link verification + observability | $0.08 |

## Execution Strategy

All phases run **sequentially** per user decision. No parallel waves.

| Phase | Depends On | Parallel With |
|-------|-----------|---------------|
| 1     | None      | -             |
| 2     | 1         | -             |
| 3     | 1         | -             |
| 4     | 3         | -             |
| 5     | 3, 4      | -             |
| 6     | 3, 4      | -             |
| 7     | 1, 4      | -             |
| 8     | 3, 4      | -             |
| 9     | None (independent surface; can land any time after P1) | - |
| 10    | 9         | -             |
| 11    | 7, 10     | -             |
| 11b   | 2, 7      | -             |
| 12    | 2, 7, 10, 11b | -         |
| 13    | 3, 5      | -             |
| 14    | All (incl. 11b — CI gate flips `EDIKT_ENABLE_HOOK_JSON_TESTS=1`) | - |

## Artifact Flow

| Producing Phase | Artifact | Consuming Phase(s) |
|-----------------|----------|---------------------|
| 1 | `test/run.sh` (sandboxed preamble) | All subsequent phases |
| 2 | `test/unit/hooks/*.sh`, `test/fixtures/hook-payloads/*.json` | 12 (integration references), 14 (CI gate) |
| 3 | `bin/edikt`, `~/.edikt/versions/`, `~/.edikt/current`, `lock.yaml`, `manifest.yaml` | 4, 5, 6, 7, 8, 11, 13 |
| 4 | M1 migration logic + `edikt migrate --abort` | 5, 7 |
| 5 | `install.sh` (thin bootstrap + legacy-detection dispatch) | 13 (brew formula references release tarball) |
| 6 | `rollback`, `prune`, `upgrade`, `upgrade-pin`, `dev`, `/edikt:upgrade` redirect | 10, 12, 14 |
| 7 | M2-M5 migration logic, `test/integration/migration/fixtures/v0.*/` | 11, 12 |
| 8 | Project-mode launcher dispatch | 12, 14 |
| 9 | `templates/agents/_substitutions.yaml`, agents with `<!-- edikt:stack:... -->` markers, `edikt_template_hash` frontmatter spec | 10, 11 |
| 10 | Provenance-first upgrade in `commands/upgrade.md` | 11, 12 |
| 11 | M6 backfill in `edikt doctor --backfill-provenance` | 12 (test coverage) |
| 11b | Rewritten `test/expected/hook-outputs/*.expected.json`, updated `fixtures.yaml` §9.1, gate removed from `test/run.sh` + `test/unit/hooks/test_*.sh` | 12 (regression museum), 14 (CI gate runs hooks by default) |
| 12 | `test/integration/**`, regression museum, `test/integration/failures/` | 14 (CI gate) |
| 13 | `Formula/edikt.rb` in `diktahq/homebrew-tap`, `.github/workflows/release.yml`, GitHub Release tarballs | 14 (docs link brew instructions) |
| 14 | README, website guides, CHANGELOG, `.github/workflows/test.yml`, `edikt doctor --report` | Release cut |

## Pre-Flight Review Summary

Run on 2026-04-14 — findings already folded into the phases below. Full findings preserved for traceability.

**Critical — resolved:**
- 🔴 SRE #1 (phase ordering risk) → Phase 5 install.sh rewrite sequenced after M1 (Phase 4) so legacy-layout detection has a migration target. EDIKT_EXPERIMENTAL gate is NOT needed because v0.5.0 ships atomically (not phase-by-phase to users); Phase 5 AC 5.8 explicitly removes any experimental gating.
- 🔴 SRE #2 (v0.4.3→v0.5.0 bridge) → **Resolved via Option B**: `install.sh` is the canonical cross-major upgrade path. `/edikt:upgrade` slash command (Phase 6) detects major-version jump and redirects. No v0.4.4 intermediate release.
- 🔴 SRE #3 (tap CI isolation) → Phase 13 AC includes staging-branch verification + full tap matrix (edikt + verikt smoke).
- 🔴 Architect #1 (M6 timing) → Phase 7 scoped to M1-M5 only. M6 backfill is its own Phase 11, after Phase 10's upgrade flow defines the hash contract.
- 🔴 Architect #2 (hash vs version semantics) → **Locked decision**: `edikt_template_hash` is content identity. `edikt_template_version` is written once at install time and does NOT bump on upgrade-preserved files. Version field answers "when was this file installed," not "was it touched by a recent upgrade."
- 🔴 Architect #4 (EDIKT_ROOT retrofit) → Phase 3 AC includes `EDIKT_ROOT` abstraction from day 1. Phase 8 becomes a targeted addition, not a rewrite.

**Warnings — folded:**
- 🟡 SRE #4: retry/backoff + `--skip-integration-on-outage` in Phase 12 AC.
- 🟡 SRE #5: `capture.sh` runs inside Phase 1 sandbox (Phase 7 depends on Phase 1, not just Phase 4).
- 🟡 SRE #6: rollback moved into Phase 3 (launcher core), removed from Phase 6.
- 🟡 SRE #7: `edikt doctor --report` bundle added to Phase 14.
- 🟡 SRE #8: NFS/WSL1 `doctor` probe + docs note in Phase 14.
- 🟡 SRE #9: Phase 3 AC includes "re-run Phase 2 tests post-symlink work; any breakage indicates real regression, not test brittleness".
- 🟡 SRE #10: Phase 4 AC includes staging dir + trap + `kill -9` mid-migration recovery test.
- 🟡 Architect #3 (rollback doesn't un-migrate) → **Locked decision**: `edikt rollback` is **payload-only**. Migrations M1-M6 are permanent once accepted. Documented in Phase 14 upgrade-and-rollback guide.
- 🟡 Architect #5 (min_payload_version) → Phase 3 AC includes the constant + refusal-to-activate logic.
- 🟡 Architect #6 (regression museum tests the class) → Phase 12 AC includes `assert_path_covered("upgrade.legacy_classifier")` and equivalent assertions for each regression fixture.

## Known Risks

- **Cross-major upgrade UX** — users must know to re-run `install.sh` for v0.4.x→v0.5.0. Mitigation: Phase 6's `/edikt:upgrade` slash command detects and redirects; Phase 14's `migrating-from-v0.4.md` guide + FAQ entry + CHANGELOG Migration Notes all document the path.
- **Integration test determinism** — Agent SDK has no mock/replay mode. Snapshot tests use fuzzy-match to tolerate model variance; drift still requires human review per Phase 12.
- **Rollback is payload-only** — users who roll back after a migration keep the migration mutations (sentinels, compile output, config schema changes). Design choice, not a bug. Documented.
- **NFS / WSL1 symlink support** — best-effort only. Phase 14 adds doctor probe + workaround docs. Not CI-tested.

## Deferred Artifacts

None — all spec artifacts (test-strategy.md, config-spec.md, fixtures.yaml) are covered by Phase 12 (integration tests consume all three), Phase 2 (hook fixtures from fixtures.yaml), and Phase 3+ (launcher state per config-spec.md).

## Artifact Coverage Check

```
✓ fixtures.yaml → Phase 2 (hook payloads), Phase 7 (migration fixtures), Phase 12 (integration fixtures, regression museum)
✓ test-strategy.md → Phases 2 (Layer 1), 12 (Layer 2 + regression museum), 14 (CI wiring)
✓ config-spec.md → Phases 3 (lock.yaml, manifest.yaml), 4 (migration events), 6 (env vars), 14 (doctor --report)
All spec artifacts have plan coverage (3/3).
```

---

## Phase 1: Test Runner Sandbox Isolation (Layer 3)

**Objective:** Eliminate shared-state flakiness by redirecting `$HOME`, `$EDIKT_HOME`, `$CLAUDE_HOME` to a per-run temp tree in `test/run.sh`.
**Model:** `haiku`
**Max Iterations:** 5
**Completion Promise:** `SANDBOX READY`
**Evaluate:** false
**Dependencies:** None
**Context Needed:**
- `test/run.sh` — current test runner
- `test/helpers.sh` — existing assertion helpers
- `docs/product/specs/SPEC-004-v050-stability/spec.md` (§9.3 "Layer 3 — Sandboxed runner")
- `docs/product/specs/SPEC-004-v050-stability/config-spec.md` (environment variables section)
- `docs/product/specs/SPEC-004-v050-stability/test-strategy.md` (Layer 3 rows)

**Acceptance Criteria:**
- [ ] `test/run.sh` exports `HOME=$TEST_SANDBOX/home`, `EDIKT_HOME=$HOME/.edikt`, `CLAUDE_HOME=$HOME/.claude` at the top of the script, before sourcing any helpers.
- [ ] `TEST_SANDBOX` is created via `mktemp -d -t edikt-test-XXXXXX` and cleaned up via `trap` on EXIT.
- [ ] `test/run.sh` accepts `SKIP_INTEGRATION=1` env var and skips `test/integration/` when set.
- [ ] A new `test/helpers.sh` function `sandbox_setup` initializes `$EDIKT_HOME` and `$CLAUDE_HOME` as empty dirs; tests needing pre-state populate inside the sandbox.
- [ ] Running `./test/run.sh` 10 times consecutively on a machine with a live Claude Code session produces identical pass/fail output. Verify via: `for i in {1..10}; do ./test/run.sh > /tmp/run-$i.txt; done && diff /tmp/run-1.txt /tmp/run-2.txt && diff /tmp/run-1.txt /tmp/run-10.txt`.
- [ ] Any test relying on `git log` of the cwd explicitly `git init` inside the sandbox or opts out via `skip_if_no_git`.

**Prompt:**
```
You are implementing Phase 1 of PLAN-v0.5.0-stability: test runner sandbox isolation.

Context to read before writing code:
- test/run.sh
- test/helpers.sh
- SPEC-004 §9.3 for the target shape
- config-spec.md for the environment variable contract (HOME, EDIKT_HOME, CLAUDE_HOME)

Implement:

1. Rewrite test/run.sh preamble per SPEC-004 §9.3 exactly:
     TEST_SANDBOX=$(mktemp -d -t edikt-test-XXXXXX)
     export HOME="$TEST_SANDBOX/home"
     export EDIKT_HOME="$HOME/.edikt"
     export CLAUDE_HOME="$HOME/.claude"
     mkdir -p "$EDIKT_HOME" "$CLAUDE_HOME"
     trap 'rm -rf "$TEST_SANDBOX"' EXIT

2. Add SKIP_INTEGRATION=1 handling — default off. When set, skip the
   integration sub-runner (not yet present, but the branch must exist).

3. Add sandbox_setup helper to test/helpers.sh. Tests that need state
   in $EDIKT_HOME or $CLAUDE_HOME invoke it explicitly.

4. Audit existing tests for:
     - git log usage on cwd (add `git init` inside sandbox or skip_if_no_git)
     - hard-coded $HOME references (replace with $HOME env, which is now
       the sandbox home)
     - Cross-test state (temp files outside $TEST_SANDBOX) — kill them.

5. Verify by running the full suite 10× back-to-back while another
   Claude Code session is active in the same terminal session. Output
   must be identical across all 10 runs.

Do not change the semantics of any individual test — this phase is
purely about isolation. If a test was fragile, note it in a comment
and fix in later phases.

When complete, output: SANDBOX READY
```

---

## Phase 2: Layer 1 Hook Unit Tests + Payload Fixtures

**Objective:** Replace offline string-grep "hook tests" with fixture-driven tests that pipe real JSON stdin payloads to each hook and assert on exit code + stdout JSON.
**Model:** `sonnet`
**Max Iterations:** 5
**Completion Promise:** `HOOK TESTS READY`
**Evaluate:** true
**Dependencies:** Phase 1
**Context Needed:**
- `test/run.sh` (now sandboxed, from Phase 1)
- `templates/hooks/*.sh` — all 16 hook scripts
- `docs/product/specs/SPEC-004-v050-stability/spec.md` (§9.1 "Layer 1 — Hook unit tests")
- `docs/product/specs/SPEC-004-v050-stability/fixtures.yaml` (hook payload + expected output scenarios)
- `docs/product/specs/SPEC-004-v050-stability/test-strategy.md` (Layer 1 rows)

**Acceptance Criteria:**
- [ ] `test/unit/hooks/` exists with one `test_<hook_name>.sh` per lifecycle hook (9+ files).
- [ ] `test/fixtures/hook-payloads/` contains every JSON payload named in SPEC-004 §9.1 and fixtures.yaml (minimum 19 pairs: session-start {with/without edikt}, user-prompt-submit {no-plan, with-plan}, pre-tool-use-write, post-tool-use {go, ts}, stop {adr-candidate, new-route, new-env-var, security-change, clean, loop-guard}, subagent-stop {critical, warning, ok}, pre-compact, post-compact {with-plan, with-failing-criteria}, instructions-loaded).
- [ ] `test/fixtures/hook-payloads/` and `test/expected/hook-outputs/` have symmetric filenames (every input has exactly one expected-output sibling). Enforced by a meta-test `test/unit/test_fixture_symmetry.sh`.
- [ ] Each test pipes the fixture JSON to the hook script via `cat fixture.json | hook.sh` and asserts: exit code matches, stdout JSON matches expected via `jq -S . | diff`.
- [ ] Negative-path fixtures (hooks must NOT fire when `.edikt/config.yaml` is absent, or when feature flags disable them) are included and pass.
- [ ] No fixture contains hardcoded user paths or timestamps — enforced by `grep -r '/Users/\|/home/' test/fixtures/ && exit 1`.
- [ ] The suite runs cleanly under sandbox isolation from Phase 1 (10/10 consecutive runs identical).

**Prompt:**
```
You are implementing Phase 2: Layer 1 hook unit tests.

Context to read:
- test/run.sh (sandboxed, from Phase 1)
- All 16 scripts in templates/hooks/
- SPEC-004 §9.1 — target test layout
- fixtures.yaml — the complete scenario list for hook-payloads and hook-outputs

Implement:

1. Create test/fixtures/hook-payloads/ with one JSON file per scenario
   enumerated in fixtures.yaml. Each file is a real Claude Code hook
   stdin payload — look at the hook scripts themselves to see what
   fields they read (last_assistant_message, tool_input, stop_hook_active,
   etc.) and produce payloads that exercise each code path.

2. Create test/expected/hook-outputs/ with one expected-output JSON
   file per input fixture. Symmetric naming: payloads/foo.json has
   outputs/foo.expected.json.

3. Create test/unit/hooks/test_<hook>.sh per hook. Each test iterates
   through its relevant fixtures, runs:
     actual=$(cat fixture.json | templates/hooks/<hook>.sh 2>/dev/null)
     diff <(echo "$actual" | jq -S .) <(cat expected.json | jq -S .)
   Fail loudly on mismatch.

4. Include negative-path fixtures: session-start-no-edikt.json (no
   .edikt/config.yaml → hook exits 0 with no output), stop-clean.json
   (no signals in message → no systemMessage emitted), etc. The full
   negative set is in fixtures.yaml scenario 1.

5. Add test/unit/test_fixture_symmetry.sh — a meta-test that lists
   both directories and fails if any payload has no matching output
   or vice versa. This prevents fixture rot.

6. Add test/unit/test_no_hardcoded_paths.sh — greps all fixtures for
   literal user paths. Fails loudly if found. Any path must be
   relative or use $HOME placeholder.

7. Wire test/unit/ into test/run.sh (Layer 1 runs on every invocation,
   ahead of anything requiring API access).

Every hook MUST have at least one happy-path fixture + one
negative-path fixture. No exceptions.

When complete, output: HOOK TESTS READY
```

**Outcome (2026-04-14):** Fixtures + meta-tests + hook-test skeletons + run.sh wiring landed. Hook-level assertions are gated behind `EDIKT_ENABLE_HOOK_JSON_TESTS=1` because the current hook scripts emit plaintext (`echo`/`printf`) rather than the Claude Code hook JSON protocol (`{continue, systemMessage, additionalContext, decision}`) that the expected-output fixtures encode. Flipping the gate on without migrating the hooks would produce 21 red fixtures on day one masquerading as "test brittleness." Migration tracked in **Phase 2b**. All other Phase 2 acceptance criteria (fixture count ≥19, symmetric naming, negative-path fixtures, no hardcoded paths, sandbox-clean 10/10) are met.

---

## Phase 2b history (2026-04-14): split, then re-scoped

Phase 2b was originally "migrate hooks to JSON protocol." On reading the 9 hooks, the gap turned out to be semantic, not just presentational — see commit `1a315bf` for the finding. It was split into two:

- **2b.i — Characterize hooks** (rewrite fixtures against actual behavior, sandbox-staged for determinism, flip `EDIKT_ENABLE_HOOK_JSON_TESTS=1`). **Re-scheduled as Phase 11b** (below) so it captures final v0.5.0 hook behavior after Phase 7's M4 rule recompile, rather than mid-flight.
- **2b.ii — Hook semantic rewrites** (subagent-stop structured evaluator input per ADR-010, wording alignment). **Deferred to v0.6.0** per `docs/internal/plans/ROADMAP.md`. Needs its own ADR for the hook input contract.

The original Phase 2b body (JSON-protocol migration) is fully superseded by 11b's characterization approach: fixtures now adapt to hooks, not the other way around. No content from the original 2b prompt is needed.

---

## (original Phase 2b — superseded by 11b)

**Original objective:** Rewrite the 9 lifecycle hooks listed in SPEC-004 §9.1 to read JSON stdin and emit JSON stdout per Claude Code's hook protocol (`continue`, `systemMessage`, `additionalContext`, `decision`). Once migrated, flip `EDIKT_ENABLE_HOOK_JSON_TESTS=1` as the default in `test/run.sh` so Phase 2 hook tests run on every invocation.
**Model:** `sonnet`
**Max Iterations:** 5
**Completion Promise:** `HOOK JSON PROTOCOL READY`
**Evaluate:** true
**Dependencies:** Phase 2
**Context Needed:**
- `templates/hooks/*.sh` (current plaintext-emitting scripts)
- `test/fixtures/hook-payloads/*.json` + `test/expected/hook-outputs/*.expected.json` (the target contract)
- `docs/product/specs/SPEC-004-v050-stability/spec.md` §9.1 — target output shape
- Claude Code hook protocol reference (read cwd from JSON stdin, emit JSON on stdout, exit 0)

**Acceptance Criteria:**
- [ ] Each of the 9 hooks (`session-start.sh`, `user-prompt-submit.sh`, `pre-tool-use.sh`, `post-tool-use.sh`, `stop-hook.sh`, `subagent-stop.sh`, `pre-compact.sh`, `post-compact.sh`, `instructions-loaded.sh`) reads payload fields from stdin via `jq -r`, not from shell env or argv.
- [ ] Each hook emits either a valid JSON object on stdout (`continue`, `systemMessage`, `additionalContext`, `decision` per spec §9.1) or exits 0 silently — never plaintext.
- [ ] `cwd` for filesystem work is taken from the JSON payload (`.cwd`), not from `$PWD`. This is what makes the tests sandbox-able.
- [ ] `EDIKT_ENABLE_HOOK_JSON_TESTS` default in `test/run.sh` flips to `1`. All 21 fixture pairs pass against the migrated hooks.
- [ ] `templates/settings.json.tmpl` hook invocations unchanged — same paths, same lifecycle events. Only the script internals change.
- [ ] No regression in `test-hooks.sh`, `test-stop-hook-e2e.sh`, or any existing hook suite.

**Prompt:**
```
You are implementing Phase 2b: migrate hooks to Claude Code JSON protocol.

Context:
- The current hooks emit plaintext via echo/printf. Claude Code tolerates
  this today, but Phase 2's fixture tests expect structured JSON output
  (continue/systemMessage/additionalContext/decision). Tests are gated
  behind EDIKT_ENABLE_HOOK_JSON_TESTS=1 until this phase lands.

- Read test/fixtures/hook-payloads/*.json for the exact input shape each
  hook will see. Read test/expected/hook-outputs/*.expected.json for the
  exact output shape each hook must produce.

Implementation:

1. Pick one hook at a time. For each:
   a. Parse stdin once: PAYLOAD=$(cat); use jq -r on $PAYLOAD for fields.
   b. Take cwd from .cwd in the payload, not $PWD. This is load-bearing
      for sandbox testing.
   c. Emit the JSON output at exit time via jq -cn or a single printf
      of a valid JSON object. Empty output + exit 0 is valid for the
      "no-op" case (e.g. session-start-no-edikt, stop-loop-guard).
   d. Preserve side effects (git analysis, rotation of session-signals.log,
      etc.). Only the presentation layer (stdout) changes shape.

2. Flip test/run.sh default to EDIKT_ENABLE_HOOK_JSON_TESTS=1. Keep the
   env var so engineers debugging a single hook can opt out during
   iteration.

3. Run ./test/run.sh. All 45+ suites must remain green. The 9 unit/hooks/
   suites must flip from SKIP to PASS (21 fixture pairs).

Pitfalls:
- Do NOT break test-hooks.sh or test-stop-hook-e2e.sh. They exercise
  end-to-end behavior and may rely on plaintext today. If they break,
  they need matching updates in this same phase — not a follow-up.
- Hooks still run under `set -uo pipefail`. jq pipelines that fail must
  still exit 0 when the hook's contract is "silently no-op."

When complete, output: HOOK JSON PROTOCOL READY
```

---

## Phase 3: Launcher Core + Versioned Layout + Symlinks

**Objective:** Implement `~/.edikt/bin/edikt` shell launcher with foundational subcommands (install, use, list, version, doctor, uninstall, rollback), the versioned on-disk layout, the generation symlink chain, `lock.yaml` + `manifest.yaml` formats, and `EDIKT_ROOT`/`min_payload_version` abstractions.
**Model:** `opus`
**Max Iterations:** 5
**Completion Promise:** `LAUNCHER CORE READY`
**Evaluate:** true
**Dependencies:** Phase 1
**Context Needed:**
- `install.sh` (current, 453 lines)
- `commands/upgrade.md` (current version-reading logic)
- `templates/settings.json.tmpl` (hardcoded `$HOME/.edikt/hooks/*.sh` paths that must remain valid through symlink chain)
- `docs/product/specs/SPEC-004-v050-stability/spec.md` (§1 layout, §2 launcher subcommand contracts)
- `docs/product/specs/SPEC-004-v050-stability/config-spec.md` (lock.yaml schema, manifest.yaml schema, events.jsonl new event types, filesystem & permissions section)
- INV-001 invariant — launcher must remain POSIX sh, no Python/compiled runtime

**Acceptance Criteria:**
- [ ] `bin/edikt` is a POSIX shell script (no bash-isms, tested via `sh -n bin/edikt` and `checkbashisms bin/edikt` if available). No Python, no Node, no compiled dependency.
- [ ] `bin/edikt` resolves `EDIKT_ROOT` as the first operation in every subcommand: prefers env var, then `$PWD/.edikt/bin/edikt` ancestor detection (project mode), then `$HOME/.edikt` (global mode). Documented in launcher `--help`.
- [ ] `bin/edikt` defines `MIN_PAYLOAD_VERSION` constant and refuses to activate a payload older than that constant with a clear error message.
- [ ] Subcommands implemented: `install <tag>`, `use <tag>`, `list [--verbose]`, `version`, `doctor`, `uninstall [--yes]`, `rollback`. Each matches the contract in SPEC-004 §2 (exit codes, behavior).
- [ ] On `install <tag>`: launcher fetches `github.com/diktahq/edikt/archive/refs/tags/<tag>.tar.gz`, extracts to `$EDIKT_ROOT/versions/<tag>/`, writes `manifest.yaml` with SHA256 per file, verifies checksum against an embedded reference. Fails with exit 2 on checksum mismatch.
- [ ] On `use <tag>`: launcher snapshots current symlink target to `$EDIKT_ROOT/backups/<ts>/`, flips `$EDIKT_ROOT/current` to `versions/<tag>`, verifies `$EDIKT_ROOT/hooks`, `$EDIKT_ROOT/templates`, `$CLAUDE_HOME/commands/edikt` symlinks all resolve, updates `lock.yaml` atomically (write to `.tmp` then `mv`).
- [ ] `rollback` reads `lock.yaml:previous`, exits 1 with message if unset, otherwise invokes `use <previous>`.
- [ ] `doctor` verifies: launcher version, payload version, every symlink resolves, `manifest.yaml` SHA256s match disk state, `EDIKT_ROOT` is writable. Reports structured output.
- [ ] `uninstall` prompts unless `--yes`, removes `$EDIKT_ROOT/`, unlinks `$CLAUDE_HOME/commands/edikt`. Never touches any project's `.edikt/`.
- [ ] `lock.yaml` schema matches config-spec.md §4.2 exactly (`active`, `previous`, `installed_at`, `installed_via`, `history[]`).
- [ ] `manifest.yaml` schema matches config-spec.md §4.2 exactly (`version`, `installed_at`, `files[]` with `path` + `sha256`).
- [ ] Writes `version_installed` + `version_activated` + `rollback_performed` events to `$EDIKT_ROOT/events.jsonl` per config-spec.md §4.4 schemas.
- [ ] `flock` on `$EDIKT_ROOT/.lock` serializes concurrent launcher invocations. Detects and exits cleanly on NFS `EXDEV` / `ENOLCK` with a documented fallback message.
- [ ] Unit tests in `test/unit/launcher/test_*.sh` cover every subcommand's happy path + at least one failure path (network error, checksum mismatch, missing version, no previous).
- [ ] Phase 2's hook unit tests continue to pass after symlink chain installation. If any break, the breakage is a real regression, not test brittleness.

**Prompt:**
```
You are implementing Phase 3: launcher core + versioned layout.

This is the foundational phase of v0.5.0. Every later phase depends
on a correct launcher. Opus is assigned for a reason — take the
complexity seriously.

Context to read before writing:
- install.sh end to end
- commands/upgrade.md
- templates/settings.json.tmpl (note every `$HOME/.edikt/hooks/*.sh` —
  these paths must resolve through the symlink chain you're building)
- SPEC-004 §1 (versioned install layout), §2 (subcommand contracts)
- config-spec.md §4.2 (lock.yaml + manifest.yaml schemas), §4.4 (event
  types), §7 (symlink compat matrix), §8 (operational concerns:
  flock, staging dir, EXDEV, MIN_PAYLOAD_VERSION)
- INV-001 — launcher is POSIX sh. Verify with `sh -n` before committing.

Implementation order (do NOT skip):

1. Define EDIKT_ROOT resolution. This is the FIRST thing every
   subcommand does. Project-mode (phase 8) will piggyback on this
   abstraction — if you hardcode $HOME anywhere in the launcher,
   phase 8 becomes a rewrite. Don't.

2. Implement the versioned layout on disk per SPEC-004 §1:
     $EDIKT_ROOT/versions/<tag>/
     $EDIKT_ROOT/current -> versions/<tag>      (generation symlink)
     $EDIKT_ROOT/hooks -> current/hooks         (stable external path)
     $EDIKT_ROOT/templates -> current/templates
     $EDIKT_ROOT/bin/edikt                       (launcher itself — survives version flips)
     $EDIKT_ROOT/config.yaml                    (user data, untouched)
     $EDIKT_ROOT/custom/                         (user data)
     $EDIKT_ROOT/backups/<ts>/                   (pre-flip snapshots)
     $EDIKT_ROOT/lock.yaml
     $EDIKT_ROOT/events.jsonl
   And $CLAUDE_HOME/commands/edikt -> $EDIKT_ROOT/current/commands/edikt

3. Write the MIN_PAYLOAD_VERSION constant at the top of the launcher.
   On `use <tag>` or first invocation after launcher upgrade, check
   that the payload's VERSION file is >= MIN_PAYLOAD_VERSION. Refuse
   to activate an older payload with a clear remediation message.

4. Implement subcommands per SPEC-004 §2 table, in this order (so
   you can test as you go):
   a) version — trivial, just cat current/VERSION
   b) list — scan versions/, mark active (current symlink target)
   c) install — fetch tarball, extract, compute SHA256 manifest,
      verify against embedded checksum OR .sha256 sibling file.
      Must be interruptible safely (use a staging dir, move atomically).
   d) use — snapshot current, flip symlinks, update lock.yaml, emit
      event. Atomic where possible (rename, not copy-then-delete).
   e) rollback — read lock.yaml:previous, invoke use
   f) doctor — structured report, exit 0 healthy, 1 warnings, 2 errors
   g) uninstall — prompt unless --yes, remove $EDIKT_ROOT

5. flock serialization on $EDIKT_ROOT/.lock. Use flock(1) on Linux,
   a mkdir-based mutex fallback on macOS if flock is absent. Detect
   NFS ENOLCK and print the documented fallback per config-spec.md §8.8.

6. Atomic writes: lock.yaml and manifest.yaml updates write to
   ".tmp" sibling then mv to final. Never leave half-written files.

7. Unit tests in test/unit/launcher/test_*.sh. Use the Phase 1
   sandbox. Cover happy path + failure path for each subcommand:
   - test_install_happy.sh, test_install_checksum_mismatch.sh,
     test_install_network_fail.sh
   - test_use_happy.sh, test_use_missing_version.sh, test_use_pin_warn.sh
   - test_rollback_happy.sh, test_rollback_no_previous.sh
   - test_doctor_healthy.sh, test_doctor_broken_symlink.sh,
     test_doctor_manifest_tampered.sh
   - test_uninstall_prompt.sh, test_uninstall_yes.sh
   - test_concurrent_invocation.sh (two launchers race on .lock)

8. Re-run Phase 2 hook unit tests after symlink chain is in place.
   The existing settings.json.tmpl hook paths ($HOME/.edikt/hooks/*.sh)
   MUST resolve through the new symlink chain. Any breakage is a real
   regression in your symlink plumbing, not flaky test setup.

Pitfalls to avoid:
- Do not use `cp -r` where `mv` works. Interrupted copy leaves corruption.
- Do not assume sha256sum exists — macOS ships `shasum -a 256`. Dispatch
  per config-spec.md §7.
- Do not write lock.yaml directly. Staging + atomic rename or you lose
  rollback state on crash.
- Remember: launcher survives payload version flips. It lives in
  $EDIKT_ROOT/bin/, not $EDIKT_ROOT/versions/*/bin/.

When complete, output: LAUNCHER CORE READY
```

---

## Phase 4: M1 Migration (Flat → Versioned) with Staging + Trap + Abort

**Objective:** Implement the launcher's migration flow that detects a pre-v0.5.0 flat layout (`~/.edikt/hooks/` as real dir) and moves it into the versioned layout with dry-run preview, always-prompt + `--yes`, staging dir, signal traps, and `edikt migrate --abort` recovery.
**Model:** `sonnet`
**Max Iterations:** 5
**Completion Promise:** `M1 MIGRATION READY`
**Evaluate:** true
**Dependencies:** Phase 3
**Context Needed:**
- `bin/edikt` (from Phase 3)
- `docs/product/specs/SPEC-004-v050-stability/spec.md` (§2 "migration logic FR-018", §10.M1)
- `docs/product/specs/SPEC-004-v050-stability/config-spec.md` (§8.2 operational concern: mid-migration corruption mitigation)

**Acceptance Criteria:**
- [ ] Launcher adds `migrate` subcommand: `edikt migrate [--dry-run] [--yes] [--abort]`.
- [ ] Detection: triggered automatically on first launcher invocation when `$EDIKT_ROOT/hooks/` is a **directory**, not a symlink, AND `$EDIKT_ROOT/VERSION` exists. Skip cleanly on fresh installs or already-migrated layouts.
- [ ] `--dry-run` prints the exact move plan (source → destination per file) and exits 0 without mutating anything.
- [ ] Without `--yes`: launcher prints the dry-run plan, then asks `Proceed? [y/N]:` via `/dev/tty`. Non-interactive (no TTY) sessions refuse to migrate without `--yes` and exit with instructions.
- [ ] Staging dir: all moves land in `$EDIKT_ROOT/.migrate-staging-<ts>/` first. Only after full staging success does the launcher atomically swap the staging tree into `$EDIKT_ROOT/versions/<current>/`.
- [ ] Signal traps on SIGINT/SIGTERM/EXIT invoke `migrate --abort` cleanup: staging dir removed, `$EDIKT_ROOT/` state identical to pre-migration.
- [ ] `migrate --abort` restores the pre-migration state from `$EDIKT_ROOT/backups/migration-<ts>/` if present. Idempotent — re-running does nothing when no staging or backup exists.
- [ ] Emits `layout_migrated` event to `events.jsonl` per config-spec.md §4.4 on success; `migration_aborted` on abort.
- [ ] Crash recovery test: a test script runs `edikt migrate --yes &`, then `kill -9 $!` mid-migration, then runs `edikt doctor` — doctor must detect the interrupted state and recommend `edikt migrate --abort`. After abort, the layout must be identical to the pre-migration layout byte-for-byte.
- [ ] Idempotent: running `edikt migrate` twice in succession (second call is a no-op, exit 0) produces identical output.
- [ ] Phase 2 hook unit tests still pass post-migration against the new symlink-backed layout.

**Prompt:**
```
You are implementing Phase 4: M1 migration logic.

The hardest thing here isn't the move — it's making sure a SIGKILL
mid-migration never leaves a user's ~/.edikt/ in a corrupt state.

Context to read:
- bin/edikt (Phase 3 output)
- SPEC-004 §2 (migration logic per FR-018), §10.M1
- config-spec.md §8.2 (operational concern: staging dir + trap-based abort)

Implementation:

1. Add `migrate` subcommand to bin/edikt. Flags: --dry-run, --yes, --abort.

2. Detection logic (run this on every launcher invocation):
     if [ -d "$EDIKT_ROOT/hooks" ] && [ ! -L "$EDIKT_ROOT/hooks" ]; then
       # pre-v0.5.0 flat layout detected
       if [ "$1" != "migrate" ] && [ "$1" != "doctor" ]; then
         echo "Layout migration needed. Run: edikt migrate [--dry-run]"
         exit 1
       fi
     fi
   Migrate and doctor must be reachable in the migration-pending state.

3. Dry-run: enumerate every file that would move, print source → dest.
   Never touch disk. Exit 0.

4. Full migration order (staged, atomic):
     a. Record pre-state: tar the flat layout into
        $EDIKT_ROOT/backups/migration-<ts>/pre-migration.tar.gz
     b. Create $EDIKT_ROOT/.migrate-staging-<ts>/
     c. Copy (not move) flat layout into staging/<VERSION>/
        — hooks/, templates/, commands/, VERSION, CHANGELOG.md
     d. Generate staging/<VERSION>/manifest.yaml with SHA256 per file
     e. Atomic swap:
          mv flat dirs to $EDIKT_ROOT/.pre-migration-<ts>/  (backout path)
          mv staging/<VERSION>/ to $EDIKT_ROOT/versions/<VERSION>/
          ln -sfn versions/<VERSION> $EDIKT_ROOT/current
          ln -sfn current/hooks $EDIKT_ROOT/hooks
          ln -sfn current/templates $EDIKT_ROOT/templates
          ln -sfn $EDIKT_ROOT/current/commands/edikt $CLAUDE_HOME/commands/edikt
     f. Write lock.yaml with active=<VERSION>, previous=null
     g. Emit layout_migrated event
     h. Remove $EDIKT_ROOT/.pre-migration-<ts>/ only after everything
        verified OK

5. Signal traps:
     trap 'migrate_abort' INT TERM
     trap 'if [ $? -ne 0 ]; then migrate_abort; fi' EXIT

   migrate_abort function:
     - Remove staging dir if present
     - Move $EDIKT_ROOT/.pre-migration-<ts>/ back to flat paths
     - Remove any partial symlinks
     - Emit migration_aborted event
     - Exit 1

6. `--abort` flag: invoke migrate_abort directly, bypassing the normal
   flow. Idempotent — if nothing to abort, exit 0 with message.

7. Tests in test/unit/launcher/test_migrate_*.sh:
     - test_migrate_dry_run.sh (no mutation)
     - test_migrate_yes.sh (happy path, idempotent on second run)
     - test_migrate_prompt.sh (tty mocking; --yes bypass)
     - test_migrate_crash_recovery.sh — the critical test:
         edikt migrate --yes &
         PID=$!
         sleep 0.05  # let it start
         kill -9 $PID
         # assert pre-migration state restored byte-for-byte
         # via sha256sum comparison against a pre-recorded manifest
     - test_migrate_abort_explicit.sh
     - test_migrate_abort_nothing_to_do.sh

8. Re-run Phase 2 hook unit tests against the migrated layout.
   settings.json references $HOME/.edikt/hooks/ — that's now a symlink
   into versions/<current>/hooks/. Every hook must still execute.

When complete, output: M1 MIGRATION READY
```

---

## Phase 5: install.sh Rewrite — Canonical Cross-Major Upgrade Path

**Objective:** Rewrite `install.sh` as a thin bootstrap that installs the launcher and delegates payload fetch + migration to it. This becomes the authoritative path for v0.4.x → v0.5.0 upgrades. `/edikt:upgrade` (handled in Phase 6) redirects cross-major users to `install.sh`.
**Model:** `sonnet`
**Max Iterations:** 5
**Completion Promise:** `INSTALL REWRITE DONE`
**Evaluate:** true
**Dependencies:** Phase 3, Phase 4
**Context Needed:**
- `install.sh` (current, 453 lines)
- `bin/edikt` (Phase 3)
- Phase 4 migration logic
- `docs/product/specs/SPEC-004-v050-stability/spec.md` (§3 "install.sh rewrite")
- `docs/product/prds/PRD-002-v050-stability-release.md` (FR-008)

**Acceptance Criteria:**
- [ ] `install.sh` accepts `--ref <tag>` (default: latest stable tag via GitHub API). No longer hardcoded to `main`.
- [ ] `install.sh` still accepts `--project`, `--global`, `--dry-run` (backward-compatible).
- [ ] On invocation: detects legacy layout (presence of `~/.edikt/hooks/` as real dir OR `~/.edikt/VERSION` < v0.5.0 in both global and project targets).
- [ ] Fresh install path: downloads launcher to `$EDIKT_ROOT/bin/edikt`, `chmod +x`, invokes `edikt install <tag> --yes && edikt use <tag>`. Adds launcher to `$PATH` if not already present (via `.bashrc`/`.zshrc` append with idempotent marker).
- [ ] Cross-major upgrade path (legacy v0.4.x detected): prints clear banner `"Detected v0.4.x install. Migrating to versioned layout..."`, installs launcher, invokes `edikt migrate --yes` (or `--dry-run` if `--dry-run` flag set), then `edikt install <tag> --yes && edikt use <tag>`.
- [ ] Already-v0.5.0 path: delegates cleanly to `edikt install <tag> && edikt use <tag>`. Never rewrites launcher unless launcher itself is a newer version than installed.
- [ ] Prints a post-install instruction banner: installed version, launcher location, suggested next step (`edikt doctor`).
- [ ] Exits with distinct codes: 0 success, 1 network error, 2 permission error, 3 version mismatch.
- [ ] Re-entrant: re-running `install.sh` with the same args is idempotent.
- [ ] Tests in `test/integration/install/`: test_fresh_install.sh, test_v043_cross_major_upgrade.sh, test_v050_to_v050_noop.sh, test_install_with_ref_flag.sh, test_install_dry_run.sh.
- [ ] EDIKT_EXPERIMENTAL env var NOT required for v0.5.0 stable — remove experimental gating from the rewritten script.

**Prompt:**
```
You are implementing Phase 5: install.sh rewrite.

This is the canonical path for v0.4.x → v0.5.0 cross-major upgrades
(chosen via Option B). /edikt:upgrade from v0.4.3 will redirect users
here. Get this right.

Context to read:
- install.sh (current, 453 lines — study the TTY handling, the
  existing install detection, the dry-run path)
- bin/edikt (Phase 3)
- Phase 4 migration logic
- SPEC-004 §3 — target shape of the rewrite
- PRD-002 FR-008

Implement:

1. Keep backward-compatible flag surface: --project, --global, --dry-run.
   Add: --ref <tag> (default: GitHub API query for latest stable release).

2. State detection (first thing after flag parsing):
   - fresh_install if $EDIKT_ROOT and .claude/commands/edikt both absent
   - legacy_v04 if $EDIKT_ROOT/VERSION exists and < 0.5.0
     (OR $EDIKT_ROOT/hooks/ is a real dir)
   - current_v05 if $EDIKT_ROOT/bin/edikt exists

3. For each state:
   fresh_install:
     - mkdir -p $EDIKT_ROOT/bin
     - curl launcher → $EDIKT_ROOT/bin/edikt
     - chmod +x
     - Add to PATH idempotently (marker comment in shell rc)
     - exec $EDIKT_ROOT/bin/edikt install <tag> --yes
     - exec $EDIKT_ROOT/bin/edikt use <tag>

   legacy_v04:
     - Print banner: "Detected v0.4.x layout. Migrating to v0.5.0..."
     - Same launcher install as fresh_install
     - exec edikt migrate --yes   (--dry-run if --dry-run flag was set)
     - exec edikt install <tag> --yes
     - exec edikt use <tag>
     - Print: "Migration complete. Run `edikt doctor` to verify."

   current_v05:
     - If launcher is outdated: update launcher script only
     - exec edikt install <tag> && edikt use <tag>
     - No migration.

4. Error handling:
     - network failure during launcher download → exit 1 with retry hint
     - $EDIKT_ROOT not writable → exit 2 with chown guidance
     - requested tag < current → exit 3 with rollback command hint

5. Dry-run mode: walk through every state branch, print the sequence
   of commands without executing. Must match non-dry-run behavior
   exactly except for side effects.

6. Integration tests in test/integration/install/:
     - test_fresh_install.sh: empty sandbox → full install → doctor clean
     - test_v043_cross_major_upgrade.sh: seed sandbox with a captured
       v0.4.3 fixture (produced by Phase 7's capture.sh) → run install.sh
       → verify migration happened, launcher present, symlinks OK,
       lock.yaml populated
     - test_v050_to_v050_noop.sh: already on v0.5.0 → install.sh is a
       no-op beyond the tag fetch
     - test_install_with_ref_flag.sh: --ref v0.5.1 installs that version
     - test_install_dry_run.sh: no disk mutation in dry-run

Do not gate with EDIKT_EXPERIMENTAL — v0.5.0 is the stable release.

When complete, output: INSTALL REWRITE DONE
```

---

## Phase 6: Launcher Extended CLI — prune, upgrade, upgrade-pin, dev, Slash-Upgrade Redirect

**Objective:** Complete the launcher CLI surface with prune, upgrade, upgrade-pin, dev link/unlink subcommands. Update the `/edikt:upgrade` slash command to detect major-version jumps and redirect users to `install.sh`.
**Model:** `sonnet`
**Max Iterations:** 5
**Completion Promise:** `LAUNCHER CLI COMPLETE`
**Evaluate:** true
**Dependencies:** Phase 3, Phase 4
**Context Needed:**
- `bin/edikt` (Phases 3 + 4)
- `commands/upgrade.md` (current v0.4.3 content — the redirect target)
- `docs/product/specs/SPEC-004-v050-stability/spec.md` (§2 subcommand table)

**Acceptance Criteria:**
- [ ] `edikt prune [--keep N]`: deletes `$EDIKT_ROOT/versions/*` older than the Nth most recent (default N=3). Never deletes `active` or `previous` from `lock.yaml`, regardless of age.
- [ ] `edikt upgrade [--dry-run] [--yes]`: fetches latest tag from GitHub API, installs if newer, prompts before `use` unless `--yes`. Writes `version_installed` event. Minor-version bumps (v0.5.x → v0.5.y) only — a detected major-version jump prints an error directing to `install.sh`.
- [ ] `edikt upgrade-pin`: updates `.edikt/config.yaml:edikt_version:` to the global active version. Only valid inside a project dir with `.edikt/config.yaml`. Exits 1 elsewhere.
- [ ] `edikt dev link <path>`: creates `$EDIKT_ROOT/versions/dev/` with symlinks into `<path>`, invokes `use dev`.
- [ ] `edikt dev unlink`: removes `$EDIKT_ROOT/versions/dev/`, reverts to the most-recent tagged version via `use`.
- [ ] `/edikt:upgrade` slash command (`commands/upgrade.md`) now detects: if `$EDIKT_ROOT/bin/edikt version` is absent OR `$LAUNCHER_MAJOR > $INSTALLED_MAJOR`, it prints: *"This is a major upgrade. Run `curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash` to complete the upgrade."* and exits without mutating disk.
- [ ] `/edikt:upgrade` for minor jumps: delegates to `edikt upgrade --yes`.
- [ ] Project-pin warn (per SPEC-004 FR-019): every launcher invocation except `version`, `list`, `doctor` walks ancestor dirs for `.edikt/config.yaml`; if found with `edikt_version:` different from `lock.yaml:active`, prints warning to stderr. Does not block.
- [ ] Tests in `test/unit/launcher/`: test_prune_keeps_active_previous.sh, test_upgrade_minor.sh, test_upgrade_major_redirects.sh, test_upgrade_pin_inside_project.sh, test_upgrade_pin_outside_project.sh, test_dev_link.sh, test_dev_unlink.sh, test_pin_warn.sh.

**Prompt:**
```
You are implementing Phase 6: launcher extended CLI + slash-upgrade redirect.

Context to read:
- bin/edikt (Phases 3+4)
- commands/upgrade.md (current content — you will rewrite its logic)
- SPEC-004 §2 subcommand table

Implement each subcommand per the table in SPEC-004 §2. Specific calls
out:

1. prune: protects `active` and `previous` from lock.yaml regardless
   of how old they are. Only prunes beyond the --keep N window.

2. upgrade: minor-version only. If the latest tag is a major jump
   (different X in X.Y.Z), refuse and redirect to install.sh with
   the exact command string.

3. upgrade-pin: updates .edikt/config.yaml edikt_version key.
   Preserves the rest of the file byte-for-byte (use sed/yq with
   surgical edit). If yq is unavailable, use a shell-only YAML edit
   that only touches that one line.

4. dev link: $EDIKT_ROOT/versions/dev/ symlinks into the user's repo:
     dev/VERSION      -> <path>/VERSION
     dev/hooks        -> <path>/templates/hooks
     dev/templates    -> <path>/templates
     dev/commands     -> <path>/commands
   Then: edikt use dev (no migration, no manifest verification —
   dev mode is permissive).

5. dev unlink: remove versions/dev/, use most-recent tagged version
   from versions/ listing (skip `dev`).

6. /edikt:upgrade (commands/upgrade.md) rewrite:
   Replace the v0.4.3 hash-diff flow with:
     a. Read $EDIKT_ROOT/VERSION (or .edikt/VERSION in project mode)
     b. Fetch latest stable tag from GitHub API
     c. If launcher missing OR major version jump → print install.sh
        redirect message with exact command, exit without mutation
     d. Otherwise → delegate: !`edikt upgrade --yes`
     e. Print post-upgrade summary: old → new version, any
        migrations applied, suggested verification (edikt doctor)
   Keep the v0.4.3 three-bucket classification for legacy agents
   that lack provenance frontmatter (fallback path — phase 10 will
   integrate this fully).

7. Project-pin warn: add a pre_invocation_hook in bin/edikt that
   runs before every subcommand (except version/list/doctor):
     PROJECT_CONFIG=$(find_ancestor .edikt/config.yaml)
     if [ -n "$PROJECT_CONFIG" ]; then
       PINNED=$(grep edikt_version: $PROJECT_CONFIG | awk...)
       ACTIVE=$(read lock.yaml active)
       if [ "$PINNED" != "$ACTIVE" ]; then
         echo "⚠ This project pins edikt v$PINNED..." >&2
       fi
     fi

8. Tests per acceptance criteria. Use sandbox from Phase 1.

When complete, output: LAUNCHER CLI COMPLETE
```

---

## Phase 7: Multi-Version Migration M2-M5 + Historical Fixture Capture

**Objective:** Implement migrations M2-M5 (CLAUDE.md sentinels, flat command names, compile schema, config.yaml additions) and build `test/integration/migration/capture.sh` that produces frozen historical-version fixtures (v0.1.0, v0.1.4, v0.2.0, v0.3.0, v0.4.3) by running each tag's `install.sh` inside the Phase 1 sandbox.
**Model:** `opus`
**Max Iterations:** 5
**Completion Promise:** `MULTI VERSION MIGRATION READY`
**Evaluate:** true
**Dependencies:** Phase 1, Phase 4
**Context Needed:**
- `bin/edikt` (Phases 3, 4, 6)
- Migration matrix in `docs/product/specs/SPEC-004-v050-stability/spec.md` (§10)
- `docs/product/specs/SPEC-004-v050-stability/fixtures.yaml` (migration scenarios)
- `ADR-006` (visible sentinels) — the M2 transformation target
- Historical git tags: `v0.1.0`, `v0.1.4`, `v0.2.0`, `v0.3.0`, `v0.4.3` — the capture source

**Acceptance Criteria:**
- [ ] `test/integration/migration/capture.sh` accepts a list of tags, for each tag: `git worktree add` into temp dir, runs that tag's `install.sh` inside the Phase 1 sandbox, snapshots `$EDIKT_ROOT/` + `$CLAUDE_HOME/commands/edikt/` to `test/integration/migration/fixtures/v<tag>/`, sanitizes `$HOME` references (replaces with `${HOME}` placeholder), removes timestamps.
- [ ] Capture script runs on macOS and Linux. Re-runs are deterministic — same tag produces byte-identical fixture output.
- [ ] Fixtures checked in: `test/integration/migration/fixtures/v0.1.0/`, `v0.1.4/`, `v0.2.0/`, `v0.3.0/`, `v0.4.3/`. Each contains `edikt/` (what was in `~/.edikt/`) + `commands/` (what was in `~/.claude/commands/edikt/`) + `manifest.txt` (SHA256 of each file).
- [ ] Launcher implements M2 (CLAUDE.md HTML → markdown link-ref sentinels) for v0.1.0 fixture → detects `<!-- edikt:start -->` / `<!-- edikt:end -->`, rewrites to `[edikt:start]: #` / `[edikt:end]: #` blocks.
- [ ] Launcher implements M3 (flat command names → namespaced) for v0.1.x fixtures → deletes unmodified top-level `*.md` in `~/.claude/commands/edikt/` that have namespaced replacements, preserves any that were user-modified (SHA256 comparison against captured template).
- [ ] Launcher implements M4 (compile schema v1 → v2) for v0.2.x and v0.3.x fixtures → invokes `/edikt:gov:compile` non-destructively to regenerate `.claude/rules/governance.md` with v2 sentinel blocks.
- [ ] Launcher implements M5 (config.yaml additions) for v0.1.x and v0.2.x fixtures → adds missing keys (`paths:`, `stack:`, `gates:`) with defaults. Never removes or renames existing keys.
- [ ] Migration run order enforced: M1 → M2 → M3 → M5 → M4 (compile last because it reads everything). Verified via test that runs each fixture through the full migration and asserts the final layout matches a v0.5.0 reference.
- [ ] `edikt migrate --dry-run` shows the full migration chain that applies to the current install, not just M1.
- [ ] Tests in `test/integration/migration/`: `test_v010_to_v050.py`, `test_v014_to_v050.py`, `test_v020_to_v050.py`, `test_v030_to_v050.py`, `test_v043_to_v050.py`. Each loads its fixture into sandbox, runs launcher install + migrate, asserts final state.
- [ ] M6 (provenance backfill) is explicitly NOT in this phase. Stubbed with a `TODO Phase 11` marker.

**Prompt:**
```
You are implementing Phase 7: multi-version migration M2-M5 + capture.sh.

This is the hardest phase. Five source versions, each with subtle
differences. One capture script that must be reproducible forever.
Opus is assigned.

Context to read:
- bin/edikt (Phases 3-6)
- SPEC-004 §10 migration matrix (each step's action)
- ADR-006 (visible sentinels — M2 target)
- fixtures.yaml migration scenarios (detection signals per version)
- git log --all --oneline for the five tags: v0.1.0 v0.1.4 v0.2.0 v0.3.0 v0.4.3

Part A: capture.sh

1. Script signature: test/integration/migration/capture.sh [tag ...]
2. For each tag:
     git worktree add /tmp/edikt-capture-<tag> <tag>
     cd /tmp/edikt-capture-<tag>
     TEST_SANDBOX=$(mktemp -d); export HOME=$TEST_SANDBOX/home; mkdir -p $HOME
     bash install.sh --global --yes
     # Snapshot
     mkdir -p test/integration/migration/fixtures/<tag>/
     rsync -a $HOME/.edikt/ test/integration/migration/fixtures/<tag>/edikt/
     rsync -a $HOME/.claude/commands/edikt/ test/integration/migration/fixtures/<tag>/commands/
     # Sanitize
     find test/integration/migration/fixtures/<tag>/ -type f -exec \
       sed -i '' "s|$HOME|\${HOME}|g" {} \;
     # Zero timestamps
     find test/integration/migration/fixtures/<tag>/ -exec touch -t 197001010000 {} \;
     # Manifest
     (cd test/integration/migration/fixtures/<tag>/ && \
       find . -type f | sort | xargs sha256sum > manifest.txt)
     git worktree remove /tmp/edikt-capture-<tag>

3. Run the script once, commit the fixtures. Document in a README
   in that dir how to regenerate (re-run capture.sh).

Part B: M2-M5 migrations

4. M2 (CLAUDE.md sentinels): detect files with
     <!-- edikt:start -->
     ...
     <!-- edikt:end -->
   Rewrite to:
     [edikt:start]: #
     ...
     [edikt:end]: #
   Preserve content between markers byte-for-byte. Idempotent — skip
   files already using the new format.

5. M3 (flat commands → namespaced): for each top-level .md in
   ~/.claude/commands/edikt/ that has a namespaced replacement in the
   v0.5.0 payload (e.g., ~/.claude/commands/edikt/plan.md →
   ~/.claude/commands/edikt/sdlc/plan.md):
     stored_sha=$(sha256sum old_file)
     template_sha=$(sha256sum versions/<old_version>/commands/<path>)
     if [ "$stored_sha" == "$template_sha" ]; then
       rm old_file  # unmodified, safe to delete; namespaced version
                    # is already in place via symlink
     else
       mv old_file ~/.edikt/custom/<old_name>.md
       echo "⚠ Preserved user-modified command: <old_name>.md →
         ~/.edikt/custom/" >&2
     fi

6. M4 (compile schema v1 → v2): invoke
     claude -p "/edikt:gov:compile" --bare
   from within the migration step (requires API — only run in
   non-dry-run mode; dry-run prints "would recompile" and moves on).
   Note in migration log if API unavailable; don't fail migration.

7. M5 (config.yaml additions): for each expected v0.5.0 config key
   that's absent, append with a default value. Use a deterministic
   key-merge (never reorder existing keys). Add a comment:
     # Added by edikt v0.5.0 migration
   above new keys.

8. Run order in migrate subcommand:
     M1 (if needed) → M2 (if HTML sentinels detected) →
     M3 (if flat commands detected) → M5 (if missing config keys) →
     M4 (always, to pick up new rule pack updates) →
     M6 stub (TODO Phase 11)

9. Tests in test/integration/migration/:
     test_v010_to_v050.py:
       load fixtures/v0.1.0/ into sandbox
       run install.sh (detects legacy, invokes migrate)
       assert: no HTML sentinels remain
       assert: no top-level .md in commands/edikt/
       assert: config.yaml has paths: section
       assert: .claude/rules/governance.md present, v2 schema
       assert: symlink chain intact, hooks resolve
     (similar for each source version — each tests the migrations
      that actually apply to that version per the §10 signal table)

10. Migration integrity test: run test_v010_to_v050 → compare final
    state against a reference v0.5.0 fresh install → structural
    equivalence (files present, paths resolve, permissions correct).
    User-generated content (plans, ADRs, PRDs) must be untouched.

When complete, output: MULTI VERSION MIGRATION READY
```

---

## Phase 8: Project-Mode Install Parity

**Objective:** Ensure `install.sh --project` produces the same versioned layout under `.edikt/` as global mode does under `~/.edikt/`, using the `EDIKT_ROOT` abstraction already in Phase 3. Rewrite project-mode `settings.json` hook paths to `${PROJECT_ROOT}/.edikt/hooks/*.sh`.
**Model:** `sonnet`
**Max Iterations:** 5
**Completion Promise:** `PROJECT MODE READY`
**Evaluate:** true
**Dependencies:** Phase 3, Phase 4
**Context Needed:**
- `bin/edikt` (uses `EDIKT_ROOT`)
- `install.sh` (Phase 5)
- `templates/settings.json.tmpl`
- `docs/product/specs/SPEC-004-v050-stability/spec.md` (§11 project-mode installs)

**Acceptance Criteria:**
- [ ] `install.sh --project` creates `.edikt/bin/edikt`, `.edikt/versions/<tag>/`, `.edikt/current`, `.edikt/hooks`, `.edikt/templates`, `.claude/commands/edikt`, `.edikt/lock.yaml` under the project root — mirroring global layout.
- [ ] Launcher auto-detects project mode when invoked inside a project whose `$PWD/.edikt/bin/edikt` exists (takes precedence over global `$PATH` launcher).
- [ ] Project-mode `settings.json` hook paths are `${PROJECT_ROOT}/.edikt/hooks/*.sh` (not `$HOME/.edikt/hooks/*.sh`). Launcher rewrites these during install, and `settings.json.tmpl` is parameterized accordingly.
- [ ] Project-mode migration works the same as global: `edikt migrate --dry-run` inside a project dir migrates only that project's `.edikt/`. Global `~/.edikt/` is untouched when operating on a project.
- [ ] No bidirectional sync — global and project are independent. A project on v0.4.2 + global on v0.5.0 is supported (tested in `test_global_v050_project_v042_coexist.py`).
- [ ] `edikt list` inside a project dir shows project versions by default, with a `--global` flag to show global.
- [ ] Tests: `test/integration/project-mode/test_fresh_project_install.py`, `test_project_migrate_v043.py`, `test_project_vs_global_independence.py`, `test_hook_paths_project_relative.py`.

**Prompt:**
```
You are implementing Phase 8: project-mode install parity.

If Phase 3 correctly abstracted EDIKT_ROOT, this phase is a targeted
addition, not a rewrite. If not, you'll feel the pain — consider
fixing Phase 3's abstraction before proceeding.

Context:
- bin/edikt (Phase 3 — EDIKT_ROOT resolution)
- install.sh (Phase 5)
- templates/settings.json.tmpl (hook paths — will be parameterized)
- SPEC-004 §11

Implement:

1. Parameterize templates/settings.json.tmpl:
   Change $HOME/.edikt/hooks/ references to ${EDIKT_HOOK_DIR}
   Launcher substitutes at install time based on mode:
     global: ${EDIKT_HOOK_DIR} = $HOME/.edikt/hooks
     project: ${EDIKT_HOOK_DIR} = ${PROJECT_ROOT}/.edikt/hooks

2. install.sh --project path:
     PROJECT_ROOT=$PWD
     EDIKT_ROOT=$PWD/.edikt
     Invoke edikt install + use as usual
     Then install .claude/settings.json with ${EDIKT_HOOK_DIR} resolved to
     ${PROJECT_ROOT}/.edikt/hooks

3. Launcher auto-detection priority on every invocation:
     a. If $EDIKT_ROOT env set → use it
     b. Else walk ancestors for .edikt/bin/edikt → use that dir's parent
     c. Else default to $HOME/.edikt

4. Project-mode migrate: same flow as global, just scoped to the
   project's .edikt/ subtree.

5. Tests: use the Phase 1 sandbox, create a fake project dir at
   $HOME/my-project/ with .edikt/, verify independence.

When complete, output: PROJECT MODE READY
```

---

## Phase 9: Init Provenance — Path Substitution, Stack Filters, Hash Frontmatter

**Objective:** Extend `/edikt:init` to substitute `paths.*` from config into installed agents, filter language-specific sections via `<!-- edikt:stack:<lang>,... -->` markers, and append `edikt_template_hash` + `edikt_template_version` provenance frontmatter. Hash is content-identity over the raw template (before substitution).
**Model:** `sonnet`
**Max Iterations:** 5
**Completion Promise:** `INIT PROVENANCE READY`
**Evaluate:** true
**Dependencies:** None (independent surface)
**Context Needed:**
- `commands/init.md`
- `templates/agents/*.md` — especially architect, dba, backend, qa, frontend (candidates for stack filtering and path substitution)
- `docs/product/specs/SPEC-004-v050-stability/spec.md` (§5 path substitution, §6 stack filtering, §7 provenance frontmatter)
- `.edikt/config.yaml` schema (paths + stack keys)

**Acceptance Criteria:**
- [ ] `templates/agents/_substitutions.yaml` created per SPEC-004 §5 with entries for `decisions`, `invariants`, `specs`, `prds`, `plans`, `guidelines` (minimum 6 entries).
- [ ] `/edikt:init` reads `paths.*` from the written `.edikt/config.yaml` and substitutes default strings with configured paths in each installed agent. `paths.decisions: adr` produces `adr/` in architect.md, not `docs/architecture/decisions/`.
- [ ] `<!-- edikt:stack:<lang>,<lang> --> … <!-- /edikt:stack -->` markers added to backend.md, qa.md, frontend.md, mobile.md (language-heavy agent templates).
- [ ] `/edikt:init` filter logic: for each stack block, keeps content if `<langs>` intersects the configured stack; deletes entire block (markers + content) if no intersection.
- [ ] Unterminated markers (missing `</edikt:stack>`) log a warning and leave the block verbatim. No silent data loss.
- [ ] Every installed agent carries frontmatter: `edikt_template_hash: <md5 of source template BEFORE substitution>`, `edikt_template_version: "0.5.0"`. Preserved across upgrades (phases 10 + 11).
- [ ] Hash is deterministic and reproducible — hashing the same source template on any machine produces the same hash.
- [ ] Content-identity semantics locked: if source template doesn't change between v0.5.0 and v0.5.1, the hash is unchanged. `edikt_template_version` is written once at install and is NOT bumped by upgrades when hash matches (per Architect #2 decision).
- [ ] Tests in `test/integration/init/`: `test_init_paths_substituted.py`, `test_init_stack_go_only.py`, `test_init_stack_multi.py`, `test_init_unterminated_marker_warns.py`, `test_init_hash_deterministic.py`, `test_init_version_locked.py`.

**Prompt:**
```
You are implementing Phase 9: init provenance.

Context:
- commands/init.md
- templates/agents/*.md (find the language-heavy ones: backend, qa,
  frontend, mobile)
- SPEC-004 §5 (substitution), §6 (stack filtering), §7 (provenance hash)

Implement:

1. Create templates/agents/_substitutions.yaml per SPEC-004 §5.
   Structure is given verbatim in the spec.

2. Update each language-heavy agent template with stack markers.
   Example in backend.md:
     <!-- edikt:stack:go,typescript -->
     ## File Formatting
     - Go: gofmt -w
     - TypeScript: prettier --write
     <!-- /edikt:stack -->

     <!-- edikt:stack:python -->
     ## File Formatting
     - Python: black or ruff format
     <!-- /edikt:stack -->

3. Update commands/init.md to run these steps when installing an agent:
     a. Load _substitutions.yaml
     b. Compute md5 of the source template (raw, untouched — this is
        the "edikt_template_hash")
     c. Apply path substitutions (read .edikt/config.yaml paths.*)
     d. Apply stack filter (read .edikt/config.yaml stack)
     e. Prepend/update YAML frontmatter with:
          edikt_template_hash: <md5>
          edikt_template_version: "0.5.0"
     f. Write the processed file to .claude/agents/<name>.md

4. Stack filter impl: parse markers, drop blocks with no intersection,
   warn on unterminated markers:
     grep -q "<!-- edikt:stack:" <file> && ! grep -q "</edikt:stack -->" <file>
     → warn, skip filtering for this file

5. Hash locking (Architect #2 decision): hash before substitution.
   version is written once at install and never bumped on upgrade-preserved
   files. This means "version answers `when was this installed`, not
   `was this touched by a recent upgrade`."

6. Tests in test/integration/init/. Use the Phase 1 sandbox.

When complete, output: INIT PROVENANCE READY
```

---

## Phase 10: Upgrade Provenance-First Comparison Flow

**Objective:** Rewrite `/edikt:upgrade` agent comparison to use `edikt_template_hash` as the primary anchor. Preserve user customizations when hash matches current template. Show 3-way diff when template moved. Fall back to v0.4.3's diff classifier only for legacy agents without provenance.
**Model:** `opus`
**Max Iterations:** 5
**Completion Promise:** `UPGRADE PROVENANCE READY`
**Evaluate:** true
**Dependencies:** Phase 9
**Context Needed:**
- `commands/upgrade.md` (current v0.4.3 diff classifier)
- Phase 9 output — every installed agent now has provenance frontmatter
- `docs/product/specs/SPEC-004-v050-stability/spec.md` (§8 upgrade provenance-first comparison)
- v0.4.3 fix commit (d81f6e3) — understand the classifier the new flow replaces

**Acceptance Criteria:**
- [ ] `commands/upgrade.md` (agent upgrade section) implements the flow in SPEC-004 §8 verbatim:
  1. Read stored_hash from installed agent frontmatter
  2. If absent → legacy classifier fallback (v0.4.3 behavior preserved)
  3. Compute current_template_hash from current source template
  4. If stored_hash == current_template_hash → preserve installed file entirely (template hasn't moved; any difference is user customization)
  5. Else → re-synthesize what init would have produced with the stored template + current config; compute user_diff against installed; if empty → safe replace; else → 3-way preview + user prompt
- [ ] 3-way preview format: left column = stored template (reconstructed from git history or cached), middle = current template re-synthesized with user config, right = user's installed file with customizations.
- [ ] User prompt options: `[a]pply new template (loses customizations)`, `[k]eep current (misses template updates)`, `[m]erge interactively (opens $EDITOR with conflict markers)`, `[s]kip this agent`.
- [ ] Upgrade writes `upgrade_agent_preserved` / `upgrade_agent_replaced` / `upgrade_agent_conflict_resolved` events to `events.jsonl`.
- [ ] Legacy fallback classifier retained verbatim from v0.4.3 for agents without provenance. Preserves backward-compatible behavior — no silent data loss possible.
- [ ] Tests in `test/integration/upgrade/`:
  - `test_upgrade_preserves_customization_unchanged_template.py` (hash match → no touch)
  - `test_upgrade_3way_template_moved.py` (hash differs → prompt flow)
  - `test_upgrade_legacy_agent_uses_classifier.py` (no provenance → v0.4.3 path)
  - `test_upgrade_replaces_unchanged_user_file.py` (user never touched → safe replace)
- [ ] Each test includes `assert_path_covered("<code path id>")` per Architect #6 — verifies the test actually exercises the intended branch, not a degraded fallback.

**Prompt:**
```
You are implementing Phase 10: upgrade provenance-first flow.

This phase has data-loss risk if wrong. Opus is assigned.

Context:
- commands/upgrade.md (v0.4.3 classifier — read end to end)
- Phase 9 output: installed agents now carry edikt_template_hash
- SPEC-004 §8 — the target flow
- git show d81f6e3 — v0.4.3 classifier commit for reference

Implement:

1. Rewrite the agent comparison block in commands/upgrade.md per
   SPEC-004 §8 step by step. Use exact control flow — don't paraphrase.

2. For 3-way diff preview, use a lightweight shell impl:
     diff3 <(echo "$stored_template") <(echo "$resynth") <(echo "$installed")
   On systems without diff3, print each section separately.

3. User prompt with [a/k/m/s] — match the convention v0.4.3 used.

4. Event logging:
     upgrade_agent_preserved { agent, hash, reason: "template unchanged" }
     upgrade_agent_replaced { agent, hash_old, hash_new, user_accepted }
     upgrade_agent_conflict_resolved { agent, resolution: a|k|m|s }

5. Legacy fallback: when frontmatter lacks edikt_template_hash, call
   the v0.4.3 classifier function verbatim. DO NOT simplify or
   refactor the legacy path — it's exercised by test_legacy_*.py
   and needs to stay byte-compatible.

6. For assert_path_covered: add an event emission at each distinct
   code path (fast_preserve, resynth_safe_replace, threeway_prompt,
   legacy_classifier_entered). Tests grep events.jsonl to verify
   the intended path was reached.

7. Tests in test/integration/upgrade/. Each fixture hits exactly
   one code path by design:
     test_upgrade_preserves_customization_unchanged_template: install
       an agent at hash X, bump edikt version without changing the
       template, upgrade → expect fast_preserve event
     test_upgrade_3way_template_moved: install at hash X, change the
       template → expect threeway_prompt event, simulate user picking
       'k' (keep)
     test_upgrade_legacy_agent_uses_classifier: install without
       provenance frontmatter → expect legacy_classifier_entered event
     test_upgrade_replaces_unchanged_user_file: install + never modify
       + change template → resynth equals installed → expect
       resynth_safe_replace event

When complete, output: UPGRADE PROVENANCE READY
```

---

## Phase 11: M6 Provenance Backfill + `edikt doctor --backfill-provenance`

**Objective:** Implement opt-in provenance backfill for agents installed before v0.5.0 (no `edikt_template_hash` frontmatter). User explicitly invokes `edikt doctor --backfill-provenance`; launcher computes the hash using the template source from the agent's stored `edikt_version` (recoverable from git history for versions we tagged).
**Model:** `sonnet`
**Max Iterations:** 5
**Completion Promise:** `PROVENANCE BACKFILL READY`
**Evaluate:** true
**Dependencies:** Phase 7, Phase 10
**Context Needed:**
- Phase 7's captured fixtures (template state per historical version)
- Phase 9 hash contract (before-substitution)
- Phase 10 legacy fallback (when backfill may be desired)
- `docs/product/specs/SPEC-004-v050-stability/spec.md` (§10.M6)

**Acceptance Criteria:**
- [ ] `edikt doctor --backfill-provenance` scans `.claude/agents/*.md`, identifies files without `edikt_template_hash` in frontmatter.
- [ ] For each candidate: inspects the agent file for any hint at its install version (frontmatter `version:` if v0.4.x wrote one; otherwise prompts user). Uses `test/integration/migration/fixtures/v<detected>/` template as the hash source, computes md5, writes provenance frontmatter.
- [ ] Dry-run mode lists candidates without writing.
- [ ] After backfill, `/edikt:upgrade` treats backfilled agents identically to v0.5.0-installed agents (Phase 10's fast path triggers).
- [ ] Backfill is NEVER automatic — explicit flag required. Documented in `edikt doctor --help` and Phase 14 docs.
- [ ] Edge case: if the user-modified file diverges substantially from every captured template version, backfill warns and skips with a clear reason. Does not guess.
- [ ] Events `provenance_backfilled` + `provenance_backfill_skipped` written per file.
- [ ] Tests: `test/integration/migration/test_m6_backfill_exact_match.py`, `test_m6_backfill_diverged_skips.py`, `test_m6_backfill_unknown_version_prompts.py`.

**Prompt:**
```
You are implementing Phase 11: M6 provenance backfill.

This phase finalizes the provenance story. Backfill is opt-in —
never automatic — because it's inherently a guess (we can only
infer, not know, what template produced a user's installed file).

Context:
- Phase 7 captured fixtures (source templates per historical version)
- Phase 9 hash contract
- Phase 10 fallback path
- SPEC-004 §10.M6

Implement:

1. Add --backfill-provenance flag to edikt doctor.

2. Algorithm per candidate agent:
     a. Parse frontmatter; if edikt_template_hash present → skip
     b. Detect install version hint:
          - frontmatter "version:" key (some v0.4.x agents had this)
          - OR prompt user: "Which edikt version installed <agent>?
            [0.1.0 / 0.1.4 / 0.2.0 / 0.3.0 / 0.4.x / skip]"
     c. Load candidate source template from
        test/integration/migration/fixtures/v<version>/edikt/templates/agents/<name>.md
     d. If source template file absent → can't backfill; skip with reason
     e. Compute md5 of source template
     f. Compare md5 against what user has installed:
          - Near-match (substitutions applied, no user edits): safe,
            write frontmatter
          - Substantial divergence: user-customized; backfill would
            misrepresent state; skip with reason
     g. Log event

3. "Substantial divergence" heuristic: compute Levenshtein distance
   between re-synthesized (source + substitutions) and installed.
   If > 15% of installed file size, skip. Threshold documented.

4. Edge case test: agent template whose source happened to be identical
   across two versions → backfill asks user which to stamp; user picks,
   version field is written.

5. Tests use fixtures from Phase 7. Every test asserts the final
   frontmatter state + the event log entry.

When complete, output: PROVENANCE BACKFILL READY
```

---

## Phase 11b: Hook Fixture Characterization (was 2b.i)

**Objective:** Rewrite `fixtures.yaml` §9.1 and `test/expected/hook-outputs/` to match what the v0.5.0 hooks actually emit, using deterministic sandbox-staged inputs (fixed git history, staged `.edikt/config.yaml`, staged plan files) so fixture diffs are reproducible. Flip `EDIKT_ENABLE_HOOK_JSON_TESTS=1` once tests pass. This turns Phase 2 tests into a **characterization suite** of today's hooks — a regression net, not an aspirational contract.
**Model:** `sonnet`
**Max Iterations:** 5
**Completion Promise:** `HOOK CHARACTERIZATION READY`
**Evaluate:** true
**Dependencies:** Phase 2, Phase 7
**Context Needed:**
- `templates/hooks/*.sh` (the 9 lifecycle hooks — current production behavior is the spec)
- `test/fixtures/hook-payloads/*.json` (existing 21 payloads — adapt or replace)
- `test/expected/hook-outputs/*.expected.json` (existing 21 expected outputs — rewrite to actual emissions)
- `test/unit/hooks/_runner.sh` + `test/unit/hooks/test_*.sh` (test wiring already in place from Phase 2)
- `docs/product/specs/SPEC-004-v050-stability/fixtures.yaml` §9.1 (record-by-record `_note` updates)
- `docs/product/specs/SPEC-004-v050-stability/spec.md` §9.1 (Layer 1 contract — note the inversion: fixtures conform to hooks, not the other way around)
- Phase 7 outputs (M4 rule recompile may have shifted `instructions-loaded.sh` observable state)

**Acceptance Criteria:**
- [ ] For each hook, the fixture payload includes any sandbox staging needed (e.g. a real `test/fixtures/projects/mid-plan/` directory with a staged plan file consumed by `user-prompt-submit.sh`).
- [ ] Expected-output fixtures encode exact strings the v0.5.0 hooks emit, verified by piping the payload to the hook script and comparing.
- [ ] `EDIKT_ENABLE_HOOK_JSON_TESTS=1` is the default in `test/run.sh`. All 9 hook suites pass (no SKIPs).
- [ ] 10/10 consecutive sandbox runs produce identical output for every fixture (no time-dependent text, no path leakage). Time-stamped or path-leaking emissions are normalized to placeholders before diffing.
- [ ] `fixtures.yaml` §9.1 is updated; each record's `_note` field explains why the expected output is what it is (characterization, not prescription).
- [ ] Any fixture pair that cannot be characterized deterministically (e.g. random IDs, model nondeterminism) is removed with a `_note` explaining why, rather than flaking the suite.
- [ ] `test/run.sh` no longer suggests "awaiting Phase 2b" anywhere; the gate is removed and the env var becomes opt-out for local debugging only.

**Prompt:**
```
You are implementing Phase 11b: hook fixture characterization.

The bet from Phase 2: fixtures encode the aspirational JSON-protocol
contract from SPEC-004 §9.1. The hooks emit plaintext today. We chose
to characterize hooks (rewrite fixtures to match production) rather
than rewrite hooks (which 2b.ii defers to v0.6.0). This phase does
the fixture rewrite.

Context to read:
- All 9 templates/hooks/*.sh (the 21 fixtures map to these)
- test/unit/hooks/_runner.sh (the test harness — already correct,
  do not change)
- test/unit/hooks/test_*.sh (one per hook, all currently SKIP-gated
  on EDIKT_ENABLE_HOOK_JSON_TESTS)
- test/fixtures/hook-payloads/*.json (21 inputs)
- test/expected/hook-outputs/*.expected.json (21 outputs to rewrite)
- fixtures.yaml §9.1 (the spec for what each fixture encodes)

Implementation:

1. For each hook, run it once with each existing payload and capture
   the actual stdout. If stdout is empty (silent no-op), the expected
   output should be `{}` per the runner's empty→{} convention.

2. Diagnose any payload that produces nondeterministic output:
   - session-start.sh reads git log → stage a fixed-history fixture
     repo under test/fixtures/projects/git-history/ and have the
     fixture payload point cwd at it
   - subagent-stop.sh writes events.jsonl with timestamps → either
     use jq to strip ts before diffing, or pre-set a clock via
     EDIKT_NOW env var if the hook honors one (add the env var hook
     if it doesn't)
   - any path-leaking output → normalize $TEST_SANDBOX paths to
     <SANDBOX> via a sed pass in the runner before diff

3. Rewrite each *.expected.json to the actual (normalized)
   emission. Use jq -S . to canonicalize key order.

4. If a payload is genuinely impossible to characterize
   deterministically (e.g. relies on a clock the hook doesn't
   parameterize), remove the pair and document in fixtures.yaml's
   _note field why. Do not weaken the runner's diff to "best effort."

5. Update fixtures.yaml §9.1 — each record's _note explains *why*
   the output is what it is. e.g. "subagent-stop emits gate_fired
   event when severity is critical AND .edikt/config.yaml has
   gates.security: critical (config staged in fixture sandbox)".

6. Remove the EDIKT_ENABLE_HOOK_JSON_TESTS gate:
   - test/run.sh: remove any reference to the env var (it's no
     longer needed; tests run by default)
   - test/unit/hooks/test_*.sh: remove the early-exit SKIP block
   - test/unit/hooks/_runner.sh: remove the comment about awaiting
     Phase 2b (now historical)
   - For local debugging convenience, leave the env var as opt-OUT:
     EDIKT_SKIP_HOOK_TESTS=1 → skip. Inverted polarity, default off.

7. Run ./test/run.sh 10 times. All 9 hook suites must PASS, all
   suite outputs must be byte-identical (after normalization).

Pitfalls:
- Do not weaken the runner. If a hook emits nondeterministic output,
  fix the hook to honor a clock env var or remove the fixture —
  never relax the diff to tolerate noise.
- Do not change templates/hooks/*.sh in this phase. Behavior changes
  belong in 2b.ii (v0.6.0). The only exception: adding an EDIKT_NOW
  env var hook if it's strictly to make tests deterministic — and
  even then, prefer normalizing in the runner if possible.
- Re-verify against Phase 7's M4 output. If M4 changed what
  instructions-loaded.sh observes (governance.md content), the
  fixture must use the v0.5.0 governance.md, not pre-migration state.

When complete, output: HOOK CHARACTERIZATION READY
```

---

## Phase 12: Layer 2 Agent SDK Integration Tests + Regression Museum

**Objective:** Stand up `test/integration/` with pytest + `claude-agent-sdk` (Python), fuzzy-match snapshot helper, failure-log persistence for `claude-replay`, retry/backoff + skip-on-outage, and the regression museum (one test per v0.4.0–v0.4.3 bug). Every regression test asserts it covers a specific code path.
**Model:** `sonnet`
**Max Iterations:** 5
**Completion Promise:** `INTEGRATION TESTS READY`
**Evaluate:** true
**Dependencies:** Phase 2, Phase 7, Phase 10
**Context Needed:**
- `docs/product/specs/SPEC-004-v050-stability/spec.md` (§9.2 Layer 2)
- `docs/product/specs/SPEC-004-v050-stability/fixtures.yaml` (integration project fixtures, regression fixtures)
- `docs/product/specs/SPEC-004-v050-stability/test-strategy.md` (Layer 2 + regression museum rows)
- Phases 2, 7, 10 outputs

**Acceptance Criteria:**
- [ ] `test/integration/pyproject.toml` declares `claude-agent-sdk`, `pytest`, `pytest-asyncio`. Exact pinned versions.
- [ ] `test/integration/conftest.py` provides fixtures: `sandbox_home`, `fresh_project`, `project_with_plan`, `project_post_compact`, `project_with_customized_agents`.
- [ ] Fuzzy-match snapshot helper (`assert_tool_sequence`) compares sequences by tool type + path pattern, ignoring exact wording — per SPEC-004 §9.2.
- [ ] Failing test runs persist SDK message streams to `test/integration/failures/<test_name>-<iso_ts>.jsonl` via pytest hook.
- [ ] Retry-with-backoff wraps SDK `query()` calls: 3 attempts, jittered exponential backoff (1s, 2–4s, 4–8s).
- [ ] `pytest --skip-on-outage` flag available: catches Anthropic API 5xx after retries exhaust, marks the test as `skipped` (not failed), writes an event so CI can surface "integration run partial".
- [ ] Regression museum tests in `test/integration/regression/`:
  - `test_v040_silent_overwrite.py` (d81f6e3) — customized agent + template moved → upgrade must not overwrite. Asserts `assert_path_covered("upgrade.threeway_prompt")` or legacy classifier.
  - `test_v042_blank_line_preprocessing.py` (c3df32c) — spec.md with leading blank line in `!` block → preprocessing does not corrupt.
  - `test_v042_preflight_order.py` (8a86c22) — plan command with dirty working tree → pre-flight runs before conclusion step.
  - `test_v043_evaluator_blocked.py` (58ce609) — evaluator invoked under permission sandbox → returns BLOCKED, not silent PASS.
- [ ] Every regression test carries a bold header comment per SPEC-004 §13 (DO NOT DELETE + bug commit + fix commit + invariant preserved).
- [ ] `test/integration/` wired into `test/run.sh` as an opt-in branch (runs only when `SKIP_INTEGRATION != 1`).
- [ ] API key handling: missing `ANTHROPIC_API_KEY` causes the runner to fail loudly (exit 1), not silent-skip. Per SRE #4 and test-strategy.md "no silent skips".

**Prompt:**
```
You are implementing Phase 12: Layer 2 integration tests + regression museum.

Context:
- SPEC-004 §9.2, §13
- fixtures.yaml (integration project scenarios + regression fixtures)
- test-strategy.md (Layer 2 rows, regression museum rows)

Implement:

1. test/integration/pyproject.toml with pinned deps:
     [project]
     dependencies = [
       "claude-agent-sdk==<latest>",
       "pytest>=8.0",
       "pytest-asyncio>=0.23",
     ]

2. test/integration/conftest.py:
   - sandbox_home fixture: creates $HOME override, cleans up on teardown
   - fresh_project fixture: minimal project with .edikt/config.yaml
   - project_with_plan: has an active plan mid-phase
   - project_post_compact: simulates post-compaction state
   - project_with_customized_agents: has one agent with user edits
     and provenance frontmatter
   - failure_logger: pytest_runtest_makereport hook writes SDK
     stream to failures/ on failure

3. Helpers:
   - assert_tool_sequence(tool_calls, snapshot_name): reads
     test/integration/snapshots/<snapshot_name>.json, compares
     by fuzzy match (tool type + path pattern), ignores ordering
     of parallel tool calls
   - with_retry(func, attempts=3): jittered exponential backoff
     around SDK query() calls

4. Core integration tests (one per SPEC-004 §9.2 fixture scenario):
     test_init_greenfield.py — /edikt:init on empty project
     test_plan_phase_execution.py — mid-plan phase advances
     test_post_compact_recovery.py — context restored after compaction
     test_upgrade_preserves_customization.py — Phase 10 flow end-to-end
     test_spec_preprocessing.py — spec command with edge-case files
     test_evaluator_blocked_verdict.py — sandbox restriction → BLOCKED

5. Regression museum in test/integration/regression/. One file per bug
   from v0.4.0-v0.4.3. Each file starts with:

     """
     REGRESSION TEST — DO NOT DELETE.
     Reproduces: <bug description>
     Bug commit: <sha>
     Fix commit: <sha>
     Invariant: <what MUST hold going forward>
     Removing this test reopens the bug.
     """

   And asserts via assert_path_covered that the intended code path
   was exercised, so a refactor that removes the path doesn't cause
   a false pass.

6. API key handling: at test session start, check os.environ["ANTHROPIC_API_KEY"]
   — if missing, pytest.exit("ANTHROPIC_API_KEY required — add secret or
   run ./test/run.sh with SKIP_INTEGRATION=1").

7. --skip-on-outage flag: after retry budget exhausted on 5xx,
   pytest.skip("Upstream outage after 3 retries") + event to
   failures/outages.jsonl.

8. Wire into test/run.sh after the Layer 1 block:
     if [ "${SKIP_INTEGRATION:-0}" != "1" ]; then
       cd test/integration && pytest -v
     fi

When complete, output: INTEGRATION TESTS READY
```

---

## Phase 13: Homebrew Formula + Release Automation

**Objective:** Add `Formula/edikt.rb` to `diktahq/homebrew-tap` alongside existing `verikt.rb`. Create `.github/workflows/release.yml` that on every `v*` tag: builds launcher tarball + full payload tarball, computes SHA256s, dispatches `homebrew-releaser` to bump the formula on a staging branch, waits for tap CI (edikt + verikt smoke tests) to pass, then auto-merges.
**Model:** `sonnet`
**Max Iterations:** 5
**Completion Promise:** `HOMEBREW READY`
**Evaluate:** true
**Dependencies:** Phase 3, Phase 5
**Context Needed:**
- `bin/edikt` (Phase 3 — the launcher artifact the formula ships)
- `install.sh` (Phase 5)
- `docs/product/specs/SPEC-004-v050-stability/spec.md` (§4 Homebrew)
- Existing `diktahq/homebrew-tap` repo (verikt formula for reference)

**Acceptance Criteria:**
- [ ] Release tarball structure: `edikt-v<version>.tar.gz` contains `bin/edikt`, `LICENSE`, `README.md` ONLY. No templates, no commands, no hooks — those are fetched by the launcher on first `edikt install`.
- [ ] `Formula/edikt.rb` (Ruby) installs the launcher into `$HOMEBREW_PREFIX/bin/edikt`, runs `bin/edikt version` as its `test do` block. Matches SPEC-004 §4 skeleton.
- [ ] `.github/workflows/release.yml` triggered on `v*` tag: builds both tarballs (launcher + full payload), uploads as Release assets, computes SHA256.
- [ ] `homebrew-releaser` step opens PR against `diktahq/homebrew-tap` on a staging branch `edikt-bump-<version>`, not directly on main.
- [ ] Tap CI is invoked on the staging branch. Passes only when `brew audit --strict edikt`, `brew audit --strict verikt`, `brew install --HEAD edikt` smoke test, and `brew install --HEAD verikt` smoke test all succeed.
- [ ] Auto-merge is conditional on staging CI passing. PR is merged automatically; on failure, PR stays open for human review.
- [ ] The `homebrew-releaser` action configuration explicitly scopes to `Formula/edikt.rb`. Other formula files are blocklisted.
- [ ] Documentation: add `website/guides/homebrew.md` distinguishing `brew upgrade edikt` (launcher) from `edikt upgrade` (payload) — covered fully in Phase 14, but stub it here so the release announcement links correctly.
- [ ] Tests: `.github/workflows/release.yml` has a `workflow_dispatch` input `--dry-run` that does everything except the final tap PR — verifies the pipeline without mutating the tap. Covered by manual test in Phase 13 acceptance.
- [ ] Verikt formula integrity: post-release, the existing `verikt.rb` is byte-identical to its pre-release content. Enforced by staging-branch diff audit.
- [ ] **Checksum sidecar format decision.** The v0.5.0 launcher (Phase 3) validates network install tarballs via a `<tarball>.sha256` simple-sibling file (single hex hash, or `hash  filename` per `sha256sum` convention). Before wiring the release workflow, confirm this is the desired shape OR upgrade to an aggregated `SHA256SUMS` listing all release artifacts. Signing (GPG / Sigstore / cosign) is an open question — decide here, because both the workflow emitter and `bin/edikt`'s verification logic need matching updates. Record decision as an ADR.

**Prompt:**
```
You are implementing Phase 13: Homebrew formula + release automation.

Critical: the tap is SHARED with verikt. Your release automation
cannot touch verikt.rb, or verikt's next release breaks.

Context:
- bin/edikt (launcher from Phase 3)
- install.sh (Phase 5)
- SPEC-004 §4 — tap layout and formula skeleton
- Existing diktahq/homebrew-tap/Formula/verikt.rb for reference

Implement:

1. Formula/edikt.rb in diktahq/homebrew-tap:
     class Edikt < Formula
       desc "Governance layer for agentic engineering (Claude Code)"
       homepage "https://edikt.dev"
       url "https://github.com/diktahq/edikt/releases/download/v0.5.0/edikt-v0.5.0.tar.gz"
       sha256 "<computed at release time>"
       license "MIT"
       version "0.5.0"

       def install
         bin.install "bin/edikt"
       end

       test do
         assert_match "0.5.0", shell_output("#{bin}/edikt version")
       end
     end

2. .github/workflows/release.yml in the edikt repo:
     on: push: tags: [v*]
     jobs:
       build:
         - Check out the tag
         - Build bin/edikt tarball (launcher only) → edikt-v<ver>.tar.gz
         - Build full payload tarball (templates + commands + hooks)
         - Compute sha256 for both
         - Upload as GitHub Release assets
       bump-tap:
         needs: build
         - Use Justintime50/homebrew-releaser@v2 configured to:
             formula_folder: Formula
             target_file: edikt.rb     # SCOPED strictly to edikt.rb
             update_readme: false
             branch: edikt-bump-v<ver>  # staging branch, NOT main
             pr_title: "bump: edikt v<ver>"
       wait-for-ci:
         needs: bump-tap
         - Poll diktahq/homebrew-tap PR CI status
         - Fail if verikt smoke test regresses
       auto-merge:
         needs: wait-for-ci
         if: success()
         - gh pr merge --squash --delete-branch

3. Smoke test in diktahq/homebrew-tap's existing test.yml:
   Extend to explicitly cover both formulas:
     matrix:
       formula: [edikt, verikt]
     brew audit --strict ${{ matrix.formula }}
     brew install --HEAD ${{ matrix.formula }}

4. Add workflow_dispatch input `dry_run: true` to release.yml —
   runs build step, prints what would be bumped, does not open PR.

5. Dogfood: once merged, run `brew install diktahq/tap/edikt` on a
   clean macOS and verify the launcher works + `edikt install v0.5.0
   --yes` fetches payload successfully.

When complete, output: HOMEBREW READY
```

---

## Phase 14: Docs + Website Refresh + CI Test Workflow + `doctor --report`

**Objective:** Update README, write the three new website guides (upgrade-and-rollback, migrating-from-v0.4, homebrew), refresh existing command + governance pages, add FAQ entries, write structured CHANGELOG entry for v0.5.0, wire `.github/workflows/test.yml` (Layers 1+3 on PR, Layer 2 on tag), add `edikt doctor --report` observability bundle, and the NFS/WSL1 doctor probe + workaround docs.
**Model:** `sonnet`
**Max Iterations:** 5
**Completion Promise:** `DOCS CI READY`
**Evaluate:** true
**Dependencies:** All previous phases
**Context Needed:**
- Everything shipped in Phases 1–13
- `README.md`
- `CHANGELOG.md`
- `website/getting-started.md`, `website/index.md`, `website/faq.md`, `website/commands/**`, `website/governance/**`, `website/guides/**`
- `docs/product/specs/SPEC-004-v050-stability/spec.md` (§12 documentation deliverables)

**Acceptance Criteria:**
- [ ] `README.md` install section rewritten: brew-first primary, `curl | bash` fallback, Windows/WSL note. "Upgrade and rollback" subsection added.
- [ ] `website/guides/upgrade-and-rollback.md` created: covers `edikt upgrade`, `edikt rollback`, `edikt use <tag>`, pinning, "rollback is payload-only — migrations are permanent" (per Architect #3 decision).
- [ ] `website/guides/migrating-from-v0.4.md` created: step-by-step v0.4.x → v0.5.0 walkthrough, example output, troubleshooting.
- [ ] `website/guides/homebrew.md` created: tap install, `brew upgrade edikt` vs `edikt upgrade` distinction.
- [ ] `website/commands/upgrade.md` updated: rollback, provenance-first, 3-way diff, migration.
- [ ] `website/commands/init.md` created (if absent): path substitution, stack filters, provenance frontmatter.
- [ ] `website/commands/doctor.md` created (if absent): launcher-level checks, `--report` bundle, `--backfill-provenance`, NFS/WSL1 probe.
- [ ] `website/commands/sdlc/artifacts.md` updated: artifact enumeration reflects v0.5.0 generated set.
- [ ] `website/governance/sentinels.md` updated: adds `~/.edikt/lock.yaml`, `manifest.yaml`, new events.jsonl event types.
- [ ] `website/governance/features.md` updated: notes any feature flags (none expected at GA).
- [ ] `website/faq.md` adds 4 Q&As: "How do I roll back a bad release?", "Can I pin edikt per project?", "What happened to my old ~/.edikt/hooks/?", "Why did brew upgrade edikt but `edikt upgrade` still says there's an update?".
- [ ] `website/getting-started.md` updated: install walkthrough, pinning subsection, dev-loop for contributors.
- [ ] `website/index.md` updated: install snippet brew-first, reliability callout.
- [ ] `CHANGELOG.md` v0.5.0 entry: structured by bundle (Testing / Versioning / Distribution / Init), Breaking Changes section, Migration Notes linking to `migrating-from-v0.4.md`.
- [ ] Cross-link pass: every new guide linked from at least one command page AND from `getting-started.md`. Enforced by `test/unit/test_docs_crosslinks.sh` (greps for link presence).
- [ ] `edikt doctor --report` command produces a shareable debug bundle: version info, symlink health, manifest integrity, event log tail, system info (OS, shell, filesystem type under `$EDIKT_ROOT`). Writes to `$EDIKT_ROOT/reports/doctor-<ts>.txt`.
- [ ] NFS/WSL1 probe in `edikt doctor`: detects filesystem type under `$EDIKT_ROOT`, warns with documented workaround from config-spec.md §7 if risky fs detected.
- [ ] `.github/workflows/test.yml` created: on PR → Layers 1+3 (unit + sandbox), ~3-minute CI gate, free of API cost. On tag push → Layer 2 (integration), requires `ANTHROPIC_API_KEY` secret.
- [ ] All three CI layers are blocking — tag cannot ship without all passing.
- [ ] Hook unit tests (Phase 11b output) run by default in CI — no `EDIKT_ENABLE_HOOK_JSON_TESTS=1` opt-in flag in the workflow. Confirms Phase 11b's gate removal landed.
- [ ] `test/unit/test_docs_sanity.sh`: greps all docs for outdated install snippets (`raw.githubusercontent.com/...main/install.sh` without version reference should be allowed; explicit v0.4.x hardcoded should fail), stale version references, broken markdown link targets.

**Prompt:**
```
You are implementing Phase 14: docs, CI, doctor --report.

This phase ships the user-facing story. v0.5.0 looks great
technically but the release doesn't land if users can't find
the docs to migrate.

Context:
- SPEC-004 §12 (complete doc deliverable list)
- Everything from Phases 1-13 (what actually ships)
- Existing docs in README.md, website/**, CHANGELOG.md

Implement:

1. README.md install section rewrite:
     ## Install
     ### macOS / Linux (via Homebrew)
     brew install diktahq/tap/edikt
     ### Any platform (via curl)
     curl -fsSL .../install.sh | bash
     ### Upgrading from v0.4.x?
     Re-run the curl command. See migrating-from-v0.4.md

2. Three new website guides. Each < 300 lines, concrete examples,
   no AI slop. Target audience: developers with no prior edikt
   internals knowledge.

3. Existing pages updated per SPEC-004 §12.5 list.

4. 4 new FAQ Q&As per SPEC-004 §12.6.

5. CHANGELOG.md v0.5.0 entry structure:
     ## [0.5.0] — 2026-MM-DD
     ### Testing
       - Layer 1 hook unit tests with JSON stdin fixtures
       - Layer 2 Agent SDK integration tests
       - Layer 3 sandboxed test runner
       - CI gate (Layers 1+3 on PR, Layer 2 on tag)
     ### Versioning & rollback
       - Shell launcher `edikt` with install/use/rollback/prune/doctor
       - Versioned layout at ~/.edikt/versions/<tag>/
       - Multi-version migration from v0.1.0 → v0.5.0
     ### Distribution
       - Homebrew tap: brew install diktahq/tap/edikt
     ### Init provenance
       - Path substitution from .edikt/config.yaml
       - Stack-aware section filtering via <!-- edikt:stack:... --> markers
       - edikt_template_hash frontmatter for clean upgrade detection
     ### Breaking changes
       - ~/.edikt/hooks/ is now a symlink (transparently via migration)
       - ~/.claude/commands/edikt/ is now a symlink
     ### Migration notes
       Coming from v0.4.x? Re-run install.sh — it detects legacy layout
       and migrates. See guides/migrating-from-v0.4.md.

6. Cross-link sanity test:
     test/unit/test_docs_crosslinks.sh
     - Asserts each new guide appears in at least one link from
       website/commands/*.md and one link from website/getting-started.md
     - Asserts all markdown links resolve to existing files or external URLs

7. doctor --report:
     edikt doctor --report
     Outputs to $EDIKT_ROOT/reports/doctor-<iso_ts>.txt:
       - Launcher version, payload version
       - Symlink health for each expected symlink
       - Manifest SHA256 integrity status
       - Last 50 lines of events.jsonl
       - System: uname -a, $SHELL, fs type under $EDIKT_ROOT
     Shareable (no secrets, no tokens) for triage.

8. NFS/WSL1 probe:
     doctor auto-detects filesystem type of $EDIKT_ROOT (stat -f -c %T
     on Linux, mount | grep on macOS). Prints warning + link to guide
     if nfs/9p/refs detected.

9. .github/workflows/test.yml:
     name: test
     on:
       pull_request:
       push: { branches: [main], tags: [v*] }
     jobs:
       unit-sandbox:
         runs-on: ubuntu-latest
         steps:
           - checkout
           - run: ./test/run.sh
             env: { SKIP_INTEGRATION: "1" }
       integration:
         if: startsWith(github.ref, 'refs/tags/v')
         runs-on: ubuntu-latest
         steps:
           - checkout
           - setup-python@v5 with 3.12
           - pip install -e test/integration
           - run: cd test/integration && pytest
             env: { ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }} }

10. All three CI layers block tag release. CODEOWNERS is NOT added
    per locked decision 5.

When complete, output: DOCS CI READY
```

---
