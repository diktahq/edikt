#!/usr/bin/env bash
# Integration test for Phase 12 — /edikt:sdlc:plan invokes `edikt verify`
# before flipping a phase row to `done`.
#
# This is a static-contract test: it asserts commands/sdlc/plan.md
# documents the verify gate in the PASS path, names the expected
# command form, documents the override marker for failed verifies, and
# documents each of the documented exit codes (0, 1, 2, 3). The full
# end-to-end behavior — actually running the command and observing a
# refused flip — lives in test/integration/test_e2e_*.py and is gated on
# Claude Code session auth (Phase 9).

set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$PROJECT_ROOT"

GREEN='\033[0;32m'
RED='\033[0;31m'
DIM='\033[0;90m'
RESET='\033[0m'

pass_count=0
fail_count=0

assert() {
    local label="$1"
    local cmd="$2"
    if eval "$cmd" >/dev/null 2>&1; then
        echo -e "  ${GREEN}+${RESET} $label"
        pass_count=$((pass_count + 1))
    else
        echo -e "  ${RED}x${RESET} $label"
        echo -e "    ${DIM}cmd: $cmd${RESET}"
        fail_count=$((fail_count + 1))
    fi
}

PLAN_MD="commands/sdlc/plan.md"
echo "Phase 12 — /edikt:sdlc:plan gate on edikt verify"

assert "$PLAN_MD references the verify subcommand" \
    "grep -q 'edikt verify' '$PLAN_MD'"
assert "$PLAN_MD documents per-phase invocation pattern" \
    "grep -E 'edikt verify .*--phase' '$PLAN_MD'"
assert "$PLAN_MD documents the override marker" \
    "grep -q 'overrides:' '$PLAN_MD'"
assert "$PLAN_MD references all four exit codes (0/1/2/3)" \
    "grep -q 'Exit 0' '$PLAN_MD' && grep -q 'Exit 1' '$PLAN_MD' && grep -q 'Exit 2' '$PLAN_MD' && grep -q 'Exit 3' '$PLAN_MD'"
assert "$PLAN_MD ties verify gate to Phase 12 of PLAN-sidecar-architecture" \
    "grep -q 'Phase 12' '$PLAN_MD'"

echo
echo -e "${DIM}$pass_count passed, $fail_count failed.${RESET}"
echo -e "${DIM}E2E flip-refusal lives in test/integration/test_e2e_plan_with_sidecar.py — gated on session auth.${RESET}"
exit "$fail_count"
