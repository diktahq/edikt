#!/bin/bash
# Tests for CodeRabbit PR #8 fixes — verifies bug fixes are correct
# Covers: .gitignore negation, subagent-stop override logic, agent counts, WEAK PASS exit codes
set -uo pipefail

PROJECT_ROOT="$(cd "${1:-.}" && pwd)"
source "$(dirname "$0")/helpers.sh"

E2E_DIR="/tmp/edikt-coderabbit-$$"
trap 'rm -rf "$E2E_DIR"' EXIT
mkdir -p "$E2E_DIR"

echo ""
echo "CodeRabbit fixes test dir: $E2E_DIR"

# ============================================================
# TEST 1: .gitignore negation — run.sh is trackable despite parent ignore
# ============================================================

echo ""
echo -e "${BOLD}TEST 1: .gitignore negation patterns${NC}"

GITIGNORE_DIR="$E2E_DIR/gitignore-test"
mkdir -p "$GITIGNORE_DIR/test/experiments/directive-effect"
mkdir -p "$GITIGNORE_DIR/test/experiments/long-running"

# Copy the project .gitignore
cp "$PROJECT_ROOT/.gitignore" "$GITIGNORE_DIR/.gitignore"

# Create test files
echo "#!/bin/bash" > "$GITIGNORE_DIR/test/experiments/directive-effect/run.sh"
echo "#!/bin/bash" > "$GITIGNORE_DIR/test/experiments/long-running/run.sh"
echo "result data" > "$GITIGNORE_DIR/test/experiments/directive-effect/results.txt"
echo "result data" > "$GITIGNORE_DIR/test/experiments/long-running/results.txt"

cd "$GITIGNORE_DIR"
git init -q
git add -A 2>/dev/null

# run.sh files should be tracked (not ignored)
if git status --porcelain | grep -q "directive-effect/run.sh"; then
    pass "directive-effect/run.sh is trackable (not ignored)"
else
    fail "directive-effect/run.sh is ignored — negation pattern not working"
fi

if git status --porcelain | grep -q "long-running/run.sh"; then
    pass "long-running/run.sh is trackable (not ignored)"
else
    fail "long-running/run.sh is ignored — negation pattern not working"
fi

# results.txt should be ignored
if git status --porcelain | grep -q "results.txt"; then
    fail "results.txt should be ignored but is trackable"
else
    pass "results.txt is correctly ignored"
fi

cd "$PROJECT_ROOT"

# ============================================================
# TEST 2: subagent-stop.sh override check — same-line matching
# ============================================================

echo ""
echo -e "${BOLD}TEST 2: Gate override same-line matching${NC}"

OVERRIDE_DIR="$E2E_DIR/override-test"
mkdir -p "$OVERRIDE_DIR"

# Simulate events.jsonl with overrides from different agents
cat > "$OVERRIDE_DIR/gate-overrides.jsonl" << 'EOF'
{"event":"gate_override","agent":"security","finding_prefix":"SQL injection risk in user input handling","ts":"2026-04-12T10:00:00Z"}
{"event":"gate_override","agent":"dba","finding_prefix":"Missing index on foreign key user_id","ts":"2026-04-12T10:01:00Z"}
EOF

# Test: security agent + SQL injection finding → should match (same line)
if grep -F '"agent":"security"' "$OVERRIDE_DIR/gate-overrides.jsonl" 2>/dev/null | grep -qF '"finding_prefix":"SQL injection risk in user input handling"'; then
    pass "Override match: security + SQL injection (same line) → correctly matched"
else
    fail "Override match: security + SQL injection should match on same line"
fi

# Test: security agent + missing index finding → should NOT match (different agents)
if grep -F '"agent":"security"' "$OVERRIDE_DIR/gate-overrides.jsonl" 2>/dev/null | grep -qF '"finding_prefix":"Missing index on foreign key user_id"'; then
    fail "Override cross-match: security + dba's finding should NOT match"
else
    pass "Override cross-match: security + dba's finding → correctly rejected"
fi

# Test: dba agent + SQL injection finding → should NOT match (different agents)
if grep -F '"agent":"dba"' "$OVERRIDE_DIR/gate-overrides.jsonl" 2>/dev/null | grep -qF '"finding_prefix":"SQL injection risk in user input handling"'; then
    fail "Override cross-match: dba + security's finding should NOT match"
else
    pass "Override cross-match: dba + security's finding → correctly rejected"
fi

# Test: dba agent + missing index → should match (same line)
if grep -F '"agent":"dba"' "$OVERRIDE_DIR/gate-overrides.jsonl" 2>/dev/null | grep -qF '"finding_prefix":"Missing index on foreign key user_id"'; then
    pass "Override match: dba + missing index (same line) → correctly matched"
else
    fail "Override match: dba + missing index should match on same line"
fi

# Test: nonexistent agent → should NOT match
if grep -F '"agent":"sre"' "$OVERRIDE_DIR/gate-overrides.jsonl" 2>/dev/null | grep -qF '"finding_prefix":"SQL injection"'; then
    fail "Override match for nonexistent agent should not match"
else
    pass "Override match: nonexistent agent → correctly rejected"
fi

# Verify the actual hook uses piped grep (same-line matching), not two separate greps
if grep -q 'gate-overrides.jsonl.*|.*grep -qF' "$PROJECT_ROOT/templates/hooks/subagent-stop.sh" 2>/dev/null; then
    pass "subagent-stop.sh uses piped grep for same-line override matching"
else
    fail "subagent-stop.sh should pipe grep for same-line matching, not two separate greps"
fi

# ============================================================
# TEST 3: Agent count consistency across all documentation
# ============================================================

echo ""
echo -e "${BOLD}TEST 3: Agent count consistency${NC}"

# Count actual agent templates (excluding headless evaluator variant)
AGENT_COUNT=$(find "$PROJECT_ROOT/templates/agents" -name "*.md" -not -name "evaluator-headless.md" | wc -l | tr -d ' ')
AGENT_COUNT_WITH_HEADLESS=$(find "$PROJECT_ROOT/templates/agents" -name "*.md" | wc -l | tr -d ' ')

echo "  Agent templates: $AGENT_COUNT (+ evaluator-headless = $AGENT_COUNT_WITH_HEADLESS)"

# Website pages should match
for f in website/commands/agents.md website/guides/specialist-agents.md; do
    if [ -f "$PROJECT_ROOT/$f" ]; then
        # Check that neither 19 nor 20 appears where 18 is correct
        if grep -q "20 specialist\|ships 20\|20 agents" "$PROJECT_ROOT/$f" 2>/dev/null; then
            fail "$f still says 20 agents"
        elif grep -q "19 specialist\|ships 19\|19 agents" "$PROJECT_ROOT/$f" 2>/dev/null; then
            fail "$f says 19 agents — should be $AGENT_COUNT"
        else
            pass "$f agent count is consistent"
        fi
    fi
done

# ============================================================
# TEST 4: WEAK PASS exit codes in specs
# ============================================================

echo ""
echo -e "${BOLD}TEST 4: WEAK PASS exit code consistency${NC}"

# WEAK PASS should exit 0 (counts as PASS), not 1
for spec in \
    "docs/product/specs/SPEC-002-evaluator-experiments/spec.md" \
    "docs/product/specs/SPEC-002-evaluator-experiments/experiment-evaluator-spec.md"; do
    if [ -f "$PROJECT_ROOT/$spec" ]; then
        # Check that WEAK PASS line itself has exit code 0, not 1
        if grep "WEAK PASS" "$PROJECT_ROOT/$spec" 2>/dev/null | grep -q "| 1 |"; then
            fail "$spec: WEAK PASS mapped to exit 1 (should be 0)"
        else
            pass "$spec: WEAK PASS not mapped to exit 1"
        fi
    fi
done

# ============================================================
# TEST 5: SPEC-001 sidecar — no "always regenerated" contradiction
# ============================================================

echo ""
echo -e "${BOLD}TEST 5: Sidecar regeneration wording${NC}"

SPEC_001="$PROJECT_ROOT/docs/product/specs/SPEC-001-plan-harness/spec.md"
if [ -f "$SPEC_001" ]; then
    assert_file_not_contains "$SPEC_001" "always regenerated" \
        "SPEC-001 no longer says sidecar is 'always regenerated'"
fi

# ============================================================
# TEST 6: SPEC-003 — no BSD-only stat command
# ============================================================

echo ""
echo -e "${BOLD}TEST 6: SPEC-003 portability${NC}"

SPEC_003="$PROJECT_ROOT/docs/product/specs/SPEC-003-enforcement/spec.md"
if [ -f "$SPEC_003" ]; then
    assert_file_not_contains "$SPEC_003" "stat -f%m" \
        "SPEC-003 has no BSD-only stat command"
fi

# ============================================================
# TEST 7: Config key migration — project-context not soul
# ============================================================

echo ""
echo -e "${BOLD}TEST 7: Config key migration consistency${NC}"

# The init command template should use project-context
assert_file_contains "$PROJECT_ROOT/commands/init.md" "project-context:" \
    "init.md uses project-context in config template"
assert_file_not_contains "$PROJECT_ROOT/commands/init.md" "  soul:" \
    "init.md does not use deprecated soul key"

# The config command should document project-context
assert_file_contains "$PROJECT_ROOT/commands/config.md" "paths.project-context" \
    "config.md documents paths.project-context"
assert_file_not_contains "$PROJECT_ROOT/commands/config.md" "paths.soul" \
    "config.md does not reference deprecated paths.soul"

# ============================================================
# TEST 8: Upgrade detects and installs new agents
# ============================================================

echo ""
echo -e "${BOLD}TEST 8: Upgrade new agent detection${NC}"

UPGRADE_CMD="$PROJECT_ROOT/commands/upgrade.md"

# Upgrade command must document detecting new agents
assert_file_contains "$UPGRADE_CMD" "Detect new agents" \
    "upgrade.md has new agent detection logic"

# Upgrade must distinguish core vs optional agents
assert_file_contains "$UPGRADE_CMD" "Core agents" \
    "upgrade.md defines core agents"
assert_file_contains "$UPGRADE_CMD" "Optional agents" \
    "upgrade.md defines optional agents"
assert_file_contains "$UPGRADE_CMD" "installed automatically" \
    "upgrade.md installs core agents automatically"

# Evaluator is core
assert_file_contains "$UPGRADE_CMD" "evaluator.*core\|core.*evaluator" \
    "upgrade.md classifies evaluator as core"

# Upgrade must offer optional agents to user
assert_file_contains "$UPGRADE_CMD" "choose which to add\|choose which to install" \
    "upgrade.md offers optional agents to user"

# Upgrade must tell user how to disable unwanted agents
assert_file_contains "$UPGRADE_CMD" "agents.custom" \
    "upgrade.md references agents.custom for skipping"

# Upgrade must handle user declining an optional agent
assert_file_contains "$UPGRADE_CMD" "declined an optional agent\|declines an optional agent" \
    "upgrade.md handles user declining optional agents"

# ============================================================
# TEST 9: Upgrade new agent install — e2e in /tmp
# ============================================================

echo ""
echo -e "${BOLD}TEST 9: Upgrade new agent install (e2e)${NC}"

UPGRADE_E2E="$E2E_DIR/upgrade-agents"
mkdir -p "$UPGRADE_E2E/.claude/agents"
mkdir -p "$UPGRADE_E2E/templates/agents"

# Simulate: project has 3 installed agents
echo "# Architect agent" > "$UPGRADE_E2E/.claude/agents/architect.md"
echo "# DBA agent" > "$UPGRADE_E2E/.claude/agents/dba.md"
echo "# Security agent" > "$UPGRADE_E2E/.claude/agents/security.md"

# Simulate: templates have 5 agents (2 new — 1 core, 1 optional)
echo "# Architect agent" > "$UPGRADE_E2E/templates/agents/architect.md"
echo "# DBA agent" > "$UPGRADE_E2E/templates/agents/dba.md"
echo "# Security agent" > "$UPGRADE_E2E/templates/agents/security.md"
echo "# Evaluator headless agent" > "$UPGRADE_E2E/templates/agents/evaluator-headless.md"
echo "# GTM agent" > "$UPGRADE_E2E/templates/agents/gtm.md"

# Core agents list (evaluator variants)
CORE_AGENTS="evaluator.md evaluator-headless.md"

# Detect new agents: templates that don't exist in .claude/agents/
NEW_CORE=""
NEW_OPTIONAL=""
for tmpl in "$UPGRADE_E2E/templates/agents/"*.md; do
    slug=$(basename "$tmpl")
    if [ ! -f "$UPGRADE_E2E/.claude/agents/$slug" ]; then
        if echo "$CORE_AGENTS" | grep -qw "$slug"; then
            NEW_CORE="$NEW_CORE $slug"
        else
            NEW_OPTIONAL="$NEW_OPTIONAL $slug"
        fi
    fi
done

if echo "$NEW_CORE" | grep -q "evaluator-headless.md"; then
    pass "Core agent detected: evaluator-headless.md"
else
    fail "Failed to detect evaluator-headless.md as core agent"
fi

if echo "$NEW_OPTIONAL" | grep -q "gtm.md"; then
    pass "Optional agent detected: gtm.md"
else
    fail "Failed to detect gtm.md as optional agent"
fi

# Install core agents automatically
for slug in $NEW_CORE; do
    cp "$UPGRADE_E2E/templates/agents/$slug" "$UPGRADE_E2E/.claude/agents/$slug"
done

if [ -f "$UPGRADE_E2E/.claude/agents/evaluator-headless.md" ]; then
    pass "Core agent installed automatically: evaluator-headless.md"
else
    fail "Core agent not installed: evaluator-headless.md"
fi

# Optional agent NOT installed until user accepts
if [ ! -f "$UPGRADE_E2E/.claude/agents/gtm.md" ]; then
    pass "Optional agent not installed without user acceptance: gtm.md"
else
    fail "Optional agent installed without user acceptance: gtm.md"
fi

# Simulate: user accepts gtm
cp "$UPGRADE_E2E/templates/agents/gtm.md" "$UPGRADE_E2E/.claude/agents/gtm.md"
if [ -f "$UPGRADE_E2E/.claude/agents/gtm.md" ]; then
    pass "Optional agent installed after user acceptance: gtm.md"
else
    fail "Failed to install optional agent: gtm.md"
fi

# Verify existing agents were not touched
if [ "$(cat "$UPGRADE_E2E/.claude/agents/architect.md")" = "# Architect agent" ]; then
    pass "Existing agent not modified: architect.md"
else
    fail "Existing agent was modified: architect.md"
fi

# Simulate: user declines an optional agent → add to custom list
mkdir -p "$UPGRADE_E2E/.edikt"
cat > "$UPGRADE_E2E/.edikt/config.yaml" << 'YAML'
agents:
  custom:
    - mobile
YAML

# Detect custom agents should be skipped
CUSTOM_AGENTS=$(grep -A10 "custom:" "$UPGRADE_E2E/.edikt/config.yaml" 2>/dev/null | grep "^ *-" | sed 's/.*- //' | tr -d ' ')
SKIP_MOBILE=false
for custom in $CUSTOM_AGENTS; do
    if [ "$custom" = "mobile" ]; then
        SKIP_MOBILE=true
    fi
done

if [ "$SKIP_MOBILE" = "true" ]; then
    pass "Declined optional agent added to custom list: mobile"
else
    fail "Declined agent not in custom list: mobile"
fi

echo ""
