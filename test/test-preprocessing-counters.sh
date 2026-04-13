#!/bin/bash
# Test: preprocessing counter regression — verify prefix-based counting
# and config-aware path resolution in all commands with live blocks.
#
# Guards against:
# - README.md or other non-artifact files inflating counters
# - Hardcoded subpaths instead of reading paths.* from config
# - Baked-in static output (missing preprocessing block)
set -uo pipefail

PROJECT_ROOT="$(cd "${1:-.}" && pwd)"
source "$(dirname "$0")/helpers.sh"

echo ""

# ============================================================
# TEST 1: All preprocessing blocks use prefix patterns
# ============================================================

echo -e "${BOLD}TEST 1: Prefix-based counting (no *.md wildcards)${NC}"

# PRD: must glob PRD-*.md, not *.md
if grep -q 'PRD-\*\.md' "$PROJECT_ROOT/commands/sdlc/prd.md" 2>/dev/null; then
    pass "prd.md uses PRD-*.md prefix pattern"
else
    fail "prd.md should use PRD-*.md prefix pattern, not *.md"
fi

# ADR: must glob ADR-*.md
if grep -q 'ADR-\*\.md' "$PROJECT_ROOT/commands/adr/new.md" 2>/dev/null; then
    pass "adr/new.md uses ADR-*.md prefix pattern"
else
    fail "adr/new.md should use ADR-*.md prefix pattern, not *.md"
fi

# INV: must glob INV-*.md
if grep -q 'INV-\*\.md' "$PROJECT_ROOT/commands/invariant/new.md" 2>/dev/null; then
    pass "invariant/new.md uses INV-*.md prefix pattern"
else
    fail "invariant/new.md should use INV-*.md prefix pattern, not *.md"
fi

# SPEC: must glob SPEC-*/spec.md
if grep -q 'SPEC-\*/spec\.md' "$PROJECT_ROOT/commands/sdlc/spec.md" 2>/dev/null; then
    pass "spec.md uses SPEC-*/spec.md prefix pattern"
else
    fail "spec.md should use SPEC-*/spec.md prefix pattern"
fi

# ============================================================
# TEST 2: All preprocessing blocks read paths from config
# ============================================================

echo ""
echo -e "${BOLD}TEST 2: Config-aware path resolution${NC}"

# PRD: reads paths.prds from config
assert_file_contains "$PROJECT_ROOT/commands/sdlc/prd.md" "prds:" "prd.md reads paths.prds from config"

# ADR: reads paths.decisions from config
assert_file_contains "$PROJECT_ROOT/commands/adr/new.md" "decisions:" "adr/new.md reads paths.decisions from config"

# INV: reads paths.invariants from config
assert_file_contains "$PROJECT_ROOT/commands/invariant/new.md" "invariants:" "invariant/new.md reads paths.invariants from config"

# SPEC: reads paths.specs from config
assert_file_contains "$PROJECT_ROOT/commands/sdlc/spec.md" "specs:" "spec.md reads paths.specs from config"

# Plan: reads paths.plans from config
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "plans:" "plan.md reads paths.plans from config"

# ============================================================
# TEST 3: All preprocessing blocks are live (not baked-in)
# ============================================================

echo ""
echo -e "${BOLD}TEST 3: Preprocessing blocks are live (not static)${NC}"

# Each command with a counter must have a !` preprocessing block
for cmd in "commands/sdlc/prd.md" "commands/sdlc/spec.md" "commands/adr/new.md" "commands/invariant/new.md" "commands/sdlc/plan.md"; do
    if grep -q '^!\`' "$PROJECT_ROOT/$cmd" 2>/dev/null; then
        pass "$cmd has live preprocessing block"
    else
        fail "$cmd missing live preprocessing block (baked-in static output?)"
    fi
done

# No baked-in (eval) errors in any command
for cmd in "commands/sdlc/prd.md" "commands/sdlc/spec.md" "commands/adr/new.md" "commands/invariant/new.md"; do
    if grep -q '(eval):' "$PROJECT_ROOT/$cmd" 2>/dev/null; then
        fail "$cmd has baked-in (eval) error output — preprocessing was lost"
    else
        pass "$cmd has no baked-in (eval) errors"
    fi
done

# ============================================================
# TEST 4: Functional test — prefix counting excludes README
# ============================================================

echo ""
echo -e "${BOLD}TEST 4: Prefix counting excludes non-artifact files${NC}"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

# Set up a mock project
mkdir -p "$TMPDIR/docs/product/prds"
mkdir -p "$TMPDIR/docs/architecture/decisions"
mkdir -p "$TMPDIR/docs/architecture/invariants"
mkdir -p "$TMPDIR/docs/product/specs/SPEC-001-foo"
mkdir -p "$TMPDIR/docs/product/specs/SPEC-002-bar"
mkdir -p "$TMPDIR/.edikt"

cat > "$TMPDIR/.edikt/config.yaml" << 'YAML'
edikt_version: "0.4.0"
base: docs
paths:
  prds: docs/product/prds
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  specs: docs/product/specs
YAML

# Create artifacts + noise files
touch "$TMPDIR/docs/product/prds/PRD-001-foo.md"
touch "$TMPDIR/docs/product/prds/PRD-002-bar.md"
touch "$TMPDIR/docs/product/prds/PRD-003-baz.md"
touch "$TMPDIR/docs/product/prds/README.md"
touch "$TMPDIR/docs/product/prds/notes.md"
touch "$TMPDIR/docs/product/prds/template.md"

touch "$TMPDIR/docs/architecture/decisions/ADR-001-foo.md"
touch "$TMPDIR/docs/architecture/decisions/ADR-002-bar.md"
touch "$TMPDIR/docs/architecture/decisions/README.md"

touch "$TMPDIR/docs/architecture/invariants/INV-001-foo.md"
touch "$TMPDIR/docs/architecture/invariants/README.md"
touch "$TMPDIR/docs/architecture/invariants/notes.md"

touch "$TMPDIR/docs/product/specs/SPEC-001-foo/spec.md"
touch "$TMPDIR/docs/product/specs/SPEC-002-bar/spec.md"

# Test PRD counting
cd "$TMPDIR"
PRD_DIR="docs/product/prds"
PRD_COUNT=$(ls "${PRD_DIR}/"PRD-*.md 2>/dev/null | wc -l | tr -d ' ')
if [ "$PRD_COUNT" -eq 3 ]; then
    pass "PRD count is 3 (excludes README, notes, template)"
else
    fail "PRD count is $PRD_COUNT, expected 3"
fi

# Test ADR counting
ADR_DIR="docs/architecture/decisions"
ADR_COUNT=$(ls "${ADR_DIR}/"ADR-*.md 2>/dev/null | wc -l | tr -d ' ')
if [ "$ADR_COUNT" -eq 2 ]; then
    pass "ADR count is 2 (excludes README)"
else
    fail "ADR count is $ADR_COUNT, expected 2"
fi

# Test INV counting
INV_DIR="docs/architecture/invariants"
INV_COUNT=$(ls "${INV_DIR}/"INV-*.md 2>/dev/null | wc -l | tr -d ' ')
if [ "$INV_COUNT" -eq 1 ]; then
    pass "INV count is 1 (excludes README, notes)"
else
    fail "INV count is $INV_COUNT, expected 1"
fi

# Test SPEC counting
SPEC_DIR="docs/product/specs"
SPEC_COUNT=$(ls -d "${SPEC_DIR}/"SPEC-*/spec.md 2>/dev/null | wc -l | tr -d ' ')
if [ "$SPEC_COUNT" -eq 2 ]; then
    pass "SPEC count is 2"
else
    fail "SPEC count is $SPEC_COUNT, expected 2"
fi

# Verify next numbers
PRD_NEXT=$(printf "%03d" $((PRD_COUNT + 1)))
ADR_NEXT=$(printf "%03d" $((ADR_COUNT + 1)))
INV_NEXT=$(printf "%03d" $((INV_COUNT + 1)))
SPEC_NEXT=$(printf "%03d" $((SPEC_COUNT + 1)))

if [ "$PRD_NEXT" = "004" ]; then pass "Next PRD is PRD-004"; else fail "Next PRD is PRD-$PRD_NEXT, expected PRD-004"; fi
if [ "$ADR_NEXT" = "003" ]; then pass "Next ADR is ADR-003"; else fail "Next ADR is ADR-$ADR_NEXT, expected ADR-003"; fi
if [ "$INV_NEXT" = "002" ]; then pass "Next INV is INV-002"; else fail "Next INV is INV-$INV_NEXT, expected INV-002"; fi
if [ "$SPEC_NEXT" = "003" ]; then pass "Next SPEC is SPEC-003"; else fail "Next SPEC is SPEC-$SPEC_NEXT, expected SPEC-003"; fi

cd "$PROJECT_ROOT"

# ============================================================
# Preprocessing format regression — no blank line before !` block,
# argument-hint present on all commands with preprocessing
# ============================================================

echo ""
echo -e "${BOLD}Preprocessing format regression${NC}"

PREPROC_CMDS=$(grep -rl '^!`' "$PROJECT_ROOT/commands/" 2>/dev/null)

for cmd in $PREPROC_CMDS; do
    name=$(basename "$cmd")

    # No blank line between frontmatter closing --- and !` preprocessing
    # The !` must be on the line immediately after the closing ---
    line_after_frontmatter=$(awk '/^---$/{c++} c==2{getline; print; exit}' "$cmd")
    if echo "$line_after_frontmatter" | grep -q '^!`'; then
        pass "$name: no blank line before preprocessing"
    else
        fail "$name: blank line between frontmatter and preprocessing (causes shell corruption)"
    fi

    # argument-hint must be present in frontmatter
    if grep -q "argument-hint" "$cmd"; then
        pass "$name: has argument-hint in frontmatter"
    else
        fail "$name: missing argument-hint in frontmatter"
    fi

    # awk '{print $2}' must be inside single quotes (not corrupted)
    if grep '^!`' "$cmd" | grep -q "awk '{print \$2}'"; then
        pass "$name: awk pattern intact"
    else
        # Some commands may not use awk — only fail if they have awk at all
        if grep '^!`' "$cmd" | grep -q "awk"; then
            fail "$name: awk pattern corrupted in preprocessing"
        else
            pass "$name: no awk in preprocessing (ok)"
        fi
    fi
done

test_summary
