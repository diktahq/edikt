# hook-corpus-coverage self-test fixtures

Two matrices with known verdicts, so `hook-corpus-coverage.sh --self-test` can
prove the check fails when it should as well as passes when it should (INV-014).

- `instructions-loaded/` — all four axes tagged; no finding class applies to an
  `InstructionsLoaded` injector, so it is COVERED.
- `cwd-changed/` — payload and config only, and no F-041 scenario even though a
  `CwdChanged` payload carries a path. Expected verdict: GAP.
- `event-log/` — a `class: library` row. Expected verdict: category error, not
  a gap; the matrix here exists only to prove the library check fires BEFORE the
  matrix is read.

`post-compact` deliberately has no directory at all: the fourth assertion is
that a hook with no matrix reads as unmeasured rather than as clean.
