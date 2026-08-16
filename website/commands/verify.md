# edikt verify

**CLI reference.** `edikt verify` is the runner that executes the `verify:` shell commands declared in sidecars (plan, governance, PRD, SPEC). It is invoked **by slash commands and by the binary's own subcommands** — not typed directly by users in normal workflows. When you create an ADR, `/edikt:adr:new` shells to `bin/edikt verify gov <ID>` for you; when you run `/edikt:sdlc:prd ship`, the slash command runs `bin/edikt verify prd <PRD-ID>` first; `bin/edikt gov compile` calls `verify all` after Phase B. The exit code is the gate.

This page is reference material for understanding the runner's contract — what verifies, where reports land, what exit codes mean, and which slash commands invoke each subcommand. Manual invocation is supported (e.g., `bin/edikt verify gov ADR-007` when debugging) but isn't the daily-driver path.

In v0.6.0 the surface grew from one mode (plan-criteria) to five — the plan flow, three artifact-class flows (gov / prd / spec), and an all-walker. Every flow shares the same runner, the same exit-code contract, and the same per-item report shape.

## Usage

```bash
edikt verify <plan-id>                  # plan criteria (legacy / canonical)
edikt verify <plan-id> --phase 4b       # one phase of a plan
edikt verify gov  <ADR-NNN | INV-NNN | guideline-slug>
edikt verify prd  <PRD-NNN>
edikt verify spec <SPEC-NNN>
edikt verify all                        # every gov + prd + spec sidecar
edikt verify all --json                 # structured report on stdout
edikt verify <any-of-the-above> --allow-failures
```

`<plan-id>` is the slug from `PLAN-<plan-id>-criteria.yaml`. `<ADR-NNN>` / `<INV-NNN>` resolve under `paths.{decisions,invariants}`; a guideline slug resolves under `paths.guidelines`. `<PRD-NNN>` and `<SPEC-NNN>` resolve under `paths.prds` and `paths.specs`. The runner walks up from the working directory looking for `.edikt/config.yaml` to resolve those paths, falling back to conventional defaults.

## Subcommands

| Subcommand | Walks | Used by |
|---|---|---|
| `verify <plan-id>` | The plan's `-criteria.yaml` (every phase, or one phase via `--phase`) | `/edikt:sdlc:plan` row-flip gate |
| `verify gov <ID>` | `directives[].verify`, `prohibitions[].verify`, structured `verification[].verify` in `<ID>.edikt.yaml` | `/edikt:adr:new`, `/edikt:invariant:new`, `/edikt:guideline:new` post-write step; `gov compile`'s post-merge gate (via `verify all`) |
| `verify prd <PRD-NNN>` | `requirements[].verify` (FRs) + `acceptance_criteria[].verify` (ACs) in the PRD sidecar | `/edikt:sdlc:prd PRD-NNN ship` and `supersede` pre-transition gates |
| `verify spec <SPEC-NNN>` | `requirements[].verify` (SRs) + `acceptance_criteria[].verify` (ACs / SACs) in the SPEC sidecar | `/edikt:sdlc:drift` (verify failures fold into the drift report) |
| `verify all` | Every gov / prd / spec sidecar under `paths.*` in one pass | `gov compile` post-merge gate; `doctor` coverage signal |

Items declared without a `verify:` field are recorded as `skipped` — never `passed`, never `failed` — so coverage can be measured separately from health.

## Flags

| Flag | Description |
|---|---|
| `--phase <id>` | Plan-only. Run only the named phase. Accepts numeric (`4`) or numeric-with-suffix (`4b`, `12c`). |
| `--json` | **Persistent** — emit the full JSON report to stdout in addition to writing it to disk. Inherited by `verify gov`, `prd`, `spec`, `all`. Suppresses the human-readable progress lines. |
| `--allow-failures` | **Persistent** — suppress exit-1 on failures or timeouts (failures are still recorded in the report). Inherited by all subcommands. Used by `/edikt:sdlc:plan` to surface failures without blocking. |

## Exit codes

Shared contract across every subcommand:

| Code | Meaning |
|---|---|
| `0` | All executed items passed (or only `skipped` / `informational` results). |
| `1` | At least one item failed or timed out. Suppressed by `--allow-failures`. |
| `2` | Sidecar missing or YAML malformed. |
| `3` | Invalid argument (unknown id, malformed `--phase`, etc.). |

Every claim-gating path in edikt consumes the exit code only — output is for humans and structured-report consumers.

## Report file

Every run writes a report to `.edikt/state/verify/`:

```
PLAN-<plan-id>-<phase-or-all>-<timestamp>.json   # plan-criteria flow
gov-<ID>-all-<timestamp>.json                     # verify gov
prd-<ID>-all-<timestamp>.json                     # verify prd
spec-<ID>-all-<timestamp>.json                    # verify spec
```

`verify all` writes the per-sidecar report at the same path each per-class invocation would, so the on-disk history is shared.

The JSON report carries the addressed `plan_id` (or `<kind>-<ID>`), `phase` (or `all`), `git_sha` (HEAD short-sha, suffixed `-dirty` on uncommitted changes), `summary` (counts), and `results[]` with per-item `id`, `statement`, `status` (`passed | failed | timeout | skipped_operational | skipped_informational`), `duration_ms`, captured `stdout` / `stderr` excerpts, and the `verify:` command that ran.

## How verifies execute

Every `verify:` field is a shell command. The runner invokes it as `bash -c "<command>"` with:

- working directory = the project root (walked up from CWD until `.edikt/config.yaml` is found)
- environment = the inherited shell with `EDIKT_VERIFY=1` exported
- timeout = 30 seconds per command (a timeout records `status: timeout` and exit code `-1`)
- captured stdout / stderr capped at 4 KiB per stream in the report

A command that returns exit 0 is a pass. Any non-zero exit is a fail. Items without a `verify:` field are recorded as `skipped:operational` so the doctor's coverage metric can measure gaps.

## Where the gates are wired

The verify gate is enforced by the binary, not by the slash-command markdown. Slash commands shell to `bin/edikt verify <subcommand> <ID>` and gate on the exit code only — they never parse the binary's text output.

- **`bin/edikt gov compile`** — after Phase B succeeds, subprocesses `bin/edikt verify all`. Failure blocks the success path with exit 1 and a per-sidecar summary. Skipped in `--check` and `--json` modes; opt-out via `--skip-verify`.
- **`/edikt:adr:new`, `/edikt:invariant:new`, `/edikt:guideline:new`** — Step 7 runs `verify gov <ID>` after writing both files. Failure surfaces a warning; the artifact stays on disk.
- **`/edikt:sdlc:prd PRD-NNN ship`** — runs `verify prd PRD-NNN` first; refuses ship on failure (the sidecar is not mutated).
- **`/edikt:sdlc:prd PRD-NNN supersede`** — same gate; override via `--force-verify` (recorded in `revision_history`).
- **`/edikt:sdlc:drift`** — invokes `verify spec <SPEC-NNN>` as Step 10b; each failed item folds into the drift report as a 🔴 finding.
- **`bin/edikt doctor`** — reports `[ok] / [!!] Sidecar verify coverage` walking `verify all`'s output. Soft signal, never blocks the doctor exit.
- **`templates/hooks/stop-hook.sh`** — runtime safety net. Detects completion phrases during an in-progress plan phase and emits a non-blocking `systemMessage` suggesting `bin/edikt verify <plan-id> --phase <N>`.

## Integration with `/edikt:sdlc:plan`

When a phase reaches `evaluating`, `/edikt:sdlc:plan` invokes:

```bash
edikt verify <plan-id> --phase <N> --allow-failures
```

It reads the report, gates the row-flip on `summary.failed == 0 && summary.timeout == 0`, and surfaces every failure with its captured stderr in the prompt. Verification is the structural gate — the plan harness never flips `done` based on prose claims alone.

## Natural language triggers

- "verify phase 4"
- "verify gov ADR-007" / "verify prd PRD-001" / "verify spec SPEC-005"
- "verify everything" / "run the verify gate"
- "did phase N actually pass"

## Reference

- [Sidecar architecture](/governance/sidecar) — the source-of-truth shape `verify:` lives in
- [`/edikt:sdlc:plan`](/commands/sdlc/plan) — plan-flow consumer
- [`/edikt:gov:compile`](/commands/gov/compile) — runs the post-merge gate
- [`/edikt:doctor`](/commands/doctor) — coverage signal + stale-report detection
