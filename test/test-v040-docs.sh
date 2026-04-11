#!/bin/bash
# Test: v0.4.0 documentation — pre-existing fixes + v0.4.0 feature docs + release prep
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

echo ""

# ============================================================
# TEST 1: Stale count fixes
# ============================================================

echo -e "${BOLD}TEST 1: Stale count fixes${NC}"

STALE=$(grep -rl "18 specialist" "$PROJECT_ROOT/website/" "$PROJECT_ROOT/docs/project-context.md" "$PROJECT_ROOT/README.md" 2>/dev/null | wc -l | tr -d ' ')
if [ "$STALE" -eq 0 ]; then
    pass "No stale '18 specialist agents' references"
else
    fail "Found $STALE files still referencing '18 specialist agents'"
fi

assert_file_not_contains "$PROJECT_ROOT/README.md" "25+ commands" "README command count updated"
assert_file_not_contains "$PROJECT_ROOT/README.md" "9 lifecycle hooks" "README hook count updated"

# ============================================================
# TEST 2: Website command index
# ============================================================

echo ""
echo -e "${BOLD}TEST 2: Website command index${NC}"

assert_file_contains "$PROJECT_ROOT/website/commands/index.md" "gov:score" "Index has gov:score"
assert_file_contains "$PROJECT_ROOT/website/commands/index.md" "config" "Index has config"

if grep -qi "team.*deprecated\|deprecated.*team" "$PROJECT_ROOT/website/commands/index.md" 2>/dev/null; then
    pass "Index marks team as deprecated"
else
    fail "Index should mark team as deprecated"
fi

# ============================================================
# TEST 3: Stale command names
# ============================================================

echo ""
echo -e "${BOLD}TEST 3: Stale command names${NC}"

if grep -q '`/edikt:adr`' "$PROJECT_ROOT/website/governance/features.md" 2>/dev/null; then
    fail "features.md still has old /edikt:adr (should be /edikt:adr:new)"
else
    pass "features.md uses namespaced /edikt:adr:new"
fi

# ============================================================
# TEST 4: Changelog and version
# ============================================================

echo ""
echo -e "${BOLD}TEST 4: Changelog and version${NC}"

assert_file_contains "$PROJECT_ROOT/CHANGELOG.md" "v0.4.0" "Changelog has v0.4.0 entry"
assert_file_contains "$PROJECT_ROOT/CHANGELOG.md" "Iteration Tracking" "Changelog covers plan harness"
assert_file_contains "$PROJECT_ROOT/CHANGELOG.md" "Headless" "Changelog covers headless evaluator"
assert_file_contains "$PROJECT_ROOT/CHANGELOG.md" "events.jsonl" "Changelog covers gate logging"
assert_file_contains "$PROJECT_ROOT/CHANGELOG.md" "Artifact Lifecycle" "Changelog covers lifecycle"

VER=$(cat "$PROJECT_ROOT/VERSION" | tr -d '[:space:]')
if [ "$VER" = "0.4.0" ]; then
    pass "VERSION is 0.4.0"
else
    fail "VERSION is $VER, expected 0.4.0"
fi

CONFIG_VER=$(grep 'edikt_version' "$PROJECT_ROOT/.edikt/config.yaml" | awk '{print $2}' | tr -d '"')
if [ "$CONFIG_VER" = "0.4.0" ]; then
    pass "Config edikt_version is 0.4.0"
else
    fail "Config edikt_version is $CONFIG_VER, expected 0.4.0"
fi

# ============================================================
# TEST 5: Website plan page
# ============================================================

echo ""
echo -e "${BOLD}TEST 5: Website plan page${NC}"

assert_file_contains "$PROJECT_ROOT/website/commands/sdlc/plan.md" "Attempt" "Plan page has Attempt column"
assert_file_contains "$PROJECT_ROOT/website/commands/sdlc/plan.md" "Context Needed" "Plan page has Context Needed"
assert_file_contains "$PROJECT_ROOT/website/commands/sdlc/plan.md" "criteria.yaml" "Plan page has criteria sidecar"
assert_file_contains "$PROJECT_ROOT/website/commands/sdlc/plan.md" "stuck" "Plan page has stuck status"
assert_file_contains "$PROJECT_ROOT/website/commands/sdlc/plan.md" "evaluator" "Plan page has evaluator config"

# ============================================================
# TEST 6: Website gates page
# ============================================================

echo ""
echo -e "${BOLD}TEST 6: Website gates page${NC}"

assert_file_contains "$PROJECT_ROOT/website/governance/gates.md" "events.jsonl" "Gates page has events.jsonl"
assert_file_contains "$PROJECT_ROOT/website/governance/gates.md" "override" "Gates page has override docs"

if grep -qE "re-fire|Re-fire|session" "$PROJECT_ROOT/website/governance/gates.md" 2>/dev/null; then
    pass "Gates page has re-fire prevention"
else
    fail "Gates page missing re-fire prevention"
fi

# ============================================================
# TEST 7: Website chain page
# ============================================================

echo ""
echo -e "${BOLD}TEST 7: Website chain page${NC}"

assert_file_contains "$PROJECT_ROOT/website/governance/chain.md" "in-progress" "Chain page has in-progress state"
assert_file_contains "$PROJECT_ROOT/website/governance/chain.md" "implemented" "Chain page has implemented state"
assert_file_contains "$PROJECT_ROOT/website/governance/chain.md" "superseded" "Chain page has superseded state"

# ============================================================
# TEST 8: Website features page
# ============================================================

echo ""
echo -e "${BOLD}TEST 8: Website features page${NC}"

assert_file_contains "$PROJECT_ROOT/website/governance/features.md" "evaluator" "Features page has evaluator section"
assert_file_contains "$PROJECT_ROOT/website/governance/features.md" "preflight" "Features page has preflight toggle"
assert_file_contains "$PROJECT_ROOT/website/governance/features.md" "phase-end" "Features page has phase-end toggle"
assert_file_contains "$PROJECT_ROOT/website/governance/features.md" "headless" "Features page has headless mode"

# ============================================================
# TEST 9: Website doctor and drift pages
# ============================================================

echo ""
echo -e "${BOLD}TEST 9: Website doctor and drift pages${NC}"

if grep -qi "spec.*artifact\|SPEC-.*draft\|stale.*artifact" "$PROJECT_ROOT/website/commands/doctor.md" 2>/dev/null; then
    pass "Doctor page has spec-artifact stale draft docs"
else
    fail "Doctor page missing spec-artifact stale draft docs"
fi

assert_file_contains "$PROJECT_ROOT/website/commands/sdlc/drift.md" "filter" "Drift page has status filtering"

if grep -qE "auto-promote|Auto-promote" "$PROJECT_ROOT/website/commands/sdlc/drift.md" 2>/dev/null; then
    pass "Drift page has auto-promote docs"
else
    fail "Drift page missing auto-promote docs"
fi

# ============================================================
# TEST 10: AGENTS.md cleanup
# ============================================================

echo ""
echo -e "${BOLD}TEST 10: AGENTS.md cleanup${NC}"

if grep -q "AGENTS.md" "$PROJECT_ROOT/.gitignore" 2>/dev/null; then
    pass "AGENTS.md is gitignored"
else
    fail "AGENTS.md should be in .gitignore"
fi

test_summary
