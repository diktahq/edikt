---
type: prd
id: PRD-002
title: "v0.5.0 — Stability Release: Testing, Versioning, Distribution, Init Provenance"
status: accepted
author: Daniel Gomes
stakeholders: []
created_at: 2026-04-14T00:00:00Z
references:
  adrs: [ADR-001, ADR-005, ADR-006]
  invariants: [INV-001]
  source_documents:
    - docs/internal/plans/ROADMAP.md (v0.5.0 Target — item 2.13)
    - CHANGELOG.md (v0.4.0 through v0.4.3 regressions)
---

# PRD-002: v0.5.0 — Stability Release

**Status:** accepted
**Date:** 2026-04-14
**Author:** Daniel Gomes

---

## Problem

v0.4.0 through v0.4.3 shipped four regressions inside two days — phase-end evaluator never firing, upgrade silently overwriting user customizations, spec preprocessing corrupted by a blank line, plan pre-flight running after conclusion. Each one was caught by a user, not by a test. The current suite has 26 files under `test/`, none of which exercise Claude Code behavior end to end. There is no CI gate. Releases ship on manual confidence.

This is compounded by three missing capabilities that make every release risky:

1. **No way to test Claude behavior.** The 9 lifecycle hooks, 50 slash commands, and 20 agents can only be exercised manually. Hook "tests" check that the script exists and grep for guard clauses — they never pipe real JSON stdin. Flakiness sources trace back to shared state in `~/.edikt/` and `~/.claude/` leaking between runs.

2. **No version control for the installed payload.** `install.sh` pulls per-file from `main` with no tag support. Users can't pin a version. When an upgrade breaks them, there is no rollback — `~/.edikt/backups/{timestamp}/` exists but nothing reads it. A broken release is a broken machine until we ship a fix.

3. **No standard distribution channel.** `curl | bash` is our only entry point. Discoverability is low. Developers who expect `brew install <tool>` hit friction. Uninstall is manual.

Separately but related: **init produces agents with default paths and all-stack formatter references**, then `/edikt:upgrade` treats any user edit as a conflict. v0.4.3 added a reactive classifier that prompts before overwriting, but the real fix is init producing correctly-customized agents in the first place, with provenance so upgrades can distinguish "template moved" from "user customized" cleanly.

## Users

- **Every edikt user** — releases need to be trustworthy. This PRD exists because recent releases weren't.
- **edikt maintainers (Daniel, future contributors)** — need a local dev loop and CI gate to catch regressions before they ship.
- **New users** — need a low-friction install, ideally `brew install`, with the ability to pin a version if a release is bad.
- **Project teams** — need `.edikt/config.yaml` to pin an edikt version so the whole team runs the same rules.

## Goals

- Release confidence restored: no regression in v0.5.0+ reaches users because of a gap our tests could have caught.
- Any user can roll back a bad release in one command without network.
- Any user can pin a specific version globally or per-project.
- `brew install` works on macOS and Linux; `curl | bash` still works everywhere.
- Init produces agents that are already customized to the project's paths and stack — no post-install editing needed, no ambiguity on upgrade.
- Upgrade can cleanly distinguish "the template moved" from "the user customized" without heuristics.

## Non-Goals

- **homebrew-core submission** — custom tap only in this release. Core comes later when edikt has more traction.
- **Windows native install** — WSL continues to work via `curl | bash`. No PowerShell installer, no Chocolatey / winget.
- **Mock / replay mode for integration tests** — Claude Agent SDK does not expose one (confirmed 2026-04-14). Tests hit the real API. Cost absorbed via Daniel's subscription.
- **Deterministic tool-ordering, RNG seeding, or cost-capped integration runs** — the SDK exposes no hooks for any of these. Snapshot tests will tolerate natural variance via fuzzy-match assertions rather than strict equality.
- **Automatic migration of every existing user's `~/.edikt/` layout** — launcher detects old layout on first run and migrates once. No background daemon.
- **Multi-platform compile targets** (Codex, Cursor) — that's the v0.6.0+ track (roadmap item 2.5).
- **Remaining v0.5.0 roadmap items** (4.4 Deprecated stubs, 2.8 Shared agent routing, 2.9 Events.jsonl awareness, 2.12 Pre-push invariant validation, 2.11 Configurable gate severity tiers) — **all deferred to v0.6.0**. v0.5.0 is stability-only.
- **New CLI surface beyond the launcher** — the launcher is pure operational tooling (install/use/rollback). No new slash commands, no new agents.

## Requirements

### Must Have

**Testing infrastructure:**

- FR-001: Test runner isolates `$HOME` to a temp dir and redirects `~/.edikt/` and `~/.claude/` into the temp tree before any test runs [MUST] — kills 4 of 5 flakiness sources.
- FR-002: `test/fixtures/hook-payloads/` contains real Claude Code hook stdin JSON for every hook event (SessionStart, UserPromptSubmit with active plan, PreToolUse Write, PostToolUse Write, Stop with ADR-candidate message, Stop with new route, SubagentStop critical, SubagentStop warning, PreCompact, PostCompact with failing criteria, InstructionsLoaded) [MUST].
- FR-003: Layer 1 hook tests pipe each fixture payload to the corresponding hook script and assert on exit code + stdout JSON shape [MUST]. Replaces current string-grep pseudo-tests.
- FR-004: `test/integration/` drives sessions via the Claude Agent SDK (Python) — `claude_agent_sdk.query()` in-process, with `setting_sources=["project"]` pointing at fixture project dirs — against fixtures covering greenfield init, mid-plan phase execution, post-compact recovery, upgrade with user customization, spec preprocessing, and evaluator verdict under permission sandbox [MUST]. The SDK is chosen over `claude -p` subprocess invocation because it exposes `PreToolUse` / `PostToolUse` / `Stop` / `SessionStart` / `UserPromptSubmit` as in-process callbacks that the test can assert on directly, and supports deterministic multi-turn flows via `session_id` resume.
- FR-005: Integration tests assert on both snapshot (diff captured SDK message stream against baseline) and behavior (specific assertions registered via hook callbacks: evaluator fired with expected verdict, plan phase advanced, no silent overwrite, preprocessing emitted correct counter) [MUST].
- FR-005a: Failing integration test runs persist their raw SDK session log to `test/integration/failures/<test-name>-<timestamp>.jsonl` for local debugging via `claude-replay` or manual inspection [SHOULD].
- FR-006: CI workflow `.github/workflows/test.yml` runs Layer 1 + Layer 3 on every PR [MUST]. Release tags trigger Layer 2.
- FR-007: All three layers are release-blocking. A failure on any layer blocks the tag [MUST].

**Versioning & rollback:**

- FR-008: `install.sh` accepts `--ref <tag>` to install a specific release version [MUST]. Default stays latest stable tag, no longer raw `main`.
- FR-009: Installed payload lives in `~/.edikt/versions/<tag>/` with full contents (templates/, hooks/, commands/, VERSION, CHANGELOG.md, manifest) [MUST].
- FR-010: `~/.edikt/current` is a symlink to the active version [MUST]. Switching versions flips exactly this symlink.
- FR-011: `~/.edikt/hooks` and `~/.edikt/templates` are symlinks to `current/hooks` and `current/templates` [MUST] — preserves the absolute paths baked into every project's `.claude/settings.json` (`$HOME/.edikt/hooks/*.sh`).
- FR-012: `~/.claude/commands/edikt` is a symlink to `current/commands/edikt` [MUST] — preserves Claude Code's command discovery path.
- FR-013: User data lives outside `versions/`: `~/.edikt/config.yaml`, `~/.edikt/custom/`, `~/.edikt/backups/` [MUST]. Upgrades never touch these.
- FR-014: `~/.edikt/lock.yaml` records `active:` and `previous:` version + install timestamp [MUST]. Rollback uses this.
- FR-015: Shell launcher `~/.edikt/bin/edikt` exposes: `install <tag>`, `use <tag>`, `upgrade`, `rollback`, `list`, `prune [--keep N]`, `doctor`, `uninstall`, `dev link <path>`, `dev unlink`, `version` [MUST].
- FR-016: Launcher is pure POSIX shell, no new runtime dependencies beyond what install.sh already requires (curl, tar, ln, grep, awk) [MUST]. Complies with INV-001.
- FR-017: Existing `<!-- edikt:custom -->` markers, `agents.custom:` config list, and `<!-- edikt:generated -->` markers continue to work unchanged [MUST]. This PRD does not redesign customization detection.
- FR-018: On first `edikt upgrade` after v0.5.0 ships, launcher detects the old flat layout (`~/.edikt/hooks/*.sh` as real files) and prompts the user with a dry-run preview before migrating [MUST]. `--yes` flag skips the prompt for scripted use. Idempotent.
- FR-019: `.edikt/config.yaml` `edikt_version:` is honored by the launcher — when the global version does not match the pinned version, the launcher prints a warning and suggests `edikt use <pinned>` or `edikt upgrade-pin` [MUST]. No silent switching, no hard block. User decides.

**Homebrew distribution:**

- FR-020: The existing `diktahq/homebrew-tap` repo publishes a Ruby formula `edikt.rb` that installs the launcher script into `$HOMEBREW_PREFIX/bin/edikt` [MUST]. The tap already hosts other dikta-umbrella formulae (e.g., verikt); edikt is added alongside. Users install via `brew tap diktahq/tap && brew install edikt`, or the short form `brew install diktahq/tap/edikt`.
- FR-021: Formula does not write to `$HOME` at install time. First invocation of `edikt` (any subcommand) bootstraps `~/.edikt/` if absent [MUST].
- FR-022: GitHub Action in the main edikt repo bumps the `edikt.rb` formula in `diktahq/homebrew-tap` on every published Release — tag, tarball URL, SHA256 [MUST]. Use [homebrew-releaser](https://github.com/Justintime50/homebrew-releaser) or equivalent. Action must target only `Formula/edikt.rb` and must not touch other formulae in the shared tap.
- FR-023: `brew upgrade edikt` updates the launcher; it does not touch `~/.edikt/versions/` [MUST]. Payload upgrades remain `edikt upgrade`.
- FR-024: `brew uninstall edikt` removes only the launcher. Launcher subcommand `edikt uninstall` removes `~/.edikt/` and clears `~/.claude/commands/edikt` symlink [MUST].

**Init customization + agent provenance (roadmap item 2.13):**

- FR-025: `/edikt:init` reads `paths.*` from `.edikt/config.yaml` and substitutes default paths in installed agent templates at install time [MUST]. `paths.decisions: adr` produces `adr/` in the installed agent, not `docs/architecture/decisions/`.
- FR-026: Agent templates support stack-filter markers `<!-- edikt:stack:<lang>,<lang> -->` delimiting language-specific sections [MUST]. Init reads `stack:` from config and strips sections whose markers don't intersect the detected stack.
- FR-027: Installed agents carry provenance frontmatter: `edikt_template_hash: <md5 of source template at install time>` and `edikt_template_version: "<edikt version that installed it>"` [MUST].
- FR-028: `/edikt:upgrade` compares stored `edikt_template_hash` against current template hash to detect "template moved forward" vs. "user customized" [MUST]. Eliminates the v0.4.3 diff-classification heuristic for files that carry provenance.
- FR-029: Agents without provenance frontmatter (installed before v0.5.0) fall back to the v0.4.3 classifier [MUST]. No forced migration.

### Should Have

- FR-030: `edikt doctor` reports: active version, previous version, symlink health (all expected symlinks resolve), disk usage per version, provenance coverage (% of installed agents with valid `edikt_template_hash`) [SHOULD].
- FR-031: `edikt list --verbose` shows each installed version's install date, SHA, which payload items differ from the active version [SHOULD].
- FR-032: Integration test fixtures include one "known-bad" project that reproduces each v0.4.0–v0.4.3 regression [SHOULD]. Regression museum — if these ever pass silently again, we have a hole.
- FR-033: Launcher `edikt upgrade --dry-run` previews changes without applying [SHOULD].
- FR-034: Release notes template includes a "Rollback" section stating `edikt rollback` restores the previous version [SHOULD].

### Won't Have

- FR-035: Linuxbrew parity beyond what the tap gives for free [WON'T] — we test on macOS, Linuxbrew users get best-effort.
- FR-036: Binary signing or notarization [WON'T] — pure shell, no binary.
- FR-037: Delta updates between versions [WON'T] — full payload copy per version. Markdown is cheap on disk.
- FR-038: GUI installer, menu-bar app, or any non-CLI surface [WON'T].

## Decisions Locked

1. **Release channels:** Tagged releases only in v0.5.0. No `edge` or rolling channel. Revisit if users ask for it.
2. **Project-pin precedence:** Warn-only. When global version ≠ `.edikt/config.yaml` `edikt_version:`, the launcher prints a warning and suggests `edikt use <pinned>`. No auto-switch, no hard block. User decides per invocation.
3. **Migration prompt:** Always prompt with a dry-run preview on first `edikt upgrade` after v0.5.0. `--yes` flag available for scripted / CI use.
4. **Integration test cost:** Not gated. Covered by maintainer's Claude Code subscription. No `MAX_TEST_COST_USD` enforcement.
5. **Regression museum enforcement:** Bold header comment at the top of each regression fixture stating its origin bug and "DO NOT DELETE". No CODEOWNERS gate — single-maintainer project doesn't benefit from forced review routing. Revisit when contributor count grows.

## Success Metrics

- **Release confidence:** zero user-reported regressions between v0.5.0 release and v0.5.1 (if there is a v0.5.1, it ships planned work, not hotfixes).
- **Rollback works:** manual test — break a v0.5.0 install, run `edikt rollback`, verify working state restored in <10 seconds.
- **Brew adoption:** within 30 days of v0.5.0, ≥20% of new installs come via brew (tracked by install.sh vs. brew formula download telemetry if we add it, or GitHub tap star count as proxy).
- **Init provenance coverage:** 100% of agents installed by v0.5.0+ carry valid `edikt_template_hash`.
- **Test suite no longer flaky:** `./test/run.sh` passes 10/10 runs back-to-back on a developer machine with a live Claude session running concurrently.

## Roadmap Impact

v0.5.0 scope was previously six items. Five are deferred to v0.6.0:

- 4.4 Deprecated Command Stub Removal → v0.6.0
- 2.8 Shared Agent Routing Layer → v0.6.0
- 2.9 Events.jsonl Session-Crossing Memory → v0.6.0
- 2.12 Pre-Push Invariant Validation → v0.6.0
- 2.11 Configurable Gate Severity Tiers → v0.6.0

Only 2.13 (Init Customization + Agent Provenance) stays in v0.5.0 because it's tightly coupled to the versioning/upgrade rework — the provenance hash is the cleanest handoff between init and upgrade, and it would be wasteful to ship the versioning work without it.

## References

- CHANGELOG.md v0.4.0–v0.4.3 — source of regression cases for the regression museum.
- `docs/internal/plans/ROADMAP.md` — roadmap item 2.13 motivation.
- `install.sh`, `commands/upgrade.md`, `templates/settings.json.tmpl` — current install/upgrade surface being rewritten.
- ADR-005 (Extensibility model) — customization markers this PRD preserves.
- ADR-006 (Visible sentinels) — path conventions this PRD preserves.
- INV-001 (Pure markdown/YAML) — constrains the launcher to shell-only. Does not apply to `test/` — test harness may use Python + Agent SDK since tests are not shipped product.
- Claude Agent SDK (Python) — https://docs.claude.com/en/api/agent-sdk — integration test driver.
- `claude-replay` (https://github.com/es617/claude-replay) — local debugging aid for failing integration runs. Not a runtime dependency.
