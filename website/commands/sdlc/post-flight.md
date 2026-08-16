# edikt sdlc post-flight

**Run the post-flight review pipeline after a phase.** Composes L1 (criteria verify — the caller-passed verdict) + L2 (governance verifier) + L3 (specialist routing) + a synthesizer fork that deduplicates findings, and persists a composite report. Auto-fired by `phase-end-detector.sh` on an L1 PASS; invocable manually for any plan and phase.

This is a read-only advisor — it never modifies code. Downstream callers (the plan harness's row-flip gate, `bin/edikt doctor`, future CI surfaces) consume the composite report and decide what gates on it. It is the completion-evidence composite for a finished phase, not a single check: L1 supplies the criteria verdict already computed for the phase, L2 asks whether the phase diff violates compiled governance, and L3 routes the same diff to the domain specialists the changed files call for. The composite report is the deduplicated union of all three — no layer's verdict is the pipeline's verdict on its own.

## Synopsis

```bash
/edikt:sdlc:post-flight <plan> --phase <N>     # explicit
/edikt:sdlc:post-flight <plan>                  # phase auto-detected from active plan state
```

Plan slug and phase number are NFKC-normalized, casefolded, stripped, and allowlist-validated before any shell interpolation. The plan slug is the basename of `<plans-dir>/PLAN-<slug>.md`; when `--phase` is omitted it resolves from the plan's progress table (the most recent `done` or `in-progress` row).

## How it works

1. **Parse and validate arguments**, then check the kill switches — `EDIKT_DISABLE_POST_FLIGHT=1` (env, takes precedence) and `post-flight.enabled: false` (config) both skip the run with a structured reason.
2. **Resolve the L1 verdict** — the latest file under `.edikt/state/verify/plan-<plan>-phase-<N>-*.json`, which must conform to the evaluator-verdict schema. On a missing or malformed verdict the command exits 1; it never fabricates an L1 outcome.
3. **Resolve the diff.** Prefers the phase-start SHA captured by the plan harness, falling back to `HEAD~1..HEAD`. The unified diff is materialized to a temp file — the *path* is the contract surface to every dispatched agent; diff *text* is never interpolated into prompts. Empty diffs skip with an audit record still persisted.
4. **Dispatch L2 + L3 concurrently** in a single Agent message. L2 invokes the governance verifier per in-scope compiled topic file; L3 routes to the effective specialist set (auto-detected from changed-file patterns, then composed against config). If no compiled governance exists, L2 is skipped and L3 still runs. Partial L3 waves still feed the synthesizer.
5. **Dispatch the synthesizer fork** (read-only, locked-task) over L1 + L2 + L3 + the diff. It dedupes findings by the `(file_path, line_range, issue_class)` tuple and emits the composite report. If the synthesizer fails, raw L1/L2/L3 outputs are still persisted (no data loss).
6. **Persist the composite report** to `.edikt/state/post-flight/<plan>-<phase>-<ts>.json` and a human-readable `.md`, append a telemetry line, and print a JSON summary on stdout.

## Skip semantics

The command exits 0 on every successful completion — gating is the caller's job. Skips are emitted as structured JSON to stdout and persisted in the composite report so the audit trail captures them: empty diff, no compiled governance (L2 only — L3 still runs), `post-flight.enabled: false`, or `EDIKT_DISABLE_POST_FLIGHT=1`.

## Stub mode

`EDIKT_POSTFLIGHT_STUB=1` short-circuits the Agent dispatches and reads canned verdicts from `test/fixtures/post-flight-reports/valid/` (scenario controlled by `EDIKT_POSTFLIGHT_STUB_SCENARIO`). It exists for hermetic CI (`test/test-sdlc-post-flight.sh`). Production users should never set it.

## Verdict shape

The composite JSON conforms to the post-flight report schema. Required fields: `meta`, `l1_summary`, `l2_summary`, `l3_summary`, `findings[]`. Each finding's `sources` array lists every origin as a `<layer>:<issue_class>` string, since the synthesizer collapses overlapping L2+L3 findings.

## Related

- [`/edikt:gov:verify-diff`](/commands/gov/verify-diff) — the L2 dispatch target.
- [`/edikt:sdlc:code-review`](/commands/sdlc/code-review) — the L3 specialist routing.
- [`/edikt:verify`](/commands/verify) — the L1 sidecar-verify runner whose verdict this pipeline consumes.
