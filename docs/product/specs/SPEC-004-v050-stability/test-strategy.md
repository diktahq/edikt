---
type: artifact
artifact_type: test-strategy
spec: SPEC-004
status: in-progress
reviewed_by: qa
created_at: 2026-04-14T00:00:00Z
---

# Test Strategy — v0.5.0 Stability Release

This release *is* the test infrastructure. The strategy below is how we test that infrastructure itself. Product code (launcher, hooks, commands, templates) stays pure `.md` / `.yaml` / `bash` per INV-001. Test harness code (pytest, Agent SDK) lives in `test/` and never ships.

## Testing Boundaries

- **Layer 1 — Hook unit tests (Bash + BATS-style):** no Claude required, fast, deterministic.
- **Layer 2 — Agent SDK integration (Python + pytest):** real Claude, fuzzy-match snapshots, gated on `ANTHROPIC_API_KEY`.
- **Layer 3 — Sandboxed runner:** launcher, migration, and Homebrew behavior in ephemeral `$HOME` (tmp dir overriding `HOME`, `XDG_*`, `PATH`).

Everything that touches the user's real `~/.edikt/` is forbidden outside Layer 3 sandboxes.

## Unit Tests

### Launcher subcommands (Layer 3 — sandboxed shell)

| Component | What to test | Priority |
|---|---|---|
| `edikt install <version>` | creates `~/.edikt/versions/<v>/`, writes `manifest.yaml`, flips `current` symlink atomically | high |
| `edikt install` (rerun) | idempotent — no duplicate version dir, no symlink churn, exit 0 | high |
| `edikt install` (corrupt tarball) | SHA256 mismatch vs `manifest.yaml` aborts before symlink flip; leaves previous `current` intact | high |
| `edikt use <version>` | retargets `current` symlink; fails loudly if version dir missing | high |
| `edikt use <missing>` | non-zero exit, actionable error naming `edikt list` | medium |
| `edikt upgrade` | fetches latest, installs, calls `use`, runs post-install migration check | high |
| `edikt upgrade --pin <v>` | respects pin; subsequent `upgrade` is a no-op when pinned | high |
| `edikt upgrade-pin clear` | removes pin; next `upgrade` proceeds | medium |
| `edikt rollback` | flips `current` to previous generation; errors if no prior generation exists | high |
| `edikt list` | prints every dir under `versions/`, marks active, sorted semver | medium |
| `edikt prune` | keeps N most recent + active + pinned; never deletes `current` target | high |
| `edikt prune --dry-run` | prints deletions, touches nothing | medium |
| `edikt doctor` | reports symlink health, config schema version, PATH placement, write perms; non-zero on any failure | high |
| `edikt uninstall` | removes `~/.edikt/` only after explicit confirm; leaves project-mode installs alone | high |
| `edikt dev link <path>` | symlinks `current` into a working tree for local dev | medium |
| `edikt dev unlink` | restores last real generation | medium |
| `edikt version` | prints resolved generation + launcher version + manifest SHA | medium |

Each subcommand gets at least one happy-path test and one failure-path test.

### Hook unit tests (Layer 1)

| Component | What to test | Priority |
|---|---|---|
| `hooks/post-tool-use/format.sh` | formats known extensions (`.ts`, `.py`, `.go`, `.rs`, `.rb`); skips unknown | high |
| `hooks/post-tool-use/format.sh` | `EDIKT_FORMAT_SKIP=1` short-circuits; config `format.enabled: false` short-circuits | high |
| `hooks/post-tool-use/format.sh` | missing formatter binary exits 0 silently (does not block Write) | high |
| `hooks/stop/phase-end-eval.sh` | fires evaluator only when plan phase transitions; no-op otherwise | high |
| `hooks/pre-tool-use/plan-guard.sh` | blocks interactive commands inside plan mode with actionable message | high |
| `hooks/post-compact/reinject.sh` | re-injects active plan phase + invariants; tolerates missing plan | high |
| `hooks/user-prompt-submit/intent-match.sh` | natural-language triggers route to the right `/edikt:*` command | medium |
| All hooks | exit 0 when `.edikt/` is absent (don't break non-edikt projects) | high |
| All hooks | tolerate malformed `config.yaml` — log, don't crash | high |

Fixtures: a `test/fixtures/projects/` tree with (a) no `.edikt/`, (b) minimal `.edikt/`, (c) fully-configured `.edikt/`, (d) corrupt `.edikt/config.yaml`.

### Init provenance

| Component | What to test | Priority |
|---|---|---|
| Path substitution | `{{paths.decisions}}` etc. resolve from `config.yaml`; no unsubstituted `{{...}}` in output | high |
| Stack filter | `stack: [ts, react]` installs only matching rule tiers; `base` always included | high |
| `edikt_template_hash` frontmatter | SHA256 of source template embedded on every generated file | high |
| Agent provenance | generated agents carry `edikt_template_hash`; `<!-- edikt:custom -->` marker absent by default | high |
| Overrides | `.edikt/templates/` override wins; hash reflects the override source, not the default | high |

### Upgrade provenance-first flow

| Component | What to test | Priority |
|---|---|---|
| Preserve-on-unchanged | user file hash == `edikt_template_hash` and template unchanged → skip silently | high |
| Overwrite-safe | user file hash == `edikt_template_hash` and template changed → overwrite, update hash | high |
| 3-way diff | user-modified + template-moved → present 3-way diff; never silent overwrite (regression guard) | high |
| Custom marker | file carries `<!-- edikt:custom -->` → always skip, even on template change | high |
| Missing hash | legacy file with no hash → treated as user-modified, 3-way diff path | high |

## Integration Tests

### Layer 2 — Agent SDK flows (Python + pytest, fuzzy-match snapshots)

| Scenario | Components involved | Priority |
|---|---|---|
| `/edikt:init` on empty project | command + templates + hooks registration | high |
| `/edikt:sdlc:plan` full flow | plan command + phase table + progress tracking | high |
| `/edikt:adr:new` → `/edikt:adr:compile` | adr creation + sentinel generation + governance compile | high |
| `/edikt:sdlc:spec` → `/edikt:sdlc:artifacts` | spec + artifact generation chain | high |
| `/edikt:sdlc:drift` | drift detection against committed spec | medium |
| `/edikt:capture` mid-session | capture command state flush | medium |
| `/edikt:upgrade` from pinned prior version | upgrade command + migration + provenance preservation | high |
| `/edikt:doctor` on broken install | doctor detects + reports missing symlink / stale hash | high |

**Fixture projects** live under `test/fixtures/projects/`:
- `empty/` — no `.edikt/`
- `ts-react/` — TypeScript + React stack
- `py-fastapi/` — Python + FastAPI stack
- `go-service/` — Go service
- `legacy-v014/` — a real v0.1.4 install (frozen)

**Fuzzy-match strategy:** snapshots stored as Markdown with structural placeholders (`{{TIMESTAMP}}`, `{{ADR_NUMBER}}`, `{{HASH}}`). Matcher compares tokenized structure + ADR/INV IDs + key headings; ignores whitespace, cosmetic wording drift, and model-side phrasing. Snapshots are regenerated only via explicit `--update-snapshots`, never automatically. Snapshot drift between Claude model versions is surfaced, not hidden (see Edge Cases).

### Migration tests (Layer 3)

One frozen fixture per source version, committed under `test/fixtures/installs/`. Migration is tested end-to-end — fixture in, migrated `~/.edikt/` out, assertions on final state.

| Scenario | Components involved | Priority |
|---|---|---|
| M1: v0.1.0 → v0.5.0 | sentinel format migration (HTML → visible), dir layout | high |
| M2: v0.1.4 → v0.5.0 | rule pack restructure, agent governance fields added | high |
| M3: v0.2.0 → v0.5.0 | intelligent-compile rule file topic grouping | high |
| M4: v0.3.0 → v0.5.0 | harness/evaluator artifacts preserved | high |
| M5: v0.4.3 → v0.5.0 | no-op structurally; symlink + manifest adoption only | high |
| M6: Resume interrupted migration | re-run after simulated crash between M3 and M5 steps | high |

Each migration test asserts: (a) user content preserved, (b) `edikt_template_hash` present on generated files, (c) custom-marked files untouched, (d) config schema upgraded with no unknown-field loss, (e) rollback path viable.

### Project-mode parity

| Scenario | Components involved | Priority |
|---|---|---|
| Project-mode install mirrors global layout | `<project>/.edikt/versions/`, symlink, manifest parity | high |
| `edikt doctor` in project mode | reports project generation separate from global | high |
| Global + project coexistence | project generation wins for that repo; global untouched | high |
| Project upgrade independent of global | pinning in project doesn't affect `~/.edikt/` | high |
| Shared fixture matrix | every Layer 2 scenario above runs in both global and project mode | high |

### Homebrew distribution (Layer 3)

| Scenario | Components involved | Priority |
|---|---|---|
| `brew audit --strict --online` on tap formula | formula syntax + metadata | high |
| `brew install --HEAD` from tap | launcher lands in `PATH`, runs `edikt version` | high |
| `brew uninstall` | removes launcher only; never touches `~/.edikt/` | high |
| Upgrade via `brew upgrade` | new launcher, existing generations untouched | high |
| Reinstall after manual `rm -rf ~/.edikt/` | `brew reinstall` + `edikt install` restores cleanly | medium |

### Regression museum backfill

One test per shipped regression — named after the version that introduced it:

| Scenario | What the test locks in | Priority |
|---|---|---|
| `regression_v040_rule_pack_overwrite` | reinstall does not silently clobber user-edited rule packs | high |
| `regression_v042_spec_preprocess_order` | plan pre-flight runs before spec preprocessing | high |
| `regression_v042_audit_jump_target` | audit command resolves correct jump target on renamed files | high |
| `regression_v043_evaluator_phase_end` | evaluator auto-fires on phase completion | high |
| `regression_v043_agent_diff_classification` | upgrade classifies agent diffs; no silent overwrite on customized agents | high |

### Docs sanity

| Scenario | What the test verifies | Priority |
|---|---|---|
| Install snippet consistency | README + website + `install.sh` reference the same command string | medium |
| Version references | no doc references an unreleased version; current version matches `manifest.yaml` | medium |
| Homebrew tap instructions | match the actual tap name and formula | medium |
| Command inventory | every command in `commands/` is listed in CLAUDE.md template and website index | medium |

## Edge Cases

- **Partial install interrupted mid-download.** Tarball extraction must stage to `versions/<v>.partial/` and only rename after SHA256 matches `manifest.yaml`. Kill `edikt install` mid-extract; assert no `versions/<v>/` dir exists and `current` symlink still points at the previous good generation.
- **Broken symlink.** Delete `~/.edikt/versions/<current-target>/` out from under a live `current` symlink. `edikt doctor` must detect a dangling link, exit non-zero, and suggest `edikt use <list-item>` or `edikt install`. No launcher subcommand may crash with a raw shell error.
- **Filesystem without symlinks.** On `tmpfs` or Windows-mounted volumes where `ln -s` fails, launcher must fall back to a `current` pointer file (plain text holding the generation name) and warn loudly. Covered by a sandbox that forces `ln -s` to fail.
- **Older config schema.** `~/.edikt/config.yaml` written by v0.1.x must be auto-migrated or, if auto-migration is not possible, `doctor` prints a precise diff and `upgrade` refuses until resolved. Never silently drop unknown fields.
- **Concurrent invocations racing on `lock.yaml`.** Two `edikt install` processes started simultaneously: one acquires the lock, the other waits or exits with a clear "another edikt process is running" message. Assert no half-written manifest, no split-brain `current` symlink.
- **Migration interrupted between M3 and M5.** Simulate a crash with a partial marker file. Re-running `edikt upgrade` must detect the marker and resume from M4, not restart from M1. Assert final state matches a clean-path migration byte-for-byte (excluding timestamps).
- **CI API key missing.** Layer 2 tests must fail loudly (`pytest` fails with a clear message) when `ANTHROPIC_API_KEY` is unset on CI. Silent skips are banned — a flaky test suite trains engineers to ignore failures. Local runs may `xfail` with a visible warning banner.
- **Claude Agent SDK version drift.** Pin the SDK in `test/requirements.txt`. A nightly job bumps the pin on a branch and runs the full Layer 2 suite; snapshot deltas surface as a PR diff, not an auto-merge. Document which phrasings are load-bearing vs. cosmetic in each snapshot so drift is triageable.
- **Rollback after partial migration.** `edikt rollback` immediately after an interrupted `upgrade` must restore the prior generation and its matching config schema, not a mixed state.
- **`$HOME` is a symlink.** Some corporate macOS setups symlink `/Users/<x>` into `/home/<x>`. Launcher must resolve with `realpath` consistently so the `current` symlink and manifest paths don't diverge.
- **User PATH shadowing.** A stale `edikt` binary earlier on PATH (e.g., from a previous brew tap) must be detected by `edikt doctor` and flagged.

## Coverage Target

Numeric coverage is a weak signal for a markdown/shell product. The release ships when all of the following hold:

- **Launcher:** every subcommand has at least one happy-path test and one failure-path test. `doctor` additionally has a test per diagnostic it emits.
- **Hooks:** every hook has unit coverage for (a) its happy path, (b) its disable switch, (c) its absent-`.edikt/` no-op, (d) its malformed-config tolerance.
- **Migrations:** M1–M6 each have a green end-to-end test from a frozen fixture. Interrupted-migration resume (M3→M5) has a dedicated test.
- **Provenance:** every generated file type (rules, agents, CLAUDE.md block, hooks registry) has a test asserting `edikt_template_hash` correctness, preserve-on-unchanged, and 3-way-diff-on-moved behavior.
- **Project-mode:** the full Layer 2 matrix runs in both global and project modes; parity is enforced by running the same assertions.
- **Regression museum:** one named test per regression in v0.4.0, v0.4.2, and v0.4.3. New regressions caught during v0.5.0 RC get a test before the fix merges.
- **Homebrew:** `brew audit --strict` green; `brew install --HEAD` + `edikt version` green on macOS runners (Intel + Apple Silicon).
- **Docs sanity:** grep-based checks pass in CI for install snippets and version references.
- **Flake budget:** zero. A test that flakes twice in a week is quarantined and fixed before the next RC, not muted.
- **Snapshot drift:** Layer 2 snapshot churn is reviewed by a human on every PR; auto-regeneration is never enabled in CI.

Release is blocked if any high-priority row above lacks a passing test, or if the regression museum is missing a test for any shipped-and-fixed regression.
