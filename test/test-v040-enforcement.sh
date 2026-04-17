#!/bin/bash
# Test: v0.4.0 SPEC-003 enforcement — quality gate UX, artifact lifecycle
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

HOOK="$PROJECT_ROOT/templates/hooks/subagent-stop.sh"
SESSION_HOOK="$PROJECT_ROOT/templates/hooks/session-start.sh"
PLAN_CMD="$PROJECT_ROOT/commands/sdlc/plan.md"
DRIFT_CMD="$PROJECT_ROOT/commands/sdlc/drift.md"
DOCTOR_CMD="$PROJECT_ROOT/commands/doctor.md"

echo ""

# ============================================================
# TEST 1: Quality gate override logging
# ============================================================

echo -e "${BOLD}TEST 1: Gate override logging${NC}"

assert_file_contains "$HOOK" "events.jsonl" \
  "Hook writes to events.jsonl"

assert_file_contains "$HOOK" "gate_fired" \
  "Hook logs gate_fired event"

# v0.5.0 INV-004 hardening consolidated the per-outcome events (gate_override,
# gate_blocked) into a single gate_fired event plus a silent override branch.
# The audit trail still distinguishes the two cases: override matches skip
# silently (no event, {"continue": true}) and gate fires write gate_fired with
# decision=block. The assertions below cover both branches.
if grep -q '"decision": "block"' "$HOOK" && grep -q 'gate_fired' "$HOOK"; then
  pass "Hook emits block decision with gate_fired event"
else
  fail "Hook should emit block decision with gate_fired event"
fi

# Override branch must exit with continue:true and NOT emit a block decision
# within the same code path. We grep the override-match block (100 lines of
# context are plenty for the python3 heredoc) for a continue-true emission.
if awk '/gate-overrides.jsonl/{flag=1} flag{print} flag && /exit 0/{exit}' "$HOOK" | grep -q '"continue": true'; then
  pass "Override branch returns continue:true"
else
  fail "Override branch should return continue:true"
fi

assert_file_contains "$HOOK" "git config user.name" \
  "Hook captures git user name"

assert_file_contains "$HOOK" "git config user.email" \
  "Hook captures git email"

# ============================================================
# TEST 2: Gate re-fire prevention
# ============================================================

echo ""
echo -e "${BOLD}TEST 2: Gate re-fire prevention${NC}"

assert_file_contains "$HOOK" "gate-overrides.jsonl" \
  "Hook checks override file"

assert_file_contains "$HOOK" "finding_prefix" \
  "Hook matches on finding prefix"

assert_file_contains "$HOOK" "cut -c1-80" \
  "Finding prefix is first 80 chars"

# Override check returns continue when matched. The override-match branch can
# span many lines (python3 heredoc for JSON parsing), so widen the window to
# the full sequence between the gate-overrides.jsonl reference and its exit.
if awk '/gate-overrides.jsonl/{flag=1} flag{print} flag && /exit 0/{exit}' "$HOOK" | grep -q '"continue": true'; then
  pass "Override check returns continue when matched"
else
  fail "Override check should return continue when matched"
fi

# Override check comes BEFORE gate firing. v0.5.0 consolidated the block path
# into the gate_fired event itself; use that as the firing marker.
OVERRIDE_LINE=$(grep -n "gate-overrides.jsonl" "$HOOK" | head -1 | cut -d: -f1)
GATE_FIRE_LINE=$(grep -n '"event": "gate_fired"' "$HOOK" | head -1 | cut -d: -f1)
if [ -n "$OVERRIDE_LINE" ] && [ -n "$GATE_FIRE_LINE" ] && [ "$OVERRIDE_LINE" -lt "$GATE_FIRE_LINE" ]; then
  pass "Override check comes before gate firing"
else
  fail "Override check must come before gate firing (override: line $OVERRIDE_LINE, fire: line $GATE_FIRE_LINE)"
fi

# ============================================================
# TEST 3: Session cleanup
# ============================================================

echo ""
echo -e "${BOLD}TEST 3: Session cleanup${NC}"

assert_file_contains "$SESSION_HOOK" "gate-overrides.jsonl" \
  "Session start clears override file"

# ============================================================
# TEST 4: Plan draft artifact warning
# ============================================================

echo ""
echo -e "${BOLD}TEST 4: Plan draft warning${NC}"

assert_file_contains "$PLAN_CMD" "still in draft" \
  "Plan has draft artifact warning"

assert_file_contains "$PLAN_CMD" "Proceed anyway" \
  "Plan offers proceed option"

assert_file_contains "$PLAN_CMD" "review and accept" \
  "Plan offers stop option"

assert_file_contains "$PLAN_CMD" "Known Risks" \
  "Plan has Known Risks section"

assert_file_contains "$PLAN_CMD" "draft artifacts" \
  "Known Risks mentions draft artifacts"

# ============================================================
# TEST 5: Plan auto-promote
# ============================================================

echo ""
echo -e "${BOLD}TEST 5: Plan auto-promote${NC}"

assert_file_contains "$PLAN_CMD" "in-progress" \
  "Plan references in-progress status"

# Plan has accepted → in-progress promotion instruction
if grep -qi "accepted.*in-progress\|status.*promoted\|promote.*accepted" "$PLAN_CMD" 2>/dev/null; then
  pass "Plan has accepted → in-progress promotion"
else
  fail "Plan missing accepted → in-progress promotion"
fi

# ============================================================
# TEST 6: Drift status filter
# ============================================================

echo ""
echo -e "${BOLD}TEST 6: Drift status filter${NC}"

assert_file_contains "$DRIFT_CMD" "draft" \
  "Drift references draft status"

# Drift skips draft artifacts
if grep -qi "skip.*draft\|skipping.*draft" "$DRIFT_CMD" 2>/dev/null; then
  pass "Drift skips draft artifacts"
else
  fail "Drift should skip draft artifacts"
fi

# Drift skips superseded artifacts
if grep -qi "skip.*superseded\|skipping.*superseded" "$DRIFT_CMD" 2>/dev/null; then
  pass "Drift skips superseded artifacts"
else
  fail "Drift should skip superseded artifacts"
fi

assert_file_contains "$DRIFT_CMD" "accepted" \
  "Drift validates accepted artifacts"

assert_file_contains "$DRIFT_CMD" "implemented" \
  "Drift validates implemented artifacts"

# ============================================================
# TEST 7: Drift auto-promote
# ============================================================

echo ""
echo -e "${BOLD}TEST 7: Drift auto-promote${NC}"

# Drift has in-progress → implemented promotion
if grep -qi "in-progress.*implemented\|promoted.*implemented\|no drift.*implemented" "$DRIFT_CMD" 2>/dev/null; then
  pass "Drift has in-progress → implemented promotion"
else
  fail "Drift missing in-progress → implemented promotion"
fi

assert_file_contains "$DRIFT_CMD" "in-progress" \
  "Drift requires in-progress before implemented"

# ============================================================
# TEST 8: Doctor stale draft detection
# ============================================================

echo ""
echo -e "${BOLD}TEST 8: Doctor stale drafts${NC}"

# Doctor checks spec-artifacts (not just PRDs/specs)
if grep -qi "SPEC-\|spec.*artifact\|spec.*dir\|spec.*folder" "$DOCTOR_CMD" 2>/dev/null; then
  pass "Doctor checks spec-artifacts for stale drafts"
else
  fail "Doctor should check spec-artifacts for stale drafts"
fi

# Doctor parses multiple comment header formats (at least 2 of: %%, #, --, <!-- )
FORMAT_COUNT=0
grep -q '%%' "$DOCTOR_CMD" 2>/dev/null && ((FORMAT_COUNT++)) || true
grep -q '# edikt:artifact\|# edikt' "$DOCTOR_CMD" 2>/dev/null && ((FORMAT_COUNT++)) || true
grep -q '\-\- edikt:artifact\|\-\- edikt' "$DOCTOR_CMD" 2>/dev/null && ((FORMAT_COUNT++)) || true
grep -q '<!-- edikt:artifact\|<!-- edikt' "$DOCTOR_CMD" 2>/dev/null && ((FORMAT_COUNT++)) || true
if [ "$FORMAT_COUNT" -ge 2 ]; then
  pass "Doctor parses multiple comment header formats ($FORMAT_COUNT found)"
else
  fail "Doctor should parse at least 2 comment header formats (found $FORMAT_COUNT)"
fi

assert_file_contains "$DOCTOR_CMD" "7 day" \
  "Doctor uses 7-day stale threshold"

test_summary
