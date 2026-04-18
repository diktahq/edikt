---
type: artifact
artifact_type: config-spec
spec: SPEC-006
status: accepted
created_at: 2026-04-18T00:00:00Z
reviewed_by: sre
---

# SPEC-006 Configuration Surface

**Spec:** SPEC-006 — v0.6.0 SDLC rework, tier-2 Go install, and hook hardening
**Date:** 2026-04-18

This document is the authoritative specification for every environment variable, `.edikt/config.yaml` key, and runtime configuration flag introduced or modified by SPEC-006. Implementers must read this alongside the spec itself; the spec describes behavior, this document describes the control surface.

---

## 1. Environment Variables

All variables follow a common contract: they are read at process start, never mutated by edikt, and never passed to Claude-facing channels (INV-004). Variables that carry file paths are validated against an allowlist regex before interpolation into argv (INV-006). Variables with a `SKIP` or `BYPASS` semantic MUST be logged to `events.jsonl` before acting on them.

| Name | Type | Default | Required | Scope | Description | Constraint |
|------|------|---------|----------|-------|-------------|------------|
| `EDIKT_HOME` | path | `~/.edikt` | no | install-time, test-only | Override the edikt data home directory. Used by the test harness to sandbox installs without touching the host `~/.edikt`. | Path must exist and be writable. INV-007: test harness MUST set this to a temp directory; never leave it pointing at the real user home during test runs. |
| `CLAUDE_HOME` | path | `~/.claude` | no | install-time, test-only | Override the Claude Code home directory. Used to direct `benchmark.md` and attack templates to a test sandbox instead of the real `~/.claude/commands/`. | Same as `EDIKT_HOME`: must exist, be writable. INV-007: `setting_sources=["project"]` MUST be in effect; the real `~/.claude/settings.json` MUST NOT be read by any subprocess invoked during tests. |
| `EDIKT_TIER2_SKIP_PIP` | `"1"` flag | unset | no | install-time, test-only | If set to `"1"`, the `edikt install benchmark` sequence skips `venv` creation and `pip install`. Writes `$EDIKT_HOME/venv/gov-benchmark/.pip-skipped` as a sentinel. All other install steps (markdown copy, checksum verification, prereq check) still run. | Must be the literal string `"1"` to activate. Any other value is treated as unset. Do not use in production installs; this flag exists for offline dev and test isolation. |
| `EDIKT_TIER2_PYTHON` | path | `python3` | no | install-time | Python binary to use for the `python --version` prereq check and for `python -m venv`. Overriding this allows CI to pin to a specific interpreter without modifying `$PATH`. | Binary must be on the filesystem. Validated against allowlist pattern `^[a-zA-Z0-9_./-]+$` before argv interpolation (INV-006). If the binary reports Python < 3.10, install aborts with the exact message `edikt benchmark requires Python 3.10+; found X.Y at <path>` on stderr before any filesystem writes (FR-001 step 1). |
| `EDIKT_TIER2_WHEEL` | path | unset | no | install-time | Path to a pre-built `.whl` file for the gov-benchmark Python package. When set, pip installs from this wheel instead of from `EDIKT_TIER2_SOURCE/pyproject.toml`. Enables fully offline installs. | If the path contains `/current/` or matches a release-path pattern (see FR-001), `EDIKT_TIER2_WHEEL_SHA256` becomes required; absence aborts install. Path validated against allowlist regex before passing to pip (INV-006). |
| `EDIKT_TIER2_WHEEL_SHA256` | hex string | unset | conditional | install-time | Expected SHA-256 hash of the wheel at `EDIKT_TIER2_WHEEL`. Required when the wheel path resembles a release path (`/current/`, `releases/`, `download/`). SHA-256 is computed over the raw wheel bytes and compared before any pip invocation. | Must be exactly 64 lowercase hex characters. Mismatch produces `Wheel checksum mismatch` on stderr and exits non-zero. This check MUST run before any filesystem writes (ADR-015: "fail fast with an actionable error before touching the filesystem"). |
| `EDIKT_TIER2_SOURCE` | path | unset | no | install-time, dev-mode | Source directory for `benchmark.md` and attack templates. When set, `edikt install benchmark` copies from this path instead of from the release-bundled location at `$EDIKT_HOME/current/tools/gov-benchmark/`. Intended for local development of the benchmark tool against an unpackaged repo checkout. | Directory must contain `commands/gov/benchmark.md` and `templates/attacks/` under the given root. If absent or misstructured, install aborts. This variable is NOT for production use; any CI that sets it without also setting `EDIKT_TIER2_SKIP_PIP=1` should be treated as a misconfiguration. |
| `EDIKT_BYPASS_PREPUSH` | `"1"` flag | unset | no | runtime | If set to `"1"`, the pre-push hook skips invariant validation for the current push. The bypass event MUST be written to `events.jsonl` with the timestamp, file list, and the literal `"event": "prepush_bypassed"` before the hook exits 0. | Must be the literal string `"1"`. Any other value is treated as unset. This variable bypasses governance enforcement — see Security Notes §4. In CI, this MUST NOT be set. `edikt:doctor` reports bypass frequency from `events.jsonl`. |
| `EDIKT_GATE_SEVERITY_THRESHOLD` | `critical` \| `warning` \| `info` | unset (reads config) | no | runtime | Per-invocation override for the gate severity threshold used by `subagent-stop.sh`. When set, overrides the agent-specific value from `.edikt/config.yaml gates.<agent>` for this invocation only. Useful for ad-hoc tightening in CI pipelines without modifying config. | Must be one of the three valid values; any other value causes `subagent-stop.sh` to abort with a clear error (not silently fall back). Does not persist — the override applies only for the duration of the hook invocation. Validated before use (INV-006). |

---

## 2. Feature Flags

SPEC-006 introduces no new boolean feature flags under `features:` in `.edikt/config.yaml`. The `quality-gates: true` flag introduced in v0.5.0 continues to govern whether `subagent-stop.sh` and the pre-push hook are active at all.

The `gates:` section described in §3 effectively acts as per-agent severity flags — configurable thresholds that refine how `quality-gates` fires rather than toggling it. If `quality-gates: false`, the `gates:` section has no effect and `subagent-stop.sh` will not block regardless of severity.

### Interaction matrix

| `quality-gates` | `gates.security` | Effect on security gate |
|-----------------|-----------------|-------------------------|
| `true` | `warning` | Blocks on `warning` and `critical` findings |
| `true` | `critical` | Blocks only on `critical` findings |
| `false` | any | Security gate disabled entirely |

---

## 3. Config Schema Extension — `gates:` section

### 3.1 Full schema

Add the following to `.edikt/config.yaml`:

```yaml
gates:
  security: warning    # block on warning and above (default: warning)
  dba: critical        # block only on critical (default: critical)
  sre: warning         # block on warning and above (default: warning)
  architect: warning   # block on warning and above (default: warning)
  performance: critical # block only on critical (default: critical)
  api: warning         # block on warning and above (default: warning)
  default: critical    # fallback for agents not listed above (default: critical)
```

All keys are optional. If a key is absent, the value from `default` is used. If `default` is also absent, the compiled-in fallback is `critical` — the most conservative posture.

### 3.2 Valid values

| Value | Numeric level | Meaning |
|-------|--------------|---------|
| `critical` | 3 | Block only when severity == `critical` |
| `warning` | 2 | Block when severity >= `warning` (i.e., `warning` or `critical`) |
| `info` | 1 | Block on any finding, including `info` |

Severity ordering is `critical(3) > warning(2) > info(1)`. The gate fires when the finding's severity level is **greater than or equal to** the configured threshold level. In other words: a lower threshold string value means a more aggressive gate.

Example: threshold is `warning` (level 2); finding severity is `critical` (level 3). `3 >= 2` is true — gate fires.

Example: threshold is `critical` (level 3); finding severity is `warning` (level 2). `2 >= 3` is false — gate does not fire.

### 3.3 How `subagent-stop.sh` reads this section

`subagent-stop.sh` resolves the threshold for a given invocation as follows:

1. If `EDIKT_GATE_SEVERITY_THRESHOLD` is set in the environment, use it directly (validation required — see §1).
2. Otherwise, read `.edikt/config.yaml`, navigate to `gates.<agent_domain>`.
3. If the key is absent, fall back to `gates.default`.
4. If `gates.default` is absent, use `critical`.

Agent domain is determined from the structured `evaluator_output.agent` field in the hook payload (ADR-019). If that field is absent (legacy unstructured payload), the hook logs a warning and falls back to `gates.default`. Content-based keyword detection for agent domain is the legacy path — it is deprecated and will be removed in v0.7.0.

### 3.4 User-visible output when gate fires

When the gate fires, `subagent-stop.sh` MUST emit (ref: AC-030):

```
🔴 BLOCKED — <agent> gate fired (severity: <severity> ≥ threshold: <threshold>)
   To change threshold: .edikt/config.yaml  gates.<agent>: critical
```

This message appears as a `systemMessage` in the Claude Code hook output (JSON-wrapped per ADR-014). The message is assembled inside the hook process using `python3 -c 'import json; print(json.dumps(...))'` with the untrusted values as argv (INV-003).

### 3.5 `/edikt:config` documentation

`/edikt:config` MUST display the current `gates:` values and explain the severity ordering when the user asks about gate configuration. It MUST NOT require the user to know the section name in advance — surfacing it on `config show` is sufficient.

---

## 4. Security Notes

### 4.1 Variables dangerous if misused

**`EDIKT_BYPASS_PREPUSH`** is the highest-risk variable in this surface. Setting it to `"1"` disables the pre-push invariant gate entirely for that push — meaning INV-001 violations (compiled code in `commands/`), INV-002 violations (editing accepted ADRs), and INV-003 violations (shell-concatenated JSON in hooks) will all pass undetected. The bypass is logged to `events.jsonl`, but the log is only useful if someone reviews it.

Guidance for CI: `EDIKT_BYPASS_PREPUSH` MUST NOT be set in any CI pipeline. If a CI job sets it to work around a failing gate, the underlying violation should be fixed, not bypassed. `/edikt:doctor` reports bypass frequency so teams can audit accumulated bypasses (FR-008).

**`EDIKT_HOME` and `CLAUDE_HOME`** are sandbox-escape vectors if misset. If either points to the wrong directory, `edikt install benchmark` will write files there — including overwriting existing command files. In production environments, these variables should never be set; they are exclusively for test harness use (INV-007). A CI pipeline that accidentally inherits `CLAUDE_HOME=/home/user/.claude` from a previous test run will mutate the user's Claude installation.

Guidance: CI jobs that use these vars MUST set them to freshly-created temp directories (`mktemp -d`) and clean them up on exit. The test harness in `test/integration/` does this automatically via the `sandbox` fixture.

**`EDIKT_GATE_SEVERITY_THRESHOLD`** when set to `info` will cause the gate to fire on every advisory finding, including informational ones. This is rarely what teams want and will produce excessive friction in active development. Do not set this globally in shell profiles. Use it per-invocation only.

### 4.2 Variables with no production footprint

`EDIKT_TIER2_SKIP_PIP`, `EDIKT_TIER2_SOURCE`, `EDIKT_TIER2_WHEEL`, and `EDIKT_TIER2_WHEEL_SHA256` are install-time and test-only. They have no effect on runtime hook behavior and no effect on tier-1 commands. None of these variables should appear in team-shared `.env` files or shell profiles in production environments.

### 4.3 Checksum integrity

`EDIKT_TIER2_WHEEL_SHA256` is the integrity gate for offline installs. Its absence on a release-path wheel is itself an error (see FR-001 step 2). Do not disable or skip this check. The check runs before pip and before any filesystem write, so a mismatch is always safe to catch.

### 4.4 CI recommendations

For CI pipelines that run `edikt install benchmark` as part of a release smoke test:

```sh
# Recommended CI pattern
export EDIKT_HOME="$(mktemp -d)"
export CLAUDE_HOME="$(mktemp -d)"
export EDIKT_TIER2_WHEEL="/path/to/release/gov-benchmark-<ver>.whl"
export EDIKT_TIER2_WHEEL_SHA256="<sha256 from SHA256SUMS>"
# EDIKT_BYPASS_PREPUSH — NEVER set in CI
# EDIKT_TIER2_SKIP_PIP — set only in unit/integration tests, not release smoke tests
```

INV-007 requires that `setting_sources=["project"]` be set for any subprocess Claude invocations inside tests. The `EDIKT_HOME`/`CLAUDE_HOME` override is the mechanism that keeps test-spawned processes from reading the real user settings.

---

## 5. Migration Guide

### 5.1 Who needs to act

Users upgrading from v0.5.0 to v0.6.0 need to:

1. Add the `gates:` section to `.edikt/config.yaml` if they want per-agent control.
2. No other config changes are required. The `gates:` section is additive — its absence is valid and falls back to `default: critical` (most conservative).

### 5.2 Minimal migration

If you want to preserve the v0.5.0 behavior (block only on critical for all agents), add this to `.edikt/config.yaml`:

```yaml
gates:
  default: critical
```

That is the implicit default, so this change has no behavioral effect — but it makes your posture explicit and visible in `/edikt:config` output.

### 5.3 Recommended starting point

Most teams will want to run security, SRE, and architecture agents at `warning` and DBA and performance agents at `critical` (to reduce noise from agents that produce many advisory findings). The recommended starting configuration:

```yaml
gates:
  security: warning
  dba: critical
  sre: warning
  architect: warning
  performance: critical
  api: warning
  default: critical
```

This is also the configuration shown in the spec (FR-009) and used in the test fixtures. If you install the benchmark tool and run `/edikt:gov:benchmark`, this configuration matches the threshold assumptions used when scoring benchmark results.

### 5.4 Where to find the section in your file

The `gates:` section lives at the top level of `.edikt/config.yaml`, alongside `features:` and `sdlc:`. There is no nesting under another key. Example of a complete post-v0.6.0 config with only the new section added:

```yaml
edikt_version: "0.6.0"
base: docs

stack: []

paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  # ... existing paths unchanged ...

features:
  auto-format: true
  session-summary: true
  signal-detection: true
  plan-injection: true
  quality-gates: true

gates:
  security: warning      # NEW in v0.6.0
  dba: critical          # NEW in v0.6.0
  sre: warning           # NEW in v0.6.0
  architect: warning     # NEW in v0.6.0
  performance: critical  # NEW in v0.6.0
  api: warning           # NEW in v0.6.0
  default: critical      # NEW in v0.6.0

sdlc:
  commit-convention: conventional
  pr-template: false
```

### 5.5 Pre-push hook new behavior

The pre-push hook gains invariant validation in v0.6.0 (FR-010). No config change is needed to activate it — it fires automatically when `quality-gates: true`. If the hook catches a real violation in your repo, fix the violation rather than setting `EDIKT_BYPASS_PREPUSH=1`. If you need to temporarily bypass while fixing, use it once, commit the fix immediately, and confirm the bypass appears in `events.jsonl` (which `/edikt:doctor` will surface).

### 5.6 `edikt_version` field

Update `edikt_version` in `.edikt/config.yaml` to `"0.6.0"` after upgrading. The `/edikt:upgrade` command does this automatically; if you upgrade manually, set it by hand. Leaving it at `"0.5.0"` will cause `/edikt:doctor` to report a version mismatch.

---

## References

- [ADR-014] Hook JSON-wrapping migration is in-scope for v0.5.0 stability release — governs how `subagent-stop.sh` emits JSON output including gate-fired messages.
- [ADR-015] Tier-2 optional tools may depend on packages; core stays markdown-only — source of truth for all `EDIKT_TIER2_*` variable semantics and install contract.
- [ADR-018] Evaluator verdict schema — defines the `severity` field consumed by `subagent-stop.sh` and the `gates:` threshold logic.
- [ADR-021] Go is the language for tier-2 deterministic helpers — constrains the implementation backing `edikt install benchmark` and future tier-2 helpers.
- [INV-001] Commands are plain markdown, no compiled code — the `EDIKT_TIER2_*` vars exist precisely because the install logic that needs them cannot live in a tier-1 markdown command.
- [INV-003] Hooks emit structured JSON — `subagent-stop.sh` gate-fired messages MUST use `python3 -c 'import json; print(json.dumps(...))'` with untrusted `severity`/`agent` values as argv.
- [INV-006] Externally-controlled input validation — all env vars carrying paths or severity values are validated before use.
- [INV-007] Hermetic test sandboxes — `EDIKT_HOME` and `CLAUDE_HOME` are the mechanism for satisfying this invariant in integration tests.
