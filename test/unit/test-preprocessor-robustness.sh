#!/usr/bin/env bash
# Test: live-block preprocessor robustness across shells, cwds, and config states.
# Guards against three bug classes:
#   1. cwd assumption — preprocessor opens .edikt/config.yaml relative to $PWD
#   2. zsh nomatch — bare glob expansion leaks shell errors
#   3. broken fallback — `|| echo "docs"` binds to tr, not the pipeline
#
# Extract each command's `!` preprocessor block, execute it with varied
# SHELL and cwd, and assert clean output.

set -uo pipefail

PROJECT_ROOT="$(cd "${1:-.}" && pwd)"
source "$(dirname "$0")/../helpers.sh" 2>/dev/null || source "$(dirname "$0")/helpers.sh" 2>/dev/null || {
    # Minimal fallback helpers if helpers.sh not in expected location
    PASS_COUNT=0
    FAIL_COUNT=0
    GREEN="\033[0;32m"
    RED="\033[0;31m"
    BOLD="\033[1m"
    NC="\033[0m"
    pass() { echo -e "  ${GREEN}✓${NC} $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
    fail() { echo -e "  ${RED}✗${NC} $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
}

echo ""
echo -e "${BOLD}Preprocessor Robustness Tests${NC}"
echo ""

# Commands to exercise — (file, expected-output-regex)
COMMANDS=(
    "commands/adr/new.md|Next ADR number: ADR-"
    "commands/invariant/new.md|Next INV number: INV-"
    "commands/sdlc/prd.md|Next PRD number: PRD-"
    "commands/sdlc/spec.md|Next SPEC number: SPEC-"
)
# plan.md is special — outputs "Active plan:" only when a plan exists; skipped in non-state scenarios

# Extract the preprocessor block from a command file.
# Returns the string between `!\`` and the closing backtick on the same line.
extract_preprocessor() {
    local file="$1"
    # Grep the line starting with !` and strip the !` prefix and trailing `
    grep -m1 '^!`' "$PROJECT_ROOT/$file" | sed -e 's/^!`//' -e 's/`$//'
}

# Run a preprocessor under a given shell and cwd, capture stdout+stderr.
run_under() {
    local shell="$1"
    local cwd="$2"
    local block="$3"
    cd "$cwd" && SHELL="$shell" "$shell" -c "$block" 2>&1
}

# ============================================================
# TEST 1: Each preprocessor produces expected output under zsh from project root
# ============================================================

echo -e "${BOLD}TEST 1: Baseline (zsh + project root)${NC}"

for entry in "${COMMANDS[@]}"; do
    file="${entry%%|*}"
    expect="${entry#*|}"
    block=$(extract_preprocessor "$file")
    if [ -z "$block" ]; then
        fail "$file: no \`!\` preprocessor block found"
        continue
    fi
    output=$(run_under /bin/zsh "$PROJECT_ROOT" "$block")
    if echo "$output" | grep -q "$expect"; then
        pass "$file produces '$expect' under zsh"
    else
        fail "$file missing expected output '$expect'. Got: $output"
    fi
done

# ============================================================
# TEST 2: No zsh nomatch errors leak
# ============================================================

echo ""
echo -e "${BOLD}TEST 2: No zsh nomatch error leakage${NC}"

for entry in "${COMMANDS[@]}"; do
    file="${entry%%|*}"
    block=$(extract_preprocessor "$file")
    [ -z "$block" ] && continue
    output=$(run_under /bin/zsh "$PROJECT_ROOT" "$block")
    if echo "$output" | grep -qE '\(eval\):|no matches found'; then
        fail "$file leaks zsh shell error: $(echo "$output" | grep -E '\(eval\):|no matches found' | head -1)"
    else
        pass "$file has no zsh shell errors in output"
    fi
done

# ============================================================
# TEST 3: Cwd-agnostic (works from /tmp — graceful no-op)
# ============================================================

echo ""
echo -e "${BOLD}TEST 3: Graceful no-op when cwd outside project${NC}"

# Create an empty /tmp dir with no config
tmpdir=$(mktemp -d)
trap "rm -rf $tmpdir" EXIT

for entry in "${COMMANDS[@]}"; do
    file="${entry%%|*}"
    block=$(extract_preprocessor "$file")
    [ -z "$block" ] && continue
    output=$(run_under /bin/zsh "$tmpdir" "$block")
    # Should produce either empty output (plan.md) or the "(none yet)" fallback
    if echo "$output" | grep -qE '\(eval\):|no matches found'; then
        fail "$file from /tmp leaks shell error: $(echo "$output" | grep -E '\(eval\):|no matches found' | head -1)"
    elif echo "$output" | grep -qE '(none yet)|^$'; then
        pass "$file from /tmp produces graceful no-op"
    elif [ -z "$output" ]; then
        pass "$file from /tmp produces empty output (plan.md expected)"
    else
        # Some output that doesn't indicate an error is acceptable (e.g. found a parent config somewhere)
        pass "$file from /tmp produces clean output (no shell errors)"
    fi
done

# ============================================================
# TEST 4: Works from project subdirectory (cwd-agnostic)
# ============================================================

echo ""
echo -e "${BOLD}TEST 4: Cwd-agnostic (project subdirectory)${NC}"

subdir="$PROJECT_ROOT/commands"
for entry in "${COMMANDS[@]}"; do
    file="${entry%%|*}"
    expect="${entry#*|}"
    block=$(extract_preprocessor "$file")
    [ -z "$block" ] && continue
    output=$(run_under /bin/zsh "$subdir" "$block")
    if echo "$output" | grep -q "$expect"; then
        pass "$file from project subdirectory produces '$expect'"
    else
        fail "$file from project subdirectory did not produce '$expect'. Got: $output"
    fi
done

# ============================================================
# TEST 5: Works under bash too (not just zsh)
# ============================================================

echo ""
echo -e "${BOLD}TEST 5: Bash compatibility${NC}"

for entry in "${COMMANDS[@]}"; do
    file="${entry%%|*}"
    expect="${entry#*|}"
    block=$(extract_preprocessor "$file")
    [ -z "$block" ] && continue
    output=$(run_under /bin/bash "$PROJECT_ROOT" "$block")
    if echo "$output" | grep -q "$expect"; then
        pass "$file produces '$expect' under bash"
    else
        fail "$file missing expected output under bash. Got: $output"
    fi
done

# ============================================================
# TEST 6: Config missing `base:` line uses "docs" default
# ============================================================

echo ""
echo -e "${BOLD}TEST 6: BASE fallback defaults to 'docs'${NC}"

# Create a test project with config lacking base:
testproj=$(mktemp -d)
mkdir -p "$testproj/.edikt" "$testproj/docs/architecture/decisions"
cat > "$testproj/.edikt/config.yaml" <<EOF
edikt_version: "0.5.0"
stack: []
paths: {}
EOF
touch "$testproj/docs/architecture/decisions/ADR-001-test.md"

block=$(extract_preprocessor "commands/adr/new.md")
output=$(run_under /bin/zsh "$testproj" "$block")
if echo "$output" | grep -q "Next ADR number: ADR-002"; then
    pass "adr/new.md with config lacking base: defaults to 'docs'"
else
    fail "adr/new.md did not use 'docs' default. Got: $output"
fi

rm -rf "$testproj"

# ============================================================
# TEST 7: The legacy `|| echo "docs"` pattern is gone
# ============================================================

echo ""
echo -e "${BOLD}TEST 7: Legacy broken fallback pattern removed${NC}"

offenders=$(grep -l '|| echo "docs"' \
    "$PROJECT_ROOT/commands/adr/new.md" \
    "$PROJECT_ROOT/commands/invariant/new.md" \
    "$PROJECT_ROOT/commands/sdlc/prd.md" \
    "$PROJECT_ROOT/commands/sdlc/plan.md" \
    "$PROJECT_ROOT/commands/sdlc/spec.md" 2>/dev/null | wc -l | tr -d ' ')

if [ "$offenders" = "0" ]; then
    pass "No command uses the broken \`|| echo \"docs\"\` pipeline fallback"
else
    fail "$offenders commands still use the broken fallback pattern"
fi

# ============================================================
# TEST 8: bash -c wrapper is present (shell isolation)
# ============================================================

echo ""
echo -e "${BOLD}TEST 8: Shell-isolation wrapper (bash -c) present${NC}"

for entry in "${COMMANDS[@]}"; do
    file="${entry%%|*}"
    if grep -q '^!`bash -c' "$PROJECT_ROOT/$file"; then
        pass "$file wraps preprocessor in bash -c"
    else
        fail "$file missing bash -c wrapper — may leak zsh errors"
    fi
done

# plan.md check
if grep -q '^!`bash -c' "$PROJECT_ROOT/commands/sdlc/plan.md"; then
    pass "commands/sdlc/plan.md wraps preprocessor in bash -c"
else
    fail "commands/sdlc/plan.md missing bash -c wrapper"
fi

# ============================================================
# Summary
# ============================================================

echo ""
echo -e "${BOLD}Summary${NC}"
if [ "${FAIL_COUNT:-0}" -eq 0 ]; then
    echo -e "${GREEN}All preprocessor robustness tests passed (${PASS_COUNT} assertions).${NC}"
    exit 0
else
    echo -e "${RED}${FAIL_COUNT} failure(s), ${PASS_COUNT} pass(es).${NC}"
    exit 1
fi
