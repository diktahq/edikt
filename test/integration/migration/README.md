# Migration test harness

Pytest tests that exercise the launcher's multi-version migration chain
(M1-M6 in `bin/edikt`).

## Layout

```
test/integration/migration/
├── README.md            # this file
├── capture.sh           # captures historical edikt installs as fixtures
├── conftest.py          # pytest fixtures (sandbox, synthetic source layouts)
├── pytest.ini           # pytest config (lives one level up at integration/pytest.ini for now)
├── fixtures/            # captured frozen historical snapshots (Phase 7b populates)
│   └── <tag>/
│       ├── edikt/       # mirror of $HOME/.edikt/ at install time for <tag>
│       ├── commands/    # mirror of $HOME/.claude/commands/edikt/ at install time
│       └── manifest.txt # sha256sum lines for every file
└── test_v0XX_to_v050.py # one test file per source version
```

## What `capture.sh` does

For each git tag passed on the command line:

1. `git worktree add /tmp/edikt-capture-<tag> <tag>`
2. Sandbox `$HOME` under a per-tag `mktemp -d`
3. Run that tag's `install.sh --global --yes` inside the sandbox
4. Rsync `$HOME/.edikt/` and `$HOME/.claude/commands/edikt/` into
   `fixtures/<tag>/`
5. Replace absolute `$HOME` references with the literal `${HOME}` placeholder
6. Zero timestamps with `touch -t 197001010000` so re-captures are
   byte-stable
7. Write `manifest.txt` containing sha256s of every file
8. Remove the worktree and per-tag sandbox

## How to run

```sh
./capture.sh v0.1.0 v0.1.4 v0.2.0 v0.3.0 v0.4.3
```

Re-runs are idempotent: the script removes the prior worktree and prior
fixture directory before re-creating them. Two consecutive runs against
the same tag produce byte-identical fixture trees.

## Why CI does NOT run capture.sh

- **Network-dependent.** Historical `install.sh`s reach out for templates
  and dependencies; CI runs need to be hermetic.
- **Reproducibility risk.** The captured bytes drift if the historical
  install path drifts (e.g. a transitive download URL flips). We lock
  the fixtures in git so tests have something deterministic to replay.
- **Capture is a release-engineering activity, not a test.** It happens
  once per tag, by hand, in Phase 7b.

The tests in this directory replay against the **committed** fixtures
(once Phase 7b commits them). Until then, Phase 7a's tests run against
**synthetic fixtures** built inline in `conftest.py` — shape-correct
mock layouts that carry the right detection signals per source version
without requiring real captured bytes.

## Phase 7a vs Phase 7b

| Item                                 | 7a    | 7b    |
|--------------------------------------|-------|-------|
| capture.sh script committed          | ✓     |       |
| capture.sh executed                  |       | ✓     |
| Real fixtures committed              |       | ✓     |
| Synthetic fixtures (in conftest.py)  | ✓     |       |
| M2/M3/M5 migration code              | ✓     |       |
| M4 stub (records intent)             | ✓     |       |
| M4 wired to `claude -p /edikt:gov:compile` |  | ✓     |
| pytest scaffolding (conftest, ini)   | ✓     |       |
| Five `test_v*_to_v050.py` files      | ✓     |       |

7b will replace the synthetic fixtures with the captured ones in tests
that benefit from byte-for-byte realism, and will wire M4 to the
compile invocation it currently stubs.
