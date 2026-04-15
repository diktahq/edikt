# Test Migration Notes — v0.5.0 Phase 5 Hardening

Assertions deleted from legacy suites in this phase, and where equivalent
coverage now lives.

## Retired install.sh-content assertions

All assertions below grepped `install.sh` for strings that were present in the
pre-v0.5.0 payload-enumeration installer. The v0.5.0 bootstrap delegates
all payload work (backups, commands, hooks, agents, templates) to `bin/edikt`
via the versioned layout. Equivalent coverage lives under:

- `test/unit/launcher/` — launcher subcommand unit tests
- `test/integration/install/` — install.sh bootstrap integration tests

| Suite | Retired assertion | New home |
|---|---|---|
| `test-security.sh` | install_file backup function | `test/unit/launcher/test_install_happy.sh` |
| `test-security.sh` | BACKUP_DIR / backups/ dir | `test/unit/launcher/test_install_happy.sh` |
| `test-security.sh` | edikt:custom marker | `test/unit/launcher/test_install_happy.sh` |
| `test-security.sh` | "Existing edikt installation detected" | `test/unit/launcher/test_install_happy.sh` |
| `test-security.sh` | "Backed up" count | `test/unit/launcher/test_install_happy.sh` |
| `test-e2e.sh` | install_file / BACKUP_DIR / edikt:custom | `test/unit/launcher/test_install_happy.sh` |
| `test-e2e.sh` | headless-ask, evaluator, stop-failure, task-created, cwd-changed, file-changed | `test/unit/launcher/test_install_happy.sh` |
| `test-v021-regressions.sh` | V01_MOVED_COMMANDS list | `test/unit/launcher/test_migrate_yes.sh` |
| `test-v021-regressions.sh` | _fetch helper / --retry / --max-time / empty download | `test/integration/install/test_fresh_install.sh` |
| `test-v022-regressions.sh` | BACKUP_COUNT arithmetic expansion | `test/unit/launcher/test_install_happy.sh` |
| `test-v030-phase1.sh` | templates/examples shipped by installer | `test/unit/launcher/test_install_happy.sh` |
| `test-v030-phase1.sh` | guideline namespace loop | `test/unit/launcher/test_install_happy.sh` |
| `test-v030-phase6.sh` | templates/examples/invariants shipped by installer | `test/unit/launcher/test_install_happy.sh` |
| `test-v031-team-consolidation.sh` | FLAT_COMMANDS includes config, team removed | `test/unit/launcher/test_install_happy.sh` |
| `test-hooks.sh` | install.sh includes sdlc namespace (x2) | `test/unit/launcher/test_install_happy.sh` |
| `test-hooks.sh` | install.sh includes gov namespace | `test/unit/launcher/test_install_happy.sh` |
| `test-sync.sh` | install.sh includes gov namespace (covers sync) | `test/unit/launcher/test_install_happy.sh` |
| `test-version.sh` | install.sh downloads VERSION + CHANGELOG.md | `test/unit/launcher/test_install_happy.sh` |
| `test-quality.sh` | install.sh includes headless-ask hook | `test/unit/launcher/test_install_happy.sh` |

## New tests added in this phase

| File | Covers |
|---|---|
| `test/integration/install/test_launcher_checksum.sh` | Finding #1 — launcher sidecar integrity verification |
| `test/integration/install/test_launcher_version_extraction.sh` | Finding #4 — launcher_script_version awk extraction |
