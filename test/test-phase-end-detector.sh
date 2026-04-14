#!/bin/bash
# Test: phase-end-detector.sh auto-fires the evaluator when Claude completes a phase
# Covers v0.4.3 bug: phase-end evaluator never fired because there was no hook
# detecting completion signals mid-session.
set -uo pipefail

PROJECT_ROOT="$(cd "${1:-.}" && pwd)"
source "$(dirname "$0")/helpers.sh"

HOOK="$PROJECT_ROOT/templates/hooks/phase-end-detector.sh"
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo ""

# ============================================================
# TEST 1: Hook exists and is executable
# ============================================================

echo -e "${BOLD}TEST 1: Hook file structure${NC}"

assert_file_exists "$HOOK"

if [ -x "$HOOK" ]; then
    pass "Hook is executable"
else
    fail "Hook is not executable"
fi

# Hook is registered in settings template
assert_file_contains "$PROJECT_ROOT/templates/settings.json.tmpl" \
    "phase-end-detector.sh" "Stop hook registers phase-end-detector"

# ============================================================
# TEST 2: Hook exits silently when not in an edikt project
# ============================================================

echo ""
echo -e "${BOLD}TEST 2: Skip when not in edikt project${NC}"

EMPTY="$TMPDIR/empty"
mkdir -p "$EMPTY"
cd "$EMPTY"

OUTPUT=$(echo '{"last_assistant_message":"Phase 1 done"}' | "$HOOK" 2>&1)
EXIT=$?

if [ $EXIT -eq 0 ] && [ -z "$OUTPUT" ]; then
    pass "Hook exits silently when .edikt/config.yaml missing"
else
    fail "Hook should exit silently outside edikt project (exit=$EXIT, output=$OUTPUT)"
fi

# ============================================================
# TEST 3: Hook respects evaluator.phase-end: false config
# ============================================================

echo ""
echo -e "${BOLD}TEST 3: Config skip path${NC}"

PROJ="$TMPDIR/proj-disabled"
mkdir -p "$PROJ/.edikt" "$PROJ/docs/plans"
cat > "$PROJ/.edikt/config.yaml" << 'YAML'
base: docs
paths:
  plans: docs/plans
evaluator:
  phase-end: false
YAML

cat > "$PROJ/docs/plans/PLAN-test.md" << 'PLAN'
# Plan: Test

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | in-progress | 1/5 | 2026-04-14 |
PLAN

cd "$PROJ"
OUTPUT=$(echo '{"last_assistant_message":"Phase 1 done"}' | EDIKT_EVALUATOR_DRY_RUN=1 "$HOOK" 2>&1)

if [ -z "$OUTPUT" ] || echo "$OUTPUT" | grep -q "continue"; then
    pass "Hook skips when phase-end: false in config"
else
    fail "Hook should skip when phase-end disabled (output=$OUTPUT)"
fi

# ============================================================
# TEST 4: Hook skips when stop_hook_active (loop prevention)
# ============================================================

echo ""
echo -e "${BOLD}TEST 4: Loop prevention${NC}"

PROJ="$TMPDIR/proj-active"
mkdir -p "$PROJ/.edikt" "$PROJ/docs/plans"
cat > "$PROJ/.edikt/config.yaml" << 'YAML'
base: docs
paths:
  plans: docs/plans
evaluator:
  phase-end: true
YAML

cat > "$PROJ/docs/plans/PLAN-test.md" << 'PLAN'
# Plan: Test

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | in-progress | 1/5 | 2026-04-14 |
PLAN

cd "$PROJ"
OUTPUT=$(echo '{"stop_hook_active":true,"last_assistant_message":"Phase 1 done"}' | "$HOOK" 2>&1)

if [ -z "$OUTPUT" ]; then
    pass "Hook exits silently when stop_hook_active=true"
else
    fail "Hook should exit silently on stop_hook_active (output=$OUTPUT)"
fi

# ============================================================
# TEST 5: Hook detects completion signal (pattern 1: "Phase N complete")
# ============================================================

echo ""
echo -e "${BOLD}TEST 5: Detect 'Phase N complete' pattern${NC}"

PROJ="$TMPDIR/proj-pattern1"
mkdir -p "$PROJ/.edikt" "$PROJ/docs/plans"
cat > "$PROJ/.edikt/config.yaml" << 'YAML'
base: docs
paths:
  plans: docs/plans
evaluator:
  phase-end: true
YAML

cat > "$PROJ/docs/plans/PLAN-test.md" << 'PLAN'
# Plan: Test

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | in-progress | 1/5 | 2026-04-14 |
PLAN

cd "$PROJ"
OUTPUT=$(echo '{"last_assistant_message":"Great news — Phase 1 is complete. Running tests."}' | EDIKT_EVALUATOR_DRY_RUN=1 "$HOOK" 2>&1)

if echo "$OUTPUT" | grep -q "Phase 1 completion detected"; then
    pass "Hook detects 'Phase N complete' pattern"
else
    fail "Hook should detect 'Phase N complete' (output=$OUTPUT)"
fi

# ============================================================
# TEST 6: Hook detects completion signal (pattern 2: "Completed phase N")
# ============================================================

echo ""
echo -e "${BOLD}TEST 6: Detect 'Completed phase N' pattern${NC}"

cd "$PROJ"
OUTPUT=$(echo '{"last_assistant_message":"Implemented phase 1 with all 5 tests passing."}' | EDIKT_EVALUATOR_DRY_RUN=1 "$HOOK" 2>&1)

if echo "$OUTPUT" | grep -q "Phase 1 completion detected"; then
    pass "Hook detects 'Implemented phase N' pattern"
else
    fail "Hook should detect 'Implemented phase N' (output=$OUTPUT)"
fi

# ============================================================
# TEST 7: Hook detects completion promise format (PHASE N DONE)
# ============================================================

echo ""
echo -e "${BOLD}TEST 7: Detect completion promise format${NC}"

cd "$PROJ"
OUTPUT=$(echo '{"last_assistant_message":"PHASE 1 MIGRATION DONE"}' | EDIKT_EVALUATOR_DRY_RUN=1 "$HOOK" 2>&1)

if echo "$OUTPUT" | grep -q "Phase 1 completion detected"; then
    pass "Hook detects PHASE N ... DONE format"
else
    fail "Hook should detect PHASE N DONE format (output=$OUTPUT)"
fi

# ============================================================
# TEST 8: Hook does NOT fire on unrelated messages
# ============================================================

echo ""
echo -e "${BOLD}TEST 8: No false positives${NC}"

cd "$PROJ"
# Normal work message — no completion signal
OUTPUT=$(echo '{"last_assistant_message":"I added a new function to handle retries."}' | EDIKT_EVALUATOR_DRY_RUN=1 "$HOOK" 2>&1)

if echo "$OUTPUT" | grep -q "continue"; then
    pass "Hook does not fire on unrelated messages"
else
    fail "Hook should not fire on normal work messages (output=$OUTPUT)"
fi

# ============================================================
# TEST 9: Hook exits silently when no plan exists
# ============================================================

echo ""
echo -e "${BOLD}TEST 9: No active plan${NC}"

PROJ="$TMPDIR/proj-noplan"
mkdir -p "$PROJ/.edikt" "$PROJ/docs/plans"
cat > "$PROJ/.edikt/config.yaml" << 'YAML'
base: docs
paths:
  plans: docs/plans
evaluator:
  phase-end: true
YAML

cd "$PROJ"
OUTPUT=$(echo '{"last_assistant_message":"Phase 1 complete"}' | EDIKT_EVALUATOR_DRY_RUN=1 "$HOOK" 2>&1)

if echo "$OUTPUT" | grep -q "continue"; then
    pass "Hook exits cleanly when no plan file exists"
else
    fail "Hook should exit cleanly without a plan (output=$OUTPUT)"
fi

# ============================================================
# TEST 10: Hook exits silently when no phase is in-progress
# ============================================================

echo ""
echo -e "${BOLD}TEST 10: No in-progress phase${NC}"

PROJ="$TMPDIR/proj-noinprogress"
mkdir -p "$PROJ/.edikt" "$PROJ/docs/plans"
cat > "$PROJ/.edikt/config.yaml" << 'YAML'
base: docs
paths:
  plans: docs/plans
evaluator:
  phase-end: true
YAML

cat > "$PROJ/docs/plans/PLAN-test.md" << 'PLAN'
# Plan: Test

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | done   | 1/5 | 2026-04-14 |
| 2     | pending | 0/5 | 2026-04-14 |
PLAN

cd "$PROJ"
OUTPUT=$(echo '{"last_assistant_message":"Phase 1 complete"}' | EDIKT_EVALUATOR_DRY_RUN=1 "$HOOK" 2>&1)

if echo "$OUTPUT" | grep -q "continue"; then
    pass "Hook exits cleanly when no phase is in-progress"
else
    fail "Hook should exit cleanly with no in-progress phase (output=$OUTPUT)"
fi

# ============================================================
# TEST 11: Hook logs detection event to events.jsonl
# ============================================================

echo ""
echo -e "${BOLD}TEST 11: Event logging${NC}"

PROJ="$TMPDIR/proj-events"
mkdir -p "$PROJ/.edikt" "$PROJ/docs/plans"
HOME_BACKUP="$HOME"
FAKE_HOME="$TMPDIR/fake-home"
mkdir -p "$FAKE_HOME/.edikt"

cat > "$PROJ/.edikt/config.yaml" << 'YAML'
base: docs
paths:
  plans: docs/plans
evaluator:
  phase-end: true
YAML

cat > "$PROJ/docs/plans/PLAN-test.md" << 'PLAN'
# Plan: Test

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 2     | in-progress | 1/5 | 2026-04-14 |
PLAN

cd "$PROJ"
HOME="$FAKE_HOME" EDIKT_EVALUATOR_DRY_RUN=1 bash -c "echo '{\"last_assistant_message\":\"Phase 2 complete\"}' | \"$HOOK\"" > /dev/null 2>&1

if [ -f "$FAKE_HOME/.edikt/events.jsonl" ] && grep -q "phase_completion_detected" "$FAKE_HOME/.edikt/events.jsonl"; then
    pass "Hook logs phase_completion_detected event"
else
    fail "Hook should log completion event to events.jsonl"
fi

# Event contains correct phase number
if grep -q '"phase":2' "$FAKE_HOME/.edikt/events.jsonl" 2>/dev/null; then
    pass "Event log contains correct phase number (2)"
else
    fail "Event log missing phase:2"
fi

# ============================================================
# TEST 12: Hook picks the correct in-progress phase when multiple phases exist
# ============================================================

echo ""
echo -e "${BOLD}TEST 12: Correct phase selection${NC}"

PROJ="$TMPDIR/proj-multiphase"
mkdir -p "$PROJ/.edikt" "$PROJ/docs/plans"
cat > "$PROJ/.edikt/config.yaml" << 'YAML'
base: docs
paths:
  plans: docs/plans
evaluator:
  phase-end: true
YAML

cat > "$PROJ/docs/plans/PLAN-test.md" << 'PLAN'
# Plan: Test

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | done   | 1/5 | 2026-04-14 |
| 2     | done   | 2/5 | 2026-04-14 |
| 3     | in-progress | 1/5 | 2026-04-14 |
| 4     | pending | 0/5 | - |
PLAN

cd "$PROJ"
OUTPUT=$(echo '{"last_assistant_message":"Phase 3 complete"}' | EDIKT_EVALUATOR_DRY_RUN=1 "$HOOK" 2>&1)

if echo "$OUTPUT" | grep -q "Phase 3 completion detected"; then
    pass "Hook correctly identifies phase 3 as in-progress"
else
    fail "Hook should identify phase 3 (output=$OUTPUT)"
fi

# Make sure it does NOT fire for phase 1 mention when phase 3 is the in-progress one
cd "$PROJ"
OUTPUT=$(echo '{"last_assistant_message":"Phase 1 complete (reminder)"}' | EDIKT_EVALUATOR_DRY_RUN=1 "$HOOK" 2>&1)

if echo "$OUTPUT" | grep -q "continue"; then
    pass "Hook does not fire when completion signal refers to a different phase"
else
    fail "Hook should not fire for non-in-progress phase completion (output=$OUTPUT)"
fi

# ============================================================
# TEST 13: Test helper override (EDIKT_SKIP_PHASE_EVAL)
# ============================================================

echo ""
echo -e "${BOLD}TEST 13: Skip override${NC}"

PROJ="$TMPDIR/proj-skip"
mkdir -p "$PROJ/.edikt" "$PROJ/docs/plans"
cat > "$PROJ/.edikt/config.yaml" << 'YAML'
base: docs
paths:
  plans: docs/plans
evaluator:
  phase-end: true
YAML

cat > "$PROJ/docs/plans/PLAN-test.md" << 'PLAN'
# Plan: Test

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | in-progress | 1/5 | 2026-04-14 |
PLAN

cd "$PROJ"
OUTPUT=$(EDIKT_SKIP_PHASE_EVAL=1 bash -c "echo '{\"last_assistant_message\":\"Phase 1 complete\"}' | \"$HOOK\"" 2>&1)

if [ -z "$OUTPUT" ]; then
    pass "EDIKT_SKIP_PHASE_EVAL=1 skips evaluation"
else
    fail "EDIKT_SKIP_PHASE_EVAL should cause silent exit (output=$OUTPUT)"
fi

cd "$PROJECT_ROOT"
test_summary
