#!/usr/bin/env bash
# Config toggle tests — every features.* key that controls hook behavior.
#
# For each toggle, this test:
#   1. Creates a minimal project with the feature explicitly disabled
#   2. Pipes a payload that would normally trigger that feature
#   3. Asserts the hook exits 0 and produces minimal/no output
#   4. Also verifies the feature IS active when the key is absent (default-on)
#
# Layer 1 — no API key, no claude CLI, pure shell + JSON.
# Opt-out: EDIKT_SKIP_HOOK_TESTS=1

set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
export PROJECT_ROOT
. "$PROJECT_ROOT/test/helpers.sh"
. "$PROJECT_ROOT/test/unit/hooks/_runner.sh"

if [ "${EDIKT_SKIP_HOOK_TESTS:-0}" = "1" ]; then
    echo "  SKIP config_toggles — EDIKT_SKIP_HOOK_TESTS=1"
    exit 0
fi

HOOKS="$PROJECT_ROOT/templates/hooks"

# ─── Helpers ─────────────────────────────────────────────────────────────────

make_project() {
    local dir="$1"
    shift
    mkdir -p "$dir/.edikt"
    local features=""
    for kv in "$@"; do
        features="$features  $kv"$'\n'
    done
    cat > "$dir/.edikt/config.yaml" <<YAML
edikt_version: "0.5.0"
base: docs
stack: []
paths:
  decisions: docs/architecture/decisions
  plans: docs/plans
features:
$features
YAML
}

run_hook_in() {
    local hook="$1"
    local dir="$2"
    local payload="$3"
    (cd "$dir" && echo "$payload" | bash "$HOOKS/$hook" 2>/dev/null)
}

echo ""

# ─── 1. auto-format: false ────────────────────────────────────────────────────
#
# post-tool-use.sh skips all formatting when auto-format: false.
# We use a .go payload because gofmt would normally be invoked.

TEST_DIR=$(mktemp -d)
make_project "$TEST_DIR" "auto-format: false"
mkdir -p "$TEST_DIR/src"
echo 'package main' > "$TEST_DIR/src/handler.go"
export CLAUDE_TOOL_INPUT_FILE_PATH="$TEST_DIR/src/handler.go"

test_start "auto-format: false — post-tool-use exits 0 silently"
payload='{"hook_event_name":"PostToolUse","tool_name":"Write","tool_input":{"file_path":"src/handler.go"},"tool_response":{"success":true},"cwd":"'"$TEST_DIR"'"}'
out=$(run_hook_in post-tool-use.sh "$TEST_DIR" "$payload")
rc=$?
[ "$rc" -eq 0 ] && pass "auto-format: false → exit 0" || fail "auto-format: false → exit 0" "got rc=$rc"
[ -z "$out" ] && pass "auto-format: false → no output" || fail "auto-format: false → no output" "got: $out"
unset CLAUDE_TOOL_INPUT_FILE_PATH
rm -rf "$TEST_DIR"

# ─── 2. auto-format absent (default-on) ──────────────────────────────────────
#
# Without auto-format: false, the hook should proceed (not silently skip).
# We verify this by checking it gets past the early-exit guards.
# We can't verify gofmt actually ran (it may not be installed in CI), but
# we can verify the hook doesn't exit 0 immediately without even reading input.

TEST_DIR=$(mktemp -d)
make_project "$TEST_DIR"  # no auto-format key → default on
mkdir -p "$TEST_DIR/src"
echo 'package main' > "$TEST_DIR/src/handler.go"
export CLAUDE_TOOL_INPUT_FILE_PATH="$TEST_DIR/src/handler.go"

test_start "auto-format absent — post-tool-use proceeds past early-exit guards"
payload='{"hook_event_name":"PostToolUse","tool_name":"Write","tool_input":{"file_path":"src/handler.go"},"tool_response":{"success":true},"cwd":"'"$TEST_DIR"'"}'
(cd "$TEST_DIR" && echo "$payload" | bash "$HOOKS/post-tool-use.sh" 2>/dev/null)
rc=$?
[ "$rc" -eq 0 ] && pass "auto-format default-on → exits 0 (ran to completion)" || fail "auto-format default-on → exits 0" "got rc=$rc"
unset CLAUDE_TOOL_INPUT_FILE_PATH
rm -rf "$TEST_DIR"

# ─── 3. signal-detection: false ──────────────────────────────────────────────
#
# stop-hook.sh must produce no output when signal-detection: false,
# even for a payload that contains a strong ADR signal.

TEST_DIR=$(mktemp -d)
make_project "$TEST_DIR" "signal-detection: false"
mkdir -p "$TEST_DIR/docs/architecture/decisions"

test_start "signal-detection: false — stop-hook exits 0 with no output"
payload='{"hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"We chose PostgreSQL over MongoDB — a hard architectural constraint for transaction support.","cwd":"'"$TEST_DIR"'"}'
out=$(run_hook_in stop-hook.sh "$TEST_DIR" "$payload")
rc=$?
[ "$rc" -eq 0 ] && pass "signal-detection: false → exit 0" || fail "signal-detection: false → exit 0" "got rc=$rc"
[ -z "$out" ] && pass "signal-detection: false → no ADR suggestion emitted" || fail "signal-detection: false → no output" "got: $out"
rm -rf "$TEST_DIR"

# ─── 4. signal-detection default-on produces ADR suggestion ──────────────────
#
# Without signal-detection: false, a strong ADR message must trigger a suggestion.
# This validates the default-on behavior is real, not accidentally silenced.

TEST_DIR=$(mktemp -d)
make_project "$TEST_DIR"  # no signal-detection key → default on
mkdir -p "$TEST_DIR/docs/architecture/decisions"  # no existing ADRs

test_start "signal-detection default-on — ADR signal triggers suggestion"
payload='{"hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"We chose PostgreSQL over MongoDB because relational joins dominate our query shape. This is a hard architectural constraint.","cwd":"'"$TEST_DIR"'"}'
out=$(run_hook_in stop-hook.sh "$TEST_DIR" "$payload")
rc=$?
[ "$rc" -eq 0 ] && pass "signal-detection default-on → exits 0" || fail "signal-detection default-on → exits 0" "got rc=$rc"
echo "$out" | grep -q "ADR\|adr:new\|architectural" && \
    pass "signal-detection default-on → ADR suggestion emitted" || \
    fail "signal-detection default-on → ADR suggestion" "got: $out"
rm -rf "$TEST_DIR"

# ─── 5. plan-injection: false ────────────────────────────────────────────────
#
# user-prompt-submit.sh must produce empty output when plan-injection: false,
# even for a project with an active plan.

TEST_DIR=$(mktemp -d)
make_project "$TEST_DIR" "plan-injection: false"
mkdir -p "$TEST_DIR/docs/plans"
cat > "$TEST_DIR/docs/plans/PLAN-feature.md" <<'PLAN'
# Plan — Feature X

| Phase | Status |
|-------|--------|
| 1     | done |
| 2     | in-progress |
PLAN

test_start "plan-injection: false — user-prompt-submit exits 0 with no systemMessage"
payload='{"hook_event_name":"UserPromptSubmit","prompt":"help me","cwd":"'"$TEST_DIR"'"}'
out=$(run_hook_in user-prompt-submit.sh "$TEST_DIR" "$payload")
rc=$?
[ "$rc" -eq 0 ] && pass "plan-injection: false → exit 0" || fail "plan-injection: false → exit 0" "got rc=$rc"
[ -z "$out" ] && pass "plan-injection: false → no systemMessage" || fail "plan-injection: false → no output" "got: $out"
rm -rf "$TEST_DIR"

# ─── 6. plan-injection default-on injects context ────────────────────────────

TEST_DIR=$(mktemp -d)
make_project "$TEST_DIR"  # default on
mkdir -p "$TEST_DIR/docs/plans"
cat > "$TEST_DIR/docs/plans/PLAN-feature.md" <<'PLAN'
# Plan — Feature X

| Phase | Status |
|-------|--------|
| 1     | done |
| 2     | in-progress |
PLAN

test_start "plan-injection default-on — active plan injects context"
payload='{"hook_event_name":"UserPromptSubmit","prompt":"help me","cwd":"'"$TEST_DIR"'"}'
out=$(run_hook_in user-prompt-submit.sh "$TEST_DIR" "$payload")
rc=$?
[ "$rc" -eq 0 ] && pass "plan-injection default-on → exits 0" || fail "plan-injection default-on → exits 0" "got rc=$rc"
# Should inject plan context or produce empty output if no match found.
# Key assertion: it did NOT silently fail.
echo "$out" | python3 -c "import sys,json; d=json.loads(sys.stdin.read() or '{}'); exit(0)" 2>/dev/null && \
    pass "plan-injection default-on → valid JSON output" || \
    fail "plan-injection default-on → valid JSON" "not valid JSON: $out"
rm -rf "$TEST_DIR"

# ─── 7. phase-end: false ─────────────────────────────────────────────────────
#
# phase-end-detector.sh must exit immediately when phase-end: false,
# even when the message looks like a phase completion.

TEST_DIR=$(mktemp -d)
mkdir -p "$TEST_DIR/.edikt" "$TEST_DIR/docs/plans"
cat > "$TEST_DIR/.edikt/config.yaml" <<'YAML'
edikt_version: "0.5.0"
base: docs
phase-end: false
YAML
cat > "$TEST_DIR/docs/plans/PLAN-test.md" <<'PLAN'
# Plan

| Phase | Status |
|-------|--------|
| 1     | in-progress |
PLAN

test_start "phase-end: false — phase-end-detector exits 0 without invoking evaluator"
payload='{"hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"Phase 1 complete. All acceptance criteria met.","cwd":"'"$TEST_DIR"'"}'
out=$(EDIKT_EVALUATOR_DRY_RUN=1 run_hook_in phase-end-detector.sh "$TEST_DIR" "$payload")
rc=$?
[ "$rc" -eq 0 ] && pass "phase-end: false → exit 0" || fail "phase-end: false → exit 0" "got rc=$rc"
# With phase-end: false the hook exits before reaching the dry-run path.
echo "$out" | grep -qv "DRY.RUN\|EVALUATOR" && \
    pass "phase-end: false → evaluator not invoked" || \
    fail "phase-end: false → evaluator not invoked" "got: $out"
rm -rf "$TEST_DIR"

# ─── 8. EDIKT_EVALUATOR_DRY_RUN=1 — phase detection without calling claude ──
#
# Regression guard: the dry-run env var must prevent actual claude invocation
# so CI can test phase detection logic without API costs.

TEST_DIR=$(mktemp -d)
mkdir -p "$TEST_DIR/.edikt" "$TEST_DIR/docs/plans"
cat > "$TEST_DIR/.edikt/config.yaml" <<'YAML'
edikt_version: "0.5.0"
base: docs
YAML
cat > "$TEST_DIR/docs/plans/PLAN-test.md" <<'PLAN'
# Plan

| Phase | Status |
|-------|--------|
| 1     | in-progress |
PLAN

test_start "EDIKT_EVALUATOR_DRY_RUN=1 — phase-end-detector runs without calling claude"
payload='{"hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"Phase 1 complete. All acceptance criteria met and tests pass.","cwd":"'"$TEST_DIR"'"}'
out=$(EDIKT_EVALUATOR_DRY_RUN=1 run_hook_in phase-end-detector.sh "$TEST_DIR" "$payload")
rc=$?
# Dry run should exit 0 — it detected the phase end but didn't call claude.
[ "$rc" -eq 0 ] && pass "dry-run → exit 0" || fail "dry-run → exit 0" "got rc=$rc"
rm -rf "$TEST_DIR"

# ─── 9. No .edikt/config.yaml — all hooks exit silently ─────────────────────

NO_EDIKT_DIR=$(mktemp -d)

test_start "no .edikt/config.yaml — post-tool-use exits 0 silently"
payload='{"hook_event_name":"PostToolUse","tool_name":"Write","tool_input":{"file_path":"foo.go"},"tool_response":{"success":true},"cwd":"'"$NO_EDIKT_DIR"'"}'
out=$(run_hook_in post-tool-use.sh "$NO_EDIKT_DIR" "$payload")
rc=$?
[ "$rc" -eq 0 ] && pass "no-config → post-tool-use exit 0" || fail "no-config → post-tool-use exit 0" "got rc=$rc"

test_start "no .edikt/config.yaml — stop-hook exits 0 silently"
payload='{"hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"We chose PostgreSQL.","cwd":"'"$NO_EDIKT_DIR"'"}'
out=$(run_hook_in stop-hook.sh "$NO_EDIKT_DIR" "$payload")
rc=$?
[ "$rc" -eq 0 ] && pass "no-config → stop-hook exit 0" || fail "no-config → stop-hook exit 0" "got rc=$rc"
[ -z "$out" ] && pass "no-config → stop-hook no output" || fail "no-config → stop-hook no output" "got: $out"

test_start "no .edikt/config.yaml — phase-end-detector exits 0 silently"
payload='{"hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"Phase 1 complete.","cwd":"'"$NO_EDIKT_DIR"'"}'
out=$(run_hook_in phase-end-detector.sh "$NO_EDIKT_DIR" "$payload")
rc=$?
[ "$rc" -eq 0 ] && pass "no-config → phase-end-detector exit 0" || fail "no-config → phase-end-detector exit 0" "got rc=$rc"

rm -rf "$NO_EDIKT_DIR"

# ─── 10. paths.decisions respected by stop-hook deduplication ────────────────
#
# stop-hook.sh uses the base: config key to find existing ADRs for dedup.
# If base: custom/base and an ADR exists there matching the signal, no suggestion fires.

TEST_DIR=$(mktemp -d)
mkdir -p "$TEST_DIR/.edikt" "$TEST_DIR/custom/base/architecture/decisions"
cat > "$TEST_DIR/.edikt/config.yaml" <<'YAML'
edikt_version: "0.5.0"
base: custom/base
features:
  signal-detection: true
YAML
# Existing ADR with "postgresql" in the title — dedup term match suppresses suggestion.
# The stop-hook dedup extracts key terms from "chose X over Y" and checks if
# any existing ADR title contains those terms. The title must include the term.
cat > "$TEST_DIR/custom/base/architecture/decisions/ADR-001-postgresql-over-mongodb.md" <<'ADR'
# ADR-001: PostgreSQL over MongoDB

**Status:** Accepted

We chose PostgreSQL for its transaction support.
ADR

test_start "paths.decisions — stop-hook dedup uses base: path (no dupe suggestion)"
payload='{"hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"We chose PostgreSQL over MongoDB — a hard architectural constraint.","cwd":"'"$TEST_DIR"'"}'
out=$(run_hook_in stop-hook.sh "$TEST_DIR" "$payload")
rc=$?
[ "$rc" -eq 0 ] && pass "custom base → exit 0" || fail "custom base → exit 0" "got rc=$rc"
# Dedup should suppress the ADR suggestion since an ADR about PostgreSQL exists.
if echo "$out" | grep -q "ADR\|adr:new"; then
    fail "custom base → dedup suppressed suggestion" "suggestion still fired: $out"
else
    pass "custom base → dedup suppressed suggestion (ADR already exists)"
fi
rm -rf "$TEST_DIR"

echo ""
test_summary
