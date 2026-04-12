#!/bin/bash
# Test: v0.4.0 SPEC-001 plan harness — iteration tracking, context handoff, criteria sidecar
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

PLAN_CMD="$PROJECT_ROOT/commands/sdlc/plan.md"
HOOK="$PROJECT_ROOT/templates/hooks/post-compact.sh"
SCHEMA="$PROJECT_ROOT/docs/product/specs/SPEC-001-plan-harness/plan-criteria-schema.yaml"

echo ""

# ============================================================
# TEST 1: Iteration Tracking (Phase 1 verification)
# ============================================================

echo -e "${BOLD}TEST 1: Iteration tracking${NC}"

assert_file_contains "$PLAN_CMD" "| Phase | Status | Attempt | Updated |" \
  "Plan.md progress table has Attempt column"

for status in pending in-progress evaluating stuck done skipped; do
  assert_file_contains "$PLAN_CMD" "$status" \
    "Plan.md documents status value: $status"
done

assert_file_contains "$PLAN_CMD" "criteria.yaml" \
  "Plan.md phase-end flow references criteria sidecar"

assert_file_contains "$PLAN_CMD" "Previous attempt failed" \
  "Plan.md has fail forwarding instruction"

assert_file_contains "$PLAN_CMD" "failed 3 consecutive" \
  "Plan.md has escalation at 3 failures"

assert_file_contains "$PLAN_CMD" "stuck" \
  "Plan.md has stuck status"

assert_file_contains "$PLAN_CMD" "Continue trying" \
  "Plan.md has 'Continue trying' option"

assert_file_contains "$PLAN_CMD" "Skip this phase" \
  "Plan.md has 'Skip this phase' option"

assert_file_contains "$PLAN_CMD" "Rewrite failing criteria" \
  "Plan.md has 'Rewrite failing criteria' option"

assert_file_contains "$PLAN_CMD" "Stop and review" \
  "Plan.md has 'Stop and review' option"

assert_file_contains "$PLAN_CMD" "max-attempts" \
  "Plan.md reads evaluator.max-attempts config"

# ============================================================
# TEST 2: Context Handoff (Phase 2 verification)
# ============================================================

echo ""
echo -e "${BOLD}TEST 2: Context handoff${NC}"

assert_file_contains "$PLAN_CMD" "Context Needed" \
  "Plan.md Phase Structure includes Context Needed field"

assert_file_contains "$PLAN_CMD" "Artifact Flow" \
  "Plan.md template has Artifact Flow section"

assert_file_contains "$PLAN_CMD" "Producing Phase" \
  "Plan.md Artifact Flow table has Producing Phase column"

assert_file_contains "$PLAN_CMD" "Consuming Phase" \
  "Plan.md Artifact Flow table has Consuming Phase column"

assert_file_contains "$PLAN_CMD" "Before implementing any plan phase" \
  "Plan.md has phase startup directive"

assert_file_contains "$PLAN_CMD" "Context Needed section" \
  "Plan.md startup directive references Context Needed section"

# ============================================================
# TEST 3: Criteria Sidecar (Phase 3 verification)
# ============================================================

echo ""
echo -e "${BOLD}TEST 3: Criteria sidecar${NC}"

assert_file_contains "$PLAN_CMD" "criteria.yaml" \
  "Plan.md has sidecar emission step"

assert_file_contains "$PLAN_CMD" "sibling" \
  "Plan.md sidecar is always a sibling of the plan file"

assert_file_contains "$PLAN_CMD" "fail_count" \
  "Plan.md phase-end updates sidecar fail_count"

assert_file_contains "$PLAN_CMD" "fail_reason" \
  "Plan.md phase-end updates sidecar fail_reason"

assert_file_contains "$PLAN_CMD" "last_evaluated" \
  "Plan.md phase-end updates sidecar last_evaluated"

assert_file_exists "$SCHEMA" \
  "Sidecar schema reference file exists"

# ============================================================
# TEST 4: PostCompact Hook — structural assertions
# ============================================================

echo ""
echo -e "${BOLD}TEST 4: PostCompact hook structure${NC}"

assert_file_contains "$HOOK" "ATTEMPT" \
  "PostCompact hook extracts ATTEMPT variable"

assert_file_contains "$HOOK" "criteria.yaml" \
  "PostCompact hook reads criteria sidecar"

assert_file_contains "$HOOK" "fail" \
  "PostCompact hook processes failing criteria"

assert_file_contains "$HOOK" "Context Needed" \
  "PostCompact hook reads Context Needed from plan"

assert_file_contains "$HOOK" "grep -qE" \
  "PostCompact hook has backward compatibility check (grep -qE)"

# ============================================================
# TEST 5: PostCompact Hook Regex — functional tests
# ============================================================

echo ""
echo -e "${BOLD}TEST 5: PostCompact hook regex (functional)${NC}"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

mkdir -p "$TMPDIR/docs/plans"

# 4-column table (new format with Attempt column)
cat > "$TMPDIR/docs/plans/PLAN-test.md" << 'PLAN'
# Plan: Test Plan

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | done   | 1/5     | 2026-04-11 |
| 2     | in-progress | 2/5 | 2026-04-11 |
| 3     | pending | 0/5    | - |
PLAN

# Extract using the same pipeline as the hook
PHASE_LINE=$(grep -iE '\| *(Phase )?[0-9]+ *\|.*in[_ -]progress' \
  "$TMPDIR/docs/plans/PLAN-test.md" | head -1)
PHASE_NUM=$(echo "$PHASE_LINE" | sed 's/|/\n/g' | sed -n '2p' | tr -d ' ' | grep -oE '[0-9]+')
ATTEMPT=$(echo "$PHASE_LINE" | sed 's/|/\n/g' | sed -n '4p' | sed 's/^ *//;s/ *$//')

if [ "$PHASE_NUM" = "2" ]; then
  pass "4-column table: PHASE_NUM is '2'"
else
  fail "4-column table: PHASE_NUM is '2'" "Got: '$PHASE_NUM'"
fi

if [ "$ATTEMPT" = "2/5" ]; then
  pass "4-column table: ATTEMPT is '2/5'"
else
  fail "4-column table: ATTEMPT is '2/5'" "Got: '$ATTEMPT'"
fi

# Backward compatibility: 3-column table (no Attempt column)
cat > "$TMPDIR/docs/plans/PLAN-old.md" << 'PLAN'
# Plan: Old Plan

## Progress

| Phase | Status | Updated |
|-------|--------|---------|
| 1     | done   | 2026-04-11 |
| 2     | in-progress | 2026-04-11 |
PLAN

PHASE_LINE_OLD=$(grep -iE '\| *(Phase )?[0-9]+ *\|.*in[_ -]progress' \
  "$TMPDIR/docs/plans/PLAN-old.md" | head -1)
PHASE_NUM_OLD=$(echo "$PHASE_LINE_OLD" | sed 's/|/\n/g' | sed -n '2p' | tr -d ' ' | grep -oE '[0-9]+')
ATTEMPT_OLD=$(echo "$PHASE_LINE_OLD" | sed 's/|/\n/g' | sed -n '4p' | sed 's/^ *//;s/ *$//')

if [ "$PHASE_NUM_OLD" = "2" ]; then
  pass "3-column table (backward compat): PHASE_NUM is '2'"
else
  fail "3-column table (backward compat): PHASE_NUM is '2'" "Got: '$PHASE_NUM_OLD'"
fi

# ATTEMPT_OLD column 4 in old table is the date — should NOT match N/N pattern
if echo "$ATTEMPT_OLD" | grep -qE '^[0-9]+/[0-9]+$'; then
  fail "3-column table (backward compat): ATTEMPT does not match N/N pattern" \
    "Got: '$ATTEMPT_OLD' — should not be a valid attempt"
else
  pass "3-column table (backward compat): ATTEMPT does not match N/N — no crash"
fi

test_summary
