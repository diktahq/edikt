# edikt verify

Run a plan's criteria sidecar verifications and produce a structured pass/fail report. `edikt verify` is a tier-2 binary subcommand (delivered by the Go `edikt` binary) and the structural gate that `/edikt:sdlc:plan` invokes before flipping a phase row from `evaluating` to `done`.

## Usage

```bash
edikt verify <plan-id>                    # run every phase's criteria
edikt verify <plan-id> --phase 4b         # run a single phase
edikt verify <plan-id> --json             # emit the JSON report to stdout
edikt verify <plan-id> --allow-failures   # exit 0 even when criteria fail
```

`<plan-id>` is the slug from `PLAN-<plan-id>-criteria.yaml` — for example, `sidecar-architecture` or `v0.6.0-rc1`. The runner walks up from the working directory looking for `.edikt/config.yaml` and resolves `paths.plans` to find the criteria sidecar, falling back to `docs/internal/plans/`, `docs/plans/`, and `docs/product/plans/`.

## Flags

| Flag | Description |
|---|---|
| `--phase <id>` | Run only the named phase. Accepts numeric (`4`) or numeric-with-suffix (`4b`, `12c`). |
| `--json` | Emit the full JSON report to stdout in addition to writing it to disk. Suppresses the human-readable progress lines. |
| `--allow-failures` | Suppress exit-1 on failures or timeouts (failures are still recorded in the report). Used by `/edikt:sdlc:plan` to surface failures without blocking — the plan command makes the gating decision. |

## Exit codes

| Code | Meaning |
|---|---|
| `0` | All executed criteria passed (or only `skipped` / `informational` results). |
| `1` | At least one criterion failed or timed out. Suppressed by `--allow-failures`. |
| `2` | Criteria sidecar missing or YAML malformed. |
| `3` | Invalid argument (unknown plan-id, malformed `--phase`, etc.). |

## Report file

Every run writes a report to:

```
.edikt/state/verify/PLAN-<plan-id>-<phase-or-all>-<timestamp>.json
.edikt/state/verify/PLAN-<plan-id>-<phase-or-all>-<timestamp>.txt
```

The JSON report carries `plan_id`, `phase` (or `all`), `git_sha` (HEAD short-sha, suffixed `-dirty` if the working tree has uncommitted changes), `summary` (counts), and `results[]` with per-criterion `id`, `statement`, `status` (`passed | failed | timeout | skipped_operational | skipped_informational`), `duration_ms`, captured `stdout` / `stderr` excerpts, and the `verify:` command that ran.

## Integration with `/edikt:sdlc:plan`

When a phase reaches `evaluating`, `/edikt:sdlc:plan` invokes:

```bash
edikt verify <plan-id> --phase <N> --allow-failures
```

It reads the report, gates the row-flip on `summary.failed == 0 && summary.timeout == 0`, and surfaces every failure with its captured stderr in the prompt. Verification is the structural gate — the plan harness never flips `done` based on prose claims alone.

## Natural language triggers

- "verify phase 4"
- "run the verify runner"
- "check phase N criteria"
- "did phase N actually pass"

## Reference

- ADR-021 — Go as Tier-2 language
- ADR-022 — Single Go binary replaces bash launcher
- [`/edikt:sdlc:plan`](/commands/sdlc/plan) — the consumer of `edikt verify`
- [`/edikt:doctor`](/commands/doctor) — checks for stale verify reports on phases marked `done`
