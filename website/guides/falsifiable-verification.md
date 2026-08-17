# Falsifiable verification

Most governance tooling trusts the agent's word. It marks a task done because the agent *said* it was done. edikt v0.7.0 stops doing that: a completion claim only counts if there is **fresh, executable evidence** for it in the same turn — and the check that produces that evidence has to be one an agent can't trivially fake.

This guide covers the verification layer end to end: executable `verify:` commands, the behavioral/tooling/structural grading, the pre-edit gate, intent-aware diff review, the cheat-rate benchmark, and the post-flight pipeline that ties them together.

## The core idea: a rule carries its own check

Any directive, requirement, or acceptance criterion that can be checked mechanically carries an executable `verify:` shell command that exits `0` exactly when the asserted state holds:

```yaml
directives:
  - text: "Hooks MUST emit JSON via a structured serializer, never shell concatenation."
    verify: "! rg -nE 'echo .*\\{|printf .*\\{' templates/hooks/"
    verify_kind: structural
```

`bin/edikt verify gov|prd|spec <id>` (and `verify all`) runs every `verify:` in the addressed sidecars under `bash -c` with a 30-second timeout, captures pass / fail / timeout / skipped, and writes a JSON + text report under `.edikt/state/verify/`. Exit codes: `0` all passed, `1` a failure or timeout, `2` sidecar missing/malformed, `3` bad arguments. `gov compile` runs the gate after merging — if a declared `verify:` fails, the compile fails.

## Not all checks are equal: `verify_kind`

A `verify:` that greps for a string is easy to satisfy without doing the work. One that runs the code and asserts a property is not. `verify_kind` records which kind a check is:

| `verify_kind` | What it does | Example |
|---|---|---|
| `behavioral` | Runs code and asserts a runtime property | spin up the handler, assert a 4xx returns a structured error body |
| `tooling` | Invokes a stable external tool with a fixed rule ID | `go vet ./...`, `eslint --rule ...` |
| `structural` | grep / file-presence over content | "no `go.mod` at the tier-1 root" |

`structural` is only legitimate when the property *is* structural. Anything describing runtime behavior should be `behavioral`. Phase B compile enforces the pairing: if `verify` is set, `verify_kind` must be too.

### Behavioral verifies require human approval

Because a behavioral verify runs arbitrary code as part of the gate, promoting one isn't automatic. It carries `human_approved_at` (an RFC 3339 timestamp), and Phase B refuses to compile a behavioral verify until that field is set. You approve a pending proposal with:

```
/edikt:sidecar:approve <id>      # review → promote / reject / defer
```

which calls `bin/edikt sidecar approve` and records the approval. No approval, no behavioral verify in compiled governance.

## The pre-edit gate

Verification is only honest if a claim *can't* land without evidence — but how far the gate goes is a posture dial, not a fixed behavior. The `verify-gate` PreToolUse hook watches for completion-claim edits — flipping a sidecar to `passes: true`, marking a plan row `done`, ticking an `AC-NNN` checkbox — and evaluates them against `features.evidence-gate` in `.edikt/config.yaml`. Each of the four postures is observably distinct (ADR-062):

| posture | write | stdout | audit record |
|---|---|---|---|
| `block` | refused | `hookSpecificOutput.permissionDecision: "deny"` | `gate.deny` |
| `warn` (default) | allowed, with a model-facing warning | `hookSpecificOutput.additionalContext` carrying the warning | `gate.warn` |
| `educate` | allowed | `{"continue": true}` | `posture.educate` |
| `disabled` | allowed | `{"continue": true}` | none — short-circuited before any state I/O |

An unrecognised or unreadable value resolves to `warn`, not `block` — the gate's safe state is the shipped default, not the strictest posture. Only under `block` does the hook actually refuse a completion-claim edit that lacks a fresh verify report; under the default `warn` posture the edit still lands, and the model gets a warning on the same turn instead.

Bypass envelope, for the operations that legitimately write completion state regardless of posture:

- `EDIKT_HOOK_ACTOR` ∈ `{migrate, compile, upgrade}` — the edikt flows themselves.
- `EDIKT_DISABLE_VERIFY_GATE=1` — an ad-hoc operator escape hatch.

`/edikt:doctor` reports the resolved posture (`block` / `warn` / `educate` / `disabled`), not a binary enabled/bypassed state.

## Intent-aware diff review

`/edikt:gov:verify-diff` is the diff-time layer. For each compiled governance topic whose `paths:` glob matches a changed file, it forks a read-only `governance-verifier` agent and emits per-directive `PASS` / `FAIL` / `NEEDS_REVIEW`. When a directive carries `intent` + `falsifying_observation` fields, the verifier evaluates against *those* — the directive's stated intent and what a violation would look like — rather than the raw wording, so the check resists phrasing the generator controls.

## Measuring whether the checks are any good: cheat-rate

A `verify:` you can satisfy without doing the work is worse than no check — it's false confidence. The cheat-rate benchmark measures exactly that:

```
bin/edikt gov benchmark cheat-rate <id>     # or --all for the corpus
```

It dispatches an adversary model that tries to make a `verify:` exit `0` **without** implementing the directive's behavioral intent. The aggregate `cheat_rate = cheated / total` has a soft ceiling of **20%** — above that, the verify is too cheatable and should be re-authored (usually: promote it from `structural` to `behavioral`). The benchmark is advisory and opt-in (`edikt install benchmark`); it never blocks a build.

## Post-flight: putting it together after a phase

When a plan phase finishes, `/edikt:sdlc:post-flight` composes the layers into one report:

- **L1** — the criteria `verify:` run (pass/fail).
- **L2** — the governance verifier over the diff (`verify-diff`).
- **L3** — specialist-agent review routed by what changed.
- **synthesis** — deduplicated into a single composite under `.edikt/state/post-flight/`.

It fires automatically after a phase (only when L1 passes) via the phase-end hook, and gates the plan's row-flip-to-done on the combined L1+L2 verdict. Kill-switches: `EDIKT_DISABLE_POST_FLIGHT=1` or `post-flight.enabled: false` in `.edikt/config.yaml`.

## In one sentence

A claim isn't done because the agent says so — it's done because an executable, hard-to-fake check says so, the gate refused to record it otherwise, and the benchmark keeps the checks themselves honest.
