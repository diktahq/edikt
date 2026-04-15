---
type: spec
id: SPEC-004
title: "v0.5.0 Stability Release — Testing, Versioning, Distribution, Init Provenance"
status: accepted
author: Daniel Gomes
implements: PRD-002
source_prd: docs/product/prds/PRD-002-v050-stability-release.md
created_at: 2026-04-14T00:00:00Z
references:
  adrs: [ADR-001, ADR-005, ADR-006]
  invariants: [INV-001]
---

# SPEC-004: v0.5.0 Stability Release

**Implements:** PRD-002 (FR-001 through FR-029)
**Date:** 2026-04-14
**Author:** Daniel Gomes

---

## Summary

Four bundled deliverables for release-confidence restoration:

1. **Testing** — hook unit tests (JSON stdin fixtures), Agent SDK integration tests, sandboxed runner, CI gate.
2. **Versioning** — on-disk layout redesign (`~/.edikt/versions/<tag>/`), shell launcher (`~/.edikt/bin/edikt`), rollback.
3. **Homebrew** — reuse existing shared tap `diktahq/homebrew-tap` (already hosts verikt); add `edikt.rb` formula distributing the launcher only.
4. **Init provenance** — path substitution, stack-aware filters, `edikt_template_hash` frontmatter.

This spec defines the on-disk schema, command-line surface, data formats, migration strategy, and test architecture. It does not specify individual agent template edits — those come in the plan.

## Context

PRD-002 documented four regressions shipped in two days (v0.4.0–v0.4.3). Every one required *executing* the command to catch; offline file-structure tests couldn't see them. On top of that: no version pinning, no rollback, no standard distribution, and an init flow that produces uncustomized agents which then collide with upgrade's classifier.

Constraints this spec honors:

- **Absolute hook paths** in every project's `.claude/settings.json` are `$HOME/.edikt/hooks/*.sh`. Must stay resolvable through any layout change.
- **Claude Code command discovery** reads `~/.claude/commands/edikt/`. Must stay resolvable.
- **INV-001** — edikt product is pure markdown + YAML + shell. Test harness is exempt (not shipped).
- **ADR-005** — customization markers (`<!-- edikt:custom -->`, `agents.custom:`, `<!-- edikt:generated -->`) are preserved unchanged.
- **ADR-006** — CLAUDE.md visible sentinels unchanged.

## Existing Architecture

### Install path (current)

- `install.sh` (453 lines) — per-file `curl` from `raw.githubusercontent.com/diktahq/edikt/main/`. No `--ref` support.
- Writes to `~/.edikt/` (flat): `VERSION`, `CHANGELOG.md`, `templates/`, `hooks/`, `backups/{timestamp}/`.
- Writes to `~/.claude/commands/edikt/` (flat directory of `.md` files).
- Writes project state to `.edikt/config.yaml` and `.claude/settings.json` (absolute hook paths).

### Upgrade path (current)

- `commands/upgrade.md` — orchestrates detection + diff + copy. Reads `$HOME/.edikt/VERSION` (authority) and `.edikt/config.yaml:edikt_version:` (project record).
- Diff model: MD5 hash vs. installed file, `<!-- edikt:custom -->` / config-list skip, interactive preview.
- v0.4.3 added a diff classifier (`docs/plans/PLAN-upgrade-diff-classifier.md`) to distinguish "template moved" from "user customized" via heuristics. Heuristics go away for agents with provenance (FR-028).

### Test runner (current)

- `test/run.sh` discovers `test-*.sh` and runs sequentially. 26 files. All offline.
- `test/helpers.sh` — file existence, grep assertions, YAML sanity.
- `test/fixtures/` — greenfield, established-*, compile, specs. No hook-payload fixtures. No integration fixtures.
- No CI gate. Only `.github/workflows/docs.yml` exists.

## Proposed Design

### 1. Versioned install layout (FR-009 through FR-014)

```
~/.edikt/
  bin/
    edikt                      # shell launcher (~250 lines, POSIX sh)
  versions/
    v0.4.3/                    # prior release, retained for rollback
      VERSION
      CHANGELOG.md
      manifest.yaml            # new: file list + SHA256 for integrity check
      templates/
      hooks/
      commands/
        edikt/                 # flat + namespaced .md files
    v0.5.0/                    # new release
      …same structure…
    dev/                       # present only when `edikt dev link` active
      …symlinks into repo…
  current -> versions/v0.5.0   # THE generation symlink
  hooks -> current/hooks       # stable path used by every project
  templates -> current/templates
  config.yaml                  # user data (global defaults)
  custom/                      # user data (custom agents/rules)
  backups/
    {timestamp}/               # pre-flip snapshot of `current`
  lock.yaml                    # active + previous, install timestamps
  events.jsonl                 # existing; gains install/upgrade/rollback events
~/.claude/
  commands/
    edikt -> ~/.edikt/current/commands/edikt  # symlinked; no longer copied
```

**Critical invariants this layout preserves:**

- `$HOME/.edikt/hooks/session-start.sh` resolves through the symlink chain to `versions/<current>/hooks/session-start.sh`. Existing `.claude/settings.json` files in every project keep working without edits.
- `~/.claude/commands/edikt/` is a symlink into the active version. Claude Code's command discovery continues to work — it dereferences symlinks.

**`manifest.yaml` schema (new, per-version):**

```yaml
version: "0.5.0"
installed_at: "2026-04-14T10:00:00Z"
files:
  - path: hooks/session-start.sh
    sha256: "abc123..."
  - path: commands/edikt/context.md
    sha256: "def456..."
  # one entry per payload file
```

Used by `edikt doctor` to detect tampering or partial installs.

**`lock.yaml` schema (new, global):**

```yaml
active: "0.5.0"
previous: "0.4.3"
installed_at: "2026-04-14T10:00:00Z"
installed_via: "launcher"    # one of: launcher, install.sh, brew, dev
history:
  - version: "0.5.0"
    installed_at: "2026-04-14T10:00:00Z"
    activated_at: "2026-04-14T10:00:00Z"
  - version: "0.4.3"
    installed_at: "2026-04-13T08:00:00Z"
    activated_at: "2026-04-13T08:00:00Z"
```

Updated atomically on every `edikt use`, `edikt install`, `edikt rollback`.

### 2. Shell launcher — `~/.edikt/bin/edikt` (FR-015, FR-016)

Single POSIX shell script. Subcommands dispatch via `case` on `$1`. No external runtime dependencies beyond `curl`, `tar`, `grep`, `awk`, `sed`, `ln`, `sha256sum` / `shasum` (macOS fallback).

**Subcommand contracts:**

| Subcommand | Behavior | Exit codes |
|---|---|---|
| `edikt install <tag>` | Fetch release tarball from `github.com/diktahq/edikt/archive/refs/tags/<tag>.tar.gz`, extract to `versions/<tag>/`, write `manifest.yaml`, verify SHA256 against embedded checksum file. Does **not** activate. | 0 ok, 1 network, 2 checksum, 3 already installed |
| `edikt use <tag>` | Flip `current` symlink to `versions/<tag>`. Update `lock.yaml`. Snapshot prior `current` target to `backups/<ts>/`. | 0 ok, 1 not installed, 2 pin mismatch (warn path) |
| `edikt upgrade` | `install latest-stable` + prompt for `use`. Unless `--yes`, confirm. | 0 ok, 1 already latest, 2 declined |
| `edikt rollback` | `use` with `lock.yaml:previous`. | 0 ok, 1 no previous recorded |
| `edikt list` | Print installed versions + active marker. `--verbose` adds install date, SHA, disk usage. | 0 |
| `edikt prune [--keep N]` | Delete `versions/*` older than Nth most recent. Default N=3. Never delete `active` or `previous`. | 0 |
| `edikt doctor` | Verify symlink chain, manifest integrity, provenance coverage %, orphan detection. | 0 healthy, 1 warnings, 2 errors |
| `edikt uninstall` | Remove `~/.edikt/`, unlink `~/.claude/commands/edikt`. Does not touch any project's `.edikt/` or `.claude/` dirs. Prompts unless `--yes`. | 0 |
| `edikt dev link <path>` | Symlink `versions/dev/` → subpaths of `<path>`. `use dev`. For maintainer local loop. | 0 |
| `edikt dev unlink` | Remove `versions/dev/`, `use` the most-recent tagged version. | 0 |
| `edikt version` | Print active version to stdout. | 0 |
| `edikt upgrade-pin` | Update `.edikt/config.yaml:edikt_version:` to the global active version. Only runs inside a project. | 0 |

**Project-pin warn logic (FR-019):** on every invocation (except `version`, `list`, `doctor`), the launcher checks for `.edikt/config.yaml` in `$PWD` and ancestors. If found and `edikt_version:` is set and differs from `lock.yaml:active`, print to stderr:

```
⚠ This project pins edikt v0.4.2 but you are on v0.5.0.
  Run `edikt use 0.4.2` to match, or `edikt upgrade-pin` to update the pin.
```

Does not block. Exit code unaffected.

**Migration logic (FR-018):** on launcher start, if `~/.edikt/hooks/` is a *directory* (not symlink) and `~/.edikt/VERSION` exists, the launcher enters migration mode:

1. Compute target version = `cat ~/.edikt/VERSION`.
2. Print dry-run plan:
   ```
   Migration needed: flat layout → versioned layout.

   Will move:
     ~/.edikt/hooks/           → ~/.edikt/versions/0.4.3/hooks/
     ~/.edikt/templates/       → ~/.edikt/versions/0.4.3/templates/
     ~/.claude/commands/edikt/ → symlinked into versions/0.4.3/commands/edikt/

   Will preserve unchanged:
     ~/.edikt/config.yaml
     ~/.edikt/custom/ (if exists)
     ~/.edikt/backups/
   ```
3. Prompt `Proceed? [y/N]:` — unless `--yes` flag was passed.
4. On yes: move files, create symlinks, write `lock.yaml`, emit `{"event":"layout_migrated","from":"flat","to":"versioned","version":"0.4.3"}` to `events.jsonl`.
5. Idempotent: re-running migration detects symlinked layout and exits 0 with "already migrated".

### 3. `install.sh` rework (FR-008)

Post-v0.5.0, `install.sh` becomes thin — its only job is first-time bootstrap:

1. Parse `--ref <tag>` (default: latest stable tag fetched from GitHub API).
2. Download launcher script to `~/.edikt/bin/edikt` + `chmod +x`.
3. Invoke `~/.edikt/bin/edikt install <tag> --yes` to fetch the payload.
4. Invoke `~/.edikt/bin/edikt use <tag>`.
5. Print next steps (PATH addition if needed, `edikt doctor` suggestion).

Everything else moves into the launcher.

### 4. Homebrew distribution (FR-020 through FR-024)

**Tap repo (existing, shared): `github.com/diktahq/homebrew-tap`**

This tap already hosts dikta-umbrella formulae (e.g., verikt). edikt is added alongside. We are not creating a new repo.

```
homebrew-tap/
  Formula/
    verikt.rb         # existing
    edikt.rb          # NEW — added by this release
  README.md
  .github/workflows/
    test.yml          # `brew audit --strict` + install smoke test (existing; extended to cover edikt)
```

**Install UX for users:**

```bash
brew tap diktahq/tap            # once
brew install edikt              # short form

# or one-liner
brew install diktahq/tap/edikt
```

**Isolation contract:** the release automation (FR-022) must target only `Formula/edikt.rb`. Never modify `verikt.rb` or other formulae. Smoke-test workflow must continue to cover every formula in the tap.

**`Formula/edikt.rb` skeleton:**

```ruby
class Edikt < Formula
  desc "Governance layer for agentic engineering (Claude Code)"
  homepage "https://edikt.dev"
  url "https://github.com/diktahq/edikt/releases/download/v0.5.0/edikt-v0.5.0.tar.gz"
  sha256 "..."
  license "MIT"
  version "0.5.0"

  def install
    bin.install "bin/edikt"
    # No ~/.edikt/ writes here — first invocation bootstraps.
  end

  test do
    system "#{bin}/edikt", "version"
  end
end
```

**Release tarball contents** (`edikt-v0.5.0.tar.gz`):

```
edikt-v0.5.0/
  bin/edikt              # the launcher
  LICENSE
  README.md
```

**NOT in the brew tarball:** templates, commands, hooks. Those ship separately via the launcher's `install` subcommand hitting the main repo's tag archive. Brew formula is only the launcher; payload is fetched on first `edikt install`.

**Release automation** (`edikt/.github/workflows/release.yml`):

On every pushed tag `v*`:

1. Build launcher tarball → GitHub Release asset.
2. Build full payload tarball (existing) → GitHub Release asset.
3. Compute SHA256 of both.
4. Call [homebrew-releaser](https://github.com/Justintime50/homebrew-releaser) action to open PR on `diktahq/homebrew-tap` updating `Formula/edikt.rb` (`url`, `sha256`, `version`). Scoped strictly to `edikt.rb` — never touches `verikt.rb` or sibling formulae.
5. Publish release notes from `CHANGELOG.md`.

### 5. Init — path substitution (FR-025)

During `/edikt:init`, when installing an agent template:

1. Load `paths.*` from the just-written `.edikt/config.yaml`.
2. Read the source template (e.g., `~/.edikt/templates/agents/architect.md`).
3. Apply string substitutions. Defaults stored in `templates/agents/_substitutions.yaml`:

```yaml
substitutions:
  decisions:
    default: "docs/architecture/decisions"
    config_key: "paths.decisions"
  invariants:
    default: "docs/architecture/invariants"
    config_key: "paths.invariants"
  specs:
    default: "docs/product/specs"
    config_key: "paths.specs"
  prds:
    default: "docs/product/prds"
    config_key: "paths.prds"
  plans:
    default: "docs/plans"
    config_key: "paths.plans"
```

4. For each entry: if `config.paths.<key>` is set and differs from `default`, replace all occurrences of `default` string in the template with the configured path, then write the result.

### 6. Init — stack-aware section filtering (FR-026)

**Marker syntax in agent templates:**

```markdown
<!-- edikt:stack:go,typescript -->
## File Formatting

- Go: `gofmt -w`
- TypeScript: `prettier --write`
<!-- /edikt:stack -->

<!-- edikt:stack:python -->
## File Formatting

- Python: `black` or `ruff format`
<!-- /edikt:stack -->
```

**Filter logic during init:**

1. Read `stack:` (list) from `.edikt/config.yaml`.
2. For each `<!-- edikt:stack:<langs> --> … <!-- /edikt:stack -->` block:
   - If `<langs>` intersects `stack:`, keep the block content (strip the markers).
   - If no intersection, delete the block entirely (markers + content).
3. Markers without closing tag log a warning and are left verbatim (fail-safe).

Applies to every agent template during init. `backend.md`, `qa.md`, `frontend.md` are the initial targets (language-heavy).

### 7. Init — provenance frontmatter (FR-027)

On install, init appends two fields to the installed agent's YAML frontmatter:

```yaml
---
name: architect
description: Software architect specialist
model: opus
memory: project
edikt_template_hash: "a1b2c3d4e5f6…"     # md5 of source template BEFORE substitution
edikt_template_version: "0.5.0"           # edikt version that installed it
---
```

**Hashing rule:** md5 is computed against the raw template file on disk (before path substitution and stack filtering). This gives upgrade a stable anchor: "what version N of the template looked like."

### 8. Upgrade — provenance-first comparison (FR-028, FR-029)

New upgrade logic for agents:

```
for each installed agent:
  read frontmatter.edikt_template_hash as stored_hash
  if stored_hash is missing:
    # legacy install (pre-v0.5.0) — fall back to v0.4.3 diff classifier
    use existing classifier flow
    continue
  current_template_hash = md5(current source template)
  if stored_hash == current_template_hash:
    # template hasn't moved; any install differences are user customizations
    # do not overwrite
    log "preserved user edits on {agent}"
    continue
  else:
    # template moved forward; compute user's custom diff
    stored_expected = re-synthesize what init would have produced with stored_hash template + current config
    user_diff = diff(stored_expected, installed_file)
    if user_diff is empty:
      # no customizations; safe to replace with new template (re-synthesized with current config)
      replace installed file
      update frontmatter hash + version
    else:
      # user customized; prompt with 3-way preview (old template, new template, user edits)
      present diff to user, ask to proceed/skip/abort
```

**Benefit over v0.4.3 classifier:** no heuristics. `stored_hash` is ground truth for "what init produced." Any observed difference from the re-synthesis is definitionally a user edit.

### 9. Testing — three layers (FR-001 through FR-007)

#### Layer 1 — Hook unit tests (Bash + JSON fixtures)

**Directory layout:**

```
test/unit/
  hooks/
    test_session_start.sh
    test_user_prompt_submit.sh
    test_pre_tool_use.sh
    test_post_tool_use.sh
    test_stop.sh
    test_subagent_stop.sh
    test_pre_compact.sh
    test_post_compact.sh
    test_instructions_loaded.sh
  fixtures/
    hook-payloads/
      session-start.json
      user-prompt-submit-no-plan.json
      user-prompt-submit-with-plan.json
      stop-adr-candidate.json
      stop-new-route.json
      stop-new-env-var.json
      stop-security-change.json
      subagent-stop-critical.json
      subagent-stop-warning.json
      subagent-stop-ok.json
      post-compact-with-plan.json
      post-compact-with-failing-criteria.json
      instructions-loaded-governance.json
      pre-tool-use-write.json
      post-tool-use-go.json
      post-tool-use-ts.json
  expected/
    hook-outputs/
      stop-adr-candidate.expected.json   # expected stdout
      …
```

**Test shape** (each `test_*.sh`):

```bash
#!/usr/bin/env bash
. "$(dirname "$0")/../../helpers.sh"

assert_hook_output() {
  local hook=$1
  local fixture=$2
  local expected=$3
  local actual
  actual=$(cat "$fixture" | "$hook" 2>/dev/null)
  if ! diff <(echo "$actual" | jq -S .) <(echo "$expected" | jq -S .) > /dev/null; then
    fail "hook output mismatch: $hook with $fixture"
  fi
}

assert_hook_output "$HOOKS/stop-hook.sh" \
  fixtures/hook-payloads/stop-adr-candidate.json \
  expected/hook-outputs/stop-adr-candidate.expected.json
```

Every hook × every fixture is a test row. Fast, deterministic, free.

#### Layer 2 — Agent SDK integration tests (Python)

**Directory layout:**

```
test/integration/
  pyproject.toml              # declares claude-agent-sdk, pytest
  conftest.py                 # fixture project setup, SDK session helpers
  test_init_greenfield.py
  test_plan_phase_execution.py
  test_post_compact_recovery.py
  test_upgrade_preserves_customization.py
  test_spec_preprocessing.py
  test_evaluator_blocked_verdict.py
  regression/
    test_v040_silent_overwrite.py
    test_v042_blank_line_corruption.py
    test_v042_preflight_order.py
    test_v043_evaluator_silent_fail.py
  fixtures/
    project-greenfield/
    project-mid-plan/
    project-post-compact/
    project-with-customized-agents/
  snapshots/
    test_init_greenfield.snapshot.json
    …
  failures/                    # session logs written here when a test fails
```

**Test shape (pytest + SDK):**

```python
from claude_agent_sdk import query, ClaudeAgentOptions, HookMatcher
from pathlib import Path

async def test_plan_phase_execution(fixture_project_mid_plan, assert_tool_sequence):
    tool_calls = []
    hook_events = []

    options = ClaudeAgentOptions(
        setting_sources=["project"],
        cwd=fixture_project_mid_plan,
        hooks={
            "PreToolUse": [HookMatcher(matcher="Write|Edit", hooks=[
                lambda payload: tool_calls.append(payload) or {}
            ])],
            "Stop": [HookMatcher(hooks=[
                lambda payload: hook_events.append(payload) or {}
            ])],
        },
    )

    async for msg in query(prompt="Continue phase 2", options=options):
        pass  # drain stream

    # Behavior assertions
    assert any(tc["tool_input"]["file_path"].endswith("handler.go") for tc in tool_calls)
    assert any(e.get("last_assistant_message", "").startswith("Phase 2 complete") for e in hook_events)

    # Snapshot assertion (fuzzy-match)
    assert_tool_sequence(tool_calls, snapshot="test_plan_phase_execution")
```

**Fuzzy-match snapshot helper:** compares tool sequences by *type + target pattern*, ignoring exact arg order and wording. E.g., `Write(path matching *.go)` is the assertion, not `Write(path="foo/bar/handler.go", content="…")`.

**Failure hook:** `conftest.py` registers a pytest failure handler that writes the SDK session message stream to `test/integration/failures/<test_name>-<timestamp>.jsonl` for `claude-replay` inspection.

#### Layer 3 — Sandboxed runner (FR-001)

**New `test/run.sh` preamble:**

```bash
#!/usr/bin/env bash
set -euo pipefail

# Sandbox: redirect HOME + edikt state to a temp tree per-run
TEST_SANDBOX=$(mktemp -d -t edikt-test-XXXXXX)
export HOME="$TEST_SANDBOX/home"
export EDIKT_HOME="$HOME/.edikt"
export CLAUDE_HOME="$HOME/.claude"
mkdir -p "$EDIKT_HOME" "$CLAUDE_HOME"

cleanup() {
  rm -rf "$TEST_SANDBOX"
}
trap cleanup EXIT

# Layered runs
./test/unit/run.sh                    # Layer 1 (hooks + shell)
./test/run-versioning.sh              # Layer 1 (launcher subcommands)
if [ "${SKIP_INTEGRATION:-0}" != "1" ]; then
  cd test/integration && pytest        # Layer 2
fi
```

**Kills:** shared state leakage (FR-001 primary goal), encoding collisions, git-repo assumptions (tests set up their own repo inside sandbox).

#### CI gate — `.github/workflows/test.yml`

```yaml
name: test
on:
  pull_request:
  push:
    branches: [main]
    tags: ["v*"]

jobs:
  unit-and-sandbox:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: ./test/run.sh
        env: { SKIP_INTEGRATION: "1" }

  integration:
    if: github.ref_type == 'tag'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with: { python-version: "3.12" }
      - run: pip install claude-agent-sdk pytest pytest-asyncio
      - run: cd test/integration && pytest
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
```

Unit + sandbox run on every PR (fast, free). Integration runs on tag push (slower, costs API). Both layers block — tag fails to release if integration fails.

### 10. Multi-version migration matrix

FR-018 covers the flat→versioned layout migration. That's one of several transitions an old install might need. The launcher detects which migrations apply and runs them in order.

**Detection** — launcher inspects installed state on first run:

| Signal | What it means |
|---|---|
| `~/.edikt/hooks/` is a real dir (not symlink) | Pre-v0.5.0 layout |
| `~/.edikt/VERSION` reads `0.1.*` | v0.1.x install — HTML sentinels, flat command names |
| `~/.edikt/VERSION` reads `0.2.*` | v0.2.x install — namespaced commands, new sentinels, old compile schema |
| `~/.edikt/VERSION` reads `0.3.*` | v0.3.x install — compile schema v1, no criteria sidecar |
| `~/.edikt/VERSION` reads `0.4.*` | v0.4.x install — current layout but no provenance on agents |
| Missing or absent | Fresh install — no migration needed |

**Migration steps by source version:**

| Migration | Applies to | Action |
|---|---|---|
| M1: layout (flat → versioned) | All pre-v0.5.0 | Covered by FR-018 |
| M2: CLAUDE.md sentinels (HTML → markdown link refs) | v0.1.0 only | Detect `<!-- edikt:start -->`, rewrite to `[edikt:start]: #` block. Existing upgrade logic handles this; ensure it runs during migration. Per ADR-006 backward compatibility. |
| M3: flat command names (`/edikt:plan` → `/edikt:sdlc:plan`) | v0.1.x | Commands re-install from new layout (symlink into versioned `commands/`). Old flat files in `~/.claude/commands/edikt/*.md` (top-level, non-namespaced) deleted if unmodified. |
| M4: compile schema v1 → v2 | v0.2.x, v0.3.x | Re-run `/edikt:gov:compile` automatically after migration to regenerate `.claude/rules/governance.md` with v2 sentinel blocks. Non-destructive — existing manual directives preserved via ADR-008 three-list schema. |
| M5: config.yaml schema | v0.1.x, v0.2.x | Add missing keys with defaults (`paths:`, `stack:`, `gates:`). Never remove or rename existing keys. |
| M6: agent provenance backfill | All pre-v0.5.0 | **No auto-backfill.** Agents without `edikt_template_hash` fall back to v0.4.3 classifier per FR-029. Users can opt into backfill with `edikt doctor --backfill-provenance` which assumes installed file matches the template of their stored `edikt_version` at install time. |

**Run order:** M1 → M2 → M3 → M5 → M4 → M6 (layout first so subsequent steps can find files; sentinels and command names before config re-synthesis; compile last because it reads everything else; provenance only on demand).

**Testing (Layer 2):** `test/integration/migration/` holds one fixture per source version (v0.1.0, v0.1.4, v0.2.0, v0.3.0, v0.4.3). Each fixture is a frozen snapshot of that version's full install. Migration test runs launcher against each, asserts final layout matches a v0.5.0 reference install and no user-modifiable data was lost.

**What we do NOT migrate automatically:**

- Deprecated command stubs (`commands/deprecated/*`) — deferred to v0.6.0 per roadmap item 4.4.
- User-written plans, ADRs, invariants, PRDs, specs — never touched.
- Project-local `.edikt/config.yaml` — only the global one gets schema migration; project configs are migrated on first in-project launcher invocation with user confirmation.

**Escape hatch:** `edikt migrate --dry-run` shows exactly what would change. `edikt migrate --abort` restores the pre-migration snapshot from `~/.edikt/backups/migration-<ts>/`. Every migration step writes a backup before acting. (The `--from <version>` selector was reserved for the multi-tag migration scenarios M2–M5; those ship in Phase 7, so v0.5.0 ships M1 only via implicit detection and omits `--from`.)

### 11. Project-mode installs

edikt's `install.sh` supports `--project` mode, which writes to `.edikt/` and `.claude/commands/edikt/` inside the project rather than `$HOME/`. This mode matters for teams that want edikt versioned alongside the project's code.

**Project-mode layout mirrors global-mode:**

```
<project>/.edikt/
  bin/edikt                    # launcher copy (or symlink if global is also installed)
  versions/<tag>/              # payload per tag
  current -> versions/<tag>    # generation symlink
  hooks -> current/hooks
  templates -> current/templates
  config.yaml                  # project config (this file is the whole reason for project mode)
  custom/
  backups/
  lock.yaml
```

**Key differences from global mode:**

- `.claude/commands/edikt/` symlinks into `.edikt/current/commands/edikt/` (project-relative).
- `.claude/settings.json` hook paths use `${PROJECT_ROOT}/.edikt/hooks/*.sh` instead of `$HOME/.edikt/hooks/*.sh`. Launcher rewrites these on project install.
- `edikt_version:` in `.edikt/config.yaml` is the authoritative version for the project (matches what global mode treats as a pin).
- Launcher auto-detects mode: if `$PWD/.edikt/bin/edikt` exists, it overrides the global `$PATH` launcher. Mirrors how `nvm` / `mise` handle project-local toolchains.

**Migration:** project-mode installs follow the same M1–M6 steps but scoped to `.edikt/` inside the project. The launcher migrates the one it's invoked from.

**Non-goal:** bidirectional sync between global and project installs. They're independent; a project on v0.4.2 + a global on v0.5.0 is allowed and documented.

### 12. Documentation deliverables

The product change is visible to every user; docs must keep up.

#### 12.1 `README.md` (root)

- Install section rewritten: brew-first (macOS/Linux primary), `curl | bash` fallback (all platforms), Windows/WSL note.
- New "Upgrade and rollback" section covering `edikt upgrade`, `edikt rollback`, `edikt use <tag>`.
- Link to v0.5.0 CHANGELOG highlights.
- Badge strip: tag version, CI status, homebrew tap.

#### 12.2 `website/getting-started.md`

- Install walkthrough updated (both channels).
- "Pin a version" subsection showing `.edikt/config.yaml:edikt_version:` usage and the warn behavior.
- Dev-loop subsection for contributors (`edikt dev link`).

#### 12.3 `website/guides/` — new pages

- `upgrade-and-rollback.md` — full guide: how upgrade works, how to roll back, how to pin, what gets preserved.
- `migrating-from-v0.4.md` — step-by-step walkthrough of the one-time layout migration, with screenshots of the prompt and expected output.
- `homebrew.md` — tap install, `brew upgrade` vs `edikt upgrade` distinction.

#### 12.4 `website/index.md`

- Top-of-page install snippet updated to brew-first.
- "Reliability" callout referencing the new test layers and regression museum (optional, marketing-adjacent).

#### 12.5 `website/commands/` and `website/governance/` — artifact & command docs refresh

v0.5.0 changes what `/edikt:init`, `/edikt:upgrade`, and `/edikt:sdlc:artifacts` do. The website's reference pages must reflect the new behavior or users are reading stale docs.

**Pages to update:**

- `website/commands/upgrade.md` — document rollback, `edikt rollback` command, provenance-first flow, 3-way diff prompt on moved templates, `--dry-run` for upgrade, migration from pre-v0.5.0 layouts.
- `website/commands/init.md` *(create if missing)* — document path substitution, stack-aware section filtering, `edikt_template_hash` + `edikt_template_version` frontmatter, how `paths.*` and `stack:` in `.edikt/config.yaml` drive init.
- `website/commands/doctor.md` *(create if missing)* — document new launcher-level `edikt doctor` checks: symlink health, manifest integrity, provenance coverage %, disk usage per version.
- `website/commands/sdlc/artifacts.md` — reflect any additions to the artifact enumeration; confirm generated-artifact list is accurate against what v0.5.0 produces.
- `website/governance/sentinels.md` — if it enumerates on-disk state, add `~/.edikt/lock.yaml`, `manifest.yaml`, updated `events.jsonl` event types.
- `website/governance/evaluator.md` — no functional change in v0.5.0, but cross-link to new testing layers section in the guides.
- `website/governance/features.md` — list the new `EDIKT_EXPERIMENTAL` feature flag gating pre-v0.5.0-GA behavior, if applicable.
- `website/rules/` — no changes expected; rule pack mechanism is unchanged.

**Cross-link pass** — every new guide page (`guides/upgrade-and-rollback.md`, `guides/migrating-from-v0.4.md`, `guides/homebrew.md`) must be linked from at least one command page and from `getting-started.md`. The docs-sanity test (§9) greps for broken cross-links.

#### 12.6 `website/faq.md`

- New Q: "How do I roll back a bad release?"
- New Q: "Can I pin edikt per project?"
- New Q: "What happened to my old `~/.edikt/hooks/`?"
- New Q: "Why did brew upgrade edikt but `edikt upgrade` still says there's an update?"

#### 12.7 `CHANGELOG.md` — v0.5.0 entry

Structured by deliverable bundle:

- **Testing** — Layers 1/2/3, CI gate, regression museum.
- **Versioning** — new layout, launcher CLI, rollback, migration.
- **Distribution** — Homebrew tap, release automation.
- **Init** — path substitution, stack filters, provenance.
- **Breaking changes** — any user-facing behavioral shift (e.g., `~/.edikt/hooks/` is now a symlink, `~/.claude/commands/edikt/` is now a symlink).
- **Migration notes** — link to `migrating-from-v0.4.md`.

#### 12.8 Website rebuild triggered

Existing `.github/workflows/docs.yml` already rebuilds the website on push to `main` when `website/**` changes. No workflow change needed — the content updates alone trigger rebuild.

### 13. Regression museum (FR-032)

`test/integration/regression/` holds one test file per historical bug. Header convention:

```python
"""
REGRESSION TEST — DO NOT DELETE.

Reproduces: v0.4.0 silent overwrite of customized agents during upgrade.
Bug commit:  d81f6e3
Fix commit:  (this PR)
Invariant:   /edikt:upgrade MUST NOT overwrite files with user customizations
             without explicit consent, even when the template has moved.

Removing this test reopens the bug. Only delete when the code path that
caused it has been removed entirely and replaced with something
demonstrably different.
"""
```

No CODEOWNERS enforcement. Header + PR discipline.

## File-Level Changes

| Area | Files modified / added | Nature |
|---|---|---|
| Launcher | `bin/edikt` (NEW, ~250 lines shell) | New |
| Install | `install.sh` | Major rewrite |
| Upgrade | `commands/upgrade.md` | Major update (provenance-first flow) |
| Init | `commands/init.md` | Add substitution + stack filter + frontmatter steps |
| Init helpers | `templates/agents/_substitutions.yaml` (NEW) | New |
| Agent templates | `templates/agents/architect.md`, `dba.md`, `backend.md`, `qa.md`, `frontend.md`, `mobile.md` | Add stack markers |
| Tests — Layer 1 | `test/unit/hooks/*.sh` (9 files NEW) | New |
| Tests — fixtures | `test/fixtures/hook-payloads/*.json` (~16 files NEW) | New |
| Tests — Layer 2 | `test/integration/` (NEW dir, pytest suite) | New |
| Tests — runner | `test/run.sh` | Sandbox preamble |
| CI | `.github/workflows/test.yml` (NEW) | New |
| CI | `.github/workflows/release.yml` (NEW) | New |
| Homebrew tap | `diktahq/homebrew-tap` — add `Formula/edikt.rb` alongside existing `verikt.rb` | Extend existing repo |
| Docs | `README.md` | Rewrite install + upgrade/rollback sections |
| Docs | `website/getting-started.md` | Install walkthrough, pinning, dev loop |
| Docs | `website/guides/upgrade-and-rollback.md` (NEW) | New guide |
| Docs | `website/guides/migrating-from-v0.4.md` (NEW) | Migration walkthrough |
| Docs | `website/guides/homebrew.md` (NEW) | Brew tap usage |
| Docs | `website/index.md` | Install snippet + reliability callout |
| Docs | `website/faq.md` | 4 new Q&As |
| Docs | `website/commands/upgrade.md` | Rollback + provenance-first + 3-way diff + migration |
| Docs | `website/commands/init.md` (NEW if missing) | Path substitution + stack filters + provenance frontmatter |
| Docs | `website/commands/doctor.md` (NEW if missing) | Launcher-level doctor checks |
| Docs | `website/commands/sdlc/artifacts.md` | Artifact enumeration refresh |
| Docs | `website/governance/sentinels.md` | New on-disk state entries |
| Docs | `website/governance/features.md` | `EDIKT_EXPERIMENTAL` flag if kept |
| Docs | `CHANGELOG.md` | v0.5.0 entry by bundle |
| Migration fixtures | `test/integration/migration/fixtures/v0.1.0/`, `v0.1.4/`, `v0.2.0/`, `v0.3.0/`, `v0.4.3/` (NEW) | Frozen-install snapshots |
| Migration tests | `test/integration/migration/test_*.py` (NEW, one per source version) | New |

## Dependencies

- `claude-agent-sdk` (Python, test-only).
- `pytest`, `pytest-asyncio` (test-only).
- `jq` (CI, already broadly available).
- `sha256sum` / `shasum` (launcher, platform-dependent fallback).
- GitHub Actions: `actions/checkout`, `actions/setup-python`, `Justintime50/homebrew-releaser`.
- **Removed dependency:** per-file `raw.githubusercontent.com` fetching — replaced with single tarball per release.

## Rollout Order

The plan phase will sequence these. Suggested order (lowest risk first):

1. **Layer 3 sandbox + Layer 1 hook unit tests** — no API cost, no user impact. Immediately stabilizes the suite.
2. **Launcher + versioned layout + migration** — everything offline. Ship behind `EDIKT_EXPERIMENTAL=1` flag first.
3. **`install.sh` rewrite** — depends on launcher.
4. **Init provenance** — depends on launcher only for path consistency.
5. **Upgrade provenance-first flow** — depends on init provenance.
6. **Layer 2 Agent SDK tests** — depends on launcher + init provenance for fixture setup.
7. **Homebrew tap + release automation** — depends on all of the above.
8. **Regression museum backfill** — written concurrently with fixes in earlier phases.

## Testing Strategy

How we test this spec itself:

- **Launcher:** every subcommand covered by Layer 1 shell tests in `test/unit/launcher/`. Covers symlink flip correctness, lock.yaml updates, rollback, prune, pin-warn paths.
- **Migration:** `test/integration/migration/` has one fixture per source version (v0.1.0, v0.1.4, v0.2.0, v0.3.0, v0.4.3). Each test runs the launcher against its fixture, asserts the final layout matches a v0.5.0 reference, verifies CLAUDE.md sentinels migrated, command namespaces updated, compile schema re-synthesized, no user-modifiable data lost. Migration dry-run and `--abort` paths covered.
- **Project-mode:** parallel fixture set in `test/integration/project-mode/`. Asserts `.claude/settings.json` hook paths are rewritten to project-relative, launcher detection prefers project install over global when both present.
- **Docs:** `test/unit/test-docs.sh` greps README and website pages for outdated install snippets (`curl ... raw.githubusercontent.com/...`), stale version references, and broken cross-links to the new guides. Fast and free.
- **Init provenance:** test fixture with sample `paths.*` config; assert installed agent has substituted paths, filtered stack sections, and correct frontmatter hash.
- **Upgrade:** Layer 2 test with a customized agent fixture; assert upgrade preserves customizations when template unchanged, prompts on 3-way diff when template moved.
- **Homebrew:** `diktahq/homebrew-tap`'s existing CI runs `brew install --HEAD` smoke test + `brew audit --strict` for every formula in the tap. edikt's addition extends this suite; verikt coverage must not regress.

## Non-Goals (carried from PRD)

- homebrew-core submission
- Windows native install
- Mock/replay mode for tests
- Auto-migration across major layout changes beyond the one in FR-018
- Remaining v0.5.0 roadmap items (moved to v0.6.0)
- Deterministic tool ordering or cost caps in integration tests

## Decisions Locked (from PRD)

1. Tagged releases only, no edge channel.
2. Warn-only on pin mismatch.
3. Always-prompt migration, `--yes` flag for scripters.
4. No cost gating (maintainer subscription covers it).
5. Regression museum via header comments only, no CODEOWNERS.
