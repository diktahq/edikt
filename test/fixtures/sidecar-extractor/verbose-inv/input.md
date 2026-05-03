---
type: invariant
id: INV-099
title: "INV-099 — Hermetic Test Sandboxes"
status: active
date: 2026-05-03
---

# INV-099 — Hermetic Test Sandboxes

## Statement

Test sandboxes MUST be hermetic and MUST NOT copy host credentials, shell rc files, or SSH keys into the sandbox environment.

## Rationale

Non-hermetic sandboxes leak host state into test runs, making tests environment-dependent and creating credential exposure risk. A test that passes on the author's machine but fails in CI because it relied on a host `.ssh/` key is a test that cannot be trusted.

## Enforcement

Before running any test that spawns a subprocess Claude session, verify that `setting_sources` is restricted to `["project"]` only.
CI grep MUST assert `grep -rn 'setting_sources=\[.*user' test/` returns zero matches.

## Examples

A hermetic sandbox provides only a curated minimal `settings.json` written by the test harness, with no `hooks` key and no user-global entries.

## Anti-patterns

- Calling `shutil.copytree(os.path.expanduser('~/.claude'), sandbox_dir)` — copies all host settings.
- Passing `setting_sources=["project", "user"]` — includes user-global hooks in the sandbox.
