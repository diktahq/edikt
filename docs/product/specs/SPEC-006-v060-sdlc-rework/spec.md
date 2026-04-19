---
type: spec
id: SPEC-006
title: v0.6.0 — SDLC rework, tier-2 Go install, and hook hardening
status: accepted
author: Daniel Gomes
created_at: 2026-04-18T00:00:00Z
references:
  adrs: [ADR-001, ADR-010, ADR-014, ADR-015, ADR-018]
  invariants: [INV-001, INV-002, INV-007]
  specs: [SPEC-004, SPEC-005]
new_adrs:
  - ADR-023 (to write in Phase 1): "SubagentStop structured evaluator-input contract"
baseline:
  v050_shipped:
    - SPEC-004 stability release (three-layer testing, versioned payload, Homebrew)
    - SPEC-005 directive hardening + gov benchmark (all 10 phases done 2026-04-17)
    - ADR-014 hook JSON-wrapping migration
    - ADR-015 tier-2 tooling carve-out
    - ADR-018 evaluator verdict schema
    - INV-003 through INV-008
---

# SPEC-006: v0.6.0 — SDLC rework, tier-2 Go install, and hook hardening

**Date:** 2026-04-18
**Author:** Daniel Gomes

---

## Summary

v0.6.0 is the first feature release after the v0.5.0 stability sprint. It ships six
grouped workstreams:

1. **Tier-2 install system** — `edikt install benchmark` / `edikt uninstall benchmark`
   verbs in `bin/edikt` backed by a new Go binary at
   `tools/gov-benchmark/cmd/install.go`. Covers Python prereq check, venv isolation,
   checksum verification, rollback on failure, and idempotent uninstall. Satisfies the
   12 pre-written AC-023 tests in `test/integration/test_install_tier2.py` and the
   13 e2e smoke gates in `test/integration/test_e2e_v060_release.py`.

2. **Hook hardening** — ADR-023 formalizes the `subagent-stop` structured evaluator-input
   contract (`{verdict, evaluator_output: {severity, findings[]}}`). `subagent-stop.sh`
   consumes the structured payload. Two deferred fixture pairs from ADR-014
   (`session-start-with-edikt`, `subagent-stop-critical`) are re-added with
   `_staged_runner.sh` extensions for determinism.

3. **Fixture characterization lifecycle** — `fixtures.yaml` gains `status:
   characterized | aspirational` on each expected-output record.
   `/edikt:sdlc:drift` surfaces a characterization-rate section.
   `/edikt:sdlc:artifacts` defaults new records to `aspirational`.
   `/edikt:doctor` flags stale characterizations.

4. **SDLC command hardening** — Shared agent routing template extracted from
   `plan.md`, `review.md`, `drift.md`; deprecated command stubs (16) removed from
   `commands/deprecated/`; configurable gate severity tiers added to
   `.edikt/config.yaml`; pre-push hook extended to validate staged files against
   installed invariants.

5. **Events.jsonl session-crossing memory** — `/edikt:doctor` reads `events.jsonl`
   and surfaces unresolved gate findings; session-start hook surfaces unresolved
   findings from the prior session.

6. **Release infrastructure** — CHANGELOG v0.6.0 entry, ROADMAP updated to mark
   v0.6.0 items shipped.

---

## Context

v0.5.0 shipped 2026-04-17. The 12 `test_install_tier2.py` tests and the 13
`test_e2e_v060_release.py` tests were written as v0.6.0 acceptance gates and are
currently failing (expected). This spec implements them.

The v0.5.0 benchmark run (SPEC-005 Phase 10, `docs/reports/governance-benchmark-baseline/`)
is the baseline. v0.6.0 does not repeat the benchmark — it hardens the infrastructure
that runs it and the hooks it measures.

**What SPEC-005 deferred to v0.6.0 (per ADR-014 + ROADMAP):**
- `subagent-stop.sh` structured evaluator-input contract (ADR-023 not yet written)
- `session-start-with-edikt` fixture pair (requires `_staged_runner.sh` git-history stub)
- `subagent-stop-critical` fixture pair (requires deterministic env vars for clock/identity)
- `stop-hook.sh` command name + emoji cosmetic alignment

**What the ROADMAP adds for v0.6.0:**
- §2.8 Shared agent routing layer
- §2.9 Events.jsonl session-crossing memory
- §2.11 Configurable gate severity tiers
- §2.12 Pre-push invariant validation
- §4.4 Deprecated command stub removal
- §5.1 + §5.1a Hook semantic rewrites + fixture re-additions
- §5.2 Fixture status field + drift axis

---

## Non-Goals (v0.6.0)

- Multi-platform support (Codex adapter, §2.5) — no ADR-001 supersession yet.
- Semantic stop hook (Haiku agent replacing regex, §3.1).
- PRD/SPEC hash-sync extension (§3.8 BRAIN-001 follow-up).
- Cross-agent synthesis (§2.10).
- `--fail-below` CI gate on `/edikt:gov:benchmark`.
- Go rewrite of `bin/edikt` shell launcher (launcher stays shell; Go backs the install
  subcommand only).

---

## Architecture

### FR-001 — Tier-2 install system (`edikt install / uninstall benchmark`)

The `bin/edikt` shell launcher gains two new top-level verbs:

```
edikt install <tool>     — install a tier-2 tool
edikt uninstall <tool>   — remove a tier-2 tool
```

For `tool = benchmark`, the install sequence (in order, with rollback on any failure):

1. **Python prereq check** — verify `$EDIKT_TIER2_PYTHON` (default: `python3`) reports
   version ≥ 3.10. Failure message MUST be exactly:
   `edikt benchmark requires Python 3.10+; found X.Y at <path>` on stderr, exit ≠ 0.
   This check MUST run BEFORE any filesystem writes.

2. **Checksum verification** — if `$EDIKT_TIER2_WHEEL` is set, compute SHA-256 of the
   wheel file. Compare against `$EDIKT_TIER2_WHEEL_SHA256`. Mismatch MUST print
   `Wheel checksum mismatch` on stderr and exit ≠ 0. If `$EDIKT_TIER2_WHEEL_SHA256`
   is absent AND the wheel path includes `/current/` or resembles a release path,
   also abort with a clear error.

3. **Copy markdown** — copy `benchmark.md` to
   `$CLAUDE_HOME/commands/edikt/gov/benchmark.md` and all 10 attack templates from
   `tools/gov-benchmark/templates/attacks/` to
   `$CLAUDE_HOME/commands/edikt/templates/attacks/`. Track copied paths for rollback.

4. **Create venv** — `$EDIKT_TIER2_PYTHON -m venv $EDIKT_HOME/venv/gov-benchmark`.
   If `$EDIKT_TIER2_SKIP_PIP=1` is set, skip steps 4–5 and write a sentinel file
   `$EDIKT_HOME/venv/gov-benchmark/.pip-skipped` instead.

5. **Pip install** — install from `$EDIKT_TIER2_WHEEL` (if set) or from
   `$EDIKT_TIER2_SOURCE/pyproject.toml`. Dependencies MUST be pinned with `==`.

6. **On any failure in steps 3–5** — delete all files copied in step 3, remove the
   venv directory, and exit ≠ 0 with a diagnostic message.

**Invariants (from ADR-015):**
- `install.sh` MUST NOT reference `benchmark`, `gov-benchmark`, or any `pip install`.
- Tier-1 commands (`~/.edikt/current/commands/`) MUST be byte-equal before and after
  tier-2 install.
- `edikt uninstall benchmark` MUST be idempotent: exit 0 on already-uninstalled state,
  print a human-readable message, remove both markdown files and the venv.
- `benchmark.md` MUST declare `tier: 2` in its YAML frontmatter.

**Source of truth for test behavior:** `test/integration/test_install_tier2.py` (12 tests).
The spec follows the tests; the tests do not follow the spec.

### FR-002 — Go binary backing `edikt install benchmark`

A Go binary at `tools/gov-benchmark/cmd/install.go` implements the install/uninstall
logic from FR-001. `bin/edikt` delegates to the compiled binary when available at
`$EDIKT_HOME/bin/gov-install`, falling back to inline shell logic for dev mode (when
`$EDIKT_TIER2_SOURCE` is set to the repo's `tools/gov-benchmark/`).

The Go binary:
- Reads all configuration from environment variables (matching the test harness:
  `EDIKT_TIER2_PYTHON`, `EDIKT_TIER2_WHEEL`, `EDIKT_TIER2_WHEEL_SHA256`,
  `EDIKT_TIER2_SKIP_PIP`, `EDIKT_TIER2_SOURCE`, `EDIKT_HOME`, `CLAUDE_HOME`).
- Shares exit-code semantics with `tools/edikt/cmd/install.go`:
  `0 = success`, `1 = prereq/network`, `2 = checksum`, `3 = already installed`,
  `5 = path traversal / malicious`.
- Is compiled into `~/.edikt/bin/gov-install` at `edikt install --compile-tools` time
  (a new sub-option, run once after `brew install edikt` or after updating the SDK).
- Is NOT required for `EDIKT_TIER2_SKIP_PIP=1` dev runs; the shell launcher handles
  those directly.

**Build target:** `tools/gov-benchmark/Makefile` with `make install-binary` that builds
and places the binary. `make test` runs the Go unit tests.

### FR-003 — SubagentStop structured evaluator-input contract (ADR-023)

ADR-023 defines the structured payload that `subagent-stop.sh` reads from its hook
input and the structured evaluator-output that evaluator agents MUST emit:

**Evaluator output schema** (what evaluator agents write to stdout):
```json
{
  "verdict": "PASS" | "FAIL" | "BLOCKED",
  "evaluator_output": {
    "severity": "critical" | "warning" | "info",
    "findings": [
      {
        "criterion_id": "C1-pytest-suite",
        "status": "pass" | "fail" | "blocked",
        "evidence_type": "test_run" | "observation" | "none",
        "detail": "pytest exited 0 in 4.2s"
      }
    ]
  }
}
```

**Hook behavior (subagent-stop.sh) after ADR-023:**
- Parse the `evaluator_output` field from the hook input JSON (PostToolUse payload).
- Extract `severity` and `findings[]`. Apply the gate severity threshold from
  `.edikt/config.yaml` (see FR-009).
- Block only when `verdict == "FAIL"` AND `severity` meets or exceeds the configured
  threshold for the agent domain.
- Write a structured event to `events.jsonl`:
  ```json
  {"event": "gate_fired", "agent": "evaluator", "verdict": "FAIL",
   "severity": "critical", "findings_count": 2, "ts": "..."}
  ```

**Acceptance:** ADR-023 written and accepted before `subagent-stop.sh` rewrite lands.
Each change ships with its fixture update in the same commit.

### FR-004 — Fixture re-additions (session-start-with-edikt, subagent-stop-critical)

**Blocker resolved:** Extend `test/unit/hooks/_staged_runner.sh` with:
- `stub_git_identity()` — sets `GIT_AUTHOR_NAME`, `GIT_AUTHOR_EMAIL`,
  `GIT_COMMITTER_NAME`, `GIT_COMMITTER_EMAIL` to static test values.
- `provision_memory_fixture()` — writes a minimal `~/.claude/MEMORY.md` with
  deterministic content (no date-dependent fields).
- `stub_clock()` — sets `FAKETIME` or uses the `libfaketime` wrapper when available;
  falls back to `EDIKT_TEST_FIXED_TIMESTAMP`.

With these helpers, both fixture pairs can be characterized:
- `session-start-with-edikt.expected.json` — JSON `additionalContext` containing the
  edikt project context banner.
- `subagent-stop-critical.expected.json` — JSON `systemMessage` with blocking-path
  output including static git identity.

Both records ship with `status: characterized`, `verified_by`, and `verified_at`.
Both `_note: defer` lines in `fixtures.yaml` §9.1 are removed.

### FR-005 — Fixture characterization lifecycle

`fixtures.yaml` gains new optional fields on each expected-output record:

| Field | Type | Required when | Values |
|-------|------|---------------|--------|
| `status` | string | recommended | `characterized` \| `aspirational` |
| `verified_by` | string | `status=characterized` | shell command that produced the output |
| `verified_at` | date | `status=characterized` | ISO date of last verification |
| `target_phase` | string | `status=aspirational` | roadmap reference |
| `target_contract` | string | `status=aspirational` | one-line description of desired behavior |

**Backfill:** all Phase 11b characterized records in `fixtures.yaml` §9.1 receive
`status: characterized` + `verified_by` + `verified_at` in a single backfill commit.

**`/edikt:sdlc:drift`** gains a `§ Fixture characterization status` section:
```
§ Fixture characterization status

  Characterized (N):
    <filename>  ✓ verified <date>
    ...

  Aspirational (M):
    <filename>
      target: <target_phase>
      contract: <target_contract>
    ...

  Aspirational debt: M/N+M (pct%)
```
Aspirational entries are warnings, not errors.

**`/edikt:sdlc:artifacts`** generates new fixture records with `status: aspirational`
by default and prints:
```
⚠ New fixture records default to status: aspirational. Run the verified_by
  command after the hook ships and update status to characterized.
```

**`/edikt:doctor`** adds two checks:
1. Characterization rate: if >50% aspirational → warning.
2. Stale characterization (opt-in, `--deep`): runs `verified_by` and diffs output vs
   `content`; flags if >90 days old AND output differs.

### FR-006 — Deprecated command stub removal

Remove all 16 deprecated stubs from `commands/deprecated/`. These are v0.1.x/v0.2.x
flat-name aliases that redirect to namespaced commands (e.g. `compile.md` → uses
`edikt:gov:compile`).

**Prerequisite:** v0.6.0 install guide notes the removal and provides a migration table.
`/edikt:upgrade` displays a one-time warning when upgrading from < 0.6.0 if any deprecated
stubs were customized.

### FR-007 — Shared agent routing layer

Domain signal detection and agent-spawning logic is extracted from `plan.md`,
`review.md`, and `drift.md` into `commands/_shared-agent-routing.md`.

The shared file defines:
- The canonical domain-signal table (database, infrastructure, security, API,
  architecture, performance) with keyword lists.
- The `spawn_specialists(signals, plan_text)` procedure used by all three orchestrating
  commands.
- The `consolidate_findings(specialist_outputs[])` step that runs after specialists
  complete (pre-cursor to §2.10 cross-agent synthesis, which ships in a later release).

Each of `plan.md`, `review.md`, `drift.md` includes the shared file with:
```
<!-- edikt:include _shared-agent-routing.md -->
```

### FR-008 — Events.jsonl session-crossing memory

**Doctor gate awareness:**
`/edikt:doctor` reads `.edikt/events.jsonl` (if present) and reports:
- Unresolved gate findings from the last 7 days (gate fired but no subsequent
  `gate_resolved` event).
- Frequency pattern: "Security gate fired N times this week."
- Override frequency: "Pre-push hook bypassed N times in last 30 days."

Output section in doctor:
```
§ Gate activity (last 7 days)
  Unresolved: 2
    2026-04-17: security gate (critical) — no resolution recorded
    2026-04-16: dba gate (warning) — no resolution recorded
  Overrides: 1
    2026-04-15: pre-push bypass
```

**Session-start surfacing:**
The session-start hook (or PostCompact hook) reads `events.jsonl` and surfaces the
most recent unresolved finding as a `systemMessage`:
```
⚠ Last session: security gate fired on a hardcoded credential — was it resolved?
  To dismiss: edikt resolve <event_id>   or   /edikt:session (end-of-session sweep)
```

Only the most recent unresolved finding is surfaced (cap: 1 per session start).

### FR-009 — Configurable gate severity tiers

`.edikt/config.yaml` gains a `gates:` section:
```yaml
gates:
  security: warning    # fire on warning and above
  dba: critical        # fire only on critical
  sre: warning
  architect: warning
  performance: critical
  api: warning
  default: critical    # for unknown agents
```

`subagent-stop.sh` reads `gates.<agent>` (or `gates.default`) and applies the threshold
when deciding whether to block.

**Agent domains** are inferred from the structured payload's `agent` field (ADR-023).
If absent, fall back to content-based keyword detection (legacy path, logs a warning).

**UX:** gate output includes the applied threshold:
```
🔴 BLOCKED — security gate fired (severity: critical ≥ threshold: warning)
   To change threshold: .edikt/config.yaml gates.security: critical
```

`/edikt:config` documents the `gates:` section and shows current values.

### FR-010 — Pre-push invariant validation

The pre-push hook (`templates/hooks/pre-push.sh`) currently runs a security-only grep.
Extend it to validate staged files against installed invariants:

1. For each staged file path, check if any invariant in `docs/architecture/invariants/`
   defines a `paths:` pattern that matches.
2. For matching invariants, run a lightweight lint against the diff (not the full file):
   - INV-001: reject `.ts`, `.js`, `.py` in `commands/` or `templates/`.
   - INV-002: reject edits to ADR files with `status: accepted` in frontmatter.
   - INV-003: reject `echo '{"` or `printf '{"` patterns in hook scripts.
   - INV-006: reject raw string interpolation into hook payloads.
3. Exit 1 with an actionable message on any violation.
4. User can bypass with `EDIKT_BYPASS_PREPUSH=1 git push` (logged to events.jsonl).

This runs as a pure shell/Python check — no Claude call, no model cost.

---

## Acceptance Criteria

### AC-001 — install.sh excludes tier-2 (existing test, must pass)

`test_install_sh_does_not_install_benchmark`: `install.sh` contains no reference to
`benchmark`, `gov-benchmark`, `pip install`, or `pipx`.

### AC-002 — `edikt install benchmark` copies markdown

`test_install_benchmark_adds_markdown_and_venv`: with `EDIKT_TIER2_SKIP_PIP=1`,
`benchmark.md` and all 10 attack templates are present under
`$CLAUDE_HOME/commands/edikt/`.

### AC-003 — tier-1 unchanged after tier-2 install

`test_install_benchmark_leaves_tier1_unchanged`: SHA-256 hash of every file under
`~/.edikt/versions/<tag>/commands/` is byte-equal before and after `edikt install
benchmark`.

### AC-004 — Python version check fires before filesystem mutation

`test_python_version_check_uses_literal_message` + `test_python_version_check_rejects_old_python`:
- Missing Python → `edikt benchmark requires Python 3.10+` on stderr, exit ≠ 0.
- Python 3.9 → same message + `found 3.9 at <path>`, exit ≠ 0.
- No filesystem writes occur before the check.

### AC-005 — pip failure rolls back markdown

`test_pip_failure_rolls_back_markdown`: `EDIKT_TIER2_WHEEL=/nonexistent/…` → install
exits ≠ 0; `benchmark.md` and all attack templates are absent from `$CLAUDE_HOME/`.

### AC-006 — venv isolation

Install with `EDIKT_TIER2_SKIP_PIP=1` writes venv sentinel to
`$EDIKT_HOME/venv/gov-benchmark/`. No venv-related files appear outside that path.

### AC-007 — uninstall idempotence

`test_uninstall_on_empty_state_exits_zero`: `edikt uninstall benchmark` on empty state
exits 0 and prints "already uninstalled" (case-insensitive substring).

`test_uninstall_after_install_exits_zero`: install then uninstall removes `benchmark.md`.

`test_uninstall_tolerates_partial_state`: manual markdown deletion followed by uninstall
exits 0.

### AC-008 — wheel checksum mismatch aborts

`test_wheel_checksum_mismatch_aborts`: wrong `EDIKT_TIER2_WHEEL_SHA256` → stderr
contains `Wheel checksum mismatch`, exit ≠ 0.

`test_wheel_checksum_match_proceeds`: matching SHA-256 → exit 0 with `SKIP_PIP=1`.

### AC-009 — pyproject.toml pins exactly

`test_pyproject_pins_sdk_exactly`: `tools/gov-benchmark/pyproject.toml` contains
`claude-agent-sdk==` and does not contain `>=`, `~=`, or `*` for that dependency.

### AC-010 — benchmark.md tier frontmatter

`test_tier_frontmatter_declared_on_benchmark`: `commands/gov/benchmark.md` frontmatter
contains `tier: 2`.

### AC-011 — release-path wheel requires SHA-256

`test_release_path_wheel_without_sha256_is_rejected`: wheel at a `/current/` path
without `EDIKT_TIER2_WHEEL_SHA256` set → abort, clear error.

### AC-012 — e2e smoke: tier-2 install chain (test_e2e_v060_release.py)

`TestTier2InstallIsolation::test_tier2_install_benchmark_markdown_only`: benchmark.md
present after install; tier-1 `context.md` byte-equal.

### AC-013 — e2e smoke: orphan detection warn→block (test_e2e_v060_release.py)

First compile with orphan ADR → exit 0, `.edikt/state/compile-history.json` written.
Second compile → exit ≠ 0, block message, history NOT overwritten.
Resolve via `no-directives:` frontmatter → compile exits 0 again.

### AC-014 — e2e smoke: doctor missing ADR source (test_e2e_v060_release.py)

`governance.md` references `ADR-999` which does not exist on disk → `/edikt:doctor`
exits ≠ 0 (or reports failure) and names the missing source.

### AC-015 — e2e smoke: benchmark preflight without Python helper (test_e2e_v060_release.py)

With `EDIKT_TIER2_SKIP_PIP=1` (no venv), running the benchmark command exits 2 with
an actionable message (not a crash). The message explains that
`python -m gov_benchmark.run` is unavailable and names the install step needed.

### AC-016 — ADR-023 written and accepted

`docs/architecture/decisions/ADR-023-subagent-stop-structured-evaluator-input.md`
exists with `status: accepted` before any hook changes land.

### AC-017 — subagent-stop.sh consumes structured payload

`subagent-stop.sh` reads `evaluator_output.severity` and `evaluator_output.findings[]`
from the hook input JSON. Legacy unstructured payloads are handled via the fallback path
with a logged warning, not a crash.

### AC-018 — session-start-with-edikt fixture re-added

`fixtures.yaml` §9.1 contains `session-start-with-edikt.expected.json` with
`status: characterized`, a `verified_by` command, and a `verified_at` date.
The `_note` deferral marker is removed. The fixture pair passes in the hook unit suite.

### AC-019 — subagent-stop-critical fixture re-added

Same as AC-018 for `subagent-stop-critical.expected.json`.

### AC-020 — _staged_runner.sh has git-identity and clock stubs

`test/unit/hooks/_staged_runner.sh` exports `stub_git_identity()`,
`provision_memory_fixture()`, and `stub_clock()`. All existing fixture tests still pass.

### AC-021 — fixtures.yaml schema has status field

All Phase 11b expected-output records have `status: characterized` backfilled.
No record has `status` missing and `verified_by` present (or vice versa).

### AC-022 — /edikt:sdlc:drift shows characterization section

When run against a project with mixed characterized/aspirational fixtures, drift output
contains `§ Fixture characterization status` with counts and per-aspirational rationale.

### AC-023 — /edikt:sdlc:artifacts defaults to aspirational

New fixture records generated by `/edikt:sdlc:artifacts` contain `status: aspirational`
and display a warning in command output.

### AC-024 — /edikt:doctor characterization check

Doctor reports characterization rate and warns when >50% aspirational.
With `--deep`, runs `verified_by` for records >90 days old and flags stale output.

### AC-025 — deprecated stub removal

`commands/deprecated/` directory is absent (or empty) after this release. `git log`
shows a commit with message `chore: remove deprecated command stubs (v0.6.0)`.

### AC-026 — shared routing template exists and is consumed

`commands/_shared-agent-routing.md` exists. `plan.md`, `review.md`, and `drift.md`
each reference it. Domain signal table is not duplicated across these three files.

### AC-027 — doctor reads events.jsonl for unresolved findings

`/edikt:doctor` includes a `§ Gate activity` section when `events.jsonl` has entries.
Section is absent (not an error) when the file is missing.

### AC-028 — session-start hook surfaces one unresolved finding

Session-start (or PostCompact) hook emits `systemMessage` with unresolved finding
when `events.jsonl` has an unresolved gate entry. Emits nothing (not an empty field)
when all findings are resolved.

### AC-029 — gates config in .edikt/config.yaml

`config.yaml` template includes `gates:` section with default values. `subagent-stop.sh`
reads `gates.<agent>` and applies the threshold. `/edikt:config` documents the section.

### AC-030 — gate output includes threshold in block message

When gate fires, user-visible output includes `severity: X ≥ threshold: Y` and a
one-liner to change the threshold.

### AC-031 — pre-push hook validates invariant compliance on diff

Pre-push hook catches an INV-001 violation (`.ts` file in `commands/`), an INV-002
violation (edit to accepted ADR), and an INV-003 violation (`echo '{"`) in the diff.
Each fires a specific error message with the invariant ID.

### AC-032 — pre-push bypass is logged

`EDIKT_BYPASS_PREPUSH=1 git push` runs without the gate AND writes a bypass event to
`events.jsonl` with timestamp and file list.

---

## Data model changes

No new persistent state schemas beyond those introduced by SPEC-005
(`compile-history.json`, benchmark report formats). Changes in this spec:

**`fixtures.yaml` schema extension** (additive, backward-compatible):
```yaml
- path: <name>.expected.json
  format: json
  status: characterized        # NEW optional field
  verified_by: "<command>"     # NEW optional field (required when status=characterized)
  verified_at: "2026-04-17"    # NEW optional field (required when status=characterized)
  target_phase: "..."          # NEW optional field (when status=aspirational)
  target_contract: "..."       # NEW optional field (when status=aspirational)
  content: { ... }
```

**`events.jsonl` gate event schema** (additive):
```json
{
  "event": "gate_fired",
  "agent": "evaluator",
  "verdict": "FAIL",
  "severity": "critical",
  "findings_count": 2,
  "ts": "2026-04-18T10:00:00Z",
  "resolved": false
}
```

**`.edikt/config.yaml` schema extension** (additive):
```yaml
gates:
  security: warning
  dba: critical
  default: critical
```

---

## New ADRs

### ADR-023 — SubagentStop structured evaluator-input contract

**Decision:** `subagent-stop.sh` accepts a structured JSON field
`evaluator_output: {severity, findings[]}` in the hook payload (written by evaluator
agents per ADR-018). This replaces content-based severity inference. Legacy unstructured
payloads are supported via a fallback path; a warning is logged.

**Supersedes:** nothing. Extends ADR-018 (evaluator verdict schema) and ADR-014
(hook JSON transport).

**Status:** to be written in Phase 1.

---

## Phases (see PLAN-SPEC-006 for full prompts)

| Phase | Title | Wave | Depends on |
|-------|-------|------|------------|
| 1 | ADR-023 + bin/edikt tier-2 verbs (shell) | 1 | — |
| 2 | Go binary for benchmark install | 2 | 1 |
| 3 | subagent-stop.sh structured payload consumer | 2 | 1 |
| 4 | _staged_runner.sh stubs + fixture re-additions | 2 | 1, 3 |
| 5 | Fixture status field backfill + drift axis | 2 | — |
| 6 | /edikt:sdlc:artifacts aspirational default + doctor checks | 3 | 5 |
| 7 | Shared agent routing layer + deprecated stub removal | 2 | — |
| 8 | Events.jsonl doctor awareness + session-start surfacing | 2 | — |
| 9 | Configurable gate severity tiers | 3 | 3, 8 |
| 10 | Pre-push invariant validation | 2 | — |
| 11 | CHANGELOG + ROADMAP update + passing the AC-023 test suite | 4 | all |

**Wave 1:** Phase 1 (installs the verbs needed by AC-023 tests and ADR-023 needed by Phase 3).
**Wave 2 (parallel):** Phases 3, 4, 5, 7, 8, 10 — all independent given Phase 1.
**Wave 3 (parallel):** Phases 2, 6, 9 — Go binary needs shell working, fixture doctor needs schema, severity needs structured hook.
**Wave 4:** Phase 11 — integration, passing all tests, release docs.
