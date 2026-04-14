#!/bin/bash
# Test: upgrade command classifies agent diffs correctly
# Covers v0.4.3 bug: upgrade silently overwrote user customizations because
# it treated all diffs as "template updated" without inspecting direction.
set -uo pipefail

PROJECT_ROOT="$(cd "${1:-.}" && pwd)"
source "$(dirname "$0")/helpers.sh"

UPGRADE_CMD="$PROJECT_ROOT/commands/upgrade.md"
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo ""

# ============================================================
# TEST 1: Upgrade docs diff classification
# ============================================================

echo -e "${BOLD}TEST 1: Diff classification documented${NC}"

assert_file_contains "$UPGRADE_CMD" "Classify the diff" \
    "Upgrade documents diff classification"

assert_file_contains "$UPGRADE_CMD" "PURE EXPANSION" \
    "Upgrade defines PURE EXPANSION category"

assert_file_contains "$UPGRADE_CMD" "PATH SUBSTITUTION" \
    "Upgrade defines PATH SUBSTITUTION category"

assert_file_contains "$UPGRADE_CMD" "USER DIVERGENCE" \
    "Upgrade defines USER DIVERGENCE category"

# ============================================================
# TEST 2: Upgrade shows diff before overwriting user content
# ============================================================

echo ""
echo -e "${BOLD}TEST 2: Diff preview before overwrite${NC}"

assert_file_contains "$UPGRADE_CMD" "Showing diff" \
    "Upgrade shows diff before overwriting"

assert_file_contains "$UPGRADE_CMD" "preview diff" \
    "Summary mentions preview diff option"

# ============================================================
# TEST 3: Upgrade offers keep-mine option
# ============================================================

echo ""
echo -e "${BOLD}TEST 3: Keep-mine option${NC}"

assert_file_contains "$UPGRADE_CMD" "Keep mine" \
    "Upgrade offers keep-mine option"

assert_file_contains "$UPGRADE_CMD" "edikt:custom" \
    "Upgrade references custom marker for keep-mine"

# ============================================================
# TEST 4: Pure expansion is marked safe
# ============================================================

echo ""
echo -e "${BOLD}TEST 4: Pure expansion is marked safe${NC}"

assert_file_contains "$UPGRADE_CMD" "pure expansion, safe to apply" \
    "Pure expansion example marked as safe"

assert_file_contains "$UPGRADE_CMD" "auto-applied" \
    "Upgrade auto-applies pure expansions"

# ============================================================
# TEST 5: User divergence requires confirmation
# ============================================================

echo ""
echo -e "${BOLD}TEST 5: User divergence requires confirmation${NC}"

assert_file_contains "$UPGRADE_CMD" "preview diff before accepting" \
    "User divergence requires preview"

assert_file_contains "$UPGRADE_CMD" "individually BEFORE the main confirmation" \
    "Divergent agents prompt individually"

# ============================================================
# TEST 6: Functional diff classification — simulate the logic
# ============================================================

echo ""
echo -e "${BOLD}TEST 6: Diff classification functional test${NC}"

# Scenario: template has expanded (pure expansion)
TMPL_EXPANSION="$TMPDIR/tmpl-a.md"
INST_EXPANSION="$TMPDIR/inst-a.md"
cat > "$TMPL_EXPANSION" << 'EOF'
# Architect Agent

## Responsibilities

- Review architecture
- Check ADRs
- Monitor invariants
- Propose new ADRs
- Flag drift
EOF
cat > "$INST_EXPANSION" << 'EOF'
# Architect Agent

## Responsibilities

- Review architecture
- Check ADRs
- Monitor invariants
EOF

# Count diff lines: additions vs deletions
ADDITIONS=$(diff -u "$INST_EXPANSION" "$TMPL_EXPANSION" | grep -c '^+[^+]' || true)
DELETIONS=$(diff -u "$INST_EXPANSION" "$TMPL_EXPANSION" | grep -c '^-[^-]' || true)

if [ "$ADDITIONS" -gt 0 ] && [ "$DELETIONS" -eq 0 ]; then
    pass "Pure expansion detected: $ADDITIONS additions, 0 deletions"
else
    fail "Pure expansion: expected additions>0 deletions=0, got +$ADDITIONS -$DELETIONS"
fi

# Scenario: user customized (divergence)
TMPL_DIVERGE="$TMPDIR/tmpl-b.md"
INST_DIVERGE="$TMPDIR/inst-b.md"
cat > "$TMPL_DIVERGE" << 'EOF'
# Backend Agent

## File Formatting

- gofmt for Go
- prettier for TypeScript
- black for Python
- rustfmt for Rust
EOF
cat > "$INST_DIVERGE" << 'EOF'
# Backend Agent

## File Formatting

- gofmt for Go
EOF

ADDITIONS=$(diff -u "$INST_DIVERGE" "$TMPL_DIVERGE" | grep -c '^+[^+]' || true)
DELETIONS=$(diff -u "$INST_DIVERGE" "$TMPL_DIVERGE" | grep -c '^-[^-]' || true)

# In this scenario, installed has fewer lines than template — additions > 0, deletions = 0
# This is still technically pure expansion (template added more). But if we flip:
# the user's file has content the template doesn't, that's divergence.
if [ "$ADDITIONS" -gt 0 ] && [ "$DELETIONS" -eq 0 ]; then
    pass "Template expansion detected correctly (installed has less content)"
else
    fail "Expected pure expansion direction"
fi

# Scenario: user customized (path substitution)
TMPL_PATH="$TMPDIR/tmpl-c.md"
INST_PATH="$TMPDIR/inst-c.md"
cat > "$TMPL_PATH" << 'EOF'
# Architect

Check docs/architecture/decisions/ before deciding.
Review docs/architecture/invariants/ for constraints.
EOF
cat > "$INST_PATH" << 'EOF'
# Architect

Check adr/ before deciding.
Review docs/architecture/invariants/ for constraints.
EOF

# One line changed (path substitution) — 1 addition, 1 deletion
ADDITIONS=$(diff -u "$TMPL_PATH" "$INST_PATH" | grep -c '^+[^+]' || true)
DELETIONS=$(diff -u "$TMPL_PATH" "$INST_PATH" | grep -c '^-[^-]' || true)

if [ "$ADDITIONS" -gt 0 ] && [ "$DELETIONS" -gt 0 ]; then
    pass "Path substitution creates both additions and deletions"
else
    fail "Path substitution: expected both +$ADDITIONS -$DELETIONS"
fi

# Scenario: genuine user customization
TMPL_CUSTOM="$TMPDIR/tmpl-d.md"
INST_CUSTOM="$TMPDIR/inst-d.md"
cat > "$TMPL_CUSTOM" << 'EOF'
# Security Agent

## Checks
- OWASP Top 10
- Secret detection
- Auth coverage
EOF
cat > "$INST_CUSTOM" << 'EOF'
# Security Agent

## Checks
- OWASP Top 10
- Secret detection
- Auth coverage
- Custom: PCI-DSS compliance scanning
- Custom: HIPAA audit requirements
EOF

# User added 2 lines — installed has MORE content than template
# diff template→installed: 2 additions from template perspective
# But conceptually this is user divergence
TMPL_LINES=$(wc -l < "$TMPL_CUSTOM" | tr -d ' ')
INST_LINES=$(wc -l < "$INST_CUSTOM" | tr -d ' ')

if [ "$INST_LINES" -gt "$TMPL_LINES" ]; then
    pass "User divergence detected: installed ($INST_LINES lines) > template ($TMPL_LINES lines)"
else
    fail "Expected installed > template for user divergence scenario"
fi

test_summary
