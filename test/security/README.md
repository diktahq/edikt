# test/security/

Regression tests that pin every finding from
`docs/reports/security-audit-v0.5.0-2026-04-17.md`. Each test maps to
a specific audit finding ID so future refactors cannot silently re-open
a Critical or High without a corresponding test failure.

## Layout

| File | Pins |
|---|---|
| `lints.sh`        | INV-003, INV-004, INV-008 — grep-based linters |
| `inputs.sh`       | INV-006 — externally-controlled input validators |
| `sentinel.sh`     | INV-005 / HI-4 — byte-range sentinel guard |
| `sandbox.sh`      | INV-007 / HI-10, HI-11 — hermetic benchmark sandbox |
| `permissions.sh`  | ADR-017 / HI-9, LOW-7 — default `permissions` block |
| `evaluator.sh`    | ADR-018 / HI-7 — verdict schema shape + evidence gate |
| `release.sh`      | ADR-016 / CRIT-6, CRIT-7 — cosign/signing wiring |
| `hooks.sh`        | INV-003, INV-004 / CRIT-1,2,4,5 — adversarial hook inputs |
| `run.sh`          | Entry point — runs every `*.sh` above, exits non-zero on first fail |

## Running

```bash
./test/security/run.sh             # run all security tests
./test/security/lints.sh           # run one specifically
```

## CI

`.github/workflows/test.yml` invokes `./test/security/run.sh` on every PR.
Failures block merge. Cosign-dependent tests skip gracefully when cosign
is not installed.

## Why these tests exist

The v0.5.0 security audit surfaced 48 findings (7 Critical, 14 High,
14 Medium, 13 Low). Every fix landed in Phases 1-13 of
`docs/product/plans/PLAN-v0.5.0-security-hardening.md`. These tests
exist so that a refactor, dependency bump, or contributor PR that
re-introduces any of the fixed classes fails loudly before it merges.

If you're adding a new fix that plugs a new class of finding, add a
test here too. If you're refactoring a hook or a validator and a test
here breaks, fix the code — do NOT adjust the test to match. These
tests encode security contract, not implementation detail.
