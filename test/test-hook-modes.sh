#!/bin/bash
# Test: every templates/hooks/* file is committed as exactly mode 100755.
#
# Why: settings.json invokes hooks as `"command": "${EDIKT_HOOK_DIR}/<hook>.sh"`,
# which Claude Code execs directly. Without the exec bit (100644), every fire
# returns "Permission denied" — most failures are silent (PostToolUse, etc.),
# only Stop/UserPromptSubmit surface UI errors. Over-permissioned files
# (100777) are world-writable and would let a local user substitute hook
# content. Reject both directions of regression.
#
# (ref: PLAN-v0.5.0-stability Phase 21)

set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

HOOKS_DIR="$PROJECT_ROOT/templates/hooks"

if [ ! -d "$HOOKS_DIR" ]; then
    fail "templates/hooks/ directory missing at $HOOKS_DIR"
    test_summary
    exit "$FAIL_COUNT"
fi

# Use git ls-files -s to read the committed mode (100644 / 100755).
# This is the source of truth; the on-disk mode is checked separately by
# test/integration/test_install_modes.py after extraction.
cd "$PROJECT_ROOT" || {
    fail "cannot cd to $PROJECT_ROOT"
    test_summary
    exit "$FAIL_COUNT"
}

CHECKED=0
while IFS= read -r mode_path; do
    mode="${mode_path%% *}"
    path="${mode_path##* }"
    CHECKED=$((CHECKED + 1))
    if [ "$mode" = "100755" ]; then
        pass "Hook mode 100755: $path"
    else
        fail "Hook mode WRONG: $path is $mode (expected exactly 100755)" \
             "fix: git update-index --chmod=+x $path"
    fi
done < <(git ls-files -s templates/hooks/ | awk '{print $1, $4}')

if [ "$CHECKED" -eq 0 ]; then
    fail "No hooks found via 'git ls-files -s templates/hooks/' — check repo state"
fi

test_summary
