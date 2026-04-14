---
type: artifact
artifact_type: config-spec
spec: SPEC-004
status: in-progress
reviewed_by: sre
created_at: 2026-04-14T00:00:00Z
---

# Config Spec — SPEC-004 (v0.5.0 Stability Release)

SRE review of the configuration surface introduced by v0.5.0: environment variables, feature flags, on-disk state files, file schemas, CI secrets, filesystem permissions, and platform differences. INV-001 applies — every artifact below is `.yaml`, `.md`, `.json`, or `.jsonl`. No binary formats.

---

## 1. Environment Variables

All launcher and test-harness variables. "Owner" column identifies who reads the variable.

| Variable | Type | Default | Required | Owner | Description |
|---|---|---|---|---|---|
| `HOME` | path | OS-provided | yes | launcher, hooks | Resolves `~/.edikt/` and `~/.claude/`. Test runner overrides to a temp sandbox. |
| `EDIKT_HOME` | path | `$HOME/.edikt` | no | launcher, hooks, `install.sh` | Root of edikt global state. Overridden by test runner sandbox (see §4). Currently computed inside `install.sh`; v0.5.0 promotes it to a first-class launcher env var so tests and project-mode installs can redirect it. |
| `CLAUDE_HOME` | path | `$HOME/.claude` | no | launcher, test runner | Root of Claude Code state. Needed so sandbox tests can redirect command discovery without touching a developer's live Claude session. |
| `EDIKT_FORMAT_SKIP` | bool (`0`/`1`) | `0` | no | `post-tool-use.sh` hook | Existing. When `1`, skips PostToolUse auto-formatting. Preserved unchanged per ADR-003. |
| `EDIKT_EXPERIMENTAL` | bool (`0`/`1`) | `0` | no | launcher | Gates pre-release launcher subcommands during phased rollout (spec §Rollout Order step 2 — "Ship behind `EDIKT_EXPERIMENTAL=1` flag first"). When unset or `0`, experimental subcommands print "experimental; set `EDIKT_EXPERIMENTAL=1` to enable" and exit 3. Removed once the feature graduates. |
| `SKIP_INTEGRATION` | bool (`0`/`1`) | `0` | no | `test/run.sh` | When `1`, Layer 3 runner skips Layer 2 pytest. CI uses `SKIP_INTEGRATION=1` on the PR job and unsets it on the tag job. Also used by contributors without an `ANTHROPIC_API_KEY`. |
| `ANTHROPIC_API_KEY` | secret string | unset | yes for Layer 2 | `test/integration/` (pytest), Claude Agent SDK | Only required when `SKIP_INTEGRATION` is not `1`. Consumed by the Agent SDK inside integration tests. Never required at product runtime — Claude Code handles its own auth. |
| `PROJECT_ROOT` | path | `$PWD` (launcher auto-detect) | no | launcher project-mode, `.claude/settings.json` | Used by project-mode installs (spec §11) to rewrite hook paths to `${PROJECT_ROOT}/.edikt/hooks/*.sh`. Auto-detected by walking up from `$PWD` for `.edikt/bin/edikt`; explicit override supported for CI. |
| `PATH` | path list | OS-provided | yes | launcher discovery | Must contain `~/.edikt/bin` (global install) or Homebrew's `bin` (brew install) for `edikt` to resolve. `install.sh` prints a PATH-addition hint if missing. |

**Rejected (documented for reviewers who expect them):**

- `MAX_TEST_COST_USD` — rejected per PRD-002 Decision #4. Integration test cost is absorbed by the maintainer's Claude subscription. No enforcement variable.
- `EDIKT_VERSION` (env) — version is read from `~/.edikt/lock.yaml` `active:`, never env. Prevents drift between env and on-disk truth.

---

## 2. Feature Flags

edikt has no runtime feature-flag service. "Flags" here are config.yaml booleans and launcher env gates. `.edikt/config.yaml` `features:` block is existing (see current config); v0.5.0 adds none. The only new flag surface is `EDIKT_EXPERIMENTAL`.

| Flag | Default | Owner | Description | Rollout plan |
|---|---|---|---|---|
| `EDIKT_EXPERIMENTAL` (env) | `0` | launcher | Gate for versioned-layout launcher during phased rollout. | v0.5.0-rc.1 through v0.5.0-rc.N: flag required. v0.5.0 GA: flag removed, behavior becomes default. Documented in CHANGELOG breaking-change section. |
| `features.auto-format` (config) | `true` | PostToolUse hook | Existing. Unchanged by v0.5.0. | n/a |
| `features.session-summary` (config) | `true` | SessionStart hook | Existing. Unchanged. | n/a |
| `features.signal-detection` (config) | `true` | Stop hook | Existing. Unchanged. | n/a |
| `features.plan-injection` (config) | `true` | UserPromptSubmit hook | Existing. Unchanged. | n/a |
| `features.quality-gates` (config) | `true` | SubagentStop hook | Existing. Unchanged. | n/a |

**No new `features.*` keys** are introduced in v0.5.0. The versioning, launcher, provenance, and test-harness deliverables are not user-toggleable — they are structural.

**Existing config.yaml keys preserved:** `edikt_version`, `base`, `stack`, `paths.*`, `rules`, `features.*`, `artifacts.*`, `sdlc.*`. v0.5.0 reads `paths.*` and `stack` for init substitution (FR-025, FR-026) and reads `edikt_version` for pin-warn (FR-019). No new top-level keys required.

---

## 3. On-Disk State Files

Every file the v0.5.0 product writes outside the project tree. Paths below assume global mode; project mode uses `<project>/.edikt/` in place of `~/.edikt/` (spec §11).

| Path | Format | Owner | Lifecycle |
|---|---|---|---|
| `~/.edikt/bin/edikt` | POSIX shell script (0755) | `install.sh` / brew formula | Created on first install. Updated by `brew upgrade edikt` or a fresh `curl \| bash` run. Deleted by `edikt uninstall` or `brew uninstall edikt`. |
| `~/.edikt/versions/<tag>/` | directory tree (0755) | launcher `install` subcommand | Created on `edikt install <tag>`. Never modified after creation (immutable release payload). Deleted by `edikt prune` or `edikt uninstall`. Must never be `active` or `previous` to be pruned. |
| `~/.edikt/versions/<tag>/VERSION` | plain text (single line) | launcher `install` | Written at install-extraction time. Read by launcher for version checks. |
| `~/.edikt/versions/<tag>/CHANGELOG.md` | Markdown | launcher `install` | Bundled in the release tarball. Read-only. |
| `~/.edikt/versions/<tag>/manifest.yaml` | YAML | launcher `install` | Written at install-extraction. Read by `edikt doctor` for tamper detection (SHA256 per file). |
| `~/.edikt/versions/<tag>/templates/`, `hooks/`, `commands/edikt/` | directory trees | launcher `install` | Created at install. Read-only after install (reads only — hooks execute, templates copy). |
| `~/.edikt/versions/dev/` | directory of symlinks | launcher `dev link <path>` | Created by `edikt dev link`. Removed by `edikt dev unlink`. Target of `current` symlink only while dev-linked. |
| `~/.edikt/current` | symlink → `versions/<tag>` | launcher `use` | Created on first activation. Atomically swapped on every `use`, `upgrade`, `rollback`. Never a real directory. |
| `~/.edikt/hooks` | symlink → `current/hooks` | launcher `install` (one-time) | Stable path baked into every project's `.claude/settings.json`. Created once per install tree and never touched again. |
| `~/.edikt/templates` | symlink → `current/templates` | launcher | Same lifecycle as `hooks`. |
| `~/.claude/commands/edikt` | symlink → `~/.edikt/current/commands/edikt` | launcher `install` | Created/repaired on every `install` and `use`. Removed by `edikt uninstall`. |
| `~/.edikt/config.yaml` | YAML | user (manual) | User data. Launcher never writes. Preserved across upgrades / rollbacks / uninstalls (uninstall prompts before removing). |
| `~/.edikt/custom/` | directory tree | user (manual) | User data. Launcher never writes. |
| `~/.edikt/backups/<timestamp>/` | directory tree | launcher `use`, `upgrade`, migration | Created before every symlink flip and before every migration step. Retention: last 10, pruned FIFO. Never automatically deleted without user-visible log. |
| `~/.edikt/backups/migration-<ts>/` | directory tree | launcher migration | Pre-migration snapshot. Consumed by `edikt migrate --abort`. |
| `~/.edikt/lock.yaml` | YAML | launcher `install` / `use` / `rollback` | Created on first activation. Rewritten atomically on every version transition (write-temp + rename). |
| `~/.edikt/events.jsonl` | JSONL (append-only) | launcher + hooks | Existing. v0.5.0 adds 5 new event types (§4.4). Append-only; rotated at 10 MB (current behavior). |
| `~/.edikt/gate-overrides.jsonl` | JSONL (append-only) | SubagentStop hook | Existing. Unchanged by v0.5.0. Listed for completeness. |
| `~/.edikt/session-signals.log` | plain text (append-only) | SessionStart / Stop hooks | Existing. Unchanged by v0.5.0. Listed for completeness. |
| `test/integration/failures/<test>-<ts>.jsonl` | JSONL | pytest conftest failure hook | Written only when a Layer 2 test fails (FR-005a). Not cleaned automatically — consumed by `claude-replay`. Outside `~/.edikt/`; lives in repo. Git-ignored. |

**Ownership key:**

- **launcher** = `~/.edikt/bin/edikt` subcommands.
- **hook** = one of the 9 lifecycle hooks in `~/.edikt/hooks/*.sh`.
- **claude** = Claude Code itself (e.g., command discovery reads `~/.claude/commands/edikt`).

---

## 4. File Schemas

### 4.1 `~/.edikt/lock.yaml`

```yaml
# Active + previous version state. Authoritative source for `edikt version` and rollback.
active: "0.5.0"
previous: "0.4.3"
installed_at: "2026-04-14T10:00:00Z"   # timestamp of the last `use` that produced `active`
installed_via: "launcher"              # enum: launcher | install.sh | brew | dev
history:
  - version: "0.5.0"
    installed_at: "2026-04-14T10:00:00Z"
    activated_at: "2026-04-14T10:00:00Z"
    installed_via: "launcher"
  - version: "0.4.3"
    installed_at: "2026-04-13T08:00:00Z"
    activated_at: "2026-04-13T08:00:00Z"
    installed_via: "install.sh"
```

**Field notes:**

- `active` and `previous` are tag strings without a leading `v`. Canonical form is semver. Launcher normalizes `v0.5.0` → `0.5.0` on read.
- `previous` may be absent on a first-ever install. `edikt rollback` exits 1 when absent.
- `history[]` is append-only and capped at 50 entries (oldest trimmed).
- `installed_via: dev` marks a `dev link` activation. `rollback` skips `dev` entries when picking a target.

### 4.2 `~/.edikt/versions/<tag>/manifest.yaml`

```yaml
# Per-version payload manifest. Written once at install extraction. Read-only thereafter.
version: "0.5.0"
installed_at: "2026-04-14T10:00:00Z"
source:
  type: "tarball"                       # enum: tarball | brew | dev
  url: "https://github.com/diktahq/edikt/archive/refs/tags/v0.5.0.tar.gz"
  sha256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
files:
  - path: "hooks/session-start.sh"
    sha256: "a1b2c3d4e5f6..."
    mode: "0755"
  - path: "hooks/stop-hook.sh"
    sha256: "b2c3d4e5f6a1..."
    mode: "0755"
  - path: "commands/edikt/context.md"
    sha256: "c3d4e5f6a1b2..."
    mode: "0644"
  - path: "templates/agents/_substitutions.yaml"
    sha256: "d4e5f6a1b2c3..."
    mode: "0644"
  # one entry per payload file
```

`edikt doctor` walks this manifest and re-hashes each file; mismatches are reported as tamper warnings (exit 1).

### 4.3 `templates/agents/_substitutions.yaml`

This is a **template file shipped in the product**, not on-disk state. Lives at `~/.edikt/versions/<tag>/templates/agents/_substitutions.yaml` and is read by `/edikt:init` during agent installation.

```yaml
# Path substitution map consumed by /edikt:init (FR-025).
# Keys are substitution IDs referenced inside agent templates.
# `default` is the literal string matched in template source.
# `config_key` is the dotted path into .edikt/config.yaml whose value replaces `default`.
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
  guidelines:
    default: "docs/guidelines"
    config_key: "paths.guidelines"
  reports:
    default: "docs/reports"
    config_key: "paths.reports"
  project_context:
    default: "docs/project-context.md"
    config_key: "paths.project-context"
```

**Matching rules:**

- Replacement is literal-string, not regex. `default` strings must be unique enough to avoid collisions — these are.
- If `config_key` is unset or equals `default`, no substitution is performed for that entry.
- Substitution runs **before** the provenance hash is computed against the *source* template (per spec §7 "md5 is computed against the raw template file on disk BEFORE substitution and stack filtering"). The hash anchors the pre-substitution source.

### 4.4 `~/.edikt/events.jsonl` — new event types

One JSON object per line. Existing envelope: `{ "event": "<type>", "timestamp": "<ISO-8601>", ... }`. v0.5.0 adds five types.

```jsonl
{"event":"layout_migrated","timestamp":"2026-04-14T10:00:00Z","from":"flat","to":"versioned","version":"0.4.3","backup":"~/.edikt/backups/migration-20260414T100000Z"}
{"event":"version_installed","timestamp":"2026-04-14T10:05:12Z","version":"0.5.0","source":"tarball","sha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","installed_via":"launcher"}
{"event":"version_activated","timestamp":"2026-04-14T10:05:15Z","version":"0.5.0","previous":"0.4.3","symlink":"~/.edikt/current","backup":"~/.edikt/backups/20260414T100515Z"}
{"event":"rollback_performed","timestamp":"2026-04-14T11:30:00Z","from":"0.5.0","to":"0.4.3","reason":"user","backup":"~/.edikt/backups/20260414T113000Z"}
{"event":"migration_aborted","timestamp":"2026-04-14T10:01:00Z","step":"M4","from_version":"0.2.3","error":"compile_failed: rules/governance.md missing sentinel","restored_from":"~/.edikt/backups/migration-20260414T100000Z"}
```

**Schemas (per type):**

| Event | Required fields | Optional fields |
|---|---|---|
| `layout_migrated` | `from`, `to`, `version`, `backup` | `dry_run` (bool) |
| `version_installed` | `version`, `source`, `sha256`, `installed_via` | `url` |
| `version_activated` | `version`, `symlink` | `previous`, `backup` |
| `rollback_performed` | `from`, `to`, `backup` | `reason` (`user` / `auto-abort` / `doctor`) |
| `migration_aborted` | `step` (M1–M6), `from_version`, `error`, `restored_from` | `stderr_tail` (string) |

**Append-only contract:** hooks and launcher both write via `>>` with `flock` — see operational concerns §8.

---

## 5. CI Secrets & GitHub Actions

| Workflow | File | Triggers | Secrets consumed | Access |
|---|---|---|---|---|
| `test` | `.github/workflows/test.yml` | `pull_request`, `push` to `main`, tag `v*` | — (unit-and-sandbox job) | public; PRs from forks get unit-only |
| `test` (integration job) | same file | `push` tag `v*` only (gated by `if: github.ref_type == 'tag'`) | `ANTHROPIC_API_KEY` | repo admins only (`diktahq` org) |
| `release` | `.github/workflows/release.yml` | tag `v*` | `GITHUB_TOKEN` (default), `HOMEBREW_TAP_DEPLOY_KEY` | repo admins only |
| `docs` | `.github/workflows/docs.yml` (existing) | `push` to `main` with `website/**` changes | `GITHUB_TOKEN` | existing |
| `tap:test` | `diktahq/homebrew-edikt/.github/workflows/test.yml` | `pull_request`, `push` | — | public |

**Secret scope:**

- **`ANTHROPIC_API_KEY`** — repository secret on `diktahq/edikt`. Scope: Layer 2 integration tests only. Never exposed to PR-from-fork jobs (GitHub default: secrets withheld from fork PRs). Rotation cadence: every 90 days. Rotation owner: Daniel. No organization-level secret — scope is limited to this one repo.
- **`HOMEBREW_TAP_DEPLOY_KEY`** — deploy key on `diktahq/homebrew-edikt` with write scope, added as a secret on `diktahq/edikt` to allow `homebrew-releaser` to open PRs against the tap. Scope: release workflow only. No read access to any other repo.
- **`GITHUB_TOKEN`** — default per-workflow token. Permissions explicitly pinned to minimum required per job via workflow `permissions:` block (`contents: read` for `test`, `contents: write` for `release`).

**What no workflow holds:**

- No cloud credentials (no AWS, GCP, Cloudflare).
- No signing keys (per FR-036, binary signing is a WON'T).
- No registry credentials (no npm publish, no Docker Hub).

**Fork PR behavior:** integration tests are tag-gated, so fork PRs never attempt to use `ANTHROPIC_API_KEY`. Unit + sandbox run on fork PRs without secrets. This is intentional — fork PRs must not have a path to exfiltrate the API key.

---

## 6. Permissions & Filesystem

**Required filesystem permissions:**

| Path | Required permission | Rationale |
|---|---|---|
| `~/.edikt/` | user read + write + exec | All state lives here. |
| `~/.edikt/bin/edikt` | user read + exec (0755) | Launcher invocation. |
| `~/.edikt/versions/*/hooks/*.sh` | user read + exec (0755) | Hook invocation by Claude Code. |
| `~/.claude/commands/` | user read + write + exec | `~/.claude/commands/edikt` symlink created here. |
| `/usr/local/bin` or `$HOMEBREW_PREFIX/bin` | via brew install | Homebrew writes launcher here. Not required if using `curl \| bash`. |
| Ability to create symlinks (`ln -s`) | user | Core mechanism of the versioned layout. |
| Ability to `rename(2)` across `~/.edikt/` | user | Atomic symlink flip relies on `mv -Tf newlink current` (Linux) or equivalent rename. Must stay within same filesystem. |

**Symlink creation failure modes — where this can break:**

| Platform / filesystem | Symlink support | Behavior |
|---|---|---|
| macOS (APFS, HFS+) | yes | Works. Default path. |
| Linux (ext4, btrfs, xfs, zfs) | yes | Works. Default path. |
| Linux tmpfs | yes | Works. Relevant for CI containers. |
| WSL2 (ext4 backing) | yes | Works — same as Linux. |
| WSL1 | partial | Symlinks to Windows paths may fail; symlinks inside WSL rootfs work. `~/.edikt/` is inside rootfs, so expected to work. **Unverified in CI** (see Operational Concerns). |
| `/mnt/c` under WSL (DrvFs) | no reliable symlink | Documented: do not install edikt under `/mnt/c`. Launcher prints a warning if `$HOME` resolves under `/mnt/`. |
| Windows native (NTFS, no WSL) | N/A — Windows native is a non-goal per PRD. | N/A |
| FAT32 / exFAT | no | Detect at install time, abort with actionable error. Fringe — no user has reported installing to a removable drive. |
| Container overlayfs (Docker `COPY`) | yes (at runtime) | Works for CI. Be aware that `COPY` flattens symlinks at build time — bake the launcher install into `RUN` steps, not `COPY`. |

**Fallback behavior:** if `ln -s` fails (EPERM, EXDEV, read-only FS), launcher aborts the operation with exit 2 and a message recommending either a supported filesystem or filing an issue. No silent copy-fallback — copying a "versioned" tree as a flat directory recreates the v0.4.x problem the spec exists to eliminate.

**Permissions set by the launcher:**

- Extracted payload files: `0644` for `.md` / `.yaml`, `0755` for `.sh` (launcher reads the `mode` field in `manifest.yaml`).
- `~/.edikt/bin/edikt`: `0755`.
- `lock.yaml`, `events.jsonl`: `0644`.
- `backups/`: `0755` for dirs, contents preserve source modes.

**Umask:** launcher sets `umask 022` at entry to prevent group/world writable files regardless of the user's global umask.

---

## 7. Platform Differences

Explicit compatibility matrix. Claude Code itself is Darwin + Linux; v0.5.0 matches that surface.

| Platform | Shell | SHA256 tool | Tar | Install channel | Status |
|---|---|---|---|---|---|
| macOS 13+ (Intel) | `/bin/sh` (bash 3.2) or user zsh | `shasum -a 256` | BSD tar | brew (primary) or `curl \| bash` | supported |
| macOS 13+ (Apple Silicon) | same | `shasum -a 256` | BSD tar | brew at `/opt/homebrew/bin` | supported |
| Ubuntu 22.04+, Debian 12+ | `/bin/sh` (dash) | `sha256sum` | GNU tar | `curl \| bash` or brew (Linuxbrew) | supported |
| Fedora 40+, Arch | `/bin/sh` (bash) | `sha256sum` | GNU tar | `curl \| bash` | supported |
| Alpine (CI) | `/bin/sh` (ash/busybox) | `sha256sum` | busybox tar | `curl \| bash` | supported (CI only) |
| WSL2 (any distro) | same as Linux | `sha256sum` | GNU tar | `curl \| bash` | supported |
| WSL1 | same | `sha256sum` | GNU tar | `curl \| bash` | best-effort (not in CI matrix) |
| Linuxbrew | same as Linux | `sha256sum` | GNU tar | brew | best-effort per FR-035 |
| Windows native | N/A | N/A | N/A | N/A | unsupported per PRD non-goals |

**Launcher portability rules:**

- POSIX `sh` only. No bashisms (`[[ ]]`, `(( ))`, arrays, `$'...'`). `shellcheck -s sh` runs in `test/unit/launcher/`.
- SHA256 dispatch:
  ```sh
  if command -v sha256sum >/dev/null 2>&1; then
    SHA256="sha256sum"
  elif command -v shasum >/dev/null 2>&1; then
    SHA256="shasum -a 256"
  else
    echo "error: need sha256sum or shasum" >&2; exit 1
  fi
  ```
- Tar extraction: `tar -xzf` is portable across BSD and GNU tar. Avoid `--strip-components` flag variants; construct target path explicitly.
- Date formatting: use ISO-8601 via `date -u +%Y-%m-%dT%H:%M:%SZ` (works on both BSD and GNU `date`). No `date -d` (GNU-only) or `date -v` (BSD-only).
- Path resolution: avoid `readlink -f` (GNU-only on older macOS). Use `python3 -c "import os,sys;print(os.path.realpath(sys.argv[1]))"` only if absolutely required; otherwise walk symlinks manually.

**CI matrix:** `.github/workflows/test.yml` `unit-and-sandbox` job runs on `ubuntu-latest` + `macos-latest`. Layer 2 runs only on `ubuntu-latest` (cost optimization — SDK behavior is platform-independent).

---

## 8. Operational Concerns (SRE findings)

Real risks introduced by this design, with the mitigation the launcher implementation must honor.

### 8.1 Lock contention — two `edikt` processes running concurrently

**Risk:** User runs `edikt upgrade` in one terminal while an agent session triggers `edikt rollback` via a hook. Both rewrite `lock.yaml` and flip `current`. Outcomes range from interleaved `history[]` entries to a dangling symlink.

**Mitigation:**

- Every launcher subcommand that mutates state acquires `~/.edikt/.lock` via `flock -n 9` (Linux) or `shlock` fallback (macOS if `flock` absent; Homebrew's `util-linux` provides `flock` on macOS but we cannot assume it).
- If lock acquisition fails: exit 4 with "another edikt process is running (pid from `.lock`). Retry or remove stale lock."
- Lock file contains the PID + ISO timestamp. `edikt doctor` detects stale locks (PID no longer alive) and offers to clear.
- Read-only subcommands (`version`, `list`, `doctor` without `--fix`) do not acquire the lock.

### 8.2 Corruption risk — launcher killed mid-migration

**Risk:** User runs `edikt upgrade`, launcher moves `hooks/` into `versions/0.4.3/hooks/`, then the terminal is killed before the `current` symlink is created. `~/.edikt/hooks` no longer resolves; every project breaks.

**Mitigation:**

- Migration writes a pre-migration snapshot to `~/.edikt/backups/migration-<ts>/` **before** any mutation (spec §10 "Every migration step writes a backup before acting").
- Migration is staged: (1) build full new layout in `~/.edikt/.staging-<ts>/`, (2) atomic rename to `~/.edikt/versions/<tag>/`, (3) flip symlinks via `ln -sfn` (atomic on single-rename systems).
- Launcher installs an `EXIT` + `INT` + `TERM` trap that emits `migration_aborted` to `events.jsonl` and runs `edikt migrate --abort` implicitly if we're past step 1 but before step 3.
- `edikt doctor` on every invocation detects a partial migration (staging dir present + `current` unresolved) and prompts to resume or abort.

### 8.3 `current` symlink points to a pruned version

**Risk:** `edikt prune --keep 1` while `current` points at an older version that also happens to be `active`. Launcher must refuse.

**Mitigation:**

- Prune logic explicitly reads `lock.yaml` and excludes `active` and `previous` from deletion candidates, regardless of age.
- Post-prune, `edikt doctor` verifies `readlink current` still resolves — if not, launcher exits 2 and recommends `edikt install <active> && edikt use <active>` from the release archive.
- Guardrail test in Layer 1: `test_prune_never_deletes_active.sh`.

### 8.4 Append races on `events.jsonl`

**Risk:** Hook and launcher both append to `events.jsonl` concurrently. On Linux, `O_APPEND` guarantees atomicity per `write(2)` up to `PIPE_BUF` (4096 bytes). On macOS, atomicity holds for writes that fit in the filesystem block. Our events are well under that — but a malformed hook that writes multi-line JSON would tear.

**Mitigation:**

- Contract: every event is a **single line** terminated with `\n`, emitted via one `printf '%s\n' "$json"` call. Hooks lint-tested to enforce.
- Rotation: at 10 MB, launcher atomically renames `events.jsonl` → `events.jsonl.<ts>` and creates a fresh file. Rotation takes the `flock` from §8.1.

### 8.5 Homebrew upgrade ≠ payload upgrade (FAQ Q4)

**Risk:** User runs `brew upgrade edikt`, launcher version bumps (say from 0.5.0 launcher to 0.5.1 launcher), but `~/.edikt/versions/` still contains only 0.5.0 payload. New launcher features that expect new payload behaviors fail silently.

**Mitigation:**

- Launcher embeds `MIN_PAYLOAD_VERSION` constant. On every invocation, compares against `lock.yaml:active`. If active payload is older, emits a one-time warning: "Launcher is 0.5.1 but payload is 0.5.0. Run `edikt upgrade`."
- FAQ entry (§12.5 of spec) explicitly calls out the distinction.
- Release automation keeps launcher and payload versions in lockstep for the first year — do not bump launcher alone except for launcher-only bug fixes (documented as patch-version bumps with no payload change required).

### 8.6 `dev link` leaves `current` dangling on `git clean`

**Risk:** Maintainer runs `edikt dev link ~/src/edikt`, then `git clean -fdx` in that repo deletes the symlink targets.

**Mitigation:**

- `edikt dev link` records the source path in `lock.yaml:history[].dev_source`.
- `edikt doctor` detects broken dev-link targets and suggests `edikt dev unlink && edikt use <latest>`.
- `edikt dev unlink` always succeeds even if source is gone (removes symlinks only).

### 8.7 First-invocation bootstrap on brew install race

**Risk:** Brew formula puts `edikt` on `$PATH`. User runs two `edikt` commands in parallel shell tabs immediately after install. Both race to create `~/.edikt/`. Mitigated by §8.1 but worth calling out: the first-run bootstrap is the single most contested state transition.

**Mitigation:**

- Bootstrap wraps *everything* after `~/.edikt/` exists in the `flock`. If two processes race to create the directory, `mkdir -p` is idempotent; whichever wins the `flock` next completes initialization; the other sees a populated state and no-ops.

### 8.8 Symlink flip on NFS / network home directories

**Risk:** Corporate setups with NFS-mounted `$HOME`. Symlink semantics hold, but `flock` over NFS is historically unreliable. Cross-filesystem `rename(2)` fails with `EXDEV`.

**Mitigation:**

- `flock` fallback: if `flock` returns `ENOLCK`, fall back to a lockfile + `ln -s` lock (create-link is atomic on NFS). Documented in launcher comments.
- Renames are always within `~/.edikt/`, which lives on one filesystem (the user's home). `EXDEV` only possible if `~/.edikt/.staging-<ts>/` is on a different FS — we enforce colocating it inside `~/.edikt/`.
- Not in CI matrix. Documented as "best effort" in FAQ.

### 8.9 Events.jsonl exposure of tag metadata

**Risk:** `version_installed` records full tarball URL and SHA. Low-severity info disclosure if a user pastes `events.jsonl` for debugging.

**Mitigation:**

- All recorded data is public (GitHub release URLs are public). No secret material.
- Documented: `events.jsonl` contains no secrets; safe to share. Pre-empts a future request to add API keys to event records.

### 8.10 INV-001 compliance audit

Every artifact in §3 and §4 is `.yaml`, `.md`, `.json`, `.jsonl`, shell script, or symlink. No binary formats. No compiled schemas. Passes INV-001.

---

## Summary

The v0.5.0 configuration surface adds one new template file (`_substitutions.yaml`), one new state file (`lock.yaml`), one per-version manifest (`manifest.yaml`), and five new `events.jsonl` event types. Environment surface gains three controllable variables (`EDIKT_EXPERIMENTAL`, `SKIP_INTEGRATION`, `EDIKT_HOME` formalization) and one test-only secret (`ANTHROPIC_API_KEY`). No new `config.yaml` keys. The dominant SRE risks — concurrent lock contention, mid-migration corruption, and cross-filesystem symlink flips — all have explicit mitigations that belong in the launcher implementation and must be covered by Layer 1 tests before the versioning bundle is release-ready.
