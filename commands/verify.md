---
name: verify
description: "Run a plan's verify-shell criteria and report pass/fail/timeout per criterion"
effort: low
context: fork
allowed-tools:
  - Bash
  - Read
tier_2_dependency: edikt verify
on_absent: refuse-and-direct-user
---

# edikt:verify

Run the `verify:` shell commands declared in `PLAN-<plan-id>-criteria.yaml`, capture pass/fail/timeout/skipped per criterion, and write a JSON + text report to `.edikt/state/verify/`.

This is the user-facing entry point for plan verification. `/edikt:sdlc:plan` uses the same underlying runner to gate row-flips at phase end; this command lets you trigger it explicitly outside the plan flow.

## Arguments

- `<plan-id>` (required) — the plan ID to verify (e.g. `v060-governance-accuracy`).
- `--phase <id>` — verify a single phase only (e.g. `1`, `4b`, `12`). Default: all phases.
- `--json` — emit the JSON report to stdout in addition to writing it under `.edikt/state/verify/`.
- `--allow-failures` — exit 0 even when criteria fail (failures still recorded in the report). Useful for "report-only" runs that should not block CI.

## Instructions

### 1. Resolve the plan ID

If `$ARGUMENTS` is empty, ask the user which plan to verify. Show the list:

```bash
ls docs/internal/plans/PLAN-*-criteria.yaml 2>/dev/null | sed 's|.*/PLAN-||; s|-criteria.yaml$||' | sort -u
```

(Adjust path if the project sets `paths.plans` in `.edikt/config.yaml`.)

### 2. Invoke the runner

```bash
edikt verify <plan-id> [--phase N] [--json] [--allow-failures]
```

Exit codes:
- `0` — all executed criteria passed (or only skipped/informational)
- `1` — at least one criterion failed or timed out
- `2` — sidecar missing or malformed YAML (`PLAN-<id>-criteria.yaml` not found or invalid)
- `3` — invalid args (unknown plan ID, malformed `--phase`, etc.)

### 3. Report the result

On `0`:

```
✅ Plan verify: <plan-id> — all criteria passed
   Report: .edikt/state/verify/<plan-id>-<timestamp>.json
```

On non-zero, show the actionable failures from the report (the runner writes a per-criterion breakdown to `.edikt/state/verify/<plan-id>-<timestamp>.{json,txt}`). Surface the first failing criterion's command + stderr so the user can act without opening files.

### 4. Recovery path

If exit code is `2`:

```
PLAN-<id>-criteria.yaml not found or invalid.
Generate it with /edikt:sdlc:plan, or fix the YAML manually under docs/internal/plans/.
```

If `bin/edikt` is missing:

```
edikt binary not found. Bootstrap via /edikt:upgrade in Claude Code (primary path).
```

## Reference

### When to use this directly vs. via /edikt:sdlc:plan

- **`/edikt:sdlc:plan`** uses `edikt verify` internally to gate every phase row-flip from `in_progress` → `complete`. You normally never invoke verify directly when working a plan.
- **`/edikt:verify`** is useful for: re-running a single phase's criteria after a code change, CI checks that don't drive the full plan flow, and debugging why a row-flip was rejected.

### JSON report shape

```json
{
  "plan_id": "v060-governance-accuracy",
  "phase": "all",
  "run_at": "2026-05-17T20:00:00Z",
  "criteria": [
    {
      "id": "1.1",
      "phase": "1",
      "command": "go test ./tools/edikt/...",
      "status": "pass",
      "duration_ms": 1432
    },
    {
      "id": "1.2",
      "phase": "1",
      "command": "test/test-e2e-v04-guideline-heavy.sh",
      "status": "fail",
      "duration_ms": 2104,
      "stderr": "FAIL: gov compile produced empty output"
    }
  ],
  "summary": {
    "passed": 1,
    "failed": 1,
    "timed_out": 0,
    "skipped": 0
  }
}
```

### Natural-language triggers

- "verify plan N", "verify phase X of plan Y"
- "run the verify checks for [plan-id]"
- "did phase N actually pass"

### Notes

This command is a thin orchestrator over the `edikt verify` tier-2 helper. Per the architectural principle "slash commands are the primary user surface always," users should invoke `/edikt:verify`, not `edikt verify` directly. The binary remains fully discoverable via `edikt --help` for debugging and direct CI use.
