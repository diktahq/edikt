---
type: artifact
artifact_type: test-strategy
spec: SPEC-006
status: accepted
created_at: 2026-04-18T00:00:00Z
reviewed_by: qa
---

# Test Strategy — v0.6.0 SDLC rework, tier-2 Go install, and hook hardening

## Testing tiers

| Tier | Runs when | Authentication | Scope |
|---|---|---|---|
| Unit | Every commit | None | Shell launcher verbs, hook scripts against payloads, pure-Python inline scripts extracted from command .md files |
| Integration | Every commit | None / stubbed | AC-series gates: install/uninstall chain, Python version check, rollback, checksum, fixture lifecycle, events.jsonl, pre-push validation |
| E2E smoke | Every commit | None (EDIKT_TIER2_SKIP_PIP=1) | Full chain from install.sh through tier-2 install, orphan detection, doctor check, CHANGELOG shape |
| Behavioral / model | Gated by EDIKT_RUN_EXPENSIVE=1 | Claude session or ANTHROPIC_API_KEY | Real model + real governance; not in scope for v0.6.0 CI gate |

---

## Unit Tests

### FR-001 / FR-002 — Tier-2 install system (`bin/edikt`)

| Component | What to test | Priority | AC |
|---|---|---|---|
| `install.sh` exclusion | `install.sh` contains no reference to `benchmark`, `gov-benchmark`, `pip install`, or `pipx` — text grep, no subprocess | critical | AC-001 |
| `benchmark.md` tier frontmatter | File starts with `---`, contains `tier: 2` inside the opening frontmatter block | high | AC-010 |
| `pyproject.toml` pin discipline | `claude-agent-sdk==` present; `claude-agent-sdk>=`, `~=`, `*` absent; `pyyaml==` present (runtime dep for sandbox.py) | high | AC-009 |
| Python version probe: missing binary | `EDIKT_TIER2_PYTHON=/nonexistent/path` → stderr contains literal `edikt benchmark requires Python 3.10+`, exit ≠ 0 | critical | AC-004 |
| Python version probe: old version | Stub reporting `3.9` → stderr contains `edikt benchmark requires Python 3.10+` AND `found 3.9 at`, exit ≠ 0 | critical | AC-004 |
| Python version probe fires before filesystem write | No files appear under `$CLAUDE_HOME/commands/edikt/` before the Python check completes | high | AC-004 |
| Checksum mismatch abort | `EDIKT_TIER2_WHEEL=<fake>`, `EDIKT_TIER2_WHEEL_SHA256=000...` → stderr contains `Wheel checksum mismatch`, exit ≠ 0 | critical | AC-008 |
| Checksum match proceeds | Correct SHA-256 + `EDIKT_TIER2_SKIP_PIP=1` → exit 0, no `Wheel checksum mismatch` in stderr | high | AC-008 |
| Release-path wheel without SHA256 rejected | Wheel at `$EDIKT_HOME/current/...` with no `EDIKT_TIER2_WHEEL_SHA256` set → exit ≠ 0, stderr contains `Release install requires EDIKT_TIER2_WHEEL_SHA256` | critical | AC-011 |
| Release-path wheel with SHA256 proceeds | Same wheel, correct SHA-256 set → exit 0 | high | AC-011 |
| Non-release-path wheel without SHA256 allowed | Dev wheel under `/tmp/...` → `Release install requires...` must NOT appear | medium | AC-011 |
| Markdown copied on success | `EDIKT_TIER2_SKIP_PIP=1` → `benchmark.md` at `$CLAUDE_HOME/commands/edikt/gov/benchmark.md`; attack templates at `$CLAUDE_HOME/commands/edikt/templates/attacks/` (refuse_tool_use.md, refuse_file_pattern.md, must_cite.md, refuse_edit_matching_frontmatter.md) | critical | AC-002 |
| Tier-1 byte-equality | SHA-256 hash of every file under `$EDIKT_HOME/versions/0.6.0/commands/` is identical before and after `edikt install benchmark` | critical | AC-003 |
| Rollback on pip failure | `EDIKT_TIER2_WHEEL=/nonexistent/...` (no SKIP_PIP) → exit ≠ 0, `benchmark.md` absent from `$CLAUDE_HOME`, attacks dir absent or empty | critical | AC-005 |
| Venv sentinel on SKIP_PIP | `EDIKT_TIER2_SKIP_PIP=1` → `.pip-skipped` sentinel written under `$EDIKT_HOME/venv/gov-benchmark/`; no venv created | medium | AC-006 |
| Uninstall on empty state | `edikt uninstall benchmark` with nothing installed → exit 0, stdout or stderr contains `already uninstalled` (case-insensitive) | high | AC-007 |
| Uninstall after install | Install then uninstall → exit 0, `benchmark.md` absent | high | AC-007 |
| Uninstall tolerates partial state | Delete `benchmark.md` by hand, then uninstall → exit 0 | high | AC-007 |
| Rollback path guard | Receipt containing path outside `$CLAUDE_ROOT` → file outside `$CLAUDE_ROOT` survives rollback; only legitimate tier-2 artifacts deleted | high | INV-007 |
| Dangling symlink recovery | `$CLAUDE_HOME/commands/edikt` is a dangling symlink to non-existent target → install creates the target dir, benchmark.md resolves, exit 0 | medium | FR-001 |

**Test file:** `test/integration/governance/test_install_tier2.py`
**Env vars exercised:** `EDIKT_TIER2_SKIP_PIP`, `EDIKT_TIER2_PYTHON`, `EDIKT_TIER2_WHEEL`, `EDIKT_TIER2_WHEEL_SHA256`, `EDIKT_TIER2_SOURCE`, `EDIKT_HOME`, `CLAUDE_HOME`

---

### FR-003 — SubagentStop structured evaluator-input contract (ADR-019)

| Component | What to test | Priority | AC |
|---|---|---|---|
| ADR-019 file exists with `status: accepted` | `docs/architecture/decisions/ADR-019-subagent-stop-structured-evaluator-input.md` exists; frontmatter `status: accepted` | critical | AC-016 |
| `subagent-stop.sh` consumes structured payload | Fixture with `evaluator_output.severity` + `evaluator_output.findings[]` → hook does not crash; applies severity threshold from config | critical | AC-017 |
| Legacy unstructured payload fallback | Payload without `evaluator_output` field → hook continues on fallback path with a warning emitted to stderr, does not crash | high | AC-017 |
| Gate fires on FAIL + severity ≥ threshold | `verdict: FAIL`, `severity: critical`, `gates.security: warning` → hook blocks (exit ≠ 0) | high | AC-029 / AC-030 |
| Gate does not fire on PASS | `verdict: PASS` regardless of severity → hook passes through | high | AC-029 |
| Gate output includes threshold line | Block message contains `severity: X ≥ threshold: Y` and one-liner to change it | medium | AC-030 |
| `events.jsonl` gate event written | After block, `events.jsonl` contains a `gate_fired` JSON record with `event`, `agent`, `verdict`, `severity`, `findings_count`, `ts`, `resolved: false` | high | FR-003 / FR-008 |
| Hook JSON emission conformance (INV-003) | All subagent-stop.sh output is `json.loads`-parseable; inputs with embedded `"`, `\`, newlines do not break the JSON | critical | INV-003 |

**Test files:**
- Unit: `test/unit/hooks/test_subagent_stop.sh` (fixture pairs: `subagent-stop-warning`, `subagent-stop-ok`, plus new `subagent-stop-critical` after FR-004)
- Integration: extract inline script from `templates/hooks/subagent-stop.sh` and exercise directly

---

### FR-004 — `_staged_runner.sh` stubs + fixture re-additions

| Component | What to test | Priority | AC |
|---|---|---|---|
| `stub_git_identity()` exported | `_staged_runner.sh` defines and exports `stub_git_identity()`; sourcing it does not crash | critical | AC-020 |
| `provision_memory_fixture()` exported | Defines and exports `provision_memory_fixture()`; sourcing it does not crash | critical | AC-020 |
| `stub_clock()` exported | Defines and exports `stub_clock()`; sourcing it does not crash | critical | AC-020 |
| Existing fixture tests unaffected | `subagent-stop-warning`, `subagent-stop-ok`, `session-start-no-edikt` fixtures all PASS after extension additions | critical | AC-020 |
| `session-start-with-edikt` fixture pair | `test/fixtures/hook-payloads/session-start-with-edikt.json` + `test/expected/hook-outputs/session-start-with-edikt.expected.json` exist; `test_session_start.sh` runs this pair and passes using `stub_git_identity` + `provision_memory_fixture` + `stub_clock` | high | AC-018 |
| `subagent-stop-critical` fixture pair | `test/fixtures/hook-payloads/subagent-stop-critical.json` + `test/expected/hook-outputs/subagent-stop-critical.expected.json` exist; `test_subagent_stop.sh` runs this pair using the new stubs | high | AC-019 |
| `fixtures.yaml` §9.1 deferral notes removed | `_note: defer` lines for both pairs are absent; both records have `status: characterized`, `verified_by`, `verified_at` | high | AC-018 / AC-019 |

**Test file:** `test/unit/hooks/run.sh` (full suite); individual: `test_session_start.sh`, `test_subagent_stop.sh`

---

### FR-005 — Fixture characterization lifecycle (`fixtures.yaml`)

| Component | What to test | Priority | AC |
|---|---|---|---|
| All Phase 11b characterized records backfilled | Every expected-output record in `fixtures.yaml` §9.1 that was previously verified has `status: characterized` | high | AC-021 |
| No `verified_by` without `status: characterized` | No record has `verified_by` present AND `status` missing or `aspirational` | high | AC-021 |
| No `status: characterized` without `verified_by` | The inverse: `status: characterized` requires `verified_by` and `verified_at` | high | AC-021 |
| Aspirational records have `target_phase` or `target_contract` | Every `status: aspirational` record has at least one of these fields | medium | AC-023 |

---

### FR-007 — Shared agent routing layer

| Component | What to test | Priority | AC |
|---|---|---|---|
| `_shared-agent-routing.md` exists | File present at `commands/_shared-agent-routing.md` | high | AC-026 |
| `plan.md` references the shared file | `commands/sdlc/plan.md` contains `edikt:include _shared-agent-routing.md` | high | AC-026 |
| `review.md` references the shared file | `commands/sdlc/review.md` contains `edikt:include _shared-agent-routing.md` | high | AC-026 |
| `drift.md` references the shared file | `commands/sdlc/drift.md` contains `edikt:include _shared-agent-routing.md` | high | AC-026 |
| Domain signal table not duplicated | `plan.md`, `review.md`, `drift.md` do NOT each define their own domain-signal keyword lists | medium | AC-026 |

---

### FR-006 — Deprecated stub removal

| Component | What to test | Priority | AC |
|---|---|---|---|
| `commands/deprecated/` is absent or empty | Directory does not exist or contains no `.md` files after this release | high | AC-025 |
| Removal commit message shape | `git log --oneline` contains a commit with message matching `chore: remove deprecated command stubs` | medium | AC-025 |

---

### FR-010 — Pre-push invariant validation

| Component | What to test | Priority | AC |
|---|---|---|---|
| INV-001 violation detected | Diff containing a `.ts` file in `commands/` → pre-push exits 1, message includes `INV-001` | critical | AC-031 |
| INV-002 violation detected | Diff editing an accepted ADR (frontmatter `status: accepted`) → exit 1, message includes `INV-002` | critical | AC-031 |
| INV-003 violation detected | Diff containing `echo '{"` in a hook script → exit 1, message includes `INV-003` | critical | AC-031 |
| Clean diff passes | Diff with no invariant violations → exit 0 | high | AC-031 |
| Bypass logs to `events.jsonl` | `EDIKT_BYPASS_PREPUSH=1 git push` (simulated) → exit 0 AND `events.jsonl` contains a bypass event with timestamp and file list | high | AC-032 |

---

## Integration Tests

### Tier-2 install isolation (AC-002, AC-003, AC-012)

| Scenario | Components | Priority | AC |
|---|---|---|---|
| `TestTier2InstallIsolation::test_tier2_install_benchmark_markdown_only` | `bin/edikt install benchmark` with `EDIKT_TIER2_SKIP_PIP=1`; asserts `benchmark.md` present, mock tier-1 `context.md` byte-equal | critical | AC-002, AC-003, AC-012 |
| `TestTier2InstallIsolation::test_tier2_uninstall_is_idempotent` | `bin/edikt uninstall benchmark` with nothing installed → exit 0 | high | AC-007, AC-012 |
| Sandbox isolation (INV-007) | Test harness writes minimal `settings.json` into each sandbox; `hooks` key absent; `setting_sources: ["project"]`; no copy of host `~/.claude/settings.json` | critical | INV-007 |
| Tier-1 hash baseline pre/post | `_hash_tree()` over `versions/0.6.0/commands/` identical before and after tier-2 install | critical | AC-003 |

**Test file:** `test/integration/test_e2e_v060_release.py::TestTier2InstallIsolation`

---

### Orphan detection chain (AC-013)

| Scenario | Components | Priority | AC |
|---|---|---|---|
| `TestOrphanDetectionChain::test_orphan_warn_then_block_then_resolve` | Extract orphan-detection Python script from `commands/gov/compile.md §Pass 2`; run with `EDIKT_ORPHAN_IDS`, `EDIKT_HISTORY_PATH`, `EDIKT_VERSION`; first run: exit 0 + `[WARN]` + history written; second run same set: exit ≠ 0 + `[BLOCK]`; change set: exit 0 reset | critical | AC-013 |
| `TestOrphanDetectionChain::test_empty_orphan_set_always_exits_0` | `EDIKT_ORPHAN_IDS=""` → exit 0, no WARN, no BLOCK | high | AC-013 |
| Full five-scenario lifecycle (from `test_compile_orphan_detection.py`) | First detection, consecutive block, subset reset, superset reset, corrupt history — all scenarios covered by existing suite | high | AC-013 |

**Test file:** `test/integration/test_e2e_v060_release.py::TestOrphanDetectionChain` (smoke); `test/integration/test_compile_orphan_detection.py` (full coverage)

---

### Doctor missing ADR source (AC-014)

| Scenario | Components | Priority | AC |
|---|---|---|---|
| `TestDoctorMissingADRSource::test_doctor_fails_for_missing_adr_source` | Extract doctor source-check script from `commands/doctor.md §Routed source files`; fixture project with `governance.md` citing `ADR-999`; script exits ≠ 0, stdout contains `ADR-999` AND literal path `docs/architecture/decisions/ADR-999` | critical | AC-014 |
| Mixed state lists each missing | `architecture.md` cites ADR-001 (present), ADR-777, INV-777 (missing) → exit ≠ 0, both missing IDs named | high | AC-014 |
| Clean state passes | All cited IDs have source files → exit 0, `[ok] Routed sources` in stdout | high | AC-014 |

**Test file:** `test/integration/test_e2e_v060_release.py::TestDoctorMissingADRSource`; `test/integration/test_doctor_source_check.py` (full coverage)

---

### Benchmark preflight without helper (AC-015)

| Scenario | Components | Priority | AC |
|---|---|---|---|
| `TestBenchmarkPreflightNoDirectives::test_benchmark_precheck_exits_cleanly_without_helper` | Install with `EDIKT_TIER2_SKIP_PIP=1` → venv absent, `benchmark.md` present, no `summary.json` written under fixture project `docs/reports/` | high | AC-015 |
| Venv path isolation | Venv would only appear under `$EDIKT_HOME/venv/gov-benchmark/`; no venv-related artifacts outside that path | high | AC-006 |

**Test file:** `test/integration/test_e2e_v060_release.py::TestBenchmarkPreflightNoDirectives`

---

### Baseline artifact shape (SPEC-005 prerequisite gate)

| Scenario | Components | Priority | AC |
|---|---|---|---|
| `TestBaselineArtifact::test_baseline_summary_json_exists` | `docs/reports/governance-benchmark-baseline/summary.json` committed | high | prereq |
| `TestBaselineArtifact::test_baseline_summary_json_is_valid` | Parses as JSON; contains `edikt_version`, `target_model`, `timestamp`, `directive_count`, `overall` | high | prereq |
| `TestBaselineArtifact::test_baseline_summary_has_status_deferred_or_pass_rate` | Either `status: deferred` OR `overall.pass` and `overall.fail` are integers | medium | prereq |
| `TestBaselineArtifact::test_baseline_readme_explains_deferred_state` | If `status: deferred`, `README.md` exists in baseline dir and mentions `backfill` | medium | prereq |

**Test file:** `test/integration/test_e2e_v060_release.py::TestBaselineArtifact`

---

### CHANGELOG shape (AC-011 of SPEC-006 release gates)

| Scenario | Components | Priority | AC |
|---|---|---|---|
| `TestChangelogEntry::test_changelog_has_v060_entry` | `CHANGELOG.md` contains `## v0.6.0` section | critical | release gate |
| `TestChangelogEntry::test_changelog_v060_covers_migration_notes` | v0.6.0 section mentions `FR-003a` or `warn-only`; mentions `v0.7.0`; contains `--backfill` | high | release gate |
| `TestChangelogEntry::test_changelog_v060_documents_known_risks` | v0.6.0 section mentions tier-2 install and/or sandbox parity risks | medium | release gate |

**Test file:** `test/integration/test_e2e_v060_release.py::TestChangelogEntry`

---

### Events.jsonl session memory (FR-008)

| Scenario | Components | Priority | AC |
|---|---|---|---|
| Doctor shows `§ Gate activity` with entries | `events.jsonl` containing `gate_fired` + `resolved: false` within 7 days → doctor output contains `§ Gate activity` section and names the gate | high | AC-027 |
| Doctor skips section when file absent | No `events.jsonl` → section absent, not an error | high | AC-027 |
| Session-start surfaces one unresolved finding | `events.jsonl` with one unresolved finding → session-start hook emits `systemMessage` containing the finding and dismiss instructions | high | AC-028 |
| Session-start emits nothing when all resolved | All `events.jsonl` entries have `resolved: true` → hook emits no `systemMessage` field (not even an empty string) | high | AC-028 |
| Cap at one finding per session start | Multiple unresolved findings → only the most recent is surfaced | medium | AC-028 |
| Bypass event written | `EDIKT_BYPASS_PREPUSH=1` path → `events.jsonl` gains a bypass event with correct schema | high | AC-032 |

---

### Fixture characterization drift axis (FR-005 / FR-006)

| Scenario | Components | Priority | AC |
|---|---|---|---|
| `/edikt:sdlc:drift` output contains characterization section | Running drift against a project with mixed `characterized`/`aspirational` fixtures → output contains `§ Fixture characterization status` with counts | high | AC-022 |
| Aspirational entries show `target_phase` / `target_contract` | Aspirational records render their rationale in drift output | medium | AC-022 |
| Characterization rate warning | >50% aspirational → drift or doctor emits warning | high | AC-024 |
| Doctor with `--deep` runs `verified_by` for stale records | Records >90 days old where `verified_by` output differs → flagged in doctor output | medium | AC-024 |
| `/edikt:sdlc:artifacts` defaults new records to `aspirational` | New records generated by `artifacts` command contain `status: aspirational` | high | AC-023 |
| Aspirational default warning printed | `artifacts` command prints warning about needing to characterize new records | medium | AC-023 |

---

### Gate severity tiers (FR-009, AC-029)

| Scenario | Components | Priority | AC |
|---|---|---|---|
| `.edikt/config.yaml` template has `gates:` section | Config template ships with `security`, `dba`, `sre`, `architect`, `performance`, `api`, `default` keys and values | high | AC-029 |
| Subagent-stop applies per-agent threshold | `gates.dba: critical` → FAIL with `severity: warning` does NOT block; FAIL with `severity: critical` blocks | high | AC-029 |
| Unknown agent falls back to `gates.default` | Agent domain not in config → `gates.default: critical` applies | medium | AC-029 |
| `/edikt:config` documents `gates:` section | `config.md` or equivalent mentions the `gates:` section and shows valid values | medium | AC-029 |

---

## Edge Cases

Derived from spec ACs, invariants, and known failure modes from SPEC-004 / SPEC-005:

- **Tier-2 install into versioned layout**: `$EDIKT_HOME/current` is a symlink to `versions/0.6.0/`; `edikt install benchmark` must create destination dirs through the symlink without failing on `mkdir -p` through a dangling link.
- **Rollback path traversal**: A maliciously crafted receipt file that lists a path outside `$CLAUDE_ROOT` must not cause `tier2_rollback_markdown()` to delete files outside the sandbox boundary (INV-007 boundary enforcement).
- **Checksum with `EDIKT_TIER2_SKIP_PIP=1`**: When `SKIP_PIP=1` is set but a wheel path and checksum are both set, the checksum check should still run and pass before proceeding — skipping pip does not skip verification.
- **`events.jsonl` malformed lines**: One corrupt JSON line among valid lines → doctor skips the corrupt line and processes the rest; does not crash.
- **`events.jsonl` absent entirely**: Neither doctor nor session-start hook crashes when the file does not exist; sections are simply omitted.
- **Gate severity fallback with no `gates:` in config**: If `.edikt/config.yaml` has no `gates:` key, `subagent-stop.sh` must not crash — it falls back to a hardcoded default of `critical` and logs a warning.
- **Pre-push hook with `EDIKT_BYPASS_PREPUSH=1` and no `events.jsonl` directory**: Bypass event write must create the directory if it does not exist; must not fail silently.
- **`_staged_runner.sh` on CI without `libfaketime`**: `stub_clock()` must degrade gracefully when `libfaketime` is unavailable — fall back to `EDIKT_TEST_FIXED_TIMESTAMP` env var and emit a notice, not a hard failure.
- **Fixture pair output that contains Unicode or control characters**: `session-start-with-edikt` output may contain the edikt banner with unicode symbols; `jq -S` normalization in `_runner.sh` must handle this without `diff` false-failures.
- **Concurrent `edikt install benchmark` runs**: Two simultaneous installs into the same sandbox — last writer wins; neither must leave a half-written receipt or venv.
- **Pre-push hook on binary files in `commands/`**: Only text files should be scanned; binary blobs in `commands/` (if any) must not cause the grep-based check to crash or produce false positives.
- **Deprecated stub redirect after removal (AC-025)**: If a user's `~/.claude/commands/edikt/deprecated/` still has old stubs from a prior version, `/edikt:upgrade` warning fires on the first run from < 0.6.0 but does not block the upgrade.

---

## Coverage Target

| Tier | Target | Definition of green |
|---|---|---|
| Unit (`test/unit/hooks/run.sh`) | All hook fixture pairs PASS | Every existing + two new fixture pairs (`session-start-with-edikt`, `subagent-stop-critical`) PASS without `EDIKT_SKIP_HOOK_TESTS=1` |
| Regression (`pytest test/integration/regression/`) | 100% green | No SDK auth required; must pass on every commit |
| Integration — install suite | 14 tests green | All 14 tests in `test/integration/governance/test_install_tier2.py` pass (12 original AC-023 tests + 2 additional hardening tests) |
| Integration — e2e smoke | 13 test classes green | All classes in `test/integration/test_e2e_v060_release.py` pass |
| Integration — full suite | No regressions | `pytest test/integration/` passes at the same rate as v0.5.0 baseline; no previously-green test regresses |
| AC coverage | AC-001 through AC-032 | Every AC in `spec.md` has at least one test asserting its pass condition; no AC is covered only by "we believe it works" |

---

## Test ladder — which tests gate which phases

The spec defines 11 phases in four waves. The following table maps tests to phase gates so that each wave's exit criterion is explicit.

### Wave 1 gate (Phase 1 exit)

Phase 1 ships `bin/edikt install/uninstall benchmark` shell verbs and ADR-019.

| Gate | Test | Must pass before Wave 2 starts |
|---|---|---|
| install.sh exclusion | `test_install_sh_does_not_install_benchmark` | yes |
| Markdown copied | `test_install_benchmark_adds_markdown_and_venv` | yes |
| Tier-1 unchanged | `test_install_benchmark_leaves_tier1_unchanged` | yes |
| Python check literal message | `test_python_version_check_uses_literal_message` | yes |
| Python check rejects old | `test_python_version_check_rejects_old_python` | yes |
| Checksum mismatch aborts | `test_wheel_checksum_mismatch_aborts` | yes |
| Checksum match proceeds | `test_wheel_checksum_match_proceeds` | yes |
| Rollback on pip failure | `test_pip_failure_rolls_back_markdown` | yes |
| Uninstall empty state | `test_uninstall_on_empty_state_exits_zero` | yes |
| Uninstall after install | `test_uninstall_after_install_exits_zero` | yes |
| ADR-019 file exists + accepted | File presence + frontmatter check | yes |

### Wave 2 gates (Phases 3, 4, 5, 7, 8, 10 exit)

These phases are independent given Wave 1. Each phase below has its own exit gate; all must be green before Wave 3 begins.

| Phase | Gate | Test |
|---|---|---|
| Phase 3 (subagent-stop structured payload) | Hook consumes `evaluator_output` field; legacy fallback does not crash; INV-003 parseable output | `test_subagent_stop.sh` fixtures; new integration test against structured payload |
| Phase 4 (staged_runner stubs + fixture re-additions) | `stub_git_identity`, `provision_memory_fixture`, `stub_clock` exported; both new fixture pairs PASS; existing fixtures unaffected | `test/unit/hooks/run.sh` full suite |
| Phase 5 (fixtures.yaml backfill) | All Phase 11b records have `status: characterized`; no orphaned `verified_by` | Schema validation in `test/integration/governance/test_benchmark_sandbox_parity.py` or equivalent |
| Phase 7 (shared routing + deprecated removal) | `_shared-agent-routing.md` exists; `plan.md`, `review.md`, `drift.md` each reference it; `commands/deprecated/` empty/absent | File existence + grep checks in integration |
| Phase 8 (events.jsonl doctor + session-start) | Doctor shows `§ Gate activity`; session-start surfaces one finding; clean file → section absent | New integration tests against `events.jsonl` fixtures |
| Phase 10 (pre-push invariant validation) | INV-001, INV-002, INV-003 violations each caught by pre-push; bypass writes event | Unit-level diff parser tests; end-to-end `git push` simulation |

### Wave 3 gates (Phases 2, 6, 9 exit)

| Phase | Gate | Test |
|---|---|---|
| Phase 2 (Go binary) | `make test` in `tools/gov-benchmark/` passes Go unit tests; binary handles all exit codes (0, 1, 2, 3, 5) | `tools/gov-benchmark/Makefile test` |
| Phase 6 (artifacts aspirational default + doctor checks) | `/edikt:sdlc:artifacts` generates `status: aspirational`; `/edikt:doctor` reports characterization rate | `test_shared_directive_checks_drift.py` pattern; doctor script extraction + run |
| Phase 9 (configurable gate severity) | Config template has `gates:` section; `subagent-stop.sh` applies per-agent threshold; unknown agent uses `default` | Gate threshold integration tests against structured payload fixtures |

### Wave 4 gate (Phase 11 exit — release)

All of the following must be green to ship v0.6.0:

| Gate | Test |
|---|---|
| All 14 tests in `test_install_tier2.py` green | `pytest test/integration/governance/test_install_tier2.py` |
| All 13 test classes in `test_e2e_v060_release.py` green | `pytest test/integration/test_e2e_v060_release.py` |
| Hook unit suite fully green | `bash test/unit/hooks/run.sh` (no EDIKT_SKIP_HOOK_TESTS=1) |
| Full integration suite no regressions | `pytest test/integration/` passes at v0.5.0 baseline rate |
| CHANGELOG has `## v0.6.0` with migration notes | `TestChangelogEntry` suite |
| AC-001 through AC-032 all covered | Triage pass against this document before shipping |

---

## Things that are hard to test

- **Go binary exit-code parity with shell fallback.** The Go binary (`tools/gov-benchmark/cmd/install.go`) must produce identical exit codes to the inline shell logic for the same inputs. The shell path is tested by the Python integration suite; the Go path requires `make test` and a cross-compilation check. These run in different test environments and are not directly comparable in CI without a matrix build.
- **`libfaketime` availability on CI.** `stub_clock()` in `_staged_runner.sh` relies on `libfaketime` for deterministic timestamps in the `subagent-stop-critical` fixture. macOS CI typically lacks `libfaketime`; the fallback (`EDIKT_TEST_FIXED_TIMESTAMP`) must be exercised instead. This means the characterized output for `subagent-stop-critical.expected.json` must be regenerated on the target CI environment, not locally.
- **`events.jsonl` race conditions.** Concurrent gate firings from parallel subagents writing to `events.jsonl` are hard to reproduce deterministically. The spec says append-only, one JSON object per line — this is asserted by schema checks on the output, not by a concurrency stress test.
- **Pre-push hook against real git diff.** The pre-push invariant validation runs against the actual diff context when invoked via `git push`. Integration tests simulate the diff by passing crafted patch text; they may miss edge cases from git's binary diff output or large diffs with encoding issues.
- **`/edikt:sdlc:artifacts` and `/edikt:sdlc:drift` as Claude commands.** These are markdown files interpreted by Claude, not executable scripts. The `status: aspirational` default and `§ Fixture characterization status` section tests rely on extracting inline Python/shell scripts from the command files (the same pattern as `test_compile_orphan_detection.py` and `test_doctor_source_check.py`). If those sections are written as prose rather than inline scripts, they cannot be unit-tested without a live Claude session.

---

## Fixture catalog

See `docs/product/specs/SPEC-005-directive-hardening-and-gov-benchmark/fixtures.yaml` for the fixture definitions that carry forward into SPEC-006 with the `status:` field extension.

New fixture pairs added in SPEC-006:

| Fixture name | Hook | Status after Phase 4 |
|---|---|---|
| `session-start-with-edikt` | `session-start.sh` | `characterized` — verified by running hook against `session-start-with-edikt.json` with `stub_git_identity`, `provision_memory_fixture`, `stub_clock` |
| `subagent-stop-critical` | `subagent-stop.sh` | `characterized` — verified by running hook against `subagent-stop-critical.json` with `stub_git_identity`, `stub_clock`; requires `status: accepted` ADR-019 and structured payload in hook |

New event fixtures for FR-008:

| Fixture | Purpose |
|---|---|
| `events-unresolved-critical.jsonl` | One `gate_fired` entry, `resolved: false`, `severity: critical`, within 7 days — used by doctor and session-start tests |
| `events-all-resolved.jsonl` | All entries have `resolved: true` — session-start must emit no `systemMessage` |
| `events-bypass.jsonl` | One `pre_push_bypass` entry — doctor bypass frequency test |
