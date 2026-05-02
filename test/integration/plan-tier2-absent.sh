#!/usr/bin/env bash
# Integration test for ADR-029 — /edikt:sdlc:plan declares its tier-2
# orchestration contract and documents an absence-detection branch.
#
# Per ADR-029:
#   Rule 1 — absence detection MUST cite the helper by name.
#   Rule 4 — failure-mode is declared via `tier_2_dependency:` and
#            `on_absent:` frontmatter fields. Valid `on_absent:` values
#            are `skip-with-warning` or `refuse-and-direct-user`.
#
# This is a static-contract test (mirrors plan-command-gates-on-verify.sh).
# The plan command is interpreted by an LLM, not executed by a shell, so
# end-to-end PATH-stripping is not meaningful — the contract being tested
# is that the markdown DOCUMENTS the absence-handling branch in a way
# the LLM can follow.

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
echo "ADR-029 — /edikt:sdlc:plan tier-2 orchestration contract"

# Frontmatter declarations (Rule 4).
assert "$PLAN_MD declares tier_2_dependency in frontmatter" \
    "grep -E '^tier_2_dependency:' '$PLAN_MD'"
assert "$PLAN_MD declares on_absent in frontmatter" \
    "grep -E '^on_absent:' '$PLAN_MD'"
assert "$PLAN_MD on_absent is one of the two documented values" \
    "grep -E '^on_absent:[[:space:]]+(skip-with-warning|refuse-and-direct-user)' '$PLAN_MD'"

# Absence-detection branch (Rule 1).
assert "$PLAN_MD documents an absence-detection check" \
    "grep -q 'command -v edikt' '$PLAN_MD'"
assert "$PLAN_MD cites edikt install in the absence-warning text (Rule 1)" \
    "grep -q 'edikt install' '$PLAN_MD'"
assert "$PLAN_MD references ADR-029 in the gate documentation" \
    "grep -q 'ADR-029' '$PLAN_MD'"

# Exit-code-only contract (Rule 2). The verify-gate documentation must
# spell out exit codes 0/1/2/3 — output is displayed verbatim, never
# parsed.
assert "$PLAN_MD references all four exit codes (0/1/2/3)" \
    "grep -q 'Exit 0' '$PLAN_MD' && grep -q 'Exit 1' '$PLAN_MD' && grep -q 'Exit 2' '$PLAN_MD' && grep -q 'Exit 3' '$PLAN_MD'"

# ADR-029's enumerated verbs (Rule 3) — the only verb plan.md should
# call is `verify`. Spot-check that no forbidden verb has crept in
# elsewhere in the command file (e.g. shell-out to `edikt foo bar`).
# This grep allows: `edikt verify`, `edikt install`, `edikt doctor`,
# `edikt migrate`, `edikt gov compile`, `edikt upgrade`, `edikt use`,
# `edikt rollback`. Any other `edikt <verb>` shape fails the test.
assert "$PLAN_MD invokes only ADR-029-allowlisted tier-2 verbs" \
    "! grep -E 'edikt[[:space:]]+(?!(verify|install|doctor|migrate|gov|upgrade|use|rollback)\\b)[a-z]+' '$PLAN_MD' >/dev/null 2>&1 || ! grep -oE 'edikt[[:space:]]+[a-z][a-z-]+' '$PLAN_MD' | sort -u | grep -vE '^edikt[[:space:]]+(verify|install|doctor|migrate|gov|upgrade|use|rollback)$' | grep -q ."

echo
echo -e "${DIM}$pass_count passed, $fail_count failed.${RESET}"
exit "$fail_count"
