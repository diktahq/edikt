#!/bin/bash
# Test: v0.4.0 SPEC-002 evaluator — config, headless prompt, plan integration, experiment runner
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

PLAN_CMD="$PROJECT_ROOT/commands/sdlc/plan.md"
CONFIG_CMD="$PROJECT_ROOT/commands/config.md"
EVALUATOR_HEADLESS="$PROJECT_ROOT/templates/agents/evaluator-headless.md"
EVALUATOR_SUBAGENT="$PROJECT_ROOT/templates/agents/evaluator.md"
RUNNER="$PROJECT_ROOT/test/experiments/directive-effect/run.sh"
EVAL_PROMPT="$PROJECT_ROOT/test/experiments/lib/evaluator-system-prompt.md"
WEBSITE_EVAL="$PROJECT_ROOT/website/governance/evaluator.md"

echo ""

# ============================================================
# TEST 1: Headless evaluator prompt
# ============================================================

echo -e "${BOLD}TEST 1: Headless evaluator prompt${NC}"

assert_file_exists "$EVALUATOR_HEADLESS"

# No YAML frontmatter — first non-empty line must not be "---"
FIRST_LINE=$(head -1 "$EVALUATOR_HEADLESS" | tr -d '[:space:]')
if [ "$FIRST_LINE" = "---" ]; then
    fail "evaluator-headless.md has no YAML frontmatter (must not)"
else
    pass "evaluator-headless.md has no YAML frontmatter"
fi

assert_file_contains "$EVALUATOR_HEADLESS" "incomplete until proven" \
  "Headless has skeptical stance"

# ADR-018 replaced the v0.4.0 "PHASE EVALUATION" plaintext header with a
# JSON-verdict contract defined by evaluator-verdict.schema.json. The output
# format section is now the schema-pinned block — assert that plus the
# verdict enum, not the old plaintext marker.
assert_file_contains "$EVALUATOR_HEADLESS" "Output Format" \
  "Headless has output format"
assert_file_contains "$EVALUATOR_HEADLESS" "evaluator-verdict.schema.json" \
  "Headless output format references ADR-018 verdict schema"

assert_file_contains "$EVALUATOR_HEADLESS" "NEVER modify code" \
  "Headless has read-only constraint"

assert_file_contains "$EVALUATOR_HEADLESS" "Pre-flight Mode" \
  "Headless has pre-flight section"

assert_file_contains "$EVALUATOR_HEADLESS" "TESTABLE" \
  "Headless has TESTABLE classification"

assert_file_contains "$EVALUATOR_HEADLESS" "VAGUE" \
  "Headless has VAGUE classification"

assert_file_contains "$EVALUATOR_HEADLESS" "SUBJECTIVE" \
  "Headless has SUBJECTIVE classification"

assert_file_contains "$EVALUATOR_HEADLESS" "BLOCKED" \
  "Headless has BLOCKED classification"

# Zero-context note — match common phrasings
if grep -qi "zero.*context\|no.*shared.*context\|no.*conversation.*history" \
    "$EVALUATOR_HEADLESS" 2>/dev/null; then
    pass "Headless has zero-context note"
else
    fail "Headless missing zero-context note"
fi

# ============================================================
# TEST 2: Subagent evaluator updated
# ============================================================

echo ""
echo -e "${BOLD}TEST 2: Subagent evaluator${NC}"

assert_file_contains "$EVALUATOR_SUBAGENT" "evaluator-headless.md" \
  "Subagent points to headless version"

# ============================================================
# TEST 3: Config command has evaluator keys
# ============================================================

echo ""
echo -e "${BOLD}TEST 3: Config evaluator keys${NC}"

assert_file_contains "$CONFIG_CMD" "evaluator.preflight" \
  "Config has evaluator.preflight"

assert_file_contains "$CONFIG_CMD" "evaluator.phase-end" \
  "Config has evaluator.phase-end"

assert_file_contains "$CONFIG_CMD" "evaluator.mode" \
  "Config has evaluator.mode"

assert_file_contains "$CONFIG_CMD" "evaluator.max-attempts" \
  "Config has evaluator.max-attempts"

assert_file_contains "$CONFIG_CMD" "evaluator.model" \
  "Config has evaluator.model"

# ============================================================
# TEST 4: Plan command integration
# ============================================================

echo ""
echo -e "${BOLD}TEST 4: Plan command integration${NC}"

assert_file_contains "$PLAN_CMD" "evaluator.preflight" \
  "Plan reads evaluator.preflight"

assert_file_contains "$PLAN_CMD" "evaluator.phase-end" \
  "Plan reads evaluator.phase-end"

assert_file_contains "$PLAN_CMD" "evaluator.mode" \
  "Plan reads evaluator.mode"

# Use grep -F to avoid "--bare" being interpreted as a grep option
if grep -qF -- "--bare" "$PLAN_CMD" 2>/dev/null; then
    pass "Plan has headless --bare flag"
else
    fail "Plan has headless --bare flag"
fi

assert_file_contains "$PLAN_CMD" "disallowedTools" \
  "Plan has --disallowedTools in headless invocation"

assert_file_contains "$PLAN_CMD" "Evaluator template missing" \
  "Plan has file existence check error"

# Toggle: preflight false
if grep -qi "preflight.*false\|skipped.*preflight\|preflight.*skip" \
    "$PLAN_CMD" 2>/dev/null; then
    pass "Plan handles evaluator.preflight: false"
else
    fail "Plan missing evaluator.preflight toggle behavior"
fi

# Toggle: phase-end false
if grep -qi "phase-end.*false\|skipped.*phase-end\|phase-end.*skip" \
    "$PLAN_CMD" 2>/dev/null; then
    pass "Plan handles evaluator.phase-end: false"
else
    fail "Plan missing evaluator.phase-end toggle behavior"
fi

# ============================================================
# TEST 5: Experiment runner (conditional)
# ============================================================

echo ""
echo -e "${BOLD}TEST 5: Experiment runner${NC}"

if [ ! -f "$RUNNER" ]; then
    echo "  SKIP  Experiment runner not found (gitignored) — skipping runner tests"
else
    assert_file_contains "$RUNNER" "llm-eval" \
      "Runner has --llm-eval flag"

    assert_file_contains "$RUNNER" "evaluator-criteria.yaml" \
      "Runner detects criteria file"

    assert_file_contains "$RUNNER" "WEAK PASS" \
      "Runner has WEAK PASS verdict"

    assert_file_contains "$RUNNER" "claude -p" \
      "Runner uses claude -p for evaluator"

    assert_file_contains "$RUNNER" "verdict_source" \
      "Runner tracks verdict source"

    assert_file_contains "$RUNNER" "fallback" \
      "Runner has grep fallback"

    assert_file_contains "$RUNNER" "eval.txt" \
      "Runner writes eval verdict file"
fi

# ============================================================
# TEST 6: Experiment evaluator system prompt (conditional)
# ============================================================

echo ""
echo -e "${BOLD}TEST 6: Evaluator system prompt${NC}"

if [ ! -f "$EVAL_PROMPT" ]; then
    echo "  SKIP  Evaluator system prompt not found — skipping"
else
    assert_file_exists "$EVAL_PROMPT" \
      "Evaluator system prompt exists"

    assert_file_contains "$EVAL_PROMPT" "skeptical" \
      "System prompt has skeptical stance"

    assert_file_contains "$EVAL_PROMPT" "critical" \
      "System prompt has critical severity"

    assert_file_contains "$EVAL_PROMPT" "important" \
      "System prompt has important severity"

    assert_file_contains "$EVAL_PROMPT" "informational" \
      "System prompt has informational severity"

    assert_file_contains "$EVAL_PROMPT" "EXPERIMENT EVALUATION" \
      "System prompt has output format"

    assert_file_contains "$EVAL_PROMPT" "WEAK PASS" \
      "System prompt documents WEAK PASS"
fi

# ============================================================
# TEST 7: Website evaluator docs
# ============================================================

echo ""
echo -e "${BOLD}TEST 7: Website evaluator docs${NC}"

assert_file_exists "$WEBSITE_EVAL" \
  "Website evaluator page exists"

assert_file_contains "$WEBSITE_EVAL" "headless" \
  "Website documents headless mode"

assert_file_contains "$WEBSITE_EVAL" "subagent" \
  "Website documents subagent mode"

assert_file_contains "$WEBSITE_EVAL" "evaluator.preflight" \
  "Website has config reference"

assert_file_contains "$WEBSITE_EVAL" "claude -p" \
  "Website shows headless invocation"

# ============================================================
# TEST: ADR-010 — BLOCKED verdict, visible fallback, doctor probe, --eval-only
# ============================================================

echo ""
echo -e "${BOLD}TEST: ADR-010 evaluator contracts${NC}"

DOCTOR_CMD="$PROJECT_ROOT/commands/doctor.md"

# BLOCKED verdict in both evaluator templates
assert_file_contains "$EVALUATOR_SUBAGENT" "BLOCKED" \
  "Subagent evaluator declares BLOCKED verdict (ADR-010)"
assert_file_contains "$EVALUATOR_HEADLESS" "BLOCKED" \
  "Headless evaluator declares BLOCKED verdict (ADR-010)"

# Subagent-only: Capability Self-Check
assert_file_contains "$EVALUATOR_SUBAGENT" "Capability Self-Check" \
  "Subagent evaluator has Capability Self-Check section (ADR-010)"

# Both templates have the never-PASS-when-exec-unavailable rule
assert_file_contains "$EVALUATOR_SUBAGENT" "never PASS" \
  "Subagent evaluator enforces never-PASS-without-execution rule"
assert_file_contains "$EVALUATOR_HEADLESS" "never PASS" \
  "Headless evaluator enforces never-PASS-without-execution rule"

# Subagent top comment warns Bash may be denied
assert_file_contains "$EVALUATOR_SUBAGENT" "Bash execution may be denied" \
  "Subagent evaluator warns about parent sandbox Bash denial"

# plan.md: visible fallback banners
assert_file_contains "$PLAN_CMD" "⚠ EVALUATOR FALLBACK" \
  "plan.md emits visible EVALUATOR FALLBACK banner (ADR-010)"
assert_file_contains "$PLAN_CMD" "✗ EVALUATION FAILED" \
  "plan.md emits EVALUATION FAILED banner on double-failure (ADR-010)"

# plan.md: --eval-only flag
if grep -q -- '--eval-only' "$PLAN_CMD"; then
  pass "plan.md documents --eval-only flag (ADR-010)"
else
  fail "plan.md missing --eval-only flag documentation"
fi

# doctor.md: evaluator probe
assert_file_contains "$DOCTOR_CMD" "Evaluator" \
  "doctor.md has Evaluator probe section (ADR-010)"
assert_file_contains "$DOCTOR_CMD" "command -v claude" \
  "doctor.md probes claude CLI on PATH"
assert_file_contains "$DOCTOR_CMD" "evaluator-headless.md" \
  "doctor.md probes headless template presence"

test_summary
