# Governance Benchmark — v0.6.0 Baseline

**Status: deferred — see reason below.**

## Why the baseline is deferred

The v0.6.0 governance benchmark shipped with 17 ADRs and 2 invariants (15 + 2 = 17 total
governance artifacts). However, none of them had a `behavioral_signal` block populated at
ship time — because `behavioral_signal` is a *new* field introduced in SPEC-005 and
populated through `/edikt:adr:review --backfill` (also new in v0.6.0).

When `/edikt:gov:benchmark` ran against this repo at v0.6.0 tag time, every directive was
reported as `[SKIP] {id} — no behavioral_signal`. The benchmark exited 0 with 0 testable
directives. This is the correct, expected behavior — the benchmark is functional, the
governance catalog simply hasn't been migrated yet.

**This is not a gap in the tooling. It is the expected v0.6.0 pre-migration state.**

## How to capture the real baseline

1. Run `/edikt:adr:review --backfill` against this repo. This populates `canonical_phrases`
   and `behavioral_signal` for each ADR interactively. Approve the proposed phrases per ADR.

2. After backfill: run `/edikt:gov:benchmark --yes --model claude-opus-4-7` to get the first
   meaningful adversarial pass rate.

3. Copy the resulting `docs/reports/governance-benchmark-{ISO}/summary.json` into this
   directory as the real baseline. The CHANGELOG note for v0.6.0 uses the placeholder
   `{deferred — see docs/reports/governance-benchmark-baseline/README.md}` until this step
   is done.

## CHANGELOG note update

Once the real baseline is captured, update CHANGELOG.md under `## v0.6.0` → `**Baseline:**`
to replace the deferred placeholder with the actual pass rate (e.g., `14/17 directives hold`).

## Artifact contract

This directory (`docs/reports/governance-benchmark-baseline/`) is explicitly NOT matched by
the `.gitignore` pattern `docs/reports/governance-benchmark-*/`. It is committable. The
pattern is:

```
docs/reports/governance-benchmark-*/
!docs/reports/governance-benchmark-baseline/
```

Per AC-015b in SPEC-005, the benchmark install adds these two lines to `.gitignore` when
the first benchmark run creates a `docs/reports/governance-benchmark-{ISO}/` directory.
The baseline path stays committed so teams can track regression across releases.
