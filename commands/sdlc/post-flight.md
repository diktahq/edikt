---
name: sdlc:post-flight
description: "Post-flight review composition — runs L1 (criteria verify, caller-passed) + L2 (governance verifier) + L3 (specialist routing) + synthesis after a phase. Auto-fired by phase-end-detector.sh on L1 PASS; invocable manually for any plan/phase."
effort: high
allowed-tools:
  - Glob
  - Read
  - Bash
  - Agent
context: fork
---

# edikt:sdlc:post-flight

Run the **post-flight review pipeline**. Compose L1 (criteria verify — already-passed verdict from the caller) + L2 (governance verifier) + L3 (specialist routing via `/edikt:sdlc:code-review`) + a synthesizer fork that deduplicates findings. Persist a composite report to `.edikt/state/post-flight/<plan>-<phase>-<ts>.{json,md}`.

This is a **read-only advisor**. It never modifies code. Downstream callers — the plan harness's row-flip gate (`commands/sdlc/plan.md`), `bin/edikt doctor`, and any future CI surface — consume the composite report and decide what gates on it.

The completion-evidence check at the diff timeframe. The other layers verify that asserted state still holds at static call sites; this one verifies that a *diff* does not silently violate compiled governance.

## Synopsis

```bash
/edikt:sdlc:post-flight <plan> --phase <N>             # explicit
/edikt:sdlc:post-flight <plan>                          # phase auto-detected from active plan state
```

Plan name and phase are validated against an allowlist regex (NFKC-normalized, stripped — case preserved, never casefolded) before any shell interpolation — input-validation defense.

## Inputs and contract

- **Plan slug** — basename of `<plans-dir>/PLAN-<slug>.md` without extension. Allowlist: `^[A-Za-z0-9._-]+$`, length ≤ 200. Mixed case is preserved — the value is NFKC-normalized and stripped, but never casefolded, so it round-trips into filesystem paths exactly as the caller typed it. (This resolves a prior contradiction: this line always allowed `A-Za-z`, but Step 1's implementation casefolded to lowercase before matching — see Step 1 below.)
- **Phase number** — non-negative integer (0–999). When omitted, resolved from the plan's progress table (the most recent `done` row, or `in-progress` if no row has flipped to done yet).
- **L1 verdict** — the most recent file under `.edikt/state/verify/` matching `{stem}-phase-{N}-*.json` (latest by timestamp), where `{stem}` is tried against BOTH naming forms defined in `templates/hooks/_plan-naming.sh`: `edikt_plan_stem` (keeps the `PLAN-` prefix) and `edikt_plan_id` (drops it). The two families are not interchangeable — see that file's header comment — so both are globbed and whichever form has matches wins. Must conform to `templates/schemas/verify-report.v1.schema.json` — the schema `bin/edikt verify` actually writes for files under `.edikt/state/verify/`. (`templates/agents/evaluator-verdict.schema.json` governs a different artifact — the phase-completion evaluator's verdict — and is NOT the schema for this directory.) On missing/malformed: exit 1 with `{"status":"error","reason":"L1 verdict malformed or missing"}` — the post-flight pipeline NEVER fabricates an L1 outcome.

## Skip semantics (informational)

The command exits `0` on every successful completion. Gating is the caller's job. Skips are emitted as structured JSON to stdout AND persisted in the composite report so the audit trail captures them:

| Condition | Stdout |
|---|---|
| Empty diff (after binary filter) | `{"status":"skipped","reason":"empty diff"}` |
| No compiled governance topic files | L2 skipped: `{"status":"skipped","reason":"no compiled governance"}` — L3 still runs |
| `post-flight.enabled: false` in config | `{"status":"skipped","reason":"post-flight disabled"}` |
| `EDIKT_DISABLE_POST_FLIGHT=1` env var | `{"status":"skipped","reason":"post-flight disabled (env)"}` — overrides config |

## Stub mode

`EDIKT_POSTFLIGHT_STUB=1` short-circuits the Agent dispatches. The orchestrator reads canned verdicts from `test/fixtures/post-flight-reports/valid/happy-path.json` (and friends, per scenario flag — see `EDIKT_POSTFLIGHT_STUB_SCENARIO`) and writes them directly to the report path. Used by `test/test-sdlc-post-flight.sh` for hermetic CI. Production users should never set this env var.

## Instructions

0. If `.edikt/config.yaml` does not exist, output:
   ```
   No edikt config found. Run /edikt:init to set up this project.
   ```
   And stop.

### 1. Parse and validate arguments

```bash
# Extract plan + phase from $ARGUMENTS. Plan is mandatory; --phase is optional
# (auto-detected if absent).
PLAN_RAW="${ARGUMENTS%% *}"
PHASE_RAW=""
case " $ARGUMENTS " in
  *" --phase "*) PHASE_RAW=$(echo "$ARGUMENTS" | sed -nE 's/.*--phase[= ]+([^ ]+).*/\1/p') ;;
esac

# THE ONE DEFINITION of plan-state filenames lives in _plan-naming.sh — source
# it rather than re-deriving stems/ids inline (see that file's header for why).
. "${EDIKT_HOOK_DIR:-$HOME/.edikt/hooks}/_plan-naming.sh"

# Allowlist validation (NFKC + strip + regex). NEVER casefold: the plan slug
# is used verbatim to construct filesystem paths ("<plans-dir>/PLAN-${PLAN}.md",
# L1 verdict globs, the composite report path), and this file's own "Inputs
# and contract" section above has always allowed mixed case (`A-Za-z`).
# Casefolding here silently disagreed with that contract — fixed by dropping
# it, not by lowercasing the contract.
PLAN=$(python3 -c '
import sys, unicodedata, re
v = unicodedata.normalize("NFKC", sys.argv[1]).strip()
if not re.match(r"^[A-Za-z0-9._-]{1,200}$", v):
    sys.exit(1)
sys.stdout.write(v)
' "$PLAN_RAW") || {
    python3 -c 'import json,sys; print(json.dumps({"status":"error","reason":"plan slug fails allowlist","input":sys.argv[1]}))' "$PLAN_RAW"
    exit 1
}
PLAN_FILE="<plans-dir>/PLAN-${PLAN}.md"

if [ -n "$PHASE_RAW" ]; then
    PHASE=$(python3 -c '
import sys, unicodedata, re
v = unicodedata.normalize("NFKC", sys.argv[1]).strip()
if not re.match(r"^[0-9]{1,3}$", v):
    sys.exit(1)
sys.stdout.write(v)
' "$PHASE_RAW") || {
        python3 -c 'import json,sys; print(json.dumps({"status":"error","reason":"phase number fails allowlist","input":sys.argv[1]}))' "$PHASE_RAW"
        exit 1
    }
else
    # Auto-detect from plan progress table — implementation reads the
    # most recent done or in-progress row.
    PHASE=$(python3 -c '
import re, sys
text = open(sys.argv[1]).read()
last = None
for line in text.splitlines():
    m = re.match(r"^\|\s*([0-9]+)\s*\|\s*(done|in-progress)\s*", line, re.I)
    if m: last = m.group(1)
print(last or "")
' "$PLAN_FILE") || PHASE=""
    if [ -z "$PHASE" ]; then
        python3 -c 'import json,sys; print(json.dumps({"status":"error","reason":"could not auto-detect phase from plan"}))'
        exit 1
    fi
fi
```

### 1a. Resolve the L1 verdict path

```bash
# Try both naming forms from _plan-naming.sh — the state families on disk
# are not interchangeable (phase-start SHAs drop the PLAN- prefix, verify
# reports have been observed keeping it). Glob both, take the newest match
# by filename timestamp across whichever form actually has files.
STEM_ID=$(edikt_plan_id "$PLAN_FILE")      # e.g. "v072-defect-sweep"
STEM_FULL=$(edikt_plan_stem "$PLAN_FILE")  # e.g. "PLAN-v072-defect-sweep"

L1_VERDICT_PATH=$(ls -t ".edikt/state/verify/${STEM_ID}-phase-${PHASE}-"*.json \
                        ".edikt/state/verify/${STEM_FULL}-phase-${PHASE}-"*.json \
                        2>/dev/null | head -1)

if [ -z "$L1_VERDICT_PATH" ] || ! python3 -c 'import json,sys; json.load(open(sys.argv[1]))' "$L1_VERDICT_PATH" 2>/dev/null; then
    python3 -c 'import json,sys; print(json.dumps({"status":"error","reason":"L1 verdict malformed or missing"}))'
    exit 1
fi
# L1_VERDICT_PATH must conform to templates/schemas/verify-report.v1.schema.json.
```

### 2. Kill-switch checks (env var first, then config)

```bash
# Env var takes precedence — for emergency rollback when config is corrupt.
if [ "${EDIKT_DISABLE_POST_FLIGHT:-0}" = "1" ]; then
    python3 -c 'import json,sys; print(json.dumps({"status":"skipped","reason":"post-flight disabled (env)"}))'
    exit 0
fi

# Config gate.
SCOPE_JSON=$(bin/edikt gov post-flight-scope --json 2>/dev/null || echo '{"enabled":true,"specialists":[]}')
ENABLED=$(echo "$SCOPE_JSON" | python3 -c 'import json,sys; d=json.load(sys.stdin); print("1" if d.get("enabled") else "0")')

if [ "$ENABLED" != "1" ]; then
    python3 -c 'import json,sys; print(json.dumps({"status":"skipped","reason":"post-flight disabled"}))'
    exit 0
fi
```

### 3. Resolve the diff

```bash
# Prefer phase-start SHA captured by the plan harness on row-flip-to-in-progress.
# Fall back to HEAD~1..HEAD — loudly, since a silent fallback can under- or
# over-capture the diff when the phase's commits don't align with HEAD~1.
#
# SHA_FILE path comes from the same accessor plan.md's row-flip step uses
# (templates/hooks/_plan-naming.sh, sourced in Step 1) — never re-derive it
# inline with a casefolded PLAN, which drifts from the file plan.md actually
# writes.
SHA_FILE="$(edikt_phase_start_sha "$PLAN_FILE" "$PHASE")"
DIFF_FALLBACK=""
if [ -f "$SHA_FILE" ]; then
    START_SHA=$(cat "$SHA_FILE" | tr -d '[:space:]')
    RANGE="${START_SHA}..HEAD"
else
    RANGE="HEAD~1..HEAD"
    DIFF_FALLBACK="HEAD~1..HEAD"
    echo "⚠ post-flight: no phase-start SHA at ${SHA_FILE} — falling back to ${DIFF_FALLBACK}. This can under- or over-capture the phase's diff if its commits don't align with HEAD~1..HEAD." >&2
fi

# Changed files, binary-filtered.
CHANGED_FILES=$(git diff --numstat "$RANGE" 2>/dev/null \
    | awk '$1 != "-" && $2 != "-" { print $3 }' \
    | grep -v '^$' || true)

# Materialize the unified diff to a temp file. The PATH is the contract surface
# to every dispatched agent — diff TEXT is never interpolated into prompts.
DIFF_FILE=$(mktemp -t edikt-post-flight.XXXXXX.diff)
git diff "$RANGE" > "$DIFF_FILE" 2>/dev/null || true
```

### 4. Empty-diff guard

```bash
if [ -z "$CHANGED_FILES" ]; then
    # Persist an audit record even on skip — downstream callers depend on the
    # composite report existing for every post-flight run.
    write_skip_report "$PLAN" "$PHASE" "empty diff" "$DIFF_FILE"
    python3 -c 'import json,sys; print(json.dumps({"status":"skipped","reason":"empty diff"}))'
    rm -f "$DIFF_FILE"
    exit 0
fi
```

### 5. L2 dispatch — governance verifier

Invoke `/edikt:gov:verify-diff <range>`. Pre-flight: check for compiled topic files via the Glob tool. If empty, L2 is skipped (`l2_summary.status=skipped`, reason=`no compiled governance`) but L3 still runs.

```bash
# Glob: .claude/rules/governance/*.md
# If empty → L2 skipped, set L2_VERDICT_PATH to a stub envelope file.
```

The L2 dispatch produces zero or more per-topic verdict JSONs under `.edikt/state/gov-verify/`. The orchestrator captures the **latest run's** outputs (by timestamp) and aggregates them into the L2 input file passed to the synthesizer.

### 6. L3 dispatch — specialists

Compute the effective specialist set:

```bash
# Auto-detection: shell out to a python heredoc that reads
# commands/_shared-agent-routing.md's pattern table and matches CHANGED_FILES.
# Pass the list to gov post-flight-scope which composes (auto ∪ required) − never.
AUTO_DETECTED=$(python3 -c '
import re, sys
# Minimal auto-detect: file-extension → specialist. The full routing table
# lives in commands/_shared-agent-routing.md.
routing = {
    r"\.go$": "backend",
    r"\.(ts|tsx)$": "frontend",
    r"\.sql$": "dba",
    r"templates/hooks/.*\.sh$": "security",
    r"\.github/workflows/": "sre",
}
detected = set()
for line in sys.argv[1].splitlines():
    for pat, spec in routing.items():
        if re.search(pat, line):
            detected.add(spec)
print(",".join(sorted(detected)))
' "$CHANGED_FILES")

SPECIALISTS_JSON=$(bin/edikt gov post-flight-scope --json --auto-detected "$AUTO_DETECTED" 2>/dev/null || echo '{"specialists":[]}')
SPECIALISTS=$(echo "$SPECIALISTS_JSON" | python3 -c 'import json,sys; print(",".join(json.load(sys.stdin).get("specialists", [])))')
```

If `SPECIALISTS` is empty after scope filter, L3 is skipped (`l3_summary.status=skipped`, reason=`no specialists in effective set`).

### 7. PARALLEL dispatch — L2 + L3 in a single Agent message

**Critical:** L2 and L3 dispatch CONCURRENTLY. Send a single message with MULTIPLE Agent tool calls:

```text
Agent(subagent_type: "governance-verifier", ...)   # one per in-scope topic
Agent(subagent_type: "<specialist>", ...)          # one per effective specialist
```

Neither layer's prompt references the other's verdict. Each agent receives the diff PATH (never the diff TEXT), the topic / specialist scope, and writes its output to `mktemp -t edikt-l{2,3}.XXXXXX.json`. The synthesizer (Step 9) reads all four input file paths.

Concurrent dispatch is what keeps post-flight wall-clock under control on diffs that touch multiple specialist domains.

### 8. Handle partial wave failures

If an L3 dispatch fails with `Agent type '<slug>' not found` but `.claude/agents/<slug>.md` exists, first re-dispatch via the fallback in `commands/_shared-agent-routing.md` § "Fallback: agent installed this session" — only degrade to `partial` if the fallback also fails.

If any L3 specialist times out or returns non-JSON:

- `l3_summary.status` = `partial`
- Record which specialists completed and which timed out in `l3_summary.reason`
- The synthesizer (Step 9) still runs — partial L3 data is better than no L3 data

If L2 itself errors (the entire `gov:verify-diff` dispatch fails, not per-topic):

- `l2_summary.status` = `unavailable`
- Synthesis still runs over (L1, empty L2, partial L3)

### 9. Synthesizer dispatch — fresh fork

After L2+L3 complete (or timeout), dispatch the synthesizer:

```text
Agent(
  subagent_type: "post-flight-synthesizer",
  prompt: "
    L1: <L1_VERDICT_PATH>
    L2: <L2_AGGREGATED_PATH>
    L3: <L3_REPORTS_DIR>
    Diff: <DIFF_FILE>
    meta.plan = <PLAN>
    meta.phase = <PHASE>
    meta.dispatch_mode = auto-hook | manual | stub
  ",
  description: "Synthesize post-flight composite"
)
```

The synthesizer is locked-task (Read-only). It dedupes findings by the `(file_path, line_range, issue_class)` tuple and emits the composite JSON conforming to `templates/agents/post-flight-report.schema.json`.

### 9a. Synthesizer-fork failure handling

If the synthesizer fork fails (spawn error, timeout, non-JSON output):

- The orchestrator still persists L1+L2+L3 raw outputs to `.edikt/state/post-flight/<plan>-<phase>-<ts>-{l1,l2,l3}.raw.json` (NO DATA LOSS)
- The composite report file is written with `meta.dispatch_mode` unchanged and a synthetic `findings: []` array
- `l3_summary.status` (or a dedicated `synthesizer_status` field if the schema evolves) reflects `unavailable`

### 9b. EDIKT_POSTFLIGHT_STUB short-circuit

```bash
if [ "${EDIKT_POSTFLIGHT_STUB:-0}" = "1" ]; then
    # Stub mode: read a canned composite report from Phase 1 fixtures.
    # Scenario controlled by EDIKT_POSTFLIGHT_STUB_SCENARIO (default: happy-path).
    SCENARIO="${EDIKT_POSTFLIGHT_STUB_SCENARIO:-happy-path}"
    FIXTURE="test/fixtures/post-flight-reports/valid/${SCENARIO}.json"
    if [ ! -f "$FIXTURE" ]; then
        FIXTURE="test/fixtures/post-flight-reports/valid/happy-path.json"
    fi
    REPORT_JSON=$(python3 -c '
import json, sys, datetime, pathlib
tpl = json.loads(pathlib.Path(sys.argv[1]).read_text())
tpl["meta"]["plan"] = sys.argv[2]
tpl["meta"]["phase"] = int(sys.argv[3])
tpl["meta"]["ran_at"] = datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")
tpl["meta"]["dispatch_mode"] = "stub"
print(json.dumps(tpl, indent=2))
' "$FIXTURE" "$PLAN" "$PHASE")
fi
```

### 10. Persist composite report

```bash
TS=$(date -u +%Y%m%dT%H%M%SZ)
REPORT_DIR=".edikt/state/post-flight"
mkdir -p "$REPORT_DIR"
REPORT_JSON_PATH="$REPORT_DIR/${PLAN}-${PHASE}-${TS}.json"
REPORT_MD_PATH="$REPORT_DIR/${PLAN}-${PHASE}-${TS}.md"

# Validate + write the JSON via python3 json.dumps.
python3 -c '
import json, sys, pathlib
data = json.loads(sys.argv[1])
pathlib.Path(sys.argv[2]).write_text(json.dumps(data, indent=2) + "\n")
' "$REPORT_JSON" "$REPORT_JSON_PATH"

# Human-readable Markdown rendering (inline — no separate .sh helper).
python3 -c '
import json, sys, pathlib
data = json.loads(sys.argv[1])
lines = []
m = data["meta"]
lines.append(f"# Post-flight report — {m[\"plan\"]} phase {m[\"phase\"]}\n")
lines.append(f"Ran at: {m[\"ran_at\"]}  ({m[\"dispatch_mode\"]})\n")
for layer in ("l1", "l2", "l3"):
    s = data[f"{layer}_summary"]
    lines.append(f"## {layer.upper()}: {s[\"status\"]}")
    if "reason" in s: lines.append(f"  {s[\"reason\"]}")
findings = data.get("findings", [])
if findings:
    lines.append(f"\n## Findings ({len(findings)})\n")
    for f in findings:
        srcs = ", ".join(f["sources"])
        lines.append(f"- **{f[\"severity\"]}** `{f[\"file\"]}:{f[\"line_range\"]}` [{srcs}] {f[\"issue_class\"]} — {f[\"description\"]}")
else:
    lines.append("\n## Findings: none\n")
pathlib.Path(sys.argv[2]).write_text("\n".join(lines) + "\n")
' "$REPORT_JSON" "$REPORT_MD_PATH"
```

### 11. Append telemetry

```bash
# Append-only audit log of dispatch outcomes. flock prevents concurrent
# writers from corrupting the file.
METRICS=".edikt/state/post-flight/.metrics.jsonl"
LOCK="${METRICS}.lock"
(
    flock -x 9
    python3 -c '
import json, sys
print(json.dumps({
    "ts": sys.argv[1],
    "plan": sys.argv[2],
    "phase": int(sys.argv[3]),
    "dispatch_mode": sys.argv[4],
    "l1_status": sys.argv[5],
    "l2_status": sys.argv[6],
    "l3_status": sys.argv[7],
    "synthesis_status": sys.argv[8],
    "elapsed_ms": int(sys.argv[9])
}))' "$TS" "$PLAN" "$PHASE" "$DISPATCH_MODE" "$L1_STATUS" "$L2_STATUS" "$L3_STATUS" "$SYNTH_STATUS" "$ELAPSED_MS" >> "$METRICS"
) 9>"$LOCK"
```

### 12. Stdout summary + exit

`diff_fallback` surfaces the Step 3 fallback loudly in the command's own output contract — `null` when the phase-start SHA was found, `"HEAD~1..HEAD"` when it wasn't (matching the ⚠ line already printed in Step 3). This field is part of the stdout summary only; it is NOT added to the persisted composite report, whose `meta` object is schema-locked (`additionalProperties: false` in `templates/agents/post-flight-report.schema.json`).

```bash
python3 -c '
import json, sys
print(json.dumps({
    "status": "ok",
    "plan": sys.argv[1],
    "phase": int(sys.argv[2]),
    "report_json": sys.argv[3],
    "report_md": sys.argv[4],
    "elapsed_ms": int(sys.argv[5]),
    "diff_fallback": (sys.argv[6] or None)
}, indent=2))' "$PLAN" "$PHASE" "$REPORT_JSON_PATH" "$REPORT_MD_PATH" "$ELAPSED_MS" "$DIFF_FALLBACK"

rm -f "$DIFF_FILE"
exit 0
```

## Verdict shape

The composite JSON conforms to `templates/agents/post-flight-report.schema.json`. Required fields: `meta`, `l1_summary`, `l2_summary`, `l3_summary`, `findings[]`.

`findings[]` carries deduplicated entries — the synthesizer collapses overlapping L2+L3 findings by the `(file_path, line_range, issue_class)` tuple. Each entry's `sources` array lists all origins as `<layer>:<issue_class>` strings.

## Invariants honoured

- **Tier-1 markdown only.** No new Go binary verb. Glob/Read/Bash/Agent tools, plus the `bin/edikt gov post-flight-scope` query.
- **Safe JSON construction.** Every JSON object the command emits is constructed via `python3 -c 'import json,sys; print(json.dumps(...))'` with values passed as separate argv elements. Shell-string concatenation of JSON is forbidden.
- **No agent text into Claude-facing channels.** Agent verdict text is persisted to JSON report files. It is NEVER interpolated into `systemMessage` or any other Claude-facing channel by this command. Downstream callers that surface verdict text must follow the same rule.
- **Input validation.** Every external value — plan slug, phase number, ref range, topic name, specialist name — is NFKC-normalized + allowlist-validated before it reaches shell argv, a path, or a prompt. Untrusted values flow as separate argv elements, never concatenated into evaluated strings.
- **Hermetic tests.** The `test/test-sdlc-post-flight.sh` e2e is hermetic: tmpdir-staged, no host `~/.claude/` reads, runs under `EDIKT_POSTFLIGHT_STUB=1`.
- **Read-only advisor composition.** Every dispatched agent uses `context: fork`; no agent sees the parent session's conversation history. Only the explicitly-passed file paths and the agent's system prompt.

## Helper: write_skip_report

```bash
# Inline bash function used by the empty-diff guard (Step 4) and the env-var /
# config kill-switch paths to persist an audit record even when no real
# dispatch happens. Keeps the composite-report directory complete across all
# exit paths.
write_skip_report() {
    local plan="$1" phase="$2" reason="$3" diff_file="$4"
    local ts
    ts=$(date -u +%Y%m%dT%H%M%SZ)
    local report_dir=".edikt/state/post-flight"
    mkdir -p "$report_dir"
    python3 -c '
import json, sys, datetime, pathlib
report = {
    "meta": {
        "plan": sys.argv[1],
        "phase": int(sys.argv[2]),
        "ran_at": datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ"),
        "synthesizer_version": "1.0.0",
        "dispatch_mode": "auto-hook" if sys.argv[5] == "1" else "manual",
    },
    "l1_summary": {"status": "passed"},  # caller asserted L1 PASS to even invoke us
    "l2_summary": {"status": "skipped", "reason": sys.argv[3]},
    "l3_summary": {"status": "skipped", "reason": sys.argv[3]},
    "findings": [],
}
pathlib.Path(sys.argv[4]).write_text(json.dumps(report, indent=2) + "\n")
' "$plan" "$phase" "$reason" "$report_dir/${plan}-${phase}-${ts}.json" "${EDIKT_AUTO_HOOK:-0}"
}
```

## See also

- `commands/gov/verify-diff.md` — L2 dispatch target
- `commands/sdlc/code-review.md` — L3 specialist routing
- `templates/agents/post-flight-synthesizer.md` — synthesizer agent
- `templates/agents/post-flight-report.schema.json` — composite report schema

## Completion

```
✅ Post-flight composition complete
   Composite report: .edikt/state/post-flight/<plan>-<phase>-<ts>.json
                     .edikt/state/post-flight/<plan>-<phase>-<ts>.md

   Next: Open the .md report to review findings, or let the plan harness
   consume the .json verdict at the next row-flip evaluation.
```
