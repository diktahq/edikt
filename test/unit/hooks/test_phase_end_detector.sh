#!/usr/bin/env bash
# E2E test: phase-end-detector sidecar update + verdict persistence (ADR-018).
#
# End-to-end flow without a real Claude API:
#   1. Build a temp project with plan + criteria sidecar.
#   2. Inject a mock `claude` binary on PATH that returns a canned FAIL verdict
#      (AC-2.1 met, AC-2.2 unmet) — exercises both pass/fail sidecar paths.
#   3. Run the real phase-end-detector.sh hook with a Stop payload that
#      contains a "PHASE 2 DONE" completion signal.
#   4. Assert:
#      - Hook output is valid JSON with a systemMessage
#      - AC-2.1 sidecar status updated to "pass", fail_count reset to 0
#      - AC-2.2 sidecar status updated to "fail", fail_count incremented to 1
#      - AC-2.2 fail_reason populated from evaluator evidence
#      - Top-level last_evaluated updated (not null)
#      - Verdict JSON persisted at docs/product/plans/verdicts/PLAN-test-feature/phase-2.json
#      - Persisted verdict contains "verdict": "FAIL"
#
# Opt-out: EDIKT_SKIP_HOOK_TESTS=1

set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
HOOKS="$PROJECT_ROOT/templates/hooks"

if [ "${EDIKT_SKIP_HOOK_TESTS:-0}" = "1" ]; then
    echo "  SKIP: phase-end-detector e2e — EDIKT_SKIP_HOOK_TESTS=1"
    exit 0
fi

FAIL=0

# ── 1. Build temp project ─────────────────────────────────────────────────────
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

PROJECT="$TMP/project"
mkdir -p "$PROJECT/.edikt"
mkdir -p "$PROJECT/docs/product/plans"

cat > "$PROJECT/.edikt/config.yaml" <<'YAML'
edikt_version: "0.5.0"
base: docs
evaluator:
  mode: headless
  model: sonnet
YAML

cat > "$PROJECT/docs/product/plans/PLAN-test-feature.md" <<'MD'
# Plan: test-feature

| Phase | Title | Status |
|-------|-------|--------|
| 1 | Setup | done |
| 2 | Implementation | in-progress |

## Phase 2: Implementation

**Completion Promise:** `PHASE 2 DONE`

### Acceptance Criteria
- AC-2.1: The feature exists
- AC-2.2: Tests pass
MD

# Criteria sidecar — AC-2.1 has fail_count 2 from prior runs (to verify reset on pass)
cat > "$PROJECT/docs/product/plans/PLAN-test-feature-criteria.yaml" <<'YAML'
plan: PLAN-test-feature
generated: "2026-04-18"
last_evaluated: null

phases:
  - phase: 1
    title: Setup
    status: done
    attempt: "1/3"
    criteria:
      - id: AC-1.1
        description: "Config exists"
        status: pass
        verify: null
        last_evaluated: "2026-04-17"
        fail_reason: null
        fail_count: 0
  - phase: 2
    title: Implementation
    status: in-progress
    attempt: "1/3"
    criteria:
      - id: AC-2.1
        description: "The feature exists"
        status: fail
        verify: "grep -r 'feature' src/"
        last_evaluated: "2026-04-17"
        fail_reason: "grep found no matches"
        fail_count: 2
      - id: AC-2.2
        description: "Tests pass"
        status: pending
        verify: "grep -r 'test' tests/"
        last_evaluated: null
        fail_reason: null
        fail_count: 0
YAML

# ── 2. Mock claude binary ─────────────────────────────────────────────────────
# Returns a canned FAIL verdict: AC-2.1 met, AC-2.2 unmet.
# The verify commands use grep (not pytest/make) so the evidence gate is
# not triggered — this tests the sidecar update path, not the gate path.
MOCK_BIN="$TMP/bin"
mkdir -p "$MOCK_BIN"
cat > "$MOCK_BIN/claude" <<'SH'
#!/usr/bin/env bash
# Mock claude: ignore all args, emit a canned evaluator verdict to stdout.
cat <<'JSON'
{
  "verdict": "FAIL",
  "criteria": [
    {
      "id": "AC-2.1",
      "status": "met",
      "evidence_type": "grep",
      "evidence": "grep found 'feature' in src/feature.go"
    },
    {
      "id": "AC-2.2",
      "status": "unmet",
      "evidence_type": "grep",
      "evidence": "grep found 0 test files under tests/",
      "notes": "tests directory is empty — tests not yet written"
    }
  ],
  "meta": {"evaluator_mode": "headless"}
}
JSON
SH
chmod +x "$MOCK_BIN/claude"
export PATH="$MOCK_BIN:$PATH"

# ── 3. Stage evaluator template ───────────────────────────────────────────────
# The hook looks for the template at ~/.edikt/templates/agents/evaluator-headless.md
# or falls back to templates/agents/evaluator-headless.md relative to CWD.
# Seed a stub in the project so the CWD fallback resolves.
mkdir -p "$PROJECT/templates/agents"
echo "# Evaluator stub" > "$PROJECT/templates/agents/evaluator-headless.md"

# ── 4. Stop payload with PHASE 2 DONE completion signal ──────────────────────
PAYLOAD=$(python3 -c '
import json
print(json.dumps({
    "stop_hook_active": False,
    "last_assistant_message": "Implementation is complete. PHASE 2 DONE",
    "session_id": "test-session-001"
}))
')

# ── 5. Run the real hook ──────────────────────────────────────────────────────
HOOK_OUT=$(cd "$PROJECT" && \
  EDIKT_SKIP_SIDECAR_REGEN=1 \
  printf '%s' "$PAYLOAD" | bash "$HOOKS/phase-end-detector.sh" 2>/dev/null)

# ── 6. Assertions ─────────────────────────────────────────────────────────────

_assert() {
    local label="$1"
    local result="$2"  # "0" = pass
    if [ "$result" = "0" ]; then
        echo "  PASS: $label"
    else
        echo "  FAIL: $label"
        FAIL=1
    fi
}

# 6a. Hook output is valid JSON
python3 -c 'import json,sys; json.loads(sys.argv[1])' "$HOOK_OUT" 2>/dev/null
_assert "hook output is valid JSON" "$?"

# 6b. systemMessage present
echo "$HOOK_OUT" | python3 -c 'import json,sys; d=json.loads(sys.argv[1]); assert "systemMessage" in d' "$HOOK_OUT" 2>/dev/null
_assert "hook emits systemMessage" "$?"

SIDECAR="$PROJECT/docs/product/plans/PLAN-test-feature-criteria.yaml"

# 6c. AC-2.1 updated: status=pass, fail_count reset to 0
AC21_STATUS=$(python3 -c "
import re, sys
text = open(sys.argv[1]).read()
blocks = re.split(r'- id:', text)
for b in blocks:
    if b.strip().startswith('AC-2.1'):
        m = re.search(r'status:\s+(\S+)', b)
        print(m.group(1) if m else 'NOT_FOUND')
        break
" "$SIDECAR" 2>/dev/null)
[ "$AC21_STATUS" = "pass" ]
_assert "AC-2.1 status updated to pass (was fail)" "$?"

AC21_FAILCOUNT=$(python3 -c "
import re, sys
text = open(sys.argv[1]).read()
blocks = re.split(r'- id:', text)
for b in blocks:
    if b.strip().startswith('AC-2.1'):
        m = re.search(r'fail_count:\s+(\d+)', b)
        print(m.group(1) if m else 'NOT_FOUND')
        break
" "$SIDECAR" 2>/dev/null)
[ "$AC21_FAILCOUNT" = "0" ]
_assert "AC-2.1 fail_count reset to 0 (was 2)" "$?"

# 6d. AC-2.2 updated: status=fail, fail_count incremented to 1
AC22_STATUS=$(python3 -c "
import re, sys
text = open(sys.argv[1]).read()
blocks = re.split(r'- id:', text)
for b in blocks:
    if b.strip().startswith('AC-2.2'):
        m = re.search(r'status:\s+(\S+)', b)
        print(m.group(1) if m else 'NOT_FOUND')
        break
" "$SIDECAR" 2>/dev/null)
[ "$AC22_STATUS" = "fail" ]
_assert "AC-2.2 status updated to fail (was pending)" "$?"

AC22_FAILCOUNT=$(python3 -c "
import re, sys
text = open(sys.argv[1]).read()
blocks = re.split(r'- id:', text)
for b in blocks:
    if b.strip().startswith('AC-2.2'):
        m = re.search(r'fail_count:\s+(\d+)', b)
        print(m.group(1) if m else 'NOT_FOUND')
        break
" "$SIDECAR" 2>/dev/null)
[ "$AC22_FAILCOUNT" = "1" ]
_assert "AC-2.2 fail_count incremented to 1 (was 0)" "$?"

# 6e. AC-2.2 fail_reason populated
AC22_REASON=$(python3 -c "
import re, sys
text = open(sys.argv[1]).read()
blocks = re.split(r'- id:', text)
for b in blocks:
    if b.strip().startswith('AC-2.2'):
        m = re.search(r'fail_reason:\s+(.+)', b)
        val = (m.group(1) if m else '').strip()
        print('null' if val == 'null' else 'set')
        break
" "$SIDECAR" 2>/dev/null)
[ "$AC22_REASON" = "set" ]
_assert "AC-2.2 fail_reason populated (was null)" "$?"

# 6f. Top-level last_evaluated updated (not null)
TOP_EVAL=$(python3 -c "
import re, sys
text = open(sys.argv[1]).read()
m = re.match(r'(?s).*?^last_evaluated:\s+(.+)', text, re.MULTILINE)
print((m.group(1) if m else '').strip())
" "$SIDECAR" 2>/dev/null)
[ "$TOP_EVAL" != "null" ] && [ -n "$TOP_EVAL" ]
_assert "top-level last_evaluated updated (was null)" "$?"

# 6g. Verdict JSON persisted
VERDICT_FILE="$PROJECT/docs/product/plans/verdicts/PLAN-test-feature/phase-2.json"
[ -f "$VERDICT_FILE" ]
_assert "verdict JSON file created at verdicts/PLAN-test-feature/phase-2.json" "$?"

# 6h. Persisted verdict has correct verdict value
VERDICT_VAL=$(python3 -c "import json; d=json.load(open('$VERDICT_FILE')); print(d.get('verdict',''))" 2>/dev/null)
[ "$VERDICT_VAL" = "FAIL" ]
_assert "persisted verdict value is FAIL" "$?"

# 6i. Persisted verdict has evaluated_at timestamp
HAS_TS=$(python3 -c "import json; d=json.load(open('$VERDICT_FILE')); print('yes' if d.get('meta',{}).get('evaluated_at') else 'no')" 2>/dev/null)
[ "$HAS_TS" = "yes" ]
_assert "persisted verdict has meta.evaluated_at timestamp" "$?"

# 6j. Phase 1 criteria NOT modified (only target phase updated)
AC11_STATUS=$(python3 -c "
import re, sys
text = open(sys.argv[1]).read()
blocks = re.split(r'- id:', text)
for b in blocks:
    if b.strip().startswith('AC-1.1'):
        m = re.search(r'status:\s+(\S+)', b)
        print(m.group(1) if m else 'NOT_FOUND')
        break
" "$SIDECAR" 2>/dev/null)
[ "$AC11_STATUS" = "pass" ]
_assert "AC-1.1 (phase 1) status unchanged at pass" "$?"

if [ "$FAIL" = "0" ]; then
    echo ""
    echo "  ALL PASS: phase-end-detector sidecar+verdict e2e (10/10 assertions)"
fi
exit "$FAIL"
